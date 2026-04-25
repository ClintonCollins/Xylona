package actions

import (
	"context"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
)

func newTestInstance(t *testing.T) *Instance {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	conn := dbtest.NewMigratedConnection(t, "test-actions.sqlite")
	t.Cleanup(cancel)

	return NewInstance(ctx, conn, nil, nil, nil, versiontracker.NewVersionStateMap(), versiontracker.ResolverConfig{})
}

func newSupervisorBackedNodeClient(ctx context.Context, t *testing.T, supervisorInst *supervisor.Instance, conn *db.Connection) nodeclient.NodeClient {
	t.Helper()

	embeddedNode := node.New(ctx, supervisorInst, conn)
	return nodeclient.NewInProcessClient("node-local", embeddedNode)
}
