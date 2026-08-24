package rpc

import (
	"context"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestBuildProtectionPolicyUsesTargetNodeOS(t *testing.T) {
	t.Parallel()

	controllerCommand := "./start.sh"
	remoteCommand := "start.bat"
	remoteOS := "windows"
	if runtime.GOOS == "windows" {
		controllerCommand = "start.bat"
		remoteCommand = "./start.sh"
		remoteOS = "linux"
	}

	registry := noderegistry.New("node-local", nil)
	registry.Register(&nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		SnapshotResult: &node.NodeSnapshot{
			OS: remoteOS,
		},
	})

	xs := &XylonaService{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		NodeID:           "node-remote",
		ServerExecutable: null.From("server.jar"),
	}
	setGameServerRelation(t, gameServer, &models.Game{
		WindowsBaseCommand: "start.bat",
		LinuxBaseCommand:   "./start.sh",
	})

	policy := xs.buildProtectionPolicy(gameServer)
	if policy.ServerExecutable != "server.jar" {
		t.Fatalf("buildProtectionPolicy().ServerExecutable = %q, want %q", policy.ServerExecutable, "server.jar")
	}
	if policy.BaseCommand != remoteCommand {
		t.Fatalf("buildProtectionPolicy().BaseCommand = %q, want %q (controller command %q)", policy.BaseCommand, remoteCommand, controllerCommand)
	}
}

func TestBuildProtectionPolicyUsesBaseCommandOverride(t *testing.T) {
	t.Parallel()

	xs := &XylonaService{}
	gameServer := &models.GameServer{
		BaseCommandOverride: " ./custom-start.sh ",
		ServerExecutable:    null.From("server.jar"),
	}

	policy := xs.buildProtectionPolicy(gameServer)
	if policy.BaseCommand != "./custom-start.sh" {
		t.Fatalf("buildProtectionPolicy().BaseCommand = %q, want %q", policy.BaseCommand, "./custom-start.sh")
	}
	if policy.ServerExecutable != "server.jar" {
		t.Fatalf("buildProtectionPolicy().ServerExecutable = %q, want %q", policy.ServerExecutable, "server.jar")
	}
}

func setGameServerRelation(t *testing.T, gameServer *models.GameServer, game *models.Game) {
	t.Helper()

	relationsValue := reflect.ValueOf(gameServer).Elem().FieldByName("R")
	gameField := relationsValue.FieldByName("Game")
	reflect.NewAt(gameField.Type(), unsafe.Pointer(gameField.UnsafeAddr())).Elem().Set(reflect.ValueOf(game))
}
