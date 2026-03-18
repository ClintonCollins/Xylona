package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

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

func runSingleSetup(ctx context.Context, backendURL, adminUsername, adminPassword, e2eDir, projectRoot string) error {
	log.Info().Msgf("[E2E Setup] Connecting to backend at %s", backendURL)

	// Build the dummy game server binary.
	e2eBinDir := filepath.Join(e2eDir, ".e2e-bin")
	dummyServerExe := filepath.Join(e2eBinDir, "dummy-game-server.exe")
	errBuild := buildDummyGameServer(projectRoot, dummyServerExe)
	if errBuild != nil {
		return fmt.Errorf("build dummy game server: %w", errBuild)
	}

	// Login as admin.
	log.Info().Msgf("[E2E Setup] Logging in as admin: %s", adminUsername)
	client, errLogin := newAuthenticatedClient(ctx, backendURL, adminUsername, adminPassword)
	if errLogin != nil {
		return fmt.Errorf("login as admin: %w", errLogin)
	}

	// Create test users.
	var createdUsers []testUser
	for _, ud := range testUserDefs {
		resp, errCreate := client.rpc.CreateUser(ctx, connect.NewRequest(&xylona.CreateUserRequest{
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
		if resp.Msg.User != nil {
			userID = resp.Msg.User.Id
		}
		createdUsers = append(createdUsers, testUser{
			ID:        userID,
			Username:  ud.userName,
			Password:  ud.password,
			SuperUser: ud.superUser,
		})
		log.Info().Msgf("[E2E Setup] Created user: %s (id: %s)", ud.userName, userID)
	}

	errSaveUsers := saveTestUsers(e2eDir, createdUsers)
	if errSaveUsers != nil {
		return fmt.Errorf("save test users: %w", errSaveUsers)
	}

	// Ensure a game server exists for testing.
	listResp, errList := client.rpc.ListGameServers(ctx, connect.NewRequest(&xylona.ListGameServersRequest{}))
	if errList != nil {
		return fmt.Errorf("list game servers: %w", errList)
	}
	gameServers := listResp.Msg.GameServers

	var testGameID string
	var testServerID string

	if len(gameServers) == 0 {
		log.Info().Msg("[E2E Setup] No game servers found; creating a test game server...")

		// Check for an existing test game or create one.
		gamesResp, errGames := client.rpc.ListGames(ctx, connect.NewRequest(&xylona.ListGamesRequest{}))
		if errGames != nil {
			return fmt.Errorf("list games: %w", errGames)
		}

		var game *xylona.Game
		for _, g := range gamesResp.Msg.Games {
			if g.Name == "E2E Test Game" {
				game = g
				break
			}
		}

		dummyExePath := strings.ReplaceAll(dummyServerExe, "\\", "/")

		if game == nil {
			log.Info().Msg("[E2E Setup] Creating test game definition...")
			addResp, errAdd := client.rpc.AddGame(ctx, connect.NewRequest(&xylona.AddGameRequest{
				Game: &xylona.Game{
					Name:                 "E2E Test Game",
					WindowsStartCommand:  dummyExePath,
					WindowsStopCommand:   "stop",
					WindowsInstallCommand: "echo installed",
					WindowsSupport:       true,
					DefaultPort:          25599,
					DefaultQueryPort:     25599,
				},
			}))
			if errAdd != nil {
				return fmt.Errorf("add game: %w", errAdd)
			}
			game = addResp.Msg.Game
			testGameID = game.Id
			log.Info().Msgf("[E2E Setup] Created test game: %s", game.Id)
		} else {
			testGameID = game.Id
		}

		// Create a working directory for the game server.
		gsDir := filepath.Join(e2eDir, ".e2e-bin", "test-server")
		errMkdir := os.MkdirAll(gsDir, 0o755)
		if errMkdir != nil {
			return fmt.Errorf("create game server dir: %w", errMkdir)
		}

		// Get available IPs.
		ipsResp, errIPs := client.rpc.ListIPs(ctx, connect.NewRequest(&xylona.ListIPsRequest{}))
		if errIPs != nil {
			return fmt.Errorf("list IPs: %w", errIPs)
		}
		ipAddress := "0.0.0.0"
		if len(ipsResp.Msg.Ips) > 0 {
			ipAddress = ipsResp.Msg.Ips[0].Address
		}

		gsDirPath := strings.ReplaceAll(gsDir, "\\", "/")

		// Create the game server.
		createResp, errCreate := client.rpc.CreateGameServer(ctx, connect.NewRequest(&xylona.CreateGameServerRequest{
			GameServer: &xylona.GameServer{
				Name:          "E2E Test Server",
				GameId:        game.Id,
				UserId:        client.userID,
				StartCommand:  dummyExePath + " -heartbeat 5s",
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
		testServerID = createResp.Msg.GameServer.Id
		log.Info().Msgf("[E2E Setup] Created game server: %s", testServerID)

		// Start the game server.
		_, errStart := client.rpc.StartGameServer(ctx, connect.NewRequest(&xylona.StartGameServerRequest{
			ServerId: testServerID,
		}))
		if errStart != nil {
			log.Warn().Err(errStart).Msg("[E2E Setup] Warning: could not start game server")
		} else {
			log.Info().Msg("[E2E Setup] Started test game server")
			time.Sleep(2 * time.Second)
		}

		// Re-fetch game servers list.
		listResp, errList = client.rpc.ListGameServers(ctx, connect.NewRequest(&xylona.ListGameServersRequest{}))
		if errList != nil {
			log.Warn().Err(errList).Msg("[E2E Setup] Warning: could not re-list game servers")
		} else {
			gameServers = listResp.Msg.GameServers
		}
	} else {
		testServerID = gameServers[0].Id
		log.Info().Msgf("[E2E Setup] Using existing game server: %s", gameServers[0].Name)
	}

	// Create test files in the game server directory for file browser tests.
	if testServerID != "" {
		testFiles := []struct {
			path      string
			isDir     bool
			content   string
		}{
			{path: "e2e-test-config.cfg", isDir: false, content: "key=value\nport=25565\n"},
			{path: "e2e-test-readme.txt", isDir: false, content: "This is a test file for E2E testing.\n"},
			{path: "e2e-test-subdir", isDir: true, content: ""},
			{path: "e2e-test-subdir/nested-file.txt", isDir: false, content: "Nested test file content.\n"},
		}
		for _, tf := range testFiles {
			_, errFile := client.rpc.GameServersFileOrDirectoryCreate(ctx, connect.NewRequest(&xylona.GameServerFileOrDirectoryCreateRequest{
				GameServerId: testServerID,
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

	// Save test state.
	errSaveState := saveTestState(e2eDir, &testState{
		GameServerID: testServerID,
		GameID:       testGameID,
		GameName:     "E2E Test Game",
	})
	if errSaveState != nil {
		return fmt.Errorf("save test state: %w", errSaveState)
	}

	// Assign RBAC roles if game servers exist.
	if len(gameServers) > 0 {
		firstServer := gameServers[0]
		log.Info().Msgf("[E2E Setup] Assigning RBAC roles on game server: %s", firstServer.Name)

		for _, user := range createdUsers {
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
				continue // e2e-noaccess gets no role
			}

			_, errGrant := client.rpc.GrantGameServerAccess(ctx, connect.NewRequest(&xylona.GrantGameServerAccessRequest{
				GameServerId: firstServer.Id,
				UserId:       user.ID,
				RoleId:       roleID,
			}))
			if errGrant != nil {
				log.Warn().Err(errGrant).Msgf("[E2E Setup] Warning: could not grant access to %s", user.Username)
			} else {
				log.Info().Msgf("[E2E Setup] Granted %s on %s to %s", roleID, firstServer.Name, user.Username)
			}
		}
	} else {
		log.Info().Msg("[E2E Setup] No game servers found; skipping RBAC role assignments")
	}

	log.Info().Msg("[E2E Setup] Setup complete")
	return nil
}
