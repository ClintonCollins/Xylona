package rpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGetGameServerConfigFileReadsRemoteNodeFile(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	updateGameConfigSchemasForRemoteParity(t, fixture, `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","properties":{"motd":{"type":"string","default":"Hello world"}}}}]`)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-config")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-remote",
		ReadFileResult: []byte("motd=Remote MOTD\n"),
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GetGameServerConfigFileRequest{
		GameServerId: "server-remote-config",
		FilePath:     "server.properties",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetGameServerConfigFile(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetGameServerConfigFile() error = %v", errGet)
	}
	if len(remoteClient.ReadFileCalls) != 1 {
		t.Fatalf("ReadFile call count = %d, want 1", len(remoteClient.ReadFileCalls))
	}
	call := remoteClient.ReadFileCalls[0]
	if call.Directory != "/srv/remote-server" {
		t.Fatalf("ReadFile directory = %q, want %q", call.Directory, "/srv/remote-server")
	}
	if call.RelativePath != "server.properties" {
		t.Fatalf("ReadFile relative path = %q, want %q", call.RelativePath, "server.properties")
	}
	if response.Msg.GetFilePath() != "server.properties" {
		t.Fatalf("GetGameServerConfigFile().FilePath = %q, want %q", response.Msg.GetFilePath(), "server.properties")
	}
	if len(response.Msg.GetFields()) != 1 {
		t.Fatalf("GetGameServerConfigFile().Fields length = %d, want 1", len(response.Msg.GetFields()))
	}
	if response.Msg.GetFields()[0].GetValue() != "Remote MOTD" {
		t.Fatalf("GetGameServerConfigFile().Fields[0].Value = %q, want %q", response.Msg.GetFields()[0].GetValue(), "Remote MOTD")
	}
}

func TestUpdateGameServerConfigFileWritesRemoteNodeFile(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	updateGameConfigSchemasForRemoteParity(t, fixture, `[{"path":"nested/server.properties","format":"properties","category":"Core","schema":{"type":"object","properties":{"motd":{"type":"string","default":"Hello world"}}}}]`)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-config")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-remote",
		ReadFileResult: []byte("motd=Old MOTD\n"),
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.UpdateGameServerConfigFileRequest{
		GameServerId: "server-remote-config",
		FilePath:     "nested/server.properties",
		Fields: []*xylona.ConfigFieldData{
			{
				Key:       "motd",
				Value:     "New Remote MOTD",
				FieldType: "string",
			},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errUpdate := fixture.service.UpdateGameServerConfigFile(context.Background(), request)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerConfigFile() error = %v", errUpdate)
	}
	if !response.Msg.GetSuccess() {
		t.Fatal("UpdateGameServerConfigFile().Success = false, want true")
	}
	if len(remoteClient.CreateFileOrDirectoryCalls) != 1 {
		t.Fatalf("CreateFileOrDirectory call count = %d, want 1", len(remoteClient.CreateFileOrDirectoryCalls))
	}
	if remoteClient.CreateFileOrDirectoryCalls[0].RelativePath != "nested" {
		t.Fatalf("CreateFileOrDirectory relative path = %q, want %q", remoteClient.CreateFileOrDirectoryCalls[0].RelativePath, "nested")
	}
	if len(remoteClient.WriteFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(remoteClient.WriteFileCalls))
	}
	if remoteClient.WriteFileCalls[0].RelativePath != "nested/server.properties" {
		t.Fatalf("WriteFile relative path = %q, want %q", remoteClient.WriteFileCalls[0].RelativePath, "nested/server.properties")
	}
	if got := string(remoteClient.WriteFileCalls[0].Content); got != "motd=New Remote MOTD\n" {
		t.Fatalf("WriteFile content = %q, want %q", got, "motd=New Remote MOTD\n")
	}
}

func TestGenerateGameServerConfigFileWritesRemoteDefaults(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	updateGameConfigSchemasForRemoteParity(t, fixture, `[{"path":"server.properties","format":"properties","category":"Core","generate_before_start":true,"schema":{"type":"object","properties":{"motd":{"type":"string","default":"Generated MOTD"}}}}]`)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-config")

	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GenerateGameServerConfigFileRequest{
		GameServerId: "server-remote-config",
		FilePath:     "server.properties",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGenerate := fixture.service.GenerateGameServerConfigFile(context.Background(), request)
	if errGenerate != nil {
		t.Fatalf("GenerateGameServerConfigFile() error = %v", errGenerate)
	}
	if !response.Msg.GetSuccess() {
		t.Fatal("GenerateGameServerConfigFile().Success = false, want true")
	}
	if len(remoteClient.WriteFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(remoteClient.WriteFileCalls))
	}
	if got := string(remoteClient.WriteFileCalls[0].Content); got != "motd=Generated MOTD\n" {
		t.Fatalf("WriteFile content = %q, want %q", got, "motd=Generated MOTD\n")
	}
}

func TestGetVersionInfoReadsRemoteExecutableViaNodeClient(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-version")

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:               omit.From("server-remote-version"),
		ServerExecutable: omitnull.From("server.jar"),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}
	gameServer, errGetServer := fixture.conn.GetGameServerByID("server-remote-version")
	if errGetServer != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGetServer)
	}
	contextKey := fixture.service.remoteVersionTrackerContext(gameServer).CacheKey()

	versionStates := versiontracker.NewVersionStateMap()
	versionStates.Set("server-remote-version", versiontracker.VersionState{
		Status:          versiontracker.VersionStatusChecked,
		LatestVersion:   "1.21.5",
		LatestCheckTime: time.Now(),
		TrackerType:     "minecraft",
		ContextKey:      contextKey,
	})
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		ProbeInstalledVersionResult: node.InstalledVersionProbeResult{
			Found:      true,
			Version:    "1.21.4",
			SourcePath: "server.jar",
		},
		SnapshotResult: &node.NodeSnapshot{OS: "linux"},
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	fixture.service.nodeRegistry = registry
	fixture.service.versionState = versionStates
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		registry,
		nil,
		versionStates,
		versiontracker.ResolverConfig{},
	)

	request := connect.NewRequest(&xylona.GetVersionInfoRequest{GameServerId: "server-remote-version"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetVersionInfo(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetVersionInfo() error = %v", errGet)
	}
	if response.Msg.GetVersionInfo().GetInstalledVersion() != "1.21.4" {
		t.Fatalf("GetVersionInfo().InstalledVersion = %q, want %q", response.Msg.GetVersionInfo().GetInstalledVersion(), "1.21.4")
	}
	if len(remoteClient.ProbeInstalledVersionCalls) != 1 {
		t.Fatalf("ProbeInstalledVersion call count = %d, want 1", len(remoteClient.ProbeInstalledVersionCalls))
	}
	probeCall := remoteClient.ProbeInstalledVersionCalls[0]
	if probeCall.Kind != node.InstalledVersionProbeKindMinecraftJar {
		t.Fatalf("ProbeInstalledVersion kind = %v, want minecraft jar", probeCall.Kind)
	}
	if len(probeCall.RelativePaths) == 0 || probeCall.RelativePaths[0] != "server.jar" {
		t.Fatalf("ProbeInstalledVersion relative paths = %v, want server.jar first", probeCall.RelativePaths)
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
	if len(remoteClient.ListFilesCalls) != 0 {
		t.Fatalf("ListFiles call count = %d, want 0", len(remoteClient.ListFilesCalls))
	}
}

func TestSetServerVariantUploadsDownloadedFilesToRemoteNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-variant")

	game, errGetGame := fixture.conn.GetGameByID("minecraft")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}
	setMinecraftRemoteVariantConfig(t, game)
	_, errUpdateGame := fixture.conn.UpdateGame(fixture.conn.DB, game, &models.GameSetter{
		ID:             omit.From(game.ID),
		ServerSoftware: omitnull.From(game.ServerSoftware.GetOr("")),
	})
	if errUpdateGame != nil {
		t.Fatalf("UpdateGame() error = %v", errUpdateGame)
	}
	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:               omit.From("server-remote-variant"),
		Status:           omit.From(xylona.Status_OFFLINE.String()),
		ServerExecutable: omitnull.From("paper-1.21.4.jar"),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                    "node-remote",
		DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "paper-1.21.5.jar", BytesWritten: 4096, SHA256: "paper-sha"},
		GetProcessSnapshotFound:   true,
		GetProcessSnapshotResult: &node.ProcessSnapshot{
			ID:     "server-remote-variant",
			Status: xylona.Status_OFFLINE.String(),
		},
		SnapshotResult: &node.NodeSnapshot{OS: "linux"},
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	fixture.service.nodeRegistry = registry
	fixture.service.installTracker = modmanager.NewInstallTracker()
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		registry,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	provider := &remoteVariantProvider{remoteDirectory: "/srv/remote-server"}
	previousLookup := variantProviderLookup
	variantProviderLookup = func(cfg updateproviders.ProviderConfig) (modproviders.ModProvider, bool) {
		if cfg.Kind == updateproviders.ProviderKindPaperMC {
			return provider, true
		}
		return previousLookup(cfg)
	}
	defer func() {
		variantProviderLookup = previousLookup
	}()

	request := connect.NewRequest(&xylona.SetServerVariantRequest{
		GameServerId: "server-remote-variant",
		VariantId:    "paper",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errSet := fixture.service.SetServerVariant(context.Background(), request)
	if errSet != nil {
		t.Fatalf("SetServerVariant() error = %v", errSet)
	}
	if response.Msg.GetStatus() != modmanager.InstallStatusInstalling {
		t.Fatalf("SetServerVariant().Status = %q, want %q", response.Msg.GetStatus(), modmanager.InstallStatusInstalling)
	}

	waitForCondition(t, func() bool {
		state, ok := fixture.service.installTracker.Get("server-remote-variant")
		return ok && state.Status != modmanager.InstallStatusInstalling
	})

	state, _ := fixture.service.installTracker.Get("server-remote-variant")
	if state.Status != modmanager.InstallStatusComplete {
		t.Fatalf("install status = %q, want %q (error=%q)", state.Status, modmanager.InstallStatusComplete, state.Error)
	}
	if provider.downloadTargetDir != "" {
		t.Fatalf("provider.Download target dir = %q, want no controller-side provider download", provider.downloadTargetDir)
	}
	if len(remoteClient.DownloadFileFromURLCalls) != 1 {
		t.Fatalf("DownloadFileFromURL call count = %d, want 1", len(remoteClient.DownloadFileFromURLCalls))
	}
	downloadCall := remoteClient.DownloadFileFromURLCalls[0]
	if downloadCall.Directory != "/srv/remote-server" {
		t.Fatalf("DownloadFileFromURL directory = %q, want %q", downloadCall.Directory, "/srv/remote-server")
	}
	if downloadCall.RawURL != "https://downloads.example.test/paper-1.21.5.jar" {
		t.Fatalf("DownloadFileFromURL raw URL = %q, want provider metadata URL", downloadCall.RawURL)
	}
	if downloadCall.DestinationDirectoryPath != "" {
		t.Fatalf("DownloadFileFromURL destination = %q, want server root", downloadCall.DestinationDirectoryPath)
	}
	if downloadCall.Integrity.ExpectedSize != 4096 || downloadCall.Integrity.ExpectedSHA256 != "paper-sha" {
		t.Fatalf("DownloadFileFromURL integrity = %+v, want size=4096 paper-sha", downloadCall.Integrity)
	}
	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("DeleteFiles call count = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	if remoteClient.DeleteFilesCalls[0].Files[0] != "paper-1.21.4.jar" {
		t.Fatalf("DeleteFiles file = %q, want %q", remoteClient.DeleteFilesCalls[0].Files[0], "paper-1.21.4.jar")
	}
}

func TestSetServerVariantRejectsRemoteDownloadWithoutIntegrity(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-variant")

	game, errGetGame := fixture.conn.GetGameByID("minecraft")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}
	setMinecraftRemoteVariantConfig(t, game)
	_, errUpdateGame := fixture.conn.UpdateGame(fixture.conn.DB, game, &models.GameSetter{
		ID:             omit.From(game.ID),
		ServerSoftware: omitnull.From(game.ServerSoftware.GetOr("")),
	})
	if errUpdateGame != nil {
		t.Fatalf("UpdateGame() error = %v", errUpdateGame)
	}
	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:               omit.From("server-remote-variant"),
		Status:           omit.From(xylona.Status_OFFLINE.String()),
		ServerExecutable: omitnull.From("paper-1.21.4.jar"),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		GetProcessSnapshotResult: &node.ProcessSnapshot{
			ID:     "server-remote-variant",
			Status: xylona.Status_OFFLINE.String(),
		},
		GetProcessSnapshotFound: true,
		SnapshotResult:          &node.NodeSnapshot{OS: "linux"},
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	fixture.service.nodeRegistry = registry
	fixture.service.installTracker = modmanager.NewInstallTracker()
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		registry,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	provider := &remoteVariantProvider{remoteDirectory: "/srv/remote-server", omitIntegrity: true}
	previousLookup := variantProviderLookup
	variantProviderLookup = func(cfg updateproviders.ProviderConfig) (modproviders.ModProvider, bool) {
		if cfg.Kind == updateproviders.ProviderKindPaperMC {
			return provider, true
		}
		return previousLookup(cfg)
	}
	defer func() {
		variantProviderLookup = previousLookup
	}()

	request := connect.NewRequest(&xylona.SetServerVariantRequest{
		GameServerId: "server-remote-variant",
		VariantId:    "paper",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errSet := fixture.service.SetServerVariant(context.Background(), request)
	if errSet != nil {
		t.Fatalf("SetServerVariant() error = %v", errSet)
	}
	if response.Msg.GetStatus() != modmanager.InstallStatusInstalling {
		t.Fatalf("SetServerVariant().Status = %q, want %q", response.Msg.GetStatus(), modmanager.InstallStatusInstalling)
	}

	waitForCondition(t, func() bool {
		state, ok := fixture.service.installTracker.Get("server-remote-variant")
		return ok && state.Status != modmanager.InstallStatusInstalling
	})

	state, _ := fixture.service.installTracker.Get("server-remote-variant")
	if state.Status != modmanager.InstallStatusFailed {
		t.Fatalf("install status = %q, want %q", state.Status, modmanager.InstallStatusFailed)
	}
	if !strings.Contains(state.Error, "integrity metadata is unavailable") {
		t.Fatalf("install error = %q, want missing integrity metadata", state.Error)
	}
	if len(remoteClient.DownloadFileFromURLCalls) != 0 {
		t.Fatalf("DownloadFileFromURL call count = %d, want 0", len(remoteClient.DownloadFileFromURLCalls))
	}
	if len(remoteClient.DeleteFilesCalls) != 0 {
		t.Fatalf("DeleteFiles call count = %d, want 0", len(remoteClient.DeleteFilesCalls))
	}
	if provider.downloadTargetDir != "" {
		t.Fatalf("provider.Download target dir = %q, want no controller-side provider download", provider.downloadTargetDir)
	}
}

func updateGameConfigSchemasForRemoteParity(t *testing.T, fixture *rbacRPCFixture, schemasJSON string) {
	t.Helper()

	game, errGetGame := fixture.conn.GetGameByID("minecraft")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}
	_, errUpdate := fixture.conn.UpdateGame(fixture.conn.DB, game, &models.GameSetter{
		ID:            omit.From(game.ID),
		ConfigSchemas: omitnull.From(schemasJSON),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGame() error = %v", errUpdate)
	}
}

type remoteVariantProvider struct {
	remoteDirectory   string
	downloadTargetDir string
	omitIntegrity     bool
}

func (p *remoteVariantProvider) ID() string {
	return "remote-variant-provider"
}

func (p *remoteVariantProvider) Search(_ context.Context, _ string, _ modproviders.SearchParams) (modproviders.SearchResult, error) {
	return modproviders.SearchResult{}, nil
}

func (p *remoteVariantProvider) GetModDetails(_ context.Context, sourceID string, _ modproviders.SearchParams) (*modproviders.ModDetails, error) {
	return &modproviders.ModDetails{
		SourceID: sourceID,
		Versions: []modproviders.ModVersion{
			{VersionID: "1.21.5", VersionString: "1.21.5"},
		},
	}, nil
}

func (p *remoteVariantProvider) GetVersions(_ context.Context, _ string, gameVersion string, _ modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	version := modproviders.ModVersion{
		VersionID:     "1.21.5",
		VersionString: gameVersion,
		DownloadURL:   "https://downloads.example.test/paper-1.21.5.jar",
	}
	if !p.omitIntegrity {
		version.FileSize = 4096
		version.FileHashSHA256 = "paper-sha"
	}
	return []modproviders.ModVersion{
		version,
	}, nil
}

func (p *remoteVariantProvider) Download(_ context.Context, _ string, _ string, targetDir string) ([]modproviders.DownloadedFile, error) {
	p.downloadTargetDir = targetDir
	if targetDir == p.remoteDirectory {
		return nil, errors.New("provider download must not target the remote server directory")
	}

	fullPath := filepath.Join(targetDir, "paper-1.21.5.jar")
	errWrite := os.WriteFile(fullPath, []byte("paper-binary"), 0o600)
	if errWrite != nil {
		return nil, fmt.Errorf("write staged variant file: %w", errWrite)
	}
	return []modproviders.DownloadedFile{
		{Path: "paper-1.21.5.jar", IsPrimary: true},
	}, nil
}

func (p *remoteVariantProvider) CheckForUpdate(_ context.Context, _ string, _ string) (*modproviders.ModVersion, error) {
	return &modproviders.ModVersion{VersionID: "1.21.5", VersionString: "1.21.5"}, nil
}

func (p *remoteVariantProvider) RequiresAPIKey() bool {
	return false
}

func setMinecraftRemoteVariantConfig(t *testing.T, game *models.Game) {
	t.Helper()

	errSave := updateproviders.SaveGameConfigToModel(game, updateproviders.GameConfig{
		UpdateProvider: updateproviders.ProviderConfig{Kind: updateproviders.ProviderKindCommand},
		Variants: []updateproviders.Variant{
			{
				ID:            "paper",
				Name:          "Paper",
				DefaultTarget: "1.21.5",
				UpdateProvider: &updateproviders.ProviderConfig{
					Kind:     updateproviders.ProviderKindPaperMC,
					SourceID: "paper",
				},
			},
		},
	})
	if errSave != nil {
		t.Fatalf("SaveGameConfigToModel() error = %v", errSave)
	}
}

func waitForCondition(t *testing.T, predicate func() bool) {
	t.Helper()

	for range 200 {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied")
}
