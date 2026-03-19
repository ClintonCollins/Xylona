package db

import (
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) GetAllGameServers() ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.PreloadGameServerIP(),
		models.PreloadGameServerGame(),
		models.PreloadGameServerUser(),
		models.PreloadGameServerNode(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return gameServers, nil
}

func (c *Connection) GetGameServersByUser(userID string) ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.SelectWhere.GameServers.UserID.EQ(userID),
		models.PreloadGameServerIP(),
		models.PreloadGameServerGame(),
		models.PreloadGameServerUser(),
		models.PreloadGameServerNode(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return gameServers, nil
}

func (c *Connection) GetGameServerByID(gameServerID string) (*models.GameServer, error) {
	gameServer, err := models.GameServers.Query(
		models.SelectWhere.GameServers.ID.EQ(gameServerID),
		models.PreloadGameServerIP(),
		models.PreloadGameServerGame(),
		models.PreloadGameServerUser(),
		models.PreloadGameServerNode(),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return gameServer, nil
}

func (c *Connection) InsertGameServer(exec bob.Executor, gameServerSetter *models.GameServerSetter) (*models.GameServer, error) {
	gameServer, err := models.GameServers.Insert(gameServerSetter).One(c.ctx, exec)
	if err != nil {
		return nil, err
	}
	return gameServer, nil
}

func (c *Connection) UpdateGameServer(exec bob.Executor, gameServerSetter *models.GameServerSetter) (*models.GameServer, error) {
	log.Debug().Msgf("%v", gameServerSetter.IP)
	_, err := models.GameServers.Update(models.UpdateWhere.GameServers.ID.EQ(gameServerSetter.ID.MustGet()), gameServerSetter.UpdateMod()).One(c.ctx, exec)
	if err != nil {
		return nil, err
	}
	gameServer, errUpdatedGame := c.GetGameServerByID(gameServerSetter.ID.MustGet())
	if errUpdatedGame != nil {
		return nil, errUpdatedGame
	}
	return gameServer, nil
}

func (c *Connection) DeleteGameServer(gameServerID string) error {
	gs := models.GameServerSlice{&models.GameServer{ID: gameServerID}}
	return gs.DeleteAll(c.ctx, c.DB)
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
		return nil, errQuery
	}
	defer func() { _ = rows.Close() }()

	ownedIDs := make(map[string]struct{}, len(ownedServers))
	for _, gs := range ownedServers {
		ownedIDs[gs.ID] = struct{}{}
	}

	// Fetch granted servers that the user doesn't already own.
	var grantedServers []*models.GameServer
	for rows.Next() {
		var id string
		if errScan := rows.Scan(&id); errScan != nil {
			return nil, errScan
		}
		if _, alreadyOwned := ownedIDs[id]; alreadyOwned {
			continue
		}
		gs, errGet := c.GetGameServerByID(id)
		if errGet != nil {
			log.Warn().Err(errGet).Str("game_server_id", id).Msg("Skipping inaccessible granted server")
			continue
		}
		grantedServers = append(grantedServers, gs)
	}
	if errRows := rows.Err(); errRows != nil {
		return nil, errRows
	}

	return append(ownedServers, grantedServers...), nil
}

func (c *Connection) GetGameServersByIP(ip string) ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.SelectWhere.GameServers.IP.EQ(ip),
		models.PreloadGameServerIP(),
		models.PreloadGameServerGame(),
		models.PreloadGameServerUser(),
		models.PreloadGameServerNode(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return gameServers, nil
}

func (c *Connection) GetGameServersByGameID(gameID string) ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(
		models.SelectWhere.GameServers.GameID.EQ(gameID),
		models.PreloadGameServerIP(),
		models.PreloadGameServerGame(),
		models.PreloadGameServerUser(),
		models.PreloadGameServerNode(),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return gameServers, nil
}
