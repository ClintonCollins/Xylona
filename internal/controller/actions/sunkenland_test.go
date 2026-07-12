package actions

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestPostSunkenlandInstallWritesLauncherThroughOwningNode(t *testing.T) {
	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)
	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:        "sunkenland-remote",
		GameID:    "sunkenland",
		Directory: "D:/servers/sunkenland-remote",
		NodeID:    "node-remote",
	}

	errPostInstall := inst.postInstallStep(gameServer)
	if errPostInstall != nil {
		t.Fatalf("postInstallStep() error = %v", errPostInstall)
	}
	if len(remoteClient.CreateFileOrDirectoryCalls) != 1 {
		t.Fatalf("CreateFileOrDirectory call count = %d, want 1", len(remoteClient.CreateFileOrDirectoryCalls))
	}
	createCall := remoteClient.CreateFileOrDirectoryCalls[0]
	if createCall.Directory != gameServer.Directory || createCall.RelativePath != "worlds" || !createCall.IsDirectory {
		t.Errorf("CreateFileOrDirectory call = %+v", createCall)
	}
	if len(remoteClient.WriteFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(remoteClient.WriteFileCalls))
	}
	writeCall := remoteClient.WriteFileCalls[0]
	if writeCall.Directory != gameServer.Directory || writeCall.RelativePath != sunkenlandLauncherPath {
		t.Errorf("WriteFile call = %+v", writeCall)
	}
	if string(writeCall.Content) != sunkenlandLauncherSource {
		t.Error("WriteFile launcher content does not match built-in source")
	}
}

func TestSunkenlandLauncherPowerShellSyntax(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell launcher is Windows-only")
	}
	command := exec.CommandContext( // #nosec G204 -- PowerShell executable and parser script are fixed test inputs.
		context.Background(),
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-Command",
		`$parseErrors = $null; [System.Management.Automation.Language.Parser]::ParseInput([Console]::In.ReadToEnd(), [ref]$null, [ref]$parseErrors) | Out-Null; if ($parseErrors.Count -gt 0) { $parseErrors | ForEach-Object { [Console]::Error.WriteLine($_.Message) }; exit 1 }`,
	)
	command.Stdin = strings.NewReader(sunkenlandLauncherSource)
	output, errParse := command.CombinedOutput()
	if errParse != nil {
		t.Fatalf("PowerShell parser failed: %v\n%s", errParse, output)
	}
}
