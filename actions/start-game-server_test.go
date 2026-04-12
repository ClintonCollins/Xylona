package actions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/db/dbtest"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

func TestStartGameServerPreservesServerNodeID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	conn := dbtest.NewMigratedConnection(t, "start-game-server-node-id.sqlite")
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	inst := NewInstance(
		ctx,
		conn,
		supervisorInst,
		nil,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	gameServer := &models.GameServer{
		ID:               "server-start-node-id",
		UserID:           "user-start-node-id",
		Name:             "Start Node ID Server",
		GameID:           "game-start-node-id",
		Directory:        t.TempDir(),
		NodeID:           "node-local",
		StartArgsPatches: "[]",
	}
	gameServer.R.Game = &models.Game{
		LinuxBaseCommand:         "sh",
		LinuxStartArgsTemplate:   null.From(`[{"id":"exit","order":1,"ownership":"editable","tokens":["-c","exit 0"]}]`),
		WindowsBaseCommand:       "cmd",
		WindowsStartArgsTemplate: null.From(`[{"id":"exit","order":1,"ownership":"editable","tokens":["/c","exit 0"]}]`),
	}

	inst.StartGameServer(gameServer)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cmd, errGetCommand := supervisorInst.GetCommandByID(gameServer.ID)
		if errGetCommand != nil {
			if errors.Is(errGetCommand, supervisor.ErrCommandDoesNotExist) {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			t.Fatalf("GetCommandByID() error = %v", errGetCommand)
		}

		if cmd.NodeID() == gameServer.NodeID {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	cmd, errGetCommand := supervisorInst.GetCommandByID(gameServer.ID)
	if errGetCommand != nil {
		t.Fatalf("GetCommandByID() after start error = %v", errGetCommand)
	}
	t.Fatalf("command node ID = %q, want %q", cmd.NodeID(), gameServer.NodeID)
}
