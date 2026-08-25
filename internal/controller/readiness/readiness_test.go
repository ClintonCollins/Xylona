package readiness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestCheckStartMinecraftEULABlocksUntilAccepted(t *testing.T) {
	conn := newReadinessSecretConnection(t)
	gameServer := &models.GameServer{
		ID:        "server-1",
		GameID:    "minecraft",
		Directory: "/srv/minecraft",
	}
	client := &readinessNodeClientFake{
		readFileErr: os.ErrNotExist,
	}

	errMissing := CheckStart(context.Background(), conn, gameServer, client)
	if errMissing == nil {
		t.Fatal("CheckStart() error = nil, want Minecraft EULA required")
	}
	if !strings.Contains(errMissing.Error(), "Minecraft EULA required") {
		t.Fatalf("CheckStart() error = %v, want Minecraft EULA required", errMissing)
	}
	if len(client.writeFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(client.writeFileCalls))
	}

	errAccept := AcceptMinecraftEULA(context.Background(), conn, gameServer, client, "user-admin")
	if errAccept != nil {
		t.Fatalf("AcceptMinecraftEULA() error = %v", errAccept)
	}
	if len(client.writeFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(client.writeFileCalls))
	}
	writeCall := client.writeFileCalls[0]
	if writeCall.relativePath != minecraftEULAFileName {
		t.Fatalf("WriteFile relative path = %q, want %q", writeCall.relativePath, minecraftEULAFileName)
	}
	if string(writeCall.content) != "eula=true\n" {
		t.Fatalf("WriteFile content = %q, want eula=true", string(writeCall.content))
	}

	errReady := CheckStart(context.Background(), conn, gameServer, client)
	if errReady != nil {
		t.Fatalf("CheckStart() after AcceptMinecraftEULA error = %v", errReady)
	}
}

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
	client := &readinessNodeClientFake{
		runtimeCapabilitiesResult: node.RuntimeCapabilities{LaunchEnv: true},
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

	client.runtimeCapabilitiesResult = node.RuntimeCapabilities{LaunchEnv: false}
	errNoLaunchEnv := CheckStart(context.Background(), conn, gameServer, client)
	if errNoLaunchEnv == nil {
		t.Fatal("CheckStart() error = nil, want launch env support error")
	}
	if !strings.Contains(errNoLaunchEnv.Error(), "launch-only Hytale credentials") {
		t.Fatalf("CheckStart() error = %v, want launch-only Hytale credentials", errNoLaunchEnv)
	}

	client.runtimeCapabilitiesResult = node.RuntimeCapabilities{LaunchEnv: true}
	errReady := CheckStart(context.Background(), conn, gameServer, client)
	if errReady != nil {
		t.Fatalf("CheckStart() after Hytale link error = %v", errReady)
	}
}

func TestCheckStartSunkenlandWorld(t *testing.T) {
	const validWorldFolder = "Xylona World~11223344-5566-7788-99AA-BBCCDDEEFF00"
	tests := []struct {
		name        string
		entries     []node.FileEntry
		listError   error
		wantError   bool
		wantMessage string
	}{
		{
			name:        "worlds directory missing",
			listError:   os.ErrNotExist,
			wantError:   true,
			wantMessage: "client-created world",
		},
		{
			name: "invalid folder name",
			entries: []node.FileEntry{
				{Name: "Not A Sunkenland World", IsDirectory: true},
			},
			wantError:   true,
			wantMessage: "client-created world",
		},
		{
			name: "multiple worlds",
			entries: []node.FileEntry{
				{Name: validWorldFolder, IsDirectory: true},
				{Name: "Second~00112233-4455-6677-8899-AABBCCDDEEFF", IsDirectory: true},
			},
			wantError:   true,
			wantMessage: "multiple valid world folders",
		},
		{
			name: "single imported world",
			entries: []node.FileEntry{
				{Name: validWorldFolder, IsDirectory: true},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameServer := &models.GameServer{
				ID:        "sunkenland-server",
				GameID:    "sunkenland",
				Directory: "C:/servers/sunkenland",
			}
			client := &readinessNodeClientFake{
				listFilesResult: test.entries,
				listFilesErr:    test.listError,
			}

			errStart := CheckStart(context.Background(), nil, gameServer, client)
			if test.wantError {
				if errStart == nil {
					t.Fatal("CheckStart() error = nil")
				}
				if !strings.Contains(errStart.Error(), test.wantMessage) {
					t.Errorf("CheckStart() error = %q, want %q", errStart, test.wantMessage)
				}
				return
			}
			if errStart != nil {
				t.Fatalf("CheckStart() error = %v", errStart)
			}
			if len(client.listFilesCalls) != 2 {
				t.Fatalf("ListFiles call count = %d, want 2", len(client.listFilesCalls))
			}
			if client.listFilesCalls[1].relativePath != "worlds/"+validWorldFolder {
				t.Errorf("world inspection path = %q", client.listFilesCalls[1].relativePath)
			}
		})
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

type readinessFileCall struct {
	directory    string
	relativePath string
	content      []byte
}

type readinessNodeClientFake struct {
	snapshotResult            *node.NodeSnapshot
	listFilesResult           []node.FileEntry
	listFilesErr              error
	listFilesCalls            []readinessFileCall
	readFileResult            []byte
	readFileErr               error
	readFileCalls             []readinessFileCall
	writeFileCalls            []readinessFileCall
	runtimeCapabilitiesResult node.RuntimeCapabilities
}

func (f *readinessNodeClientFake) GetNodeSnapshot(context.Context) (*node.NodeSnapshot, error) {
	return f.snapshotResult, nil
}

func (f *readinessNodeClientFake) ListFiles(_ context.Context, directory string, relativePath string) ([]node.FileEntry, error) {
	f.listFilesCalls = append(f.listFilesCalls, readinessFileCall{directory: directory, relativePath: relativePath})
	return f.listFilesResult, f.listFilesErr
}

func (f *readinessNodeClientFake) ReadFile(_ context.Context, directory string, relativePath string) ([]byte, error) {
	f.readFileCalls = append(f.readFileCalls, readinessFileCall{directory: directory, relativePath: relativePath})
	return f.readFileResult, f.readFileErr
}

func (f *readinessNodeClientFake) WriteFile(_ context.Context, directory string, relativePath string, content []byte, _ node.ProtectionPolicy) error {
	f.writeFileCalls = append(f.writeFileCalls, readinessFileCall{
		directory:    directory,
		relativePath: relativePath,
		content:      append([]byte(nil), content...),
	})
	return nil
}

func (f *readinessNodeClientFake) GetRuntimeCapabilities(context.Context) (node.RuntimeCapabilities, error) {
	return f.runtimeCapabilitiesResult, nil
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
