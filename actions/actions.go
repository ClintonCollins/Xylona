package actions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/aarondl/opt/omit"
	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

var (
	ErrInvalidPath = errors.New("invalid path")
)

const (
	CommandTypeDirect     = "direct"
	CommandTypeBash       = "bash"
	CommandTypePowershell = "powershell"
	CommandTypeCmd        = "cmd"
	CommandTypeInternal   = "internal"
)

type Instance struct {
	ctx                  context.Context
	supervisorInstance   *supervisor.Instance
	serverQueriesInfoMap map[string]xylona.ServerQuery
	serverQueriesMutex   *sync.RWMutex
	db                   *db.Connection
}

func NewInstance(ctx context.Context, db *db.Connection, supervisorInstance *supervisor.Instance) *Instance {
	inst := &Instance{
		ctx:                  ctx,
		supervisorInstance:   supervisorInstance,
		serverQueriesInfoMap: make(map[string]xylona.ServerQuery),
		serverQueriesMutex:   &sync.RWMutex{},
		db:                   db,
	}
	go inst.backgroundJobQueryAllGameServers()
	return inst
}

func (inst *Instance) GetServerQueries() xylona.AllServersQueryInfo {
	inst.serverQueriesMutex.RLock()
	defer inst.serverQueriesMutex.RUnlock()
	allServerQueryInfo := xylona.AllServersQueryInfo{Servers: make(map[string]*xylona.ServerQuery)}
	for _, serverQuery := range inst.serverQueriesInfoMap {
		allServerQueryInfo.Servers[serverQuery.ServerId] = &serverQuery
	}
	return allServerQueryInfo
}

func (inst *Instance) ListGameServerFiles(gameServer *models.GameServer, path string) ([]*xylona.File, error) {
	// Check if path is empty or if it is a local path. If it is not a local path, return an error.
	if path != "" && !filepath.IsLocal(path) {
		log.Error().Err(errors.New("invalid path"))
		return nil, ErrInvalidPath
	}
	fullPath := filepath.Join(gameServer.Directory, path)
	files, errReadDir := os.ReadDir(fullPath)
	if errReadDir != nil {
		if errors.Is(errReadDir, os.ErrNotExist) {
			log.Error().Err(errReadDir).Msg("Path does not exist")
			return nil, errReadDir
		}
		log.Error().Err(errReadDir).Msg("Failed to read directory")
		return nil, errReadDir
	}

	xylonaFiles := make([]*xylona.File, 0, len(files))
	for _, file := range files {
		file := file
		fileInfo, errFileInfo := file.Info()
		if errFileInfo != nil {
			log.Error().Err(errFileInfo).Msg("Failed to get file info")
			return nil, errFileInfo
		}
		size := fileInfo.Size()
		if fileInfo.IsDir() {
			// TODO walk directory recursively to get size of directory

			var totalSize int64 = 0

			err := filepath.Walk(filepath.Join(fullPath, file.Name()), func(path string, fileInfo os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !fileInfo.IsDir() {
					totalSize += fileInfo.Size()
				}
				return nil
			})

			if err != nil {
				log.Error().Err(err).Msg("Failed to walk through directory")
				return nil, err
			}
			size = totalSize
		}
		xylonaFiles = append(xylonaFiles, &xylona.File{
			Name:         fileInfo.Name(),
			Size:         size,
			IsDirectory:  fileInfo.IsDir(),
			LastModified: timestamppb.New(fileInfo.ModTime()),
		})
	}

	return xylonaFiles, nil
}

func (inst *Instance) DownloadGameServerFile(w http.ResponseWriter, r *http.Request) {
	multiReader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Error creating multipart reader", http.StatusBadRequest)
		return
	}
	foundGameServerID := false
	foundPath := false
	gameServerID := ""
	path := ""
	for {
		part, errNext := multiReader.NextPart()
		if errNext == io.EOF {
			break
		} else {
			if errNext != nil {
				http.Error(w, "Error reading next part", http.StatusBadRequest)
				return
			}
		}
		switch part.FormName() {
		case "gameServerId":
			gameServerIDBytes, errRead := io.ReadAll(io.LimitReader(part, 10<<10))
			if errRead != nil {
				log.Error().Err(errRead).Msg("Failed to read game server ID")
				http.Error(w, "Error reading game server ID", http.StatusBadRequest)
				return
			}
			gameServerID = string(gameServerIDBytes)
			foundGameServerID = true
		case "path":
			pathBytes, errRead := io.ReadAll(io.LimitReader(part, 1<<20))
			if errRead != nil {
				log.Error().Err(errRead).Msg("Failed to read path")
				http.Error(w, "Error reading path", http.StatusBadRequest)
				return
			}
			path = string(pathBytes)
			foundPath = true
		case "file":
			if !foundGameServerID || !foundPath {
				log.Error().Msg("Game server ID and path must be specified")
				http.Error(w, "Game server ID and path must be specified", http.StatusBadRequest)
				return
			}
			filename := part.FileName()
			gameServer, errGetGameServer := inst.db.GetGameServerByID(gameServerID)
			if errGetGameServer != nil {
				if errors.Is(errGetGameServer, sql.ErrNoRows) {
					log.Error().Err(errGetGameServer).Msg("Game server not found")
					http.Error(w, "Game server not found", http.StatusNotFound)
					return
				}
				log.Error().Err(errGetGameServer).Msg("Failed to get game server")
				http.Error(w, "Failed to get game server", http.StatusInternalServerError)
				return
			}
			errDownload := inst.downloadGameServerFile(gameServer, path, filename, part)
			if errDownload != nil {
				log.Error().Err(errDownload).Msg("Failed to download file")
				http.Error(w, "Failed to download file", http.StatusInternalServerError)
				return
			}
		}
	}
}

func (inst *Instance) downloadGameServerFile(gameServer *models.GameServer, path, fileName string, fileSource io.Reader) error {
	if path != "" && !filepath.IsLocal(path) {
		log.Error().Err(errors.New("invalid path"))
		return ErrInvalidPath
	}

	gameServerDirPlusPath := filepath.Join(gameServer.Directory, path)
	errMkdirAll := os.MkdirAll(gameServerDirPlusPath, os.ModePerm)
	if errMkdirAll != nil {
		log.Error().Err(errMkdirAll).Msg("Failed to create directory")
		return errMkdirAll
	}

	fullPath := filepath.Join(gameServer.Directory, path, fileName)
	file, errCreateFile := os.Create(fullPath)
	if errCreateFile != nil {
		log.Error().Err(errCreateFile).Msg("Failed to create file")
		return errCreateFile
	}
	defer func() {
		_ = file.Close()
	}()

	_, errCopy := io.Copy(file, fileSource)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return errCopy
	}

	return nil
}

func (inst *Instance) GetGameServerFile(gameServer *models.GameServer, path string, writer io.Writer, setHeaders, setAsAttachment bool) error {
	// Check if path is empty or if it is a local path. If it is not a local path, return an error.
	if path != "" && !filepath.IsLocal(path) {
		log.Error().Err(errors.New("invalid path"))
		return ErrInvalidPath
	}
	// log.Debug().Msgf("Path %s is local: %t", path, filepath.IsLocal(path))
	fullPath := filepath.Join(gameServer.Directory, path)
	file, errReadFile := os.Open(fullPath)
	if errReadFile != nil {
		if errors.Is(errReadFile, os.ErrNotExist) {
			log.Error().Err(errReadFile).Msg("File does not exist")
			return errReadFile
		}
		log.Error().Err(errReadFile).Msg("Failed to read file")
		return errReadFile
	}
	defer func() { _ = file.Close() }()

	fileInfo, errFileInfo := file.Stat()
	if errFileInfo != nil {
		log.Error().Err(errFileInfo).Msg("Failed to get file info")
		return errFileInfo
	}

	if setHeaders {
		w, ok := writer.(http.ResponseWriter)
		if !ok {
			log.Error().Msg("Writer is not an http.ResponseWriter")
			return errors.New("writer is not an http.ResponseWriter")
		}

		mimeType, errDetect := mimetype.DetectFile(fullPath)
		if errDetect != nil {
			w.Header().Set("Content-Type", "application/octet-stream")
		} else {
			w.Header().Set("Content-Type", mimeType.String())
		}

		w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
		if setAsAttachment {
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileInfo.Name()))
		}
	}

	_, errCopy := io.Copy(writer, file)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return errCopy
	}
	return nil
}

func (inst *Instance) createGameServerDirectory(gameServer *models.GameServer, owner *models.User) (string, error) {
	gsNameSlug := slug.Make(gameServer.Name)
	gameServerDir := filepath.Join(DefaultInstallPath(), owner.UserName, gsNameSlug)
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

	t, errTx := inst.db.SQLDb.BeginTx(inst.ctx, nil)
	if errTx != nil {
		log.Error().Err(errTx).Msg("Failed to start transaction")
		return nil, errTx
	}
	tx := bob.NewTx(t)

	defer func() {
		_ = tx.Rollback()
	}()

	node, errGetNode := inst.db.GetNodeByID(gameServer.NodeID)
	if errGetNode != nil {
		log.Error().Err(errGetNode).Msg("Failed to get node")
		return nil, errGetNode
	}

	newGameServer, errInsert := inst.db.InsertGameServer(tx, &models.GameServerSetter{
		ID:                        omit.From(uuid.NewString()),
		UserID:                    omit.From(owner.ID),
		Name:                      omit.From(gameServer.Name),
		GameID:                    omit.From(game.ID),
		StartCommand:              omit.From(gameStartCommand(game)),
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
		NodeID:                    omit.From(gameServer.NodeID),
	})
	newGameServer.R.Node = node
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Failed to insert game server")
		return nil, errInsert
	}

	preparedCommand := supervisor.PreparedCommand{
		ID:                 newGameServer.ID,
		FullCommandAndArgs: ParameterSubstitution(gameInstallCommand(game), newGameServer),
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

	if gameInstallCommandType(game) == CommandTypeInternal {
		preparedCommand.InternalCommand = true
		preparedCommand.InternalGameServer = newGameServer
		preparedCommand.GameID = &game.ID
	}

	_, err := inst.supervisorInstance.StartCommand(preparedCommand)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start install command")
		return nil, err
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		log.Error().Str("Game Server ID", gameServer.ID).Err(errCommit).Msg("Failed to commit transaction")
		return nil, errCommit
	}

	return newGameServer, nil
}

func ParameterSubstitution(str string, gameServer *models.GameServer) string {
	str = strings.ReplaceAll(str, "%GAMESERVER_DIRECTORY%", gameServer.Directory)
	str = strings.ReplaceAll(str, "%GAMESERVER_ID%", gameServer.ID)
	str = strings.ReplaceAll(str, "%GAMESERVER_BACKUP_DIRECTORY%", gameServer.BackupDirectory)
	str = strings.ReplaceAll(str, "%GAMESERVER_NAME%", gameServer.Name)
	str = strings.ReplaceAll(str, "%GAMESERVER_IP%", gameServer.IP)
	str = strings.ReplaceAll(str, "%GAMESERVER_PORT%", strconv.FormatInt(gameServer.Port, 10))
	str = strings.ReplaceAll(str, "%GAMESERVER_QUERY_PORT%", strconv.FormatInt(gameServer.QueryPort, 10))
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
	gameServerCommand.Stop(gameStopCommand(gameServer.R.Game))
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
	err := inst.PurgeAllGameServerFiles(gameServer)
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

func (inst *Instance) PurgeAllGameServerFiles(gameServer *models.GameServer) error {
	err := os.RemoveAll(gameServer.Directory)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete game server files")
		return err
	}
	return nil
}
