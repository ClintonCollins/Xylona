package rpc

import (
	"path/filepath"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGenerateNodePairingObjectRequiresMTLS(t *testing.T) {
	xylonaService := XylonaService{}

	_, errGenerate := xylonaService.GenerateNodePairingObject(
		t.Context(),
		connect.NewRequest(&xylona.GenerateNodePairingObjectRequest{}),
	)
	if errGenerate == nil {
		t.Fatalf("GenerateNodePairingObject() error = nil, want error")
	}
	if connect.CodeOf(errGenerate) != connect.CodeInternal {
		t.Fatalf("GenerateNodePairingObject() code = %v, want %v", connect.CodeOf(errGenerate), connect.CodeInternal)
	}
}

func TestGenerateNodePairingObjectRejectsInvalidTargetURL(t *testing.T) {
	temporaryDirectory := t.TempDir()
	certPath := filepath.Join(temporaryDirectory, "node.crt")
	keyPath := filepath.Join(temporaryDirectory, "node.key")

	federationMTLS, _, errCreateMTLS := helpers.NewFederationMTLS("local-node-id", 8443, certPath, keyPath)
	if errCreateMTLS != nil {
		t.Fatalf("NewFederationMTLS() error = %v", errCreateMTLS)
	}

	xylonaService := XylonaService{
		federationMTLS: federationMTLS,
	}

	_, errGenerate := xylonaService.GenerateNodePairingObject(
		t.Context(),
		connect.NewRequest(&xylona.GenerateNodePairingObjectRequest{
			TargetUrl: "not-a-valid-url",
		}),
	)
	if errGenerate == nil {
		t.Fatalf("GenerateNodePairingObject() error = nil, want error")
	}
	if connect.CodeOf(errGenerate) != connect.CodeInvalidArgument {
		t.Fatalf("GenerateNodePairingObject() code = %v, want %v", connect.CodeOf(errGenerate), connect.CodeInvalidArgument)
	}
}
