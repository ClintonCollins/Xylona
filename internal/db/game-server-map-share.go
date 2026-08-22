package db

import (
	"errors"
	"fmt"

	"github.com/ClintonCollins/Xylona/sql/models/dberrors"
)

// ErrGameServerMapShareIdentifierConflict indicates that another server uses the identifier.
var ErrGameServerMapShareIdentifierConflict = errors.New("game server map share identifier is already in use")

// GameServerMapShare stores the canonical public map link for one game server.
type GameServerMapShare struct {
	GameServerID     string
	PublicIdentifier string
	Enabled          bool
}

// GetOrCreateGameServerMapShare returns the existing settings or creates disabled settings.
func (c *Connection) GetOrCreateGameServerMapShare(gameServerID string, generatedIdentifier string) (*GameServerMapShare, error) {
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		`insert into game_server_map_share (game_server_id, public_identifier)
		 values (?, ?)
		 on conflict(game_server_id) do update set game_server_id = excluded.game_server_id
		 returning game_server_id, public_identifier, enabled`,
		gameServerID,
		generatedIdentifier,
	)
	share, errScan := scanGameServerMapShare(row)
	if dberrors.ErrUniqueConstraint.Is(errScan) {
		return nil, ErrGameServerMapShareIdentifierConflict
	}
	if errScan != nil {
		return nil, fmt.Errorf("get or create game server map share: %w", errScan)
	}
	return share, nil
}

// GetGameServerMapShareByGameServerID returns settings regardless of enablement.
func (c *Connection) GetGameServerMapShareByGameServerID(gameServerID string) (*GameServerMapShare, error) {
	return scanGameServerMapShare(c.SQLDb.QueryRowContext(
		c.ctx,
		`select game_server_id, public_identifier, enabled
		 from game_server_map_share
		 where game_server_id = ?`,
		gameServerID,
	))
}

// GetEnabledGameServerMapShareByIdentifier returns only an enabled current share.
func (c *Connection) GetEnabledGameServerMapShareByIdentifier(publicIdentifier string) (*GameServerMapShare, error) {
	return scanGameServerMapShare(c.SQLDb.QueryRowContext(
		c.ctx,
		`select game_server_id, public_identifier, enabled
		 from game_server_map_share
		 where public_identifier = ? and enabled = true`,
		publicIdentifier,
	))
}

// UpdateGameServerMapShare atomically replaces the identifier and enabled state.
func (c *Connection) UpdateGameServerMapShare(gameServerID string, publicIdentifier string, enabled bool) (*GameServerMapShare, error) {
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		`update game_server_map_share
		 set public_identifier = ?, enabled = ?
		 where game_server_id = ?
		 returning game_server_id, public_identifier, enabled`,
		publicIdentifier,
		enabled,
		gameServerID,
	)
	share, errScan := scanGameServerMapShare(row)
	if dberrors.ErrUniqueConstraint.Is(errScan) {
		return nil, ErrGameServerMapShareIdentifierConflict
	}
	if errScan != nil {
		return nil, fmt.Errorf("update game server map share: %w", errScan)
	}
	return share, nil
}

func scanGameServerMapShare(row scanner) (*GameServerMapShare, error) {
	var share GameServerMapShare
	errScan := row.Scan(
		&share.GameServerID,
		&share.PublicIdentifier,
		&share.Enabled,
	)
	if errScan != nil {
		return nil, fmt.Errorf("scan game server map share: %w", errScan)
	}
	return &share, nil
}
