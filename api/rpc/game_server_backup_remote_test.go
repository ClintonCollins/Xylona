package rpc

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func configureRemoteBackupServer(t *testing.T, fixture *rbacRPCFixture, nodeID string) string {
	t.Helper()

	insertRemoteNodeForParityTests(t, fixture, nodeID)

	serverDir := filepath.Join(t.TempDir(), "server-local-1")
	backupRoot := filepath.Join(t.TempDir(), "backups")

	errMkdirServer := os.MkdirAll(serverDir, 0o750)
	if errMkdirServer != nil {
		t.Fatalf("MkdirAll(serverDir) error = %v", errMkdirServer)
	}
	errMkdirBackup := os.MkdirAll(backupRoot, 0o750)
	if errMkdirBackup != nil {
		t.Fatalf("MkdirAll(backupRoot) error = %v", errMkdirBackup)
	}

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
		&nodeclient.FakeNodeClient{NodeID: "node-remote"},
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

func TestDeleteGameServerBackupAllowsRemoteServerControllerArchive(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	backupRoot := configureRemoteBackupServer(t, fixture, "node-remote")
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		nil,
		nil,
		nil,
		versiontracker.ResolverConfig{},
	)

	archiveDir := filepath.Join(backupRoot, "server-local-1")
	errMkdirArchive := os.MkdirAll(archiveDir, 0o750)
	if errMkdirArchive != nil {
		t.Fatalf("MkdirAll(archiveDir) error = %v", errMkdirArchive)
	}
	archivePath := filepath.Join(archiveDir, "remote-delete.zip")
	errWrite := os.WriteFile(archivePath, []byte("zip-bytes"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(archivePath) error = %v", errWrite)
	}

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
	_, errStat := os.Stat(archivePath)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("os.Stat(archivePath) error = %v, want os.ErrNotExist", errStat)
	}
}
