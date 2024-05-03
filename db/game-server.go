package db

import "github.com/ClintonCollins/Xylona/sql/models"

func (c *Connection) GetAllGameServers() ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(c.ctx, c.DB,
		models.PreloadGameServerIP(),
		models.PreloadGameServerGame(),
		models.PreloadGameServerUser(),
	).All()
	if err != nil {
		return nil, err
	}
	return gameServers, nil
}

func (c *Connection) GetGameServersByUser(userID string) ([]*models.GameServer, error) {
	gameServers, err := models.GameServers.Query(c.ctx, c.DB,
		models.SelectWhere.GameServers.UserID.EQ(userID),
		models.PreloadGameServerIP(),
		models.PreloadGameServerGame(),
		models.PreloadGameServerUser(),
	).All()
	if err != nil {
		return nil, err
	}
	return gameServers, nil
}

func (c *Connection) GetGameServerByID(gameServerID string) (*models.GameServer, error) {
	gameServer, err := models.GameServers.Query(c.ctx, c.DB,
		models.SelectWhere.GameServers.ID.EQ(gameServerID),
		models.PreloadGameServerIP(),
		models.PreloadGameServerGame(),
		models.PreloadGameServerUser(),
	).One()
	if err != nil {
		return nil, err
	}
	return gameServer, nil
}

func (c *Connection) InsertGameServer(gameServerSetter *models.GameServerSetter) (*models.GameServer, error) {
	gameServer, err := models.GameServers.Insert(c.ctx, c.DB, gameServerSetter)
	if err != nil {
		return nil, err
	}
	return gameServer, nil
}

func (c *Connection) DeleteGameServer(gameServerID string) error {
	return models.GameServers.Delete(c.ctx, c.DB, &models.GameServer{ID: gameServerID})
}
