package main

import (
	"context"
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

// minecraftServerSoftwareJSON matches the Minecraft preset from frontend/src/utils/game-presets.ts.
// It defines Vanilla, Paper, Purpur, and Fabric server software options.
const minecraftServerSoftwareJSON = `[{"id":"vanilla","name":"Vanilla","jar_source":null,"mod_config":null},{"id":"paper","name":"Paper","jar_source":"papermc","mod_config":{"mod_types":[{"type":"plugin","label":"Plugins","install_path":"plugins/"}],"sources":[{"id":"modrinth","search_params":{"facets":{"project_type":"plugin","categories":["paper","spigot","bukkit"]}}},{"id":"hangar","search_params":{"platform":"PAPER"}}]}},{"id":"purpur","name":"Purpur","jar_source":"papermc","mod_config":{"mod_types":[{"type":"plugin","label":"Plugins","install_path":"plugins/"}],"sources":[{"id":"modrinth","search_params":{"facets":{"project_type":"plugin","categories":["purpur","paper","spigot","bukkit"]}}},{"id":"hangar","search_params":{"platform":"PAPER"}}]}},{"id":"fabric","name":"Fabric","jar_source":null,"mod_config":{"mod_types":[{"type":"mod","label":"Mods","install_path":"mods/"}],"sources":[{"id":"modrinth","search_params":{"facets":{"project_type":"mod","categories":["fabric"]}}}]}}]`

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

func runSingleSetup(ctx context.Context, httpPort, fedPort int, adminUsername, adminPassword, e2eDir, projectRoot string) error {
	backendURL := fmt.Sprintf("http://localhost:%d", httpPort)
	log.Info().Msgf("[E2E Setup] Target backend: %s", backendURL)

	// Acquire lock to prevent concurrent runs.
	errLock := acquireLock(e2eDir, "single", map[string]int{
		"http":       httpPort,
		"federation": fedPort,
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
	errMkdir := os.MkdirAll(dataDir, 0o755)
	if errMkdir != nil {
		return fmt.Errorf("create data dir: %w", errMkdir)
	}

	// Clean any leftover database from a previous run.
	dbFile := filepath.Join(dataDir, "data.sqlite")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(dbFile + suffix)
	}
	// Clean leftover federation identity.
	_ = os.RemoveAll(filepath.Join(dataDir, "federation"))

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
	backendCmd, errStart := startNode("E2E-Backend", dataDir, xylonaExe, httpPort, fedPort)
	if errStart != nil {
		return fmt.Errorf("start backend: %w", errStart)
	}

	// Write PID file for teardown.
	if backendCmd.Process != nil {
		pidFile := filepath.Join(dataDir, "xylona.pid")
		_ = os.WriteFile(pidFile, []byte(strconv.Itoa(backendCmd.Process.Pid)), 0o644)
	}

	// Everything after process start is wrapped so we kill the process on failure.
	errSetup := func() error {
		// Wait for the backend to be ready.
		log.Info().Msg("[E2E Setup] Waiting for backend to be ready...")
		errWait := waitForReady(ctx, backendURL, 30*time.Second)
		if errWait != nil {
			return fmt.Errorf("wait for backend: %w", errWait)
		}
		log.Info().Msg("[E2E Setup] Backend is ready")

		// Build the dummy game server binary.
		e2eBinDir := filepath.Join(dataDir, "bin")
		dummyServerExe := filepath.Join(e2eBinDir, binaryName("dummy-game-server"))
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
						Name:                           "E2E Test Game",
						LinuxSupport:                   true,
						LinuxStartCommand:              dummyExePath,
						LinuxStopCommand:               "stop",
						LinuxInstallCommand:            "echo installed",
						LinuxInstallCommandProcessor:   xylona.CommandProcessor_BASH,
						WindowsSupport:                 true,
						WindowsStartCommand:            dummyExePath,
						WindowsStopCommand:             "stop",
						WindowsInstallCommand:          `cmd /c "echo installed"`,
						WindowsInstallCommandProcessor: xylona.CommandProcessor_CMD,
						DefaultPort:                    25599,
						DefaultQueryPort:               25599,
						ServerSoftware:                 minecraftServerSoftwareJSON,
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
			gsDir := filepath.Join(dataDir, "bin", "test-server")
			errMkdirGS := os.MkdirAll(gsDir, 0o755)
			if errMkdirGS != nil {
				return fmt.Errorf("create game server dir: %w", errMkdirGS)
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

			// Set server software to "paper" so the Mods tab is available.
			_, errSetSoftware := client.rpc.SetServerSoftware(ctx, connect.NewRequest(&xylona.SetServerSoftwareRequest{
				GameServerId: testServerID,
				SoftwareId:   "paper",
				VersionId:    "1.21.4",
			}))
			if errSetSoftware != nil {
				log.Warn().Err(errSetSoftware).Msg("[E2E Setup] Warning: could not set server software to paper")
			} else {
				log.Info().Msg("[E2E Setup] Set server software to Paper")
			}

			// Start the game server.
			_, errStartGS := client.rpc.StartGameServer(ctx, connect.NewRequest(&xylona.StartGameServerRequest{
				ServerId: testServerID,
			}))
			if errStartGS != nil {
				log.Warn().Err(errStartGS).Msg("[E2E Setup] Warning: could not start game server")
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
			testGameID = gameServers[0].GameId
			log.Info().Msgf("[E2E Setup] Using existing game server: %s", gameServers[0].Name)
		}

		// Ensure the game has server_software configured for mod management tests.
		// Always update — the field may have been cleared or missing.
		if testGameID != "" {
			gameResp, errGetGame := client.rpc.GetGame(ctx, connect.NewRequest(&xylona.GetGameRequest{
				Id: testGameID,
			}))
			if errGetGame != nil {
				log.Warn().Err(errGetGame).Msg("[E2E Setup] Warning: could not fetch game")
			} else if gameResp.Msg.Game != nil && gameResp.Msg.Game.ServerSoftware == "" {
				log.Info().Msg("[E2E Setup] Updating game with server_software config...")
				gameToUpdate := gameResp.Msg.Game
				gameToUpdate.ServerSoftware = minecraftServerSoftwareJSON
				_, errEdit := client.rpc.EditGame(ctx, connect.NewRequest(&xylona.EditGameRequest{
					Game: gameToUpdate,
				}))
				if errEdit != nil {
					log.Warn().Err(errEdit).Msg("[E2E Setup] Warning: could not update game with server_software")
				} else {
					log.Info().Msg("[E2E Setup] Updated game with Minecraft server software config")
				}
			}
		}

		// Ensure the game server has server_software set to "paper".
		if testServerID != "" {
			gsResp, errGetGS := client.rpc.GetGameServer(ctx, connect.NewRequest(&xylona.GetGameServerRequest{
				Id: testServerID,
			}))
			if errGetGS != nil {
				log.Warn().Err(errGetGS).Msg("[E2E Setup] Warning: could not fetch game server")
			} else if gsResp.Msg.GameServer != nil && gsResp.Msg.GameServer.ServerSoftware == "" {
				log.Info().Msg("[E2E Setup] Setting server software to Paper...")
				_, errSetSoftware := client.rpc.SetServerSoftware(ctx, connect.NewRequest(&xylona.SetServerSoftwareRequest{
					GameServerId: testServerID,
					SoftwareId:   "paper",
					VersionId:    "1.21.4",
				}))
				if errSetSoftware != nil {
					log.Warn().Err(errSetSoftware).Msg("[E2E Setup] Warning: could not set server software to paper")
				} else {
					log.Info().Msg("[E2E Setup] Set server software to Paper")
				}
			}
		}

		// Create test files in the game server directory for file browser tests.
		if testServerID != "" {
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

		// Create a "no-tracker" game and server for version tracking E2E tests.
		noTrackerServerID := ""

		dummyExePath := strings.ReplaceAll(dummyServerExe, "\\", "/")

		gamesResp2, errGames2 := client.rpc.ListGames(ctx, connect.NewRequest(&xylona.ListGamesRequest{}))
		if errGames2 == nil {
			var noTrackerGame *xylona.Game
			for _, g := range gamesResp2.Msg.Games {
				if g.Name == "E2E No-Tracker Game" {
					noTrackerGame = g
					break
				}
			}
			if noTrackerGame == nil {
				addResp2, errAdd2 := client.rpc.AddGame(ctx, connect.NewRequest(&xylona.AddGameRequest{
					Game: &xylona.Game{
						Name:                "E2E No-Tracker Game",
						LinuxSupport:        true,
						LinuxStartCommand:   dummyExePath,
						WindowsSupport:      true,
						WindowsStartCommand: dummyExePath,
						DefaultPort:         25601,
						DefaultQueryPort:    25602,
					},
				}))
				if errAdd2 != nil {
					log.Warn().Err(errAdd2).Msg("[E2E Setup] Warning: could not create no-tracker game")
				} else {
					noTrackerGame = addResp2.Msg.Game
					log.Info().Msgf("[E2E Setup] Created no-tracker game: %s", noTrackerGame.Id)
				}
			}
			if noTrackerGame != nil {
				ntGsDir := filepath.Join(dataDir, "bin", "no-tracker-server")
				errMkdirNT := os.MkdirAll(ntGsDir, 0o755)
				if errMkdirNT != nil {
					log.Warn().Err(errMkdirNT).Msg("[E2E Setup] Warning: could not create no-tracker server dir")
				} else {
					// Check if a no-tracker server already exists.
					listResp3, errList3 := client.rpc.ListGameServers(ctx, connect.NewRequest(&xylona.ListGameServersRequest{}))
					if errList3 == nil {
						for _, gs := range listResp3.Msg.GameServers {
							if gs.Name == "E2E No-Tracker Server" {
								noTrackerServerID = gs.Id
								break
							}
						}
					}
					if noTrackerServerID == "" {
						ipsResp2, errIPs2 := client.rpc.ListIPs(ctx, connect.NewRequest(&xylona.ListIPsRequest{}))
						ipAddress2 := "0.0.0.0"
						if errIPs2 == nil && len(ipsResp2.Msg.Ips) > 0 {
							ipAddress2 = ipsResp2.Msg.Ips[0].Address
						}
						ntGsDirPath := strings.ReplaceAll(ntGsDir, "\\", "/")
						createResp2, errCreate2 := client.rpc.CreateGameServer(ctx, connect.NewRequest(&xylona.CreateGameServerRequest{
							GameServer: &xylona.GameServer{
								Name:          "E2E No-Tracker Server",
								GameId:        noTrackerGame.Id,
								UserId:        client.userID,
								StartCommand:  dummyExePath + " -heartbeat 5s",
								Directory:     ntGsDirPath,
								Ip:            &xylona.IP{Address: ipAddress2},
								Port:          25601,
								QueryPort:     25602,
								SetMaxPlayers: 20,
							},
						}))
						if errCreate2 != nil {
							log.Warn().Err(errCreate2).Msg("[E2E Setup] Warning: could not create no-tracker server")
						} else {
							noTrackerServerID = createResp2.Msg.GameServer.Id
							log.Info().Msgf("[E2E Setup] Created no-tracker server: %s", noTrackerServerID)
						}
					}
				}
			}
		}

		// Save test state.
		errSaveState := saveTestState(e2eDir, &testState{
			GameServerID:      testServerID,
			GameID:            testGameID,
			GameName:          "E2E Test Game",
			NoTrackerServerID: noTrackerServerID,
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
	}()

	if errSetup != nil {
		log.Error().Err(errSetup).Msg("[E2E Setup] Setup failed, killing backend process")
		killProcess(backendCmd)
		return errSetup
	}

	setupOK = true
	return nil
}
