package adminipc

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

// These tests pin down the expected local-only admin IPC seam from
// docs/specs/2026-04-11-cli-03-user-management-commands.md. They assume the
// implementation introduces:
//   - a DB-path-derived local transport server/client in pkg/adminipc
//   - an app-level lock used by offline mode
//   - a GuardOfflineMutation helper used by the CLI before direct DB writes

func TestLocalAdminTransportRoundTripListUsers(t *testing.T) {
	t.Parallel()

	dbPath := prepareAdminIPCTestDBPath(t, `adminipc-roundtrip.sqlite`)
	startAdminIPCTestServer(t, dbPath, &testXylonaHandler{
		listUsersResponse: &xylona.ListUsersResponse{
			Users: []*xylona.User{
				{
					Id:        `user-admin`,
					UserName:  `admin`,
					Email:     `admin@example.com`,
					SuperUser: true,
				},
			},
		},
	})

	client, errNewClient := NewClient(dbPath)
	if errNewClient != nil {
		t.Fatalf(`NewClient() error = %v`, errNewClient)
	}

	response := waitForAdminIPCListUsers(t, client)
	if len(response.Msg.GetUsers()) != 1 {
		t.Fatalf(`ListUsers() len = %d, want %d`, len(response.Msg.GetUsers()), 1)
	}
	if response.Msg.GetUsers()[0].GetUserName() != `admin` {
		t.Fatalf(`ListUsers()[0].UserName = %q, want %q`, response.Msg.GetUsers()[0].GetUserName(), `admin`)
	}
}

func TestGuardOfflineMutationRejectsWhenLiveTransportReachable(t *testing.T) {
	t.Parallel()

	dbPath := prepareAdminIPCTestDBPath(t, `adminipc-live-transport.sqlite`)
	startAdminIPCTestServer(t, dbPath, &testXylonaHandler{
		listUsersResponse: &xylona.ListUsersResponse{
			Users: []*xylona.User{},
		},
	})

	lock, errGuard := GuardOfflineMutation(context.Background(), dbPath)
	if errGuard == nil {
		if lock != nil {
			errClose := lock.Close()
			if errClose != nil {
				t.Fatalf(`GuardOfflineMutation() cleanup error = %v`, errClose)
			}
		}
		t.Fatal(`GuardOfflineMutation() error = nil, want live-daemon rejection`)
	}
	if !errors.Is(errGuard, ErrLiveDaemonRunning) {
		t.Fatalf(`GuardOfflineMutation() error = %v, want %v`, errGuard, ErrLiveDaemonRunning)
	}
}

func TestGuardOfflineMutationRejectsWhenAppLockHeld(t *testing.T) {
	t.Parallel()

	dbPath := prepareAdminIPCTestDBPath(t, `adminipc-app-lock.sqlite`)
	lock, errAcquireLock := AcquireAppLock(dbPath)
	if errAcquireLock != nil {
		t.Fatalf(`AcquireAppLock() error = %v`, errAcquireLock)
	}
	t.Cleanup(func() {
		errClose := lock.Close()
		if errClose != nil {
			t.Errorf(`AcquireAppLock() cleanup error = %v`, errClose)
		}
	})

	offlineLock, errGuard := GuardOfflineMutation(context.Background(), dbPath)
	if errGuard == nil {
		if offlineLock != nil {
			errClose := offlineLock.Close()
			if errClose != nil {
				t.Fatalf(`GuardOfflineMutation() cleanup error = %v`, errClose)
			}
		}
		t.Fatal(`GuardOfflineMutation() error = nil, want app-lock rejection`)
	}
	if !errors.Is(errGuard, ErrAppLockHeld) {
		t.Fatalf(`GuardOfflineMutation() error = %v, want %v`, errGuard, ErrAppLockHeld)
	}
}

func TestGuardOfflineMutationAllowsAccessWhenIdle(t *testing.T) {
	t.Parallel()

	dbPath := prepareAdminIPCTestDBPath(t, `adminipc-offline-idle.sqlite`)

	lock, errGuard := GuardOfflineMutation(context.Background(), dbPath)
	if errGuard != nil {
		t.Fatalf(`GuardOfflineMutation() error = %v`, errGuard)
	}
	if lock == nil {
		t.Fatal(`GuardOfflineMutation() lock = nil, want acquired lock`)
	}

	errClose := lock.Close()
	if errClose != nil {
		t.Fatalf(`GuardOfflineMutation() cleanup error = %v`, errClose)
	}
}

func TestEndpointHashRespectsPlatformPathCaseSensitivity(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	lowerPath := filepath.Join(tempDir, `xylona.sqlite`)
	upperPath := filepath.Join(tempDir, `XYLONA.sqlite`)

	lowerHash := endpointHash(lowerPath)
	upperHash := endpointHash(upperPath)

	if runtime.GOOS == `windows` {
		if lowerHash != upperHash {
			t.Fatalf(`endpointHash() mismatch on Windows: %q != %q`, lowerHash, upperHash)
		}
		return
	}

	if lowerHash == upperHash {
		t.Fatalf(`endpointHash() collision on %s: %q == %q`, runtime.GOOS, lowerHash, upperHash)
	}
}

func TestResolveEndpointUsesDatabaseDirectoryOnUnix(t *testing.T) {
	if runtime.GOOS == `windows` {
		t.Skip(`unix-only socket path assertion`)
	}

	dbPath := prepareAdminIPCTestDBPath(t, `adminipc-endpoint-dir.sqlite`)
	endpoint, errResolve := resolveEndpoint(dbPath)
	if errResolve != nil {
		t.Fatalf(`resolveEndpoint() error = %v`, errResolve)
	}

	if filepath.Dir(endpoint) != filepath.Dir(dbPath) {
		t.Fatalf(`resolveEndpoint() dir = %q, want %q`, filepath.Dir(endpoint), filepath.Dir(dbPath))
	}
}

type testXylonaHandler struct {
	xylonaconnect.UnimplementedXylonaHandler

	listUsersResponse *xylona.ListUsersResponse
}

func (h *testXylonaHandler) ListUsers(_ context.Context, _ *connect.Request[xylona.ListUsersRequest]) (*connect.Response[xylona.ListUsersResponse], error) {
	return connect.NewResponse(h.listUsersResponse), nil
}

func prepareAdminIPCTestDBPath(t *testing.T, sqliteFileName string) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), sqliteFileName)
	conn, errNewConnection := db.NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf(`NewConnection() error = %v`, errNewConnection)
	}

	errClose := conn.SQLDb.Close()
	if errClose != nil {
		t.Fatalf(`Close() error = %v`, errClose)
	}

	return dbPath
}

func startAdminIPCTestServer(t *testing.T, dbPath string, handler xylonaconnect.XylonaHandler) {
	t.Helper()

	server, errNewServer := NewServer(ServerConfig{
		DBPath:  dbPath,
		Handler: handler,
	})
	if errNewServer != nil {
		t.Fatalf(`NewServer() error = %v`, errNewServer)
	}

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	t.Cleanup(func() {
		errClose := server.Close()
		if errClose != nil {
			t.Errorf(`Close() error = %v`, errClose)
		}

		select {
		case errServe := <-serveDone:
			if errServe != nil && !errors.Is(errServe, ErrServerClosed) {
				t.Errorf(`Serve() error = %v`, errServe)
			}
		case <-time.After(2 * time.Second):
			t.Errorf(`Serve() did not return after Close()`)
		}
	})

}

func waitForAdminIPCListUsers(t *testing.T, client xylonaconnect.XylonaClient) *connect.Response[xylona.ListUsersResponse] {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	var response *connect.Response[xylona.ListUsersResponse]
	var errListUsers error
	for time.Now().Before(deadline) {
		response, errListUsers = client.ListUsers(context.Background(), connect.NewRequest(&xylona.ListUsersRequest{}))
		if errListUsers == nil {
			return response
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf(`ListUsers() error = %v`, errListUsers)
	return nil
}
