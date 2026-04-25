package modmanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/db/dbtest"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
)

// mockProvider implements modproviders.ModProvider for testing.
type mockProvider struct {
	id                    string
	searchResults         []modproviders.ModSearchResult
	searchTotalHits       int
	details               *modproviders.ModDetails
	versions              []modproviders.ModVersion
	downloadedFiles       []modproviders.DownloadedFile
	downloadErrAfterWrite error
	updateVersion         *modproviders.ModVersion
	searchErr             error
	detailsErr            error
	downloadErr           error
	checkForUpdateErr     error
}

func (m *mockProvider) ID() string { return m.id }

func (m *mockProvider) Search(_ context.Context, _ string, _ modproviders.SearchParams) (modproviders.SearchResult, error) {
	totalHits := m.searchTotalHits
	if totalHits == 0 && len(m.searchResults) > 0 {
		totalHits = len(m.searchResults)
	}
	return modproviders.SearchResult{
		Results:   m.searchResults,
		TotalHits: totalHits,
	}, m.searchErr
}

func (m *mockProvider) GetModDetails(_ context.Context, _ string, _ modproviders.SearchParams) (*modproviders.ModDetails, error) {
	return m.details, m.detailsErr
}

func (m *mockProvider) GetVersions(_ context.Context, _ string, _ string, _ modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	return m.versions, nil
}

func (m *mockProvider) Download(_ context.Context, _ string, _ string, targetDir string) ([]modproviders.DownloadedFile, error) {
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	// Write actual files to disk for tests that verify filesystem operations.
	for _, f := range m.downloadedFiles {
		fullPath := filepath.Join(targetDir, f.Path)
		errMkdir := os.MkdirAll(filepath.Dir(fullPath), 0o750)
		if errMkdir != nil {
			return nil, fmt.Errorf("mock provider: create directory: %w", errMkdir)
		}
		errWrite := os.WriteFile(fullPath, []byte("mock-content"), 0o600)
		if errWrite != nil {
			return nil, fmt.Errorf("mock provider: write file: %w", errWrite)
		}
	}
	if m.downloadErrAfterWrite != nil {
		return nil, m.downloadErrAfterWrite
	}
	return m.downloadedFiles, nil
}

func (m *mockProvider) CheckForUpdate(_ context.Context, _ string, _ string) (*modproviders.ModVersion, error) {
	return m.updateVersion, m.checkForUpdateErr
}

func (m *mockProvider) RequiresAPIKey() bool { return false }

// providerCounter ensures each test gets a unique provider ID.
var providerCounter atomic.Int64

func newUniqueProviderID() string {
	n := providerCounter.Add(1)
	return fmt.Sprintf("test-provider-%d", n)
}

func seedTestFixture(t *testing.T, conn *db.Connection) {
	t.Helper()
	ctx := context.Background()

	_, errNode := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO node (id, name, listen_url, enabled) VALUES (?, ?, ?, ?)`,
		"node-local", "Local Node", "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("failed to insert node: %v", errNode)
	}

	_, errSettings := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO local_settings (id, node_id) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET node_id = excluded.node_id`,
		"node-local",
	)
	if errSettings != nil {
		t.Fatalf("failed to insert local settings: %v", errSettings)
	}

	_, errIP := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO ip (address, node_id, usable, external) VALUES (?, ?, ?, ?)`,
		"127.0.0.1", "node-local", true, false,
	)
	if errIP != nil {
		t.Fatalf("failed to insert ip: %v", errIP)
	}

	_, errUser := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), datetime('now'))`,
		"user-owner", "owner", "owner@test.com", "Test", "Owner", "hash", false,
	)
	if errUser != nil {
		t.Fatalf("failed to insert user: %v", errUser)
	}

	_, errServer := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-1", "user-owner", "Test Server", "minecraft", "OFFLINE",
		20, 20, "world", "127.0.0.1", 25565, 25565, "/tmp/server-1", "node-local", "[]",
	)
	if errServer != nil {
		t.Fatalf("failed to insert game server: %v", errServer)
	}
}

func newTestModFileClient(t *testing.T, conn *db.Connection) FileClient {
	t.Helper()
	return nodeclient.NewInProcessClient("node-local", node.New(t.Context(), nil, conn))
}

func newMockProvider(providerID string) *mockProvider {
	return &mockProvider{
		id: providerID,
		details: &modproviders.ModDetails{
			Source:   providerID,
			SourceID: "mod-src-1",
			Name:     "Test Mod",
			Author:   "Test Author",
			Versions: []modproviders.ModVersion{
				{
					VersionID:      "v1",
					VersionString:  "1.0.0",
					DownloadURL:    "https://example.test/testmod-1.0.0.jar",
					FileSize:       1024,
					FileHashSHA256: "sha-v1",
				},
				{
					VersionID:      "v2",
					VersionString:  "2.0.0",
					DownloadURL:    "https://example.test/testmod-2.0.0.jar",
					FileSize:       2048,
					FileHashSHA256: "sha-v2",
				},
			},
		},
		downloadedFiles: []modproviders.DownloadedFile{
			{Path: "testmod-1.0.0.jar", Hash: "abc123", Size: 1024, IsPrimary: true},
		},
	}
}

func clearVersionIntegrity(version *modproviders.ModVersion) {
	version.FileSize = 0
	version.FileHashSHA256 = ""
	version.FileHashSHA1 = ""
}

func resetFakeNodeClientCalls(client *nodeclient.FakeNodeClient) {
	client.CreateFileOrDirectoryCalls = nil
	client.DeleteFilesCalls = nil
	client.DownloadFileFromURLCalls = nil
	client.MoveFilesCalls = nil
	client.RenameFileCalls = nil
}

func assertSlashPath(t *testing.T, got string) {
	t.Helper()
	if strings.Contains(got, `\`) {
		t.Fatalf("path %q contains backslash, want slash-style relative path", got)
	}
	if !path.IsAbs(got) && filepath.IsAbs(got) {
		t.Fatalf("path %q is platform absolute, want relative path", got)
	}
}

func TestRemoteModDownloadRejectsMissingIntegrityMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Run("install", func(t *testing.T) {
		conn := dbtest.NewMigratedConnection(t, "mm-remote-install-missing-integrity.sqlite")
		seedTestFixture(t, conn)

		pid := newUniqueProviderID()
		mock := newMockProvider(pid)
		clearVersionIntegrity(&mock.details.Versions[0])
		modproviders.RegisterProvider(mock)

		mgr := New(conn)
		client := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
		_, errInstall := mgr.InstallRemote(context.Background(), client, "server-1", pid, "mod-src-1", "v1", t.TempDir(), "mods")
		if !errors.Is(errInstall, modproviders.ErrMissingIntegrityMetadata) {
			t.Fatalf("InstallRemote() error = %v, want %v", errInstall, modproviders.ErrMissingIntegrityMetadata)
		}
		if len(client.DownloadFileFromURLCalls) != 0 {
			t.Fatalf("DownloadFileFromURL call count = %d, want 0", len(client.DownloadFileFromURLCalls))
		}
	})

	t.Run("update", func(t *testing.T) {
		conn := dbtest.NewMigratedConnection(t, "mm-remote-update-missing-integrity.sqlite")
		seedTestFixture(t, conn)

		pid := newUniqueProviderID()
		mock := newMockProvider(pid)
		modproviders.RegisterProvider(mock)

		mgr := New(conn)
		client := &nodeclient.FakeNodeClient{
			NodeID:                    "node-remote",
			DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "mods/.xylona-download-install/testmod-1.0.0.jar", BytesWritten: 1024, SHA256: "sha-v1"},
			RenameFileResult:          "mods/testmod-1.0.0.jar",
		}
		mod, errInstall := mgr.InstallRemote(context.Background(), client, "server-1", pid, "mod-src-1", "v1", t.TempDir(), "mods")
		if errInstall != nil {
			t.Fatalf("InstallRemote() error = %v", errInstall)
		}

		clearVersionIntegrity(&mock.details.Versions[1])
		resetFakeNodeClientCalls(client)
		_, errUpdate := mgr.UpdateRemote(context.Background(), client, mod.ID, "v2", t.TempDir())
		if !errors.Is(errUpdate, modproviders.ErrMissingIntegrityMetadata) {
			t.Fatalf("UpdateRemote() error = %v, want %v", errUpdate, modproviders.ErrMissingIntegrityMetadata)
		}
		if len(client.DownloadFileFromURLCalls) != 0 {
			t.Fatalf("DownloadFileFromURL call count = %d, want 0", len(client.DownloadFileFromURLCalls))
		}
	})
}

func TestRemoteModInstallUsesNodeDownloadAndStoresSlashPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-remote-install.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()
	client := &nodeclient.FakeNodeClient{
		NodeID:                    "node-remote",
		DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "mods/.xylona-download-remote/testmod-1.0.0.jar", BytesWritten: 1024, SHA256: "sha-v1"},
		RenameFileResult:          "mods/testmod-1.0.0.jar",
	}

	mod, errInstall := mgr.InstallRemote(context.Background(), client, "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("InstallRemote() error = %v", errInstall)
	}
	if mod.InstalledVersionID != "v1" {
		t.Fatalf("InstallRemote().InstalledVersionID = %q, want %q", mod.InstalledVersionID, "v1")
	}
	if mod.FileHash != "sha-v1" {
		t.Fatalf("InstallRemote().FileHash = %q, want sha-v1", mod.FileHash)
	}
	if len(client.DownloadFileFromURLCalls) != 1 {
		t.Fatalf("DownloadFileFromURL call count = %d, want 1", len(client.DownloadFileFromURLCalls))
	}
	downloadCall := client.DownloadFileFromURLCalls[0]
	if downloadCall.Directory != serverDir {
		t.Fatalf("DownloadFileFromURL directory = %q, want %q", downloadCall.Directory, serverDir)
	}
	if downloadCall.RawURL != "https://example.test/testmod-1.0.0.jar" {
		t.Fatalf("DownloadFileFromURL raw URL = %q", downloadCall.RawURL)
	}
	if !strings.HasPrefix(downloadCall.DestinationDirectoryPath, "mods/.xylona-download-") {
		t.Fatalf("DownloadFileFromURL destination = %q, want node staging under mods", downloadCall.DestinationDirectoryPath)
	}
	if downloadCall.Integrity.ExpectedSize != 1024 || downloadCall.Integrity.ExpectedSHA256 != "sha-v1" {
		t.Fatalf("DownloadFileFromURL integrity = %+v, want size=1024 sha-v1", downloadCall.Integrity)
	}
	if len(client.RenameFileCalls) != 1 {
		t.Fatalf("RenameFile call count = %d, want 1", len(client.RenameFileCalls))
	}
	if client.RenameFileCalls[0].OldRelativePath != "mods/.xylona-download-remote/testmod-1.0.0.jar" {
		t.Fatalf("RenameFile old path = %q", client.RenameFileCalls[0].OldRelativePath)
	}
	if client.RenameFileCalls[0].NewRelativePath != "mods/testmod-1.0.0.jar" {
		t.Fatalf("RenameFile new path = %q", client.RenameFileCalls[0].NewRelativePath)
	}

	files, errFiles := conn.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		t.Fatalf("GetInstalledModFilesByModID() error = %v", errFiles)
	}
	if len(files) != 1 {
		t.Fatalf("GetInstalledModFilesByModID() len = %d, want 1", len(files))
	}
	assertSlashPath(t, files[0].FilePath)
	if files[0].FilePath != "mods/testmod-1.0.0.jar" {
		t.Fatalf("FilePath = %q, want %q", files[0].FilePath, "mods/testmod-1.0.0.jar")
	}
	if files[0].FileHash != "sha-v1" || files[0].FileSize != 1024 {
		t.Fatalf("installed file integrity = hash %q size %d, want sha-v1/1024", files[0].FileHash, files[0].FileSize)
	}
	entries, errReadDir := os.ReadDir(serverDir)
	if errReadDir != nil {
		t.Fatalf("ReadDir(serverDir) error = %v", errReadDir)
	}
	if len(entries) != 0 {
		t.Fatalf("InstallRemote() touched controller server dir, entries = %d", len(entries))
	}
}

func TestRemoteModUpdateUsesNodeRollbackAndStoresSlashPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-remote-update.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()
	client := &nodeclient.FakeNodeClient{
		NodeID:                    "node-remote",
		DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "mods/.xylona-download-install/testmod-1.0.0.jar", BytesWritten: 1024, SHA256: "sha-v1"},
		RenameFileResult:          "mods/testmod-1.0.0.jar",
	}

	mod, errInstall := mgr.InstallRemote(context.Background(), client, "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("InstallRemote() error = %v", errInstall)
	}
	resetFakeNodeClientCalls(client)
	client.DownloadFileFromURLResult = node.DownloadFileResult{RelativePath: "mods/.xylona-download-update/testmod-2.0.0.jar", BytesWritten: 2048, SHA256: "sha-v2"}

	updated, errUpdate := mgr.UpdateRemote(context.Background(), client, mod.ID, "v2", serverDir)
	if errUpdate != nil {
		t.Fatalf("UpdateRemote() error = %v", errUpdate)
	}
	if updated.InstalledVersionID != "v2" {
		t.Fatalf("UpdateRemote().InstalledVersionID = %q, want %q", updated.InstalledVersionID, "v2")
	}
	if updated.FileHash != "sha-v2" {
		t.Fatalf("UpdateRemote().FileHash = %q, want sha-v2", updated.FileHash)
	}
	if len(client.DownloadFileFromURLCalls) != 1 {
		t.Fatalf("DownloadFileFromURL call count = %d, want 1", len(client.DownloadFileFromURLCalls))
	}
	if client.DownloadFileFromURLCalls[0].RawURL != "https://example.test/testmod-2.0.0.jar" {
		t.Fatalf("DownloadFileFromURL raw URL = %q", client.DownloadFileFromURLCalls[0].RawURL)
	}
	if client.DownloadFileFromURLCalls[0].Integrity.ExpectedSize != 2048 || client.DownloadFileFromURLCalls[0].Integrity.ExpectedSHA256 != "sha-v2" {
		t.Fatalf("DownloadFileFromURL integrity = %+v, want size=2048 sha-v2", client.DownloadFileFromURLCalls[0].Integrity)
	}
	if len(client.RenameFileCalls) < 2 {
		t.Fatalf("RenameFile call count = %d, want at least 2", len(client.RenameFileCalls))
	}
	if client.RenameFileCalls[0].OldRelativePath != "mods/testmod-1.0.0.jar" {
		t.Fatalf("rollback RenameFile old path = %q", client.RenameFileCalls[0].OldRelativePath)
	}
	if !strings.HasPrefix(client.RenameFileCalls[0].NewRelativePath, "mods/.xylona-rollback-") {
		t.Fatalf("rollback RenameFile new path = %q", client.RenameFileCalls[0].NewRelativePath)
	}
	lastRename := client.RenameFileCalls[len(client.RenameFileCalls)-1]
	if lastRename.OldRelativePath != "mods/.xylona-download-update/testmod-2.0.0.jar" {
		t.Fatalf("promote RenameFile old path = %q", lastRename.OldRelativePath)
	}
	if lastRename.NewRelativePath != "mods/testmod-2.0.0.jar" {
		t.Fatalf("promote RenameFile new path = %q", lastRename.NewRelativePath)
	}

	files, errFiles := conn.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		t.Fatalf("GetInstalledModFilesByModID() error = %v", errFiles)
	}
	if len(files) != 1 {
		t.Fatalf("GetInstalledModFilesByModID() len = %d, want 1", len(files))
	}
	assertSlashPath(t, files[0].FilePath)
	if files[0].FilePath != "mods/testmod-2.0.0.jar" {
		t.Fatalf("FilePath = %q, want %q", files[0].FilePath, "mods/testmod-2.0.0.jar")
	}
	if files[0].FileHash != "sha-v2" || files[0].FileSize != 2048 {
		t.Fatalf("updated file integrity = hash %q size %d, want sha-v2/2048", files[0].FileHash, files[0].FileSize)
	}
}

func TestRemoteModUninstallDeletesOnNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-remote-uninstall.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()
	client := &nodeclient.FakeNodeClient{
		NodeID:                    "node-remote",
		DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "mods/.xylona-download-install/testmod-1.0.0.jar"},
		RenameFileResult:          "mods/testmod-1.0.0.jar",
	}

	mod, errInstall := mgr.InstallRemote(context.Background(), client, "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("InstallRemote() error = %v", errInstall)
	}
	resetFakeNodeClientCalls(client)

	errUninstall := mgr.Uninstall(context.Background(), client, mod.ID, serverDir)
	if errUninstall != nil {
		t.Fatalf("Uninstall() error = %v", errUninstall)
	}
	if len(client.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(client.DeleteFilesCalls))
	}
	if client.DeleteFilesCalls[0].Directory != serverDir {
		t.Fatalf("DeleteFiles directory = %q, want %q", client.DeleteFilesCalls[0].Directory, serverDir)
	}
	if !slices.Equal(client.DeleteFilesCalls[0].Files, []string{"mods/testmod-1.0.0.jar"}) {
		t.Fatalf("DeleteFiles files = %v, want [mods/testmod-1.0.0.jar]", client.DeleteFilesCalls[0].Files)
	}

	files, errFiles := conn.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		t.Fatalf("GetInstalledModFilesByModID() error = %v", errFiles)
	}
	if len(files) != 0 {
		t.Fatalf("GetInstalledModFilesByModID() len = %d, want 0", len(files))
	}
}

func TestRemoteModEnableDisableMovesOnNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-remote-enable-disable.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()
	client := &nodeclient.FakeNodeClient{
		NodeID:                    "node-remote",
		DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "mods/.xylona-download-install/testmod-1.0.0.jar"},
		RenameFileResult:          "mods/testmod-1.0.0.jar",
	}

	mod, errInstall := mgr.InstallRemote(context.Background(), client, "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("InstallRemote() error = %v", errInstall)
	}
	resetFakeNodeClientCalls(client)

	errDisable := mgr.Disable(context.Background(), client, mod.ID, serverDir, "mods")
	if errDisable != nil {
		t.Fatalf("Disable() error = %v", errDisable)
	}
	if len(client.MoveFilesCalls) != 1 {
		t.Fatalf("Disable() MoveFiles call count = %d, want 1", len(client.MoveFilesCalls))
	}
	if !slices.Equal(client.MoveFilesCalls[0].Files, []string{"mods/testmod-1.0.0.jar"}) {
		t.Fatalf("Disable() files = %v", client.MoveFilesCalls[0].Files)
	}
	if client.MoveFilesCalls[0].Destination != "mods/disabled" {
		t.Fatalf("Disable() destination = %q, want %q", client.MoveFilesCalls[0].Destination, "mods/disabled")
	}
	disabled, errDisabled := conn.GetInstalledModByID(mod.ID)
	if errDisabled != nil {
		t.Fatalf("GetInstalledModByID() disabled error = %v", errDisabled)
	}
	if disabled.Enabled != 0 {
		t.Fatalf("Disable() enabled = %d, want 0", disabled.Enabled)
	}

	resetFakeNodeClientCalls(client)
	errEnable := mgr.Enable(context.Background(), client, mod.ID, serverDir, "mods")
	if errEnable != nil {
		t.Fatalf("Enable() error = %v", errEnable)
	}
	if len(client.MoveFilesCalls) != 1 {
		t.Fatalf("Enable() MoveFiles call count = %d, want 1", len(client.MoveFilesCalls))
	}
	if !slices.Equal(client.MoveFilesCalls[0].Files, []string{"mods/disabled/testmod-1.0.0.jar"}) {
		t.Fatalf("Enable() files = %v", client.MoveFilesCalls[0].Files)
	}
	if client.MoveFilesCalls[0].Destination != "mods" {
		t.Fatalf("Enable() destination = %q, want %q", client.MoveFilesCalls[0].Destination, "mods")
	}
	enabled, errEnabled := conn.GetInstalledModByID(mod.ID)
	if errEnabled != nil {
		t.Fatalf("GetInstalledModByID() enabled error = %v", errEnabled)
	}
	if enabled.Enabled != 1 {
		t.Fatalf("Enable() enabled = %d, want 1", enabled.Enabled)
	}
}

func TestRemoteModAutoUpdateUsesNodeDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-remote-autoupdate.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	mock.updateVersion = &modproviders.ModVersion{
		VersionID:     "v2",
		VersionString: "2.0.0",
	}
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()
	client := &nodeclient.FakeNodeClient{
		NodeID:                    "node-remote",
		DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "mods/.xylona-download-install/testmod-1.0.0.jar"},
		RenameFileResult:          "mods/testmod-1.0.0.jar",
	}

	mod, errInstall := mgr.InstallRemote(context.Background(), client, "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("InstallRemote() error = %v", errInstall)
	}
	_, errExec := conn.SQLDb.ExecContext(context.Background(),
		`UPDATE installed_mod SET auto_update = 1 WHERE id = ?`, mod.ID)
	if errExec != nil {
		t.Fatalf("failed to enable auto_update: %v", errExec)
	}

	resetFakeNodeClientCalls(client)
	client.DownloadFileFromURLResult = node.DownloadFileResult{RelativePath: "mods/.xylona-download-update/testmod-2.0.0.jar"}
	var messages []string
	statusFn := func(msg string) {
		messages = append(messages, msg)
	}

	errAuto := mgr.RunAutoUpdatesRemote(context.Background(), client, "server-1", "1.20.1", serverDir, statusFn)
	if errAuto != nil {
		t.Fatalf("RunAutoUpdatesRemote() error = %v", errAuto)
	}
	if len(messages) == 0 {
		t.Fatal("RunAutoUpdatesRemote() expected status messages, got none")
	}
	if len(client.DownloadFileFromURLCalls) != 1 {
		t.Fatalf("DownloadFileFromURL call count = %d, want 1", len(client.DownloadFileFromURLCalls))
	}
	if client.DownloadFileFromURLCalls[0].RawURL != "https://example.test/testmod-2.0.0.jar" {
		t.Fatalf("DownloadFileFromURL raw URL = %q", client.DownloadFileFromURLCalls[0].RawURL)
	}
	updated, errGetUpdated := conn.GetInstalledModByID(mod.ID)
	if errGetUpdated != nil {
		t.Fatalf("GetInstalledModByID() error = %v", errGetUpdated)
	}
	if updated.InstalledVersionID != "v2" {
		t.Fatalf("RunAutoUpdatesRemote() version = %q, want %q", updated.InstalledVersionID, "v2")
	}
}

func TestInstall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-install.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()

	mod, errInstall := mgr.Install(context.Background(), "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("Install() error = %v", errInstall)
	}
	if mod.ModName != "Test Mod" {
		t.Errorf("Install().ModName = %q, want %q", mod.ModName, "Test Mod")
	}
	if mod.InstalledVersion != "1.0.0" {
		t.Errorf("Install().InstalledVersion = %q, want %q", mod.InstalledVersion, "1.0.0")
	}
	if mod.Source != pid {
		t.Errorf("Install().Source = %q, want %q", mod.Source, pid)
	}
	if mod.GameServerID != "server-1" {
		t.Errorf("Install().GameServerID = %q, want %q", mod.GameServerID, "server-1")
	}

	// Verify file was written to disk.
	filePath := filepath.Join(serverDir, "mods", "testmod-1.0.0.jar")
	_, errStat := os.Stat(filePath)
	if errStat != nil {
		t.Errorf("Install() expected file at %s, got error: %v", filePath, errStat)
	}

	// Verify file records in DB.
	files, errFiles := conn.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		t.Fatalf("GetInstalledModFilesByModID() error = %v", errFiles)
	}
	if len(files) != 1 {
		t.Fatalf("GetInstalledModFilesByModID() len = %d, want 1", len(files))
	}
	if files[0].FileHash != "abc123" {
		t.Errorf("file hash = %q, want %q", files[0].FileHash, "abc123")
	}
}

func TestInstallProviderNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-install-noprovider.sqlite")
	mgr := New(conn)

	_, errInstall := mgr.Install(context.Background(), "server-1", "nonexistent-provider", "src", "v1", "/tmp", "mods")
	if errInstall == nil {
		t.Fatal("Install() expected error for unknown provider")
	}
}

func TestInstallIntegrityFailureDoesNotWriteLiveFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-install-integrity-fail.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	mock.downloadErrAfterWrite = modproviders.ErrIntegrityMismatch
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()

	_, errInstall := mgr.Install(context.Background(), "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if !errors.Is(errInstall, modproviders.ErrIntegrityMismatch) {
		t.Fatalf("Install() error = %v, want %v", errInstall, modproviders.ErrIntegrityMismatch)
	}

	filePath := filepath.Join(serverDir, "mods", "testmod-1.0.0.jar")
	_, errStat := os.Stat(filePath)
	if !os.IsNotExist(errStat) {
		t.Fatalf("Install() wrote live file on integrity failure: %v", errStat)
	}

	mods, errMods := conn.GetInstalledModsByGameServerID("server-1")
	if errMods != nil {
		t.Fatalf("GetInstalledModsByGameServerID() error = %v", errMods)
	}
	if len(mods) != 0 {
		t.Fatalf("GetInstalledModsByGameServerID() len = %d, want 0", len(mods))
	}
}

func TestUninstall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-uninstall.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()

	mod, errInstall := mgr.Install(context.Background(), "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("Install() error = %v", errInstall)
	}

	// Verify file exists.
	filePath := filepath.Join(serverDir, "mods", "testmod-1.0.0.jar")
	_, errStat := os.Stat(filePath)
	if errStat != nil {
		t.Fatalf("expected file to exist before uninstall: %v", errStat)
	}

	errUninstall := mgr.Uninstall(context.Background(), newTestModFileClient(t, conn), mod.ID, serverDir)
	if errUninstall != nil {
		t.Fatalf("Uninstall() error = %v", errUninstall)
	}

	// Verify file removed.
	_, errStatAfter := os.Stat(filePath)
	if !os.IsNotExist(errStatAfter) {
		t.Errorf("expected file to be removed after uninstall, got: %v", errStatAfter)
	}

	// Verify DB records removed.
	files, errFiles := conn.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		t.Fatalf("GetInstalledModFilesByModID() error = %v", errFiles)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 file records after uninstall, got %d", len(files))
	}
}

func TestUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-update.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()

	mod, errInstall := mgr.Install(context.Background(), "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("Install() error = %v", errInstall)
	}

	// Change what the provider returns for v2 download.
	mock.downloadedFiles = []modproviders.DownloadedFile{
		{Path: "testmod-2.0.0.jar", Hash: "def456", Size: 2048, IsPrimary: true},
	}

	updated, errUpdate := mgr.Update(context.Background(), mod.ID, "v2", serverDir)
	if errUpdate != nil {
		t.Fatalf("Update() error = %v", errUpdate)
	}
	if updated.InstalledVersion != "2.0.0" {
		t.Errorf("Update().InstalledVersion = %q, want %q", updated.InstalledVersion, "2.0.0")
	}
	if updated.InstalledVersionID != "v2" {
		t.Errorf("Update().InstalledVersionID = %q, want %q", updated.InstalledVersionID, "v2")
	}
	if updated.FileHash != "def456" {
		t.Errorf("Update().FileHash = %q, want %q", updated.FileHash, "def456")
	}

	// Verify old file removed.
	oldPath := filepath.Join(serverDir, "mods", "testmod-1.0.0.jar")
	_, errOldStat := os.Stat(oldPath)
	if !os.IsNotExist(errOldStat) {
		t.Errorf("expected old file to be removed, got: %v", errOldStat)
	}

	// Verify new file exists.
	newPath := filepath.Join(serverDir, "mods", "testmod-2.0.0.jar")
	_, errNewStat := os.Stat(newPath)
	if errNewStat != nil {
		t.Errorf("expected new file to exist: %v", errNewStat)
	}
}

func TestUpdateIntegrityFailureKeepsExistingFilesAndMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-update-integrity-fail.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()

	mod, errInstall := mgr.Install(context.Background(), "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("Install() error = %v", errInstall)
	}

	mock.downloadedFiles = []modproviders.DownloadedFile{
		{Path: "testmod-2.0.0.jar", Hash: "def456", Size: 2048, IsPrimary: true},
	}
	mock.downloadErrAfterWrite = modproviders.ErrIntegrityMismatch

	_, errUpdate := mgr.Update(context.Background(), mod.ID, "v2", serverDir)
	if !errors.Is(errUpdate, modproviders.ErrIntegrityMismatch) {
		t.Fatalf("Update() error = %v, want %v", errUpdate, modproviders.ErrIntegrityMismatch)
	}

	oldPath := filepath.Join(serverDir, "mods", "testmod-1.0.0.jar")
	_, errOldStat := os.Stat(oldPath)
	if errOldStat != nil {
		t.Fatalf("Update() removed existing file on integrity failure: %v", errOldStat)
	}

	newPath := filepath.Join(serverDir, "mods", "testmod-2.0.0.jar")
	_, errNewStat := os.Stat(newPath)
	if !os.IsNotExist(errNewStat) {
		t.Fatalf("Update() left replacement file on integrity failure: %v", errNewStat)
	}

	storedMod, errStoredMod := conn.GetInstalledModByID(mod.ID)
	if errStoredMod != nil {
		t.Fatalf("GetInstalledModByID() error = %v", errStoredMod)
	}
	if storedMod.InstalledVersionID != "v1" {
		t.Fatalf("InstalledVersionID = %q, want %q", storedMod.InstalledVersionID, "v1")
	}
	if storedMod.FileHash != "abc123" {
		t.Fatalf("FileHash = %q, want %q", storedMod.FileHash, "abc123")
	}

	files, errFiles := conn.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		t.Fatalf("GetInstalledModFilesByModID() error = %v", errFiles)
	}
	if len(files) != 1 {
		t.Fatalf("GetInstalledModFilesByModID() len = %d, want 1", len(files))
	}
	if files[0].FilePath != filepath.Join("mods", "testmod-1.0.0.jar") {
		t.Fatalf("FilePath = %q, want %q", files[0].FilePath, filepath.Join("mods", "testmod-1.0.0.jar"))
	}
}

func TestSearchAll(t *testing.T) {
	pid1 := newUniqueProviderID()
	pid2 := newUniqueProviderID()

	mock1 := &mockProvider{
		id: pid1,
		searchResults: []modproviders.ModSearchResult{
			{Source: pid1, Name: "Mod A"},
		},
	}
	mock2 := &mockProvider{
		id: pid2,
		searchResults: []modproviders.ModSearchResult{
			{Source: pid2, Name: "Mod B"},
			{Source: pid2, Name: "Mod C"},
		},
	}

	modproviders.RegisterProvider(mock1)
	modproviders.RegisterProvider(mock2)

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-searchall.sqlite")
	mgr := New(conn)

	sources := []SourceConfig{
		{ID: pid1, SearchParams: map[string]any{}},
		{ID: pid2, SearchParams: map[string]any{}},
	}

	results, totalHits, errSearch := mgr.SearchAll(context.Background(), "test", sources, "", "", nil, 0, 0)
	if errSearch != nil {
		t.Fatalf("SearchAll() error = %v", errSearch)
	}
	if len(results) != 3 {
		t.Errorf("SearchAll() len = %d, want 3", len(results))
	}
	if totalHits != 3 {
		t.Errorf("SearchAll() totalHits = %d, want 3", totalHits)
	}
}

func TestSearchAllSkipsUnknownProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-searchall-skip.sqlite")
	mgr := New(conn)

	sources := []SourceConfig{
		{ID: "nonexistent-provider-xyz", SearchParams: map[string]any{}},
	}

	results, totalHits, errSearch := mgr.SearchAll(context.Background(), "test", sources, "", "", nil, 0, 0)
	if errSearch != nil {
		t.Fatalf("SearchAll() error = %v", errSearch)
	}
	if len(results) != 0 {
		t.Errorf("SearchAll() len = %d, want 0", len(results))
	}
	if totalHits != 0 {
		t.Errorf("SearchAll() totalHits = %d, want 0", totalHits)
	}
}

func TestSearchAllReturnsUnknownTotalWhenProviderTotalIsUnknown(t *testing.T) {
	pid1 := newUniqueProviderID()
	pid2 := newUniqueProviderID()

	mock1 := &mockProvider{
		id:              pid1,
		searchResults:   []modproviders.ModSearchResult{{Source: pid1, Name: "Known"}},
		searchTotalHits: 1,
	}
	mock2 := &mockProvider{
		id:              pid2,
		searchResults:   []modproviders.ModSearchResult{{Source: pid2, Name: "Unknown"}},
		searchTotalHits: -1,
	}

	modproviders.RegisterProvider(mock1)
	modproviders.RegisterProvider(mock2)

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-searchall-unknown-total.sqlite")
	mgr := New(conn)

	sources := []SourceConfig{
		{ID: pid1, SearchParams: map[string]any{}},
		{ID: pid2, SearchParams: map[string]any{}},
	}

	results, totalHits, errSearch := mgr.SearchAll(context.Background(), "test", sources, "", "", nil, 0, 0)
	if errSearch != nil {
		t.Fatalf("SearchAll() error = %v", errSearch)
	}
	if len(results) != 2 {
		t.Fatalf("SearchAll() len = %d, want 2", len(results))
	}
	if totalHits != -1 {
		t.Fatalf("SearchAll() totalHits = %d, want -1 for unknown total", totalHits)
	}
}

func TestEnableDisable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-enable-disable.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()

	mod, errInstall := mgr.Install(context.Background(), "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("Install() error = %v", errInstall)
	}

	// Disable the mod.
	fileClient := newTestModFileClient(t, conn)

	errDisable := mgr.Disable(context.Background(), fileClient, mod.ID, serverDir, "mods")
	if errDisable != nil {
		t.Fatalf("Disable() error = %v", errDisable)
	}

	// Verify file moved to disabled dir.
	disabledPath := filepath.Join(serverDir, "mods", "disabled", "testmod-1.0.0.jar")
	_, errDisabledStat := os.Stat(disabledPath)
	if errDisabledStat != nil {
		t.Errorf("expected file in disabled dir: %v", errDisabledStat)
	}

	// Verify original location is gone.
	originalPath := filepath.Join(serverDir, "mods", "testmod-1.0.0.jar")
	_, errOrigStat := os.Stat(originalPath)
	if !os.IsNotExist(errOrigStat) {
		t.Errorf("expected original file to be gone, got: %v", errOrigStat)
	}

	// Verify DB record updated.
	disabledMod, errGetDisabled := conn.GetInstalledModByID(mod.ID)
	if errGetDisabled != nil {
		t.Fatalf("GetInstalledModByID() error = %v", errGetDisabled)
	}
	if disabledMod.Enabled != 0 {
		t.Errorf("Disable() Enabled = %d, want 0", disabledMod.Enabled)
	}

	// Enable the mod.
	errEnable := mgr.Enable(context.Background(), fileClient, mod.ID, serverDir, "mods")
	if errEnable != nil {
		t.Fatalf("Enable() error = %v", errEnable)
	}

	// Verify file moved back.
	_, errOrigStatAfter := os.Stat(originalPath)
	if errOrigStatAfter != nil {
		t.Errorf("expected file back in original location: %v", errOrigStatAfter)
	}

	// Verify DB updated.
	enabledMod, errGetEnabled := conn.GetInstalledModByID(mod.ID)
	if errGetEnabled != nil {
		t.Fatalf("GetInstalledModByID() error = %v", errGetEnabled)
	}
	if enabledMod.Enabled != 1 {
		t.Errorf("Enable() Enabled = %d, want 1", enabledMod.Enabled)
	}
}

func TestRunAutoUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-autoupdate.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	mock.updateVersion = &modproviders.ModVersion{
		VersionID:     "v2",
		VersionString: "2.0.0",
	}
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()

	// Install a mod.
	mod, errInstall := mgr.Install(context.Background(), "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("Install() error = %v", errInstall)
	}

	// Enable auto_update on the mod.
	_, errExec := conn.SQLDb.ExecContext(context.Background(),
		`UPDATE installed_mod SET auto_update = 1 WHERE id = ?`, mod.ID)
	if errExec != nil {
		t.Fatalf("failed to enable auto_update: %v", errExec)
	}

	// Change downloaded files for the update.
	mock.downloadedFiles = []modproviders.DownloadedFile{
		{Path: "testmod-2.0.0.jar", Hash: "def456", Size: 2048, IsPrimary: true},
	}

	var messages []string
	statusFn := func(msg string) {
		messages = append(messages, msg)
	}

	errAuto := mgr.RunAutoUpdates(context.Background(), "server-1", "1.20.1", serverDir, statusFn)
	if errAuto != nil {
		t.Fatalf("RunAutoUpdates() error = %v", errAuto)
	}

	if len(messages) == 0 {
		t.Error("RunAutoUpdates() expected status messages, got none")
	}

	// Verify mod was updated.
	updated, errGetUpdated := conn.GetInstalledModByID(mod.ID)
	if errGetUpdated != nil {
		t.Fatalf("GetInstalledModByID() error = %v", errGetUpdated)
	}
	if updated.InstalledVersionID != "v2" {
		t.Errorf("RunAutoUpdates() version = %q, want %q", updated.InstalledVersionID, "v2")
	}
}

func TestRunAutoUpdatesSkipsPinned(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-autoupdate-skip.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	mock.updateVersion = &modproviders.ModVersion{
		VersionID:     "v2",
		VersionString: "2.0.0",
	}
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()

	// Install mod and pin it.
	mod, errInstall := mgr.Install(context.Background(), "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("Install() error = %v", errInstall)
	}

	_, errExec := conn.SQLDb.ExecContext(context.Background(),
		`UPDATE installed_mod SET auto_update = 1, pinned_version = '1.0.0' WHERE id = ?`, mod.ID)
	if errExec != nil {
		t.Fatalf("failed to pin mod: %v", errExec)
	}

	var messages []string
	statusFn := func(msg string) {
		messages = append(messages, msg)
	}

	errAuto := mgr.RunAutoUpdates(context.Background(), "server-1", "1.20.1", serverDir, statusFn)
	if errAuto != nil {
		t.Fatalf("RunAutoUpdates() error = %v", errAuto)
	}

	// Verify mod was NOT updated (pinned).
	notUpdated, errGet := conn.GetInstalledModByID(mod.ID)
	if errGet != nil {
		t.Fatalf("GetInstalledModByID() error = %v", errGet)
	}
	if notUpdated.InstalledVersionID != "v1" {
		t.Errorf("RunAutoUpdates() should not update pinned mod, got version %q", notUpdated.InstalledVersionID)
	}
}

func TestRunAutoUpdatesContinuesOnFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	conn := dbtest.NewMigratedConnection(t, "mm-autoupdate-fail.sqlite")
	seedTestFixture(t, conn)

	pid := newUniqueProviderID()
	mock := newMockProvider(pid)
	mock.checkForUpdateErr = errors.New("simulated API failure")
	modproviders.RegisterProvider(mock)

	mgr := New(conn)
	serverDir := t.TempDir()

	mod, errInstall := mgr.Install(context.Background(), "server-1", pid, "mod-src-1", "v1", serverDir, "mods")
	if errInstall != nil {
		t.Fatalf("Install() error = %v", errInstall)
	}

	_, errExec := conn.SQLDb.ExecContext(context.Background(),
		`UPDATE installed_mod SET auto_update = 1 WHERE id = ?`, mod.ID)
	if errExec != nil {
		t.Fatalf("failed to enable auto_update: %v", errExec)
	}

	var messages []string
	statusFn := func(msg string) {
		messages = append(messages, msg)
	}

	// Should not return error even though check fails.
	errAuto := mgr.RunAutoUpdates(context.Background(), "server-1", "1.20.1", serverDir, statusFn)
	if errAuto != nil {
		t.Fatalf("RunAutoUpdates() error = %v, expected nil (should continue on failure)", errAuto)
	}

	// Verify failure message was reported.
	foundFailMsg := false
	for _, msg := range messages {
		if len(msg) > 0 {
			foundFailMsg = true
		}
	}
	if !foundFailMsg {
		t.Error("RunAutoUpdates() expected status messages about failure")
	}
}
