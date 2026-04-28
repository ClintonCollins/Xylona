package readiness

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestCheckStartSteamGSLTBlocksUntilSecretConfigured(t *testing.T) {
	conn := newReadinessSecretConnection(t)
	gameServer := &models.GameServer{ID: "server-1"}
	gameServer.R.Game = &models.Game{RequiresSteamGameServerLoginToken: true}

	errMissing := CheckStart(context.Background(), conn, gameServer, nil)
	if errMissing == nil {
		t.Fatal("CheckStart() error = nil, want Steam GSLT required")
	}
	if !strings.Contains(errMissing.Error(), "Steam GSLT required") {
		t.Fatalf("CheckStart() error = %v, want Steam GSLT required", errMissing)
	}

	errSet := SetSteamGSLT(conn, gameServer.ID, "steam-token", "user-admin")
	if errSet != nil {
		t.Fatalf("SetSteamGSLT() error = %v", errSet)
	}

	errReady := CheckStart(context.Background(), conn, gameServer, nil)
	if errReady != nil {
		t.Fatalf("CheckStart() after SetSteamGSLT error = %v", errReady)
	}
}

func TestHytaleReadinessBlocksUntilLinkedAndLaunchEnvSupported(t *testing.T) {
	conn := newReadinessSecretConnection(t)
	gameServer := &models.GameServer{ID: "server-1", GameID: "hytale"}
	client := &nodeclient.FakeNodeClient{
		RuntimeCapabilitiesResult: node.RuntimeCapabilities{LaunchEnv: true},
	}

	errMissing := CheckStart(context.Background(), conn, gameServer, client)
	if errMissing == nil {
		t.Fatal("CheckStart() error = nil, want Hytale account link required")
	}
	if !strings.Contains(errMissing.Error(), "Hytale account link required") {
		t.Fatalf("CheckStart() error = %v, want Hytale account link required", errMissing)
	}

	errPersist := PersistHytaleAccount(conn, gameServer.ID, HytaleProfile{
		UUID:     "profile-1",
		Username: "PlayerOne",
	}, "refresh-token", "user-admin")
	if errPersist != nil {
		t.Fatalf("PersistHytaleAccount() error = %v", errPersist)
	}

	client.RuntimeCapabilitiesResult = node.RuntimeCapabilities{LaunchEnv: false}
	errNoLaunchEnv := CheckStart(context.Background(), conn, gameServer, client)
	if errNoLaunchEnv == nil {
		t.Fatal("CheckStart() error = nil, want launch env support error")
	}
	if !strings.Contains(errNoLaunchEnv.Error(), "launch-only Hytale credentials") {
		t.Fatalf("CheckStart() error = %v, want launch-only Hytale credentials", errNoLaunchEnv)
	}

	client.RuntimeCapabilitiesResult = node.RuntimeCapabilities{LaunchEnv: true}
	errReady := CheckStart(context.Background(), conn, gameServer, client)
	if errReady != nil {
		t.Fatalf("CheckStart() after Hytale link error = %v", errReady)
	}
}

func TestPrepareHytaleLaunchSecretsRefreshesAndAppendsEnv(t *testing.T) {
	conn := newReadinessSecretConnection(t)
	gameServer := &models.GameServer{ID: "server-1", GameID: "hytale"}
	errPersist := PersistHytaleAccount(conn, gameServer.ID, HytaleProfile{
		UUID:     "profile-1",
		Username: "PlayerOne",
	}, "refresh-token-old", "user-admin")
	if errPersist != nil {
		t.Fatalf("PersistHytaleAccount() error = %v", errPersist)
	}

	client := &fakeHytaleClient{
		refreshTokenSet: HytaleTokenSet{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token-new",
		},
		session: HytaleGameSession{
			SessionToken:  "session-token",
			IdentityToken: "identity-token",
			ExpiresAt:     time.Now().Add(time.Hour),
		},
	}

	launchEnv, errPrepare := PrepareHytaleLaunchSecrets(
		context.Background(),
		conn,
		gameServer,
		client,
		map[string]string{"EXISTING": "value"},
	)
	if errPrepare != nil {
		t.Fatalf("PrepareHytaleLaunchSecrets() error = %v", errPrepare)
	}
	if launchEnv["EXISTING"] != "value" {
		t.Fatalf("PrepareHytaleLaunchSecrets()[EXISTING] = %q, want value", launchEnv["EXISTING"])
	}
	if launchEnv[HytaleSessionTokenEnv] != "session-token" {
		t.Fatalf("PrepareHytaleLaunchSecrets()[%s] = %q, want session-token", HytaleSessionTokenEnv, launchEnv[HytaleSessionTokenEnv])
	}
	if launchEnv[HytaleIdentityTokenEnv] != "identity-token" {
		t.Fatalf("PrepareHytaleLaunchSecrets()[%s] = %q, want identity-token", HytaleIdentityTokenEnv, launchEnv[HytaleIdentityTokenEnv])
	}
	if client.refreshTokenReceived != "refresh-token-old" {
		t.Fatalf("RefreshOAuth() token = %q, want refresh-token-old", client.refreshTokenReceived)
	}
	if client.sessionProfileUUID != "profile-1" {
		t.Fatalf("CreateGameSession() profile UUID = %q, want profile-1", client.sessionProfileUUID)
	}

	rotated, ok, errDecrypt := conn.DecryptGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindHytaleRefreshToken,
		db.GameServerSecretNameHytaleRefreshToken,
	)
	if errDecrypt != nil {
		t.Fatalf("DecryptGameServerSecret() error = %v", errDecrypt)
	}
	if !ok || rotated != "refresh-token-new" {
		t.Fatalf("DecryptGameServerSecret() = (%q, %v), want (refresh-token-new, true)", rotated, ok)
	}
}

func newReadinessSecretConnection(t *testing.T) *db.Connection {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "readiness.sqlite")
	conn, errNew := db.NewConnection(context.Background(), dbPath)
	if errNew != nil {
		t.Fatalf("NewConnection() error = %v", errNew)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("failed to close test db: %v", errClose)
		}
	})
	conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	_, errCreate := conn.SQLDb.ExecContext(
		context.Background(),
		`create table game_server_secret (
			game_server_id text not null,
			kind text not null,
			name text not null,
			value_encrypted text not null,
			updated_by_user_id text,
			created_at timestamp not null default current_timestamp,
			updated_at timestamp not null default current_timestamp,
			primary key (game_server_id, kind, name)
		)`,
	)
	if errCreate != nil {
		t.Fatalf("create game_server_secret table error = %v", errCreate)
	}

	_, errReadiness := conn.SQLDb.ExecContext(
		context.Background(),
		`create table game_server_readiness (
			game_server_id text not null,
			kind text not null,
			public_data text not null,
			updated_by_user_id text,
			created_at timestamp not null default current_timestamp,
			updated_at timestamp not null default current_timestamp,
			primary key (game_server_id, kind)
		)`,
	)
	if errReadiness != nil {
		t.Fatalf("create game_server_readiness table error = %v", errReadiness)
	}

	_, errIndex := conn.SQLDb.ExecContext(
		context.Background(),
		`create unique index game_server_secret_lower_name_idx
		 on game_server_secret (game_server_id, kind, lower(name))`,
	)
	if errIndex != nil {
		t.Fatalf("create game_server_secret lower-name index error = %v", errIndex)
	}

	return conn
}

type fakeHytaleClient struct {
	refreshTokenSet      HytaleTokenSet
	refreshTokenReceived string
	session              HytaleGameSession
	sessionAccessToken   string
	sessionProfileUUID   string
}

func (f *fakeHytaleClient) StartDeviceAuth(context.Context) (HytaleDeviceAuthorization, error) {
	return HytaleDeviceAuthorization{}, nil
}

func (f *fakeHytaleClient) PollDeviceAuth(context.Context, string) (HytaleTokenSet, HytaleDevicePollStatus, error) {
	return HytaleTokenSet{}, HytaleDevicePollPending, nil
}

func (f *fakeHytaleClient) ListProfiles(context.Context, string) ([]HytaleProfile, error) {
	return nil, nil
}

func (f *fakeHytaleClient) RefreshOAuth(_ context.Context, refreshToken string) (HytaleTokenSet, error) {
	f.refreshTokenReceived = refreshToken
	return f.refreshTokenSet, nil
}

func (f *fakeHytaleClient) CreateGameSession(_ context.Context, accessToken string, profileUUID string) (HytaleGameSession, error) {
	f.sessionAccessToken = accessToken
	f.sessionProfileUUID = profileUUID
	return f.session, nil
}
