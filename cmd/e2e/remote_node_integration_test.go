//go:build local_integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestRemoteNodeControllerBoundary(t *testing.T) {
	projectRoot, errAbs := filepath.Abs(filepath.Join("..", ".."))
	if errAbs != nil {
		t.Fatalf("resolve project root: %v", errAbs)
	}

	e2eDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)

	cfg := setupConfig{
		mode:          e2eModeRemoteNode,
		httpPort:      9391,
		nodePort:      9591,
		adminUsername: "remote-admin",
		adminPassword: "RemoteAdmin123!",
		e2eDir:        e2eDir,
		projectRoot:   projectRoot,
	}

	state, errSetup := runSetup(ctx, cfg)
	if errSetup != nil {
		t.Fatalf("runSetup() error = %v", errSetup)
	}
	t.Cleanup(func() {
		runTeardown(e2eDir, e2eModeRemoteNode)
	})

	if state.RemoteNodeID == "" {
		t.Fatal("remote node ID is empty")
	}
	if state.TargetNodeID != state.RemoteNodeID {
		t.Fatalf("target node ID = %q, want remote node %q", state.TargetNodeID, state.RemoteNodeID)
	}
	if !strings.Contains(filepath.Clean(state.GameServerDir), filepath.Clean(state.NodeHomeDir)) {
		t.Fatalf("game server dir = %q, want under remote node home %q", state.GameServerDir, state.NodeHomeDir)
	}

	client, errLogin := newAuthenticatedClient(ctx, state.BackendURL, cfg.adminUsername, cfg.adminPassword)
	if errLogin != nil {
		t.Fatalf("login after setup: %v", errLogin)
	}

	assertOutputContains(t, ctx, client, state.GameServerID, "parent-pid="+strconv.Itoa(state.RemoteNodePID))
	createRemoteFile(t, ctx, client, state.GameServerID, "remote-node-proof.txt", "created through controller RPC\n")
	assertFileExists(t, filepath.Join(state.GameServerDir, "remote-node-proof.txt"))
	assertBackupsSupported(t, ctx, client, state.GameServerID)

	backupResp, errBackup := client.rpc.CreateGameServerBackup(ctx, connect.NewRequest(&xylona.CreateGameServerBackupRequest{
		GameServerId: state.GameServerID,
		BackupName:   "remote-node-proof.zip",
	}))
	if errBackup != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errBackup)
	}
	if backupResp.Msg.GetBackup() == nil {
		t.Fatal("CreateGameServerBackup() returned no backup")
	}
	assertFileExistsEventually(t, backupResp.Msg.GetBackup().GetArchivePath(), 20*time.Second)

	restartController(t, ctx, cfg, state)

	restartedClient, errRestartLogin := newAuthenticatedClient(ctx, state.BackendURL, cfg.adminUsername, cfg.adminPassword)
	if errRestartLogin != nil {
		t.Fatalf("login after controller restart: %v", errRestartLogin)
	}
	createRemoteFile(t, ctx, restartedClient, state.GameServerID, "after-restart-proof.txt", "remote node still reachable\n")
	assertFileExists(t, filepath.Join(state.GameServerDir, "after-restart-proof.txt"))
}

func assertBackupsSupported(t *testing.T, ctx context.Context, client *e2eClient, serverID string) {
	t.Helper()

	response, errSettings := client.rpc.GetBackupSettings(ctx, connect.NewRequest(&xylona.GetBackupSettingsRequest{
		GameServerId: serverID,
	}))
	if errSettings != nil {
		t.Fatalf("GetBackupSettings() error = %v", errSettings)
	}
	settings := response.Msg.GetSettings()
	if settings == nil {
		t.Fatal("GetBackupSettings() returned no settings")
	}
	if !settings.GetBackupsSupported() {
		t.Fatalf("GetBackupSettings().BackupsSupported = false: %s", settings.GetDisabledReason())
	}
}

func assertOutputContains(t *testing.T, ctx context.Context, client *e2eClient, serverID string, needle string) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, errRead := client.rpc.ReadGameServerOutput(ctx, connect.NewRequest(&xylona.ReadGameServerOutputRequest{
			ServerId: serverID,
		}))
		if errRead == nil && strings.Contains(resp.Msg.GetOutput(), needle) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("game server output did not contain %q", needle)
}

func createRemoteFile(t *testing.T, ctx context.Context, client *e2eClient, serverID string, path string, content string) {
	t.Helper()

	_, errCreate := client.rpc.GameServersFileOrDirectoryCreate(ctx, connect.NewRequest(&xylona.GameServerFileOrDirectoryCreateRequest{
		GameServerId: serverID,
		FullFilePath: path,
		Content:      content,
	}))
	if errCreate != nil {
		t.Fatalf("GameServersFileOrDirectoryCreate(%q) error = %v", path, errCreate)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()

	_, errStat := os.Stat(path)
	if errStat != nil {
		t.Fatalf("expected file %q to exist: %v", path, errStat)
	}
}

func assertFileExistsEventually(t *testing.T, path string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, errStat := os.Stat(path)
		if errStat == nil {
			return
		}
		if !os.IsNotExist(errStat) {
			t.Fatalf("expected file %q to exist: %v", path, errStat)
		}
		time.Sleep(250 * time.Millisecond)
	}

	_, errStat := os.Stat(path)
	if errStat != nil {
		t.Fatalf("expected file %q to exist within %s: %v", path, timeout, errStat)
	}
}

func restartController(t *testing.T, ctx context.Context, cfg setupConfig, state *testState) {
	t.Helper()

	killByPIDFile(filepath.Join(state.ControllerDir, "xylona.pid"), "controller")
	time.Sleep(1 * time.Second)

	xylonaExe := filepath.Join(state.DataDir, "bin", binaryName("xylona"))
	extraEnv := []string{
		"DUMMY_GAME_ID=e2e-test-game",
		"XYLONA_VERSION_CHECK_INTERVAL=30s",
		"HOME=" + state.ControllerHomeDir,
		"USERPROFILE=" + state.ControllerHomeDir,
		"XYLONA_E2E_NODE_MARKER=controller",
	}
	cmd, errStart := startNode("Controller-Restart", state.ControllerDir, state.DataDir, xylonaExe, cfg.httpPort, state.RuntimeEnv, extraEnv...)
	if errStart != nil {
		t.Fatalf("restart controller: %v", errStart)
	}
	t.Cleanup(func() {
		killProcess(cmd)
	})

	errPID := writePIDFile(filepath.Join(state.ControllerDir, "xylona.pid"), cmd)
	if errPID != nil {
		t.Fatalf("write restarted controller PID: %v", errPID)
	}

	errReady := waitForReady(ctx, state.BackendURL+"/api/ready", 30*time.Second)
	if errReady != nil {
		t.Fatalf("wait for restarted controller: %v", errReady)
	}
}
