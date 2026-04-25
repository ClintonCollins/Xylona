package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
)

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
