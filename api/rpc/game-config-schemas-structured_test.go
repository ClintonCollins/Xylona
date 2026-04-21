package rpc

import (
	"context"
	"encoding/json"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGetGameServerConfigFileReadsStructuredJSON(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	updateGameConfigSchemasForRemoteParity(t, fixture, windroseLikeJSONSchema())
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-json-config")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		ReadFileResult: []byte(`{
  "ServerDescription_Persistent": {
    "ServerName": "Existing Windrose",
    "MaxPlayerCount": 4,
    "UseDirectConnection": false,
    "DirectConnectionServerPort": 7777,
    "UnknownNested": "kept"
  },
  "UnknownRoot": "kept"
}
`),
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GetGameServerConfigFileRequest{
		GameServerId: "server-remote-json-config",
		FilePath:     "ServerDescription.json",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetGameServerConfigFile(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetGameServerConfigFile() error = %v", errGet)
	}

	fields := configFieldsByKey(response.Msg.GetFields())
	if fields["ServerDescription_Persistent.ServerName"].GetValue() != "Existing Windrose" {
		t.Fatalf("ServerName = %q, want %q",
			fields["ServerDescription_Persistent.ServerName"].GetValue(), "Existing Windrose")
	}
	if fields["ServerDescription_Persistent.MaxPlayerCount"].GetValue() != "4" {
		t.Fatalf("MaxPlayerCount = %q, want %q",
			fields["ServerDescription_Persistent.MaxPlayerCount"].GetValue(), "4")
	}
	if fields["ServerDescription_Persistent.UseDirectConnection"].GetValue() != "false" {
		t.Fatalf("UseDirectConnection = %q, want %q",
			fields["ServerDescription_Persistent.UseDirectConnection"].GetValue(), "false")
	}
	if fields["ServerDescription_Persistent.DirectConnectionServerPort"].GetValue() != "25575" {
		t.Fatalf("DirectConnectionServerPort = %q, want managed port %q",
			fields["ServerDescription_Persistent.DirectConnectionServerPort"].GetValue(), "25575")
	}
	if !fields["ServerDescription_Persistent.DirectConnectionServerPort"].GetIsManaged() {
		t.Fatal("DirectConnectionServerPort IsManaged = false, want true")
	}
}

func TestUpdateGameServerConfigFileWritesStructuredJSON(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	updateGameConfigSchemasForRemoteParity(t, fixture, windroseLikeJSONSchema())
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-json-config")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		ReadFileResult: []byte(`{
  "ServerDescription_Persistent": {
    "ServerName": "Old Windrose",
    "MaxPlayerCount": 4,
    "UseDirectConnection": false,
    "DirectConnectionServerPort": 7777
  },
  "UnknownRoot": "preserved"
}
`),
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.UpdateGameServerConfigFileRequest{
		GameServerId: "server-remote-json-config",
		FilePath:     "ServerDescription.json",
		Fields: []*xylona.ConfigFieldData{
			{Key: "ServerDescription_Persistent.ServerName", Value: "New Windrose"},
			{Key: "ServerDescription_Persistent.MaxPlayerCount", Value: "8"},
			{Key: "ServerDescription_Persistent.UseDirectConnection", Value: "true"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errUpdate := fixture.service.UpdateGameServerConfigFile(context.Background(), request)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerConfigFile() error = %v", errUpdate)
	}
	if !response.Msg.GetSuccess() {
		t.Fatal("UpdateGameServerConfigFile().Success = false, want true")
	}
	if len(remoteClient.WriteFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(remoteClient.WriteFileCalls))
	}

	var config map[string]any
	errUnmarshal := json.Unmarshal(remoteClient.WriteFileCalls[0].Content, &config)
	if errUnmarshal != nil {
		t.Fatalf("json.Unmarshal() error = %v", errUnmarshal)
	}

	persistent, ok := config["ServerDescription_Persistent"].(map[string]any)
	if !ok {
		t.Fatalf("ServerDescription_Persistent type = %T, want object", config["ServerDescription_Persistent"])
	}
	if persistent["ServerName"] != "New Windrose" {
		t.Errorf("ServerName = %v, want %q", persistent["ServerName"], "New Windrose")
	}
	if persistent["MaxPlayerCount"] != float64(8) {
		t.Errorf("MaxPlayerCount = %v, want 8", persistent["MaxPlayerCount"])
	}
	useDirectConnection, ok := persistent["UseDirectConnection"].(bool)
	if !ok || !useDirectConnection {
		t.Errorf("UseDirectConnection = %v, want true", persistent["UseDirectConnection"])
	}
	if persistent["DirectConnectionServerPort"] != float64(7777) {
		t.Errorf("DirectConnectionServerPort = %v, want 7777", persistent["DirectConnectionServerPort"])
	}
	if config["UnknownRoot"] != "preserved" {
		t.Errorf("UnknownRoot = %v, want %q", config["UnknownRoot"], "preserved")
	}
}

func TestGenerateGameServerConfigFileWritesStructuredJSONDefaults(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	updateGameConfigSchemasForRemoteParity(t, fixture, windroseLikeJSONSchema())
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-json-config")

	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GenerateGameServerConfigFileRequest{
		GameServerId: "server-remote-json-config",
		FilePath:     "ServerDescription.json",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGenerate := fixture.service.GenerateGameServerConfigFile(context.Background(), request)
	if errGenerate != nil {
		t.Fatalf("GenerateGameServerConfigFile() error = %v", errGenerate)
	}
	if !response.Msg.GetSuccess() {
		t.Fatal("GenerateGameServerConfigFile().Success = false, want true")
	}
	if len(remoteClient.WriteFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(remoteClient.WriteFileCalls))
	}

	var config map[string]any
	errUnmarshal := json.Unmarshal(remoteClient.WriteFileCalls[0].Content, &config)
	if errUnmarshal != nil {
		t.Fatalf("json.Unmarshal() error = %v", errUnmarshal)
	}

	persistent, ok := config["ServerDescription_Persistent"].(map[string]any)
	if !ok {
		t.Fatalf("ServerDescription_Persistent type = %T, want object", config["ServerDescription_Persistent"])
	}
	if persistent["ServerName"] != "My Windrose Server" {
		t.Errorf("ServerName = %v, want %q", persistent["ServerName"], "My Windrose Server")
	}
	if persistent["MaxPlayerCount"] != float64(8) {
		t.Errorf("MaxPlayerCount = %v, want 8", persistent["MaxPlayerCount"])
	}
	useDirectConnection, ok := persistent["UseDirectConnection"].(bool)
	if !ok || useDirectConnection {
		t.Errorf("UseDirectConnection = %v, want false", persistent["UseDirectConnection"])
	}
	if persistent["DirectConnectionServerPort"] != float64(25575) {
		t.Errorf("DirectConnectionServerPort = %v, want managed port 25575", persistent["DirectConnectionServerPort"])
	}
}

func configFieldsByKey(fields []*xylona.ConfigFieldData) map[string]*xylona.ConfigFieldData {
	result := make(map[string]*xylona.ConfigFieldData, len(fields))
	for _, field := range fields {
		result[field.GetKey()] = field
	}

	return result
}

func windroseLikeJSONSchema() string {
	return `[{"managed_fields":{"ServerDescription_Persistent.DirectConnectionServerPort":"game_server.port"},"path":"ServerDescription.json","format":"json","category":"Server","generate_before_start":false,"schema":{"type":"object","properties":{"ServerDescription_Persistent.ServerName":{"type":"string","default":"My Windrose Server"},"ServerDescription_Persistent.MaxPlayerCount":{"type":"integer","default":8},"ServerDescription_Persistent.UseDirectConnection":{"type":"boolean","default":false},"ServerDescription_Persistent.DirectConnectionServerPort":{"type":"integer","default":7777}}}}]`
}
