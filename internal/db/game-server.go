package db

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetAllGameServers returns all game servers.
func (c *Connection) GetAllGameServers() ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.Preload.GameServer.IP(),
		models.Preload.GameServer.Game(),
		models.Preload.GameServer.User(),
		models.Preload.GameServer.Node(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get all game servers: %w", err)
	}
	return gameServers, nil
}

// GetGameServersByNodeID returns all game servers assigned to a specific node.
func (c *Connection) GetGameServersByNodeID(nodeID string) ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.SelectWhere.GameServers.NodeID.EQ(nodeID),
		models.Preload.GameServer.IP(),
		models.Preload.GameServer.Game(),
		models.Preload.GameServer.User(),
		models.Preload.GameServer.Node(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get game servers by node ID: %w", err)
	}
	return gameServers, nil
}

// GetGameServersByUser returns game servers owned by a user.
func (c *Connection) GetGameServersByUser(userID string) ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.SelectWhere.GameServers.UserID.EQ(userID),
		models.Preload.GameServer.IP(),
		models.Preload.GameServer.Game(),
		models.Preload.GameServer.User(),
		models.Preload.GameServer.Node(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get game servers by user: %w", err)
	}
	return gameServers, nil
}

// GetGameServerByID returns a game server by ID.
func (c *Connection) GetGameServerByID(gameServerID string) (*models.GameServer, error) {
	gameServer, err := models.GameServers.Query(
		models.SelectWhere.GameServers.ID.EQ(gameServerID),
		models.Preload.GameServer.IP(),
		models.Preload.GameServer.Game(),
		models.Preload.GameServer.User(),
		models.Preload.GameServer.Node(),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get game server by ID: %w", err)
	}
	return gameServer, nil
}

// InsertGameServer inserts a new game server record.
func (c *Connection) InsertGameServer(exec bob.Executor, gameServerSetter *models.GameServerSetter) (*models.GameServer, error) {
	gameServer, err := models.GameServers.Insert(gameServerSetter).One(c.ctx, exec)
	if err != nil {
		return nil, fmt.Errorf("insert game server: %w", err)
	}
	return gameServer, nil
}

// UpdateGameServer updates an existing game server record.
func (c *Connection) UpdateGameServer(exec bob.Executor, gameServerSetter *models.GameServerSetter) (*models.GameServer, error) {
	_, err := models.GameServers.Update(models.UpdateWhere.GameServers.ID.EQ(gameServerSetter.ID.MustGet()), gameServerSetter.UpdateMod()).One(c.ctx, exec)
	if err != nil {
		return nil, fmt.Errorf("update game server: %w", err)
	}
	gameServer, errUpdatedGame := c.GetGameServerByID(gameServerSetter.ID.MustGet())
	if errUpdatedGame != nil {
		return nil, errUpdatedGame
	}
	return gameServer, nil
}

// UpdateGameServerForEdit applies a complete edit and clears public status details
// in the same transaction when the owner changes.
func (c *Connection) UpdateGameServerForEdit(gameServerSetter *models.GameServerSetter, previousOwnerID string) (*models.GameServer, error) {
	if !gameServerSetter.ID.IsValue() || !gameServerSetter.UserID.IsValue() {
		return nil, errors.New("update game server for edit: ID and user ID are required")
	}

	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return nil, fmt.Errorf("begin game server edit: %w", errBegin)
	}
	committed := false
	defer rollbackTxIfNeeded(tx, &committed, "game server edit")

	gameServerID := gameServerSetter.ID.MustGet()
	if gameServerSetter.UserID.MustGet() != previousOwnerID {
		_, errClear := tx.ExecContext(
			c.ctx,
			`update game_server
			 set public_connection_address = null, public_status_note = null, public_status_password = null
			 where id = ? and user_id = ?`,
			gameServerID,
			previousOwnerID,
		)
		if errClear != nil {
			return nil, fmt.Errorf("clear transferred game server public status details: %w", errClear)
		}
	}

	_, errUpdate := models.GameServers.Update(
		models.UpdateWhere.GameServers.ID.EQ(gameServerID),
		gameServerSetter.UpdateMod(),
	).One(c.ctx, bob.NewTx(tx))
	if errUpdate != nil {
		return nil, fmt.Errorf("update game server for edit: %w", errUpdate)
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return nil, fmt.Errorf("commit game server edit: %w", errCommit)
	}
	committed = true
	return c.GetGameServerByID(gameServerID)
}

// DeleteGameServer deletes a game server by ID.
func (c *Connection) DeleteGameServer(gameServerID string) error {
	gs := models.GameServerSlice{&models.GameServer{ID: gameServerID}}
	errWrap := gs.DeleteAll(c.ctx, c.DB)
	if errWrap != nil {
		return fmt.Errorf("delete game server: %w", errWrap)
	}
	return nil
}

// GetGameServersAccessibleByUser returns game servers that a user either owns
// or has any RBAC role assignment on (including global role assignments).
func (c *Connection) GetGameServersAccessibleByUser(userID string) ([]*models.GameServer, error) {
	// Get servers the user owns.
	ownedServers, errOwned := c.GetGameServersByUser(userID)
	if errOwned != nil {
		return nil, errOwned
	}

	// Get server IDs the user has RBAC grants on.
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`SELECT DISTINCT ura.game_server_id FROM user_role_assignment ura
		 WHERE ura.user_id = ? AND ura.game_server_id IS NOT NULL`,
		userID,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("query granted game server IDs: %w", errQuery)
	}
	defer func() {
		if errClose := rows.Close(); errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close rows in GetGameServersAccessibleByUser")
		}
	}()

	ownedIDs := make(map[string]struct{}, len(ownedServers))
	for _, gs := range ownedServers {
		ownedIDs[gs.ID] = struct{}{}
	}

	// Collect non-owned granted server IDs.
	var grantedIDs []string
	for rows.Next() {
		var id string
		if errScan := rows.Scan(&id); errScan != nil {
			return nil, fmt.Errorf("scan granted game server ID: %w", errScan)
		}
		if _, alreadyOwned := ownedIDs[id]; alreadyOwned {
			continue
		}
		grantedIDs = append(grantedIDs, id)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate granted game server IDs: %w", errRows)
	}

	if len(grantedIDs) == 0 {
		return ownedServers, nil
	}

	// Batch-fetch all granted servers in a single query.
	grantedServers, errGranted := models.GameServers.Query(
		models.SelectWhere.GameServers.ID.In(grantedIDs...),
		models.Preload.GameServer.IP(),
		models.Preload.GameServer.Game(),
		models.Preload.GameServer.User(),
		models.Preload.GameServer.Node(),
	).All(c.ctx, c.DB)
	if errGranted != nil {
		return nil, fmt.Errorf("get granted game servers: %w", errGranted)
	}

	return append(ownedServers, grantedServers...), nil
}

// GetGameServersByIP returns game servers bound to an IP address.
func (c *Connection) GetGameServersByIP(ip string) ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.SelectWhere.GameServers.IP.EQ(ip),
		models.Preload.GameServer.IP(),
		models.Preload.GameServer.Game(),
		models.Preload.GameServer.User(),
		models.Preload.GameServer.Node(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get game servers by IP: %w", err)
	}
	return gameServers, nil
}

// GetGameServersByNodeIDAndIP returns game servers bound to an IP address on a node.
func (c *Connection) GetGameServersByNodeIDAndIP(nodeID string, ip string) ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.SelectWhere.GameServers.NodeID.EQ(nodeID),
		models.SelectWhere.GameServers.IP.EQ(ip),
		models.Preload.GameServer.IP(),
		models.Preload.GameServer.Game(),
		models.Preload.GameServer.User(),
		models.Preload.GameServer.Node(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get game servers by node ID and IP: %w", err)
	}
	return gameServers, nil
}

// GetGameServersByGameID returns all servers for a game definition.
func (c *Connection) GetGameServersByGameID(gameID string) ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.SelectWhere.GameServers.GameID.EQ(gameID),
		models.Preload.GameServer.IP(),
		models.Preload.GameServer.Game(),
		models.Preload.GameServer.User(),
		models.Preload.GameServer.Node(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get game servers by game ID: %w", err)
	}
	return gameServers, nil
}
