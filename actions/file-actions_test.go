package actions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

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

func TestArchiveAndCompressFilesRejectsProtectedDestination(t *testing.T) {
	fixture := newFileActionTestFixture(t)

	errWrite := os.WriteFile(filepath.Join(fixture.serverDir, "world.txt"), []byte("seed-data"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(world.txt) error = %v", errWrite)
	}

	_, errCompress := fixture.inst.ArchiveAndCompressFiles(
		context.Background(),
		fixture.gameServer,
		"server.jar",
		[]string{"world.txt"},
		xylona.GameServerFilesCompressionType_ZIP,
	)
	if !errors.Is(errCompress, ErrProtectedPath) {
		t.Fatalf("ArchiveAndCompressFiles() error = %v, want %v", errCompress, ErrProtectedPath)
	}

	archivePath := filepath.Join(fixture.serverDir, "server.jar.zip")
	if _, errStat := os.Stat(archivePath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(%q) error = %v, want %v", archivePath, errStat, os.ErrNotExist)
	}
}

func TestArchiveAndCompressFilesRejectsTraversalSource(t *testing.T) {
	fixture := newFileActionTestFixture(t)

	outsidePath := filepath.Join(filepath.Dir(fixture.serverDir), "outside.txt")
	errWrite := os.WriteFile(outsidePath, []byte("secret"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(outside.txt) error = %v", errWrite)
	}

	_, errCompress := fixture.inst.ArchiveAndCompressFiles(
		context.Background(),
		fixture.gameServer,
		"archive",
		[]string{"../outside.txt"},
		xylona.GameServerFilesCompressionType_ZIP,
	)
	if !errors.Is(errCompress, ErrInvalidPath) {
		t.Fatalf("ArchiveAndCompressFiles() error = %v, want %v", errCompress, ErrInvalidPath)
	}
}

func TestArchiveFilesRejectsTraversalSource(t *testing.T) {
	fixture := newFileActionTestFixture(t)

	outsidePath := filepath.Join(filepath.Dir(fixture.serverDir), "outside.txt")
	errWrite := os.WriteFile(outsidePath, []byte("secret"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(outside.txt) error = %v", errWrite)
	}

	_, errArchive := fixture.inst.ArchiveFiles(
		context.Background(),
		fixture.gameServer,
		"archive",
		[]string{"../outside.txt"},
		xylona.GameServerFilesCompressionType_ZIP,
		make(chan *xylona.GameServerFilesArchiveProgress, 1),
	)
	if !errors.Is(errArchive, ErrInvalidPath) {
		t.Fatalf("ArchiveFiles() error = %v, want %v", errArchive, ErrInvalidPath)
	}
}

func TestExtractArchiveRejectsProtectedEntry(t *testing.T) {
	fixture := newFileActionTestFixture(t)

	archivePath := filepath.Join(fixture.serverDir, "import.zip")
	createTestZipArchive(t, archivePath, map[string]string{
		"server.jar": "blocked",
	})

	_, errExtract := fixture.inst.ExtractArchive(context.Background(), fixture.gameServer, "import.zip", "")
	if !errors.Is(errExtract, ErrProtectedPath) {
		t.Fatalf("ExtractArchive() error = %v, want %v", errExtract, ErrProtectedPath)
	}

	protectedFilePath := filepath.Join(fixture.serverDir, "server.jar")
	if _, errStat := os.Stat(protectedFilePath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(%q) error = %v, want %v", protectedFilePath, errStat, os.ErrNotExist)
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

	errPurge := inst.PurgeAllGameServerFiles(gameServer)
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

	errPurge := inst.PurgeAllGameServerFiles(gameServer)
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
