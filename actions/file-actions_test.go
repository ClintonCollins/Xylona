package actions

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aarondl/opt/null"
	"github.com/rs/zerolog/log"

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

func TestSaveUploadedGameServerFileClosesTempFileOnce(t *testing.T) {
	fixture := newFileActionTestFixture(t)

	var logBuffer bytes.Buffer
	originalLogger := log.Logger
	log.Logger = log.Logger.Output(&logBuffer)
	t.Cleanup(func() {
		log.Logger = originalLogger
	})

	uploadContent := `payload-data`
	errSave := fixture.inst.saveUploadedGameServerFile(
		fixture.gameServer,
		`uploads`,
		`server.jar`,
		strings.NewReader(uploadContent),
	)
	if errSave != nil {
		t.Fatalf("saveUploadedGameServerFile() error = %v", errSave)
	}

	savedPath := filepath.Join(fixture.serverDir, "uploads", "server.jar")
	savedContent, errRead := os.ReadFile(savedPath)
	if errRead != nil {
		t.Fatalf("ReadFile(%q) error = %v", savedPath, errRead)
	}
	if string(savedContent) != uploadContent {
		t.Fatalf("saved content = %q, want %q", string(savedContent), uploadContent)
	}

	tempFiles, errGlob := filepath.Glob(filepath.Join(fixture.serverDir, "uploads", "server.jar.tmp-*"))
	if errGlob != nil {
		t.Fatalf("Glob(temp files) error = %v", errGlob)
	}
	if len(tempFiles) != 0 {
		t.Fatalf("temp files remain: %v", tempFiles)
	}

	if strings.Contains(logBuffer.String(), "Failed to close upload temp file") {
		t.Fatalf("log buffer contains false close error: %s", logBuffer.String())
	}
}

func TestDownloadFileFromURLRejectsLoopbackTarget(t *testing.T) {
	fixture := newFileActionTestFixture(t)

	_, errDownload := fixture.inst.DownloadFileFromURL(
		context.Background(),
		fixture.gameServer,
		"http://127.0.0.1:8080/file.txt",
		"",
	)
	if errDownload == nil {
		t.Fatal("DownloadFileFromURL() expected error, got nil")
	}
	if !strings.Contains(errDownload.Error(), "private or reserved") {
		t.Fatalf("DownloadFileFromURL() error = %v, want SSRF validation failure", errDownload)
	}
}

func TestValidateDownloadRedirectTargetRejectsPrivateRedirect(t *testing.T) {
	req, errRequest := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:8080/private.txt", nil)
	if errRequest != nil {
		t.Fatalf("NewRequest() error = %v", errRequest)
	}
	viaReq, errViaRequest := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://downloads.example.com/public.txt", nil)
	if errViaRequest != nil {
		t.Fatalf("NewRequest(via) error = %v", errViaRequest)
	}

	errValidate := validateDownloadRedirectTarget(req, []*http.Request{viaReq})
	if errValidate == nil {
		t.Fatal("validateDownloadRedirectTarget() expected error, got nil")
	}
	if !strings.Contains(errValidate.Error(), "private or reserved") {
		t.Fatalf("validateDownloadRedirectTarget() error = %v, want SSRF validation failure", errValidate)
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
