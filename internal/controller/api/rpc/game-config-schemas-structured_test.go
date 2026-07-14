package rpc

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const windroseConfigPath = "R5/ServerDescription.json"

const sevenDaysConfigPath = "serverconfig.xml"

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
		FilePath:     windroseConfigPath,
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

func TestGetGameServerConfigFileReadsAttributeKeyedXML(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	updateGameConfigSchemasForRemoteParity(t, fixture, sevenDaysLikeXMLSchema())
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-xml-config")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		ReadFileResult: []byte(`<?xml version="1.0"?>
<ServerSettings>
  <property name="ServerName" value="Existing Seven Days"/>
  <property name="ServerDescription" value="Existing description"/>
  <property name="UnknownSetting" value="kept"/>
</ServerSettings>
`),
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GetGameServerConfigFileRequest{
		GameServerId: "server-remote-xml-config",
		FilePath:     sevenDaysConfigPath,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetGameServerConfigFile(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetGameServerConfigFile() error = %v", errGet)
	}

	fields := configFieldsByKey(response.Msg.GetFields())
	if fields["ServerName"].GetValue() != "Existing Seven Days" {
		t.Fatalf("ServerName = %q, want %q", fields["ServerName"].GetValue(), "Existing Seven Days")
	}
	if fields["ServerDescription"].GetValue() != "Existing description" {
		t.Fatalf("ServerDescription = %q, want %q", fields["ServerDescription"].GetValue(), "Existing description")
	}

	advancedFields := response.Msg.GetAdvancedFields()
	if len(advancedFields) != 1 {
		t.Fatalf("AdvancedFields length = %d, want 1", len(advancedFields))
	}
	if advancedFields[0].GetKey() != "UnknownSetting" {
		t.Fatalf("AdvancedFields[0].Key = %q, want %q", advancedFields[0].GetKey(), "UnknownSetting")
	}
	if advancedFields[0].GetValue() != "kept" {
		t.Fatalf("AdvancedFields[0].Value = %q, want %q", advancedFields[0].GetValue(), "kept")
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
		FilePath:     windroseConfigPath,
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
	if remoteClient.WriteFileCalls[0].RelativePath != windroseConfigPath {
		t.Fatalf("WriteFile relative path = %q, want %q", remoteClient.WriteFileCalls[0].RelativePath, windroseConfigPath)
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

func TestUpdateGameServerConfigFileWritesAttributeKeyedXML(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	updateGameConfigSchemasForRemoteParity(t, fixture, sevenDaysLikeXMLSchema())
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-xml-config")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		ReadFileResult: []byte(`<?xml version="1.0"?>
<ServerSettings>
  <property name="ServerName" value="Old Seven Days"/>
  <property name="ServerDescription" value="Old description"/>
  <property name="UnknownSetting" value="kept"/>
</ServerSettings>
`),
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.UpdateGameServerConfigFileRequest{
		GameServerId: "server-remote-xml-config",
		FilePath:     sevenDaysConfigPath,
		Fields: []*xylona.ConfigFieldData{
			{Key: "ServerName", Value: "New Seven Days"},
			{Key: "ServerDescription", Value: "New description"},
		},
		AdvancedFields: []*xylona.AdvancedField{
			{Key: "UnknownSetting", Value: "kept"},
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

	properties := xmlPropertiesByName(t, remoteClient.WriteFileCalls[0].Content)
	if properties["ServerName"] != "New Seven Days" {
		t.Errorf("ServerName = %q, want %q", properties["ServerName"], "New Seven Days")
	}
	if properties["ServerDescription"] != "New description" {
		t.Errorf("ServerDescription = %q, want %q", properties["ServerDescription"], "New description")
	}
	if properties["UnknownSetting"] != "kept" {
		t.Errorf("UnknownSetting = %q, want %q", properties["UnknownSetting"], "kept")
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
		FilePath:     windroseConfigPath,
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
	if remoteClient.WriteFileCalls[0].RelativePath != windroseConfigPath {
		t.Fatalf("WriteFile relative path = %q, want %q", remoteClient.WriteFileCalls[0].RelativePath, windroseConfigPath)
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

func xmlPropertiesByName(t *testing.T, data []byte) map[string]string {
	t.Helper()

	var document struct {
		Properties []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"property"`
	}
	errUnmarshal := xml.Unmarshal(data, &document)
	if errUnmarshal != nil {
		t.Fatalf("xml.Unmarshal() error = %v", errUnmarshal)
	}

	properties := make(map[string]string, len(document.Properties))
	for _, property := range document.Properties {
		properties[property.Name] = property.Value
	}

	return properties
}

func windroseLikeJSONSchema() string {
	return `[{"managed_fields":{"ServerDescription_Persistent.DirectConnectionServerPort":"game_server.port"},"path":"R5/ServerDescription.json","format":"json","category":"Server","generate_before_start":false,"schema":{"type":"object","properties":{"ServerDescription_Persistent.ServerName":{"type":"string","default":"My Windrose Server"},"ServerDescription_Persistent.MaxPlayerCount":{"type":"integer","default":8},"ServerDescription_Persistent.UseDirectConnection":{"type":"boolean","default":false},"ServerDescription_Persistent.DirectConnectionServerPort":{"type":"integer","default":7777}}}}]`
}

func sevenDaysLikeXMLSchema() string {
	return `[{"path":"serverconfig.xml","format":"xml","category":"Server","xml_key_mode":{"mode":"attributes","element":"property","key_attr":"name","value_attr":"value"},"schema":{"type":"object","properties":{"ServerName":{"type":"string","default":"My Seven Days Server"},"ServerDescription":{"type":"string","default":""}}}}]`
}
