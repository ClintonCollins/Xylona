package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func newSupervisorBackedNodeClient(ctx context.Context, t *testing.T, supervisorInst *supervisor.Instance, conn *db.Connection) nodeclient.NodeClient {
	t.Helper()

	embeddedNode := node.New(ctx, supervisorInst, conn)
	return nodeclient.NewInProcessClient("node-local", embeddedNode)
}

func newSupervisorBackedActionsInstance(ctx context.Context, t *testing.T, conn *db.Connection, supervisorInst *supervisor.Instance) *actions.Instance {
	t.Helper()

	client := newSupervisorBackedNodeClient(ctx, t, supervisorInst, conn)
	return actions.NewInstance(ctx, conn, client, nil, nil, versiontracker.NewVersionStateMap(), versiontracker.ResolverConfig{})
}

func wireServiceEmbeddedNode(t *testing.T, fixture *rbacRPCFixture, supervisorInst *supervisor.Instance) {
	t.Helper()

	client := newSupervisorBackedNodeClient(context.Background(), t, supervisorInst, fixture.conn)
	fixture.service.nodeRegistry = noderegistry.New("node-local", client)
	if fixture.service.actionsInst == nil {
		fixture.service.actionsInst = actions.NewInstance(
			context.Background(),
			fixture.conn,
			client,
			fixture.service.nodeRegistry,
			nil,
			versiontracker.NewVersionStateMap(),
			versiontracker.ResolverConfig{},
		)
	}
}

// seedAlternateNodeAndIP inserts a second node row plus a secondary IP so
// port-validation tests can exercise multi-node game server layouts without
// bringing up a real remote node.
func seedAlternateNodeAndIP(t *testing.T, fixture *rbacRPCFixture) {
	t.Helper()

	_, errNode := fixture.conn.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-alt"),
		Name:      omit.From("Alternate Node"),
		ListenURL: omit.From("http://node-alt.local:8081"),
		Enabled:   omit.From(true),
	})
	if errNode != nil {
		t.Fatalf("InsertNode() error = %v", errNode)
	}

	_, errIP := fixture.conn.UpsertIP(&models.IPSetter{
		Address:            omit.From("127.0.0.2"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
		NodeID:             omit.From("node-alt"),
	})
	if errIP != nil {
		t.Fatalf("UpsertIP() error = %v", errIP)
	}
}

const testGameID = "test-game"

func seedTestGame(t *testing.T, fixture *rbacRPCFixture) {
	t.Helper()

	now := time.Now().UTC()
	_, errInsert := fixture.conn.InsertGame(fixture.conn.DB, &models.GameSetter{
		ID:                  omit.From(testGameID),
		Name:                omit.From("Test Game"),
		DefaultPort:         omit.From(int64(28000)),
		DefaultQueryPort:    omit.From(int64(28001)),
		DefaultMaxPlayers:   omit.From(int64(48)),
		WindowsSupport:      omit.From(true),
		LinuxAllowBackups:   omit.From(true),
		WindowsAllowBackups: omit.From(true),
		CreatedAt:           omit.From(now),
		UpdatedAt:           omit.From(now),
	})
	if errInsert != nil {
		t.Fatalf("InsertGame() error = %v", errInsert)
	}
}
