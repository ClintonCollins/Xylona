package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

var (
	DefaultInstallPath = fmt.Sprintf("%s/Xylona", os.Getenv("USERPROFILE"))
	InstallTimeout     = 2 * time.Hour
)

func (inst *Instance) createGameServerDirectory(gameServer *models.GameServer, owner *models.User) (string, error) {
	gameServerDir := filepath.Join(DefaultInstallPath, owner.UserName, gameServer.Name)
	errMakePath := os.MkdirAll(gameServerDir, os.ModePerm)
	if errMakePath != nil {
		log.Error().Err(errMakePath).Msg("Failed to create game server directory")
		return "", errMakePath
	}
	return gameServerDir, nil
}

func (inst *Instance) InstallGameServer(game *models.Game, gameServer *models.GameServer, owner *models.User) (*models.GameServer, error) {
	gameServerDir, errCreateGameServerDir := inst.createGameServerDirectory(gameServer, owner)
	if errCreateGameServerDir != nil {
		log.Error().Err(errCreateGameServerDir).Msg("Failed to create game server directory")
		return nil, errCreateGameServerDir
	}
	gameServer.Directory = gameServerDir

	newGameServer, errInsert := inst.db.InsertGameServer(&models.GameServerSetter{
		ID:                        omit.From(uuid.NewString()),
		UserID:                    omit.From(owner.ID),
		Name:                      omit.From(gameServer.Name),
		GameID:                    omit.From(game.ID),
		StartCommand:              omit.From(game.WindowsStartCommand),
		Status:                    omit.From(""),
		SetPlayers:                omit.From(gameServer.SetPlayers),
		MaxPlayers:                omit.From(gameServer.MaxPlayers),
		Map:                       omit.From(gameServer.Map),
		IP:                        omit.From(gameServer.IP),
		Port:                      omit.From(gameServer.Port),
		QueryPort:                 omit.From(gameServer.QueryPort),
		Directory:                 omit.From(gameServer.Directory),
		MaxMemoryMB:               omit.From(gameServer.MaxMemoryMB),
		BackupsEnabled:            omit.From(gameServer.BackupsEnabled),
		SteamGameServerLoginToken: omit.From(gameServer.SteamGameServerLoginToken),
		BackupDirectory:           omit.From(gameServer.BackupDirectory),
		MaxBackups:                omit.From(gameServer.MaxBackups),
	})
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Failed to insert game server")
		return nil, errInsert
	}

	preparedCommand := supervisor.PreparedCommand{
		ID:                 gameServer.ID,
		FullCommandAndArgs: game.WindowsInstallCommand,
		WorkingDirectory:   gameServer.Directory,
		User:               gameServer.UserID,
		GameServerID:       &gameServer.ID,
		CallbackFunction: func(cmd *supervisor.Command) {
			// inst.StartGameServer(gameServer)
		},
	}
	_, err := inst.supervisorInstance.StartCommand(preparedCommand)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start install command")
		return nil, err
	}
	return newGameServer, nil
}

func (inst *Instance) StartGameServer(gameServer *models.GameServer) {
	preparedCommand := supervisor.PreparedCommand{
		ID:                 gameServer.ID,
		FullCommandAndArgs: gameServer.StartCommand,
		WorkingDirectory:   gameServer.Directory,
		User:               gameServer.UserID,
		GameServerID:       &gameServer.ID,
		CallbackFunction: func(cmd *supervisor.Command) {
			log.Info().Msg("Game server stopped")
		},
	}
	_, err := inst.supervisorInstance.StartCommand(preparedCommand)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start game server")
	}
}

func (inst *Instance) StopGameServer(gameServer *models.GameServer) {
	gameServerCommand, err := inst.supervisorInstance.GetCommandByID(gameServer.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get game server command")
		return
	}
	gameServerCommand.Stop()
}

func (inst *Instance) ReadGameServerBuffer(gameServer *models.GameServer) string {
	gameServerCommand, err := inst.supervisorInstance.GetCommandByID(gameServer.ID)
	if err != nil {
		// log.Error().Err(err).Msg("Failed to get game server command")
		return ""
	}
	return gameServerCommand.GetOutputBuffer()
}
