package actions

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type cancellationAwareStopClient struct {
	*nodeclient.FakeNodeClient
}

func (c *cancellationAwareStopClient) StopProcess(ctx context.Context, _ string, _ string) error {
	<-ctx.Done()
	return fmt.Errorf("stop process wait: %w", ctx.Err())
}

func TestResolveNodeClientFailsClosedForMissingRemoteRegistryClient(t *testing.T) {
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	inst := &Instance{
		ctx:                context.Background(),
		embeddedNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-local"},
		nodeRegistry:       registry,
	}

	client, errResolve := inst.resolveNodeClient("node-remote")
	if errResolve == nil {
		t.Fatal("resolveNodeClient(missing remote) error = nil, want failure")
	}
	if client != nil {
		t.Fatalf("resolveNodeClient(missing remote) client = %T, want nil", client)
	}
	if !errors.Is(errResolve, noderegistry.ErrNodeNotRegistered) {
		t.Fatalf("resolveNodeClient(missing remote) error = %v, want %v", errResolve, noderegistry.ErrNodeNotRegistered)
	}
}

func TestStopGameServerReportsNodeFailureAndClearsIntent(t *testing.T) {
	stopFailure := errors.New("remote transport unavailable")
	client := &nodeclient.FakeNodeClient{
		NodeID:         "node-remote",
		SnapshotResult: &node.NodeSnapshot{OS: "linux"},
		StopProcessErr: stopFailure,
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(client)
	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{
		ID:     "server-remote-1",
		NodeID: "node-remote",
	}
	gameServer.R.Game = &models.Game{}

	errStop := inst.StopGameServer(t.Context(), gameServer)
	if !errors.Is(errStop, stopFailure) {
		t.Fatalf("StopGameServer() error = %v, want wrapped %v", errStop, stopFailure)
	}
	if inst.intentionalStops.take(gameServer.ID) {
		t.Fatal("StopGameServer() retained intentional-stop marker after failed stop")
	}
}

func TestStopGameServerPropagatesCallerCancellation(t *testing.T) {
	client := &cancellationAwareStopClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{
			NodeID:         "node-remote",
			SnapshotResult: &node.NodeSnapshot{OS: "linux"},
		},
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(client)
	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}
	gameServer := &models.GameServer{ID: "server-remote-1", NodeID: "node-remote"}
	gameServer.R.Game = &models.Game{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	errStop := inst.StopGameServer(ctx, gameServer)
	if !errors.Is(errStop, context.Canceled) {
		t.Fatalf("StopGameServer() error = %v, want %v", errStop, context.Canceled)
	}
	if inst.intentionalStops.take(gameServer.ID) {
		t.Fatal("StopGameServer() retained intentional-stop marker after canceled stop")
	}
}

func TestReadGameServerBufferPropagatesNodeFailure(t *testing.T) {
	readFailure := errors.New("remote console unavailable")
	client := &nodeclient.FakeNodeClient{
		NodeID:               "node-remote",
		ReadConsoleBufferErr: readFailure,
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(client)
	inst := &Instance{ctx: context.Background(), nodeRegistry: registry}

	output, errRead := inst.ReadGameServerBuffer(t.Context(), &models.GameServer{
		ID:     "server-remote-1",
		NodeID: "node-remote",
	})
	if !errors.Is(errRead, readFailure) {
		t.Fatalf("ReadGameServerBuffer() error = %v, want wrapped %v", errRead, readFailure)
	}
	if output != "" {
		t.Fatalf("ReadGameServerBuffer() output = %q, want empty on failure", output)
	}
}

func TestUpdateGameServerFailsClosedWhenRuntimeStatusUnavailable(t *testing.T) {
	client := &nodeclient.FakeNodeClient{
		NodeID:                "node-remote",
		GetProcessSnapshotErr: errors.New("snapshot unavailable"),
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(client)
	inst := &Instance{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}

	errUpdate := inst.UpdateGameServer(&models.GameServer{ID: "server-remote-1", NodeID: "node-remote"})
	if errUpdate == nil {
		t.Fatal("UpdateGameServer() error = nil, want unavailable runtime status failure")
	}
	if len(client.StartProcessCalls) != 0 {
		t.Fatalf("StartProcess call count = %d, want 0", len(client.StartProcessCalls))
	}
}
