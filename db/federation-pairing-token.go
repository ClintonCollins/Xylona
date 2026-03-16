package db

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/stephenafamo/bob/dialect/sqlite"
)

const (
	pairingTokenLength = 32
	pairingTokenTTL    = 10 * time.Minute
)

// FederationPairingToken represents a one-time token used during node pairing.
type FederationPairingToken struct {
	ID        string
	TokenHash string
	TargetURL string
	CreatedAt time.Time
	ExpiresAt time.Time
	Used      bool
}

// GeneratePairingToken creates a new pairing token, stores its hash, and returns the
// plaintext token for the admin to copy to the remote node.
func (c *Connection) GeneratePairingToken(targetURL string) (string, error) {
	tokenBytes := make([]byte, pairingTokenLength)
	_, errRead := rand.Read(tokenBytes)
	if errRead != nil {
		return "", errRead
	}
	plaintext := hex.EncodeToString(tokenBytes)
	tokenHash := hashPairingToken(plaintext)

	id := uuid.New().String()
	expiresAt := time.Now().Add(pairingTokenTTL)

	_, errExec := sqlite.RawQuery(
		`insert into federation_pairing_token (id, token_hash, target_url, expires_at)
		 values (?, ?, ?, ?)`,
		id,
		tokenHash,
		targetURL,
		expiresAt,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		return "", errExec
	}

	return plaintext, nil
}

// ValidateAndConsumePairingToken checks the token against stored hashes. It returns the
// matching row if valid, non-expired, and unused, then marks it as used.
func (c *Connection) ValidateAndConsumePairingToken(plaintext string) (*FederationPairingToken, error) {
	return c.validateAndConsumePairingToken(plaintext, "", false)
}

// ValidateAndConsumePairingTokenForTarget checks a token and binds it to an expected target URL when present.
// If the token row contains a target_url, expectedTargetURL must normalize to the same value.
func (c *Connection) ValidateAndConsumePairingTokenForTarget(plaintext string, expectedTargetURL string) (*FederationPairingToken, error) {
	return c.validateAndConsumePairingToken(plaintext, expectedTargetURL, true)
}

func (c *Connection) validateAndConsumePairingToken(
	plaintext string,
	expectedTargetURL string,
	enforceTargetURL bool,
) (*FederationPairingToken, error) {
	tokenHash := hashPairingToken(plaintext)
	normalizedExpectedTargetURL, errNormalizeExpectedTarget := normalizePairingTargetURL(expectedTargetURL)
	if errNormalizeExpectedTarget != nil {
		return nil, errNormalizeExpectedTarget
	}

	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return nil, errBegin
	}
	defer func() {
		_ = tx.Rollback()
	}()

	token := &FederationPairingToken{}
	errQuery := tx.QueryRowContext(
		c.ctx,
		`select id, token_hash, target_url, created_at, expires_at, used
		 from federation_pairing_token
		 where token_hash = ?`,
		tokenHash,
	).Scan(
		&token.ID,
		&token.TokenHash,
		&token.TargetURL,
		&token.CreatedAt,
		&token.ExpiresAt,
		&token.Used,
	)
	if errQuery != nil {
		if errors.Is(errQuery, sql.ErrNoRows) {
			return nil, errors.New("invalid pairing token")
		}
		return nil, errQuery
	}

	if token.Used {
		return nil, errors.New("pairing token has already been used")
	}
	now := time.Now()
	if now.After(token.ExpiresAt) {
		return nil, errors.New("pairing token has expired")
	}

	normalizedStoredTargetURL, errNormalizeStoredTarget := normalizePairingTargetURL(token.TargetURL)
	if errNormalizeStoredTarget != nil {
		return nil, errors.New("stored pairing token target URL is invalid")
	}
	if enforceTargetURL && normalizedStoredTargetURL != "" {
		if normalizedExpectedTargetURL == "" {
			return nil, errors.New("pairing token target URL is required")
		}
		if normalizedStoredTargetURL != normalizedExpectedTargetURL {
			return nil, errors.New("pairing token target URL mismatch")
		}
	}

	result, errMark := tx.ExecContext(
		c.ctx,
		`update federation_pairing_token set used = true where id = ? and used = false and expires_at > ?`,
		token.ID,
		now,
	)
	if errMark != nil {
		return nil, errMark
	}

	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return nil, errRowsAffected
	}
	if rowsAffected != 1 {
		return nil, errors.New("pairing token is no longer valid")
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return nil, errCommit
	}

	return token, nil
}

// CleanupExpiredPairingTokens removes tokens that have expired or been used.
func (c *Connection) CleanupExpiredPairingTokens() error {
	_, errExec := sqlite.RawQuery(
		`delete from federation_pairing_token
		 where used = true or expires_at < current_timestamp`,
	).Exec(c.ctx, c.DB)
	return errExec
}

func hashPairingToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

func normalizePairingTargetURL(targetURL string) (string, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return "", nil
	}

	parsedURL, errParse := url.Parse(targetURL)
	if errParse != nil {
		return "", errors.New("invalid pairing token target URL")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsedURL.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", errors.New("invalid pairing token target URL")
	}

	hostName := strings.ToLower(strings.TrimSpace(parsedURL.Hostname()))
	if hostName == "" {
		return "", errors.New("invalid pairing token target URL")
	}
	port := strings.TrimSpace(parsedURL.Port())
	host := hostName
	if port != "" {
		host = net.JoinHostPort(hostName, port)
	}

	normalizedURL := url.URL{
		Scheme: scheme,
		Host:   host,
	}
	return normalizedURL.String(), nil
}
