package rpc

import (
	"archive/zip"
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/go-chi/chi/v5"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

func TestGetBackupSettingsRequiresBackupPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.GetBackupSettingsRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

	_, errGet := fixture.service.GetBackupSettings(context.Background(), request)
	if errGet == nil {
		t.Fatal("GetBackupSettings(without backup permission) error = nil, want permission denied")
	}
	if connect.CodeOf(errGet) != connect.CodePermissionDenied {
		t.Fatalf("GetBackupSettings(without backup permission) code = %v, want %v", connect.CodeOf(errGet), connect.CodePermissionDenied)
	}
}

func TestGetBackupSettingsRedactsDirectoryForNonSuperuser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From("/srv/backups"),
		MaxBackups:      omit.From(int64(7)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.GetBackupSettingsRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetBackupSettings(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetBackupSettings(non-superuser) error = %v", errGet)
	}

	settings := response.Msg.GetSettings()
	if settings == nil {
		t.Fatal("GetBackupSettings(non-superuser) returned nil settings")
	}
	if !settings.GetBackupsEnabled() {
		t.Fatal("GetBackupSettings(non-superuser).BackupsEnabled = false, want true")
	}
	if settings.GetBackupDirectory() != "" {
		t.Fatalf("GetBackupSettings(non-superuser).BackupDirectory = %q, want empty", settings.GetBackupDirectory())
	}
	if settings.GetMaxBackups() != 7 {
		t.Fatalf("GetBackupSettings(non-superuser).MaxBackups = %d, want %d", settings.GetMaxBackups(), 7)
	}
}

func TestGetBackupSettingsDefaultsDirectoryForSuperuserWhenBlank(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	gameServer, errGetServer := fixture.conn.GetGameServerByID("server-local-1")
	if errGetServer != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGetServer)
	}

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		BackupsEnabled:  omit.From(false),
		BackupDirectory: omit.From(""),
		MaxBackups:      omit.From(int64(0)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.GetBackupSettingsRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	response, errGet := fixture.service.GetBackupSettings(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetBackupSettings(superuser) error = %v", errGet)
	}

	settings := response.Msg.GetSettings()
	if settings == nil {
		t.Fatal("GetBackupSettings(superuser) returned nil settings")
	}

	wantBackupDirectory, errDefaultBackupDirectory := defaultBackupDirectoryForServer(gameServer.Directory)
	if errDefaultBackupDirectory != nil {
		t.Fatalf("defaultBackupDirectoryForServer() error = %v", errDefaultBackupDirectory)
	}
	if settings.GetBackupDirectory() != wantBackupDirectory {
		t.Fatalf("GetBackupSettings(superuser).BackupDirectory = %q, want %q", settings.GetBackupDirectory(), wantBackupDirectory)
	}
	if settings.GetDefaultBackupDirectory() != wantBackupDirectory {
		t.Fatalf(
			"GetBackupSettings(superuser).DefaultBackupDirectory = %q, want %q",
			settings.GetDefaultBackupDirectory(),
			wantBackupDirectory,
		)
	}
}

func TestGetBackupSettingsDefaultsDirectoryUsesRemoteSlashPath(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		NodeID:          omit.From("node-remote"),
		Directory:       omit.From("/home/clinton/xylona/servers/alpha"),
		BackupsEnabled:  omit.From(false),
		BackupDirectory: omit.From(""),
		MaxBackups:      omit.From(int64(0)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.GetBackupSettingsRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	response, errGet := fixture.service.GetBackupSettings(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetBackupSettings(remote superuser) error = %v", errGet)
	}

	settings := response.Msg.GetSettings()
	if settings == nil {
		t.Fatal("GetBackupSettings(remote superuser) returned nil settings")
	}
	wantBackupDirectory := "/home/clinton/xylona/servers/backups"
	if settings.GetBackupDirectory() != wantBackupDirectory {
		t.Fatalf("BackupDirectory = %q, want %q", settings.GetBackupDirectory(), wantBackupDirectory)
	}
	if settings.GetDefaultBackupDirectory() != wantBackupDirectory {
		t.Fatalf("DefaultBackupDirectory = %q, want %q", settings.GetDefaultBackupDirectory(), wantBackupDirectory)
	}
}

func TestGetGameServerBackupOverviewReturnsDisabledState(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:             omit.From("server-local-1"),
		BackupsEnabled: omit.From(false),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

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
		t.Fatal("GetGameServerBackupOverview() returned nil overview")
	}
	if overview.GetEnabled() {
		t.Fatal("GetGameServerBackupOverview().Enabled = true, want false")
	}
	if overview.GetOperationsAllowed() {
		t.Fatal("GetGameServerBackupOverview().OperationsAllowed = true, want false")
	}
	if overview.GetDisabledReason() != backupDisabledReasonBackupsDisabled {
		t.Fatalf("GetGameServerBackupOverview().DisabledReason = %q, want %q", overview.GetDisabledReason(), backupDisabledReasonBackupsDisabled)
	}
	if overview.GetScheduledBackupCount() != 0 {
		t.Fatalf("GetGameServerBackupOverview().ScheduledBackupCount = %d, want %d", overview.GetScheduledBackupCount(), 0)
	}
	if overview.GetCanManageSettings() {
		t.Fatal("GetGameServerBackupOverview().CanManageSettings = true, want false")
	}
	if !overview.GetLocalServer() {
		t.Fatal("GetGameServerBackupOverview().LocalServer = false, want true")
	}
}

func TestCreateGameServerBackupRequiresBackupPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.CreateGameServerBackupRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

	_, errCreate := fixture.service.CreateGameServerBackup(context.Background(), request)
	if errCreate == nil {
		t.Fatal("CreateGameServerBackup() error = nil, want permission denied")
	}
	if connect.CodeOf(errCreate) != connect.CodePermissionDenied {
		t.Fatalf("CreateGameServerBackup() code = %v, want %v", connect.CodeOf(errCreate), connect.CodePermissionDenied)
	}
}

func TestCreateGameServerBackupRejectsUnsafeBackupDirectory(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	gameServer, errGetServer := fixture.conn.GetGameServerByID("server-local-1")
	if errGetServer != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGetServer)
	}

	unsafeBackupDirectory := filepath.Join(gameServer.Directory, "backups")
	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(unsafeBackupDirectory),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.CreateGameServerBackupRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errCreate := fixture.service.CreateGameServerBackup(context.Background(), request)
	if errCreate == nil {
		t.Fatal("CreateGameServerBackup() error = nil, want failed precondition")
	}
	if connect.CodeOf(errCreate) != connect.CodeFailedPrecondition {
		t.Fatalf("CreateGameServerBackup() code = %v, want %v", connect.CodeOf(errCreate), connect.CodeFailedPrecondition)
	}
	if errCreate.Error() != "failed_precondition: Backup directory is not valid for this server." {
		t.Fatalf("CreateGameServerBackup() error = %q, want invalid directory message", errCreate.Error())
	}
}

func TestCreateGameServerBackupUsesRequestedBackupName(t *testing.T) {
	fixture := newRBACRPCFixture(t)
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

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		Directory:       omit.From(serverDir),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(backupRoot),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}
	fixture.service.actionsInst = newSupervisorBackedActionsInstance(context.Background(), t, fixture.conn, supervisorInst)

	request := connect.NewRequest(&xylona.CreateGameServerBackupRequest{
		GameServerId: "server-local-1",
		BackupName:   "Friday Night Save",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errCreate := fixture.service.CreateGameServerBackup(context.Background(), request)
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreate)
	}
	if response.Msg.GetBackup() == nil {
		t.Fatal("CreateGameServerBackup().Backup = nil")
	}
	if filepath.Base(response.Msg.GetBackup().GetArchivePath()) != "Friday-Night-Save.zip" {
		t.Fatalf(
			"CreateGameServerBackup().Backup.ArchivePath base = %q, want %q",
			filepath.Base(response.Msg.GetBackup().GetArchivePath()),
			"Friday-Night-Save.zip",
		)
	}
}

func TestCreateGameServerBackupRejectsInvalidBackupName(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.CreateGameServerBackupRequest{
		GameServerId: "server-local-1",
		BackupName:   "!!!",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errCreate := fixture.service.CreateGameServerBackup(context.Background(), request)
	if errCreate == nil {
		t.Fatal("CreateGameServerBackup() error = nil, want invalid argument")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Fatalf("CreateGameServerBackup() code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestGetGameServerRedactsBackupDirectoryForNonSuperuser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}
	wireServiceEmbeddedNode(t, fixture, supervisorInst)

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From("/srv/backups"),
		MaxBackups:      omit.From(int64(7)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.GetGameServerRequest{
		Id: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetGameServer(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetGameServer() error = %v", errGet)
	}

	gameServer := response.Msg.GetGameServer()
	if gameServer == nil {
		t.Fatal("GetGameServer() returned nil game server")
	}
	if !gameServer.GetBackupsEnabled() {
		t.Fatal("GetGameServer().BackupsEnabled = false, want true")
	}
	if gameServer.GetBackupDirectory() != "" {
		t.Fatalf("GetGameServer().BackupDirectory = %q, want empty", gameServer.GetBackupDirectory())
	}
	if gameServer.GetMaxBackups() != 7 {
		t.Fatalf("GetGameServer().MaxBackups = %d, want %d", gameServer.GetMaxBackups(), 7)
	}
}

func TestDeleteGameServerBackupRejectsDifferentNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From("/srv/backups"),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	_, errNode := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)`,
		"node-remote", "Remote Node", "http://remotehost:9080", true,
	)
	if errNode != nil {
		t.Fatalf("insert node-remote error = %v", errNode)
	}

	backup, errCreate := fixture.conn.CreateGameServerBackup(db.CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-remote",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     "/srv/backups/server-local-1/cross-node.zip",
		ArchiveRoot:     "/srv/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       12,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
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
	if errDelete == nil {
		t.Fatal("DeleteGameServerBackup() error = nil, want failed precondition")
	}
	if connect.CodeOf(errDelete) != connect.CodeFailedPrecondition {
		t.Fatalf("DeleteGameServerBackup() code = %v, want %v", connect.CodeOf(errDelete), connect.CodeFailedPrecondition)
	}
}

func TestRestoreGameServerBackupAllowsHistoricalArchiveWithBlankCurrentBackupDirectory(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	serverDir := filepath.Join(t.TempDir(), "server-local-1")
	errMkdirServer := os.MkdirAll(serverDir, 0o750)
	if errMkdirServer != nil {
		t.Fatalf("MkdirAll(serverDir) error = %v", errMkdirServer)
	}

	errWrite := os.WriteFile(filepath.Join(serverDir, "state.txt"), []byte("before"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	archiveRoot := filepath.Join(t.TempDir(), "historical-backups")
	archiveDir := filepath.Join(archiveRoot, "server-local-1")
	errMkdirArchive := os.MkdirAll(archiveDir, 0o750)
	if errMkdirArchive != nil {
		t.Fatalf("MkdirAll(archiveDir) error = %v", errMkdirArchive)
	}

	archivePath := filepath.Join(archiveDir, "restore.zip")
	writeTestBackupZip(t, archivePath, map[string]string{
		"state.txt": "after",
	})

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		Directory:       omit.From(serverDir),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(""),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	backup, errCreateBackup := fixture.conn.CreateGameServerBackup(db.CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     archiveRoot,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       12,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC),
	})
	if errCreateBackup != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreateBackup)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}
	fixture.service.actionsInst = newSupervisorBackedActionsInstance(context.Background(), t, fixture.conn, supervisorInst)

	request := connect.NewRequest(&xylona.RestoreGameServerBackupRequest{
		GameServerId: "server-local-1",
		BackupId:     backup.ID,
		RestoreMode:  xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errRestore := fixture.service.RestoreGameServerBackup(context.Background(), request)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}

	restoredContents, errRead := os.ReadFile(filepath.Join(serverDir, "state.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile(state.txt) error = %v", errRead)
	}
	if string(restoredContents) != "after" {
		t.Fatalf("state.txt = %q, want %q", string(restoredContents), "after")
	}
}

func TestRestoreGameServerBackupHidesInternalRestoreErrors(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	serverDir := filepath.Join(t.TempDir(), "server-local-1")
	errMkdirServer := os.MkdirAll(serverDir, 0o750)
	if errMkdirServer != nil {
		t.Fatalf("MkdirAll(serverDir) error = %v", errMkdirServer)
	}

	archiveRoot := filepath.Join(t.TempDir(), "historical-backups")
	archivePath := filepath.Join(archiveRoot, "server-local-1", "missing.zip")
	errMkdirArchive := os.MkdirAll(filepath.Dir(archivePath), 0o750)
	if errMkdirArchive != nil {
		t.Fatalf("MkdirAll(archivePath dir) error = %v", errMkdirArchive)
	}

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		Directory:       omit.From(serverDir),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(archiveRoot),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	backup, errCreateBackup := fixture.conn.CreateGameServerBackup(db.CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     archiveRoot,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       12,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC),
	})
	if errCreateBackup != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreateBackup)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}
	fixture.service.actionsInst = newSupervisorBackedActionsInstance(context.Background(), t, fixture.conn, supervisorInst)

	request := connect.NewRequest(&xylona.RestoreGameServerBackupRequest{
		GameServerId: "server-local-1",
		BackupId:     backup.ID,
		RestoreMode:  xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errRestore := fixture.service.RestoreGameServerBackup(context.Background(), request)
	if errRestore == nil {
		t.Fatal("RestoreGameServerBackup() error = nil, want internal error")
	}
	if connect.CodeOf(errRestore) != connect.CodeInternal {
		t.Fatalf("RestoreGameServerBackup() code = %v, want %v", connect.CodeOf(errRestore), connect.CodeInternal)
	}
	if errRestore.Error() != "internal: failed to restore backup" {
		t.Fatalf("RestoreGameServerBackup() error = %q, want generic internal restore message", errRestore.Error())
	}
}

func TestUpdateBackupSettingsRejectsUnsafeBackupDirectory(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	gameServer, errGetServer := fixture.conn.GetGameServerByID("server-local-1")
	if errGetServer != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGetServer)
	}

	request := connect.NewRequest(&xylona.UpdateBackupSettingsRequest{
		GameServerId:    "server-local-1",
		BackupsEnabled:  true,
		BackupDirectory: filepath.Join(gameServer.Directory, "backups"),
		MaxBackups:      5,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errUpdate := fixture.service.UpdateBackupSettings(context.Background(), request)
	if errUpdate == nil {
		t.Fatal("UpdateBackupSettings() error = nil, want invalid argument")
	}
	if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateBackupSettings() code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
	}
	if errUpdate.Error() != "invalid_argument: Backup directory is not valid for this server." {
		t.Fatalf("UpdateBackupSettings() error = %q, want invalid directory message", errUpdate.Error())
	}
}

func TestUpdateBackupSettingsDefaultsBackupDirectoryWhenEnablingLegacyServer(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	gameServer, errGetServer := fixture.conn.GetGameServerByID("server-local-1")
	if errGetServer != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGetServer)
	}

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		BackupsEnabled:  omit.From(false),
		BackupDirectory: omit.From(""),
		MaxBackups:      omit.From(int64(0)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.UpdateBackupSettingsRequest{
		GameServerId:    "server-local-1",
		BackupsEnabled:  true,
		BackupDirectory: "",
		MaxBackups:      0,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	response, errUpdate := fixture.service.UpdateBackupSettings(context.Background(), request)
	if errUpdate != nil {
		t.Fatalf("UpdateBackupSettings() error = %v", errUpdate)
	}

	settings := response.Msg.GetSettings()
	if settings == nil {
		t.Fatal("UpdateBackupSettings() returned nil settings")
	}

	wantBackupDirectory, errDefaultBackupDirectory := defaultBackupDirectoryForServer(gameServer.Directory)
	if errDefaultBackupDirectory != nil {
		t.Fatalf("defaultBackupDirectoryForServer() error = %v", errDefaultBackupDirectory)
	}
	if !settings.GetBackupsEnabled() {
		t.Fatal("UpdateBackupSettings().Settings.BackupsEnabled = false, want true")
	}
	if settings.GetBackupDirectory() != wantBackupDirectory {
		t.Fatalf(
			"UpdateBackupSettings().Settings.BackupDirectory = %q, want %q",
			settings.GetBackupDirectory(),
			wantBackupDirectory,
		)
	}
	if settings.GetMaxBackups() != actions.DefaultScheduledBackupRetention {
		t.Fatalf(
			"UpdateBackupSettings().Settings.MaxBackups = %d, want %d",
			settings.GetMaxBackups(),
			actions.DefaultScheduledBackupRetention,
		)
	}

	updatedGameServer, errGetUpdatedServer := fixture.conn.GetGameServerByID("server-local-1")
	if errGetUpdatedServer != nil {
		t.Fatalf("GetGameServerByID(updated) error = %v", errGetUpdatedServer)
	}
	if !updatedGameServer.BackupsEnabled {
		t.Fatal("updated game server BackupsEnabled = false, want true")
	}
	if updatedGameServer.BackupDirectory != wantBackupDirectory {
		t.Fatalf("updated game server BackupDirectory = %q, want %q", updatedGameServer.BackupDirectory, wantBackupDirectory)
	}
	if updatedGameServer.MaxBackups != actions.DefaultScheduledBackupRetention {
		t.Fatalf(
			"updated game server MaxBackups = %d, want %d",
			updatedGameServer.MaxBackups,
			actions.DefaultScheduledBackupRetention,
		)
	}
}

func TestDownloadGameServerBackupArchiveStreamsCompletedArchive(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	archiveRoot := filepath.Join(t.TempDir(), "backups")
	archivePath := filepath.Join(archiveRoot, "server-local-1", "download.zip")
	writeTestBackupZip(t, archivePath, map[string]string{
		"world.txt": "seed-data",
	})

	backup, errCreateBackup := fixture.conn.CreateGameServerBackup(db.CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     archiveRoot,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       12,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC),
	})
	if errCreateBackup != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreateBackup)
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
	if contentDisposition != `attachment; filename="download.zip"` {
		t.Fatalf("Content-Disposition = %q, want %q", contentDisposition, `attachment; filename="download.zip"`)
	}
	if response.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("Content-Type = %q, want %q", response.Header.Get("Content-Type"), "application/zip")
	}
}

func TestDownloadGameServerBackupArchiveRejectsMissingPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	archiveRoot := filepath.Join(t.TempDir(), "backups")
	archivePath := filepath.Join(archiveRoot, "server-local-1", "download.zip")
	writeTestBackupZip(t, archivePath, map[string]string{
		"world.txt": "seed-data",
	})

	backup, errCreateBackup := fixture.conn.CreateGameServerBackup(db.CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     archiveRoot,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       12,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC),
	})
	if errCreateBackup != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreateBackup)
	}

	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/backups/download/server-local-1/"+backup.ID,
		nil,
	)
	request = withBackupDownloadRouteParams(request, "server-local-1", backup.ID)
	addSessionCookieHeaderHTTP(t, fixture.conn, fixture.secureCookie, request, "user-other")

	responseRecorder := httptest.NewRecorder()
	fixture.service.DownloadGameServerBackupArchive(responseRecorder, request)

	response := responseRecorder.Result()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("DownloadGameServerBackupArchive() status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

func TestUploadGameServerBackupArchiveImportsZip(t *testing.T) {
	fixture := newRBACRPCFixture(t)
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

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		Directory:       omit.From(serverDir),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(backupRoot),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}
	fixture.service.actionsInst = newSupervisorBackedActionsInstance(context.Background(), t, fixture.conn, supervisorInst)

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

	backups, errList := fixture.conn.ListGameServerBackupsByGameServerID("server-local-1")
	if errList != nil {
		t.Fatalf("ListGameServerBackupsByGameServerID() error = %v", errList)
	}
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want %d", len(backups), 1)
	}
	if filepath.Base(backups[0].ArchivePath) != "Friday-Night-Save.zip" {
		t.Fatalf("archive base = %q, want %q", filepath.Base(backups[0].ArchivePath), "Friday-Night-Save.zip")
	}
}

func TestUploadGameServerBackupArchiveRejectsInvalidZip(t *testing.T) {
	fixture := newRBACRPCFixture(t)
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

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		Directory:       omit.From(serverDir),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(backupRoot),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}
	fixture.service.actionsInst = newSupervisorBackedActionsInstance(context.Background(), t, fixture.conn, supervisorInst)

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	errWriteField := writer.WriteField("gameServerId", "server-local-1")
	if errWriteField != nil {
		t.Fatalf("WriteField(gameServerId) error = %v", errWriteField)
	}
	fileWriter, errCreateFormFile := writer.CreateFormFile("file", "invalid.zip")
	if errCreateFormFile != nil {
		t.Fatalf("CreateFormFile() error = %v", errCreateFormFile)
	}
	_, errWriteFile := fileWriter.Write([]byte("not-a-zip"))
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
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("UploadGameServerBackupArchive() status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
}

func TestUploadGameServerBackupArchiveWithMaxBytesRejectsOversizedMultipartBody(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)
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

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		Directory:       omit.From(serverDir),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(backupRoot),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}
	fixture.service.actionsInst = newSupervisorBackedActionsInstance(context.Background(), t, fixture.conn, supervisorInst)

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	errWriteField := writer.WriteField("gameServerId", "server-local-1")
	if errWriteField != nil {
		t.Fatalf("WriteField(gameServerId) error = %v", errWriteField)
	}
	fileWriter, errCreateFormFile := writer.CreateFormFile("file", "oversized.zip")
	if errCreateFormFile != nil {
		t.Fatalf("CreateFormFile() error = %v", errCreateFormFile)
	}
	_, errWriteFile := fileWriter.Write(bytes.Repeat([]byte("b"), 2048))
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
	fixture.service.uploadGameServerBackupArchiveWithMaxBytes(responseRecorder, request, 1024)

	response := responseRecorder.Result()
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("UploadGameServerBackupArchive() status = %d, want %d", response.StatusCode, http.StatusRequestEntityTooLarge)
	}

	backups, errList := fixture.conn.ListGameServerBackupsByGameServerID("server-local-1")
	if errList != nil {
		t.Fatalf("ListGameServerBackupsByGameServerID() error = %v", errList)
	}
	if len(backups) != 0 {
		t.Fatalf("backup count = %d, want %d", len(backups), 0)
	}
}

func writeTestBackupZip(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	errMkdirAll := os.MkdirAll(filepath.Dir(archivePath), 0o750)
	if errMkdirAll != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(archivePath), errMkdirAll)
	}

	archiveFile, errCreate := os.Create(archivePath)
	if errCreate != nil {
		t.Fatalf("Create(%s) error = %v", archivePath, errCreate)
	}

	zipWriter := zip.NewWriter(archiveFile)
	for name, contents := range files {
		fileWriter, errCreateEntry := zipWriter.Create(name)
		if errCreateEntry != nil {
			t.Fatalf("zip.Create(%s) error = %v", name, errCreateEntry)
		}

		_, errWriteEntry := fileWriter.Write([]byte(contents))
		if errWriteEntry != nil {
			t.Fatalf("zip write(%s) error = %v", name, errWriteEntry)
		}
	}

	errCloseZip := zipWriter.Close()
	if errCloseZip != nil {
		t.Fatalf("zipWriter.Close() error = %v", errCloseZip)
	}

	errCloseFile := archiveFile.Close()
	if errCloseFile != nil {
		t.Fatalf("Close(%s) error = %v", archivePath, errCloseFile)
	}
}

func withBackupDownloadRouteParams(request *http.Request, gameServerID string, backupID string) *http.Request {
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("gameServerId", gameServerID)
	routeContext.URLParams.Add("backupId", backupID)

	requestContext := context.WithValue(request.Context(), chi.RouteCtxKey, routeContext)
	return request.WithContext(requestContext)
}

func buildTestZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	for name, contents := range files {
		fileWriter, errCreateEntry := zipWriter.Create(name)
		if errCreateEntry != nil {
			t.Fatalf("zip.Create(%s) error = %v", name, errCreateEntry)
		}

		_, errWriteEntry := fileWriter.Write([]byte(contents))
		if errWriteEntry != nil {
			t.Fatalf("zip write(%s) error = %v", name, errWriteEntry)
		}
	}

	errCloseZip := zipWriter.Close()
	if errCloseZip != nil {
		t.Fatalf("zipWriter.Close() error = %v", errCloseZip)
	}

	return buffer.Bytes()
}
