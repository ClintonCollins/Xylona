package rpc

import (
	"context"
	"runtime"
	"testing"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func insertStartArgsTestServer(t *testing.T, fixture *rbacRPCFixture, serverID string, gameID string) {
	t.Helper()

	_, errInsert := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		serverID, "user-owner", "Start Args Server", gameID, "OFFLINE",
		20, 20, "world", "127.0.0.1", 28010, 28011, t.TempDir(), "node-local", "[]",
	)
	if errInsert != nil {
		t.Fatalf("insert start args test server error = %v", errInsert)
	}
}

func TestUpdateGameStartArgsTemplateRedactsHiddenFieldsForNonSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	game := addGameForTests(t, fixture, "start-args-game", "Structured Args Game")

	templateJSON := `[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx4G"],"label":"Heap"}]`
	updateReq := connect.NewRequest(&xylona.UpdateGameStartArgsTemplateRequest{
		GameId:               game.GetId(),
		Platform:             "linux",
		StartArgsTemplate:    templateJSON,
		BaseCommand:          "java",
		AllowStartArgEditing: false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-admin")

	updateResp, errUpdate := fixture.service.UpdateGameStartArgsTemplate(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("UpdateGameStartArgsTemplate() error = %v", errUpdate)
	}
	if updateResp.Msg.GetGame().GetLinuxStartArgsTemplate() != templateJSON {
		t.Fatalf("UpdateGameStartArgsTemplate().Game.LinuxStartArgsTemplate = %q, want %q", updateResp.Msg.GetGame().GetLinuxStartArgsTemplate(), templateJSON)
	}
	if updateResp.Msg.GetGame().GetLinuxBaseCommand() != "java" {
		t.Fatalf("UpdateGameStartArgsTemplate().Game.LinuxBaseCommand = %q, want %q", updateResp.Msg.GetGame().GetLinuxBaseCommand(), "java")
	}
	if updateResp.Msg.GetGame().GetAllowStartArgEditing() {
		t.Fatalf("UpdateGameStartArgsTemplate().Game.AllowStartArgEditing = true, want false")
	}

	getReq := connect.NewRequest(&xylona.GetGameRequest{Id: game.GetId()})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-owner")

	getResp, errGet := fixture.service.GetGame(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetGame(non-superuser) error = %v", errGet)
	}
	if getResp.Msg.GetGame().GetLinuxStartArgsTemplate() != "" {
		t.Fatalf("GetGame(non-superuser).Game.LinuxStartArgsTemplate = %q, want empty", getResp.Msg.GetGame().GetLinuxStartArgsTemplate())
	}
	if getResp.Msg.GetGame().GetLinuxBaseCommand() != "" {
		t.Fatalf("GetGame(non-superuser).Game.LinuxBaseCommand = %q, want empty", getResp.Msg.GetGame().GetLinuxBaseCommand())
	}
	if getResp.Msg.GetGame().GetStartArgBlocklist() != "" {
		t.Fatalf("GetGame(non-superuser).Game.StartArgBlocklist = %q, want empty", getResp.Msg.GetGame().GetStartArgBlocklist())
	}
	if !game.GetAllowStartArgEditing() && getResp.Msg.GetGame().GetAllowStartArgEditing() {
		t.Fatalf("GetGame(non-superuser).Game.AllowStartArgEditing = true, want false")
	}

	getAdminReq := connect.NewRequest(&xylona.GetGameRequest{Id: game.GetId()})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getAdminReq, "user-admin")

	getAdminResp, errGetAdmin := fixture.service.GetGame(context.Background(), getAdminReq)
	if errGetAdmin != nil {
		t.Fatalf("GetGame(superuser) error = %v", errGetAdmin)
	}
	if getAdminResp.Msg.GetGame().GetLinuxStartArgsTemplate() != templateJSON {
		t.Fatalf("GetGame(superuser).Game.LinuxStartArgsTemplate = %q, want %q", getAdminResp.Msg.GetGame().GetLinuxStartArgsTemplate(), templateJSON)
	}
}

func TestUpdateGameStartArgBlocklistRejectsInvalidRegex(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	game := addGameForTests(t, fixture, "start-args-blocklist-game", "Structured Args Blocklist Game")

	req := connect.NewRequest(&xylona.UpdateGameStartArgBlocklistRequest{
		GameId:            game.GetId(),
		StartArgBlocklist: `[{"pattern":"[","reason":"broken"}]`,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	_, errUpdate := fixture.service.UpdateGameStartArgBlocklist(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("UpdateGameStartArgBlocklist() error = nil, want invalid argument")
	}
	if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateGameStartArgBlocklist() code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
	}
}

func TestOfficialGameStartArgsDefinitionEditsMarkDiverged(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		apply func(t *testing.T, fixture *rbacRPCFixture, gameID string)
	}{
		{
			name: "template update",
			id:   "template-update",
			apply: func(t *testing.T, fixture *rbacRPCFixture, gameID string) {
				t.Helper()

				req := connect.NewRequest(&xylona.UpdateGameStartArgsTemplateRequest{
					GameId:   gameID,
					Platform: "linux",
					StartArgsTemplate: `[
						{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"],"label":"Jar"},
						{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx4G"],"label":"Heap"}
					]`,
					BaseCommand:          "java",
					AllowStartArgEditing: true,
				})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

				_, errUpdate := fixture.service.UpdateGameStartArgsTemplate(context.Background(), req)
				if errUpdate != nil {
					t.Fatalf("UpdateGameStartArgsTemplate() error = %v", errUpdate)
				}
			},
		},
		{
			name: "blocklist update",
			id:   "blocklist-update",
			apply: func(t *testing.T, fixture *rbacRPCFixture, gameID string) {
				t.Helper()

				req := connect.NewRequest(&xylona.UpdateGameStartArgBlocklistRequest{
					GameId:            gameID,
					StartArgBlocklist: `[{"pattern":"^-debug$","reason":"debug flag is not allowed"}]`,
				})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

				_, errUpdate := fixture.service.UpdateGameStartArgBlocklist(context.Background(), req)
				if errUpdate != nil {
					t.Fatalf("UpdateGameStartArgBlocklist() error = %v", errUpdate)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			game := addGameForTests(t, fixture, "official-start-args-"+test.id, "Official Start Args "+test.name)

			_, errSetOfficial := fixture.conn.SQLDb.ExecContext(
				context.Background(),
				"UPDATE game SET xylona_official = true, official_definition_diverged = false WHERE id = ?",
				game.GetId(),
			)
			if errSetOfficial != nil {
				t.Fatalf("set official game setup error = %v", errSetOfficial)
			}

			test.apply(t, fixture, game.GetId())

			updated, errGame := fixture.conn.GetGameByID(game.GetId())
			if errGame != nil {
				t.Fatalf("GetGameByID() error = %v", errGame)
			}
			if !updated.OfficialDefinitionDiverged {
				t.Fatal("OfficialDefinitionDiverged = false, want true")
			}
		})
	}
}

func TestUpdateGameServerStartArgsRequiresEditingEnabledForNonSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	game := addGameForTests(t, fixture, "start-args-server-game", "Structured Args Server Game")
	insertStartArgsTestServer(t, fixture, "server-start-args-1", game.GetId())

	templateJSON := `[
		{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"],"label":"Heap"},
		{"id":"jar","order":2,"ownership":"system","tokens":["-jar","server.jar"],"label":"Jar"}
	]`
	updateReq := connect.NewRequest(&xylona.UpdateGameStartArgsTemplateRequest{
		GameId:               game.GetId(),
		Platform:             "linux",
		StartArgsTemplate:    templateJSON,
		BaseCommand:          "java",
		AllowStartArgEditing: false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-admin")

	_, errUpdateTemplate := fixture.service.UpdateGameStartArgsTemplate(context.Background(), updateReq)
	if errUpdateTemplate != nil {
		t.Fatalf("UpdateGameStartArgsTemplate() setup error = %v", errUpdateTemplate)
	}

	ownerReq := connect.NewRequest(&xylona.UpdateGameServerStartArgsRequest{
		ServerId:         "server-start-args-1",
		StartArgsPatches: `[{"id":"heap","op":"edit","tokens":["-Xmx3G"],"afterId":null}]`,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, ownerReq, "user-owner")

	_, errOwner := fixture.service.UpdateGameServerStartArgs(context.Background(), ownerReq)
	if errOwner == nil {
		t.Fatalf("UpdateGameServerStartArgs(non-superuser) error = nil, want permission denied")
	}
	if connect.CodeOf(errOwner) != connect.CodePermissionDenied {
		t.Fatalf("UpdateGameServerStartArgs(non-superuser) code = %v, want %v", connect.CodeOf(errOwner), connect.CodePermissionDenied)
	}

	adminReq := connect.NewRequest(&xylona.UpdateGameServerStartArgsRequest{
		ServerId:         "server-start-args-1",
		StartArgsPatches: `[{"id":"heap","op":"edit","tokens":["-Xmx3G"],"afterId":null}]`,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, adminReq, "user-admin")

	adminResp, errAdmin := fixture.service.UpdateGameServerStartArgs(context.Background(), adminReq)
	if errAdmin != nil {
		t.Fatalf("UpdateGameServerStartArgs(superuser) error = %v", errAdmin)
	}
	if adminResp.Msg.GetGameServer().GetStartArgsPatches() != `[{"id":"heap","op":"edit","tokens":["-Xmx3G"],"afterId":null}]` {
		t.Fatalf("UpdateGameServerStartArgs(superuser).GameServer.StartArgsPatches = %q, want persisted patches", adminResp.Msg.GetGameServer().GetStartArgsPatches())
	}
}

func TestUpdateGameServerStartArgsPrivilegedOverrides(t *testing.T) {
	template := `[
		{"id":"system","order":1,"ownership":"system","tokens":["server.jar"]},
		{"id":"locked","order":2,"ownership":"locked","tokens":["--safe"]},
		{"id":"ordinary","order":3,"ownership":"editable","tokens":["--port","28010"]}
	]`
	tests := []struct {
		name            string
		userID          string
		existingPatches string
		existingBase    string
		patches         string
		baseOverride    string
		wantErr         bool
		wantCode        connect.Code
		wantPatches     string
		wantBase        string
	}{
		{
			name:         "superuser edits locked block and base command",
			userID:       "user-admin",
			patches:      `[{"id":"locked","op":"edit","tokens":["--custom"]}]`,
			baseOverride: " ./custom-start.sh ",
			wantPatches:  `[{"id":"locked","op":"edit","tokens":["--custom"]}]`,
			wantBase:     "./custom-start.sh",
		},
		{
			name:        "superuser removes locked block",
			userID:      "user-admin",
			patches:     `[{"id":"locked","op":"remove"}]`,
			wantPatches: `[{"id":"locked","op":"remove"}]`,
		},
		{
			name:         "superuser clears base command override",
			userID:       "user-admin",
			existingBase: "./custom-start.sh",
			patches:      `[]`,
			wantPatches:  `[]`,
		},
		{
			name:     "system block remains immutable",
			userID:   "user-admin",
			patches:  `[{"id":"system","op":"remove"}]`,
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:            "ordinary edit preserves locked patch",
			userID:          "user-owner",
			existingPatches: `[{"id":"locked","op":"edit","tokens":["--custom"]}]`,
			existingBase:    "./custom-start.sh",
			patches:         `[{"id":"ordinary","op":"edit","tokens":["--port","28012"]},{"id":"locked","op":"edit","tokens":["--custom"]}]`,
			baseOverride:    "./custom-start.sh",
			wantPatches:     `[{"id":"ordinary","op":"edit","tokens":["--port","28012"]},{"id":"locked","op":"edit","tokens":["--custom"]}]`,
			wantBase:        "./custom-start.sh",
		},
		{
			name:            "non-superuser cannot tamper with locked patch",
			userID:          "user-owner",
			existingPatches: `[{"id":"locked","op":"edit","tokens":["--custom"]}]`,
			patches:         `[{"id":"locked","op":"edit","tokens":["--tampered"]}]`,
			wantErr:         true,
			wantCode:        connect.CodePermissionDenied,
		},
		{
			name:            "non-superuser cannot delete locked patch",
			userID:          "user-owner",
			existingPatches: `[{"id":"locked","op":"remove"}]`,
			patches:         `[]`,
			wantErr:         true,
			wantCode:        connect.CodePermissionDenied,
		},
		{
			name:         "non-superuser cannot change base command override",
			userID:       "user-owner",
			existingBase: "./custom-start.sh",
			patches:      `[]`,
			baseOverride: "./tampered.sh",
			wantErr:      true,
			wantCode:     connect.CodePermissionDenied,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			game := addGameForTests(t, fixture, "privileged-start-args-game", "Privileged Start Args Game")
			insertStartArgsTestServer(t, fixture, "server-privileged-start-args", game.GetId())

			_, errGame := fixture.conn.SQLDb.ExecContext(
				t.Context(),
				`UPDATE game SET linux_base_command = ?, windows_base_command = ?, linux_start_args_template = ?,
				 windows_start_args_template = ?, allow_start_arg_editing = true WHERE id = ?`,
				"definition-start", "definition-start", template, template, game.GetId(),
			)
			if errGame != nil {
				t.Fatalf("configure game error = %v", errGame)
			}

			existingPatches := test.existingPatches
			if existingPatches == "" {
				existingPatches = "[]"
			}
			_, errServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
				ID:                  omit.From("server-privileged-start-args"),
				StartArgsPatches:    omit.From(existingPatches),
				BaseCommandOverride: omit.From(test.existingBase),
			})
			if errServer != nil {
				t.Fatalf("configure game server error = %v", errServer)
			}

			req := connect.NewRequest(&xylona.UpdateGameServerStartArgsRequest{
				ServerId:            "server-privileged-start-args",
				StartArgsPatches:    test.patches,
				BaseCommandOverride: test.baseOverride,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, test.userID)

			_, errUpdate := fixture.service.UpdateGameServerStartArgs(t.Context(), req)
			if test.wantErr {
				if connect.CodeOf(errUpdate) != test.wantCode {
					t.Fatalf("UpdateGameServerStartArgs() code = %v, want %v (error %v)", connect.CodeOf(errUpdate), test.wantCode, errUpdate)
				}
				return
			}
			if errUpdate != nil {
				t.Fatalf("UpdateGameServerStartArgs() error = %v", errUpdate)
			}

			updated, errGet := fixture.conn.GetGameServerByID("server-privileged-start-args")
			if errGet != nil {
				t.Fatalf("GetGameServerByID() error = %v", errGet)
			}
			if updated.StartArgsPatches != test.wantPatches {
				t.Fatalf("StartArgsPatches = %q, want %q", updated.StartArgsPatches, test.wantPatches)
			}
			if updated.BaseCommandOverride != test.wantBase {
				t.Fatalf("BaseCommandOverride = %q, want %q", updated.BaseCommandOverride, test.wantBase)
			}
		})
	}
}

func TestUpdateGameServerStartArgsRejectsBlockedResolvedArgs(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	game := addGameForTests(t, fixture, "start-args-blocked-game", "Structured Args Blocked Game")
	insertStartArgsTestServer(t, fixture, "server-start-args-2", game.GetId())

	// Set the template for both platforms so this test runs uniformly on any
	// host OS — the hub-spoke Node model no longer carries an "os" column that
	// the old test relied on to pin the platform to linux.
	for _, platform := range []string{"linux", "windows"} {
		templateReq := connect.NewRequest(&xylona.UpdateGameStartArgsTemplateRequest{
			GameId:   game.GetId(),
			Platform: platform,
			StartArgsTemplate: `[
				{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"],"label":"Heap"},
				{"id":"jar","order":2,"ownership":"system","tokens":["-jar","server.jar"],"label":"Jar"}
			]`,
			BaseCommand:          "java",
			AllowStartArgEditing: true,
		})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, templateReq, "user-admin")

		_, errTemplate := fixture.service.UpdateGameStartArgsTemplate(context.Background(), templateReq)
		if errTemplate != nil {
			t.Fatalf("UpdateGameStartArgsTemplate(%s) setup error = %v", platform, errTemplate)
		}
	}

	blocklistReq := connect.NewRequest(&xylona.UpdateGameStartArgBlocklistRequest{
		GameId:            game.GetId(),
		StartArgBlocklist: `[{"pattern":"^-Xmx32G$","reason":"too much memory"}]`,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, blocklistReq, "user-admin")

	_, errBlocklist := fixture.service.UpdateGameStartArgBlocklist(context.Background(), blocklistReq)
	if errBlocklist != nil {
		t.Fatalf("UpdateGameStartArgBlocklist() setup error = %v", errBlocklist)
	}

	req := connect.NewRequest(&xylona.UpdateGameServerStartArgsRequest{
		ServerId:         "server-start-args-2",
		StartArgsPatches: `[{"id":"heap","op":"edit","tokens":["-Xmx32G"],"afterId":null}]`,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-owner")

	_, errUpdate := fixture.service.UpdateGameServerStartArgs(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("UpdateGameServerStartArgs() error = nil, want invalid argument")
	}
	if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateGameServerStartArgs() code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
	}
}

func TestUpdateGameServerStartArgsRedactsBackupDirectoryForNonSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	game := addGameForTests(t, fixture, "start-args-redaction-game", "Structured Args Redaction Game")
	insertStartArgsTestServer(t, fixture, "server-start-args-3", game.GetId())

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-start-args-3"),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From("/srv/start-args-backups"),
		MaxBackups:      omit.From(int64(9)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	templateReq := connect.NewRequest(&xylona.UpdateGameStartArgsTemplateRequest{
		GameId:   game.GetId(),
		Platform: "linux",
		StartArgsTemplate: `[
			{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"],"label":"Heap"},
			{"id":"jar","order":2,"ownership":"system","tokens":["-jar","server.jar"],"label":"Jar"}
		]`,
		BaseCommand:          "java",
		AllowStartArgEditing: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, templateReq, "user-admin")

	_, errTemplate := fixture.service.UpdateGameStartArgsTemplate(context.Background(), templateReq)
	if errTemplate != nil {
		t.Fatalf("UpdateGameStartArgsTemplate() setup error = %v", errTemplate)
	}

	req := connect.NewRequest(&xylona.UpdateGameServerStartArgsRequest{
		ServerId:         "server-start-args-3",
		StartArgsPatches: `[{"id":"heap","op":"edit","tokens":["-Xmx3G"],"afterId":null}]`,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-owner")

	resp, errUpdate := fixture.service.UpdateGameServerStartArgs(context.Background(), req)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerStartArgs(non-superuser) error = %v", errUpdate)
	}
	if resp.Msg.GetGameServer().GetStartArgsPatches() != `[{"id":"heap","op":"edit","tokens":["-Xmx3G"],"afterId":null}]` {
		t.Fatalf("UpdateGameServerStartArgs(non-superuser).GameServer.StartArgsPatches = %q, want persisted patches", resp.Msg.GetGameServer().GetStartArgsPatches())
	}
	if !resp.Msg.GetGameServer().GetBackupsEnabled() {
		t.Fatal("UpdateGameServerStartArgs(non-superuser).GameServer.BackupsEnabled = false, want true")
	}
	if resp.Msg.GetGameServer().GetBackupDirectory() != "" {
		t.Fatalf("UpdateGameServerStartArgs(non-superuser).GameServer.BackupDirectory = %q, want empty", resp.Msg.GetGameServer().GetBackupDirectory())
	}
	if resp.Msg.GetGameServer().GetMaxBackups() != 9 {
		t.Fatalf("UpdateGameServerStartArgs(non-superuser).GameServer.MaxBackups = %d, want %d", resp.Msg.GetGameServer().GetMaxBackups(), 9)
	}
}

func TestUpdateGameServerStartArgsUsesOwningNodePlatform(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	game := addGameForTests(t, fixture, "start-args-remote-platform-game", "Remote Platform Game")
	insertStartArgsTestServer(t, fixture, "server-start-args-remote-platform", game.GetId())

	remoteOS := "windows"
	remotePlatform := "windows"
	if runtime.GOOS == "windows" {
		remoteOS = "linux"
		remotePlatform = "linux"
	}

	_, errNode := fixture.conn.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-remote-platform"),
		Name:      omit.From("Remote Platform Node"),
		ListenURL: omit.From("https://remote-platform.example.com"),
		Enabled:   omit.From(true),
	})
	if errNode != nil {
		t.Fatalf("InsertNode() error = %v", errNode)
	}
	insertNodeScopedIPForParityTests(t, fixture, "node-remote-platform", "127.0.0.1")

	_, errServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:     omit.From("server-start-args-remote-platform"),
		NodeID: omit.From("node-remote-platform"),
	})
	if errServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errServer)
	}

	fixture.service.nodeRegistry = noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	fixture.service.nodeRegistry.Register(&nodeclient.FakeNodeClient{
		NodeID: "node-remote-platform",
		SnapshotResult: &node.NodeSnapshot{
			OS: remoteOS,
		},
	})

	templateReq := connect.NewRequest(&xylona.UpdateGameStartArgsTemplateRequest{
		GameId:   game.GetId(),
		Platform: remotePlatform,
		StartArgsTemplate: `[
			{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"],"label":"Heap"},
			{"id":"jar","order":2,"ownership":"system","tokens":["-jar","server.jar"],"label":"Jar"}
		]`,
		BaseCommand:          "java",
		AllowStartArgEditing: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, templateReq, "user-admin")

	_, errTemplate := fixture.service.UpdateGameStartArgsTemplate(context.Background(), templateReq)
	if errTemplate != nil {
		t.Fatalf("UpdateGameStartArgsTemplate() setup error = %v", errTemplate)
	}

	req := connect.NewRequest(&xylona.UpdateGameServerStartArgsRequest{
		ServerId:         "server-start-args-remote-platform",
		StartArgsPatches: `[{"id":"heap","op":"edit","tokens":["-Xmx3G"],"afterId":null}]`,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-owner")

	resp, errUpdate := fixture.service.UpdateGameServerStartArgs(context.Background(), req)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerStartArgs() error = %v", errUpdate)
	}
	if resp.Msg.GetGameServer().GetStartArgsPatches() != `[{"id":"heap","op":"edit","tokens":["-Xmx3G"],"afterId":null}]` {
		t.Fatalf("UpdateGameServerStartArgs().GameServer.StartArgsPatches = %q, want persisted patches", resp.Msg.GetGameServer().GetStartArgsPatches())
	}
}
