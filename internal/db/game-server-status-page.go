package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models/dberrors"
)

var (
	ErrGameServerStatusPageIdentifierConflict = errors.New("game server status page identifier is already reserved")
	ErrGameServerStatusPageServerNotOwned     = errors.New("game server does not belong to the status page owner")
)

// GameServerStatusPage is the persisted owner-level public status page.
type GameServerStatusPage struct {
	UserID           string
	PublicIdentifier string
	Title            string
	Enabled          bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// GameServerStatusPageUpdate replaces all mutable page settings atomically.
type GameServerStatusPageUpdate struct {
	UserID              string
	PublicIdentifier    string
	Title               string
	Enabled             bool
	ConnectionAddresses map[string]string
}

// CreateGameServerStatusPage reserves an identifier and creates one page in a transaction.
func (c *Connection) CreateGameServerStatusPage(userID string, title string, identifier string) (*GameServerStatusPage, error) {
	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return nil, fmt.Errorf("begin game server status page create: %w", errBegin)
	}
	committed := false
	defer rollbackTxIfNeeded(tx, &committed, "game server status page create")

	_, errReserve := tx.ExecContext(
		c.ctx,
		`insert into game_server_status_page_identifier (identifier) values (?)`,
		identifier,
	)
	if errReserve != nil {
		if errors.Is(dberrors.ErrUniqueConstraint, errReserve) {
			return nil, ErrGameServerStatusPageIdentifierConflict
		}
		return nil, fmt.Errorf("reserve game server status page identifier: %w", errReserve)
	}

	_, errInsert := tx.ExecContext(
		c.ctx,
		`insert into game_server_status_page (user_id, public_identifier, title) values (?, ?, ?)`,
		userID,
		identifier,
		title,
	)
	if errInsert != nil {
		return nil, fmt.Errorf("insert game server status page: %w", errInsert)
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return nil, fmt.Errorf("commit game server status page create: %w", errCommit)
	}
	committed = true
	return c.GetGameServerStatusPageByUserID(userID)
}

// GetGameServerStatusPageByUserID returns an owner's page regardless of enablement.
func (c *Connection) GetGameServerStatusPageByUserID(userID string) (*GameServerStatusPage, error) {
	return scanGameServerStatusPage(c.SQLDb.QueryRowContext(
		c.ctx,
		`select user_id, public_identifier, title, enabled, created_at, updated_at
		 from game_server_status_page
		 where user_id = ?`,
		userID,
	))
}

// GetEnabledGameServerStatusPageByIdentifier returns only an enabled current page.
func (c *Connection) GetEnabledGameServerStatusPageByIdentifier(identifier string) (*GameServerStatusPage, error) {
	return scanGameServerStatusPage(c.SQLDb.QueryRowContext(
		c.ctx,
		`select user_id, public_identifier, title, enabled, created_at, updated_at
		 from game_server_status_page
		 where public_identifier = ? and enabled = true`,
		identifier,
	))
}

// ListEnabledGameServerStatusPages returns all current indexable pages.
func (c *Connection) ListEnabledGameServerStatusPages() ([]GameServerStatusPage, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`select user_id, public_identifier, title, enabled, created_at, updated_at
		 from game_server_status_page
		 where enabled = true
		 order by public_identifier collate binary`,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("list enabled game server status pages: %w", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close game server status page rows")
		}
	}()

	pages := make([]GameServerStatusPage, 0)
	for rows.Next() {
		var page GameServerStatusPage
		errScan := rows.Scan(
			&page.UserID,
			&page.PublicIdentifier,
			&page.Title,
			&page.Enabled,
			&page.CreatedAt,
			&page.UpdatedAt,
		)
		if errScan != nil {
			return nil, fmt.Errorf("scan enabled game server status page: %w", errScan)
		}
		pages = append(pages, page)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate enabled game server status pages: %w", errRows)
	}
	return pages, nil
}

// UpdateGameServerStatusPage replaces the page and all owned-server address overrides.
func (c *Connection) UpdateGameServerStatusPage(update GameServerStatusPageUpdate) (*GameServerStatusPage, error) {
	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return nil, fmt.Errorf("begin game server status page update: %w", errBegin)
	}
	committed := false
	defer rollbackTxIfNeeded(tx, &committed, "game server status page update")

	var currentIdentifier string
	errCurrent := tx.QueryRowContext(
		c.ctx,
		`select public_identifier from game_server_status_page where user_id = ?`,
		update.UserID,
	).Scan(&currentIdentifier)
	if errCurrent != nil {
		return nil, fmt.Errorf("get game server status page for update: %w", errCurrent)
	}

	if update.PublicIdentifier != currentIdentifier {
		_, errReserve := tx.ExecContext(
			c.ctx,
			`insert into game_server_status_page_identifier (identifier) values (?)`,
			update.PublicIdentifier,
		)
		if errReserve != nil {
			if errors.Is(dberrors.ErrUniqueConstraint, errReserve) {
				return nil, ErrGameServerStatusPageIdentifierConflict
			}
			return nil, fmt.Errorf("reserve updated game server status page identifier: %w", errReserve)
		}
	}

	_, errPage := tx.ExecContext(
		c.ctx,
		`update game_server_status_page
		 set public_identifier = ?, title = ?, enabled = ?, updated_at = current_timestamp
		 where user_id = ?`,
		update.PublicIdentifier,
		update.Title,
		update.Enabled,
		update.UserID,
	)
	if errPage != nil {
		return nil, fmt.Errorf("update game server status page: %w", errPage)
	}

	_, errReset := tx.ExecContext(
		c.ctx,
		`update game_server set public_connection_address = null where user_id = ?`,
		update.UserID,
	)
	if errReset != nil {
		return nil, fmt.Errorf("reset game server public addresses: %w", errReset)
	}

	for gameServerID, address := range update.ConnectionAddresses {
		normalized := strings.TrimSpace(address)
		if normalized == "" {
			continue
		}
		result, errAddress := tx.ExecContext(
			c.ctx,
			`update game_server
			 set public_connection_address = ?
			 where id = ? and user_id = ?`,
			normalized,
			gameServerID,
			update.UserID,
		)
		if errAddress != nil {
			return nil, fmt.Errorf("update game server public address: %w", errAddress)
		}
		rowsAffected, errRowsAffected := result.RowsAffected()
		if errRowsAffected != nil {
			return nil, fmt.Errorf("check game server public address update: %w", errRowsAffected)
		}
		if rowsAffected != 1 {
			return nil, ErrGameServerStatusPageServerNotOwned
		}
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return nil, fmt.Errorf("commit game server status page update: %w", errCommit)
	}
	committed = true
	return c.GetGameServerStatusPageByUserID(update.UserID)
}

func scanGameServerStatusPage(row *sql.Row) (*GameServerStatusPage, error) {
	var page GameServerStatusPage
	errScan := row.Scan(
		&page.UserID,
		&page.PublicIdentifier,
		&page.Title,
		&page.Enabled,
		&page.CreatedAt,
		&page.UpdatedAt,
	)
	if errScan != nil {
		return nil, errScan
	}
	return &page, nil
}
