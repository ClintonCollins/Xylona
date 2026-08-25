package rpc

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestPrepareSevenDaysToDiePrivateRead(t *testing.T) {
	tests := []struct {
		name             string
		configure        func(*rbacRPCFixture, *nodeclient.FakeNodeClient)
		wantOutcome      sevenDaysToDiePrivateReadOutcome
		wantErr          bool
		wantCode         connect.Code
		wantReady        bool
		wantProcessCalls int
		wantRuntimeCalls int
	}{
		{
			name: "missing node client", configure: func(fixture *rbacRPCFixture, _ *nodeclient.FakeNodeClient) {
				fixture.service.nodeRegistry = nil
			},
			wantOutcome: sevenDaysToDiePrivateReadNodeUnavailable,
		},
		{
			name: "process transport failure", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.GetProcessSnapshotErr = errors.New("node transport failed")
			},
			wantOutcome: sevenDaysToDiePrivateReadNodeUnavailable, wantProcessCalls: 1,
		},
		{
			name: "missing process", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.GetProcessSnapshotResult = nil
				client.GetProcessSnapshotFound = false
			},
			wantOutcome: sevenDaysToDiePrivateReadServerOffline, wantProcessCalls: 1,
		},
		{
			name: "nil process", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.GetProcessSnapshotResult = nil
				client.GetProcessSnapshotFound = true
			},
			wantOutcome: sevenDaysToDiePrivateReadServerOffline, wantProcessCalls: 1,
		},
		{
			name: "offline process", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.GetProcessSnapshotResult = &node.ProcessSnapshot{Status: xylona.Status_OFFLINE.String()}
			},
			wantOutcome: sevenDaysToDiePrivateReadServerOffline, wantProcessCalls: 1,
		},
		{
			name: "runtime capability failure", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.RuntimeCapabilitiesErr = errors.New("runtime capabilities unavailable")
			},
			wantOutcome: sevenDaysToDiePrivateReadRuntimeUnavailable, wantProcessCalls: 1, wantRuntimeCalls: 1,
		},
		{
			name: "old protocol", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.RuntimeCapabilitiesResult.ProtocolVersion = sevenDaysToDiePrivateWebAPINodeProtocol - 1
			},
			wantOutcome: sevenDaysToDiePrivateReadUnsupported, wantProcessCalls: 1, wantRuntimeCalls: 1,
		},
		{
			name: "unknown protocol", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.RuntimeCapabilitiesResult.ProtocolVersion = 0
			},
			wantOutcome: sevenDaysToDiePrivateReadUnsupported, wantProcessCalls: 1, wantRuntimeCalls: 1,
		},
		{
			name: "process canceled", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.GetProcessSnapshotErr = context.Canceled
			},
			wantErr: true, wantCode: connect.CodeCanceled, wantProcessCalls: 1,
		},
		{
			name: "process deadline exceeded", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.GetProcessSnapshotErr = context.DeadlineExceeded
			},
			wantErr: true, wantCode: connect.CodeDeadlineExceeded, wantProcessCalls: 1,
		},
		{
			name: "runtime capabilities canceled", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.RuntimeCapabilitiesErr = context.Canceled
			},
			wantErr: true, wantCode: connect.CodeCanceled, wantProcessCalls: 1, wantRuntimeCalls: 1,
		},
		{
			name: "runtime capabilities deadline exceeded", configure: func(_ *rbacRPCFixture, client *nodeclient.FakeNodeClient) {
				client.RuntimeCapabilitiesErr = context.DeadlineExceeded
			},
			wantErr: true, wantCode: connect.CodeDeadlineExceeded, wantProcessCalls: 1, wantRuntimeCalls: 1,
		},
		{
			name: "missing credential provider", configure: func(fixture *rbacRPCFixture, _ *nodeclient.FakeNodeClient) {
				fixture.service.actionsInst = nil
			},
			wantErr: true, wantCode: connect.CodeInternal, wantProcessCalls: 1, wantRuntimeCalls: 1,
		},
		{
			name: "credential derivation failure", configure: func(fixture *rbacRPCFixture, _ *nodeclient.FakeNodeClient) {
				fixture.conn.SetEncryptionKey(nil)
			},
			wantErr: true, wantCode: connect.CodeInternal, wantProcessCalls: 1, wantRuntimeCalls: 1,
		},
		{
			name: "ready", wantOutcome: sevenDaysToDiePrivateReadReady, wantReady: true,
			wantProcessCalls: 1, wantRuntimeCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, gameServer, client := newPrivateReadGateFixture(t)
			if test.configure != nil {
				test.configure(fixture, client)
			}

			access, outcome, errPrepare := fixture.service.prepareSevenDaysToDiePrivateRead(t.Context(), gameServer)
			switch {
			case test.wantErr:
				if connect.CodeOf(errPrepare) != test.wantCode {
					t.Fatalf("prepareSevenDaysToDiePrivateRead() code = %v, want %v", connect.CodeOf(errPrepare), test.wantCode)
				}
			case errPrepare != nil:
				t.Fatalf("prepareSevenDaysToDiePrivateRead() error = %v", errPrepare)
			case outcome != test.wantOutcome:
				t.Fatalf("prepareSevenDaysToDiePrivateRead() outcome = %v, want %v", outcome, test.wantOutcome)
			}
			if len(client.GetProcessSnapshotCalls) != test.wantProcessCalls {
				t.Fatalf("process snapshot calls = %d, want %d", len(client.GetProcessSnapshotCalls), test.wantProcessCalls)
			}
			if client.RuntimeCapabilitiesCalls != test.wantRuntimeCalls {
				t.Fatalf("runtime capability calls = %d, want %d", client.RuntimeCapabilitiesCalls, test.wantRuntimeCalls)
			}

			if test.wantReady {
				if access.client != client || access.workingDirectory != gameServer.Directory || access.tokenName == "" || access.tokenSecret == "" {
					t.Fatal("prepared access did not contain the expected node, directory, and credentials")
				}
			} else if access.client != nil || access.workingDirectory != "" || access.tokenName != "" || access.tokenSecret != "" {
				t.Fatal("non-ready access was populated")
			}
		})
	}
}

func newPrivateReadGateFixture(
	t *testing.T,
) (*rbacRPCFixture, *models.GameServer, *nodeclient.FakeNodeClient) {
	t.Helper()

	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
	client := &nodeclient.FakeNodeClient{
		NodeID:                   "node-local",
		GetProcessSnapshotResult: &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
		GetProcessSnapshotFound:  true,
		RuntimeCapabilitiesResult: node.RuntimeCapabilities{
			ProtocolVersion: node.RuntimeProtocolVersion,
		},
	}
	registry := noderegistry.New("node-local", client)
	fixture.service.nodeRegistry = registry
	actionsContext, cancelActions := context.WithCancel(t.Context())
	t.Cleanup(cancelActions)
	fixture.service.actionsInst = actions.NewInstance(
		actionsContext,
		fixture.conn,
		client,
		registry,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)
	gameServer, errServer := fixture.service.getGameServerFromID("server-local-1")
	if errServer != nil {
		t.Fatalf("get 7 Days to Die test server: %v", errServer)
	}
	return fixture, gameServer, client
}
