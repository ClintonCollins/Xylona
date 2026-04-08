package rpc

import (
	"path/filepath"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGenerateNodePairingObjectRequiresMTLS(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.GenerateNodePairingObjectRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	_, errGenerate := fixture.service.GenerateNodePairingObject(
		t.Context(),
		req,
	)
	if errGenerate == nil {
		t.Fatalf("GenerateNodePairingObject() error = nil, want error")
	}
	if connect.CodeOf(errGenerate) != connect.CodeInternal {
		t.Fatalf("GenerateNodePairingObject() code = %v, want %v", connect.CodeOf(errGenerate), connect.CodeInternal)
	}
}

func TestGenerateNodePairingObjectRejectsInvalidTargetURL(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	temporaryDirectory := t.TempDir()
	certPath := filepath.Join(temporaryDirectory, "node.crt")
	keyPath := filepath.Join(temporaryDirectory, "node.key")

	federationMTLS, _, errCreateMTLS := federation.NewMTLS("local-node-id", 8443, certPath, keyPath)
	if errCreateMTLS != nil {
		t.Fatalf("NewFederationMTLS() error = %v", errCreateMTLS)
	}

	fixture.service.federationMTLS = federationMTLS

	req := connect.NewRequest(&xylona.GenerateNodePairingObjectRequest{
		TargetUrl: "not-a-valid-url",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	_, errGenerate := fixture.service.GenerateNodePairingObject(
		t.Context(),
		req,
	)
	if errGenerate == nil {
		t.Fatalf("GenerateNodePairingObject() error = nil, want error")
	}
	if connect.CodeOf(errGenerate) != connect.CodeInvalidArgument {
		t.Fatalf("GenerateNodePairingObject() code = %v, want %v", connect.CodeOf(errGenerate), connect.CodeInvalidArgument)
	}
}
