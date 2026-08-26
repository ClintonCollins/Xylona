package setupcmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/adminipc"
	"github.com/ClintonCollins/Xylona/internal/firstsetup"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

func TestLiveUserServiceCreatesFirstSuperUserWithOneAtomicCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handlerErr error
		wantErr    error
	}{
		{name: "creates first superuser"},
		{
			name:       "maps already installed",
			handlerErr: connect.NewError(connect.CodeFailedPrecondition, firstsetup.ErrAlreadyInstalled),
			wantErr:    firstsetup.ErrAlreadyInstalled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := &liveSetupTestHandler{completeError: tt.handlerErr}
			dbPath := filepath.Join(t.TempDir(), "setup-live.sqlite")
			startLiveSetupTestServer(t, dbPath, handler)

			client, errClient := adminipc.NewClient(dbPath)
			if errClient != nil {
				t.Fatalf("NewClient() error = %v", errClient)
			}
			service := &liveUserService{ctx: context.Background(), client: client}
			user, errCreate := service.CreateFirstSuperUser(usermgmt.CreateInput{
				UserName: "admin",
				Email:    "admin@localhost",
				Password: "secret-password",
			})
			if !errors.Is(errCreate, tt.wantErr) {
				t.Fatalf("CreateFirstSuperUser() error = %v, want %v", errCreate, tt.wantErr)
			}
			if tt.wantErr == nil && user.UserName != "admin" {
				t.Fatalf("CreateFirstSuperUser() username = %q, want admin", user.UserName)
			}
			if handler.completeCalls != 1 {
				t.Fatalf("CompleteSetup() calls = %d, want 1", handler.completeCalls)
			}
		})
	}
}

func TestOpenUserServicePreparesOfflineSecretsWhileHoldingLock(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	t.Setenv("COOKIE_HASH_KEY_BASE64", "")
	t.Setenv("COOKIE_BLOCK_KEY_BASE64", "")
	t.Setenv("ENCRYPTION_KEY_BASE64", "")
	workingDirectory := t.TempDir()
	dbPath := filepath.Join(workingDirectory, "new-state", "data.sqlite")
	getwdFunc = func() (string, error) { return workingDirectory, nil }
	executableDirFunc = func() (string, error) { return filepath.Join(workingDirectory, "xylona"), nil }

	originalEnsure := ensureSecretsFunc
	ensureCalled := false
	ensureSecretsFunc = func(input firstsetup.EnsureSecretsInput) (firstsetup.Secrets, error) {
		ensureCalled = true
		competingLock, errCompetingLock := adminipc.AcquireAppLock(input.DBPath)
		if errCompetingLock == nil {
			errClose := competingLock.Close()
			t.Fatalf("AcquireAppLock() acquired during secret persistence; cleanup error = %v", errClose)
		}
		return originalEnsure(input)
	}

	service, cleanup, errOpen := openUserService(context.Background(), Options{}, dbPath, true)
	if errOpen != nil {
		t.Fatalf("openUserService() error = %v", errOpen)
	}
	if service == nil {
		t.Fatal("openUserService() service = nil")
	}
	errCleanup := cleanup()
	if errCleanup != nil {
		t.Fatalf("openUserService() cleanup error = %v", errCleanup)
	}
	if !ensureCalled {
		t.Fatal("openUserService() did not ensure offline secrets")
	}
	envContent, errRead := os.ReadFile(filepath.Join(filepath.Dir(dbPath), ".env"))
	if errRead != nil {
		t.Fatalf("ReadFile() nested env error = %v", errRead)
	}
	if !strings.Contains(string(envContent), "ENCRYPTION_KEY_BASE64=") {
		t.Fatalf("nested env content = %q, want encryption key", string(envContent))
	}
}

func TestRunWithInputUsesLiveServiceBeforeLocalSecrets(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	commandStdout = io.Discard
	t.Setenv("COOKIE_HASH_KEY_BASE64", "")
	t.Setenv("COOKIE_BLOCK_KEY_BASE64", "")
	t.Setenv("ENCRYPTION_KEY_BASE64", "")
	dbPath := filepath.Join(t.TempDir(), "existing.sqlite")
	errWrite := os.WriteFile(dbPath, []byte("existing database"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile() database error = %v", errWrite)
	}
	handler := &liveSetupTestHandler{}
	startLiveSetupTestServer(t, dbPath, handler)
	ensureSecretsFunc = func(firstsetup.EnsureSecretsInput) (firstsetup.Secrets, error) {
		t.Fatal("ensureSecretsFunc called while live setup service was available")
		return firstsetup.Secrets{}, nil
	}

	errRun := RunWithInput(context.Background(), Options{DefaultDBPath: dbPath}, Input{
		UserName: "admin",
		Password: "secret-password",
	})
	if errRun != nil {
		t.Fatalf("RunWithInput() error = %v", errRun)
	}
	if handler.completeCalls != 1 {
		t.Fatalf("CompleteSetup() calls = %d, want 1", handler.completeCalls)
	}
}

type liveSetupTestHandler struct {
	xylonaconnect.UnimplementedXylonaHandler

	completeCalls int
	completeError error
}

func (h *liveSetupTestHandler) ListUsers(_ context.Context, _ *connect.Request[xylona.ListUsersRequest]) (*connect.Response[xylona.ListUsersResponse], error) {
	return connect.NewResponse(&xylona.ListUsersResponse{}), nil
}

func (h *liveSetupTestHandler) CompleteSetup(_ context.Context, request *connect.Request[xylona.CompleteSetupRequest]) (*connect.Response[xylona.CompleteSetupResponse], error) {
	h.completeCalls++
	if h.completeError != nil {
		return nil, h.completeError
	}
	return connect.NewResponse(&xylona.CompleteSetupResponse{User: &xylona.User{
		Id:        "user-1",
		UserName:  request.Msg.GetUserName(),
		Email:     request.Msg.GetEmail(),
		SuperUser: true,
		CreatedAt: timestamppb.Now(),
	}}), nil
}

func startLiveSetupTestServer(t *testing.T, dbPath string, handler xylonaconnect.XylonaHandler) {
	t.Helper()

	server, errServer := adminipc.NewServer(adminipc.ServerConfig{DBPath: dbPath, Handler: handler})
	if errServer != nil {
		t.Fatalf("NewServer() error = %v", errServer)
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()
	t.Cleanup(func() {
		errClose := server.Close()
		if errClose != nil {
			t.Errorf("Close() error = %v", errClose)
		}
		select {
		case errServe := <-serveDone:
			if errServe != nil && !errors.Is(errServe, adminipc.ErrServerClosed) {
				t.Errorf("Serve() error = %v", errServe)
			}
		case <-time.After(2 * time.Second):
			t.Error("Serve() did not return after Close()")
		}
	})
}
