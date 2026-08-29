package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type cancellationAwareDeleteClient struct {
	*nodeclient.FakeNodeClient
}

func (c *cancellationAwareDeleteClient) DeleteFiles(
	ctx context.Context,
	_ string,
	_ []string,
	_ node.ProtectionPolicy,
) ([]string, error) {
	<-ctx.Done()
	return nil, fmt.Errorf("delete files wait: %w", ctx.Err())
}

type fileActionTestFixture struct {
	inst       *Instance
	gameServer *models.GameServer
	serverDir  string
}

func newFileActionTestFixture(t *testing.T) fileActionTestFixture {
	t.Helper()

	serverDir := t.TempDir()
	gameServer := &models.GameServer{
		ID:               "server-files-hv03",
		Directory:        serverDir,
		ServerExecutable: null.From("server.jar"),
	}

	return fileActionTestFixture{
		inst: &Instance{
			ctx: context.Background(),
		},
		gameServer: gameServer,
		serverDir:  serverDir,
	}
}

func TestSaveUploadedGameServerFileWritesLocalUploadThroughNodeClient(t *testing.T) {
	fixture := newFileActionTestFixture(t)
	fixture.gameServer.NodeID = "node-local"
	localClient := &nodeclient.FakeNodeClient{NodeID: "node-local"}
	fixture.inst.nodeRegistry = noderegistry.New("node-local", localClient)

	uploadContent := `payload-data`
	errSave := fixture.inst.saveUploadedGameServerFile(
		fixture.gameServer,
		`uploads\active`,
		`server.jar`,
		strings.NewReader(uploadContent),
		false,
	)
	if errSave != nil {
		t.Fatalf("saveUploadedGameServerFile() error = %v", errSave)
	}

	if len(localClient.CreateFileOrDirectoryCalls) != 1 {
		t.Fatalf("CreateFileOrDirectory call count = %d, want 1", len(localClient.CreateFileOrDirectoryCalls))
	}
	if localClient.CreateFileOrDirectoryCalls[0].Directory != fixture.gameServer.Directory {
		t.Fatalf("CreateFileOrDirectory directory = %q, want %q", localClient.CreateFileOrDirectoryCalls[0].Directory, fixture.gameServer.Directory)
	}
	if localClient.CreateFileOrDirectoryCalls[0].RelativePath != "uploads/active" {
		t.Fatalf("CreateFileOrDirectory path = %q, want %q", localClient.CreateFileOrDirectoryCalls[0].RelativePath, "uploads/active")
	}

	if len(localClient.StreamWriteFileCalls) != 1 {
		t.Fatalf("StreamWriteFile call count = %d, want 1", len(localClient.StreamWriteFileCalls))
	}
	call := localClient.StreamWriteFileCalls[0]
	if call.Directory != fixture.gameServer.Directory {
		t.Fatalf("StreamWriteFile directory = %q, want %q", call.Directory, fixture.gameServer.Directory)
	}
	if call.RelativePath != "uploads/active/server.jar" {
		t.Fatalf("StreamWriteFile path = %q, want %q", call.RelativePath, "uploads/active/server.jar")
	}
	if string(call.Content) != uploadContent {
		t.Fatalf("StreamWriteFile content = %q, want %q", string(call.Content), uploadContent)
	}
	if len(localClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(localClient.WriteFileCalls))
	}
}

func TestSaveUploadedGameServerFileProtectedPathAccess(t *testing.T) {
	tests := []struct {
		name                string
		allowProtectedPaths bool
		wantProtectedError  bool
	}{
		{name: "regular user is blocked", wantProtectedError: true},
		{name: "superuser is allowed", allowProtectedPaths: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFileActionTestFixture(t)
			fixture.gameServer.NodeID = "node-local"
			localClient := &nodeclient.FakeNodeClient{NodeID: "node-local"}
			fixture.inst.nodeRegistry = noderegistry.New("node-local", localClient)

			errSave := fixture.inst.saveUploadedGameServerFile(
				fixture.gameServer,
				"",
				"server.jar",
				strings.NewReader("payload"),
				tt.allowProtectedPaths,
			)
			if tt.wantProtectedError {
				if !errors.Is(errSave, ErrProtectedPath) {
					t.Fatalf("saveUploadedGameServerFile() error = %v, want %v", errSave, ErrProtectedPath)
				}
				if len(localClient.StreamWriteFileCalls) != 0 {
					t.Fatalf("StreamWriteFile call count = %d, want 0", len(localClient.StreamWriteFileCalls))
				}
				return
			}

			if errSave != nil {
				t.Fatalf("saveUploadedGameServerFile() error = %v", errSave)
			}
			if len(localClient.StreamWriteFileCalls) != 1 {
				t.Fatalf("StreamWriteFile call count = %d, want 1", len(localClient.StreamWriteFileCalls))
			}
			if localClient.StreamWriteFileCalls[0].Policy.IsConfigured() {
				t.Fatal("StreamWriteFile protection policy is configured for superuser")
			}
		})
	}
}

func TestPurgeAllGameServerFilesRoutesRemoteDeletesThroughNodeClient(t *testing.T) {
	serverDir := t.TempDir()
	sentinelPath := filepath.Join(serverDir, "sentinel.txt")

	errWrite := os.WriteFile(sentinelPath, []byte("keep-local"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(sentinel.txt) error = %v", errWrite)
	}

	registry := noderegistry.New("node-local", nil)
	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:        "server-remote-files",
		NodeID:    "node-remote",
		Directory: serverDir,
	}

	errPurge := inst.PurgeAllGameServerFiles(t.Context(), gameServer)
	if errPurge != nil {
		t.Fatalf("PurgeAllGameServerFiles() error = %v", errPurge)
	}

	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFilesCalls len = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	deleteCall := remoteClient.DeleteFilesCalls[0]
	if deleteCall.Directory != serverDir {
		t.Fatalf("DeleteFilesCalls[0].Directory = %q, want %q", deleteCall.Directory, serverDir)
	}
	if len(deleteCall.Files) != 1 || deleteCall.Files[0] != "" {
		t.Fatalf("DeleteFilesCalls[0].Files = %v, want [\"\"]", deleteCall.Files)
	}

	_, errStat := os.Stat(sentinelPath)
	if errStat != nil {
		t.Fatalf("Stat(%q) error = %v, want file to remain on controller", sentinelPath, errStat)
	}
}

func TestPurgeAllGameServerFilesRoutesLocalDeletesThroughNodeClient(t *testing.T) {
	serverDir := t.TempDir()
	localClient := &nodeclient.FakeNodeClient{NodeID: "node-local"}
	inst := &Instance{
		ctx:                context.Background(),
		embeddedNodeClient: localClient,
	}
	gameServer := &models.GameServer{
		ID:        "server-local-files",
		NodeID:    "node-local",
		Directory: serverDir,
	}

	errPurge := inst.PurgeAllGameServerFiles(t.Context(), gameServer)
	if errPurge != nil {
		t.Fatalf("PurgeAllGameServerFiles() error = %v", errPurge)
	}

	if len(localClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFilesCalls len = %d, want 1", len(localClient.DeleteFilesCalls))
	}
	deleteCall := localClient.DeleteFilesCalls[0]
	if deleteCall.Directory != serverDir {
		t.Fatalf("DeleteFilesCalls[0].Directory = %q, want %q", deleteCall.Directory, serverDir)
	}
	if len(deleteCall.Files) != 1 || deleteCall.Files[0] != "" {
		t.Fatalf("DeleteFilesCalls[0].Files = %v, want [\"\"]", deleteCall.Files)
	}
}

func TestPurgeAllGameServerFilesPropagatesCallerCancellation(t *testing.T) {
	remoteClient := &cancellationAwareDeleteClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-remote"},
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)
	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:        "server-remote-files",
		NodeID:    "node-remote",
		Directory: "/srv/server-remote-files",
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	errPurge := inst.PurgeAllGameServerFiles(ctx, gameServer)
	if !errors.Is(errPurge, context.Canceled) {
		t.Fatalf("PurgeAllGameServerFiles() error = %v, want %v", errPurge, context.Canceled)
	}
}

func TestCleanupFailedInstallRoutesRemoteDeletesThroughNodeClient(t *testing.T) {
	serverDir := t.TempDir()
	sentinelPath := filepath.Join(serverDir, "sentinel.txt")

	errWrite := os.WriteFile(sentinelPath, []byte("keep-local"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(sentinel.txt) error = %v", errWrite)
	}

	registry := noderegistry.New("node-local", nil)
	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}

	errCleanup := inst.cleanupFailedInstall("", serverDir, "node-remote")
	if errCleanup != nil {
		t.Fatalf("cleanupFailedInstall() error = %v", errCleanup)
	}

	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFilesCalls len = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	deleteCall := remoteClient.DeleteFilesCalls[0]
	if deleteCall.Directory != serverDir {
		t.Fatalf("DeleteFilesCalls[0].Directory = %q, want %q", deleteCall.Directory, serverDir)
	}
	if len(deleteCall.Files) != 1 || deleteCall.Files[0] != "" {
		t.Fatalf("DeleteFilesCalls[0].Files = %v, want [\"\"]", deleteCall.Files)
	}

	_, errStat := os.Stat(sentinelPath)
	if errStat != nil {
		t.Fatalf("Stat(%q) error = %v, want file to remain on controller", sentinelPath, errStat)
	}
}

func TestGetGameServerFileRejectsSymlinkEscape(t *testing.T) {
	fixture := newFileActionTestFixture(t)
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	errWrite := os.WriteFile(outsideFile, []byte("classified"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile outside error = %v", errWrite)
	}

	linkPath := filepath.Join(fixture.serverDir, "escape-link")
	errLink := os.Symlink(outsideFile, linkPath)
	if errLink != nil {
		t.Skipf("symlinks unavailable: %v", errLink)
	}

	var dest strings.Builder
	errGet := fixture.inst.GetGameServerFile(fixture.gameServer, "escape-link", &dest, false, false)
	if !errors.Is(errGet, ErrInvalidPath) && !errors.Is(errGet, node.ErrInvalidPath) {
		t.Fatalf("GetGameServerFile(escape-link) error = %v, want invalid path", errGet)
	}
	if dest.Len() != 0 {
		t.Fatalf("GetGameServerFile leaked %d bytes through symlink", dest.Len())
	}

	_, errList := fixture.inst.ListGameServerFiles(fixture.gameServer, "escape-link")
	if errList == nil {
		t.Fatal("ListGameServerFiles(escape-link) error = nil, want invalid path")
	}
}

func TestSlugifyName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "spaces become hyphens", input: "My Server", want: "my-server"},
		{name: "collapses punctuation", input: "Hello_World!!", want: "hello-world"},
		{name: "trims leftover hyphens", input: "  --Cool--  ", want: "cool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slugifyName(tt.input)
			if got != tt.want {
				t.Fatalf("slugifyName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
