package actions

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestPostInstallDoesNotAcceptMinecraftEULA(t *testing.T) {
	controllerDir := t.TempDir()
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:        "server-remote",
		GameID:    "minecraft",
		Directory: controllerDir,
		NodeID:    "node-remote",
	}

	errPostInstall := inst.postInstallStep(gameServer)
	if errPostInstall != nil {
		t.Fatalf("postInstallStep() error = %v", errPostInstall)
	}

	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
	_, errStat := os.Stat(filepath.Join(controllerDir, "eula.txt"))
	if !os.IsNotExist(errStat) {
		t.Fatalf("eula.txt stat error = %v, want not exist", errStat)
	}
}

func TestPost7DaysToDieInstallCopiesConfigThroughRemoteNodeClient(t *testing.T) {
	controllerDir := t.TempDir()
	settingsPath := filepath.Join(controllerDir, "settings.xml")
	serverConfigPath := filepath.Join(controllerDir, "serverconfig.xml")
	errWrite := os.WriteFile(serverConfigPath, []byte("<controller />"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(serverconfig.xml) error = %v", errWrite)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:        "server-remote-7dtd",
		GameID:    "7_days_to_die",
		Directory: controllerDir,
		NodeID:    "node-remote",
	}

	errPostInstall := inst.postInstallStep(gameServer)
	if errPostInstall != nil {
		t.Fatalf("postInstallStep() error = %v", errPostInstall)
	}

	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
	if len(remoteClient.CopyFilesCalls) != 1 {
		t.Fatalf("CopyFiles call count = %d, want 1", len(remoteClient.CopyFilesCalls))
	}
	copyCall := remoteClient.CopyFilesCalls[0]
	if copyCall.Directory != gameServer.Directory {
		t.Fatalf("CopyFiles directory = %q, want %q", copyCall.Directory, gameServer.Directory)
	}
	if !reflect.DeepEqual(copyCall.Operations, []node.CopyFileOperation{
		{
			SourceRelativePath:      "serverconfig.xml",
			DestinationRelativePath: "settings.xml",
		},
	}) {
		t.Fatalf("CopyFiles operations = %+v, want serverconfig.xml -> settings.xml", copyCall.Operations)
	}
	_, errStat := os.Stat(settingsPath)
	if !os.IsNotExist(errStat) {
		t.Fatalf("controller settings.xml stat error = %v, want not exist", errStat)
	}
}

func TestEnsureMinecraftServerExecutableDiscoversRemoteJar(t *testing.T) {
	jarBuildDir := t.TempDir()
	createVersionTestMinecraftJar(t, jarBuildDir, "paper-1.21.4-100.jar", "1.21.4")

	jarBytes, errRead := os.ReadFile(filepath.Join(jarBuildDir, "paper-1.21.4-100.jar"))
	if errRead != nil {
		t.Fatalf("os.ReadFile() error = %v", errRead)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		ListFilesResult: []node.FileEntry{
			node.NewFileEntry("paper-1.21.4-100.jar", int64(len(jarBytes)), false, time.Now()),
			node.NewFileEntry("readme.txt", 42, false, time.Now()),
		},
		ProbeInstalledVersionResult: node.InstalledVersionProbeResult{
			Found:      true,
			Version:    "1.21.4",
			SourcePath: "paper-1.21.4-100.jar",
		},
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)

	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:             "server-remote",
		GameID:         "minecraft",
		Directory:      "/home/clinton/xylona/clinton/media-minecraft-server",
		NodeID:         "node-remote",
		ServerSoftware: null.From("paper"),
	}

	errEnsure := inst.ensureMinecraftServerExecutable(gameServer)
	if errEnsure != nil {
		t.Fatalf("ensureMinecraftServerExecutable() error = %v", errEnsure)
	}

	if gameServer.ServerExecutable.GetOr("") != "paper-1.21.4-100.jar" {
		t.Fatalf(
			"server executable = %q, want %q",
			gameServer.ServerExecutable.GetOr(""),
			"paper-1.21.4-100.jar",
		)
	}

	if len(remoteClient.ListFilesCalls) != 1 {
		t.Fatalf("ListFiles call count = %d, want 1", len(remoteClient.ListFilesCalls))
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
	if len(remoteClient.ProbeInstalledVersionCalls) != 1 {
		t.Fatalf("ProbeInstalledVersion call count = %d, want 1", len(remoteClient.ProbeInstalledVersionCalls))
	}
	probeCall := remoteClient.ProbeInstalledVersionCalls[0]
	if probeCall.Kind != node.InstalledVersionProbeKindMinecraftJar {
		t.Fatalf("ProbeInstalledVersion kind = %v, want minecraft jar", probeCall.Kind)
	}
	if !reflect.DeepEqual(probeCall.RelativePaths, []string{"paper-1.21.4-100.jar"}) {
		t.Fatalf("ProbeInstalledVersion relative paths = %v, want [paper-1.21.4-100.jar]", probeCall.RelativePaths)
	}
}
