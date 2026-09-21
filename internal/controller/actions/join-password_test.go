package actions

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/valheim"
)

func TestJoinPasswordLaunchDeliveryAndFailureRedaction(t *testing.T) {
	for _, test := range []struct {
		name                                                             string
		remote, oldNode, fail, unprotected, defaultPassword, legacyEmpty bool
	}{
		{name: "embedded"},
		{name: "remote", remote: true},
		{name: "unprotected embedded", unprotected: true},
		{name: "unprotected remote", remote: true, unprotected: true},
		{name: "default password", unprotected: true, defaultPassword: true},
		{name: "legacy empty password", unprotected: true, legacyEmpty: true},
		{name: "old remote node", remote: true, oldNode: true},
		{name: "remote error", remote: true, fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAutoRestartTestFixture(t, 0)
			_, errUpdate := fixture.conn.SQLDb.ExecContext(t.Context(), "update game_server set game_id = 'valheim' where id = ?", fixture.gameServer.ID)
			if errUpdate != nil {
				t.Fatal(errUpdate)
			}
			server, errServer := fixture.conn.GetGameServerByID(fixture.gameServer.ID)
			if errServer != nil {
				t.Fatal(errServer)
			}
			password := "private token \" with \\ spaces"
			if test.unprotected {
				password = ""
			}
			errSet := fixture.conn.SetValheimJoinPassword(server, password)
			if errSet != nil {
				t.Fatal(errSet)
			}
			if test.defaultPassword {
				server.StartArgsPatches = ""
			}
			if test.legacyEmpty {
				server.StartArgsPatches = `[{"id":"01JQSD00000000000000000003","op":"edit","tokens":["-password",""]}]`
			}
			client := &nodeclient.FakeNodeClient{NodeID: server.NodeID, SnapshotResult: &node.NodeSnapshot{OS: "linux"}, RuntimeCapabilitiesResult: node.RuntimeCapabilities{ValheimNativeRuntimeV1: !test.oldNode}}
			if test.fail {
				client.StartProcessErr = errors.New("launch failed: " + strconv.Quote(password))
			}
			fixture.inst.embeddedNodeClient = client
			if test.remote {
				fixture.inst.nodeRegistry = noderegistry.New("controller", &nodeclient.FakeNodeClient{NodeID: "controller"})
				fixture.inst.nodeRegistry.Register(client)
			}
			_, errStart := fixture.inst.StartGameServer(server)
			if test.oldNode {
				if errStart == nil || len(client.StartProcessCalls) != 0 {
					t.Fatal("incompatible node received managed launch")
				}
				return
			}
			if (errStart != nil) != test.fail {
				t.Fatalf("unexpected start result: %v", errStart)
			}
			if errStart != nil && (strings.Contains(errStart.Error(), password) || strings.Contains(errStart.Error(), strconv.Quote(password))) {
				t.Fatal("start error exposed password")
			}
			if len(client.StartProcessCalls) != 1 {
				t.Fatal("expected exactly one launch")
			}
			config := client.StartProcessCalls[0].Config
			if config.GameID != "valheim" || config.RuntimeMode != valheim.NativeRuntime || config.StopTimeout != 120*time.Second {
				t.Fatal("Valheim launch omitted native runtime identity or graceful stop timeout")
			}
			index := slices.Index(config.Args, "-password")
			if password == "" && index >= 0 {
				t.Fatal("unprotected launch included a password argument")
			}
			if password != "" && (index < 0 || index+1 >= len(config.Args) || config.Args[index+1] != password) {
				t.Fatal("launch did not preserve one confidential argv token")
			}
			if password != "" && !slices.Contains(config.RedactValues, password) {
				t.Fatal("launch omitted password redaction")
			}
		})
	}
}
