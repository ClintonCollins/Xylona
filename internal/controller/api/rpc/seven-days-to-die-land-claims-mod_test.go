package rpc

import (
	"bytes"
	"context"
	"debug/pe"
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

	t.Run("requires the server to be offline", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-local")
		client.GetProcessSnapshotResult = &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()}
		client.GetProcessSnapshotFound = true
		request := landClaimsInstallerRequest(t, fixture)

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if connect.CodeOf(errInstall) != connect.CodeFailedPrecondition {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() code = %v, want %v", connect.CodeOf(errInstall), connect.CodeFailedPrecondition)
		}
		if len(client.StatFileCalls) != 0 || len(client.WriteFileCalls) != 0 {
			t.Fatalf("node file calls made for online server: stat = %d, write = %d", len(client.StatFileCalls), len(client.WriteFileCalls))
		}
	})

	t.Run("rejects an unknown server status", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-local")
		client.GetProcessSnapshotErr = errors.New("node unavailable")
		request := landClaimsInstallerRequest(t, fixture)

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if connect.CodeOf(errInstall) != connect.CodeFailedPrecondition {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() code = %v, want %v", connect.CodeOf(errInstall), connect.CodeFailedPrecondition)
		}
		if len(client.StatFileCalls) != 0 || len(client.WriteFileCalls) != 0 {
			t.Fatalf("node file calls made with unknown status: stat = %d, write = %d", len(client.StatFileCalls), len(client.WriteFileCalls))
		}
	})

	t.Run("installs the v2.6 build when the legacy WebServer exists", func(t *testing.T) {
		fixture, client := newLandClaimsInstallerFixture(t, "node-local")
		request := landClaimsInstallerRequest(t, fixture)

		_, errInstall := fixture.service.InstallSevenDaysToDieLandClaimsMod(t.Context(), request)
		if errInstall != nil {
			t.Fatalf("InstallSevenDaysToDieLandClaimsMod() error = %v", errInstall)
		}
		assertLandClaimsInstallerCalls(t, client, sevenDaysToDieLandClaimsV26DLL, 1)
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
		assertLandClaimsInstallerCalls(t, client, sevenDaysToDieLandClaimsV3DLL, 2)
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

func TestSevenDaysToDieLandClaimsEmbeddedAssets(t *testing.T) {
	if len(sevenDaysToDieLandClaimsModInfo) == 0 {
		t.Fatal("embedded ModInfo.xml is empty")
	}
	assets := []struct {
		name    string
		content []byte
	}{
		{name: "v2.6", content: sevenDaysToDieLandClaimsV26DLL},
		{name: "v3", content: sevenDaysToDieLandClaimsV3DLL},
	}
	for _, asset := range assets {
		t.Run(asset.name, func(t *testing.T) {
			if !bytes.Contains(asset.content, []byte("GetLandClaims.openapi.yaml")) {
				t.Fatal("embedded DLL is missing the GetLandClaims OpenAPI resource")
			}
			file, errOpen := pe.NewFile(bytes.NewReader(asset.content))
			if errOpen != nil {
				t.Fatalf("parse embedded DLL: %v", errOpen)
			}
			errClose := file.Close()
			if errClose != nil {
				t.Fatalf("close embedded DLL: %v", errClose)
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
	wantDLL []byte,
	wantInstallCount int,
) {
	t.Helper()
	if len(client.StatFileCalls) != wantInstallCount {
		t.Fatalf("stat calls = %d, want %d", len(client.StatFileCalls), wantInstallCount)
	}
	for _, call := range client.StatFileCalls {
		if call.Directory != "/tmp/server-local-1" || call.RelativePath != sevenDaysToDieWebServerDLLPath {
			t.Fatalf("stat call = %+v", call)
		}
	}
	if len(client.CreateFileOrDirectoryCalls) != wantInstallCount {
		t.Fatalf("create calls = %d, want %d", len(client.CreateFileOrDirectoryCalls), wantInstallCount)
	}
	for _, call := range client.CreateFileOrDirectoryCalls {
		if call.Directory != "/tmp/server-local-1" || call.RelativePath != sevenDaysToDieLandClaimsModDirectory || !call.IsDirectory {
			t.Fatalf("create call = %+v", call)
		}
	}
	if len(client.WriteFileCalls) != wantInstallCount*2 {
		t.Fatalf("write calls = %d, want %d", len(client.WriteFileCalls), wantInstallCount*2)
	}
	for installIndex := range wantInstallCount {
		dllCall := client.WriteFileCalls[installIndex*2]
		if dllCall.Directory != "/tmp/server-local-1" ||
			dllCall.RelativePath != sevenDaysToDieLandClaimsModDirectory+"/XylonaLandClaims.dll" ||
			!bytes.Equal(dllCall.Content, wantDLL) {
			t.Fatalf("DLL write call = %+v", dllCall)
		}
		modInfoCall := client.WriteFileCalls[installIndex*2+1]
		if modInfoCall.Directory != "/tmp/server-local-1" ||
			modInfoCall.RelativePath != sevenDaysToDieLandClaimsModDirectory+"/ModInfo.xml" ||
			!bytes.Equal(modInfoCall.Content, sevenDaysToDieLandClaimsModInfo) {
			t.Fatalf("ModInfo write call = %+v", modInfoCall)
		}
	}
}
