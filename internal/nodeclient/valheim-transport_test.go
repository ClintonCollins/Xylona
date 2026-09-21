package nodeclient_test

import (
	"slices"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

func TestValheimRequiredRuntimeCapability(t *testing.T) {
	for _, supported := range []bool{false, true} {
		rec := &callRecorder{runtimeCapsResp: &nodeprotov1.GetRuntimeCapabilitiesResponse{ValheimNativeRuntimeV1: supported}}
		url, fingerprint := newPinnedTestServer(t, rec)
		client, errClient := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
		if errClient != nil {
			t.Fatal(errClient)
		}
		errStart := client.StartProcess(t.Context(), node.ProcessConfig{ID: "server", GameID: "valheim", RuntimeMode: "valheim-native"}, xylona.Status_ONLINE)
		if (errStart == nil) != supported {
			t.Fatalf("supported %v: %v", supported, errStart)
		}
		rec.mu.Lock()
		if supported && (rec.startProcessReq.GetGameId() != "valheim" || rec.startProcessReq.GetRuntimeMode() != "valheim-native") {
			t.Error("required runtime identity lost")
		}
		if !supported && rec.startProcessReq != nil {
			t.Error("start dispatched to unsupported node")
		}
		rec.mu.Unlock()
	}
}

func TestValheimAccessListTransport(t *testing.T) {
	want := &nodeprotov1.ValheimAccessList{ListKind: "administrators", Identities: []string{"Steam_123"}, Revision: "revision", Diagnostics: []string{"Unrecognized line retained"}, Missing: true}
	rec := &callRecorder{gameOperationResp: &nodeprotov1.ExecuteGameOperationResponse{ValheimAccessList: want}}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, errClient := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
	if errClient != nil {
		t.Fatal(errClient)
	}
	result, errExecute := client.ExecuteGameOperation(t.Context(), node.GameOperationRequest{GameID: "valheim", GameServerID: "server", OperationID: "valheim.access.administrators.list"})
	if errExecute != nil {
		t.Fatal(errExecute)
	}
	got := result.ValheimAccessList
	if got == nil || got.ListKind != want.GetListKind() || got.Revision != want.GetRevision() || got.Missing != want.GetMissing() || !slices.Equal(got.Identities, want.GetIdentities()) || !slices.Equal(got.Diagnostics, want.GetDiagnostics()) {
		t.Fatalf("access list lost fields: %#v", got)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.gameOperationReq.GetGameId() != "valheim" || rec.gameOperationReq.GetGameServerId() != "server" {
		t.Fatal("operation identity lost")
	}
}
