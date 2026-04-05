package actions

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"

	internal "github.com/ClintonCollins/Xylona/api/xylona-internal"
	"github.com/ClintonCollins/Xylona/db/dbtest"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

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
	internal.RegisterGame(gameID, internalUpdateTestGame{markerPath: markerPath})

	inst := &Instance{
		ctx:                ctx,
		supervisorInstance: supervisorInst,
	}
	gameServer := &models.GameServer{
		ID:        "server-1",
		GameID:    gameID,
		Directory: serverDir,
		UserID:    "user-1",
	}
	gameServer.R.Game = &models.Game{}

	inst.UpdateGameServer(gameServer)

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
		supervisorInstance: supervisorInst,
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

func TestUpdateGameServerRejectsUnsupportedMinecraftVariant(t *testing.T) {
	ctx := context.Background()
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	serverDir := t.TempDir()
	markerPath := filepath.Join(serverDir, "updated.txt")
	previousGame, hadPrevious := internal.GetGame("minecraft")
	internal.RegisterGame("minecraft", internalUpdateTestGame{markerPath: markerPath})
	t.Cleanup(func() {
		if hadPrevious {
			internal.RegisterGame("minecraft", previousGame)
			return
		}
		delete(internal.GetGames(), "minecraft")
	})

	inst := &Instance{
		ctx:                ctx,
		supervisorInstance: supervisorInst,
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
		supervisorInstance: supervisorInst,
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
		ID:      omit.From("node-local"),
		Name:    omit.From("Local Node"),
		IsLocal: omit.From(true),
		Host:    omit.From("localhost"),
		Port:    omit.From(int64(8080)),
		BaseURL: omit.From("http://localhost:8080"),
		Enabled: omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode() error = %v", errInsertNode)
	}

	_, errUpsertIP := conn.UpsertIP(&models.IPSetter{
		Address:            omit.From("127.0.0.1"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
	})
	if errUpsertIP != nil {
		t.Fatalf("UpsertIP() error = %v", errUpsertIP)
	}

	serverDir := t.TempDir()
	markerPath := filepath.Join(serverDir, "updated.txt")
	gameID := "internal-update-console-test"
	internal.RegisterGame(gameID, internalUpdateTestGame{markerPath: markerPath})
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
		supervisorInstance: supervisorInst,
	}
	gameServer := &models.GameServer{
		ID:        "server-console-progress",
		GameID:    gameID,
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
			if msg != nil && msg.GameServerConsoleOutput != nil {
				streamedOutput.WriteString(msg.GameServerConsoleOutput.Output)
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
		supervisorInstance: supervisorInst,
	}
	gameServer := &models.GameServer{
		ID:               "minecraft-detailed-update",
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
			if msg != nil && msg.GameServerConsoleOutput != nil {
				streamedOutput.WriteString(msg.GameServerConsoleOutput.Output)
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
	step       xylona.UpdateStep
	stepStatus xylona.StepStatus
	message    string
}

type recordingUpdateProgressBroadcaster struct {
	events []recordedUpdateProgress
}

func (b *recordingUpdateProgressBroadcaster) BroadcastUpdateProgress(
	_ string,
	step xylona.UpdateStep,
	stepStatus xylona.StepStatus,
	message string,
) {
	b.events = append(b.events, recordedUpdateProgress{
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

type internalUpdateTestGame struct {
	markerPath string
}

func (g internalUpdateTestGame) Install(_ *models.GameServer, _, _ io.Writer) error {
	return nil
}

func (g internalUpdateTestGame) Update(_ *models.GameServer, _, _ io.Writer) error {
	return os.WriteFile(g.markerPath, []byte("updated"), 0o644)
}

type minecraftUpdateTestProvider struct {
	providerID          string
	latestVersion       string
	downloadVersion     string
	markerPath          string
	detailsSourceID     string
	versionsSourceID    string
	versionsGameVersion string
	downloadSourceID    string
	downloadVersionID   string
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

	errSave := updateproviders.SaveGameConfigToModel(game, updateproviders.GameConfig{
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
		{VersionID: p.downloadVersion, VersionString: "Build 9"},
	}, nil
}

func (p *minecraftUpdateTestProvider) Download(_ context.Context, sourceID string, versionID string, targetDir string) ([]modproviders.DownloadedFile, error) {
	p.downloadSourceID = sourceID
	p.downloadVersionID = versionID
	relativePath := filepath.Base(p.markerPath)
	fullPath := filepath.Join(targetDir, relativePath)
	if errWrite := os.WriteFile(fullPath, []byte("updated"), 0o644); errWrite != nil {
		return nil, errWrite
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
