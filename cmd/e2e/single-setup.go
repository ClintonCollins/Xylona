package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func minecraftE2EVariants() []*xylona.Variant {
	return []*xylona.Variant{
		{
			Id:   "vanilla",
			Name: "Vanilla",
			UpdateProvider: &xylona.UpdateProviderConfig{
				Kind: xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_COMMAND,
			},
		},
		{
			Id:            "paper",
			Name:          "Paper",
			DefaultTarget: "1.21.4",
			UpdateProvider: &xylona.UpdateProviderConfig{
				Kind:     xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_PAPERMC,
				SourceId: "paper",
			},
			ModProfile: &xylona.ModProfile{
				InstallPath: "plugins/",
				Sources: []*xylona.ModSource{
					{
						Id:               "modrinth",
						SearchParamsJson: `{"facets":{"project_type":"plugin","categories":["paper","spigot","bukkit"]}}`,
					},
					{
						Id:               "hangar",
						SearchParamsJson: `{"platform":"PAPER"}`,
					},
				},
			},
		},
		{
			Id:            "purpur",
			Name:          "Purpur",
			DefaultTarget: "1.21.4",
			UpdateProvider: &xylona.UpdateProviderConfig{
				Kind:     xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_PAPERMC,
				SourceId: "purpur",
			},
			ModProfile: &xylona.ModProfile{
				InstallPath: "plugins/",
				Sources: []*xylona.ModSource{
					{
						Id:               "modrinth",
						SearchParamsJson: `{"facets":{"project_type":"plugin","categories":["purpur","paper","spigot","bukkit"]}}`,
					},
					{
						Id:               "hangar",
						SearchParamsJson: `{"platform":"PAPER"}`,
					},
				},
			},
		},
		{
			Id:   "fabric",
			Name: "Fabric",
			UpdateProvider: &xylona.UpdateProviderConfig{
				Kind: xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_COMMAND,
			},
			ModProfile: &xylona.ModProfile{
				InstallPath: "mods/",
				Sources: []*xylona.ModSource{
					{
						Id:               "modrinth",
						SearchParamsJson: `{"facets":{"project_type":"mod","categories":["fabric"]}}`,
					},
				},
			},
		},
	}
}

var testUserDefs = []struct {
	userName  string
	email     string
	password  string
	firstName string
	lastName  string
	superUser bool
}{
	{userName: "e2e-superuser", email: "e2e-superuser@test.local", password: "TestPassword123!", firstName: "E2E", lastName: "Superuser", superUser: true},
	{userName: "e2e-operator", email: "e2e-operator@test.local", password: "TestPassword123!", firstName: "E2E", lastName: "Operator", superUser: false},
	{userName: "e2e-viewer", email: "e2e-viewer@test.local", password: "TestPassword123!", firstName: "E2E", lastName: "Viewer", superUser: false},
	{userName: "e2e-noaccess", email: "e2e-noaccess@test.local", password: "TestPassword123!", firstName: "E2E", lastName: "NoAccess", superUser: false},
}

const e2eDummyStartTemplateJSON = `[{"id":"heartbeat","order":0,"ownership":"system","tokens":["-heartbeat","5s"],"label":"Heartbeat"}]`

func runSingleSetup(ctx context.Context, httpPort int, adminUsername, adminPassword, e2eDir, projectRoot string) error {
	backendURL := fmt.Sprintf("http://localhost:%d", httpPort)
	log.Info().Msgf("[E2E Setup] Target backend: %s", backendURL)

	// Acquire lock to prevent concurrent runs.
	errLock := acquireLock(e2eDir, "single", map[string]int{
		"http": httpPort,
	})
	if errLock != nil {
		return fmt.Errorf("acquire lock: %w", errLock)
	}
	setupOK := false
	defer func() {
		if !setupOK {
			releaseLock(e2eDir, "single")
		}
	}()

	// Prepare data directory for the E2E backend instance.
	dataDir := filepath.Join(e2eDir, ".e2e-data")
	errMkdir := os.MkdirAll(dataDir, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("create data dir: %w", errMkdir)
	}

	// Clean any leftover database from a previous run.
	dbFile := filepath.Join(dataDir, "data.sqlite")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		errRemove := os.Remove(dbFile + suffix)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Warn().Err(errRemove).Str("path", dbFile+suffix).Msg("[E2E Setup] Warning: could not remove stale database file")
		}
	}

	// Build frontend SPA (embedded in the Xylona binary).
	log.Info().Msg("[E2E Setup] Building frontend SPA...")
	errFrontend := buildFrontend(projectRoot)
	if errFrontend != nil {
		return fmt.Errorf("build frontend: %w", errFrontend)
	}

	// Build the Xylona backend binary.
	xylonaExe := filepath.Join(dataDir, binaryName("xylona"))
	log.Info().Msg("[E2E Setup] Building Xylona binary...")
	errXylona := buildXylona(projectRoot, xylonaExe)
	if errXylona != nil {
		return fmt.Errorf("build xylona: %w", errXylona)
	}

	// Seed a fresh database.
	log.Info().Msg("[E2E Setup] Seeding database...")
	migrationsDir := filepath.Join(projectRoot, "sql", "migrations")
	errSeed := runSeed(dbFile, adminUsername, adminPassword, migrationsDir)
	if errSeed != nil {
		return fmt.Errorf("seed database: %w", errSeed)
	}

	// Start the Xylona backend.
	backendCmd, errStart := startNode("E2E-Backend", dataDir, e2eDir, xylonaExe, httpPort, "DUMMY_GAME_ID=e2e-test-game", "XYLONA_VERSION_CHECK_INTERVAL=30s")
	if errStart != nil {
		return fmt.Errorf("start backend: %w", errStart)
	}

	// Write PID file for teardown.
	if backendCmd.Process != nil {
		pidFile := filepath.Join(dataDir, "xylona.pid")
		errPID := os.WriteFile(pidFile, []byte(strconv.Itoa(backendCmd.Process.Pid)), 0o600)
		if errPID != nil {
			log.Warn().Err(errPID).Str("path", pidFile).Msg("[E2E Setup] Warning: could not write backend PID file")
		}
	}

	// Everything after process start is wrapped so we kill the process on failure.
	errSetup := runSingleSetupAfterStart(ctx, backendURL, adminUsername, adminPassword, e2eDir, projectRoot, dataDir)
	if errSetup != nil {
		log.Error().Err(errSetup).Msg("[E2E Setup] Setup failed, killing backend process")
		killProcess(backendCmd)
		return errSetup
	}

	setupOK = true
	return nil
}

type singleSetupRun struct {
	ctx               context.Context
	backendURL        string
	adminUsername     string
	adminPassword     string
	e2eDir            string
	projectRoot       string
	dataDir           string
	dummyServerExe    string
	client            *e2eClient
	createdUsers      []testUser
	gameServers       []*xylona.GameServer
	testGameID        string
	testServerID      string
	noTrackerServerID string
}

func runSingleSetupAfterStart(ctx context.Context, backendURL, adminUsername, adminPassword, e2eDir, projectRoot, dataDir string) error {
	run := &singleSetupRun{
		ctx:           ctx,
		backendURL:    backendURL,
		adminUsername: adminUsername,
		adminPassword: adminPassword,
		e2eDir:        e2eDir,
		projectRoot:   projectRoot,
		dataDir:       dataDir,
	}

	errReady := run.waitForBackend()
	if errReady != nil {
		return errReady
	}
	errDummy := run.buildDummyServer()
	if errDummy != nil {
		return errDummy
	}
	errLogin := run.loginAdmin()
	if errLogin != nil {
		return errLogin
	}
	errUsers := run.createTestUsers()
	if errUsers != nil {
		return errUsers
	}
	errPrimary := run.ensurePrimaryServer()
	if errPrimary != nil {
		return errPrimary
	}
	run.ensureModState()
	run.createTestFiles()
	run.noTrackerServerID = run.ensureNoTrackerServer()

	errState := run.saveState()
	if errState != nil {
		return errState
	}
	run.assignRBAC()

	log.Info().Msg("[E2E Setup] Setup complete")
	return nil
}

func (run *singleSetupRun) waitForBackend() error {
	log.Info().Msg("[E2E Setup] Waiting for backend to be ready...")
	errWait := waitForReady(run.ctx, run.backendURL, 30*time.Second)
	if errWait != nil {
		return fmt.Errorf("wait for backend: %w", errWait)
	}
	log.Info().Msg("[E2E Setup] Backend is ready")
	return nil
}

func (run *singleSetupRun) buildDummyServer() error {
	e2eBinDir := filepath.Join(run.dataDir, "bin")
	run.dummyServerExe = filepath.Join(e2eBinDir, binaryName("dummy-game-server"))
	errBuild := buildDummyGameServer(run.projectRoot, run.dummyServerExe)
	if errBuild != nil {
		return fmt.Errorf("build dummy game server: %w", errBuild)
	}
	return nil
}

func (run *singleSetupRun) loginAdmin() error {
	log.Info().Msgf("[E2E Setup] Logging in as admin: %s", run.adminUsername)
	client, errLogin := newAuthenticatedClient(run.ctx, run.backendURL, run.adminUsername, run.adminPassword)
	if errLogin != nil {
		return fmt.Errorf("login as admin: %w", errLogin)
	}
	run.client = client
	return nil
}

func (run *singleSetupRun) createTestUsers() error {
	for _, ud := range testUserDefs {
		resp, errCreate := run.client.rpc.CreateUser(run.ctx, connect.NewRequest(&xylona.CreateUserRequest{
			UserName:  ud.userName,
			Email:     ud.email,
			Password:  ud.password,
			FirstName: ud.firstName,
			LastName:  ud.lastName,
			SuperUser: ud.superUser,
		}))
		if errCreate != nil {
			log.Warn().Err(errCreate).Msgf("[E2E Setup] Warning: could not create user %s", ud.userName)
			continue
		}
		userID := ""
		if resp.Msg.GetUser() != nil {
			userID = resp.Msg.GetUser().GetId()
		}
		run.createdUsers = append(run.createdUsers, testUser{
			ID:        userID,
			Username:  ud.userName,
			Password:  ud.password,
			SuperUser: ud.superUser,
		})
		log.Info().Msgf("[E2E Setup] Created user: %s (id: %s)", ud.userName, userID)
	}

	errSaveUsers := saveTestUsers(run.e2eDir, run.createdUsers)
	if errSaveUsers != nil {
		return fmt.Errorf("save test users: %w", errSaveUsers)
	}
	return nil
}

func (run *singleSetupRun) ensurePrimaryServer() error {
	listResp, errList := run.client.rpc.ListGameServers(run.ctx, connect.NewRequest(&xylona.ListGameServersRequest{}))
	if errList != nil {
		return fmt.Errorf("list game servers: %w", errList)
	}
	run.gameServers = listResp.Msg.GetGameServers()

	if len(run.gameServers) == 0 {
		return run.createPrimaryServer()
	}

	run.testServerID = run.gameServers[0].GetId()
	run.testGameID = run.gameServers[0].GetGameId()
	log.Info().Msgf("[E2E Setup] Using existing game server: %s", run.gameServers[0].GetName())
	return nil
}

func (run *singleSetupRun) primaryOwnerUserID() string {
	testOwnerUserID := run.client.userID
	for _, user := range run.createdUsers {
		if user.Username == "e2e-superuser" {
			testOwnerUserID = user.ID
			break
		}
	}
	return testOwnerUserID
}

func (run *singleSetupRun) createPrimaryServer() error {
	log.Info().Msg("[E2E Setup] No game servers found; creating a test game server...")

	game, errGame := run.findOrCreatePrimaryGame()
	if errGame != nil {
		return errGame
	}
	run.testGameID = game.GetId()

	gsDir := filepath.Join(run.dataDir, "bin", "test-server")
	errMkdirGS := os.MkdirAll(gsDir, 0o750)
	if errMkdirGS != nil {
		return fmt.Errorf("create game server dir: %w", errMkdirGS)
	}

	ipAddress, errIP := run.firstBindableIP()
	if errIP != nil {
		return errIP
	}

	gsDirPath := strings.ReplaceAll(gsDir, "\\", "/")
	createResp, errCreate := run.client.rpc.CreateGameServer(run.ctx, connect.NewRequest(&xylona.CreateGameServerRequest{
		GameServer: &xylona.GameServer{
			Name:          "E2E Test Server",
			GameId:        game.GetId(),
			UserId:        run.primaryOwnerUserID(),
			Directory:     gsDirPath,
			Ip:            &xylona.IP{Address: ipAddress},
			Port:          25599,
			QueryPort:     25600,
			SetMaxPlayers: 20,
		},
	}))
	if errCreate != nil {
		return fmt.Errorf("create game server: %w", errCreate)
	}
	run.testServerID = createResp.Msg.GetGameServer().GetId()
	log.Info().Msgf("[E2E Setup] Created game server: %s", run.testServerID)

	run.selectPaperVariant(run.testServerID)
	run.startPrimaryGameServer()
	run.refreshGameServers()
	return nil
}

func (run *singleSetupRun) findOrCreatePrimaryGame() (*xylona.Game, error) {
	gamesResp, errGames := run.client.rpc.ListGames(run.ctx, connect.NewRequest(&xylona.ListGamesRequest{}))
	if errGames != nil {
		return nil, fmt.Errorf("list games: %w", errGames)
	}

	var game *xylona.Game
	for _, g := range gamesResp.Msg.GetGames() {
		if g.GetName() == "E2E Test Game" {
			game = g
			break
		}
	}
	if game != nil {
		return game, nil
	}

	dummyExePath := strings.ReplaceAll(run.dummyServerExe, "\\", "/")
	log.Info().Msg("[E2E Setup] Creating test game definition...")
	addResp, errAdd := run.client.rpc.AddGame(run.ctx, connect.NewRequest(&xylona.AddGameRequest{
		Game: &xylona.Game{
			Id:                             "e2e-test-game",
			Name:                           "E2E Test Game",
			LinuxSupport:                   true,
			LinuxStartArgsTemplate:         e2eDummyStartTemplateJSON,
			LinuxBaseCommand:               dummyExePath,
			LinuxStopCommand:               "stop",
			LinuxInstallCommand:            "echo installed",
			LinuxInstallCommandProcessor:   xylona.CommandProcessor_BASH,
			LinuxUpdateCommand:             "echo updated",
			LinuxUpdateCommandProcessor:    xylona.CommandProcessor_BASH,
			WindowsSupport:                 true,
			WindowsStartArgsTemplate:       e2eDummyStartTemplateJSON,
			WindowsBaseCommand:             dummyExePath,
			WindowsStopCommand:             "stop",
			WindowsInstallCommand:          `cmd /c "echo installed"`,
			WindowsUpdateCommand:           `cmd /c "echo updated"`,
			WindowsInstallCommandProcessor: xylona.CommandProcessor_CMD,
			DefaultPort:                    25599,
			DefaultQueryPort:               25599,
			UpdateProvider: &xylona.UpdateProviderConfig{
				Kind: xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_COMMAND,
			},
			Variants: minecraftE2EVariants(),
		},
	}))
	if errAdd != nil {
		return nil, fmt.Errorf("add game: %w", errAdd)
	}

	game = addResp.Msg.GetGame()
	log.Info().Msgf("[E2E Setup] Created test game: %s", game.GetId())
	return game, nil
}

func (run *singleSetupRun) firstBindableIP() (string, error) {
	ipsResp, errIPs := run.client.rpc.ListIPs(run.ctx, connect.NewRequest(&xylona.ListIPsRequest{}))
	if errIPs != nil {
		return "", fmt.Errorf("list IPs: %w", errIPs)
	}
	ipAddress := "0.0.0.0"
	if len(ipsResp.Msg.GetIps()) > 0 {
		ipAddress = ipsResp.Msg.GetIps()[0].GetAddress()
	}
	return ipAddress, nil
}

func (run *singleSetupRun) selectPaperVariant(gameServerID string) {
	_, errSetVariant := run.client.rpc.SetServerVariant(run.ctx, connect.NewRequest(&xylona.SetServerVariantRequest{
		GameServerId: gameServerID,
		VariantId:    "paper",
	}))
	if errSetVariant != nil {
		log.Warn().Err(errSetVariant).Msg("[E2E Setup] Warning: could not set server variant to paper")
		return
	}
	log.Info().Msg("[E2E Setup] Set server variant to Paper")
}

func (run *singleSetupRun) startPrimaryGameServer() {
	_, errStartGS := run.client.rpc.StartGameServer(run.ctx, connect.NewRequest(&xylona.StartGameServerRequest{
		ServerId: run.testServerID,
	}))
	if errStartGS != nil {
		log.Warn().Err(errStartGS).Msg("[E2E Setup] Warning: could not start game server")
		return
	}
	log.Info().Msg("[E2E Setup] Started test game server")
	time.Sleep(2 * time.Second)
}

func (run *singleSetupRun) refreshGameServers() {
	listResp, errList := run.client.rpc.ListGameServers(run.ctx, connect.NewRequest(&xylona.ListGameServersRequest{}))
	if errList != nil {
		log.Warn().Err(errList).Msg("[E2E Setup] Warning: could not re-list game servers")
		return
	}
	run.gameServers = listResp.Msg.GetGameServers()
}

func (run *singleSetupRun) ensureModState() {
	run.ensureGameVariants()
	run.ensureServerVariant()
}

func (run *singleSetupRun) ensureGameVariants() {
	if run.testGameID == "" {
		return
	}

	gameResp, errGetGame := run.client.rpc.GetGame(run.ctx, connect.NewRequest(&xylona.GetGameRequest{
		Id: run.testGameID,
	}))
	if errGetGame != nil {
		log.Warn().Err(errGetGame).Msg("[E2E Setup] Warning: could not fetch game")
		return
	}
	if gameResp.Msg.GetGame() == nil || len(gameResp.Msg.GetGame().GetVariants()) != 0 {
		return
	}

	log.Info().Msg("[E2E Setup] Updating game with typed variant config...")
	gameToUpdate := gameResp.Msg.GetGame()
	gameToUpdate.UpdateProvider = &xylona.UpdateProviderConfig{
		Kind: xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_COMMAND,
	}
	gameToUpdate.Variants = minecraftE2EVariants()
	_, errEdit := run.client.rpc.EditGame(run.ctx, connect.NewRequest(&xylona.EditGameRequest{
		Game: gameToUpdate,
	}))
	if errEdit != nil {
		log.Warn().Err(errEdit).Msg("[E2E Setup] Warning: could not update game with typed variant config")
		return
	}
	log.Info().Msg("[E2E Setup] Updated game with Minecraft variant config")
}

func (run *singleSetupRun) ensureServerVariant() {
	if run.testServerID == "" {
		return
	}

	gsResp, errGetGS := run.client.rpc.GetGameServer(run.ctx, connect.NewRequest(&xylona.GetGameServerRequest{
		Id: run.testServerID,
	}))
	if errGetGS != nil {
		log.Warn().Err(errGetGS).Msg("[E2E Setup] Warning: could not fetch game server")
		return
	}
	if gsResp.Msg.GetGameServer() == nil || gsResp.Msg.GetGameServer().GetSelectedVariantId() != "" {
		return
	}

	log.Info().Msg("[E2E Setup] Setting server variant to Paper...")
	run.selectPaperVariant(run.testServerID)
}

func (run *singleSetupRun) createTestFiles() {
	if run.testServerID == "" {
		return
	}

	testFiles := []struct {
		path    string
		isDir   bool
		content string
	}{
		{path: "e2e-test-config.cfg", isDir: false, content: "key=value\nport=25565\n"},
		{path: "e2e-test-readme.txt", isDir: false, content: "This is a test file for E2E testing.\n"},
		{path: "e2e-test-subdir", isDir: true, content: ""},
		{path: "e2e-test-subdir/nested-file.txt", isDir: false, content: "Nested test file content.\n"},
	}
	for _, tf := range testFiles {
		_, errFile := run.client.rpc.GameServersFileOrDirectoryCreate(run.ctx, connect.NewRequest(&xylona.GameServerFileOrDirectoryCreateRequest{
			GameServerId: run.testServerID,
			FullFilePath: tf.path,
			IsDirectory:  tf.isDir,
			Content:      tf.content,
		}))
		if errFile != nil {
			log.Warn().Err(errFile).Msgf("[E2E Setup] Warning: could not create test file %s", tf.path)
		}
	}
	log.Info().Msg("[E2E Setup] Created test files for file browser tests")
}

func (run *singleSetupRun) ensureNoTrackerServer() string {
	dummyExePath := strings.ReplaceAll(run.dummyServerExe, "\\", "/")
	gamesResp, errGames := run.client.rpc.ListGames(run.ctx, connect.NewRequest(&xylona.ListGamesRequest{}))
	if errGames != nil {
		return ""
	}

	noTrackerGame := findGameByName(gamesResp.Msg.GetGames(), "E2E No-Tracker Game")
	if noTrackerGame == nil {
		noTrackerGame = run.createNoTrackerGame(dummyExePath)
	}
	if noTrackerGame == nil {
		return ""
	}

	ntGsDir := filepath.Join(run.dataDir, "bin", "no-tracker-server")
	errMkdirNT := os.MkdirAll(ntGsDir, 0o750)
	if errMkdirNT != nil {
		log.Warn().Err(errMkdirNT).Msg("[E2E Setup] Warning: could not create no-tracker server dir")
		return ""
	}

	noTrackerServerID := run.findNoTrackerServerID()
	if noTrackerServerID != "" {
		return noTrackerServerID
	}
	return run.createNoTrackerServer(noTrackerGame, ntGsDir)
}

func findGameByName(games []*xylona.Game, name string) *xylona.Game {
	for _, game := range games {
		if game.GetName() == name {
			return game
		}
	}
	return nil
}

func (run *singleSetupRun) createNoTrackerGame(dummyExePath string) *xylona.Game {
	addResp, errAdd := run.client.rpc.AddGame(run.ctx, connect.NewRequest(&xylona.AddGameRequest{
		Game: &xylona.Game{
			Name:                           "E2E No-Tracker Game",
			LinuxSupport:                   true,
			LinuxStartArgsTemplate:         e2eDummyStartTemplateJSON,
			LinuxBaseCommand:               dummyExePath,
			LinuxInstallCommand:            "echo installed",
			LinuxInstallCommandProcessor:   xylona.CommandProcessor_BASH,
			WindowsSupport:                 true,
			WindowsStartArgsTemplate:       e2eDummyStartTemplateJSON,
			WindowsBaseCommand:             dummyExePath,
			WindowsInstallCommand:          `cmd /c "echo installed"`,
			WindowsInstallCommandProcessor: xylona.CommandProcessor_CMD,
			DefaultPort:                    25601,
			DefaultQueryPort:               25602,
		},
	}))
	if errAdd != nil {
		log.Warn().Err(errAdd).Msg("[E2E Setup] Warning: could not create no-tracker game")
		return nil
	}

	noTrackerGame := addResp.Msg.GetGame()
	log.Info().Msgf("[E2E Setup] Created no-tracker game: %s", noTrackerGame.GetId())
	return noTrackerGame
}

func (run *singleSetupRun) findNoTrackerServerID() string {
	listResp, errList := run.client.rpc.ListGameServers(run.ctx, connect.NewRequest(&xylona.ListGameServersRequest{}))
	if errList != nil {
		return ""
	}
	for _, gameServer := range listResp.Msg.GetGameServers() {
		if gameServer.GetName() == "E2E No-Tracker Server" {
			return gameServer.GetId()
		}
	}
	return ""
}

func (run *singleSetupRun) createNoTrackerServer(noTrackerGame *xylona.Game, ntGsDir string) string {
	ipsResp, errIPs := run.client.rpc.ListIPs(run.ctx, connect.NewRequest(&xylona.ListIPsRequest{}))
	ipAddress := "0.0.0.0"
	if errIPs == nil && len(ipsResp.Msg.GetIps()) > 0 {
		ipAddress = ipsResp.Msg.GetIps()[0].GetAddress()
	}
	ntGsDirPath := strings.ReplaceAll(ntGsDir, "\\", "/")
	createResp, errCreate := run.client.rpc.CreateGameServer(run.ctx, connect.NewRequest(&xylona.CreateGameServerRequest{
		GameServer: &xylona.GameServer{
			Name:          "E2E No-Tracker Server",
			GameId:        noTrackerGame.GetId(),
			UserId:        run.client.userID,
			Directory:     ntGsDirPath,
			Ip:            &xylona.IP{Address: ipAddress},
			Port:          25601,
			QueryPort:     25602,
			SetMaxPlayers: 20,
		},
	}))
	if errCreate != nil {
		log.Warn().Err(errCreate).Msg("[E2E Setup] Warning: could not create no-tracker server")
		return ""
	}

	noTrackerServerID := createResp.Msg.GetGameServer().GetId()
	log.Info().Msgf("[E2E Setup] Created no-tracker server: %s", noTrackerServerID)
	return noTrackerServerID
}

func (run *singleSetupRun) saveState() error {
	errSaveState := saveTestState(run.e2eDir, &testState{
		GameServerID:      run.testServerID,
		GameID:            run.testGameID,
		GameName:          "E2E Test Game",
		NoTrackerServerID: run.noTrackerServerID,
	})
	if errSaveState != nil {
		return fmt.Errorf("save test state: %w", errSaveState)
	}
	return nil
}

func (run *singleSetupRun) assignRBAC() {
	if len(run.gameServers) == 0 {
		log.Info().Msg("[E2E Setup] No game servers found; skipping RBAC role assignments")
		return
	}

	firstServer := run.gameServers[0]
	log.Info().Msgf("[E2E Setup] Assigning RBAC roles on game server: %s", firstServer.GetName())

	for _, user := range run.createdUsers {
		if user.SuperUser {
			continue
		}
		var roleID string
		switch user.Username {
		case "e2e-operator":
			roleID = "operator"
		case "e2e-viewer":
			roleID = "viewer"
		default:
			continue
		}

		_, errGrant := run.client.rpc.GrantGameServerAccess(run.ctx, connect.NewRequest(&xylona.GrantGameServerAccessRequest{
			GameServerId: firstServer.GetId(),
			UserId:       user.ID,
			RoleId:       roleID,
		}))
		if errGrant != nil {
			log.Warn().Err(errGrant).Msgf("[E2E Setup] Warning: could not grant access to %s", user.Username)
			continue
		}
		log.Info().Msgf("[E2E Setup] Granted %s on %s to %s", roleID, firstServer.GetName(), user.Username)
	}
}
