package rpc

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

func TestListAggregatedGameServers_UsesResolvedLocalVersion(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}
	fixture.service.supervisorInst = supervisorInst

	serverDir := t.TempDir()
	createTestMinecraftJar(t, serverDir, "server.jar", "1.21.2")

	_, errUpdate := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:               omit.From("server-local-1"),
		Directory:        omit.From(serverDir),
		ServerExecutable: omitnull.From("server.jar"),
		Version:          omit.From("1.21.11"),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	request := connect.NewRequest(&xylona.ListAggregatedGameServersRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errList := fixture.service.ListAggregatedGameServers(context.Background(), request)
	if errList != nil {
		t.Fatalf("ListAggregatedGameServers() error = %v", errList)
	}

	for _, server := range response.Msg.GetServers() {
		if !server.GetIsLocal() || server.GetLocalServer().GetId() != "server-local-1" {
			continue
		}
		if got := server.GetLocalServer().GetVersion(); got != "1.21.2" {
			t.Fatalf("LocalServer.Version = %q, want %q", got, "1.21.2")
		}
		return
	}

	t.Fatal("local server not found in aggregated server list")
}

func createTestMinecraftJar(t *testing.T, dir string, fileName string, version string) {
	t.Helper()

	jarPath := filepath.Join(dir, fileName)
	file, errCreate := os.Create(jarPath)
	if errCreate != nil {
		t.Fatalf("create jar: %v", errCreate)
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			t.Errorf("close jar file: %v", errClose)
		}
	}()

	zw := zip.NewWriter(file)
	defer func() {
		if errClose := zw.Close(); errClose != nil {
			t.Errorf("close zip writer: %v", errClose)
		}
	}()

	w, errEntry := zw.Create("version.json")
	if errEntry != nil {
		t.Fatalf("create version.json: %v", errEntry)
	}

	versionJSON := []byte(`{"id":"` + version + `","name":"` + version + `"}`)
	if _, errWrite := w.Write(versionJSON); errWrite != nil {
		t.Fatalf("write version.json: %v", errWrite)
	}
}
