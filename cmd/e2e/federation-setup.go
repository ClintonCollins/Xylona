package main

import (
	"context"
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

const federationDummyStartTemplateJSON = `[{"id":"heartbeat","order":0,"ownership":"system","tokens":["-heartbeat","5s"],"label":"Heartbeat"}]`

func runFederationSetup(ctx context.Context, e2eDir, projectRoot string, nodeAPort, nodeBPort, nodeAFedPort, nodeBFedPort int) error {
	log.Info().Msg("[Federation Setup] Starting federation E2E setup...")

	// Acquire lock to prevent concurrent runs.
	errLock := acquireLock(e2eDir, "federation", map[string]int{
		"node-a-http":       nodeAPort,
		"node-b-http":       nodeBPort,
		"node-a-federation": nodeAFedPort,
		"node-b-federation": nodeBFedPort,
	})
	if errLock != nil {
		return fmt.Errorf("acquire lock: %w", errLock)
	}
	setupOK := false
	defer func() {
		if !setupOK {
			releaseLock(e2eDir, "federation")
		}
	}()

	federationDir := filepath.Join(e2eDir, ".federation")
	nodeADir := filepath.Join(federationDir, "node-a")
	nodeBDir := filepath.Join(federationDir, "node-b")
	gsDir := filepath.Join(federationDir, "game-server-data")
	xylonaExe := filepath.Join(federationDir, binaryName("xylona"))
	dummyServerExe := filepath.Join(federationDir, binaryName("dummy-game-server"))

	nodeAURL := fmt.Sprintf("http://localhost:%d", nodeAPort)
	nodeBURL := fmt.Sprintf("http://localhost:%d", nodeBPort)

	// Create directories.
	for _, dir := range []string{federationDir, nodeADir, nodeBDir, gsDir} {
		errMkdir := os.MkdirAll(dir, 0o755)
		if errMkdir != nil {
			return fmt.Errorf("create directory %s: %w", dir, errMkdir)
		}
	}

	// Clean existing databases and federation dirs.
	cleanDatabases(nodeADir, nodeBDir)

	// Build binaries.
	log.Info().Msg("[Federation Setup] Building binaries...")
	errFrontend := buildFrontend(projectRoot)
	if errFrontend != nil {
		return fmt.Errorf("build frontend: %w", errFrontend)
	}
	errXylona := buildXylona(projectRoot, xylonaExe)
	if errXylona != nil {
		return fmt.Errorf("build xylona: %w", errXylona)
	}
	errDummy := buildDummyGameServer(projectRoot, dummyServerExe)
	if errDummy != nil {
		return fmt.Errorf("build dummy game server: %w", errDummy)
	}

	// Seed both databases.
	log.Info().Msg("[Federation Setup] Seeding databases...")
	migrationsDir := filepath.Join(projectRoot, "sql", "migrations")
	errSeedA := runSeed(filepath.Join(nodeADir, "data.sqlite"), "admin", "admin", migrationsDir)
	if errSeedA != nil {
		return fmt.Errorf("seed node A database: %w", errSeedA)
	}
	errSeedB := runSeed(filepath.Join(nodeBDir, "data.sqlite"), "admin", "admin", migrationsDir)
	if errSeedB != nil {
		return fmt.Errorf("seed node B database: %w", errSeedB)
	}

	// Start both nodes.
	nodeACmd, errStartA := startNode("Node-A", nodeADir, federationDir, xylonaExe, nodeAPort, nodeAFedPort)
	if errStartA != nil {
		return fmt.Errorf("start node A: %w", errStartA)
	}

	nodeBCmd, errStartB := startNode("Node-B", nodeBDir, federationDir, xylonaExe, nodeBPort, nodeBFedPort)
	if errStartB != nil {
		killProcess(nodeACmd)
		return fmt.Errorf("start node B: %w", errStartB)
	}

	// Write PID files.
	if nodeACmd.Process != nil {
		_ = os.WriteFile(filepath.Join(federationDir, "node-a.pid"), []byte(strconv.Itoa(nodeACmd.Process.Pid)), 0o644)
	}
	if nodeBCmd.Process != nil {
		_ = os.WriteFile(filepath.Join(federationDir, "node-b.pid"), []byte(strconv.Itoa(nodeBCmd.Process.Pid)), 0o644)
	}

	// Everything after process start is wrapped so we kill child processes on failure.
	errSetup := func() error {
		// Wait for both to be ready.
		log.Info().Msg("[Federation Setup] Waiting for nodes to be ready...")
		errWaitA := waitForReady(ctx, nodeAURL, 30*time.Second)
		if errWaitA != nil {
			return fmt.Errorf("wait for node A: %w", errWaitA)
		}
		errWaitB := waitForReady(ctx, nodeBURL, 30*time.Second)
		if errWaitB != nil {
			return fmt.Errorf("wait for node B: %w", errWaitB)
		}
		log.Info().Msg("[Federation Setup] Both nodes are ready")

		// Login as admin on both nodes.
		clientA, errLoginA := newAuthenticatedClient(ctx, nodeAURL, "admin", "admin")
		if errLoginA != nil {
			return fmt.Errorf("login to node A: %w", errLoginA)
		}
		clientB, errLoginB := newAuthenticatedClient(ctx, nodeBURL, "admin", "admin")
		if errLoginB != nil {
			return fmt.Errorf("login to node B: %w", errLoginB)
		}

		// List nodes on each, find local nodes.
		nodesAResp, errNodesA := clientA.rpc.ListNodes(ctx, connect.NewRequest(&xylona.ListNodesRequest{}))
		if errNodesA != nil {
			return fmt.Errorf("list nodes on A: %w", errNodesA)
		}
		nodesBResp, errNodesB := clientB.rpc.ListNodes(ctx, connect.NewRequest(&xylona.ListNodesRequest{}))
		if errNodesB != nil {
			return fmt.Errorf("list nodes on B: %w", errNodesB)
		}

		var localNodeA, localNodeB *xylona.Node
		for _, n := range nodesAResp.Msg.Nodes {
			if n.Local {
				localNodeA = n
				break
			}
		}
		for _, n := range nodesBResp.Msg.Nodes {
			if n.Local {
				localNodeB = n
				break
			}
		}

		// Edit local nodes to set base URLs and allow_insecure_tls.
		if localNodeA != nil {
			_, errEdit := clientA.rpc.EditNode(ctx, connect.NewRequest(&xylona.EditNodeRequest{
				Node: &xylona.Node{
					Id:               localNodeA.Id,
					Name:             "Node A",
					BaseUrl:          nodeAURL,
					AllowInsecureTls: true,
				},
			}))
			if errEdit != nil {
				return fmt.Errorf("edit node A: %w", errEdit)
			}
			log.Info().Msgf("[Federation Setup] Set Node A base URL to %s", nodeAURL)
		}

		if localNodeB != nil {
			_, errEdit := clientB.rpc.EditNode(ctx, connect.NewRequest(&xylona.EditNodeRequest{
				Node: &xylona.Node{
					Id:               localNodeB.Id,
					Name:             "Node B",
					BaseUrl:          nodeBURL,
					AllowInsecureTls: true,
				},
			}))
			if errEdit != nil {
				return fmt.Errorf("edit node B: %w", errEdit)
			}
			log.Info().Msgf("[Federation Setup] Set Node B base URL to %s", nodeBURL)
		}

		// Pair nodes: generate pairing object on B, pair from A.
		log.Info().Msg("[Federation Setup] Pairing nodes...")
		pairingResp, errPairing := clientB.rpc.GenerateNodePairingObject(ctx, connect.NewRequest(&xylona.GenerateNodePairingObjectRequest{}))
		if errPairing != nil {
			return fmt.Errorf("generate pairing object: %w", errPairing)
		}
		log.Info().Msgf("[Federation Setup] Got pairing object from Node B: baseUrl=%s", pairingResp.Msg.BaseUrl)

		pairResp, errPair := clientA.rpc.PairNode(ctx, connect.NewRequest(&xylona.PairNodeRequest{
			RemoteBaseUrl:          nodeBURL,
			RemoteSecretKey:        pairingResp.Msg.PairingToken,
			RemoteMtlsPort:         pairingResp.Msg.MtlsPort,
			LocalBaseUrl:           nodeAURL,
			RemoteName:             "Node B",
			LocalName:              "Node A",
			RemoteAllowInsecureTls: true,
			LocalAllowInsecureTls:  true,
		}))
		if errPair != nil {
			return fmt.Errorf("pair nodes: %w", errPair)
		}
		log.Info().Msgf("[Federation Setup] Pairing complete. Reciprocal added: %v", pairResp.Msg.ReciprocalAdded)

		// Add test game on Node B.
		log.Info().Msg("[Federation Setup] Creating test game on Node B...")
		dummyExePath := strings.ReplaceAll(dummyServerExe, "\\", "/")
		addGameResp, errAddGame := clientB.rpc.AddGame(ctx, connect.NewRequest(&xylona.AddGameRequest{
			Game: &xylona.Game{
				Name:                           "E2E Federation Test Game",
				LinuxSupport:                   true,
				LinuxStartArgsTemplate:         federationDummyStartTemplateJSON,
				LinuxBaseCommand:               dummyExePath,
				LinuxStopCommand:               "stop",
				LinuxInstallCommand:            "echo installed",
				LinuxInstallCommandProcessor:   xylona.CommandProcessor_BASH,
				WindowsSupport:                 true,
				WindowsStartArgsTemplate:       federationDummyStartTemplateJSON,
				WindowsBaseCommand:             dummyExePath,
				WindowsStopCommand:             "stop",
				WindowsInstallCommand:          `cmd /c "echo installed"`,
				WindowsInstallCommandProcessor: xylona.CommandProcessor_CMD,
				DefaultPort:                    25597,
				DefaultQueryPort:               25597,
			},
		}))
		if errAddGame != nil {
			return fmt.Errorf("add game on node B: %w", errAddGame)
		}
		gameID := addGameResp.Msg.Game.Id

		// Get available IPs on Node B.
		ipsResp, errIPs := clientB.rpc.ListIPs(ctx, connect.NewRequest(&xylona.ListIPsRequest{}))
		if errIPs != nil {
			return fmt.Errorf("list IPs on node B: %w", errIPs)
		}
		serverIP := "0.0.0.0"
		if len(ipsResp.Msg.Ips) > 0 {
			serverIP = ipsResp.Msg.Ips[0].Address
		}
		log.Info().Msgf("[Federation Setup] Using IP: %s", serverIP)

		// Create and start game server on Node B.
		log.Info().Msg("[Federation Setup] Creating game server on Node B...")
		gsDirPath := strings.ReplaceAll(gsDir, "\\", "/")
		createResp, errCreate := clientB.rpc.CreateGameServer(ctx, connect.NewRequest(&xylona.CreateGameServerRequest{
			GameServer: &xylona.GameServer{
				Name:          "E2E Federation Server",
				GameId:        gameID,
				UserId:        clientB.userID,
				Directory:     gsDirPath,
				Ip:            &xylona.IP{Address: serverIP},
				Port:          25597,
				QueryPort:     25598,
				SetMaxPlayers: 20,
			},
		}))
		if errCreate != nil {
			return fmt.Errorf("create game server on node B: %w", errCreate)
		}
		serverID := createResp.Msg.GameServer.Id

		log.Info().Msg("[Federation Setup] Starting game server on Node B...")
		_, errStart := clientB.rpc.StartGameServer(ctx, connect.NewRequest(&xylona.StartGameServerRequest{
			ServerId: serverID,
		}))
		if errStart != nil {
			return fmt.Errorf("start game server on node B: %w", errStart)
		}
		time.Sleep(2 * time.Second)

		// Create test files on Node B's game server.
		testFiles := []struct {
			path    string
			isDir   bool
			content string
		}{
			{path: "fed-test-config.cfg", isDir: false, content: "key=value\n"},
			{path: "fed-test-readme.txt", isDir: false, content: "Federation test file.\n"},
			{path: "fed-test-subdir", isDir: true, content: ""},
			{path: "fed-test-subdir/nested.txt", isDir: false, content: "Nested.\n"},
		}
		for _, tf := range testFiles {
			_, errFile := clientB.rpc.GameServersFileOrDirectoryCreate(ctx, connect.NewRequest(&xylona.GameServerFileOrDirectoryCreateRequest{
				GameServerId: serverID,
				FullFilePath: tf.path,
				IsDirectory:  tf.isDir,
				Content:      tf.content,
			}))
			if errFile != nil {
				log.Warn().Err(errFile).Msgf("[Federation Setup] Warning: could not create test file %s", tf.path)
			}
		}
		log.Info().Msg("[Federation Setup] Created test files on Node B")

		// Wait for federation sync -- remote server should appear on Node A.
		log.Info().Msg("[Federation Setup] Waiting for federation sync...")
		syncDeadline := time.Now().Add(60 * time.Second)
		synced := false
		for time.Now().Before(syncDeadline) {
			aggResp, errAgg := clientA.rpc.ListAggregatedGameServers(ctx, connect.NewRequest(&xylona.ListAggregatedGameServersRequest{}))
			if errAgg == nil {
				for _, s := range aggResp.Msg.Servers {
					if !s.IsLocal {
						synced = true
						break
					}
				}
			}
			if synced {
				break
			}
			time.Sleep(2 * time.Second)
		}
		if !synced {
			return fmt.Errorf("federation sync timed out after 60s")
		}
		log.Info().Msg("[Federation Setup] Remote server visible on Node A")

		// Create test users on Node A.
		log.Info().Msg("[Federation Setup] Creating test users on Node A...")
		var fedTestUsers []testUser
		for _, ud := range testUserDefs {
			resp, errCreateUser := clientA.rpc.CreateUser(ctx, connect.NewRequest(&xylona.CreateUserRequest{
				UserName:  ud.userName,
				Email:     ud.email,
				Password:  ud.password,
				FirstName: ud.firstName,
				LastName:  ud.lastName,
				SuperUser: ud.superUser,
			}))
			if errCreateUser != nil {
				log.Warn().Err(errCreateUser).Msgf("[Federation Setup] Warning: could not create user %s", ud.userName)
				continue
			}
			userID := ""
			if resp.Msg.User != nil {
				userID = resp.Msg.User.Id
			}
			fedTestUsers = append(fedTestUsers, testUser{
				ID:        userID,
				Username:  ud.userName,
				Password:  ud.password,
				SuperUser: ud.superUser,
			})
			log.Info().Msgf("[Federation Setup] Created user: %s (id: %s)", ud.userName, userID)
		}

		// Get the paired node ID on Node A (the remote node representing Node B).
		updatedNodesA, errUpdatedNodes := clientA.rpc.ListNodes(ctx, connect.NewRequest(&xylona.ListNodesRequest{}))
		var pairedNodeIDOnA string
		if errUpdatedNodes == nil {
			for _, n := range updatedNodesA.Msg.Nodes {
				if !n.Local {
					pairedNodeIDOnA = n.Id
					break
				}
			}
		}

		// Save federation state.
		var nodeAID, nodeBID string
		if localNodeA != nil {
			nodeAID = localNodeA.Id
		}
		if localNodeB != nil {
			nodeBID = localNodeB.Id
		}

		errSave := saveFederationState(e2eDir, &federationTestState{
			NodeAURL:        nodeAURL,
			NodeBURL:        nodeBURL,
			NodeAID:         nodeAID,
			NodeBID:         nodeBID,
			PairedNodeIDOnA: pairedNodeIDOnA,
			GameServerID:    serverID,
			GameID:          gameID,
			TestUsers:       fedTestUsers,
		})
		if errSave != nil {
			return fmt.Errorf("save federation state: %w", errSave)
		}

		log.Info().Msg("[Federation Setup] Setup complete")
		return nil
	}()

	if errSetup != nil {
		log.Error().Err(errSetup).Msg("[Federation Setup] Setup failed, killing node processes")
		killProcess(nodeACmd)
		killProcess(nodeBCmd)
		return errSetup
	}

	setupOK = true
	return nil
}

func cleanDatabases(dirs ...string) {
	for _, dir := range dirs {
		dbFile := filepath.Join(dir, "data.sqlite")
		_ = os.Remove(dbFile)
		for _, suffix := range []string{"-wal", "-shm"} {
			_ = os.Remove(dbFile + suffix)
		}
		fedDir := filepath.Join(dir, "federation")
		_ = os.RemoveAll(fedDir)
	}
}

func killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	killCmd := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/F", "/T") //nolint:noctx
	errKill := killCmd.Run()
	if errKill != nil {
		_ = cmd.Process.Kill()
	}
}
