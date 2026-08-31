package rpc

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestUpdateGameServerPersistsSelectedSteamBranch(t *testing.T) {
	tests := []struct {
		name                string
		target              string
		operationInProgress bool
		wantErr             bool
		wantCode            connect.Code
		wantBranch          string
	}{
		{name: "accepted update", target: "v2.5", wantBranch: "v2.5"},
		{name: "rejected concurrent update", target: "v2.5", operationInProgress: true, wantErr: true, wantCode: connect.CodeAlreadyExists, wantBranch: "public"},
		{name: "rejects steam injection target", target: "latest +force_install_dir /tmp/pwn", wantErr: true, wantCode: connect.CodeInvalidArgument, wantBranch: "public"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)

			_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
				ID:     omit.From("server-local-1"),
				GameID: omit.From("7_days_to_die"),
				Branch: omit.From("public"),
			})
			if errUpdateServer != nil {
				t.Fatalf("UpdateGameServer() setup error = %v", errUpdateServer)
			}

			fixture.service.actionsInst = actions.NewInstance(
				t.Context(),
				fixture.conn,
				&nodeclient.FakeNodeClient{NodeID: "node-local"},
				nil,
				nil,
				versiontracker.NewVersionStateMap(),
				versiontracker.ResolverConfig{},
			)

			if test.operationInProgress {
				releaseOperation, errBegin := fixture.service.actionsInst.TryBeginGameServerLifecycleOperation("server-local-1")
				if errBegin != nil {
					t.Fatalf("TryBeginGameServerLifecycleOperation() error = %v", errBegin)
				}
				t.Cleanup(releaseOperation)
			}

			request := connect.NewRequest(&xylona.UpdateGameServerRequest{
				ServerId: "server-local-1",
				Target:   test.target,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

			_, errUpdate := fixture.service.UpdateGameServer(t.Context(), request)
			if !test.wantErr && errUpdate != nil {
				t.Fatalf("UpdateGameServer() error = %v", errUpdate)
			}
			if test.wantErr && connect.CodeOf(errUpdate) != test.wantCode {
				t.Fatalf("UpdateGameServer() code = %v, want %v (error %v)", connect.CodeOf(errUpdate), test.wantCode, errUpdate)
			}

			gameServer, errGetServer := fixture.conn.GetGameServerByID("server-local-1")
			if errGetServer != nil {
				t.Fatalf("GetGameServerByID() error = %v", errGetServer)
			}
			if gameServer.Branch != test.wantBranch {
				t.Fatalf("GetGameServerByID().Branch = %q, want %q", gameServer.Branch, test.wantBranch)
			}
		})
	}
}

func TestMergeEditableGameServerUpdatePreservesStartCommandOverrides(t *testing.T) {
	existing := &models.GameServer{
		ID:                  "server-1",
		StartArgsPatches:    `[{"id":"locked","op":"remove"}]`,
		BaseCommandOverride: "./custom-start.sh",
	}
	incoming := &models.GameServer{
		ID:                  "changed-id",
		StartArgsPatches:    "[]",
		BaseCommandOverride: "./tampered.sh",
	}

	for _, allowProvisioningChanges := range []bool{false, true} {
		merged := mergeEditableGameServerUpdate(existing, incoming, allowProvisioningChanges)
		if merged.StartArgsPatches != existing.StartArgsPatches {
			t.Fatalf("allowProvisioningChanges %v: StartArgsPatches = %q, want %q", allowProvisioningChanges, merged.StartArgsPatches, existing.StartArgsPatches)
		}
		if merged.BaseCommandOverride != existing.BaseCommandOverride {
			t.Fatalf("allowProvisioningChanges %v: BaseCommandOverride = %q, want %q", allowProvisioningChanges, merged.BaseCommandOverride, existing.BaseCommandOverride)
		}
	}
}
