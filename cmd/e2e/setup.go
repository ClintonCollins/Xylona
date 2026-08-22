package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

type e2eMode string

const (
	e2eModeLocalController e2eMode = "local-controller"
	e2eModeRemoteNode      e2eMode = "remote-node"
)

type setupConfig struct {
	mode          e2eMode
	httpPort      int
	nodePort      int
	adminUsername string
	adminPassword string
	e2eDir        string
	projectRoot   string
}

type setupPaths struct {
	dataDir           string
	controllerDir     string
	controllerHomeDir string
	nodeDir           string
	nodeHomeDir       string
	binDir            string
}

type setupRun struct {
	ctx             context.Context
	cfg             setupConfig
	paths           setupPaths
	backendURL      string
	runtimeEnv      map[string]string
	xylonaExe       string
	xylonaNodeExe   string
	dummyServerExe  string
	client          *e2eClient
	createdUsers    []testUser
	selfNodeID      string
	targetNodeID    string
	remoteNodeID    string
	gameID          string
	gameServerID    string
	gameServerDir   string
	controllerCmd   *exec.Cmd
	remoteNodeCmd   *exec.Cmd
	controllerReady bool
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

const e2eDummyStartTemplateJSON = `[{"id":"heartbeat","order":0,"ownership":"system","tokens":["-heartbeat","1s"],"label":"Heartbeat"}]`

func parseE2EMode(value string) (e2eMode, error) {
	mode := e2eMode(strings.TrimSpace(value))
	switch mode {
	case e2eModeLocalController, e2eModeRemoteNode:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown E2E mode %q", value)
	}
}

func modeDataDir(e2eDir string, mode e2eMode) string {
	return filepath.Join(e2eDir, ".e2e-data", string(mode))
}

func resolveSetupPaths(e2eDir string, mode e2eMode) setupPaths {
	dataDir := modeDataDir(e2eDir, mode)
	return setupPaths{
		dataDir:           dataDir,
		controllerDir:     filepath.Join(dataDir, "controller"),
		controllerHomeDir: filepath.Join(dataDir, "controller-home"),
		nodeDir:           filepath.Join(dataDir, "node"),
		nodeHomeDir:       filepath.Join(dataDir, "node-home"),
		binDir:            filepath.Join(dataDir, "bin"),
	}
}

func runSetup(ctx context.Context, cfg setupConfig) (*testState, error) {
	run := &setupRun{
		ctx:        ctx,
		cfg:        cfg,
		paths:      resolveSetupPaths(cfg.e2eDir, cfg.mode),
		backendURL: fmt.Sprintf("http://localhost:%d", cfg.httpPort),
	}

	errLock := acquireLock(cfg.e2eDir, string(cfg.mode), map[string]int{
		"http": cfg.httpPort,
		"node": cfg.nodePort,
	})
	if errLock != nil {
		return nil, fmt.Errorf("acquire lock: %w", errLock)
	}

	setupOK := false
	defer func() {
		if setupOK {
			return
		}
		run.cleanupProcesses()
		releaseLock(cfg.e2eDir, string(cfg.mode))
	}()

	errPrepare := run.prepareFilesystem()
	if errPrepare != nil {
		return nil, errPrepare
	}

	errBuild := run.buildBinaries()
	if errBuild != nil {
		return nil, errBuild
	}

	errSeed := run.seedDatabase()
	if errSeed != nil {
		return nil, errSeed
	}

	errStartController := run.startController()
	if errStartController != nil {
		return nil, errStartController
	}

	errReady := run.waitForController()
	if errReady != nil {
		return nil, errReady
	}

	errLogin := run.loginAdmin()
	if errLogin != nil {
		return nil, errLogin
	}

	errResolveSelfNode := run.resolveSelfNode()
	if errResolveSelfNode != nil {
		return nil, errResolveSelfNode
	}

	if cfg.mode == e2eModeRemoteNode {
		errRemote := run.startAndPairRemoteNode()
		if errRemote != nil {
			return nil, errRemote
		}
		run.targetNodeID = run.remoteNodeID
	} else {
		run.targetNodeID = run.selfNodeID
	}

	errIP := run.ensureLoopbackIP(run.targetNodeID)
	if errIP != nil {
		return nil, errIP
	}

	errUsers := run.createTestUsers()
	if errUsers != nil {
		return nil, errUsers
	}

	errFixture := run.createPrimaryFixture()
	if errFixture != nil {
		return nil, errFixture
	}

	errState := run.saveState()
	if errState != nil {
		return nil, errState
	}

	state := run.state()
	errUpdateLock := updateLockRuntimePIDs(cfg.e2eDir, string(cfg.mode), state)
	if errUpdateLock != nil {
		return nil, errUpdateLock
	}

	run.assignRBAC()
	setupOK = true
	log.Info().Str("mode", string(cfg.mode)).Msg("[E2E Setup] Setup complete")
	return state, nil
}

func (run *setupRun) cleanupProcesses() {
	killProcess(run.remoteNodeCmd)
	killProcess(run.controllerCmd)
}

func (run *setupRun) prepareFilesystem() error {
	errRemove := os.RemoveAll(run.paths.dataDir)
	if errRemove != nil {
		return fmt.Errorf("remove stale E2E data dir: %w", errRemove)
	}

	for _, dir := range []string{
		run.paths.controllerDir,
		run.paths.controllerHomeDir,
		run.paths.nodeDir,
		run.paths.nodeHomeDir,
		run.paths.binDir,
	} {
		errMkdir := os.MkdirAll(dir, 0o750)
		if errMkdir != nil {
			return fmt.Errorf("create E2E directory %s: %w", dir, errMkdir)
		}
	}
	return nil
}

func (run *setupRun) buildBinaries() error {
	errFrontendPlaceholder := prepareFrontendPlaceholder(run.cfg.projectRoot)
	if errFrontendPlaceholder != nil {
		return fmt.Errorf("prepare frontend placeholder: %w", errFrontendPlaceholder)
	}

	run.xylonaExe = filepath.Join(run.paths.binDir, binaryName("xylona"))
	errXylona := buildXylona(run.cfg.projectRoot, run.xylonaExe)
	if errXylona != nil {
		return fmt.Errorf("build xylona: %w", errXylona)
	}

	run.dummyServerExe = filepath.Join(run.paths.binDir, binaryName("dummy-game-server"))
	errDummy := buildDummyGameServer(run.cfg.projectRoot, run.dummyServerExe)
	if errDummy != nil {
		return fmt.Errorf("build dummy game server: %w", errDummy)
	}

	if run.cfg.mode != e2eModeRemoteNode {
		return nil
	}

	run.xylonaNodeExe = filepath.Join(run.paths.binDir, binaryName("xylona-node"))
	errNode := buildXylonaNode(run.cfg.projectRoot, run.xylonaNodeExe)
	if errNode != nil {
		return fmt.Errorf("build xylona-node: %w", errNode)
	}
	return nil
}

func (run *setupRun) seedDatabase() error {
	dbFile := filepath.Join(run.paths.controllerDir, "data.sqlite")
	migrationsDir := filepath.Join(run.cfg.projectRoot, "sql", "migrations")
	errSeed := runSeed(dbFile, run.cfg.adminUsername, run.cfg.adminPassword, migrationsDir)
	if errSeed != nil {
		return fmt.Errorf("seed database: %w", errSeed)
	}
	return nil
}

func (run *setupRun) startController() error {
	runtimeEnv, errRuntimeEnv := generateControllerRuntimeEnv()
	if errRuntimeEnv != nil {
		return errRuntimeEnv
	}
	run.runtimeEnv = runtimeEnv

	extraEnv := []string{
		"DUMMY_GAME_ID=e2e-test-game",
		"XYLONA_VERSION_CHECK_INTERVAL=30s",
		"HOME=" + run.paths.controllerHomeDir,
		"USERPROFILE=" + run.paths.controllerHomeDir,
	}
	controllerCmd, errStart := startNode("Controller", run.paths.controllerDir, run.paths.dataDir, run.xylonaExe, run.cfg.httpPort, runtimeEnv, extraEnv...)
	if errStart != nil {
		return fmt.Errorf("start controller: %w", errStart)
	}
	run.controllerCmd = controllerCmd
	errPID := writePIDFile(filepath.Join(run.paths.controllerDir, "xylona.pid"), controllerCmd)
	if errPID != nil {
		log.Warn().Err(errPID).Msg("[E2E Setup] Could not write controller PID")
	}
	return nil
}

func (run *setupRun) waitForController() error {
	log.Info().Str("url", run.backendURL).Msg("[E2E Setup] Waiting for controller")
	errWait := waitForReady(run.ctx, run.backendURL+"/api/ready", 30*time.Second)
	if errWait != nil {
		return fmt.Errorf("wait for controller: %w", errWait)
	}
	run.controllerReady = true
	return nil
}

func (run *setupRun) loginAdmin() error {
	client, errLogin := newAuthenticatedClient(run.ctx, run.backendURL, run.cfg.adminUsername, run.cfg.adminPassword)
	if errLogin != nil {
		return errLogin
	}
	run.client = client
	return nil
}

func (run *setupRun) resolveSelfNode() error {
	nodes, errNodes := run.listNodes()
	if errNodes != nil {
		return errNodes
	}
	if len(nodes) == 0 {
		return errors.New("controller returned no nodes")
	}
	run.selfNodeID = nodes[0].GetId()
	if run.selfNodeID == "" {
		return errors.New("controller self node ID is empty")
	}
	return nil
}

func (run *setupRun) listNodes() ([]*xylona.Node, error) {
	resp, errList := run.client.rpc.ListNodes(run.ctx, connect.NewRequest(&xylona.ListNodesRequest{}))
	if errList != nil {
		return nil, fmt.Errorf("list nodes: %w", errList)
	}
	return resp.Msg.GetNodes(), nil
}

func (run *setupRun) startAndPairRemoteNode() error {
	tokenResp, errToken := run.client.rpc.GenerateNodePairingObject(run.ctx, connect.NewRequest(&xylona.GenerateNodePairingObjectRequest{
		TargetUrl: run.backendURL,
	}))
	if errToken != nil {
		return fmt.Errorf("generate node pairing object: %w", errToken)
	}
	joinToken := strings.TrimSpace(tokenResp.Msg.GetPairingToken())
	if joinToken == "" {
		return errors.New("controller returned empty node pairing token")
	}

	nodeCmd, errStart := startXylonaNode(
		run.xylonaNodeExe,
		run.backendURL,
		joinToken,
		run.cfg.nodePort,
		run.paths.nodeDir,
		run.paths.nodeHomeDir,
		"e2e-remote",
	)
	if errStart != nil {
		return errStart
	}
	run.remoteNodeCmd = nodeCmd

	errPID := writePIDFile(filepath.Join(run.paths.nodeDir, "xylona-node.pid"), nodeCmd)
	if errPID != nil {
		log.Warn().Err(errPID).Msg("[E2E Setup] Could not write remote node PID")
	}

	remoteNodeID, errWait := run.waitForRemoteNodeRegistered(60 * time.Second)
	if errWait != nil {
		return errWait
	}
	run.remoteNodeID = remoteNodeID
	return nil
}

func (run *setupRun) waitForRemoteNodeRegistered(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-run.ctx.Done():
			return "", fmt.Errorf("wait for remote node canceled: %w", run.ctx.Err())
		default:
		}

		nodes, errNodes := run.listNodes()
		if errNodes == nil {
			for _, node := range nodes {
				if node.GetId() != "" && node.GetId() != run.selfNodeID {
					return node.GetId(), nil
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
	return "", fmt.Errorf("remote node did not register within %s", timeout)
}

func startXylonaNode(nodeExe, controllerURL, joinToken string, listenPort int, dataDir, homeDir, nodeName string) (*exec.Cmd, error) {
	listenAddr := fmt.Sprintf(":%d", listenPort)
	advertiseURL := fmt.Sprintf("https://127.0.0.1:%d", listenPort)

	args := []string{
		"--controller-url=" + controllerURL,
		"--join-token=" + joinToken,
		"--listen=" + listenAddr,
		"--advertise-url=" + advertiseURL,
		"--node-name=" + nodeName,
		"--data-dir=" + dataDir,
	}

	cmd := exec.Command(nodeExe, args...) //nolint:noctx // remote node intentionally outlives setup until teardown.
	cmd.Dir = dataDir
	cmd.Env = append(os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
	)

	stdout, errStdout := cmd.StdoutPipe()
	if errStdout != nil {
		return nil, fmt.Errorf("remote node stdout pipe: %w", errStdout)
	}
	stderr, errStderr := cmd.StderrPipe()
	if errStderr != nil {
		return nil, fmt.Errorf("remote node stderr pipe: %w", errStderr)
	}

	errStart := cmd.Start()
	if errStart != nil {
		return nil, fmt.Errorf("start xylona-node: %w", errStart)
	}

	go pipeWithPrefix("[RemoteNode]", stdout, os.Stdout)
	go pipeWithPrefix("[RemoteNode]", stderr, os.Stderr)
	return cmd, nil
}

func pipeWithPrefix(prefix string, src interface {
	Read([]byte) (int, error)
}, dst *os.File) {
	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		fmt.Fprintf(dst, "%s %s\n", prefix, scanner.Text())
	}
}

func writePIDFile(pidFile string, cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	errWrite := os.WriteFile(pidFile, []byte(pid), 0o600)
	if errWrite != nil {
		return fmt.Errorf("write PID file: %w", errWrite)
	}
	return nil
}

func (run *setupRun) ensureLoopbackIP(nodeID string) error {
	_, errAdd := run.client.rpc.AddIP(run.ctx, connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address: "127.0.0.1",
			Usable:  true,
			NodeId:  nodeID,
		},
	}))
	if errAdd == nil {
		return nil
	}

	if connect.CodeOf(errAdd) == connect.CodeAlreadyExists {
		return nil
	}
	return fmt.Errorf("add loopback IP for node %s: %w", nodeID, errAdd)
}

func (run *setupRun) createTestUsers() error {
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
			return fmt.Errorf("create test user %s: %w", ud.userName, errCreate)
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
	}

	errSaveUsers := saveTestUsers(run.cfg.e2eDir, run.createdUsers)
	if errSaveUsers != nil {
		return fmt.Errorf("save test users: %w", errSaveUsers)
	}
	return nil
}

func (run *setupRun) createPrimaryFixture() error {
	game, errGame := run.createPrimaryGame()
	if errGame != nil {
		return errGame
	}
	run.gameID = game.GetId()

	ownerID := run.primaryOwnerUserID()
	requestedDirectory := filepath.Join(run.installHomeForTarget(), "requested-placeholder")
	createResp, errCreate := run.client.rpc.CreateGameServer(run.ctx, connect.NewRequest(&xylona.CreateGameServerRequest{
		GameServer: &xylona.GameServer{
			Name:          "E2E Test Server",
			GameId:        game.GetId(),
			UserId:        ownerID,
			Directory:     normalizePathForProto(requestedDirectory),
			Ip:            &xylona.IP{Address: "127.0.0.1", NodeId: run.targetNodeID},
			Port:          25599,
			QueryPort:     25600,
			MaxPlayers:    20,
			SetMaxPlayers: 20,
			MaxMemoryMb:   1024,
			NodeId:        run.targetNodeID,
		},
	}))
	if errCreate != nil {
		return fmt.Errorf("create primary game server: %w", errCreate)
	}
	if createResp.Msg.GetGameServer() == nil {
		return errors.New("create primary game server returned no game server")
	}

	run.gameServerID = createResp.Msg.GetGameServer().GetId()
	run.gameServerDir = createResp.Msg.GetGameServer().GetDirectory()

	errFiles := run.createFixtureFiles()
	if errFiles != nil {
		return errFiles
	}

	errStart := run.startPrimaryGameServer()
	if errStart != nil {
		return errStart
	}
	return nil
}

func (run *setupRun) installHomeForTarget() string {
	if run.cfg.mode == e2eModeRemoteNode {
		return run.paths.nodeHomeDir
	}
	return run.paths.controllerHomeDir
}

func normalizePathForProto(pathValue string) string {
	return strings.ReplaceAll(pathValue, "\\", "/")
}

func (run *setupRun) primaryOwnerUserID() string {
	for _, user := range run.createdUsers {
		if user.Username == "e2e-superuser" {
			return user.ID
		}
	}
	return run.client.userID
}

func (run *setupRun) createPrimaryGame() (*xylona.Game, error) {
	dummyExePath := normalizePathForProto(run.dummyServerExe)
	resp, errAdd := run.client.rpc.AddGame(run.ctx, connect.NewRequest(&xylona.AddGameRequest{
		Game: &xylona.Game{
			Id:                             "e2e-test-game",
			Name:                           "E2E Test Game",
			LinuxSupport:                   true,
			LinuxAllowBackups:              true,
			LinuxStartArgsTemplate:         e2eDummyStartTemplateJSON,
			LinuxBaseCommand:               dummyExePath,
			LinuxStopCommand:               "stop",
			LinuxInstallType:               xylona.CommandType_COMMAND,
			LinuxInstallCommand:            "echo installed",
			LinuxInstallCommandProcessor:   xylona.CommandProcessor_BASH,
			LinuxUpdateType:                xylona.CommandType_COMMAND,
			LinuxUpdateCommand:             "echo updated",
			LinuxUpdateCommandProcessor:    xylona.CommandProcessor_BASH,
			WindowsSupport:                 true,
			WindowsAllowBackups:            true,
			WindowsStartArgsTemplate:       e2eDummyStartTemplateJSON,
			WindowsBaseCommand:             dummyExePath,
			WindowsStopCommand:             "stop",
			WindowsInstallType:             xylona.CommandType_COMMAND,
			WindowsInstallCommand:          `cmd /c "echo installed"`,
			WindowsInstallCommandProcessor: xylona.CommandProcessor_CMD,
			WindowsUpdateType:              xylona.CommandType_COMMAND,
			WindowsUpdateCommand:           `cmd /c "echo updated"`,
			WindowsUpdateCommandProcessor:  xylona.CommandProcessor_CMD,
			DefaultPort:                    25599,
			DefaultQueryPort:               25600,
			UpdateProvider: &xylona.UpdateProviderConfig{
				Kind: xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_COMMAND,
			},
		},
	}))
	if errAdd != nil {
		return nil, fmt.Errorf("create primary game: %w", errAdd)
	}
	if resp.Msg.GetGame() == nil {
		return nil, errors.New("create primary game returned no game")
	}
	return resp.Msg.GetGame(), nil
}

func (run *setupRun) createFixtureFiles() error {
	files := []struct {
		path    string
		isDir   bool
		content string
	}{
		{path: "e2e-test-config.cfg", content: "key=value\nport=25565\n"},
		{path: "e2e-test-readme.txt", content: "This is a test file for E2E testing.\n"},
		{path: "e2e-test-subdir", isDir: true},
		{path: "e2e-test-subdir/nested-file.txt", content: "Nested test file content.\n"},
	}

	for _, file := range files {
		_, errFile := run.client.rpc.GameServersFileOrDirectoryCreate(run.ctx, connect.NewRequest(&xylona.GameServerFileOrDirectoryCreateRequest{
			GameServerId: run.gameServerID,
			FullFilePath: file.path,
			IsDirectory:  file.isDir,
			Content:      file.content,
		}))
		if errFile != nil {
			return fmt.Errorf("create fixture file %s: %w", file.path, errFile)
		}
	}
	return nil
}

func (run *setupRun) startPrimaryGameServer() error {
	return run.waitForGameServerOutput("started pid=", 20*time.Second)
}

func (run *setupRun) waitForGameServerOutput(needle string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, errRead := run.client.rpc.ReadGameServerOutput(run.ctx, connect.NewRequest(&xylona.ReadGameServerOutputRequest{
			ServerId: run.gameServerID,
		}))
		if errRead == nil && strings.Contains(resp.Msg.GetOutput(), needle) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("game server output did not contain %q within %s", needle, timeout)
}

func (run *setupRun) assignRBAC() {
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
			GameServerId: run.gameServerID,
			UserId:       user.ID,
			RoleId:       roleID,
		}))
		if errGrant != nil {
			log.Warn().Err(errGrant).Str("user", user.Username).Msg("[E2E Setup] Could not grant RBAC role")
		}
	}
}

func (run *setupRun) saveState() error {
	state := run.state()
	errSave := saveTestState(run.cfg.e2eDir, state)
	if errSave != nil {
		return errSave
	}
	return nil
}

func (run *setupRun) state() *testState {
	controllerPID := 0
	if run.controllerCmd != nil && run.controllerCmd.Process != nil {
		controllerPID = run.controllerCmd.Process.Pid
	}

	remoteNodePID := 0
	if run.remoteNodeCmd != nil && run.remoteNodeCmd.Process != nil {
		remoteNodePID = run.remoteNodeCmd.Process.Pid
	}

	return &testState{
		Mode:              string(run.cfg.mode),
		BackendURL:        run.backendURL,
		DataDir:           run.paths.dataDir,
		ControllerDir:     run.paths.controllerDir,
		ControllerHomeDir: run.paths.controllerHomeDir,
		ControllerPID:     controllerPID,
		NodeDir:           run.paths.nodeDir,
		NodeHomeDir:       run.paths.nodeHomeDir,
		RemoteNodePID:     remoteNodePID,
		GameServerID:      run.gameServerID,
		GameServerDir:     run.gameServerDir,
		GameID:            run.gameID,
		GameName:          "E2E Test Game",
		TargetNodeID:      run.targetNodeID,
		RemoteNodeID:      run.remoteNodeID,
		DummyServerPath:   run.dummyServerExe,
		RuntimeEnv:        run.runtimeEnv,
	}
}

func runStatus(e2eDir string, mode e2eMode) error {
	state, errState := loadTestState(e2eDir)
	if errState != nil {
		return errState
	}

	lockPath := lockFilePath(e2eDir, string(mode))
	lockData, errLock := os.ReadFile(lockPath)
	if errLock == nil {
		var lock lockInfo
		errUnmarshal := json.Unmarshal(lockData, &lock)
		if errUnmarshal == nil {
			lock.mergeStatePIDs(e2eDir, string(mode))
			printStatus(state, &lock)
			return nil
		}
	}

	printStatus(state, nil)
	return nil
}

func printStatus(state *testState, lock *lockInfo) {
	controllerPID := state.ControllerPID
	remoteNodePID := state.RemoteNodePID

	if lock != nil {
		if lock.ControllerPID != 0 {
			controllerPID = lock.ControllerPID
		}
		if lock.RemoteNodePID != 0 {
			remoteNodePID = lock.RemoteNodePID
		}

		fmt.Printf(
			"mode=%s backend=%s lock_pid=%d lock_alive=%t controller_pid=%d controller_alive=%t",
			state.Mode,
			state.BackendURL,
			lock.PID,
			processIsAlive(lock.PID),
			controllerPID,
			processIsAlive(controllerPID),
		)
	} else {
		fmt.Printf(
			"mode=%s backend=%s lock=missing controller_pid=%d controller_alive=%t",
			state.Mode,
			state.BackendURL,
			controllerPID,
			processIsAlive(controllerPID),
		)
	}

	if remoteNodePID != 0 {
		fmt.Printf(" remote_node_pid=%d remote_node_alive=%t", remoteNodePID, processIsAlive(remoteNodePID))
	}
	fmt.Println()
}
