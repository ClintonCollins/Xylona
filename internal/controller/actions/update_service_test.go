package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestUpdateGameServerWithBackupRejectsConcurrentOperation(t *testing.T) {
	inst := &Instance{activeGameServerOps: map[string]struct{}{"server-1": {}}}
	errUpdate := inst.UpdateGameServerWithBackup(&models.GameServer{ID: "server-1"}, nil)
	if !errors.Is(errUpdate, ErrGameServerOperationInProgress) {
		t.Fatalf("UpdateGameServerWithBackup() error = %v, want %v", errUpdate, ErrGameServerOperationInProgress)
	}
}

func TestTryBeginGameServerLifecycleOperationCoordinatesRestartAndUpdate(t *testing.T) {
	inst := &Instance{}
	releaseRestart, errRestart := inst.TryBeginGameServerLifecycleOperation("server-1")
	if errRestart != nil {
		t.Fatalf("TryBeginGameServerLifecycleOperation(restart) error = %v", errRestart)
	}

	errUpdate := inst.UpdateGameServerWithBackup(&models.GameServer{ID: "server-1"}, nil)
	if !errors.Is(errUpdate, ErrGameServerOperationInProgress) {
		t.Fatalf("UpdateGameServerWithBackup() during restart error = %v, want %v", errUpdate, ErrGameServerOperationInProgress)
	}

	releaseRestart()
	releaseUpdate, errUpdateOperation := inst.TryBeginGameServerLifecycleOperation("server-1")
	if errUpdateOperation != nil {
		t.Fatalf("TryBeginGameServerLifecycleOperation(update) after release error = %v", errUpdateOperation)
	}
	releaseUpdate()
}

func TestWaitForUpdateProcessExit(t *testing.T) {
	snapshotUnavailable := func(context.Context, string) (*node.ProcessSnapshot, bool, error) {
		return nil, false, errors.New("snapshot temporarily unavailable")
	}

	t.Run("returns matching updater exit", func(t *testing.T) {
		events := make(chan any, 5)
		events <- "not a status event"
		events <- eventbus.StatusChangedEvent{
			ServerID:  "other-server",
			OldStatus: "UPDATING",
			NewStatus: "OFFLINE",
		}
		events <- eventbus.StatusChangedEvent{
			ServerID:  "server-1",
			OldStatus: "ONLINE",
			NewStatus: "OFFLINE",
		}
		events <- eventbus.StatusChangedEvent{
			ServerID:    "server-1",
			ExecutionID: "stale-execution",
			OldStatus:   "UPDATING",
			NewStatus:   "OFFLINE",
		}
		want := eventbus.StatusChangedEvent{
			ServerID:    "server-1",
			ExecutionID: "execution-1",
			OldStatus:   "updating",
			NewStatus:   "offline",
			ExitCode:    23,
		}
		events <- want

		got, errWait := waitForUpdateProcessExit(
			context.Background(),
			events,
			"server-1",
			"execution-1",
			snapshotUnavailable,
			time.Second,
			time.Millisecond,
		)
		if errWait != nil {
			t.Fatalf("waitForUpdateProcessExit() error = %v", errWait)
		}
		if got != want {
			t.Fatalf("waitForUpdateProcessExit() = %+v, want %+v", got, want)
		}
	})

	t.Run("returns canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, errWait := waitForUpdateProcessExit(
			ctx,
			make(chan any),
			"server-1",
			"execution-1",
			snapshotUnavailable,
			time.Second,
			time.Millisecond,
		)
		if !errors.Is(errWait, context.Canceled) {
			t.Fatalf("waitForUpdateProcessExit() error = %v, want context.Canceled", errWait)
		}
	})

	t.Run("returns timeout", func(t *testing.T) {
		_, errWait := waitForUpdateProcessExit(
			context.Background(),
			make(chan any),
			"server-1",
			"execution-1",
			snapshotUnavailable,
			10*time.Millisecond,
			time.Millisecond,
		)
		if errWait == nil || !strings.Contains(errWait.Error(), "timed out") {
			t.Fatalf("waitForUpdateProcessExit() error = %v, want timeout", errWait)
		}
	})

	t.Run("recovers missed exit from retained snapshot", func(t *testing.T) {
		events := make(chan any)
		close(events)
		getSnapshot := func(context.Context, string) (*node.ProcessSnapshot, bool, error) {
			return &node.ProcessSnapshot{
				ID:                 "server-1",
				ExecutionID:        "execution-1",
				Status:             "OFFLINE",
				PreviousStatus:     "UPDATING",
				TransitionSequence: 2,
				ExitCode:           7,
				ExitCodeKnown:      true,
			}, true, nil
		}

		got, errWait := waitForUpdateProcessExit(
			context.Background(),
			events,
			"server-1",
			"execution-1",
			getSnapshot,
			time.Second,
			time.Millisecond,
		)
		if errWait != nil {
			t.Fatalf("waitForUpdateProcessExit() error = %v", errWait)
		}
		if got.ExecutionID != "execution-1" || got.ExitCode != 7 || !got.ExitCodeKnown {
			t.Fatalf("waitForUpdateProcessExit() = %+v", got)
		}
	})

	t.Run("rejects replacement execution", func(t *testing.T) {
		getSnapshot := func(context.Context, string) (*node.ProcessSnapshot, bool, error) {
			return &node.ProcessSnapshot{ExecutionID: "execution-2", Status: "UPDATING"}, true, nil
		}
		_, errWait := waitForUpdateProcessExit(
			context.Background(),
			make(chan any),
			"server-1",
			"execution-1",
			getSnapshot,
			time.Second,
			time.Millisecond,
		)
		if !errors.Is(errWait, errUpdateExecutionReplaced) {
			t.Fatalf("waitForUpdateProcessExit() error = %v, want replacement", errWait)
		}
	})

	t.Run("fails fast for indeterminate legacy offline state", func(t *testing.T) {
		getSnapshot := func(context.Context, string) (*node.ProcessSnapshot, bool, error) {
			return &node.ProcessSnapshot{Status: "OFFLINE"}, true, nil
		}
		_, errWait := waitForUpdateProcessExit(
			context.Background(),
			make(chan any),
			"server-1",
			"execution-1",
			getSnapshot,
			time.Second,
			time.Millisecond,
		)
		if !errors.Is(errWait, errUpdateCompletionIndeterminate) {
			t.Fatalf("waitForUpdateProcessExit() error = %v, want indeterminate", errWait)
		}
	})
}

func TestWaitForServerOnlineReturnsFalseWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	restarted := waitForServerOnline(ctx, func() (xylona.Status, bool) {
		t.Fatal("status lookup should not be called after cancellation")
		return xylona.Status_UNKNOWN, false
	}, 60, time.Second)
	if restarted {
		t.Fatal("waitForServerOnline() = true, want false after cancellation")
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("waitForServerOnline() took %v after cancellation, want fast exit", elapsed)
	}
}

func TestWaitForServerOnlineReturnsTrueWhenServerComesOnline(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	restarted := waitForServerOnline(ctx, func() (xylona.Status, bool) {
		attempts++
		if attempts < 3 {
			return xylona.Status_OFFLINE, true
		}
		return xylona.Status_ONLINE, true
	}, 5, time.Millisecond)
	if !restarted {
		t.Fatal("waitForServerOnline() = false, want true once server reports ONLINE")
	}
	if attempts != 3 {
		t.Fatalf("status lookup attempts = %d, want 3", attempts)
	}
}

func TestUpdateGameServerRunsInternalUpdaterWhenNoShellCommandConfigured(t *testing.T) {
	ctx := context.Background()
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	serverDir := t.TempDir()
	markerPath := filepath.Join(serverDir, "updated.txt")
	gameID := "internal-update-test"
	gameintegrations.RegisterGame(gameID, internalUpdateTestGame{markerPath: markerPath})

	inst := &Instance{
		ctx:                ctx,
		embeddedNodeClient: newSupervisorBackedNodeClient(ctx, t, supervisorInst, nil),
	}
	gameServer := &models.GameServer{
		ID:        "server-1",
		GameID:    gameID,
		Directory: serverDir,
		UserID:    "user-1",
	}
	gameServer.R.Game = &models.Game{}

	errUpdate := inst.UpdateGameServer(gameServer)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, errStat := os.Stat(markerPath)
		if errStat == nil {
			return
		}
		if !os.IsNotExist(errStat) {
			t.Fatalf("os.Stat(%q) error = %v", markerPath, errStat)
		}
		if time.Now().After(deadline) {
			t.Fatalf("internal updater did not create %q", markerPath)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestUpdateGameServerUsesMinecraftServerSoftwareProvider(t *testing.T) {
	ctx := context.Background()
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	serverDir := t.TempDir()
	provider := &minecraftUpdateTestProvider{
		latestVersion:   "1.21.5",
		downloadVersion: "1.21.5-build-9",
		markerPath:      filepath.Join(serverDir, "paper-1.21.5.jar"),
	}
	withMinecraftUpdateProviderLookup(t, provider)

	inst := &Instance{
		ctx:                ctx,
		embeddedNodeClient: newSupervisorBackedNodeClient(ctx, t, supervisorInst, nil),
	}
	gameServer := &models.GameServer{
		ID:               "minecraft-provider-update",
		GameID:           "minecraft",
		Directory:        serverDir,
		UserID:           "user-1",
		ServerSoftware:   null.From("paper"),
		ServerExecutable: null.From("paper-1.21.4.jar"),
	}
	gameServer.R.Game = &models.Game{
		ID: "minecraft",
	}
	setMinecraftTypedVariants(t, gameServer.R.Game, updateproviders.Variant{
		ID:            "paper",
		Name:          "Paper",
		DefaultTarget: "1.21.5",
		UpdateProvider: &updateproviders.ProviderConfig{
			Kind:     updateproviders.ProviderKindPaperMC,
			SourceID: "paper",
		},
	})

	errUpdate := inst.UpdateGameServer(gameServer)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	if provider.detailsSourceID != "paper" {
		t.Errorf("GetModDetails sourceID = %q, want %q", provider.detailsSourceID, "paper")
	}
	if provider.versionsSourceID != "paper" {
		t.Errorf("GetVersions sourceID = %q, want %q", provider.versionsSourceID, "paper")
	}
	if provider.versionsGameVersion != "1.21.5" {
		t.Errorf("GetVersions gameVersion = %q, want %q", provider.versionsGameVersion, "1.21.5")
	}
	if provider.downloadSourceID != "paper" {
		t.Errorf("Download sourceID = %q, want %q", provider.downloadSourceID, "paper")
	}
	if provider.downloadVersionID != "1.21.5-build-9" {
		t.Errorf("Download versionID = %q, want %q", provider.downloadVersionID, "1.21.5-build-9")
	}

	_, errStat := os.Stat(provider.markerPath)
	if errStat != nil {
		t.Fatalf("expected downloaded marker at %q: %v", provider.markerPath, errStat)
	}

	_, errGetCmd := supervisorInst.GetCommandByID(gameServer.ID)
	if !errors.Is(errGetCmd, supervisor.ErrCommandDoesNotExist) {
		t.Fatalf("expected no supervised update command, got %v", errGetCmd)
	}
}

func TestBackupServerFilesRoutesRemoteFilesystemThroughNodeClient(t *testing.T) {
	controllerDir := t.TempDir()
	localBackupPath := filepath.Join(controllerDir, ".update-backup")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:      "node-remote",
		StatFileErr: os.ErrNotExist,
		ListFilesResult: []node.FileEntry{
			node.NewFileEntry("server.properties", 12, false, time.Now()),
			node.NewFileEntry("paper.jar", 34, false, time.Now(), true),
			node.NewFileEntry("notes.txt", 34, false, time.Now()),
			node.NewFileEntry("world", 56, true, time.Now()),
		},
		CopyFilesResult:  []string{"pending/server.properties", "pending/paper.jar"},
		RenameFileResult: ".update-backup",
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:               "server-remote-backup",
		Directory:        controllerDir,
		NodeID:           "node-remote",
		ServerExecutable: null.From("paper.jar"),
	}

	errBackup := inst.backupServerFiles(gameServer)
	if errBackup != nil {
		t.Fatalf("backupServerFiles() error = %v", errBackup)
	}

	if len(remoteClient.CreateFileOrDirectoryCalls) != 1 {
		t.Fatalf("CreateFileOrDirectory call count = %d, want 1", len(remoteClient.CreateFileOrDirectoryCalls))
	}
	pendingDirectory := remoteClient.CreateFileOrDirectoryCalls[0].RelativePath
	if !strings.HasPrefix(pendingDirectory, ".update-backup.pending-") {
		t.Fatalf("CreateFileOrDirectory relative path = %q, want pending backup path", pendingDirectory)
	}
	if len(remoteClient.CopyFilesCalls) != 1 {
		t.Fatalf("CopyFiles call count = %d, want 1", len(remoteClient.CopyFilesCalls))
	}
	operations := remoteClient.CopyFilesCalls[0].Operations
	if len(operations) != 2 {
		t.Fatalf("CopyFiles operations length = %d, want 2", len(operations))
	}
	if operations[0].SourceRelativePath != "server.properties" || operations[0].DestinationRelativePath != pendingDirectory+"/server.properties" {
		t.Fatalf("first CopyFiles operation = %+v, want server.properties backup", operations[0])
	}
	if operations[1].SourceRelativePath != "paper.jar" || operations[1].DestinationRelativePath != pendingDirectory+"/paper.jar" {
		t.Fatalf("second CopyFiles operation = %+v, want executable jar backup", operations[1])
	}
	if len(remoteClient.RenameFileCalls) != 1 {
		t.Fatalf("RenameFile call count = %d, want 1", len(remoteClient.RenameFileCalls))
	}
	renameCall := remoteClient.RenameFileCalls[0]
	if renameCall.OldRelativePath != pendingDirectory || renameCall.NewRelativePath != ".update-backup" {
		t.Fatalf("RenameFile call = %+v, want pending backup promotion", renameCall)
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
	_, errStat := os.Stat(localBackupPath)
	if !os.IsNotExist(errStat) {
		t.Fatalf("controller backup path stat error = %v, want not exist", errStat)
	}
}

func TestBackupNodeServerFilesAtomicFailureAndRecoveryGuard(t *testing.T) {
	gameServer := &models.GameServer{
		ID:        "server-atomic-backup",
		Directory: t.TempDir(),
	}
	inst := &Instance{ctx: context.Background()}

	t.Run("copy failure removes only pending directory", func(t *testing.T) {
		client := &nodeclient.FakeNodeClient{
			StatFileErr: os.ErrNotExist,
			ListFilesResult: []node.FileEntry{
				node.NewFileEntry("server.properties", 12, false, time.Now()),
			},
			CopyFilesErr:      errors.New("injected copy failure"),
			DeleteFilesResult: []string{"pending"},
		}

		errBackup := inst.backupNodeServerFiles(gameServer, client)
		if errBackup == nil || !strings.Contains(errBackup.Error(), "injected copy failure") {
			t.Fatalf("backupNodeServerFiles() error = %v, want copy failure", errBackup)
		}
		if len(client.CreateFileOrDirectoryCalls) != 1 || len(client.DeleteFilesCalls) != 1 {
			t.Fatalf(
				"pending create/delete calls = %d/%d, want 1/1",
				len(client.CreateFileOrDirectoryCalls),
				len(client.DeleteFilesCalls),
			)
		}
		pendingDirectory := client.CreateFileOrDirectoryCalls[0].RelativePath
		if client.DeleteFilesCalls[0].Files[0] != pendingDirectory {
			t.Fatalf("deleted path = %q, want pending path %q", client.DeleteFilesCalls[0].Files[0], pendingDirectory)
		}
		if len(client.RenameFileCalls) != 0 {
			t.Fatalf("RenameFile call count = %d, want 0", len(client.RenameFileCalls))
		}
	})

	t.Run("canonical recovery directory blocks a new backup", func(t *testing.T) {
		client := &nodeclient.FakeNodeClient{}

		errBackup := inst.backupNodeServerFiles(gameServer, client)
		if errBackup == nil || !strings.Contains(errBackup.Error(), "recovery is pending") {
			t.Fatalf("backupNodeServerFiles() error = %v, want recovery pending", errBackup)
		}
		if len(client.CreateFileOrDirectoryCalls) != 0 {
			t.Fatalf("CreateFileOrDirectory call count = %d, want 0", len(client.CreateFileOrDirectoryCalls))
		}
	})
}

func TestRestoreInternalUpdateTransaction(t *testing.T) {
	manifest := gameintegrations.NewUpdateTransactionManifest("factorio", []gameintegrations.UpdateTransactionEntry{
		{Path: "bin/server", Existed: true},
		{Path: "data/new-file", Existed: false},
	})
	manifestBytes, errManifest := json.Marshal(manifest)
	if errManifest != nil {
		t.Fatalf("json.Marshal() error = %v", errManifest)
	}
	gameServer := &models.GameServer{Directory: t.TempDir()}

	t.Run("restores nested originals and removes introduced files in reverse order", func(t *testing.T) {
		client := &nodeclient.FakeNodeClient{
			ReadFileResult:    manifestBytes,
			DeleteFilesResult: []string{"deleted"},
			CopyFilesResult:   []string{"bin/server"},
		}

		found, errRestore := restoreInternalUpdateTransaction(t.Context(), gameServer, client)
		if errRestore != nil {
			t.Fatalf("restoreInternalUpdateTransaction() error = %v", errRestore)
		}
		if !found {
			t.Fatal("restoreInternalUpdateTransaction() found = false, want true")
		}
		if len(client.DeleteFilesCalls) != 2 {
			t.Fatalf("DeleteFiles call count = %d, want 2", len(client.DeleteFilesCalls))
		}
		if client.DeleteFilesCalls[0].Files[0] != "data/new-file" || client.DeleteFilesCalls[1].Files[0] != "bin/server" {
			t.Fatalf("DeleteFiles order = %v then %v, want reverse manifest order", client.DeleteFilesCalls[0].Files, client.DeleteFilesCalls[1].Files)
		}
		if len(client.CopyFilesCalls) != 1 {
			t.Fatalf("CopyFiles call count = %d, want 1", len(client.CopyFilesCalls))
		}
		operation := client.CopyFilesCalls[0].Operations[0]
		if operation.SourceRelativePath != gameintegrations.InternalUpdateFilesDirectory+"/bin/server" ||
			operation.DestinationRelativePath != "bin/server" {
			t.Fatalf("CopyFiles operation = %+v, want nested original restore", operation)
		}
	})

	t.Run("copy failure retains canonical recovery data", func(t *testing.T) {
		client := &nodeclient.FakeNodeClient{
			ReadFileResult:    manifestBytes,
			DeleteFilesResult: []string{"deleted"},
			CopyFilesErr:      errors.New("injected restore failure"),
		}

		found, errRestore := restoreInternalUpdateTransaction(t.Context(), gameServer, client)
		if !found || errRestore == nil || !strings.Contains(errRestore.Error(), "injected restore failure") {
			t.Fatalf("restoreInternalUpdateTransaction() = (%v, %v), want retained restore failure", found, errRestore)
		}
		for _, call := range client.DeleteFilesCalls {
			if len(call.Files) == 1 && call.Files[0] == ".update-backup" {
				t.Fatal("restore failure deleted canonical recovery directory")
			}
		}
	})
}

func TestRestoreServerFilesRoutesRemoteFilesystemThroughNodeClient(t *testing.T) {
	controllerDir := t.TempDir()
	localSettingsPath := filepath.Join(controllerDir, "server.properties")
	errWrite := os.WriteFile(localSettingsPath, []byte("controller"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(server.properties) error = %v", errWrite)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:      "node-remote",
		ReadFileErr: os.ErrNotExist,
		ListFilesResult: []node.FileEntry{
			node.NewFileEntry("server.properties", 12, false, time.Now()),
		},
		CopyFilesResult:   []string{"server.properties"},
		DeleteFilesResult: []string{".update-backup"},
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:        "server-remote-restore",
		Directory: controllerDir,
		NodeID:    "node-remote",
	}

	errRestore := inst.restoreServerFiles(gameServer)
	if errRestore != nil {
		t.Fatalf("restoreServerFiles() error = %v", errRestore)
	}

	if len(remoteClient.ListFilesCalls) != 1 {
		t.Fatalf("ListFiles call count = %d, want 1", len(remoteClient.ListFilesCalls))
	}
	if remoteClient.ListFilesCalls[0].RelativePath != ".update-backup" {
		t.Fatalf("ListFiles relative path = %q, want %q", remoteClient.ListFilesCalls[0].RelativePath, ".update-backup")
	}
	if len(remoteClient.CopyFilesCalls) != 1 {
		t.Fatalf("CopyFiles call count = %d, want 1", len(remoteClient.CopyFilesCalls))
	}
	operations := remoteClient.CopyFilesCalls[0].Operations
	if len(operations) != 1 {
		t.Fatalf("CopyFiles operations length = %d, want 1", len(operations))
	}
	if operations[0].SourceRelativePath != ".update-backup/server.properties" || operations[0].DestinationRelativePath != "server.properties" {
		t.Fatalf("CopyFiles operation = %+v, want restore server.properties", operations[0])
	}
	if len(remoteClient.ReadFileCalls) != 1 {
		t.Fatalf("ReadFile call count = %d, want 1", len(remoteClient.ReadFileCalls))
	}
	if remoteClient.ReadFileCalls[0].RelativePath != gameintegrations.InternalUpdateManifestPath {
		t.Fatalf("ReadFile relative path = %q, want internal update manifest", remoteClient.ReadFileCalls[0].RelativePath)
	}
	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	if len(remoteClient.DeleteFilesCalls[0].Files) != 1 || remoteClient.DeleteFilesCalls[0].Files[0] != ".update-backup" {
		t.Fatalf("DeleteFiles files = %v, want [.update-backup]", remoteClient.DeleteFilesCalls[0].Files)
	}
	controllerContents, errRead := os.ReadFile(localSettingsPath)
	if errRead != nil {
		t.Fatalf("ReadFile(server.properties) error = %v", errRead)
	}
	if string(controllerContents) != "controller" {
		t.Fatalf("controller server.properties = %q, want %q", string(controllerContents), "controller")
	}
}

func TestBackupAndRestoreServerFilesRoutesEmbeddedNodeThroughCopyFiles(t *testing.T) {
	controllerDir := t.TempDir()
	selfClient := &nodeclient.FakeNodeClient{
		NodeID:      "node-local",
		StatFileErr: os.ErrNotExist,
		ListFilesResult: []node.FileEntry{
			node.NewFileEntry("server.properties", 12, false, time.Now()),
		},
		CopyFilesResult:   []string{"pending/server.properties"},
		RenameFileResult:  ".update-backup",
		DeleteFilesResult: []string{".update-backup"},
	}
	registry := noderegistry.New("node-local", selfClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:        "server-embedded-update-copy",
		Directory: controllerDir,
		NodeID:    "node-local",
	}

	errBackup := inst.backupServerFiles(gameServer)
	if errBackup != nil {
		t.Fatalf("backupServerFiles() error = %v", errBackup)
	}
	if len(selfClient.CopyFilesCalls) != 1 {
		t.Fatalf("backup CopyFiles call count = %d, want 1", len(selfClient.CopyFilesCalls))
	}
	pendingDirectory := selfClient.CreateFileOrDirectoryCalls[0].RelativePath
	if selfClient.CopyFilesCalls[0].Operations[0].DestinationRelativePath != pendingDirectory+"/server.properties" {
		t.Fatalf("backup CopyFiles operation = %+v, want backup destination", selfClient.CopyFilesCalls[0].Operations[0])
	}

	selfClient.CopyFilesCalls = nil
	selfClient.ListFilesCalls = nil
	selfClient.ReadFileErr = os.ErrNotExist
	selfClient.CopyFilesResult = []string{"server.properties"}
	selfClient.ListFilesResult = []node.FileEntry{
		node.NewFileEntry("server.properties", 12, false, time.Now()),
	}

	errRestore := inst.restoreServerFiles(gameServer)
	if errRestore != nil {
		t.Fatalf("restoreServerFiles() error = %v", errRestore)
	}
	if len(selfClient.CopyFilesCalls) != 1 {
		t.Fatalf("restore CopyFiles call count = %d, want 1", len(selfClient.CopyFilesCalls))
	}
	if selfClient.CopyFilesCalls[0].Operations[0].SourceRelativePath != ".update-backup/server.properties" {
		t.Fatalf("restore CopyFiles operation = %+v, want backup source", selfClient.CopyFilesCalls[0].Operations[0])
	}
	if len(selfClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(selfClient.DeleteFilesCalls))
	}
}

func TestUpdateGameServerUsesRemoteNodeForMinecraftJarDownloadAndDelete(t *testing.T) {
	controllerDir := t.TempDir()
	oldJarPath := filepath.Join(controllerDir, "paper-old.jar")
	errWrite := os.WriteFile(oldJarPath, []byte("controller-old"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(paper-old.jar) error = %v", errWrite)
	}

	provider := &minecraftUpdateTestProvider{
		providerID:      "test-minecraft-remote-update-provider",
		latestVersion:   "1.21.5",
		downloadVersion: "1.21.5-build-9",
		downloadURL:     "https://downloads.example.test/paper-new.jar",
		downloadSize:    8192,
		downloadSHA256:  "paper-new-sha",
	}
	withMinecraftUpdateProviderLookup(t, provider)

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                    "node-remote",
		DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "paper-new.jar"},
		DeleteFilesResult:         []string{"paper-old.jar"},
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:               "minecraft-remote-provider-update",
		GameID:           "minecraft",
		Directory:        controllerDir,
		NodeID:           "node-remote",
		UserID:           "user-1",
		ServerSoftware:   null.From("paper"),
		ServerExecutable: null.From("paper-old.jar"),
	}
	gameServer.R.Game = &models.Game{
		ID: "minecraft",
	}
	setMinecraftTypedVariants(t, gameServer.R.Game, updateproviders.Variant{
		ID:            "paper",
		Name:          "Paper",
		DefaultTarget: "1.21.5",
		UpdateProvider: &updateproviders.ProviderConfig{
			Kind:     updateproviders.ProviderKindPaperMC,
			SourceID: "paper",
		},
	})

	errUpdate := inst.UpdateGameServer(gameServer)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	if provider.downloadCalls != 0 {
		t.Fatalf("provider Download call count = %d, want 0 for remote update", provider.downloadCalls)
	}
	if len(remoteClient.DownloadFileFromURLCalls) != 1 {
		t.Fatalf("DownloadFileFromURL call count = %d, want 1", len(remoteClient.DownloadFileFromURLCalls))
	}
	downloadCall := remoteClient.DownloadFileFromURLCalls[0]
	if downloadCall.Directory != controllerDir {
		t.Fatalf("DownloadFileFromURL directory = %q, want %q", downloadCall.Directory, controllerDir)
	}
	if downloadCall.RawURL != "https://downloads.example.test/paper-new.jar" {
		t.Fatalf("DownloadFileFromURL raw URL = %q, want %q", downloadCall.RawURL, "https://downloads.example.test/paper-new.jar")
	}
	if downloadCall.Integrity.ExpectedSize != 8192 || downloadCall.Integrity.ExpectedSHA256 != "paper-new-sha" {
		t.Fatalf("DownloadFileFromURL integrity = %+v, want size=8192 paper-new-sha", downloadCall.Integrity)
	}
	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	if len(remoteClient.DeleteFilesCalls[0].Files) != 1 || remoteClient.DeleteFilesCalls[0].Files[0] != "paper-old.jar" {
		t.Fatalf("DeleteFiles files = %v, want [paper-old.jar]", remoteClient.DeleteFilesCalls[0].Files)
	}
	if gameServer.ServerExecutable.GetOr("") != "paper-new.jar" {
		t.Fatalf("server executable = %q, want %q", gameServer.ServerExecutable.GetOr(""), "paper-new.jar")
	}
	controllerContents, errRead := os.ReadFile(oldJarPath)
	if errRead != nil {
		t.Fatalf("ReadFile(paper-old.jar) error = %v", errRead)
	}
	if string(controllerContents) != "controller-old" {
		t.Fatalf("controller old jar = %q, want %q", string(controllerContents), "controller-old")
	}
}

func TestUpdateGameServerRejectsRemoteMinecraftDownloadWithoutIntegrity(t *testing.T) {
	controllerDir := t.TempDir()
	provider := &minecraftUpdateTestProvider{
		providerID:      "test-minecraft-remote-missing-integrity-provider",
		latestVersion:   "1.21.5",
		downloadVersion: "1.21.5-build-9",
		downloadURL:     "https://downloads.example.test/paper-new.jar",
	}
	withMinecraftUpdateProviderLookup(t, provider)

	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:               "minecraft-remote-missing-integrity",
		GameID:           "minecraft",
		Directory:        controllerDir,
		NodeID:           "node-remote",
		UserID:           "user-1",
		ServerSoftware:   null.From("paper"),
		ServerExecutable: null.From("paper-old.jar"),
	}
	gameServer.R.Game = &models.Game{
		ID: "minecraft",
	}
	setMinecraftTypedVariants(t, gameServer.R.Game, updateproviders.Variant{
		ID:            "paper",
		Name:          "Paper",
		DefaultTarget: "1.21.5",
		UpdateProvider: &updateproviders.ProviderConfig{
			Kind:     updateproviders.ProviderKindPaperMC,
			SourceID: "paper",
		},
	})

	errUpdate := inst.UpdateGameServer(gameServer)
	if errUpdate == nil {
		t.Fatal("UpdateGameServer() error = nil, want missing integrity error")
	}
	if !strings.Contains(errUpdate.Error(), "integrity metadata unavailable") {
		t.Fatalf("UpdateGameServer() error = %v, want missing integrity metadata", errUpdate)
	}
	if provider.downloadCalls != 0 {
		t.Fatalf("provider Download call count = %d, want 0 for remote update", provider.downloadCalls)
	}
	if len(remoteClient.DownloadFileFromURLCalls) != 0 {
		t.Fatalf("DownloadFileFromURL call count = %d, want 0", len(remoteClient.DownloadFileFromURLCalls))
	}
	if len(remoteClient.DeleteFilesCalls) != 0 {
		t.Fatalf("DeleteFiles call count = %d, want 0", len(remoteClient.DeleteFilesCalls))
	}
	if gameServer.ServerExecutable.GetOr("") != "paper-old.jar" {
		t.Fatalf("server executable = %q, want paper-old.jar", gameServer.ServerExecutable.GetOr(""))
	}
}

func TestUpdateGameServerBuildsPaperRemoteDownloadURLWhenProviderOmitsURL(t *testing.T) {
	controllerDir := t.TempDir()
	provider := &minecraftUpdateTestProvider{
		providerID:      "test-minecraft-remote-paper-update-provider",
		latestVersion:   "1.21.5",
		downloadVersion: "1.21.5-9",
		downloadSize:    4096,
		downloadSHA256:  "paper-built-sha",
	}
	withMinecraftUpdateProviderLookup(t, provider)

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                    "node-remote",
		DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "paper-1.21.5-9.jar"},
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:             "minecraft-remote-paper-update",
		GameID:         "minecraft",
		Directory:      controllerDir,
		NodeID:         "node-remote",
		UserID:         "user-1",
		ServerSoftware: null.From("paper"),
	}
	gameServer.R.Game = &models.Game{
		ID: "minecraft",
	}
	setMinecraftTypedVariants(t, gameServer.R.Game, updateproviders.Variant{
		ID:            "paper",
		Name:          "Paper",
		DefaultTarget: "1.21.5",
		UpdateProvider: &updateproviders.ProviderConfig{
			Kind:     updateproviders.ProviderKindPaperMC,
			SourceID: "paper",
		},
	})

	errUpdate := inst.UpdateGameServer(gameServer)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	if len(remoteClient.DownloadFileFromURLCalls) != 1 {
		t.Fatalf("DownloadFileFromURL call count = %d, want 1", len(remoteClient.DownloadFileFromURLCalls))
	}
	wantURL := "https://api.papermc.io/v2/projects/paper/versions/1.21.5/builds/9/downloads/paper-1.21.5-9.jar"
	if remoteClient.DownloadFileFromURLCalls[0].RawURL != wantURL {
		t.Fatalf("DownloadFileFromURL raw URL = %q, want %q", remoteClient.DownloadFileFromURLCalls[0].RawURL, wantURL)
	}
	if remoteClient.DownloadFileFromURLCalls[0].Integrity.ExpectedSize != 4096 ||
		remoteClient.DownloadFileFromURLCalls[0].Integrity.ExpectedSHA256 != "paper-built-sha" {
		t.Fatalf("DownloadFileFromURL integrity = %+v, want size=4096 paper-built-sha", remoteClient.DownloadFileFromURLCalls[0].Integrity)
	}
	if gameServer.ServerExecutable.GetOr("") != "paper-1.21.5-9.jar" {
		t.Fatalf("server executable = %q, want %q", gameServer.ServerExecutable.GetOr(""), "paper-1.21.5-9.jar")
	}
}

func TestUpdateGameServerRejectsUnsupportedMinecraftVariant(t *testing.T) {
	ctx := context.Background()
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	serverDir := t.TempDir()
	markerPath := filepath.Join(serverDir, "updated.txt")
	previousGame, hadPrevious := gameintegrations.GetGame("minecraft")
	gameintegrations.RegisterGame("minecraft", internalUpdateTestGame{markerPath: markerPath})
	t.Cleanup(func() {
		if hadPrevious {
			gameintegrations.RegisterGame("minecraft", previousGame)
			return
		}
		gameintegrations.UnregisterGameForTest("minecraft")
	})

	inst := &Instance{
		ctx:                ctx,
		embeddedNodeClient: newSupervisorBackedNodeClient(ctx, t, supervisorInst, nil),
	}
	gameServer := &models.GameServer{
		ID:               "minecraft-unsupported-update",
		GameID:           "minecraft",
		Directory:        serverDir,
		UserID:           "user-1",
		ServerSoftware:   null.From("fabric"),
		ServerExecutable: null.From("fabric-server.jar"),
	}
	gameServer.R.Game = &models.Game{
		ID: "minecraft",
	}
	setMinecraftTypedVariants(
		t,
		gameServer.R.Game,
		updateproviders.Variant{
			ID:             "vanilla",
			Name:           "Vanilla",
			UpdateProvider: &updateproviders.ProviderConfig{Kind: updateproviders.ProviderKindMojang, SourceID: "vanilla"},
		},
		updateproviders.Variant{
			ID:             "fabric",
			Name:           "Fabric",
			UpdateProvider: &updateproviders.ProviderConfig{Kind: updateproviders.ProviderKindCommand},
		},
	)

	errUpdate := inst.UpdateGameServer(gameServer)
	if !errors.Is(errUpdate, ErrMinecraftVariantUpdateNotSupported) {
		t.Fatalf("UpdateGameServer() error = %v, want %v", errUpdate, ErrMinecraftVariantUpdateNotSupported)
	}

	_, errStat := os.Stat(markerPath)
	if !os.IsNotExist(errStat) {
		t.Fatalf("expected internal updater marker to be absent, got err=%v", errStat)
	}

	_, errGetCmd := supervisorInst.GetCommandByID(gameServer.ID)
	if !errors.Is(errGetCmd, supervisor.ErrCommandDoesNotExist) {
		t.Fatalf("expected no supervised update command, got %v", errGetCmd)
	}
}

func TestUpdateGameServerFallsBackFromInvalidStoredVanillaTarget(t *testing.T) {
	ctx := context.Background()
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	serverDir := t.TempDir()
	provider := &minecraftUpdateTestProvider{
		providerID:      "test-mojang-provider",
		latestVersion:   "1.21.5",
		downloadVersion: "1.21.5",
		markerPath:      filepath.Join(serverDir, "minecraft_server.jar"),
	}
	withMinecraftUpdateProviderLookupForKinds(t, provider, updateproviders.ProviderKindMojang)

	inst := &Instance{
		ctx:                ctx,
		embeddedNodeClient: newSupervisorBackedNodeClient(ctx, t, supervisorInst, nil),
	}
	gameServer := &models.GameServer{
		ID:               "minecraft-vanilla-invalid-target",
		GameID:           "minecraft",
		Directory:        serverDir,
		UserID:           "user-1",
		ServerSoftware:   null.From("vanilla"),
		ServerExecutable: null.From("minecraft_server.jar"),
		Branch:           "26.1",
	}
	gameServer.R.Game = &models.Game{
		ID: "minecraft",
	}
	setMinecraftTypedVariants(t, gameServer.R.Game, updateproviders.Variant{
		ID:   "vanilla",
		Name: "Vanilla",
		UpdateProvider: &updateproviders.ProviderConfig{
			Kind:     updateproviders.ProviderKindMojang,
			SourceID: "vanilla",
		},
	})

	errUpdate := inst.UpdateGameServer(gameServer)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	if provider.versionsSourceID != "vanilla" {
		t.Errorf("GetVersions sourceID = %q, want %q", provider.versionsSourceID, "vanilla")
	}
	if provider.versionsGameVersion != "1.21.5" {
		t.Errorf("GetVersions gameVersion = %q, want %q", provider.versionsGameVersion, "1.21.5")
	}
	if provider.downloadSourceID != "vanilla" {
		t.Errorf("Download sourceID = %q, want %q", provider.downloadSourceID, "vanilla")
	}
	if provider.downloadVersionID != "1.21.5" {
		t.Errorf("Download versionID = %q, want %q", provider.downloadVersionID, "1.21.5")
	}
}

func TestRunUpdateWithBackupWritesProgressToConsoleBuffer(t *testing.T) {
	ctx := context.Background()
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	conn := dbtest.NewMigratedConnection(t, "update-console.sqlite")
	now := time.Now().UTC()
	_, errCreateUser := conn.CreateUser(&models.UserSetter{
		ID:           omit.From("user-1"),
		UserName:     omit.From("console-user"),
		Email:        omit.From("console@example.com"),
		FirstName:    omit.From("Console"),
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

	_, errInsertNode := conn.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-local"),
		Name:      omit.From("Local Node"),
		ListenURL: omit.From("http://localhost:8080"),
		Enabled:   omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode() error = %v", errInsertNode)
	}

	_, errUpsertIP := conn.UpsertIP(&models.IPSetter{
		Address:            omit.From("127.0.0.1"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
		NodeID:             omit.From("node-local"),
	})
	if errUpsertIP != nil {
		t.Fatalf("UpsertIP() error = %v", errUpsertIP)
	}

	serverDir := t.TempDir()
	markerPath := filepath.Join(serverDir, "updated.txt")
	gameID := "internal-update-console-test"
	gameintegrations.RegisterGame(gameID, internalUpdateTestGame{markerPath: markerPath})
	_, errInsertGame := conn.InsertGame(conn.DB, &models.GameSetter{
		ID:                omit.From(gameID),
		Name:              omit.From("Internal Update Test"),
		DefaultPort:       omit.From(int64(25565)),
		DefaultQueryPort:  omit.From(int64(25565)),
		DefaultMaxPlayers: omit.From(int64(20)),
	})
	if errInsertGame != nil {
		t.Fatalf("InsertGame() error = %v", errInsertGame)
	}

	inst := &Instance{
		ctx:                ctx,
		db:                 conn,
		versionState:       versiontracker.NewVersionStateMap(),
		embeddedNodeClient: newSupervisorBackedNodeClient(ctx, t, supervisorInst, conn),
	}
	gameServer := &models.GameServer{
		ID:        "server-console-progress",
		GameID:    gameID,
		Name:      "Console Progress Server",
		Directory: serverDir,
		UserID:    "user-1",
	}
	gameServer.R.Game = &models.Game{}
	_, errInsertServer := conn.InsertGameServer(conn.DB, &models.GameServerSetter{
		ID:               omit.From(gameServer.ID),
		UserID:           omit.From(gameServer.UserID),
		Name:             omit.From("Console Progress Server"),
		GameID:           omit.From(gameID),
		StartArgsPatches: omit.From("[]"),
		Status:           omit.From("OFFLINE"),
		SetPlayers:       omit.From(int64(20)),
		MaxPlayers:       omit.From(int64(20)),
		Map:              omit.From("world"),
		IP:               omit.From("127.0.0.1"),
		Port:             omit.From(int64(25565)),
		QueryPort:        omit.From(int64(25565)),
		Directory:        omit.From(serverDir),
		NodeID:           omit.From("node-local"),
		CreatedAt:        omit.From(now),
		UpdatedAt:        omit.From(now),
	})
	if errInsertServer != nil {
		t.Fatalf("InsertGameServer() error = %v", errInsertServer)
	}

	outChan := make(chan *xylona.Message, 32)
	shellCommand := supervisorInst.GetCommandByIDOrCreateShell(gameServer.ID)
	shellCommand.AddOutputListener("test-progress", outChan)
	defer shellCommand.RemoveOutputListener("test-progress")

	broadcaster := &recordingUpdateProgressBroadcaster{}
	inst.runUpdateWithBackup(gameServer, broadcaster)

	var streamedOutput strings.Builder
	for {
		select {
		case msg := <-outChan:
			if msg != nil && msg.GetGameServerConsoleOutput() != nil {
				streamedOutput.WriteString(msg.GetGameServerConsoleOutput().GetOutput())
			}
		default:
			goto assertOutput
		}
	}

assertOutput:
	output := streamedOutput.String()
	for _, expected := range []string{
		"Backing up files",
		"Downloading update",
		"Installing update",
		"Update complete",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("output buffer missing %q in %q", expected, output)
		}
	}
	for _, unexpected := range []string{"Stopping server", "Restarting server", "Server restarted"} {
		if strings.Contains(output, unexpected) {
			t.Errorf("output buffer unexpectedly contained %q in %q", unexpected, output)
		}
	}
	for _, unexpectedStep := range []xylona.UpdateStep{
		xylona.UpdateStep_UPDATE_STEP_STOPPING,
		xylona.UpdateStep_UPDATE_STEP_RESTARTING,
	} {
		if broadcaster.ContainsStep(unexpectedStep) {
			t.Errorf("unexpected progress step %v recorded for offline update", unexpectedStep)
		}
	}
	if !broadcaster.AllServerNamesMatch(gameServer.Name) {
		t.Errorf("progress server names = %v, want all %q", broadcaster.ServerNames(), gameServer.Name)
	}
}

func TestRunUpdateWithBackupIncludesMinecraftUpdateDetails(t *testing.T) {
	ctx := context.Background()
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	serverDir := t.TempDir()
	provider := &minecraftUpdateTestProvider{
		providerID:      "test-minecraft-update-provider-detailed",
		latestVersion:   "1.21.5",
		downloadVersion: "1.21.5-build-9",
		markerPath:      filepath.Join(serverDir, "paper-1.21.5-9.jar"),
	}
	withMinecraftUpdateProviderLookup(t, provider)

	inst := &Instance{
		ctx:                ctx,
		embeddedNodeClient: newSupervisorBackedNodeClient(ctx, t, supervisorInst, nil),
	}
	gameServer := &models.GameServer{
		ID:               "minecraft-detailed-update",
		GameID:           "minecraft",
		Name:             "Minecraft Detailed Update",
		Directory:        serverDir,
		UserID:           "user-1",
		ServerSoftware:   null.From("paper"),
		ServerExecutable: null.From("paper-1.21.4.jar"),
	}
	gameServer.R.Game = &models.Game{
		ID: "minecraft",
	}
	setMinecraftTypedVariants(t, gameServer.R.Game, updateproviders.Variant{
		ID:            "paper",
		Name:          "Paper",
		DefaultTarget: "1.21.5",
		UpdateProvider: &updateproviders.ProviderConfig{
			Kind:     updateproviders.ProviderKindPaperMC,
			SourceID: "paper",
		},
	})

	outChan := make(chan *xylona.Message, 32)
	shellCommand := supervisorInst.GetCommandByIDOrCreateShell(gameServer.ID)
	shellCommand.AddOutputListener("test-progress", outChan)
	defer shellCommand.RemoveOutputListener("test-progress")

	broadcaster := &recordingUpdateProgressBroadcaster{}
	inst.runUpdateWithBackup(gameServer, broadcaster)

	var streamedOutput strings.Builder
	for {
		select {
		case msg := <-outChan:
			if msg != nil && msg.GetGameServerConsoleOutput() != nil {
				streamedOutput.WriteString(msg.GetGameServerConsoleOutput().GetOutput())
			}
		default:
			goto assertDetailedOutput
		}
	}

assertDetailedOutput:
	output := streamedOutput.String()
	for _, expected := range []string{
		"Downloading Paper for Minecraft 1.21.5",
		"paper-1.21.5-9.jar",
		"Applying paper-1.21.5-9.jar",
		"Installed Paper 1.21.5 with paper-1.21.5-9.jar",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("output buffer missing %q in %q", expected, output)
		}
	}
	if !broadcaster.AllServerNamesMatch(gameServer.Name) {
		t.Errorf("progress server names = %v, want all %q", broadcaster.ServerNames(), gameServer.Name)
	}
}

func TestRunUpdateWithBackupFailsClosedWhenRuntimeStatusUnavailable(t *testing.T) {
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                "node-remote",
		GetProcessSnapshotErr: errors.New("snapshot unavailable"),
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(remoteClient)
	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:     "server-remote-1",
		Name:   "Remote Server",
		NodeID: "node-remote",
	}
	broadcaster := &recordingUpdateProgressBroadcaster{}

	inst.runUpdateWithBackup(gameServer, broadcaster)

	if len(broadcaster.events) != 1 {
		t.Fatalf("update progress event count = %d, want 1", len(broadcaster.events))
	}
	event := broadcaster.events[0]
	if event.step != xylona.UpdateStep_UPDATE_STEP_STOPPING || event.stepStatus != xylona.StepStatus_STEP_STATUS_FAILED {
		t.Fatalf("update progress = (%v, %v), want stopping failed", event.step, event.stepStatus)
	}
	if len(remoteClient.StopProcessCalls) != 0 {
		t.Fatalf("StopProcess call count = %d, want 0 while runtime status is unavailable", len(remoteClient.StopProcessCalls))
	}
	if len(remoteClient.StartProcessCalls) != 0 {
		t.Fatalf("StartProcess call count = %d, want 0 while runtime status is unavailable", len(remoteClient.StartProcessCalls))
	}
}

func TestRunUpdateWithBackupStopsBeforeBackupWhenStopFails(t *testing.T) {
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                  "node-remote",
		SnapshotResult:          &node.NodeSnapshot{OS: "linux"},
		GetProcessSnapshotFound: true,
		GetProcessSnapshotResult: &node.ProcessSnapshot{
			ID:     "server-remote-1",
			Status: xylona.Status_ONLINE.String(),
		},
		StopProcessErr: errors.New("remote stop failed"),
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(remoteClient)
	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:     "server-remote-1",
		Name:   "Remote Server",
		NodeID: "node-remote",
	}
	gameServer.R.Game = &models.Game{}
	broadcaster := &recordingUpdateProgressBroadcaster{}

	inst.runUpdateWithBackup(gameServer, broadcaster)

	if len(broadcaster.events) != 2 {
		t.Fatalf("update progress event count = %d, want 2", len(broadcaster.events))
	}
	failedEvent := broadcaster.events[1]
	if failedEvent.step != xylona.UpdateStep_UPDATE_STEP_STOPPING || failedEvent.stepStatus != xylona.StepStatus_STEP_STATUS_FAILED {
		t.Fatalf("final update progress = (%v, %v), want stopping failed", failedEvent.step, failedEvent.stepStatus)
	}
	if len(remoteClient.CreateBackupArchiveCalls) != 0 {
		t.Fatalf("CreateBackupArchive call count = %d, want 0 after stop failure", len(remoteClient.CreateBackupArchiveCalls))
	}
	if len(remoteClient.StartProcessCalls) != 0 {
		t.Fatalf("StartProcess call count = %d, want 0 after stop failure", len(remoteClient.StartProcessCalls))
	}
}

func TestSteamCMDUpdateMessagesUseSteamCMDSpecificWording(t *testing.T) {
	gameServer := &models.GameServer{
		Branch: "latest_experimental",
	}
	gameServer.R.Game = &models.Game{
		UsesSteamcmd: true,
	}

	if got := downloadStartMessage(gameServer, nil); got != "Preparing SteamCMD update" {
		t.Fatalf("downloadStartMessage() = %q, want %q", got, "Preparing SteamCMD update")
	}
	if got := downloadCompleteMessage(gameServer, nil); got != "SteamCMD session ready" {
		t.Fatalf("downloadCompleteMessage() = %q, want %q", got, "SteamCMD session ready")
	}
	if got := installStartMessage(gameServer, nil); got != "Running SteamCMD update for branch latest_experimental" {
		t.Fatalf(
			"installStartMessage() = %q, want %q",
			got,
			"Running SteamCMD update for branch latest_experimental",
		)
	}
	if got := installCompleteMessage(gameServer, nil, false); got != "SteamCMD update complete" {
		t.Fatalf("installCompleteMessage() = %q, want %q", got, "SteamCMD update complete")
	}
}

type recordedUpdateProgress struct {
	serverName string
	step       xylona.UpdateStep
	stepStatus xylona.StepStatus
	message    string
}

type recordingUpdateProgressBroadcaster struct {
	events []recordedUpdateProgress
}

func (b *recordingUpdateProgressBroadcaster) BroadcastUpdateProgress(
	_ string,
	serverName string,
	step xylona.UpdateStep,
	stepStatus xylona.StepStatus,
	message string,
) {
	b.events = append(b.events, recordedUpdateProgress{
		serverName: serverName,
		step:       step,
		stepStatus: stepStatus,
		message:    message,
	})
}

func (b *recordingUpdateProgressBroadcaster) ContainsStep(step xylona.UpdateStep) bool {
	for _, event := range b.events {
		if event.step == step {
			return true
		}
	}
	return false
}

func (b *recordingUpdateProgressBroadcaster) ServerNames() []string {
	names := make([]string, 0, len(b.events))
	for _, event := range b.events {
		names = append(names, event.serverName)
	}
	return names
}

func (b *recordingUpdateProgressBroadcaster) AllServerNamesMatch(name string) bool {
	for _, event := range b.events {
		if event.serverName != name {
			return false
		}
	}
	return true
}

type internalUpdateTestGame struct {
	markerPath string
}

func (g internalUpdateTestGame) Install(_ *models.GameServer, _, _ io.Writer) error {
	return nil
}

func (g internalUpdateTestGame) Update(_ *models.GameServer, _, _ io.Writer) error {
	errWrite := os.WriteFile(g.markerPath, []byte("updated"), 0o600)
	if errWrite != nil {
		return fmt.Errorf("write update marker: %w", errWrite)
	}
	return nil
}

type minecraftUpdateTestProvider struct {
	providerID          string
	latestVersion       string
	downloadVersion     string
	downloadURL         string
	downloadSize        int64
	downloadSHA256      string
	downloadSHA1        string
	markerPath          string
	detailsSourceID     string
	versionsSourceID    string
	versionsGameVersion string
	downloadSourceID    string
	downloadVersionID   string
	downloadCalls       int
}

func (p *minecraftUpdateTestProvider) ID() string {
	if p.providerID != "" {
		return p.providerID
	}
	return "test-minecraft-update-provider"
}

func withMinecraftUpdateProviderLookup(t *testing.T, provider modproviders.ModProvider) {
	t.Helper()

	previousLookup := minecraftUpdateProviderLookup
	minecraftUpdateProviderLookup = func(kind updateproviders.ProviderKind) (modproviders.ModProvider, bool) {
		if kind == updateproviders.ProviderKindPaperMC {
			return provider, true
		}
		return previousLookup(kind)
	}
	t.Cleanup(func() {
		minecraftUpdateProviderLookup = previousLookup
	})
}

func withMinecraftUpdateProviderLookupForKinds(
	t *testing.T,
	provider modproviders.ModProvider,
	kinds ...updateproviders.ProviderKind,
) {
	t.Helper()

	kindSet := make(map[updateproviders.ProviderKind]struct{}, len(kinds))
	for _, kind := range kinds {
		kindSet[kind] = struct{}{}
	}

	previousLookup := minecraftUpdateProviderLookup
	minecraftUpdateProviderLookup = func(kind updateproviders.ProviderKind) (modproviders.ModProvider, bool) {
		if _, ok := kindSet[kind]; ok {
			return provider, true
		}
		return previousLookup(kind)
	}
	t.Cleanup(func() {
		minecraftUpdateProviderLookup = previousLookup
	})
}

func setMinecraftTypedVariants(t *testing.T, game *models.Game, variants ...updateproviders.Variant) {
	t.Helper()

	errSave := updateconfig.SaveGameConfigToModel(game, updateproviders.GameConfig{
		UpdateProvider: updateproviders.ProviderConfig{Kind: updateproviders.ProviderKindCommand},
		Variants:       variants,
	})
	if errSave != nil {
		t.Fatalf("SaveGameConfigToModel() error = %v", errSave)
	}
}

func (p *minecraftUpdateTestProvider) Search(_ context.Context, _ string, _ modproviders.SearchParams) (modproviders.SearchResult, error) {
	return modproviders.SearchResult{}, nil
}

func (p *minecraftUpdateTestProvider) GetModDetails(_ context.Context, sourceID string, _ modproviders.SearchParams) (*modproviders.ModDetails, error) {
	p.detailsSourceID = sourceID
	return &modproviders.ModDetails{
		SourceID: sourceID,
		Versions: []modproviders.ModVersion{
			{VersionID: p.latestVersion, VersionString: p.latestVersion},
		},
	}, nil
}

func (p *minecraftUpdateTestProvider) GetVersions(_ context.Context, sourceID string, gameVersion string, _ modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	p.versionsSourceID = sourceID
	p.versionsGameVersion = gameVersion
	return []modproviders.ModVersion{
		{
			VersionID:      p.downloadVersion,
			VersionString:  "Build 9",
			DownloadURL:    p.downloadURL,
			FileSize:       p.downloadSize,
			FileHashSHA256: p.downloadSHA256,
			FileHashSHA1:   p.downloadSHA1,
		},
	}, nil
}

func (p *minecraftUpdateTestProvider) Download(_ context.Context, sourceID string, versionID string, targetDir string) ([]modproviders.DownloadedFile, error) {
	p.downloadCalls++
	p.downloadSourceID = sourceID
	p.downloadVersionID = versionID
	relativePath := filepath.Base(p.markerPath)
	fullPath := filepath.Join(targetDir, relativePath)
	if errWrite := os.WriteFile(fullPath, []byte("updated"), 0o600); errWrite != nil {
		return nil, fmt.Errorf("write downloaded file: %w", errWrite)
	}
	return []modproviders.DownloadedFile{
		{Path: relativePath, IsPrimary: true},
	}, nil
}

func (p *minecraftUpdateTestProvider) CheckForUpdate(_ context.Context, _ string, _ string) (*modproviders.ModVersion, error) {
	return &modproviders.ModVersion{}, nil
}

func (p *minecraftUpdateTestProvider) RequiresAPIKey() bool {
	return false
}
