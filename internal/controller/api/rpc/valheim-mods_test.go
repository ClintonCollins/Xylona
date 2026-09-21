package rpc

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/modmanager"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestModMutationError(t *testing.T) {
	errResponse := modMutationError(fmt.Errorf("private path: %w", modmanager.ErrValheimServerRunning), "install")
	if connect.CodeOf(errResponse) != connect.CodeFailedPrecondition || strings.Contains(errResponse.Error(), "private path") {
		t.Fatalf("mutation error = %v", errResponse)
	}
}

func TestValheimModsRequireStoppedNodeProcess(t *testing.T) {
	for _, test := range []struct {
		name    string
		found   bool
		process *node.ProcessSnapshot
		err     error
		want    connect.Code
	}{
		{name: "not started"},
		{name: "offline", found: true, process: &node.ProcessSnapshot{Status: xylona.Status_OFFLINE.String()}},
		{name: "running", found: true, process: &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()}, want: connect.CodeFailedPrecondition},
		{name: "unknown", found: true, process: &node.ProcessSnapshot{Status: xylona.Status_UNKNOWN.String()}, want: connect.CodeFailedPrecondition},
		{name: "missing process data", found: true, want: connect.CodeFailedPrecondition},
		{name: "unreachable", err: errors.New("unreachable"), want: connect.CodeUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			client := &nodeclient.FakeNodeClient{NodeID: "node-local", GetProcessSnapshotFound: test.found, GetProcessSnapshotResult: test.process, GetProcessSnapshotErr: test.err}
			fixture.service.nodeRegistry = testParityRegistry(client, nil)
			server := &models.GameServer{ID: "server-local-1", NodeID: "node-local", GameID: "valheim"}
			errStopped := fixture.service.ensureValheimModsStopped(t.Context(), server)
			if test.want == 0 && errStopped != nil {
				t.Fatal(errStopped)
			}
			if test.want != 0 && connect.CodeOf(errStopped) != test.want {
				t.Fatalf("error = %v, want %v", errStopped, test.want)
			}
		})
	}
}
