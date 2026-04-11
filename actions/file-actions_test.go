package actions

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aarondl/opt/null"
	"github.com/rs/zerolog/log"

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
