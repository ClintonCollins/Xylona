package actions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
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
		ID:                 newGameServer.ID,
		FullCommandAndArgs: ParameterSubstitution(game.WindowsInstallCommand, newGameServer),
		WorkingDirectory:   gameServer.Directory,
		User:               gameServer.UserID,
		GameServerID:       &gameServer.ID,
		Status:             xylona.Status_INSTALLING,
		ServiceID:          gameServer.GameID,
		CallbackFunction: func(cmd *supervisor.Command) {
			errPostInstall := postInstallStep(newGameServer)
			if errPostInstall != nil {
				log.Error().Err(errPostInstall).Msg("Failed to perform post install step")
				return
			}
			inst.StartGameServer(newGameServer)
		},
	}

	_, err := inst.supervisorInstance.StartCommand(preparedCommand)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start install command")
		return nil, err
	}
	return newGameServer, nil
}

func ParameterSubstitution(str string, gameServer *models.GameServer) string {
	str = strings.ReplaceAll(str, "%GAMESERVER_DIRECTORY%", gameServer.Directory)
	str = strings.ReplaceAll(str, "%GAMESERVER_ID%", gameServer.ID)
	str = strings.ReplaceAll(str, "%GAMESERVER_BACKUP_DIRECTORY%", gameServer.BackupDirectory)
	str = strings.ReplaceAll(str, "%GAMESERVER_NAME%", gameServer.Name)
	str = strings.ReplaceAll(str, "%GAMESERVER_IP%", gameServer.IP)
	str = strings.ReplaceAll(str, "%GAMESERVER_PORT%", string(gameServer.Port))
	str = strings.ReplaceAll(str, "%GAMESERVER_QUERY_PORT%", string(gameServer.QueryPort))
	str = strings.ReplaceAll(str, "%GAMESERVER_MAX_MEMORY_MB%", fmt.Sprintf("%d", gameServer.MaxMemoryMB))
	str = strings.ReplaceAll(str, "%GAMESERVER_MAX_PLAYERS%", fmt.Sprintf("%d", gameServer.MaxPlayers))
	str = strings.ReplaceAll(str, "%GAMESERVER_SET_PLAYERS%", fmt.Sprintf("%d", gameServer.SetPlayers))

	return str
}

func postInstallStep(gameServer *models.GameServer) error {
	switch gameServer.GameID {
	case "minecraft":
		return postMinecraftInstall(gameServer)
	case "7_days_to_die":
		return post7DaysToDieInstall(gameServer)
	default:
		log.Debug().Str("Game ID", gameServer.GameID).Msg("No post install step")
		return nil
	}
}

func (inst *Instance) StartGameServer(gameServer *models.GameServer) {
	startCmd := ParameterSubstitution(gameServer.StartCommand, gameServer)
	preparedCommand := supervisor.PreparedCommand{
		ID:                 gameServer.ID,
		FullCommandAndArgs: startCmd,
		WorkingDirectory:   gameServer.Directory,
		User:               gameServer.UserID,
		GameServerID:       &gameServer.ID,
		Status:             xylona.Status_ONLINE,
		ServiceID:          gameServer.GameID,
		CallbackFunction: func(cmd *supervisor.Command) {
			//log.Info().Msg("Game server stopped")
		},
	}
	log.Debug().Msg("Checking input method during startup action")
	switch gameServer.GameID {
	case "7_days_to_die":
		log.Debug().Msg("Found 7 Days to Die. Setting input method to telnet")
		preparedCommand.InputMethod = supervisor.InputMethod{
			Type: supervisor.InputTypeTelnet,
			TelnetCredentials: &supervisor.TelnetCredentials{
				Port:     int(gameServer.Port + 1),
				Password: "",
			},
		}
	}

	_, err := inst.supervisorInstance.StartCommand(preparedCommand)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start game server")
	}
}

func (inst *Instance) StopGameServer(gameServer *models.GameServer) {
	gameServerCommand, err := inst.supervisorInstance.GetCommandByID(gameServer.ID)
	if err != nil {
		if errors.Is(err, supervisor.ErrCommandDoesNotExist) {
			log.Error().Err(err).Msg("Failed to get game server command")
		}
		return
	}
	gameServerCommand.Stop(gameServer.R.Game.WindowsStopCommand)
}

func (inst *Instance) ReadGameServerBuffer(gameServer *models.GameServer) string {
	gameServerCommand, err := inst.supervisorInstance.GetCommandByID(gameServer.ID)
	if err != nil {
		// log.Error().Err(err).Msg("Failed to get game server command")
		return ""
	}
	return gameServerCommand.GetOutputBuffer()
}

func (inst *Instance) RemoveGameServer(gameServer *models.GameServer, force bool) error {
	err := inst.DeleteGameServerFiles(gameServer)
	if err != nil {
		if !force {
			log.Error().Err(err).Msg("Failed to remove game server files")
			return err
		}
		log.Warn().Err(err).Msg("Failed to remove game server files")
	}
	err = inst.db.DeleteGameServer(gameServer.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to remove game server from database")
		return err
	}
	return nil
}

func (inst *Instance) DeleteGameServerFiles(gameServer *models.GameServer) error {
	err := os.RemoveAll(gameServer.Directory)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete game server files")
		return err
	}
	return nil
}
