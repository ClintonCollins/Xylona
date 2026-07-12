//go:build integration

package actions

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/gamedefinitions"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/internal/placeholder"
	gamequery "github.com/ClintonCollins/Xylona/pkg/query"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	livePalworldEnabledEnv = "XYLONA_LIVE_PALWORLD"
	livePalworldRootEnv    = "XYLONA_LIVE_PALWORLD_ROOT"
	liveSteamCMDPathEnv    = "STEAMCMD_PATH"
)

type livePalworldInstance struct {
	gameServer *models.GameServer
	password   string
	command    *supervisor.Command
}

func TestLivePalworldMultipleInstances(t *testing.T) {
	if os.Getenv(livePalworldEnabledEnv) != "1" {
		t.Skip("set XYLONA_LIVE_PALWORLD=1 to run the live Palworld integration proof")
	}
	if testing.Short() {
		t.Skip("skipping live Palworld integration proof in short mode")
	}

	definitionData, errDefinitionData := gamedefinitions.FS.ReadFile("official/palworld.json")
	if errDefinitionData != nil {
		t.Fatalf("read Palworld definition: %v", errDefinitionData)
	}
	definition, errDefinition := gamedefinitions.Parse(definitionData)
	if errDefinition != nil {
		t.Fatalf("parse Palworld definition: %v", errDefinition)
	}

	root := strings.TrimSpace(os.Getenv(livePalworldRootEnv))
	if root == "" {
		root = t.TempDir()
	}
	errRoot := os.MkdirAll(root, 0o750)
	if errRoot != nil {
		t.Fatalf("create live Palworld root: %v", errRoot)
	}

	instances := []*livePalworldInstance{
		newLivePalworldInstance(
			definition.Model,
			filepath.Join(root, "palworld-one"),
			"palworld-live-one",
			"Xylona Palworld One",
			38211,
			38212,
			"live-palworld-one-password",
		),
		newLivePalworldInstance(
			definition.Model,
			filepath.Join(root, "palworld-two"),
			"palworld-live-two",
			"Xylona Palworld Two",
			38214,
			38215,
			"live-palworld-two-password",
		),
	}

	for _, instance := range instances {
		installLivePalworld(t, instance.gameServer)
		prepareLivePalworldSettings(t, instance)
	}

	processSupervisor, errSupervisor := supervisor.New(t.Context())
	if errSupervisor != nil {
		t.Fatalf("create process supervisor: %v", errSupervisor)
	}
	actionInstance := &Instance{ctx: t.Context()}
	for _, instance := range instances {
		instance.command = startLivePalworld(t, actionInstance, processSupervisor, instance.gameServer)
	}
	t.Cleanup(func() {
		for _, instance := range instances {
			stopLivePalworld(t, instance.command)
		}
	})

	for _, instance := range instances {
		waitForLivePalworldConsoleOutput(t, instance.command, 3*time.Minute)
		info := waitForLivePalworldREST(t, instance, 5*time.Minute)
		if info.GetName() != instance.gameServer.Name {
			t.Fatalf(
				"Palworld REST name = %q, want %q",
				info.GetName(),
				instance.gameServer.Name,
			)
		}
	}

	stopLivePalworld(t, instances[0].command)
	if instances[1].command.Status() != xylona.Status_ONLINE {
		t.Fatalf(
			"second Palworld status after stopping first = %s, want ONLINE",
			instances[1].command.Status(),
		)
	}
	secondInfo := waitForLivePalworldREST(t, instances[1], time.Minute)
	if secondInfo.GetName() != instances[1].gameServer.Name {
		t.Fatalf(
			"second Palworld REST name after first stopped = %q, want %q",
			secondInfo.GetName(),
			instances[1].gameServer.Name,
		)
	}
	stopLivePalworld(t, instances[1].command)
}

func newLivePalworldInstance(
	game *models.Game,
	directory string,
	id string,
	name string,
	port int64,
	queryPort int64,
	password string,
) *livePalworldInstance {
	gameServer := &models.GameServer{
		ID:               id,
		Name:             name,
		GameID:           palworldGameID,
		Directory:        directory,
		IP:               "127.0.0.1",
		Port:             port,
		QueryPort:        queryPort,
		MaxPlayers:       32,
		StartArgsPatches: "[]",
	}
	gameServer.R.Game = game
	return &livePalworldInstance{
		gameServer: gameServer,
		password:   password,
	}
}

func installLivePalworld(t *testing.T, gameServer *models.GameServer) {
	t.Helper()
	errDirectory := os.MkdirAll(gameServer.Directory, 0o750)
	if errDirectory != nil {
		t.Fatalf("create Palworld install directory: %v", errDirectory)
	}
	installVars := placeholder.BuildVarsFromGameServer(gameServer)
	baseCommand, args, errResolve := resolveCommandLineToProcessArgs(
		gameInstallCommand(gameServer.R.Game, OperatingSystem),
		installVars,
	)
	if errResolve != nil {
		t.Fatalf("resolve Palworld install command: %v", errResolve)
	}
	steamCMDPath := strings.TrimSpace(os.Getenv(liveSteamCMDPathEnv))
	if steamCMDPath != "" {
		baseCommand = steamCMDPath
	}
	command := exec.CommandContext(t.Context(), baseCommand, args...) //nolint:gosec // Explicit live integration test executes the bundled definition.
	output, errInstall := command.CombinedOutput()
	if errInstall != nil {
		t.Fatalf("install Palworld in %s: %v\n%s", gameServer.Directory, errInstall, output)
	}
	t.Logf("installed Palworld in %s (%d SteamCMD output bytes)", gameServer.Directory, len(output))
}

func prepareLivePalworldSettings(t *testing.T, instance *livePalworldInstance) {
	t.Helper()
	defaultSettingsPath := filepath.Join(instance.gameServer.Directory, palworldDefaultFile)
	defaultSettings, errRead := os.ReadFile(defaultSettingsPath)
	if errRead != nil {
		t.Fatalf("read Palworld default settings: %v", errRead)
	}
	patchedSettings, errPatch := patchPalworldSettings(
		defaultSettings,
		instance.gameServer.Name,
		instance.password,
		instance.gameServer.QueryPort,
		instance.gameServer.MaxPlayers,
	)
	if errPatch != nil {
		t.Fatalf("patch Palworld settings: %v", errPatch)
	}
	settingsPath := filepath.Join(
		instance.gameServer.Directory,
		filepath.FromSlash(palworldSettingsPath(OperatingSystem)),
	)
	errDirectory := os.MkdirAll(filepath.Dir(settingsPath), 0o750)
	if errDirectory != nil {
		t.Fatalf("create Palworld settings directory: %v", errDirectory)
	}
	errWrite := os.WriteFile(settingsPath, patchedSettings, 0o600)
	if errWrite != nil {
		t.Fatalf("write Palworld settings: %v", errWrite)
	}
}

func startLivePalworld(
	t *testing.T,
	actionInstance *Instance,
	processSupervisor *supervisor.Instance,
	gameServer *models.GameServer,
) *supervisor.Command {
	t.Helper()
	baseCommand, args, errResolve := actionInstance.resolveStructuredStartCommand(gameServer)
	if errResolve != nil {
		t.Fatalf("resolve Palworld start command: %v", errResolve)
	}
	command, errStart := processSupervisor.StartCommand(supervisor.PreparedCommand{
		ID:               gameServer.ID,
		GameServerName:   gameServer.Name,
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: gameServer.Directory,
		NodeID:           "live-palworld-node",
		ServiceID:        gameServer.GameID,
		Status:           xylona.Status_ONLINE,
		StopTimeout:      45 * time.Second,
	})
	if errStart != nil {
		t.Fatalf("start Palworld %s: %v", gameServer.Name, errStart)
	}
	return command
}

func waitForLivePalworldConsoleOutput(t *testing.T, command *supervisor.Command, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for line := range strings.SplitSeq(command.GetOutputBuffer(), "\n") {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine != "" && !strings.Contains(trimmedLine, "[Xylona]") {
				return
			}
		}
		if command.Status() == xylona.Status_OFFLINE {
			t.Fatalf("Palworld stopped before producing console output:\n%s", command.GetOutputBuffer())
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("Palworld produced no process console output within %s", timeout)
}

func waitForLivePalworldREST(
	t *testing.T,
	instance *livePalworldInstance,
	timeout time.Duration,
) *xylona.PalworldQueryInfo {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastError error
	for time.Now().Before(deadline) {
		info, errQuery := gamequery.Palworld(
			t.Context(),
			"127.0.0.1",
			int(instance.gameServer.QueryPort),
			palworldRESTUsername,
			instance.password,
		)
		if errQuery == nil && info.GetResponded() {
			return info
		}
		lastError = errQuery
		if instance.command.Status() == xylona.Status_OFFLINE {
			t.Fatalf(
				"Palworld stopped before REST became ready: %v\n%s",
				lastError,
				instance.command.GetOutputBuffer(),
			)
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("Palworld REST did not become ready within %s: %v", timeout, lastError)
	return nil
}

func stopLivePalworld(t *testing.T, command *supervisor.Command) {
	t.Helper()
	if command == nil || command.Status() == xylona.Status_OFFLINE {
		return
	}
	command.Stop("")
	deadline := time.Now().Add(30 * time.Second)
	for command.Status() != xylona.Status_OFFLINE && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
	}
	if command.Status() != xylona.Status_OFFLINE {
		t.Errorf("Palworld status = %s after stop, want OFFLINE", command.Status())
	}
}
