package rpc

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func configureRemoteBackupServer(t *testing.T, fixture *rbacRPCFixture, nodeID string) string {
	t.Helper()

	insertRemoteNodeForParityTests(t, fixture, nodeID)

	serverDir := "/home/clinton/xylona/server-local-1"
	backupRoot := "/home/clinton/xylona/backups"

	_, errUpdate := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		NodeID:          omit.From(nodeID),
		Directory:       omit.From(serverDir),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(backupRoot),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	return backupRoot
}

func TestGetGameServerBackupOverviewAllowsRemoteServerOperations(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	configureRemoteBackupServer(t, fixture, "node-remote")
	fixture.service.nodeRegistry = testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local"},
		&nodeclient.FakeNodeClient{
			NodeID:         "node-remote",
			SnapshotResult: &node.NodeSnapshot{OS: "linux"},
		},
	)

	request := connect.NewRequest(&xylona.GetGameServerBackupOverviewRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetGameServerBackupOverview(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetGameServerBackupOverview() error = %v", errGet)
	}

	overview := response.Msg.GetOverview()
	if overview == nil {
		t.Fatal("GetGameServerBackupOverview().Overview = nil")
	}
	if overview.GetLocalServer() {
		t.Fatal("GetGameServerBackupOverview().LocalServer = true, want false")
	}
	if !overview.GetOperationsAllowed() {
		t.Fatal("GetGameServerBackupOverview().OperationsAllowed = false, want true")
	}
	if overview.GetDisabledReason() != "" {
		t.Fatalf("GetGameServerBackupOverview().DisabledReason = %q, want empty", overview.GetDisabledReason())
	}
}

func TestDeleteGameServerBackupRemovesRemoteServerNodeArchive(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	backupRoot := configureRemoteBackupServer(t, fixture, "node-remote")
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:            "node-remote",
		DeleteFilesResult: []string{"remote-delete.zip"},
	}
	fixture.service.nodeRegistry = testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local"},
		remoteClient,
	)
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		fixture.service.nodeRegistry,
		nil,
		nil,
		versiontracker.ResolverConfig{},
	)

	archiveDir := backupRoot + "/server-local-1"
	archivePath := archiveDir + "/remote-delete.zip"

	backup, errCreate := fixture.conn.CreateGameServerBackup(db.CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-remote",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     backupRoot,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       8,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreate)
	}

	request := connect.NewRequest(&xylona.DeleteGameServerBackupRequest{
		GameServerId: "server-local-1",
		BackupId:     backup.ID,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errDelete := fixture.service.DeleteGameServerBackup(context.Background(), request)
	if errDelete != nil {
		t.Fatalf("DeleteGameServerBackup() error = %v", errDelete)
	}

	_, errGet := fixture.conn.GetGameServerBackupByID(backup.ID)
	if errGet == nil {
		t.Fatal("GetGameServerBackupByID() error = nil, want missing row")
	}
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Fatalf("GetGameServerBackupByID() error = %v, want missing row", errGet)
	}
	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	if remoteClient.DeleteFilesCalls[0].Directory != archiveDir {
		t.Fatalf("DeleteFiles directory = %q, want %q", remoteClient.DeleteFilesCalls[0].Directory, archiveDir)
	}
	if !slices.Equal(remoteClient.DeleteFilesCalls[0].Files, []string{"remote-delete.zip"}) {
		t.Fatalf("DeleteFiles files = %v, want %v", remoteClient.DeleteFilesCalls[0].Files, []string{"remote-delete.zip"})
	}
}

func TestDownloadGameServerBackupArchiveStreamsRemoteServerNodeArchive(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	backupRoot := configureRemoteBackupServer(t, fixture, "node-remote")
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:           "node-remote",
		StatFileResult:   node.NewFileEntry("remote-download.zip", int64(len("remote-zip-bytes")), false, time.Now()),
		StreamFileReader: io.NopCloser(bytes.NewReader([]byte("remote-zip-bytes"))),
	}
	fixture.service.nodeRegistry = testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local"},
		remoteClient,
	)

	archiveDir := backupRoot + "/server-local-1"
	archivePath := archiveDir + "/remote-download.zip"

	backup, errCreate := fixture.conn.CreateGameServerBackup(db.CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-remote",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     backupRoot,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       16,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 20, 4, 42, 55, 0, time.UTC),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreate)
	}

	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/backups/download/server-local-1/"+backup.ID,
		nil,
	)
	request = withBackupDownloadRouteParams(request, "server-local-1", backup.ID)
	addSessionCookieHeaderHTTP(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	responseRecorder := httptest.NewRecorder()
	fixture.service.DownloadGameServerBackupArchive(responseRecorder, request)

	response := responseRecorder.Result()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("DownloadGameServerBackupArchive() status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	contentDisposition := response.Header.Get("Content-Disposition")
	if contentDisposition != `attachment; filename="remote-download.zip"` {
		t.Fatalf("Content-Disposition = %q, want %q", contentDisposition, `attachment; filename="remote-download.zip"`)
	}
	if response.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("Content-Type = %q, want %q", response.Header.Get("Content-Type"), "application/zip")
	}
	if responseRecorder.Body.String() != "remote-zip-bytes" {
		t.Fatalf("response body = %q, want %q", responseRecorder.Body.String(), "remote-zip-bytes")
	}
	if len(remoteClient.StatFileCalls) != 1 {
		t.Fatalf("StatFile call count = %d, want 1", len(remoteClient.StatFileCalls))
	}
	if len(remoteClient.StreamFileCalls) != 1 {
		t.Fatalf("StreamFile call count = %d, want 1", len(remoteClient.StreamFileCalls))
	}
	if remoteClient.StreamFileCalls[0].Directory != archiveDir {
		t.Fatalf("StreamFile directory = %q, want %q", remoteClient.StreamFileCalls[0].Directory, archiveDir)
	}
	if remoteClient.StreamFileCalls[0].RelativePath != "remote-download.zip" {
		t.Fatalf("StreamFile relative path = %q, want %q", remoteClient.StreamFileCalls[0].RelativePath, "remote-download.zip")
	}
}

func TestUploadGameServerBackupArchiveWritesRemoteUploadWithoutControllerBackupRoot(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")

	serverDir := filepath.ToSlash(filepath.Join(t.TempDir(), "remote-server"))
	backupRoot := filepath.ToSlash(filepath.Join(t.TempDir(), "remote-backups"))
	_, errUpdate := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		Directory:       omit.From(serverDir),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(backupRoot),
		MaxBackups:      omit.From(int64(5)),
		NodeID:          omit.From("node-remote"),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-remote",
		SnapshotResult: &node.NodeSnapshot{OS: "linux"},
	}
	fixture.service.nodeRegistry = testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local"},
		remoteClient,
	)
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		fixture.service.nodeRegistry,
		nil,
		nil,
		versiontracker.ResolverConfig{},
	)

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	errWriteField := writer.WriteField("gameServerId", "server-local-1")
	if errWriteField != nil {
		t.Fatalf("WriteField(gameServerId) error = %v", errWriteField)
	}
	fileWriter, errCreateFormFile := writer.CreateFormFile("file", "Friday Night Save.zip")
	if errCreateFormFile != nil {
		t.Fatalf("CreateFormFile() error = %v", errCreateFormFile)
	}
	zipContents := buildTestZipBytes(t, map[string]string{
		"world.txt": "uploaded-state",
	})
	_, errWriteFile := fileWriter.Write(zipContents)
	if errWriteFile != nil {
		t.Fatalf("form file write error = %v", errWriteFile)
	}
	errCloseWriter := writer.Close()
	if errCloseWriter != nil {
		t.Fatalf("writer.Close() error = %v", errCloseWriter)
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/backups/upload", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	addSessionCookieHeaderHTTP(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	responseRecorder := httptest.NewRecorder()
	fixture.service.UploadGameServerBackupArchive(responseRecorder, request)

	response := responseRecorder.Result()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("UploadGameServerBackupArchive() status = %d, want %d", response.StatusCode, http.StatusCreated)
	}

	archiveDir := backupRoot + "/server-local-1"
	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
	if len(remoteClient.StreamWriteFileCalls) != 1 {
		t.Fatalf("StreamWriteFile call count = %d, want 1", len(remoteClient.StreamWriteFileCalls))
	}
	if remoteClient.StreamWriteFileCalls[0].Directory != archiveDir {
		t.Fatalf("StreamWriteFile directory = %q, want %q", remoteClient.StreamWriteFileCalls[0].Directory, archiveDir)
	}
	if remoteClient.StreamWriteFileCalls[0].RelativePath != "Friday-Night-Save.zip" {
		t.Fatalf(
			"StreamWriteFile relative path = %q, want %q",
			remoteClient.StreamWriteFileCalls[0].RelativePath,
			"Friday-Night-Save.zip",
		)
	}
	if _, errStat := os.Stat(archiveDir); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(remote backup archive dir) error = %v, want %v", errStat, os.ErrNotExist)
	}

	backups, errList := fixture.conn.ListGameServerBackupsByGameServerID("server-local-1")
	if errList != nil {
		t.Fatalf("ListGameServerBackupsByGameServerID() error = %v", errList)
	}
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want %d", len(backups), 1)
	}
	if backups[0].ArchivePath != archiveDir+"/Friday-Night-Save.zip" {
		t.Fatalf("ArchivePath = %q, want %q", backups[0].ArchivePath, archiveDir+"/Friday-Night-Save.zip")
	}
	if backups[0].NodeID != "node-remote" {
		t.Fatalf("NodeID = %q, want %q", backups[0].NodeID, "node-remote")
	}
}
