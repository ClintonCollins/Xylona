package db

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// ErrJoinTokenInvalid is returned when a bootstrap join token lookup fails
// because the token is unknown, already consumed, or expired.
var ErrJoinTokenInvalid = errors.New("join token is invalid, consumed, or expired")

// joinTokenByteLen is the number of random bytes in a generated token. 32
// bytes (256 bits) is comfortably beyond any brute-force attack even against
// the hashed storage form.
const joinTokenByteLen = 32

// joinTokenDefaultTTL bounds how long an unused token remains valid. Two
// hours leaves room for a human operator to paste the token into their
// node's terminal without forcing a round trip on the happy path.
const joinTokenDefaultTTL = 2 * time.Hour

// GenerateNodeJoinToken creates a new bootstrap token row and returns the
// plaintext token string (only ever returned here — the DB stores just its
// SHA-256 hash) plus the token row id. Pass an empty nodeName to leave the
// node label unset; the node binary can send its desired name during
// bootstrap.
func (c *Connection) GenerateNodeJoinToken(nodeName string, ttl time.Duration) (string, string, error) {
	if ttl <= 0 {
		ttl = joinTokenDefaultTTL
	}

	rawToken := make([]byte, joinTokenByteLen)
	_, errRand := rand.Read(rawToken)
	if errRand != nil {
		return "", "", fmt.Errorf("generate join token bytes: %w", errRand)
	}
	token := hex.EncodeToString(rawToken)
	tokenHash := hashJoinToken(token)

	id, errID := generateJoinTokenID()
	if errID != nil {
		return "", "", errID
	}
	expiresAt := time.Now().UTC().Add(ttl)

	setter := &models.NodeJoinTokenSetter{
		ID:        omit.From(id),
		TokenHash: omit.From(tokenHash),
		NodeName:  omit.From(nodeName),
		ExpiresAt: omitnull.From(expiresAt),
		CreatedAt: omit.From(time.Now().UTC()),
	}
	_, errInsert := models.NodeJoinTokens.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		return "", "", fmt.Errorf("insert node_join_token: %w", errInsert)
	}

	return token, id, nil
}

// ConsumeNodeJoinToken marks the row matching plainToken as consumed by the
// given node id. It returns the token row (with its originally-requested
// node_name) so the controller can pass the name on to the node row. The
// lookup compares the hex-encoded SHA-256 of the presented token and the
// UPDATE is guarded by consumed_at IS NULL so double-use is a no-op.
func (c *Connection) ConsumeNodeJoinToken(plainToken string, consumedByNodeID string) (*models.NodeJoinToken, error) {
	hash := hashJoinToken(plainToken)
	now := time.Now().UTC()

	tx, errTx := c.SQLDb.BeginTx(c.ctx, nil)
	if errTx != nil {
		return nil, fmt.Errorf("begin consume tx: %w", errTx)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	row := tx.QueryRowContext(c.ctx,
		`select id, node_name, created_at, expires_at, consumed_at from node_join_token where token_hash = ?`,
		hash,
	)
	var (
		id         string
		nodeName   string
		createdAt  time.Time
		expiresAt  *time.Time
		consumedAt *time.Time
	)
	errScan := row.Scan(&id, &nodeName, &createdAt, &expiresAt, &consumedAt)
	if errScan != nil {
		return nil, ErrJoinTokenInvalid
	}
	if consumedAt != nil {
		return nil, ErrJoinTokenInvalid
	}
	if expiresAt != nil && now.After(*expiresAt) {
		return nil, ErrJoinTokenInvalid
	}

	updateResult, errUpdate := tx.ExecContext(c.ctx,
		`update node_join_token set consumed_at = ?, consumed_by_node_id = ? where id = ? and consumed_at is null`,
		now, consumedByNodeID, id,
	)
	if errUpdate != nil {
		return nil, fmt.Errorf("mark token consumed: %w", errUpdate)
	}
	rowsAffected, errRowsAffected := updateResult.RowsAffected()
	if errRowsAffected != nil {
		return nil, fmt.Errorf("count consumed join token rows: %w", errRowsAffected)
	}
	if rowsAffected == 0 {
		return nil, ErrJoinTokenInvalid
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return nil, fmt.Errorf("commit consume tx: %w", errCommit)
	}

	return &models.NodeJoinToken{
		ID:               id,
		TokenHash:        hash,
		NodeName:         nodeName,
		CreatedAt:        createdAt,
		ConsumedAt:       null.From(now),
		ConsumedByNodeID: null.From(consumedByNodeID),
	}, nil
}

// DeleteExpiredJoinTokens prunes rows older than the cutoff. Meant to be run
// on a timer from main.go; returns the number of rows deleted for logging.
func (c *Connection) DeleteExpiredJoinTokens(cutoff time.Time) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(c.ctx,
		`delete from node_join_token where (expires_at is not null and expires_at < ?) or (consumed_at is not null and consumed_at < ?)`,
		cutoff, cutoff,
	)
	if errExec != nil {
		return 0, fmt.Errorf("delete expired join tokens: %w", errExec)
	}
	affected, errAffected := result.RowsAffected()
	if errAffected != nil {
		return 0, fmt.Errorf("count deleted join tokens: %w", errAffected)
	}
	return affected, nil
}

// hashJoinToken is a pure helper to keep hashing consistent across generate
// and consume call sites.
func hashJoinToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// generateJoinTokenID returns a random id for the token row. A 16-byte hex
// string is plenty of entropy for a short-lived DB row and avoids pulling in
// the uuid helper just for this.
func generateJoinTokenID() (string, error) {
	idBytes := make([]byte, 16)
	_, errRand := rand.Read(idBytes)
	if errRand != nil {
		return "", fmt.Errorf("generate join token id: %w", errRand)
	}
	return hex.EncodeToString(idBytes), nil
}
