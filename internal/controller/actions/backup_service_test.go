package actions

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestCreateManualBackupRejectsBackupDirectoryInsideServerTree(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	fixture.gameServer.BackupDirectory = filepath.Join(fixture.gameServer.Directory, "backups")

	_, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "")
	if !errors.Is(errCreate, errBackupDirectoryInsideServer) {
		t.Fatalf("CreateManualBackup() error = %v, want %v", errCreate, errBackupDirectoryInsideServer)
	}
}

func TestCreateManualBackupRejectsPerServerArchivePathInsideServerTree(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	collidingServerDirectory := filepath.Join(filepath.Dir(fixture.gameServer.Directory), fixture.gameServer.ID)
	errMkdir := os.MkdirAll(collidingServerDirectory, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(collidingServerDirectory) error = %v", errMkdir)
	}
	fixture.gameServer.Directory = collidingServerDirectory
	fixture.gameServer.BackupDirectory = filepath.Dir(collidingServerDirectory)

	_, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "")
	if !errors.Is(errCreate, errBackupDirectoryInsideServer) {
		t.Fatalf("CreateManualBackup() error = %v, want %v", errCreate, errBackupDirectoryInsideServer)
	}
}

func TestCreateManualBackupReturnsPendingBackupBeforeArchiveCompletes(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "world.txt"), []byte("seed-data"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(world.txt) error = %v", errWrite)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})

	installBlockingBackupCreateClient(t, inst, fixture.nodeID, started, release)

	done := make(chan *models.GameServerBackup, 1)
	errs := make(chan error, 1)
	go func() {
		backup, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "")
		if errCreate != nil {
			errs <- errCreate
			return
		}
		done <- backup
	}()

	var backup *models.GameServerBackup
	select {
	case errCreate := <-errs:
		t.Fatalf("CreateManualBackup() error = %v", errCreate)
	case backup = <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("CreateManualBackup() did not return before archive worker completed")
	}

	if backup.Status != "pending" {
		t.Fatalf("CreateManualBackup().Status = %q, want %q", backup.Status, "pending")
	}
	if backup.SizeBytes != 0 {
		t.Fatalf("CreateManualBackup().SizeBytes = %d, want %d", backup.SizeBytes, 0)
	}
	_, completedAtSet := backup.CompletedAt.Get()
	if completedAtSet {
		t.Fatal("CreateManualBackup().CompletedAt was set, want nil while pending")
	}

	select {
	case <-started:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("background archive worker did not start")
	}

	storedPending, errGet := inst.db.GetGameServerBackupByID(backup.ID)
	if errGet != nil {
		t.Fatalf("GetGameServerBackupByID(pending) error = %v", errGet)
	}
	if storedPending.Status != "pending" {
		t.Fatalf("stored pending backup status = %q, want %q", storedPending.Status, "pending")
	}

	close(release)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		storedCompleted, errStored := inst.db.GetGameServerBackupByID(backup.ID)
		if errStored != nil {
			t.Fatalf("GetGameServerBackupByID(completed) error = %v", errStored)
		}
		if storedCompleted.Status == "completed" {
			if storedCompleted.SizeBytes <= 0 {
				t.Fatalf("completed backup size = %d, want > 0", storedCompleted.SizeBytes)
			}
			_, finalCompletedAtSet := storedCompleted.CompletedAt.Get()
			if !finalCompletedAtSet {
				t.Fatal("completed backup missing CompletedAt timestamp")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("manual backup did not complete in background")
}

func TestCreateManualBackupNormalizesWhitespaceInBackupDirectory(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	fixture.gameServer.BackupDirectory = `  ` + fixture.backupRoot + `  `

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	backup, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "")
	if errCreate != nil {
		t.Fatalf("CreateManualBackup() error = %v", errCreate)
	}

	wantDirectory := filepath.Join(fixture.backupRoot, fixture.gameServer.ID)
	if filepath.Dir(backup.ArchivePath) != wantDirectory {
		t.Fatalf("CreateManualBackup().ArchivePath dir = %q, want %q", filepath.Dir(backup.ArchivePath), wantDirectory)
	}
	if strings.Contains(backup.ArchivePath, `  `) {
		t.Fatalf("CreateManualBackup().ArchivePath = %q, want trimmed backup path", backup.ArchivePath)
	}
	completedBackup := fixture.waitForBackupCompletion(t, backup.ID)
	if _, errStat := os.Stat(completedBackup.ArchivePath); errStat != nil {
		t.Fatalf("Stat(ArchivePath) error = %v", errStat)
	}
}

func TestCreateManualBackupUsesProvidedNameInArchivePath(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	backup := fixture.createCompletedManualBackup(t, "Friday Night / Save #1")

	archiveBaseName := filepath.Base(backup.ArchivePath)
	if archiveBaseName != "Friday-Night-Save-1.zip" {
		t.Fatalf("CreateManualBackup().ArchivePath base = %q, want %q", archiveBaseName, "Friday-Night-Save-1.zip")
	}
}

func TestPrepareBackupForRemoteServerIgnoresControllerFilesystemCollisions(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(&nodeclient.FakeNodeClient{NodeID: fixture.nodeID})
	inst.nodeRegistry = registry

	fixture.gameServer.BackupDirectory = filepath.Join(t.TempDir(), "remote-backups")
	collisionDir := filepath.Join(fixture.gameServer.BackupDirectory, fixture.gameServer.ID)
	errMkdir := os.MkdirAll(collisionDir, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(collisionDir) error = %v", errMkdir)
	}
	collisionPath := filepath.Join(collisionDir, "Friday-Night-Save.zip")
	errWrite := os.WriteFile(collisionPath, []byte("controller-local-file"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(collisionPath) error = %v", errWrite)
	}

	backup, errPrepare := inst.prepareBackup(fixture.gameServer, "manual", fixture.userID, true, "Friday Night Save")
	if errPrepare != nil {
		t.Fatalf("prepareBackup(remote) error = %v", errPrepare)
	}
	if backup.ArchivePath != collisionPath {
		t.Fatalf("prepareBackup(remote).ArchivePath = %q, want %q", backup.ArchivePath, collisionPath)
	}
}

func TestCreateManualBackupUsesUniqueArchivePathWhenTimestampCollides(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	fixedNow := time.Date(2026, 4, 6, 12, 34, 56, 123456789, time.UTC)
	previousBackupNowFunc := backupNowFunc
	backupNowFunc = func() time.Time {
		return fixedNow
	}
	t.Cleanup(func() {
		backupNowFunc = previousBackupNowFunc
	})

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	existingArchivePath, errArchivePath := buildBackupArchivePath(fixture.backupRoot, fixture.gameServer.ID, fixedNow, "manual", "")
	if errArchivePath != nil {
		t.Fatalf("buildBackupArchivePath() error = %v", errArchivePath)
	}
	createTestZipArchive(t, existingArchivePath, map[string]string{
		"existing.txt": "existing",
	})

	backup, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "")
	if errCreate != nil {
		t.Fatalf("CreateManualBackup() error = %v", errCreate)
	}
	if backup.ArchivePath == existingArchivePath {
		t.Fatalf("CreateManualBackup().ArchivePath = %q, want unique path distinct from existing archive", backup.ArchivePath)
	}
	if _, errStat := os.Stat(existingArchivePath); errStat != nil {
		t.Fatalf("Stat(existingArchivePath) error = %v", errStat)
	}
	completedBackup := fixture.waitForBackupCompletion(t, backup.ID)
	if _, errStat := os.Stat(completedBackup.ArchivePath); errStat != nil {
		t.Fatalf("Stat(new ArchivePath) error = %v", errStat)
	}
}

func TestProduceRemoteBackupArchiveLeavesArchiveOnOwningNode(t *testing.T) {
	t.Parallel()

	serverDirectory := "/home/clinton/xylona/clinton/media-minecraft"
	remoteArchivePath := "/home/clinton/xylona/clinton/backups/media-minecraft/manual.zip"

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                   "remote-node",
		CreateBackupArchiveBytes: int64(len("remote-archive")),
		ReadFileErr:              errors.New("remote backup should stay on node"),
	}
	registry := noderegistry.New("local-node", nil)
	registry.Register(remoteClient)
	inst := &Instance{nodeRegistry: registry}

	gameServer := &models.GameServer{
		ID:        "server-remote-backup",
		NodeID:    "remote-node",
		Directory: serverDirectory,
	}

	var progressBytes []int64
	sizeBytes, errProduce := inst.produceBackupArchive(context.Background(), gameServer, remoteArchivePath, func(sizeBytes int64) {
		progressBytes = append(progressBytes, sizeBytes)
	})
	if errProduce != nil {
		t.Fatalf("produceBackupArchive() error = %v", errProduce)
	}
	if sizeBytes != int64(len("remote-archive")) {
		t.Fatalf("produceBackupArchive() size = %d, want %d", sizeBytes, len("remote-archive"))
	}
	if !slices.Equal(progressBytes, []int64{int64(len("remote-archive"))}) {
		t.Fatalf("progress bytes = %v, want %v", progressBytes, []int64{int64(len("remote-archive"))})
	}

	if len(remoteClient.CreateBackupArchiveCalls) != 1 {
		t.Fatalf("CreateBackupArchive call count = %d, want 1", len(remoteClient.CreateBackupArchiveCalls))
	}
	if remoteClient.CreateBackupArchiveCalls[0].Directory != serverDirectory {
		t.Fatalf("CreateBackupArchive directory = %q, want %q", remoteClient.CreateBackupArchiveCalls[0].Directory, serverDirectory)
	}
	if remoteClient.CreateBackupArchiveCalls[0].DestinationArchivePath != remoteArchivePath {
		t.Fatalf(
			"CreateBackupArchive destination = %q, want %q",
			remoteClient.CreateBackupArchiveCalls[0].DestinationArchivePath,
			remoteArchivePath,
		)
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
	if len(remoteClient.DeleteFilesCalls) != 0 {
		t.Fatalf("DeleteFiles call count = %d, want 0", len(remoteClient.DeleteFilesCalls))
	}
}

func TestProduceLocalBackupArchiveUsesRegisteredNodeClient(t *testing.T) {
	t.Parallel()

	serverDirectory := filepath.Join(t.TempDir(), "server")
	archivePath := filepath.Join(t.TempDir(), "backups", "server-local", "manual.zip")

	localClient := &nodeclient.FakeNodeClient{
		NodeID:                   "local-node",
		CreateBackupArchiveBytes: 4096,
	}
	registry := noderegistry.New("local-node", localClient)
	inst := &Instance{nodeRegistry: registry}

	gameServer := &models.GameServer{
		ID:        "server-local",
		NodeID:    "local-node",
		Directory: serverDirectory,
	}

	var progressBytes []int64
	sizeBytes, errProduce := inst.produceBackupArchive(context.Background(), gameServer, archivePath, func(sizeBytes int64) {
		progressBytes = append(progressBytes, sizeBytes)
	})
	if errProduce != nil {
		t.Fatalf("produceBackupArchive() error = %v", errProduce)
	}
	if sizeBytes != 4096 {
		t.Fatalf("produceBackupArchive() size = %d, want %d", sizeBytes, 4096)
	}
	if !slices.Equal(progressBytes, []int64{4096}) {
		t.Fatalf("progress bytes = %v, want [4096]", progressBytes)
	}
	if len(localClient.CreateBackupArchiveCalls) != 1 {
		t.Fatalf("CreateBackupArchive call count = %d, want 1", len(localClient.CreateBackupArchiveCalls))
	}
	call := localClient.CreateBackupArchiveCalls[0]
	if call.Directory != serverDirectory {
		t.Fatalf("CreateBackupArchive directory = %q, want %q", call.Directory, serverDirectory)
	}
	if call.DestinationArchivePath != archivePath {
		t.Fatalf("CreateBackupArchive destination = %q, want %q", call.DestinationArchivePath, archivePath)
	}
}

func TestProduceLocalBackupArchiveUsesEmbeddedNodeClient(t *testing.T) {
	t.Parallel()

	serverDirectory := filepath.Join(t.TempDir(), "server")
	archivePath := filepath.Join(t.TempDir(), "backups", "server-local", "manual.zip")

	localClient := &nodeclient.FakeNodeClient{
		NodeID:                   "local-node",
		CreateBackupArchiveBytes: 2048,
	}
	inst := &Instance{embeddedNodeClient: localClient}

	gameServer := &models.GameServer{
		ID:        "server-local",
		NodeID:    "local-node",
		Directory: serverDirectory,
	}

	sizeBytes, errProduce := inst.produceBackupArchive(context.Background(), gameServer, archivePath, nil)
	if errProduce != nil {
		t.Fatalf("produceBackupArchive() error = %v", errProduce)
	}
	if sizeBytes != 2048 {
		t.Fatalf("produceBackupArchive() size = %d, want %d", sizeBytes, 2048)
	}
	if len(localClient.CreateBackupArchiveCalls) != 1 {
		t.Fatalf("CreateBackupArchive call count = %d, want 1", len(localClient.CreateBackupArchiveCalls))
	}
	call := localClient.CreateBackupArchiveCalls[0]
	if call.Directory != serverDirectory {
		t.Fatalf("CreateBackupArchive directory = %q, want %q", call.Directory, serverDirectory)
	}
	if call.DestinationArchivePath != archivePath {
		t.Fatalf("CreateBackupArchive destination = %q, want %q", call.DestinationArchivePath, archivePath)
	}
}

func TestExecuteBackupCreateCleansRemoteArchiveWhenRemoteCreateFails(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	createErr := errors.New("forced remote create failure")
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                 "node-remote",
		CreateBackupArchiveErr: createErr,
		DeleteFilesResult:      []string{"manual.zip"},
	}
	registry := noderegistry.New(fixture.nodeID, nil)
	registry.Register(remoteClient)
	inst.nodeRegistry = registry

	_, errInsertNode := inst.db.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-remote"),
		Name:      omit.From("Remote Backup Node"),
		ListenURL: omit.From("http://remotehost:9080"),
		Enabled:   omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode(node-remote) error = %v", errInsertNode)
	}

	fixture.gameServer.NodeID = "node-remote"
	fixture.gameServer.Directory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-server"))
	fixture.gameServer.BackupDirectory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-backups"))
	archiveDir := fixture.gameServer.BackupDirectory + "/" + fixture.gameServer.ID
	archivePath := archiveDir + "/manual.zip"
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          "node-remote",
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     fixture.gameServer.BackupDirectory,
		ArchiveFormat:   "zip",
		Status:          "pending",
		SizeBytes:       0,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 20, 4, 42, 55, 0, time.UTC),
	})

	_, errCreate := inst.executeBackupCreate(context.Background(), fixture.gameServer, backup)
	if !errors.Is(errCreate, createErr) {
		t.Fatalf("executeBackupCreate() error = %v, want %v", errCreate, createErr)
	}
	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	if remoteClient.DeleteFilesCalls[0].Directory != archiveDir {
		t.Fatalf("DeleteFiles directory = %q, want %q", remoteClient.DeleteFilesCalls[0].Directory, archiveDir)
	}
	if !slices.Equal(remoteClient.DeleteFilesCalls[0].Files, []string{"manual.zip"}) {
		t.Fatalf("DeleteFiles files = %v, want %v", remoteClient.DeleteFilesCalls[0].Files, []string{"manual.zip"})
	}
}

func TestBuildBackupArchivePathPreservesRemoteDirectorySlashStyle(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 4, 20, 4, 42, 55, 325410000, time.UTC)
	tests := []struct {
		name            string
		backupDirectory string
		want            string
	}{
		{
			name:            "linux remote directory from windows controller",
			backupDirectory: "/home/clinton/xylona/clinton/backups",
			want:            "/home/clinton/xylona/clinton/backups/media-minecraft/20260420T044255.325410000Z-manual.zip",
		},
		{
			name:            "linux remote directory with trailing slash",
			backupDirectory: "/home/clinton/xylona/clinton/backups/",
			want:            "/home/clinton/xylona/clinton/backups/media-minecraft/20260420T044255.325410000Z-manual.zip",
		},
		{
			name:            "windows remote directory",
			backupDirectory: `C:\xylona\backups`,
			want:            `C:\xylona\backups\media-minecraft\20260420T044255.325410000Z-manual.zip`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, errBuild := buildBackupArchivePath(tt.backupDirectory, "media-minecraft", fixedNow, "manual", "")
			if errBuild != nil {
				t.Fatalf("buildBackupArchivePath() error = %v", errBuild)
			}
			if got != tt.want {
				t.Fatalf("buildBackupArchivePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveValidatedRemoteBackupArchivePathPreservesWindowsNodePath(t *testing.T) {
	t.Parallel()

	gameServer := &models.GameServer{
		ID:              "server-backup",
		NodeID:          "node-windows",
		Directory:       `C:\xylona\servers\server-backup`,
		BackupDirectory: `C:\xylona\backups`,
	}
	backup := &models.GameServerBackup{
		GameServerID:  gameServer.ID,
		NodeID:        gameServer.NodeID,
		ArchivePath:   `C:\xylona\backups\server-backup\manual.zip`,
		ArchiveRoot:   `C:\xylona\backups`,
		ArchiveFormat: "zip",
		Status:        "completed",
	}

	got, errResolve := resolveValidatedRemoteBackupArchivePath(gameServer, backup)
	if errResolve != nil {
		t.Fatalf("resolveValidatedRemoteBackupArchivePath() error = %v", errResolve)
	}
	if got != backup.ArchivePath {
		t.Fatalf("resolveValidatedRemoteBackupArchivePath() = %q, want %q", got, backup.ArchivePath)
	}
}

func TestResolveValidRemoteGameServerBackupDirectoryRejectsWindowsPathInsideServerTree(t *testing.T) {
	t.Parallel()

	gameServer := &models.GameServer{
		ID:              "server-backup",
		Directory:       `C:\xylona\servers\server-backup`,
		BackupDirectory: `C:\xylona\servers\server-backup\backups`,
	}

	_, errResolve := resolveValidRemoteGameServerBackupDirectory(gameServer)
	if !errors.Is(errResolve, errBackupDirectoryInsideServer) {
		t.Fatalf("resolveValidRemoteGameServerBackupDirectory() error = %v, want %v", errResolve, errBackupDirectoryInsideServer)
	}
}

func TestCreateManualBackupRejectsInvalidProvidedName(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	_, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "!!!")
	if !errors.Is(errCreate, ErrInvalidManualBackupName) {
		t.Fatalf("CreateManualBackup() error = %v, want %v", errCreate, ErrInvalidManualBackupName)
	}
}

func TestCreateManualBackupRejectsDuplicateProvidedName(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	firstBackup := fixture.createCompletedManualBackup(t, "Friday Night Save")
	if filepath.Base(firstBackup.ArchivePath) != "Friday-Night-Save.zip" {
		t.Fatalf("CreateManualBackup(first).ArchivePath base = %q, want %q", filepath.Base(firstBackup.ArchivePath), "Friday-Night-Save.zip")
	}

	_, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "Friday Night Save")
	if !errors.Is(errCreate, ErrManualBackupNameAlreadyExists) {
		t.Fatalf("CreateManualBackup(duplicate) error = %v, want %v", errCreate, ErrManualBackupNameAlreadyExists)
	}
}

func TestCreateManualBackupRejectsDuplicateProvidedNameWhilePending(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})

	installBlockingBackupCreateClient(t, inst, fixture.nodeID, started, release)

	firstBackup, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "Friday Night Save")
	if errCreate != nil {
		t.Fatalf("CreateManualBackup(first) error = %v", errCreate)
	}
	if filepath.Base(firstBackup.ArchivePath) != "Friday-Night-Save.zip" {
		t.Fatalf("CreateManualBackup(first).ArchivePath base = %q, want %q", filepath.Base(firstBackup.ArchivePath), "Friday-Night-Save.zip")
	}

	select {
	case <-started:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("background archive worker did not start")
	}

	_, errCreate = inst.CreateManualBackup(fixture.gameServer, fixture.userID, "Friday Night Save")
	if !errors.Is(errCreate, ErrManualBackupNameAlreadyExists) {
		t.Fatalf("CreateManualBackup(duplicate pending) error = %v, want %v", errCreate, ErrManualBackupNameAlreadyExists)
	}

	close(release)
	fixture.waitForBackupCompletion(t, firstBackup.ID)
}

func TestResolveUniqueBackupArchivePathFailsAfterMaxAttempts(t *testing.T) {
	t.Parallel()

	backupRoot := t.TempDir()
	gameServerID := "server-local-1"
	fixedNow := time.Date(2026, 4, 6, 12, 34, 56, 123456789, time.UTC)

	basePath, errBasePath := buildBackupArchivePath(backupRoot, gameServerID, fixedNow, "manual", "")
	if errBasePath != nil {
		t.Fatalf("buildBackupArchivePath() error = %v", errBasePath)
	}
	baseDirectory := filepath.Dir(basePath)
	baseName := strings.TrimSuffix(filepath.Base(basePath), ".zip")
	errMkdir := os.MkdirAll(baseDirectory, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(baseDirectory) error = %v", errMkdir)
	}

	for suffix := range maxBackupArchiveResolveAttempts {
		fileName := baseName + ".zip"
		if suffix > 0 {
			fileName = fmt.Sprintf("%s-%d.zip", baseName, suffix)
		}

		candidatePath := filepath.Join(baseDirectory, fileName)
		errWrite := os.WriteFile(candidatePath, []byte("existing"), 0o600)
		if errWrite != nil {
			t.Fatalf("WriteFile(%s) error = %v", candidatePath, errWrite)
		}
	}

	_, errResolve := resolveUniqueBackupArchivePath(backupRoot, gameServerID, fixedNow, "manual", "")
	if errResolve == nil {
		t.Fatal("resolveUniqueBackupArchivePath() error = nil, want exhausted candidates error")
	}
	if !strings.Contains(errResolve.Error(), "exhausted backup archive path candidates") {
		t.Fatalf("resolveUniqueBackupArchivePath() error = %v, want exhausted candidates context", errResolve)
	}
}

func TestBackupRestoreUserFacingMessage(t *testing.T) {
	t.Parallel()

	message, ok := BackupRestoreUserFacingMessage(errBackupNotCompleted)
	if !ok {
		t.Fatal("BackupRestoreUserFacingMessage(errBackupNotCompleted) ok = false, want true")
	}
	if message != errBackupNotCompleted.Error() {
		t.Fatalf("BackupRestoreUserFacingMessage(errBackupNotCompleted) message = %q, want %q", message, errBackupNotCompleted.Error())
	}

	wrappedErr := fmt.Errorf("wrapped: %w", node.ErrRestoreDestinationSymlink)
	message, ok = BackupRestoreUserFacingMessage(wrappedErr)
	if !ok {
		t.Fatal("BackupRestoreUserFacingMessage(wrapped restore symlink error) ok = false, want true")
	}
	if message != node.ErrRestoreDestinationSymlink.Error() {
		t.Fatalf("BackupRestoreUserFacingMessage(wrapped restore symlink error) message = %q, want %q", message, node.ErrRestoreDestinationSymlink.Error())
	}

	message, ok = BackupRestoreUserFacingMessage(errors.New("internal only"))
	if ok {
		t.Fatalf("BackupRestoreUserFacingMessage(internal only) ok = true, want false (message %q)", message)
	}
	if message != "" {
		t.Fatalf("BackupRestoreUserFacingMessage(internal only) message = %q, want empty", message)
	}
}

func TestCreateManualBackupCreatesZipAndCatalogRow(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.MkdirAll(filepath.Join(fixture.gameServer.Directory, "config"), 0o750)
	if errWrite != nil {
		t.Fatalf("MkdirAll() error = %v", errWrite)
	}
	errWrite = os.WriteFile(
		filepath.Join(fixture.gameServer.Directory, "config", "server.properties"),
		[]byte("motd=Alpha Base\n"),
		0o600,
	)
	if errWrite != nil {
		t.Fatalf("WriteFile(server.properties) error = %v", errWrite)
	}
	errWrite = os.WriteFile(
		filepath.Join(fixture.gameServer.Directory, "world.txt"),
		[]byte("seed-data"),
		0o600,
	)
	if errWrite != nil {
		t.Fatalf("WriteFile(world.txt) error = %v", errWrite)
	}

	backup, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "")
	if errCreate != nil {
		t.Fatalf("CreateManualBackup() error = %v", errCreate)
	}

	if backup.TriggerSource != "manual" {
		t.Fatalf("CreateManualBackup().TriggerSource = %q, want %q", backup.TriggerSource, "manual")
	}
	if !backup.RetentionExempt {
		t.Fatal("CreateManualBackup().RetentionExempt = false, want true")
	}
	if backup.Status != "pending" {
		t.Fatalf("CreateManualBackup().Status = %q, want %q", backup.Status, "pending")
	}
	if backup.NodeID != fixture.nodeID {
		t.Fatalf("CreateManualBackup().NodeID = %q, want %q", backup.NodeID, fixture.nodeID)
	}
	if filepath.Dir(backup.ArchivePath) != filepath.Join(fixture.backupRoot, fixture.gameServer.ID) {
		t.Fatalf(
			"CreateManualBackup().ArchivePath dir = %q, want %q",
			filepath.Dir(backup.ArchivePath),
			filepath.Join(fixture.backupRoot, fixture.gameServer.ID),
		)
	}
	if !strings.HasSuffix(filepath.Base(backup.ArchivePath), "-manual.zip") {
		t.Fatalf("CreateManualBackup().ArchivePath base = %q, want suffix %q", filepath.Base(backup.ArchivePath), "-manual.zip")
	}
	if backup.SizeBytes != 0 {
		t.Fatalf("CreateManualBackup().SizeBytes = %d, want %d", backup.SizeBytes, 0)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stored, errGet := inst.db.GetGameServerBackupByID(backup.ID)
		if errGet != nil {
			t.Fatalf("GetGameServerBackupByID() error = %v", errGet)
		}
		if stored.Status != "completed" {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if runtime.GOOS != "windows" {
			archiveInfo, errStat := os.Stat(stored.ArchivePath)
			if errStat != nil {
				t.Fatalf("Stat(ArchivePath) error = %v", errStat)
			}
			if archiveInfo.Mode().Perm() != 0o600 {
				t.Fatalf("ArchivePath mode = %o, want %o", archiveInfo.Mode().Perm(), 0o600)
			}
		}

		archiveEntries := readBackupArchiveEntries(t, stored.ArchivePath)
		if archiveEntries["config/server.properties"] != "motd=Alpha Base\n" {
			t.Fatalf("archive entry config/server.properties = %q, want %q", archiveEntries["config/server.properties"], "motd=Alpha Base\n")
		}
		if archiveEntries["world.txt"] != "seed-data" {
			t.Fatalf("archive entry world.txt = %q, want %q", archiveEntries["world.txt"], "seed-data")
		}
		if stored.ArchivePath != backup.ArchivePath {
			t.Fatalf("stored.ArchivePath = %q, want %q", stored.ArchivePath, backup.ArchivePath)
		}
		if createdBy, ok := stored.CreatedBy.Get(); !ok || createdBy != fixture.userID {
			t.Fatalf("stored.CreatedBy = (%q, %v), want (%q, true)", createdBy, ok, fixture.userID)
		}
		if stored.SizeBytes == 0 {
			t.Fatal("completed backup SizeBytes = 0, want > 0")
		}
		return
	}

	t.Fatal("CreateManualBackup() did not complete within timeout")
}

func TestImportUploadedBackupCreatesCompletedCatalogRow(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	uploadedPath := filepath.Join(t.TempDir(), "Friday Night Save.zip")
	createTestZipArchive(t, uploadedPath, map[string]string{
		"world.txt": "uploaded-state",
	})

	backup, errImport := inst.ImportUploadedBackup(
		fixture.gameServer,
		fixture.userID,
		uploadedPath,
		"Friday Night Save.zip",
	)
	if errImport != nil {
		t.Fatalf("ImportUploadedBackup() error = %v", errImport)
	}

	if backup.Status != "completed" {
		t.Fatalf("ImportUploadedBackup().Status = %q, want %q", backup.Status, "completed")
	}
	if backup.TriggerSource != "manual" {
		t.Fatalf("ImportUploadedBackup().TriggerSource = %q, want %q", backup.TriggerSource, "manual")
	}
	if !backup.RetentionExempt {
		t.Fatal("ImportUploadedBackup().RetentionExempt = false, want true")
	}
	if filepath.Base(backup.ArchivePath) != "Friday-Night-Save.zip" {
		t.Fatalf("ImportUploadedBackup().ArchivePath base = %q, want %q", filepath.Base(backup.ArchivePath), "Friday-Night-Save.zip")
	}
	completedBackup := fixture.waitForBackupCompletion(t, backup.ID)
	if _, errStat := os.Stat(completedBackup.ArchivePath); errStat != nil {
		t.Fatalf("Stat(ArchivePath) error = %v", errStat)
	}
	if _, errStat := os.Stat(uploadedPath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(uploadedPath) error = %v, want %v", errStat, os.ErrNotExist)
	}
}

func TestImportUploadedBackupWritesRemoteArchiveToOwningNode(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	registry := noderegistry.New(fixture.nodeID, nil)
	registry.Register(remoteClient)
	inst.nodeRegistry = registry

	_, errInsertNode := inst.db.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-remote"),
		Name:      omit.From("Remote Backup Node"),
		ListenURL: omit.From("http://remotehost:9080"),
		Enabled:   omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode(node-remote) error = %v", errInsertNode)
	}

	fixture.gameServer.NodeID = "node-remote"
	fixture.gameServer.Directory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-server"))
	fixture.gameServer.BackupDirectory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-backups"))

	uploadedPath := filepath.Join(t.TempDir(), "Friday Night Save.zip")
	createTestZipArchive(t, uploadedPath, map[string]string{
		"world.txt": "uploaded-state",
	})
	uploadedBytes, errReadUploaded := os.ReadFile(uploadedPath)
	if errReadUploaded != nil {
		t.Fatalf("ReadFile(uploadedPath) error = %v", errReadUploaded)
	}

	backup, errImport := inst.ImportUploadedBackup(
		fixture.gameServer,
		fixture.userID,
		uploadedPath,
		"Friday Night Save.zip",
	)
	if errImport != nil {
		t.Fatalf("ImportUploadedBackup() error = %v", errImport)
	}

	archiveDir := fixture.gameServer.BackupDirectory + "/" + fixture.gameServer.ID
	archivePath := archiveDir + "/Friday-Night-Save.zip"
	if backup.ArchivePath != archivePath {
		t.Fatalf("ImportUploadedBackup().ArchivePath = %q, want %q", backup.ArchivePath, archivePath)
	}
	if backup.NodeID != "node-remote" {
		t.Fatalf("ImportUploadedBackup().NodeID = %q, want %q", backup.NodeID, "node-remote")
	}
	if len(remoteClient.CreateFileOrDirectoryCalls) != 1 {
		t.Fatalf("CreateFileOrDirectory call count = %d, want 1", len(remoteClient.CreateFileOrDirectoryCalls))
	}
	if remoteClient.CreateFileOrDirectoryCalls[0].Directory != fixture.gameServer.BackupDirectory {
		t.Fatalf(
			"CreateFileOrDirectory directory = %q, want %q",
			remoteClient.CreateFileOrDirectoryCalls[0].Directory,
			fixture.gameServer.BackupDirectory,
		)
	}
	if remoteClient.CreateFileOrDirectoryCalls[0].RelativePath != fixture.gameServer.ID {
		t.Fatalf(
			"CreateFileOrDirectory relative path = %q, want %q",
			remoteClient.CreateFileOrDirectoryCalls[0].RelativePath,
			fixture.gameServer.ID,
		)
	}
	if !remoteClient.CreateFileOrDirectoryCalls[0].IsDirectory {
		t.Fatal("CreateFileOrDirectory IsDirectory = false, want true")
	}
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
	if !bytes.Equal(remoteClient.StreamWriteFileCalls[0].Content, uploadedBytes) {
		t.Fatal("StreamWriteFile content did not match uploaded archive bytes")
	}
	if _, errStat := os.Stat(backup.ArchivePath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(remote archive path) error = %v, want %v", errStat, os.ErrNotExist)
	}
	if _, errStat := os.Stat(uploadedPath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(uploadedPath) error = %v, want %v", errStat, os.ErrNotExist)
	}
}

func TestImportUploadedBackupRejectsRemoteArchiveCollision(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		ListFilesResult: []node.FileEntry{
			{Name: "Friday-Night-Save.zip"},
		},
	}
	registry := noderegistry.New(fixture.nodeID, nil)
	registry.Register(remoteClient)
	inst.nodeRegistry = registry

	_, errInsertNode := inst.db.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-remote"),
		Name:      omit.From("Remote Backup Node"),
		ListenURL: omit.From("http://remotehost:9080"),
		Enabled:   omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode(node-remote) error = %v", errInsertNode)
	}

	fixture.gameServer.NodeID = "node-remote"
	fixture.gameServer.Directory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-server"))
	fixture.gameServer.BackupDirectory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-backups"))

	uploadedPath := filepath.Join(t.TempDir(), "Friday Night Save.zip")
	createTestZipArchive(t, uploadedPath, map[string]string{
		"world.txt": "uploaded-state",
	})

	_, errImport := inst.ImportUploadedBackup(
		fixture.gameServer,
		fixture.userID,
		uploadedPath,
		"Friday Night Save.zip",
	)
	if !errors.Is(errImport, ErrManualBackupNameAlreadyExists) {
		t.Fatalf("ImportUploadedBackup() error = %v, want %v", errImport, ErrManualBackupNameAlreadyExists)
	}
	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
}

func TestImportUploadedBackupCleansPartialRemoteArchiveWhenWriteFails(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	writeErr := errors.New("forced remote write failure")
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:             "node-remote",
		StreamWriteFileErr: writeErr,
		DeleteFilesResult:  []string{"Friday-Night-Save.zip"},
	}
	registry := noderegistry.New(fixture.nodeID, nil)
	registry.Register(remoteClient)
	inst.nodeRegistry = registry

	_, errInsertNode := inst.db.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-remote"),
		Name:      omit.From("Remote Backup Node"),
		ListenURL: omit.From("http://remotehost:9080"),
		Enabled:   omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode(node-remote) error = %v", errInsertNode)
	}

	fixture.gameServer.NodeID = "node-remote"
	fixture.gameServer.Directory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-server"))
	fixture.gameServer.BackupDirectory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-backups"))

	uploadedPath := filepath.Join(t.TempDir(), "Friday Night Save.zip")
	createTestZipArchive(t, uploadedPath, map[string]string{
		"world.txt": "uploaded-state",
	})

	_, errImport := inst.ImportUploadedBackup(
		fixture.gameServer,
		fixture.userID,
		uploadedPath,
		"Friday Night Save.zip",
	)
	if !errors.Is(errImport, writeErr) {
		t.Fatalf("ImportUploadedBackup() error = %v, want %v", errImport, writeErr)
	}
	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	archiveDir := fixture.gameServer.BackupDirectory + "/" + fixture.gameServer.ID
	if remoteClient.DeleteFilesCalls[0].Directory != archiveDir {
		t.Fatalf("DeleteFiles directory = %q, want %q", remoteClient.DeleteFilesCalls[0].Directory, archiveDir)
	}
	if !slices.Equal(remoteClient.DeleteFilesCalls[0].Files, []string{"Friday-Night-Save.zip"}) {
		t.Fatalf("DeleteFiles files = %v, want %v", remoteClient.DeleteFilesCalls[0].Files, []string{"Friday-Night-Save.zip"})
	}
}

func TestImportUploadedBackupFallsBackToTimestampNameWhenFilenameIsInvalid(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	uploadedPath := filepath.Join(t.TempDir(), "!!!.zip")
	createTestZipArchive(t, uploadedPath, map[string]string{
		"world.txt": "uploaded-state",
	})

	backup, errImport := inst.ImportUploadedBackup(
		fixture.gameServer,
		fixture.userID,
		uploadedPath,
		"!!!.zip",
	)
	if errImport != nil {
		t.Fatalf("ImportUploadedBackup() error = %v", errImport)
	}

	if !strings.HasSuffix(filepath.Base(backup.ArchivePath), "-manual.zip") {
		t.Fatalf("ImportUploadedBackup().ArchivePath base = %q, want timestamped manual suffix", filepath.Base(backup.ArchivePath))
	}
}

func TestImportUploadedBackupRejectsInvalidZip(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	uploadedPath := filepath.Join(t.TempDir(), "broken.zip")
	errWrite := os.WriteFile(uploadedPath, []byte("not-a-zip"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(broken.zip) error = %v", errWrite)
	}

	_, errImport := inst.ImportUploadedBackup(
		fixture.gameServer,
		fixture.userID,
		uploadedPath,
		"broken.zip",
	)
	if errImport == nil {
		t.Fatal("ImportUploadedBackup() error = nil, want invalid zip error")
	}
}

func TestCreateManualBackupCleansUpWhenFinalizationFails(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	previousUpdateBackupResult := updateGameServerBackupResult
	updateGameServerBackupResult = func(
		conn *db.Connection,
		backupID string,
		params db.UpdateGameServerBackupResultParams,
	) (*models.GameServerBackup, error) {
		if params.Status == "completed" {
			return nil, errors.New("forced finalize failure")
		}
		return previousUpdateBackupResult(conn, backupID, params)
	}
	t.Cleanup(func() {
		updateGameServerBackupResult = previousUpdateBackupResult
	})

	broadcaster := &recordingBackupProgressBroadcaster{}
	inst.SetBackupProgressBroadcaster(broadcaster)

	backup, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "")
	if errCreate != nil {
		t.Fatalf("CreateManualBackup() error = %v", errCreate)
	}
	if backup.Status != "pending" {
		t.Fatalf("CreateManualBackup().Status = %q, want %q", backup.Status, "pending")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		backups, errList := inst.db.ListGameServerBackupsByGameServerID(fixture.gameServer.ID)
		if errList != nil {
			t.Fatalf("ListGameServerBackupsByGameServerID() error = %v", errList)
		}
		if broadcaster.containsPhase(xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED) && len(backups) == 0 {
			archivePaths, errGlob := filepath.Glob(filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "*.zip"))
			if errGlob != nil {
				t.Fatalf("Glob() error = %v", errGlob)
			}
			if len(archivePaths) != 0 {
				t.Fatalf("remaining backup archives = %v, want none", archivePaths)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("backup progress missing failed cleanup phase: %v", broadcaster.events)
}

func TestCreateManualBackupRemovesArchiveWhenWriteAndReconciliationFail(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	fakeClient := &nodeclient.FakeNodeClient{
		NodeID: fixture.nodeID,
	}
	fakeClient.CreateBackupArchiveFunc = func(_ context.Context, _ string, _ []string, archivePath string) (int64, string, error) {
		errMkdir := os.MkdirAll(filepath.Dir(archivePath), 0o750)
		if errMkdir != nil {
			return 0, "", fmt.Errorf("mkdir partial archive dir: %w", errMkdir)
		}
		fakeClient.DeleteFilesResult = []string{remotePathBase(archivePath)}
		errWriteArchive := os.WriteFile(archivePath, []byte("partial"), 0o600)
		if errWriteArchive != nil {
			return 0, "", fmt.Errorf("write partial archive: %w", errWriteArchive)
		}
		return 0, "", errors.New("forced archive write failure")
	}
	inst.embeddedNodeClient = fakeClient

	previousUpdateBackupResult := updateGameServerBackupResult
	updateGameServerBackupResult = func(
		conn *db.Connection,
		backupID string,
		params db.UpdateGameServerBackupResultParams,
	) (*models.GameServerBackup, error) {
		if params.Status == "failed" {
			return nil, errors.New("forced failed reconciliation error")
		}
		return previousUpdateBackupResult(conn, backupID, params)
	}
	t.Cleanup(func() {
		updateGameServerBackupResult = previousUpdateBackupResult
	})

	backup, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "")
	if errCreate != nil {
		t.Fatalf("CreateManualBackup() error = %v", errCreate)
	}
	if backup.Status != "pending" {
		t.Fatalf("CreateManualBackup().Status = %q, want %q", backup.Status, "pending")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		backups, errList := inst.db.ListGameServerBackupsByGameServerID(fixture.gameServer.ID)
		if errList != nil {
			t.Fatalf("ListGameServerBackupsByGameServerID() error = %v", errList)
		}
		if len(backups) == 0 {
			if len(fakeClient.DeleteFilesCalls) != 1 {
				t.Fatalf("DeleteFiles call count = %d, want 1", len(fakeClient.DeleteFilesCalls))
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("background create failure did not clean up backup row")
}

func TestCreateScheduledBackupBroadcastsCoherentProgressSequence(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	fixture.gameServer.MaxBackups = 1

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	oldScheduledArchivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "scheduled-old.zip")
	createTestZipArchive(t, oldScheduledArchivePath, map[string]string{
		"old.txt": "old",
	})
	fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       "",
		TriggerSource:   "scheduled",
		ArchivePath:     oldScheduledArchivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       13,
		RetentionExempt: false,
		CreatedAt:       time.Date(2026, 4, 1, 7, 0, 0, 0, time.UTC),
	})

	broadcaster := &recordingBackupProgressBroadcaster{}
	inst.SetBackupProgressBroadcaster(broadcaster)

	_, errCreate := inst.CreateScheduledBackup(fixture.gameServer)
	if errCreate != nil {
		t.Fatalf("CreateScheduledBackup() error = %v", errCreate)
	}

	gotPhases := broadcaster.phases()
	gotPhases = slices.Compact(gotPhases)
	wantPhases := []xylona.BackupProgressPhase{
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_PREPARING,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_ARCHIVING,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_PRUNING,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_COMPLETE,
	}
	if !slices.Equal(gotPhases, wantPhases) {
		t.Fatalf("backup progress phases = %v, want %v", gotPhases, wantPhases)
	}
	if broadcaster.containsPhase(xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED) {
		t.Fatalf("backup progress unexpectedly contained failed phase: %v", broadcaster.events)
	}
	if !broadcaster.allServerNamesMatch(fixture.gameServer.Name) {
		t.Fatalf("backup progress server names = %v, want all %q", broadcaster.serverNames(), fixture.gameServer.Name)
	}
}

func TestCreateScheduledBackupCoalescesArchiveProgressUpdates(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	broadcaster := &recordingBackupProgressBroadcaster{}
	inst.SetBackupProgressBroadcaster(broadcaster)

	inst.embeddedNodeClient = &nodeclient.FakeNodeClient{
		NodeID:                   fixture.nodeID,
		CreateBackupArchiveBytes: 4096 * 1024,
	}

	backup, errCreate := inst.CreateScheduledBackup(fixture.gameServer)
	if errCreate != nil {
		t.Fatalf("CreateScheduledBackup() error = %v", errCreate)
	}
	if backup.SizeBytes != 4096*1024 {
		t.Fatalf("CreateScheduledBackup().SizeBytes = %d, want %d", backup.SizeBytes, 4096*1024)
	}

	gotArchivingEvents := broadcaster.phaseCount(xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_ARCHIVING)
	if gotArchivingEvents > 3 {
		t.Fatalf("archiving progress events = %d, want at most %d", gotArchivingEvents, 3)
	}
	if len(broadcaster.events) > 6 {
		t.Fatalf("backup progress event count = %d, want at most %d", len(broadcaster.events), 6)
	}
}

func TestCreateScheduledBackupPrunesOnlyScheduledArtifacts(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	fixture.gameServer.MaxBackups = 1

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	_, errInsertNode := inst.db.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-remote"),
		Name:      omit.From("Remote Backup Node"),
		ListenURL: omit.From("http://remotehost:9080"),
		Enabled:   omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode(node-remote) error = %v", errInsertNode)
	}

	otherNodeArchivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "scheduled-other-node.zip")
	createTestZipArchive(t, otherNodeArchivePath, map[string]string{
		"other-node.txt": "other-node",
	})
	otherNodeBackup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          "node-remote",
		CreatedBy:       "",
		TriggerSource:   "scheduled",
		ArchivePath:     otherNodeArchivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       9,
		RetentionExempt: false,
		CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	})

	manualArchivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "manual-keep.zip")
	createTestZipArchive(t, manualArchivePath, map[string]string{
		"manual.txt": "manual",
	})
	manualBackup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     manualArchivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       10,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	})

	exemptArchivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "scheduled-exempt.zip")
	createTestZipArchive(t, exemptArchivePath, map[string]string{
		"exempt.txt": "exempt",
	})
	exemptBackup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       "",
		TriggerSource:   "scheduled",
		ArchivePath:     exemptArchivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       11,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
	})

	pendingArchivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "scheduled-pending.zip")
	createTestZipArchive(t, pendingArchivePath, map[string]string{
		"pending.txt": "pending",
	})
	pendingBackup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       "",
		TriggerSource:   "scheduled",
		ArchivePath:     pendingArchivePath,
		ArchiveFormat:   "zip",
		Status:          "pending",
		SizeBytes:       12,
		RetentionExempt: false,
		CreatedAt:       time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
	})

	oldScheduledArchivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "scheduled-old.zip")
	createTestZipArchive(t, oldScheduledArchivePath, map[string]string{
		"old.txt": "old",
	})
	oldScheduledBackup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       "",
		TriggerSource:   "scheduled",
		ArchivePath:     oldScheduledArchivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       13,
		RetentionExempt: false,
		CreatedAt:       time.Date(2026, 4, 1, 7, 0, 0, 0, time.UTC),
	})

	newBackup, errCreate := inst.CreateScheduledBackup(fixture.gameServer)
	if errCreate != nil {
		t.Fatalf("CreateScheduledBackup() error = %v", errCreate)
	}

	if newBackup.TriggerSource != "scheduled" {
		t.Fatalf("CreateScheduledBackup().TriggerSource = %q, want %q", newBackup.TriggerSource, "scheduled")
	}
	if newBackup.RetentionExempt {
		t.Fatal("CreateScheduledBackup().RetentionExempt = true, want false")
	}

	_, errGet := inst.db.GetGameServerBackupByID(oldScheduledBackup.ID)
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Fatalf("GetGameServerBackupByID(oldScheduled) error = %v, want %v", errGet, sql.ErrNoRows)
	}
	if _, errStat := os.Stat(oldScheduledArchivePath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(oldScheduledArchivePath) error = %v, want %v", errStat, os.ErrNotExist)
	}

	for _, backupID := range []string{manualBackup.ID, exemptBackup.ID, pendingBackup.ID, otherNodeBackup.ID, newBackup.ID} {
		_, errGet = inst.db.GetGameServerBackupByID(backupID)
		if errGet != nil {
			t.Fatalf("GetGameServerBackupByID(%q) error = %v", backupID, errGet)
		}
	}
	for _, archivePath := range []string{manualArchivePath, exemptArchivePath, pendingArchivePath, otherNodeArchivePath, newBackup.ArchivePath} {
		if _, errStat := os.Stat(archivePath); errStat != nil {
			t.Fatalf("Stat(%q) error = %v", archivePath, errStat)
		}
	}
}

func TestCreateScheduledBackupLeavesRowWhenPruneRowDeleteFails(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	fixture.gameServer.MaxBackups = 1

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	oldScheduledArchivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "scheduled-old.zip")
	createTestZipArchive(t, oldScheduledArchivePath, map[string]string{
		"old.txt": "old",
	})
	oldScheduledBackup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       "",
		TriggerSource:   "scheduled",
		ArchivePath:     oldScheduledArchivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       13,
		RetentionExempt: false,
		CreatedAt:       time.Date(2026, 4, 1, 7, 0, 0, 0, time.UTC),
	})

	previousDeleteGameServerBackupRow := deleteGameServerBackupRow
	deleteGameServerBackupRow = func(conn *db.Connection, backupID string) error {
		if backupID == oldScheduledBackup.ID {
			return errors.New("forced prune row delete failure")
		}
		return previousDeleteGameServerBackupRow(conn, backupID)
	}
	t.Cleanup(func() {
		deleteGameServerBackupRow = previousDeleteGameServerBackupRow
	})

	_, errCreate := inst.CreateScheduledBackup(fixture.gameServer)
	if errCreate == nil {
		t.Fatal("CreateScheduledBackup() error = nil, want prune delete failure")
	}
	if !strings.Contains(errCreate.Error(), "delete pruned backup") {
		t.Fatalf("CreateScheduledBackup() error = %v, want prune delete context", errCreate)
	}

	storedBackup, errGet := inst.db.GetGameServerBackupByID(oldScheduledBackup.ID)
	if errGet != nil {
		t.Fatalf("GetGameServerBackupByID() error = %v", errGet)
	}
	if storedBackup.ArchivePath != oldScheduledArchivePath {
		t.Fatalf("storedBackup.ArchivePath = %q, want %q", storedBackup.ArchivePath, oldScheduledArchivePath)
	}
	if _, errStat := os.Stat(oldScheduledArchivePath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(oldScheduledArchivePath) error = %v, want %v", errStat, os.ErrNotExist)
	}
}

func TestDeleteGameServerBackupRejectsBackupFromDifferentNode(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "delete-other-node.zip")
	createTestZipArchive(t, archivePath, map[string]string{
		"state.txt": "current-state",
	})
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       14,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	})
	fixture.gameServer.NodeID = "node-remote"

	errDelete := inst.DeleteGameServerBackup(fixture.gameServer, backup)
	if !errors.Is(errDelete, errBackupNodeMismatch) {
		t.Fatalf("DeleteGameServerBackup() error = %v, want %v", errDelete, errBackupNodeMismatch)
	}

	_, errGet := inst.db.GetGameServerBackupByID(backup.ID)
	if errGet != nil {
		t.Fatalf("GetGameServerBackupByID() error = %v", errGet)
	}
	if _, errStat := os.Stat(archivePath); errStat != nil {
		t.Fatalf("Stat(archivePath) error = %v", errStat)
	}
}

func TestDeleteGameServerBackupKeepsRowWhenRemoteNodeDoesNotConfirmArchiveRemoval(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	registry := noderegistry.New(fixture.nodeID, nil)
	registry.Register(remoteClient)
	inst.nodeRegistry = registry

	_, errInsertNode := inst.db.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-remote"),
		Name:      omit.From("Remote Backup Node"),
		ListenURL: omit.From("http://remotehost:9080"),
		Enabled:   omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode(node-remote) error = %v", errInsertNode)
	}

	fixture.gameServer.NodeID = "node-remote"
	fixture.gameServer.Directory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-server"))
	fixture.gameServer.BackupDirectory = filepath.ToSlash(filepath.Join(t.TempDir(), "remote-backups"))

	archiveDir := fixture.gameServer.BackupDirectory + "/" + fixture.gameServer.ID
	archivePath := archiveDir + "/delete.zip"
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          "node-remote",
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     fixture.gameServer.BackupDirectory,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       14,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	})

	errDelete := inst.DeleteGameServerBackup(fixture.gameServer, backup)
	if errDelete == nil {
		t.Fatal("DeleteGameServerBackup() error = nil, want remote delete confirmation failure")
	}
	if !strings.Contains(errDelete.Error(), "did not confirm") {
		t.Fatalf("DeleteGameServerBackup() error = %v, want delete confirmation context", errDelete)
	}
	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	if remoteClient.DeleteFilesCalls[0].Directory != archiveDir {
		t.Fatalf("DeleteFiles directory = %q, want %q", remoteClient.DeleteFilesCalls[0].Directory, archiveDir)
	}
	_, errGet := inst.db.GetGameServerBackupByID(backup.ID)
	if errGet != nil {
		t.Fatalf("GetGameServerBackupByID() error = %v", errGet)
	}
}

func TestDeleteGameServerBackupCancelsPendingBackup(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	broadcaster := &recordingBackupProgressBroadcaster{}
	inst.SetBackupProgressBroadcaster(broadcaster)

	started := make(chan struct{}, 1)
	cancelObserved := make(chan struct{}, 1)

	fakeClient := &nodeclient.FakeNodeClient{NodeID: fixture.nodeID}
	fakeClient.CreateBackupArchiveFunc = func(ctx context.Context, _ string, _ []string, _ string) (int64, string, error) {
		started <- struct{}{}
		select {
		case <-ctx.Done():
			cancelObserved <- struct{}{}
			return 0, "", ctx.Err()
		case <-time.After(5 * time.Second):
			return 0, "", errors.New("backup cancel was not observed")
		}
	}
	inst.embeddedNodeClient = fakeClient

	backup, errCreate := inst.CreateManualBackup(fixture.gameServer, fixture.userID, "")
	if errCreate != nil {
		t.Fatalf("CreateManualBackup() error = %v", errCreate)
	}

	select {
	case <-started:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("background archive worker did not start")
	}

	fakeClient.DeleteFilesResult = []string{remotePathBase(backup.ArchivePath)}
	errDelete := inst.DeleteGameServerBackup(fixture.gameServer, backup)
	if errDelete != nil {
		t.Fatalf("DeleteGameServerBackup() error = %v", errDelete)
	}

	select {
	case <-cancelObserved:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("backup cancel was not observed")
	}

	_, errGet := inst.db.GetGameServerBackupByID(backup.ID)
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Fatalf("GetGameServerBackupByID() error = %v, want %v", errGet, sql.ErrNoRows)
	}
	if len(fakeClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(fakeClient.DeleteFilesCalls))
	}
	if broadcaster.containsPhase(xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED) {
		t.Fatalf("backup progress unexpectedly contained failed phase after cancel: %v", broadcaster.events)
	}
}

func TestRestoreGameServerBackupAllowsHistoricalArchiveAfterBackupRootChange(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "keep.txt"), []byte("before"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(keep.txt) error = %v", errWrite)
	}

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "historical-restore.zip")
	createTestZipArchive(t, archivePath, map[string]string{
		"keep.txt": "after",
	})
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       10,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 12, 30, 0, 0, time.UTC),
	})

	newBackupRoot := filepath.Join(t.TempDir(), "new-backups-root")
	errMkdir := os.MkdirAll(newBackupRoot, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(newBackupRoot) error = %v", errMkdir)
	}
	fixture.gameServer.BackupDirectory = newBackupRoot

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}

	contents, errRead := os.ReadFile(filepath.Join(fixture.gameServer.Directory, "keep.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile(keep.txt) error = %v", errRead)
	}
	if string(contents) != "after" {
		t.Fatalf("keep.txt = %q, want %q", string(contents), "after")
	}
}

func TestRestoreGameServerBackupExtractsLocalArchiveThroughNodeClient(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	localClient := &nodeclient.FakeNodeClient{NodeID: fixture.nodeID}
	inst.embeddedNodeClient = localClient
	inst.nodeRegistry = nil

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "restore.zip")
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     fixture.backupRoot,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       10,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 12, 30, 0, 0, time.UTC),
	})

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_EXACT,
	)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}
	if len(localClient.ExtractBackupArchiveCalls) != 1 {
		t.Fatalf("ExtractBackupArchive call count = %d, want 1", len(localClient.ExtractBackupArchiveCalls))
	}
	call := localClient.ExtractBackupArchiveCalls[0]
	if call.Directory != fixture.gameServer.Directory {
		t.Fatalf("ExtractBackupArchive directory = %q, want %q", call.Directory, fixture.gameServer.Directory)
	}
	if call.ArchivePath != archivePath {
		t.Fatalf("ExtractBackupArchive archive path = %q, want %q", call.ArchivePath, archivePath)
	}
	if call.Mode != node.ExtractModeExact {
		t.Fatalf("ExtractBackupArchive mode = %v, want %v", call.Mode, node.ExtractModeExact)
	}
}

func TestRestoreGameServerBackupExtractsRemoteArchiveOnOwningNode(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	registry := noderegistry.New(fixture.nodeID, nil)
	registry.Register(remoteClient)
	inst.nodeRegistry = registry

	_, errInsertNode := inst.db.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-remote"),
		Name:      omit.From("Remote Backup Node"),
		ListenURL: omit.From("http://remotehost:9080"),
		Enabled:   omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode(node-remote) error = %v", errInsertNode)
	}

	fixture.gameServer.NodeID = "node-remote"
	fixture.gameServer.Directory = "/home/clinton/xylona/server-backup"
	fixture.gameServer.BackupDirectory = "/home/clinton/xylona/backups"
	archivePath := "/home/clinton/xylona/backups/server-backup/restore.zip"
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          "node-remote",
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     fixture.gameServer.BackupDirectory,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       64,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 20, 4, 42, 55, 0, time.UTC),
	})

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_EXACT,
	)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}

	if len(remoteClient.ExtractBackupArchiveCalls) != 1 {
		t.Fatalf("ExtractBackupArchive call count = %d, want 1", len(remoteClient.ExtractBackupArchiveCalls))
	}
	call := remoteClient.ExtractBackupArchiveCalls[0]
	if call.Directory != fixture.gameServer.Directory {
		t.Fatalf("ExtractBackupArchive directory = %q, want %q", call.Directory, fixture.gameServer.Directory)
	}
	if call.ArchivePath != archivePath {
		t.Fatalf("ExtractBackupArchive archive path = %q, want %q", call.ArchivePath, archivePath)
	}
	if call.Mode != node.ExtractModeExact {
		t.Fatalf("ExtractBackupArchive mode = %v, want %v", call.Mode, node.ExtractModeExact)
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
	if len(remoteClient.DeleteFilesCalls) != 0 {
		t.Fatalf("DeleteFiles call count = %d, want 0", len(remoteClient.DeleteFilesCalls))
	}
}

func TestRestoreGameServerBackupRejectsBlankArchiveRootAfterMigration(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	originalPath := filepath.Join(fixture.gameServer.Directory, "keep.txt")
	errWrite := os.WriteFile(originalPath, []byte("before"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(keep.txt) error = %v", errWrite)
	}

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "legacy-blank-root.zip")
	createTestZipArchive(t, archivePath, map[string]string{
		"keep.txt": "after",
	})

	backup, errCreate := inst.db.CreateGameServerBackup(db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     "",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       10,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 12, 45, 0, 0, time.UTC),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreate)
	}

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	)
	if !errors.Is(errRestore, errInvalidBackupArchivePath) {
		t.Fatalf("RestoreGameServerBackup() error = %v, want %v", errRestore, errInvalidBackupArchivePath)
	}

	contents, errRead := os.ReadFile(originalPath)
	if errRead != nil {
		t.Fatalf("ReadFile(keep.txt) error = %v", errRead)
	}
	if string(contents) != "before" {
		t.Fatalf("keep.txt = %q, want %q after rejected restore", string(contents), "before")
	}
}

func TestCreateScheduledBackupPrunesHistoricalBackupsAfterBackupRootChange(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	fixture.gameServer.MaxBackups = 1

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	oldArchivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "historical-scheduled.zip")
	createTestZipArchive(t, oldArchivePath, map[string]string{
		"old.txt": "old",
	})
	oldBackup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       "",
		TriggerSource:   "scheduled",
		ArchivePath:     oldArchivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       8,
		RetentionExempt: false,
		CreatedAt:       time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
	})

	newBackupRoot := filepath.Join(t.TempDir(), "new-backups-root")
	errMkdir := os.MkdirAll(newBackupRoot, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(newBackupRoot) error = %v", errMkdir)
	}
	fixture.gameServer.BackupDirectory = newBackupRoot

	newBackup, errCreate := inst.CreateScheduledBackup(fixture.gameServer)
	if errCreate != nil {
		t.Fatalf("CreateScheduledBackup() error = %v", errCreate)
	}

	_, errGet := inst.db.GetGameServerBackupByID(oldBackup.ID)
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Fatalf("GetGameServerBackupByID(oldBackup) error = %v, want %v", errGet, sql.ErrNoRows)
	}
	if _, errStat := os.Stat(oldArchivePath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(oldArchivePath) error = %v, want %v", errStat, os.ErrNotExist)
	}
	if filepath.Dir(newBackup.ArchivePath) != filepath.Join(newBackupRoot, fixture.gameServer.ID) {
		t.Fatalf("CreateScheduledBackup().ArchivePath dir = %q, want %q", filepath.Dir(newBackup.ArchivePath), filepath.Join(newBackupRoot, fixture.gameServer.ID))
	}
}

func TestRestoreGameServerBackupExactDeletesExtraFiles(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "keep.txt"), []byte("old"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(keep.txt) error = %v", errWrite)
	}
	errWrite = os.WriteFile(filepath.Join(fixture.gameServer.Directory, "extra.txt"), []byte("remove-me"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(extra.txt) error = %v", errWrite)
	}

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "restore-exact.zip")
	createTestZipArchive(t, archivePath, map[string]string{
		"keep.txt":         "new",
		"nested/file.txt":  "nested",
		"nested/inner.cfg": "config",
	})
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       20,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	})

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_EXACT,
	)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}

	keepContents, errRead := os.ReadFile(filepath.Join(fixture.gameServer.Directory, "keep.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile(keep.txt) error = %v", errRead)
	}
	if string(keepContents) != "new" {
		t.Fatalf("keep.txt = %q, want %q", string(keepContents), "new")
	}
	if _, errStat := os.Stat(filepath.Join(fixture.gameServer.Directory, "extra.txt")); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(extra.txt) error = %v, want %v", errStat, os.ErrNotExist)
	}
	nestedContents, errRead := os.ReadFile(filepath.Join(fixture.gameServer.Directory, "nested", "file.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile(nested/file.txt) error = %v", errRead)
	}
	if string(nestedContents) != "nested" {
		t.Fatalf("nested/file.txt = %q, want %q", string(nestedContents), "nested")
	}
}

func TestRestoreGameServerBackupExactPreservesEmptyDirectory(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	emptyDirectoryPath := filepath.Join(fixture.gameServer.Directory, "empty-dir")
	errMkdir := os.MkdirAll(emptyDirectoryPath, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(empty-dir) error = %v", errMkdir)
	}
	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "keep.txt"), []byte("before"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(keep.txt) error = %v", errWrite)
	}

	backup := fixture.createCompletedManualBackup(t, "")

	errRemove := os.RemoveAll(emptyDirectoryPath)
	if errRemove != nil {
		t.Fatalf("RemoveAll(empty-dir) error = %v", errRemove)
	}
	errWrite = os.WriteFile(filepath.Join(fixture.gameServer.Directory, "extra.txt"), []byte("remove-me"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(extra.txt) error = %v", errWrite)
	}

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_EXACT,
	)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}

	info, errStat := os.Stat(emptyDirectoryPath)
	if errStat != nil {
		t.Fatalf("Stat(empty-dir) error = %v", errStat)
	}
	if !info.IsDir() {
		t.Fatalf("empty-dir IsDir = false, want true")
	}
	entries, errReadDir := os.ReadDir(emptyDirectoryPath)
	if errReadDir != nil {
		t.Fatalf("ReadDir(empty-dir) error = %v", errReadDir)
	}
	if len(entries) != 0 {
		t.Fatalf("ReadDir(empty-dir) len = %d, want 0", len(entries))
	}
}

func TestRestoreGameServerBackupPreservesNonWritableDirectoryPermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("directory permission assertions are not reliable on Windows")
	}

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "restore-non-writable-dir.zip")
	createTestZipArchiveWithModes(t, archivePath, []testZipArchiveEntry{
		{
			name:  "protected-dir/",
			isDir: true,
			mode:  0o500,
		},
		{
			name:     "protected-dir/keep.txt",
			contents: "archived",
			mode:     0o600,
		},
	})
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       24,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 12, 30, 0, 0, time.UTC),
	})

	protectedDirectory := filepath.Join(fixture.gameServer.Directory, "protected-dir")
	errMkdir := os.MkdirAll(protectedDirectory, 0o700)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(protected-dir) error = %v", errMkdir)
	}
	t.Cleanup(func() {
		errChmod := os.Chmod(protectedDirectory, 0o700) //nolint:gosec // test cleanup needs owner write+execute on a directory
		if errChmod != nil && !errors.Is(errChmod, os.ErrNotExist) {
			t.Errorf("cleanup Chmod(protected-dir) error = %v", errChmod)
		}
	})
	errWrite := os.WriteFile(filepath.Join(protectedDirectory, "keep.txt"), []byte("original"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(protected-dir/keep.txt) error = %v", errWrite)
	}

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}

	info, errStat := os.Stat(protectedDirectory)
	if errStat != nil {
		t.Fatalf("Stat(protected-dir) error = %v", errStat)
	}
	if info.Mode().Perm() != 0o500 {
		t.Fatalf("protected-dir mode = %o, want %o", info.Mode().Perm(), 0o500)
	}

	restoredContents, errRead := os.ReadFile(filepath.Join(protectedDirectory, "keep.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile(protected-dir/keep.txt) error = %v", errRead)
	}
	if string(restoredContents) != "archived" {
		t.Fatalf("protected-dir/keep.txt = %q, want %q", string(restoredContents), "archived")
	}
}

func TestRestoreGameServerBackupExactPreservesNonWritableDirectoryPermissionsAfterRemovingStaleFile(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("directory permission assertions are not reliable on Windows")
	}

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "restore-exact-non-writable-dir.zip")
	createTestZipArchiveWithModes(t, archivePath, []testZipArchiveEntry{
		{
			name:  "protected-dir/",
			isDir: true,
			mode:  0o500,
		},
		{
			name:     "protected-dir/keep.txt",
			contents: "archived",
			mode:     0o600,
		},
	})
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       25,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 12, 45, 0, 0, time.UTC),
	})

	protectedDirectory := filepath.Join(fixture.gameServer.Directory, "protected-dir")
	errMkdir := os.MkdirAll(protectedDirectory, 0o700)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(protected-dir) error = %v", errMkdir)
	}
	t.Cleanup(func() {
		errChmod := os.Chmod(protectedDirectory, 0o700) //nolint:gosec // test cleanup needs owner write+execute on a directory
		if errChmod != nil && !errors.Is(errChmod, os.ErrNotExist) {
			t.Errorf("cleanup Chmod(protected-dir) error = %v", errChmod)
		}
	})
	errWrite := os.WriteFile(filepath.Join(protectedDirectory, "keep.txt"), []byte("original"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(protected-dir/keep.txt) error = %v", errWrite)
	}
	errWrite = os.WriteFile(filepath.Join(protectedDirectory, "stale.txt"), []byte("stale"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(protected-dir/stale.txt) error = %v", errWrite)
	}

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_EXACT,
	)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}

	if _, errStat := os.Stat(filepath.Join(protectedDirectory, "stale.txt")); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(protected-dir/stale.txt) error = %v, want %v", errStat, os.ErrNotExist)
	}

	info, errStat := os.Stat(protectedDirectory)
	if errStat != nil {
		t.Fatalf("Stat(protected-dir) error = %v", errStat)
	}
	if info.Mode().Perm() != 0o500 {
		t.Fatalf("protected-dir mode = %o, want %o", info.Mode().Perm(), 0o500)
	}

	restoredContents, errRead := os.ReadFile(filepath.Join(protectedDirectory, "keep.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile(protected-dir/keep.txt) error = %v", errRead)
	}
	if string(restoredContents) != "archived" {
		t.Fatalf("protected-dir/keep.txt = %q, want %q", string(restoredContents), "archived")
	}
}

func TestRestoreGameServerBackupExactReplacesConflictingDestinationTypes(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	errMkdir := os.MkdirAll(filepath.Join(fixture.gameServer.Directory, "dir-target"), 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(dir-target) error = %v", errMkdir)
	}
	errWrite := os.WriteFile(
		filepath.Join(fixture.gameServer.Directory, "dir-target", "nested.txt"),
		[]byte("nested"),
		0o600,
	)
	if errWrite != nil {
		t.Fatalf("WriteFile(dir-target/nested.txt) error = %v", errWrite)
	}
	errWrite = os.WriteFile(filepath.Join(fixture.gameServer.Directory, "file-target.txt"), []byte("file-shape"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(file-target.txt) error = %v", errWrite)
	}

	backup := fixture.createCompletedManualBackup(t, "")

	errRemove := os.RemoveAll(filepath.Join(fixture.gameServer.Directory, "dir-target"))
	if errRemove != nil {
		t.Fatalf("RemoveAll(dir-target) error = %v", errRemove)
	}
	errWrite = os.WriteFile(filepath.Join(fixture.gameServer.Directory, "dir-target"), []byte("wrong-file"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(dir-target as file) error = %v", errWrite)
	}

	errRemove = os.Remove(filepath.Join(fixture.gameServer.Directory, "file-target.txt"))
	if errRemove != nil {
		t.Fatalf("Remove(file-target.txt) error = %v", errRemove)
	}
	errMkdir = os.MkdirAll(filepath.Join(fixture.gameServer.Directory, "file-target.txt"), 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(file-target.txt as dir) error = %v", errMkdir)
	}
	errWrite = os.WriteFile(
		filepath.Join(fixture.gameServer.Directory, "file-target.txt", "extra.txt"),
		[]byte("wrong-dir"),
		0o600,
	)
	if errWrite != nil {
		t.Fatalf("WriteFile(file-target.txt/extra.txt) error = %v", errWrite)
	}

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_EXACT,
	)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}

	dirInfo, errStat := os.Stat(filepath.Join(fixture.gameServer.Directory, "dir-target"))
	if errStat != nil {
		t.Fatalf("Stat(dir-target) error = %v", errStat)
	}
	if !dirInfo.IsDir() {
		t.Fatalf("dir-target IsDir = false, want true")
	}
	nestedContents, errRead := os.ReadFile(filepath.Join(fixture.gameServer.Directory, "dir-target", "nested.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile(dir-target/nested.txt) error = %v", errRead)
	}
	if string(nestedContents) != "nested" {
		t.Fatalf("dir-target/nested.txt = %q, want %q", string(nestedContents), "nested")
	}

	fileInfo, errStat := os.Stat(filepath.Join(fixture.gameServer.Directory, "file-target.txt"))
	if errStat != nil {
		t.Fatalf("Stat(file-target.txt) error = %v", errStat)
	}
	if fileInfo.IsDir() {
		t.Fatalf("file-target.txt IsDir = true, want false")
	}
	fileContents, errRead := os.ReadFile(filepath.Join(fixture.gameServer.Directory, "file-target.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile(file-target.txt) error = %v", errRead)
	}
	if string(fileContents) != "file-shape" {
		t.Fatalf("file-target.txt = %q, want %q", string(fileContents), "file-shape")
	}
}

func TestRestoreGameServerBackupRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "restore-traversal.zip")
	createTestZipArchive(t, archivePath, map[string]string{
		"../escaped.txt": "nope",
	})
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       21,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC),
	})

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	)
	if errRestore == nil {
		t.Fatal("RestoreGameServerBackup() error = nil, want path traversal error")
	}
	if !strings.Contains(strings.ToLower(errRestore.Error()), "escapes root") {
		t.Fatalf("RestoreGameServerBackup() error = %v, want archive root escape context", errRestore)
	}
	if _, errStat := os.Stat(filepath.Join(filepath.Dir(fixture.gameServer.Directory), "escaped.txt")); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(escaped.txt) error = %v, want %v", errStat, os.ErrNotExist)
	}
}

func TestRestoreGameServerBackupRejectsCorruptedCRCEntry(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	originalPath := filepath.Join(fixture.gameServer.Directory, "world.txt")
	errWrite := os.WriteFile(originalPath, []byte("live-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(world.txt) error = %v", errWrite)
	}

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "restore-crc-corrupt.zip")
	createTestZipArchive(t, archivePath, map[string]string{
		"world.txt": "seed-data",
	})
	corruptTestZipEntryCRC(t, archivePath, "world.txt")
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       26,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 13, 30, 0, 0, time.UTC),
	})

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	)
	if errRestore == nil {
		t.Fatal("RestoreGameServerBackup() error = nil, want CRC corruption failure")
	}
	if !strings.Contains(strings.ToLower(errRestore.Error()), "crc") && !strings.Contains(strings.ToLower(errRestore.Error()), "checksum") {
		t.Fatalf("RestoreGameServerBackup() error = %v, want integrity failure context", errRestore)
	}
	currentContents, errRead := os.ReadFile(originalPath)
	if errRead != nil {
		t.Fatalf("ReadFile(world.txt) error = %v", errRead)
	}
	if string(currentContents) != "live-state" {
		t.Fatalf("world.txt = %q, want %q after failed restore", string(currentContents), "live-state")
	}
}

func TestRestoreGameServerBackupBroadcastsFailedProgressWhenNodeExtractFails(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "restore-staging-failure.zip")
	createTestZipArchive(t, archivePath, map[string]string{
		"world.txt": "seed-data",
	})
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       27,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 13, 45, 0, 0, time.UTC),
	})

	inst.embeddedNodeClient = &nodeclient.FakeNodeClient{
		NodeID:                  fixture.nodeID,
		ExtractBackupArchiveErr: errors.New("forced node restore failure"),
	}

	broadcaster := &recordingBackupProgressBroadcaster{}
	inst.SetBackupProgressBroadcaster(broadcaster)

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	)
	if errRestore == nil {
		t.Fatal("RestoreGameServerBackup() error = nil, want node extract failure")
	}
	if !strings.Contains(errRestore.Error(), "forced node restore failure") {
		t.Fatalf("RestoreGameServerBackup() error = %v, want node extract failure context", errRestore)
	}

	gotPhases := broadcaster.phases()
	wantPhases := []xylona.BackupProgressPhase{
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_PREPARING,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_STAGING,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_APPLYING,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
	}
	if !slices.Equal(gotPhases, wantPhases) {
		t.Fatalf("backup progress phases = %v, want %v", gotPhases, wantPhases)
	}
	if !broadcaster.allServerNamesMatch(fixture.gameServer.Name) {
		t.Fatalf("backup progress server names = %v, want all %q", broadcaster.serverNames(), fixture.gameServer.Name)
	}
}

func TestRestoreGameServerBackupRejectsDestinationSymlink(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	outsideDirectory := t.TempDir()
	outsideTargetPath := filepath.Join(outsideDirectory, "outside.txt")
	linkPath := filepath.Join(fixture.gameServer.Directory, "link.txt")
	errLink := os.Symlink(outsideTargetPath, linkPath)
	if errLink != nil {
		t.Skipf("os.Symlink() unsupported in this environment: %v", errLink)
	}

	archivePath := filepath.Join(fixture.backupRoot, fixture.gameServer.ID, "restore-symlink.zip")
	createTestZipArchive(t, archivePath, map[string]string{
		"link.txt": "blocked",
	})
	backup := fixture.createBackupRow(t, db.CreateGameServerBackupParams{
		GameServerID:    fixture.gameServer.ID,
		NodeID:          fixture.nodeID,
		CreatedBy:       fixture.userID,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       22,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 14, 0, 0, 0, time.UTC),
	})

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	)
	if !errors.Is(errRestore, node.ErrRestoreDestinationSymlink) {
		t.Fatalf("RestoreGameServerBackup() error = %v, want %v", errRestore, node.ErrRestoreDestinationSymlink)
	}
	if _, errStat := os.Stat(outsideTargetPath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(outsideTargetPath) error = %v, want %v", errStat, os.ErrNotExist)
	}
}

func TestRestoreGameServerBackupReplacesExistingFileWithBackedUpContentAndMode(t *testing.T) {
	t.Parallel()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	originalPath := filepath.Join(fixture.gameServer.Directory, "replace.txt")
	errWrite := os.WriteFile(originalPath, []byte("original"), 0o400)
	if errWrite != nil {
		t.Fatalf("WriteFile(replace.txt original) error = %v", errWrite)
	}

	backup := fixture.createCompletedManualBackup(t, "")

	errChmod := os.Chmod(originalPath, 0o600)
	if errChmod != nil {
		t.Fatalf("Chmod(replace.txt mutated) error = %v", errChmod)
	}
	errWrite = os.WriteFile(originalPath, []byte("mutated"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(replace.txt mutated) error = %v", errWrite)
	}

	errRestore := inst.RestoreGameServerBackup(
		fixture.gameServer,
		backup.ID,
		xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY,
	)
	if errRestore != nil {
		t.Fatalf("RestoreGameServerBackup() error = %v", errRestore)
	}

	restoredContents, errRead := os.ReadFile(originalPath)
	if errRead != nil {
		t.Fatalf("ReadFile(replace.txt) error = %v", errRead)
	}
	if string(restoredContents) != "original" {
		t.Fatalf("replace.txt = %q, want %q", string(restoredContents), "original")
	}

	if runtime.GOOS != "windows" {
		info, errStat := os.Stat(originalPath)
		if errStat != nil {
			t.Fatalf("Stat(replace.txt) error = %v", errStat)
		}
		if info.Mode().Perm() != 0o400 {
			t.Fatalf("replace.txt mode = %o, want %o", info.Mode().Perm(), 0o400)
		}
	}
}

func TestValidateBackupZipEntryPathPreservesWhitespace(t *testing.T) {
	t.Parallel()

	got, errValidate := validateBackupZipEntryPath(" folder/ keep .txt ")
	if errValidate != nil {
		t.Fatalf("validateBackupZipEntryPath() error = %v", errValidate)
	}
	if got != " folder/ keep .txt " {
		t.Fatalf("validateBackupZipEntryPath() = %q, want %q", got, " folder/ keep .txt ")
	}
}

func TestValidateBackupPathsAllowColonInFileName(t *testing.T) {
	t.Parallel()

	relativePath := "logs/latest:1.txt"

	gotRelativePath, errValidate := validateBackupRelativePath(relativePath)
	if errValidate != nil {
		t.Fatalf("validateBackupRelativePath() error = %v", errValidate)
	}
	if gotRelativePath != relativePath {
		t.Fatalf("validateBackupRelativePath() = %q, want %q", gotRelativePath, relativePath)
	}

	gotArchivePath, errValidate := validateBackupZipEntryPath(relativePath)
	if errValidate != nil {
		t.Fatalf("validateBackupZipEntryPath() error = %v", errValidate)
	}
	if gotArchivePath != relativePath {
		t.Fatalf("validateBackupZipEntryPath() = %q, want %q", gotArchivePath, relativePath)
	}
}

type backupServiceFixture struct {
	userID     string
	nodeID     string
	gameID     string
	backupRoot string
	gameServer *models.GameServer
	inst       *Instance
}

func newBackupServiceFixture(t *testing.T, inst *Instance) backupServiceFixture {
	t.Helper()

	now := time.Date(2026, 4, 1, 6, 0, 0, 0, time.UTC)
	userID := "user-backup"
	nodeID := "node-backup"
	gameID := "game-backup"
	serverID := "server-backup"
	serverDir := filepath.Join(t.TempDir(), "server-data")
	backupRoot := filepath.Join(t.TempDir(), "backups-root")

	if inst.embeddedNodeClient == nil {
		inst.embeddedNodeClient = nodeclient.NewInProcessClient(nodeID, node.New(inst.ctx, nil, inst.db))
	}

	errMkdir := os.MkdirAll(serverDir, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(serverDir) error = %v", errMkdir)
	}
	errMkdir = os.MkdirAll(backupRoot, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(backupRoot) error = %v", errMkdir)
	}

	_, errCreateUser := inst.db.CreateUser(&models.UserSetter{
		ID:           omit.From(userID),
		UserName:     omit.From("backup-user"),
		Email:        omit.From("backup@example.com"),
		FirstName:    omit.From("Backup"),
		LastName:     omit.From("User"),
		PasswordHash: omit.From("hash"),
		SuperUser:    omit.From(false),
		LastLoginAt:  omit.From(now),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	})
	if errCreateUser != nil {
		t.Fatalf("CreateUser() error = %v", errCreateUser)
	}

	_, errInsertNode := inst.db.InsertNode(&models.NodeSetter{
		ID:        omit.From(nodeID),
		Name:      omit.From("Backup Node"),
		ListenURL: omit.From("http://localhost:8080"),
		Enabled:   omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode() error = %v", errInsertNode)
	}

	_, errUpsertIP := inst.db.UpsertIP(&models.IPSetter{
		Address:            omit.From("127.0.0.1"),
		NodeID:             omit.From(nodeID),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
	})
	if errUpsertIP != nil {
		t.Fatalf("UpsertIP() error = %v", errUpsertIP)
	}

	_, errInsertGame := inst.db.InsertGame(inst.db.DB, &models.GameSetter{
		ID:                omit.From(gameID),
		Name:              omit.From("Backup Test Game"),
		DefaultPort:       omit.From(int64(25565)),
		DefaultQueryPort:  omit.From(int64(25565)),
		DefaultMaxPlayers: omit.From(int64(20)),
	})
	if errInsertGame != nil {
		t.Fatalf("InsertGame() error = %v", errInsertGame)
	}

	gameServer, errInsertServer := inst.db.InsertGameServer(inst.db.DB, &models.GameServerSetter{
		ID:               omit.From(serverID),
		UserID:           omit.From(userID),
		Name:             omit.From("Backup Server"),
		GameID:           omit.From(gameID),
		Status:           omit.From("OFFLINE"),
		SetPlayers:       omit.From(int64(20)),
		MaxPlayers:       omit.From(int64(20)),
		Map:              omit.From("world"),
		IP:               omit.From("127.0.0.1"),
		Port:             omit.From(int64(25565)),
		QueryPort:        omit.From(int64(25565)),
		Directory:        omit.From(serverDir),
		BackupsEnabled:   omit.From(true),
		BackupDirectory:  omit.From(backupRoot),
		MaxBackups:       omit.From(int64(2)),
		NodeID:           omit.From(nodeID),
		StartArgsPatches: omit.From("[]"),
		CreatedAt:        omit.From(now),
		UpdatedAt:        omit.From(now),
	})
	if errInsertServer != nil {
		t.Fatalf("InsertGameServer() error = %v", errInsertServer)
	}

	return backupServiceFixture{
		userID:     userID,
		nodeID:     nodeID,
		gameID:     gameID,
		backupRoot: backupRoot,
		gameServer: gameServer,
		inst:       inst,
	}
}

func installBlockingBackupCreateClient(t *testing.T, inst *Instance, nodeID string, started chan<- struct{}, release <-chan struct{}) {
	t.Helper()

	realClient := inst.embeddedNodeClient
	fakeClient := &nodeclient.FakeNodeClient{NodeID: nodeID}
	fakeClient.CreateBackupArchiveFunc = func(ctx context.Context, directory string, includePaths []string, archivePath string) (int64, string, error) {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return 0, "", ctx.Err()
		}
		return realClient.CreateBackupArchive(ctx, directory, includePaths, archivePath)
	}
	inst.embeddedNodeClient = fakeClient
}

func (f backupServiceFixture) createBackupRow(t *testing.T, params db.CreateGameServerBackupParams) *models.GameServerBackup {
	t.Helper()

	if params.ArchiveRoot == "" {
		params.ArchiveRoot = filepath.Dir(filepath.Dir(params.ArchivePath))
	}

	backup, errCreate := f.inst.db.CreateGameServerBackup(params)
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreate)
	}

	return backup
}

func (f backupServiceFixture) waitForBackupCompletion(t *testing.T, backupID string) *models.GameServerBackup {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		backup, errGet := f.inst.db.GetGameServerBackupByID(backupID)
		if errGet == nil {
			if backup.Status == "completed" {
				return backup
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("backup %q did not reach status %q within timeout", backupID, "completed")
	return nil
}

func (f backupServiceFixture) createCompletedManualBackup(t *testing.T, name string) *models.GameServerBackup {
	t.Helper()

	backup, errCreate := f.inst.CreateManualBackup(f.gameServer, f.userID, name)
	if errCreate != nil {
		t.Fatalf("CreateManualBackup() error = %v", errCreate)
	}

	return f.waitForBackupCompletion(t, backup.ID)
}

func createTestZipArchive(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()

	errMkdir := os.MkdirAll(filepath.Dir(archivePath), 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(archivePath), errMkdir)
	}

	file, errCreate := os.Create(archivePath)
	if errCreate != nil {
		t.Fatalf("Create(%q) error = %v", archivePath, errCreate)
	}
	defer func() {
		_ = file.Close()
	}()

	writer := zip.NewWriter(file)
	for name, contents := range entries {
		entryWriter, errEntry := writer.Create(name)
		if errEntry != nil {
			t.Fatalf("Create zip entry %q error = %v", name, errEntry)
		}
		_, errWrite := entryWriter.Write([]byte(contents))
		if errWrite != nil {
			t.Fatalf("Write zip entry %q error = %v", name, errWrite)
		}
	}

	errClose := writer.Close()
	if errClose != nil {
		t.Fatalf("Close zip writer error = %v", errClose)
	}
}

type testZipArchiveEntry struct {
	name     string
	contents string
	mode     fs.FileMode
	isDir    bool
}

func createTestZipArchiveWithModes(t *testing.T, archivePath string, entries []testZipArchiveEntry) {
	t.Helper()

	errMkdir := os.MkdirAll(filepath.Dir(archivePath), 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(archivePath), errMkdir)
	}

	file, errCreate := os.Create(archivePath)
	if errCreate != nil {
		t.Fatalf("Create(%q) error = %v", archivePath, errCreate)
	}
	defer func() {
		_ = file.Close()
	}()

	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{
			Name: entry.name,
		}
		header.SetMode(entry.mode)
		if entry.isDir {
			if !strings.HasSuffix(header.Name, "/") {
				header.Name += "/"
			}
			header.Method = zip.Store
		} else {
			header.Method = zip.Deflate
		}

		entryWriter, errHeader := writer.CreateHeader(header)
		if errHeader != nil {
			t.Fatalf("CreateHeader(%q) error = %v", header.Name, errHeader)
		}
		if entry.isDir {
			continue
		}

		_, errWrite := entryWriter.Write([]byte(entry.contents))
		if errWrite != nil {
			t.Fatalf("Write zip entry %q error = %v", header.Name, errWrite)
		}
	}

	errClose := writer.Close()
	if errClose != nil {
		t.Fatalf("Close zip writer error = %v", errClose)
	}
}

func corruptTestZipEntryCRC(t *testing.T, archivePath string, entryName string) {
	t.Helper()

	cleanArchivePath := filepath.Clean(archivePath)
	archiveBytes, errRead := os.ReadFile(cleanArchivePath)
	if errRead != nil {
		t.Fatalf("ReadFile(%q) error = %v", cleanArchivePath, errRead)
	}

	if !patchZipHeaderCRC(archiveBytes, []byte("PK\x03\x04"), 14, entryName, 0) {
		t.Fatalf("patch local zip header CRC for %q failed", entryName)
	}
	if !patchZipHeaderCRC(archiveBytes, []byte("PK\x01\x02"), 16, entryName, 0) {
		t.Fatalf("patch central zip header CRC for %q failed", entryName)
	}

	//nolint:gosec // test helper rewrites a temp archive created within the fixture-controlled workspace.
	errWrite := os.WriteFile(cleanArchivePath, archiveBytes, 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(%q) error = %v", cleanArchivePath, errWrite)
	}
}

func patchZipHeaderCRC(archiveBytes []byte, signature []byte, crcOffset int, entryName string, crc uint32) bool {
	searchIndex := 0
	for {
		headerIndex := bytes.Index(archiveBytes[searchIndex:], signature)
		if headerIndex == -1 {
			return false
		}
		headerIndex += searchIndex

		if len(signature) == 4 && bytes.Equal(signature, []byte("PK\x03\x04")) {
			if headerIndex+30 > len(archiveBytes) {
				return false
			}
			nameLength := int(binary.LittleEndian.Uint16(archiveBytes[headerIndex+26 : headerIndex+28]))
			extraLength := int(binary.LittleEndian.Uint16(archiveBytes[headerIndex+28 : headerIndex+30]))
			nameStart := headerIndex + 30
			nameEnd := nameStart + nameLength
			if nameEnd > len(archiveBytes) {
				return false
			}
			if string(archiveBytes[nameStart:nameEnd]) == entryName {
				binary.LittleEndian.PutUint32(archiveBytes[headerIndex+crcOffset:headerIndex+crcOffset+4], crc)
				_ = extraLength
				return true
			}
			searchIndex = nameEnd + extraLength
			continue
		}

		if headerIndex+46 > len(archiveBytes) {
			return false
		}
		nameLength := int(binary.LittleEndian.Uint16(archiveBytes[headerIndex+28 : headerIndex+30]))
		extraLength := int(binary.LittleEndian.Uint16(archiveBytes[headerIndex+30 : headerIndex+32]))
		commentLength := int(binary.LittleEndian.Uint16(archiveBytes[headerIndex+32 : headerIndex+34]))
		nameStart := headerIndex + 46
		nameEnd := nameStart + nameLength
		if nameEnd > len(archiveBytes) {
			return false
		}
		if string(archiveBytes[nameStart:nameEnd]) == entryName {
			binary.LittleEndian.PutUint32(archiveBytes[headerIndex+crcOffset:headerIndex+crcOffset+4], crc)
			return true
		}
		searchIndex = nameEnd + extraLength + commentLength
	}
}

func readBackupArchiveEntries(t *testing.T, archivePath string) map[string]string {
	t.Helper()

	reader, errOpen := zip.OpenReader(archivePath)
	if errOpen != nil {
		t.Fatalf("OpenReader(%q) error = %v", archivePath, errOpen)
	}
	defer func() {
		_ = reader.Close()
	}()

	entries := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		entryReader, errEntry := file.Open()
		if errEntry != nil {
			t.Fatalf("Open zip entry %q error = %v", file.Name, errEntry)
		}
		contents, errRead := io.ReadAll(entryReader)
		_ = entryReader.Close()
		if errRead != nil {
			t.Fatalf("Read zip entry %q error = %v", file.Name, errRead)
		}
		entries[file.Name] = string(contents)
	}

	return entries
}

type recordingBackupProgressBroadcaster struct {
	events []*xylona.BackupProgress
	mu     sync.Mutex
}

func (b *recordingBackupProgressBroadcaster) BroadcastBackupProgress(_ string, progress *xylona.BackupProgress) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, progress)
}

func (b *recordingBackupProgressBroadcaster) phases() []xylona.BackupProgressPhase {
	b.mu.Lock()
	defer b.mu.Unlock()
	phases := make([]xylona.BackupProgressPhase, 0, len(b.events))
	for _, event := range b.events {
		phases = append(phases, event.GetPhase())
	}
	return phases
}

func (b *recordingBackupProgressBroadcaster) containsPhase(phase xylona.BackupProgressPhase) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, event := range b.events {
		if event.GetPhase() == phase {
			return true
		}
	}
	return false
}

func (b *recordingBackupProgressBroadcaster) phaseCount(phase xylona.BackupProgressPhase) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	count := 0
	for _, event := range b.events {
		if event.GetPhase() == phase {
			count++
		}
	}

	return count
}

func (b *recordingBackupProgressBroadcaster) serverNames() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	names := make([]string, 0, len(b.events))
	for _, event := range b.events {
		names = append(names, event.GetGameServerName())
	}

	return names
}

func (b *recordingBackupProgressBroadcaster) allServerNamesMatch(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, event := range b.events {
		if event.GetGameServerName() != name {
			return false
		}
	}

	return true
}
