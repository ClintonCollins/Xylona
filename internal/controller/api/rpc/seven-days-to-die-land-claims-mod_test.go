package rpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestInstallSevenDaysToDieLandClaimsMod(t *testing.T) {
	t.Run("requires authentication", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		request := connect.NewRequest(&xylona.InstallSevenDaysToDieLandClaimsModRequest{GameServerId: "server-local-1"})

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if connect.CodeOf(errInstall) != connect.CodeUnauthenticated {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() code = %v, want %v", connect.CodeOf(errInstall), connect.CodeUnauthenticated)
		}
	})

	t.Run("rejects other games", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		request := connect.NewRequest(&xylona.InstallSevenDaysToDieLandClaimsModRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if connect.CodeOf(errInstall) != connect.CodeFailedPrecondition {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() code = %v, want %v", connect.CodeOf(errInstall), connect.CodeFailedPrecondition)
		}
	})

	t.Run("requires settings permission", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-local")
		request := connect.NewRequest(&xylona.InstallSevenDaysToDieLandClaimsModRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if connect.CodeOf(errInstall) != connect.CodePermissionDenied {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() code = %v, want %v", connect.CodeOf(errInstall), connect.CodePermissionDenied)
		}
		if len(client.StatFileCalls) != 0 || len(client.WriteFileCalls) != 0 {
			t.Fatalf("node calls made without permission: stat = %d, write = %d", len(client.StatFileCalls), len(client.WriteFileCalls))
		}
	})

	t.Run("installs while the server is online", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-local")
		client.GetProcessSnapshotResult = &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()}
		client.GetProcessSnapshotFound = true
		request := landClaimsInstallerRequest(t, fixture)

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if errInstall != nil {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() error = %v", errInstall)
		}
		assertLandClaimsInstallerCalls(t, client, 1)
	})

	t.Run("installs without a runtime status", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-local")
		client.GetProcessSnapshotErr = errors.New("node unavailable")
		request := landClaimsInstallerRequest(t, fixture)

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if errInstall != nil {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() error = %v", errInstall)
		}
		assertLandClaimsInstallerCalls(t, client, 1)
	})

	t.Run("installs the v2.6 build when the legacy WebServer exists", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-local")
		request := landClaimsInstallerRequest(t, fixture)

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if errInstall != nil {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() error = %v", errInstall)
		}
		assertLandClaimsInstallerCalls(t, client, 1)
	})

	t.Run("installs and repairs the v3 build on the owning remote node", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-remote")
		client.StatFileErr = os.ErrNotExist
		request := landClaimsInstallerRequest(t, fixture)

		for range 2 {
			_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
			if errInstall != nil {
				t.Fatalf("InstallSevenDaysToDieLandClaimsMod() error = %v", errInstall)
			}
		}
		assertLandClaimsInstallerCalls(t, client, 2)
	})

	t.Run("does not guess the version after a stat failure", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-local")
		client.StatFileErr = errors.New("node unavailable")
		request := landClaimsInstallerRequest(t, fixture)

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if connect.CodeOf(errInstall) != connect.CodeUnavailable {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() code = %v, want %v", connect.CodeOf(errInstall), connect.CodeUnavailable)
		}
		if len(client.CreateFileOrDirectoryCalls) != 0 || len(client.WriteFileCalls) != 0 {
			t.Fatalf("install calls after stat failure: create = %d, write = %d", len(client.CreateFileOrDirectoryCalls), len(client.WriteFileCalls))
		}
	})

	t.Run("reports directory creation failure", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-local")
		client.CreateFileOrDirectoryErr = errors.New("create failed")
		request := landClaimsInstallerRequest(t, fixture)

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if connect.CodeOf(errInstall) != connect.CodeUnavailable {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() code = %v, want %v", connect.CodeOf(errInstall), connect.CodeUnavailable)
		}
		if len(client.WriteFileCalls) != 0 {
			t.Fatalf("write calls after create failure = %d, want 0", len(client.WriteFileCalls))
		}
	})

	writeFailureTests := []struct {
		name       string
		failAtCall int
	}{
		{name: "reports DLL write failure", failAtCall: 1},
		{name: "reports metadata write failure", failAtCall: 2},
	}
	for _, test := range writeFailureTests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_OFFLINE.String(), "node-local")
			client := &landClaimsWriteFailureClient{
				FakeNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-local"},
				failAtCall:     test.failAtCall,
			}
			fixture.service.nodeRegistry = noderegistry.New("node-local", client)
			request := landClaimsInstallerRequest(t, fixture)

			_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
			if connect.CodeOf(errInstall) != connect.CodeUnavailable {
				t.Fatalf("InstallSevenDaysToDieLandClaimsMod() code = %v, want %v", connect.CodeOf(errInstall), connect.CodeUnavailable)
			}
			if len(client.WriteFileCalls) != test.failAtCall {
				t.Fatalf("write calls = %d, want %d", len(client.WriteFileCalls), test.failAtCall)
			}
		})
	}
}

type landClaimsWriteFailureClient struct {
	*nodeclient.FakeNodeClient
	failAtCall int
	writeCalls int
}

func (client *landClaimsWriteFailureClient) WriteFile(
	ctx context.Context,
	directory string,
	relativePath string,
	content []byte,
	policy node.ProtectionPolicy,
) error {
	client.writeCalls++
	errRecord := client.FakeNodeClient.WriteFile(ctx, directory, relativePath, content, policy)
	if errRecord != nil {
		return fmt.Errorf("record write: %w", errRecord)
	}
	if client.writeCalls == client.failAtCall {
		return errors.New("write failed")
	}
	return nil
}

func newLandClaimsInstallerFixture(t *testing.T, nodeID string) (*rbacRPCFixture, *nodeclient.FakeNodeClient) {
	t.Helper()
	fixture := newRBACRPCFixture(t)
	if nodeID != "node-local" {
		insertRemoteNodeForParityTests(t, fixture, nodeID)
		insertNodeScopedIPForParityTests(t, fixture, nodeID, "127.0.0.2")
	}
	setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_OFFLINE.String(), nodeID)

	localClient := &nodeclient.FakeNodeClient{NodeID: "node-local"}
	client := localClient
	registry := noderegistry.New("node-local", localClient)
	if nodeID != "node-local" {
		client = &nodeclient.FakeNodeClient{NodeID: nodeID}
		registry.Register(client)
	}
	fixture.service.nodeRegistry = registry
	return fixture, client
}

func landClaimsInstallerRequest(
	t *testing.T,
	fixture *rbacRPCFixture,
) *connect.Request[xylona.InstallSevenDaysToDieLandClaimsModRequest] {
	t.Helper()
	request := connect.NewRequest(&xylona.InstallSevenDaysToDieLandClaimsModRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	return request
}

func assertLandClaimsInstallerCalls(
	t *testing.T,
	client *nodeclient.FakeNodeClient,
	wantInstallCount int,
) {
	t.Helper()
	if len(client.StatFileCalls) != wantInstallCount {
		t.Fatalf("stat calls = %d, want %d", len(client.StatFileCalls), wantInstallCount)
	}
	for _, call := range client.StatFileCalls {
		if call.Directory != "/tmp/server-local-1" || call.RelativePath != "Mods/TFP_WebServer/WebServer.dll" {
			t.Fatalf("stat call = %+v", call)
		}
	}
	if len(client.CreateFileOrDirectoryCalls) != wantInstallCount {
		t.Fatalf("create calls = %d, want %d", len(client.CreateFileOrDirectoryCalls), wantInstallCount)
	}
	for _, call := range client.CreateFileOrDirectoryCalls {
		if call.Directory != "/tmp/server-local-1" || call.RelativePath != "Mods/Xylona_LandClaims" || !call.IsDirectory {
			t.Fatalf("create call = %+v", call)
		}
	}
	if len(client.WriteFileCalls) != wantInstallCount*2 {
		t.Fatalf("write calls = %d, want %d", len(client.WriteFileCalls), wantInstallCount*2)
	}
	for installIndex := range wantInstallCount {
		dllCall := client.WriteFileCalls[installIndex*2]
		if dllCall.Directory != "/tmp/server-local-1" ||
			dllCall.RelativePath != "Mods/Xylona_LandClaims/XylonaLandClaims.dll" ||
			len(dllCall.Content) == 0 {
			t.Fatalf("DLL write call = %+v", dllCall)
		}
		modInfoCall := client.WriteFileCalls[installIndex*2+1]
		if modInfoCall.Directory != "/tmp/server-local-1" ||
			modInfoCall.RelativePath != "Mods/Xylona_LandClaims/ModInfo.xml" ||
			len(modInfoCall.Content) == 0 {
			t.Fatalf("ModInfo write call = %+v", modInfoCall)
		}
	}
}
