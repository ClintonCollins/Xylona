package sevendaystodiemod

import (
	"bytes"
	"debug/pe"
	"os"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestInstall(t *testing.T) {
	tests := []struct {
		name      string
		statError error
		wantDLL   []byte
	}{
		{name: "legacy WebServer", wantDLL: v26DLL},
		{name: "current WebServer", statError: os.ErrNotExist, wantDLL: v3DLL},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &nodeclient.FakeNodeClient{StatFileErr: test.statError}
			gameServer := &models.GameServer{Directory: "/servers/7dtd"}

			errInstall := Install(t.Context(), client, gameServer, node.ProtectionPolicy{})
			if errInstall != nil {
				t.Fatalf("Install() error = %v", errInstall)
			}
			if len(client.WriteFileCalls) != 2 {
				t.Fatalf("WriteFile call count = %d, want 2", len(client.WriteFileCalls))
			}
			if !bytes.Equal(client.WriteFileCalls[0].Content, test.wantDLL) {
				t.Fatal("Install() wrote the wrong DLL")
			}
		})
	}
}

func TestEmbeddedAssets(t *testing.T) {
	if len(modInfo) == 0 {
		t.Fatal("embedded ModInfo.xml is empty")
	}
	assets := []struct {
		name    string
		content []byte
	}{
		{name: "v2.6", content: v26DLL},
		{name: "v3", content: v3DLL},
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
