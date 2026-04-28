package rpc

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestAcceptMinecraftEulaAutoStartsWhenReadinessComplete(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	ctx := context.Background()

	_, errGame := fixture.conn.SQLDb.ExecContext(
		ctx,
		`update game
		 set linux_base_command = ?,
		     linux_start_args_template = ?,
		     windows_base_command = ?,
		     windows_start_args_template = ?
		 where id = ?`,
		"java",
		`[{"id":"jar","order":1,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"]}]`,
		"java",
		`[{"id":"jar","order":1,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"]}]`,
		"minecraft",
	)
	if errGame != nil {
		t.Fatalf("configure minecraft start command error = %v", errGame)
	}

	_, errServer := fixture.conn.SQLDb.ExecContext(
		ctx,
		`update game_server
		 set directory = ?,
		     server_executable = ?
		 where id = ?`,
		"/srv/server-local-1",
		"server.jar",
		"server-local-1",
	)
	if errServer != nil {
		t.Fatalf("configure minecraft server error = %v", errServer)
	}

	fakeNode := &nodeclient.FakeNodeClient{
		NodeID: "node-local",
		SnapshotResult: &node.NodeSnapshot{
			OS: "linux",
		},
		RuntimeCapabilitiesResult: node.RuntimeCapabilities{LaunchEnv: true},
	}
	registry := noderegistry.New("node-local", fakeNode)
	fixture.service.nodeRegistry = registry
	fixture.service.actionsInst = actions.NewInstance(
		ctx,
		fixture.conn,
		fakeNode,
		registry,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	request := connect.NewRequest(&xylona.AcceptMinecraftEulaRequest{
		ServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	response, errAccept := fixture.service.AcceptMinecraftEula(ctx, request)
	if errAccept != nil {
		t.Fatalf("AcceptMinecraftEula() error = %v", errAccept)
	}
	item := findReadinessItem(t, response.Msg.GetItems(), "minecraft_eula")
	if !item.GetComplete() || item.GetBlocking() {
		t.Fatalf("AcceptMinecraftEula().Items minecraft_eula = %+v, want complete non-blocking", item)
	}
	if len(fakeNode.StartProcessCalls) != 1 {
		t.Fatalf("StartProcess call count = %d, want 1", len(fakeNode.StartProcessCalls))
	}
	startCall := fakeNode.StartProcessCalls[0]
	if startCall.Config.ID != "server-local-1" {
		t.Fatalf("StartProcess ID = %q, want server-local-1", startCall.Config.ID)
	}
	if startCall.Config.BaseCommand != "java" {
		t.Fatalf("StartProcess base command = %q, want java", startCall.Config.BaseCommand)
	}
	if strings.Join(startCall.Config.Args, " ") != "-jar server.jar" {
		t.Fatalf("StartProcess args = %q, want -jar server.jar", strings.Join(startCall.Config.Args, " "))
	}
}

func TestSteamGSLTReadinessRPCStoresEncryptedSecret(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	steamGSLTValue := strings.Join([]string{"steam", "gslt", "fixture"}, "-")

	_, errRequireSteam := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game set requires_steam_game_server_login_token = true where id = ?`,
		"minecraft",
	)
	if errRequireSteam != nil {
		t.Fatalf("failed to mark game as requiring Steam GSLT: %v", errRequireSteam)
	}

	setRequest := connect.NewRequest(&xylona.SetSteamGSLTRequest{
		ServerId: "server-local-1",
		Token:    "  " + steamGSLTValue + "  ",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setRequest, "user-admin")

	setResponse, errSet := fixture.service.SetSteamGSLT(context.Background(), setRequest)
	if errSet != nil {
		t.Fatalf("SetSteamGSLT() error = %v", errSet)
	}
	setItem := findReadinessItem(t, setResponse.Msg.GetItems(), "steam_gslt")
	if !setItem.GetComplete() || setItem.GetBlocking() {
		t.Fatalf("SetSteamGSLT().Items steam_gslt = %+v, want complete non-blocking", setItem)
	}

	var encryptedValue string
	errScan := fixture.conn.SQLDb.QueryRowContext(
		context.Background(),
		`select value_encrypted
		 from game_server_secret
		 where game_server_id = ? and kind = ? and name = ?`,
		"server-local-1",
		db.GameServerSecretKindSteamGSLT,
		db.GameServerSecretNameSteamGSLT,
	).Scan(&encryptedValue)
	if errScan != nil {
		t.Fatalf("query raw Steam GSLT secret error = %v", errScan)
	}
	if encryptedValue == steamGSLTValue {
		t.Fatal("Steam GSLT stored as plaintext, want encrypted ciphertext")
	}

	decrypted, ok, errDecrypt := fixture.conn.DecryptGameServerSecret(
		"server-local-1",
		db.GameServerSecretKindSteamGSLT,
		db.GameServerSecretNameSteamGSLT,
	)
	if errDecrypt != nil {
		t.Fatalf("DecryptGameServerSecret(steam_gslt) error = %v", errDecrypt)
	}
	if !ok {
		t.Fatal("DecryptGameServerSecret(steam_gslt) ok = false, want true")
	}
	if decrypted != steamGSLTValue {
		t.Fatalf("DecryptGameServerSecret(steam_gslt) = %q, want %q", decrypted, steamGSLTValue)
	}

	clearRequest := connect.NewRequest(&xylona.ClearSteamGSLTRequest{
		ServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, clearRequest, "user-admin")

	clearResponse, errClear := fixture.service.ClearSteamGSLT(context.Background(), clearRequest)
	if errClear != nil {
		t.Fatalf("ClearSteamGSLT() error = %v", errClear)
	}
	clearItem := findReadinessItem(t, clearResponse.Msg.GetItems(), "steam_gslt")
	if clearItem.GetComplete() || !clearItem.GetBlocking() {
		t.Fatalf("ClearSteamGSLT().Items steam_gslt = %+v, want incomplete blocking", clearItem)
	}
}

func TestHytaleReadinessRPCLinksAndClearsAccount(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	fixture.service.hytaleAuth = readiness.NewHytaleDeviceAuthManager(&fakeHytaleReadinessClient{
		auth: readiness.HytaleDeviceAuthorization{
			DeviceCode:              "device-code",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "https://oauth.accounts.hytale.com/device",
			VerificationURIComplete: "https://oauth.accounts.hytale.com/device?user_code=ABCD-EFGH",
			ExpiresIn:               900,
		},
		token: readiness.HytaleTokenSet{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
		},
		profiles: []readiness.HytaleProfile{
			{UUID: "profile-1", Username: "PlayerOne"},
		},
	})

	_, errGame := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game_server set game_id = ? where id = ?`,
		"hytale",
		"server-local-1",
	)
	if errGame != nil {
		t.Fatalf("failed to mark server as Hytale: %v", errGame)
	}

	startRequest := connect.NewRequest(&xylona.StartHytaleDeviceAuthRequest{
		ServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, startRequest, "user-admin")
	startResponse, errStart := fixture.service.StartHytaleDeviceAuth(context.Background(), startRequest)
	if errStart != nil {
		t.Fatalf("StartHytaleDeviceAuth() error = %v", errStart)
	}
	if startResponse.Msg.GetFlowId() == "" {
		t.Fatal("StartHytaleDeviceAuth().FlowId is empty")
	}

	denyPollRequest := connect.NewRequest(&xylona.PollHytaleDeviceAuthRequest{
		FlowId: startResponse.Msg.GetFlowId(),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, denyPollRequest, "user-other")
	_, errDenyPoll := fixture.service.PollHytaleDeviceAuth(context.Background(), denyPollRequest)
	if connect.CodeOf(errDenyPoll) != connect.CodeFailedPrecondition {
		t.Fatalf("PollHytaleDeviceAuth(other user) code = %v, want %v", connect.CodeOf(errDenyPoll), connect.CodeFailedPrecondition)
	}

	pollRequest := connect.NewRequest(&xylona.PollHytaleDeviceAuthRequest{
		FlowId: startResponse.Msg.GetFlowId(),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, pollRequest, "user-admin")
	pollResponse, errPoll := fixture.service.PollHytaleDeviceAuth(context.Background(), pollRequest)
	if errPoll != nil {
		t.Fatalf("PollHytaleDeviceAuth() error = %v", errPoll)
	}
	if pollResponse.Msg.GetStatus() != string(readiness.HytaleDevicePollReady) {
		t.Fatalf("PollHytaleDeviceAuth().Status = %q, want ready", pollResponse.Msg.GetStatus())
	}
	if len(pollResponse.Msg.GetProfiles()) != 1 {
		t.Fatalf("PollHytaleDeviceAuth().Profiles length = %d, want 1", len(pollResponse.Msg.GetProfiles()))
	}

	selectRequest := connect.NewRequest(&xylona.SelectHytaleProfileRequest{
		ServerId:    "server-local-1",
		FlowId:      startResponse.Msg.GetFlowId(),
		ProfileUuid: "profile-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, selectRequest, "user-admin")
	selectResponse, errSelect := fixture.service.SelectHytaleProfile(context.Background(), selectRequest)
	if errSelect != nil {
		t.Fatalf("SelectHytaleProfile() error = %v", errSelect)
	}
	item := findReadinessItem(t, selectResponse.Msg.GetItems(), "hytale_account")
	if !item.GetComplete() || item.GetBlocking() {
		t.Fatalf("SelectHytaleProfile().Items hytale_account = %+v, want complete non-blocking", item)
	}

	refreshToken, ok, errDecrypt := fixture.conn.DecryptGameServerSecret(
		"server-local-1",
		db.GameServerSecretKindHytaleRefreshToken,
		db.GameServerSecretNameHytaleRefreshToken,
	)
	if errDecrypt != nil {
		t.Fatalf("DecryptGameServerSecret(hytale_refresh_token) error = %v", errDecrypt)
	}
	if !ok || refreshToken != "refresh-token" {
		t.Fatalf("DecryptGameServerSecret(hytale_refresh_token) = (%q, %v), want (refresh-token, true)", refreshToken, ok)
	}

	clearRequest := connect.NewRequest(&xylona.ClearHytaleAccountRequest{
		ServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, clearRequest, "user-admin")
	clearResponse, errClear := fixture.service.ClearHytaleAccount(context.Background(), clearRequest)
	if errClear != nil {
		t.Fatalf("ClearHytaleAccount() error = %v", errClear)
	}
	clearItem := findReadinessItem(t, clearResponse.Msg.GetItems(), "hytale_account")
	if clearItem.GetComplete() || !clearItem.GetBlocking() {
		t.Fatalf("ClearHytaleAccount().Items hytale_account = %+v, want incomplete blocking", clearItem)
	}
}

func findReadinessItem(t *testing.T, items []*xylona.GameServerReadinessItem, kind string) *xylona.GameServerReadinessItem {
	t.Helper()
	for _, item := range items {
		if item.GetKind() == kind {
			return item
		}
	}
	t.Fatalf("readiness item %q not found", kind)
	return nil
}

type fakeHytaleReadinessClient struct {
	auth     readiness.HytaleDeviceAuthorization
	token    readiness.HytaleTokenSet
	profiles []readiness.HytaleProfile
}

func (f *fakeHytaleReadinessClient) StartDeviceAuth(context.Context) (readiness.HytaleDeviceAuthorization, error) {
	return f.auth, nil
}

func (f *fakeHytaleReadinessClient) PollDeviceAuth(context.Context, string) (readiness.HytaleTokenSet, readiness.HytaleDevicePollStatus, error) {
	return f.token, readiness.HytaleDevicePollReady, nil
}

func (f *fakeHytaleReadinessClient) ListProfiles(context.Context, string) ([]readiness.HytaleProfile, error) {
	return f.profiles, nil
}

func (f *fakeHytaleReadinessClient) RefreshOAuth(context.Context, string) (readiness.HytaleTokenSet, error) {
	return readiness.HytaleTokenSet{}, nil
}

func (f *fakeHytaleReadinessClient) CreateGameSession(context.Context, string, string) (readiness.HytaleGameSession, error) {
	return readiness.HytaleGameSession{}, nil
}
