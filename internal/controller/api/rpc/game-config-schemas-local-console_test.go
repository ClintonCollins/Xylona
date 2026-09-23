package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGetGameServerConfigFileShowsLocalConsoleLikeStart(t *testing.T) {
	type wantField struct {
		value   string
		managed bool
	}
	tests := []struct {
		name       string
		mapEnabled bool
		mapSaved   bool
		want       map[string]wantField
	}{
		{
			name:       "console on manages RCON with the query port plus one",
			mapEnabled: true,
			mapSaved:   true,
			want: map[string]wantField{
				"enable-rcon":   {value: "true", managed: true},
				"rcon.port":     {value: "25577", managed: true},
				"rcon.password": {value: "", managed: true},
			},
		},
		{
			name: "console never set up leaves RCON to the file",
			want: map[string]wantField{
				"enable-rcon":   {value: "false", managed: false},
				"rcon.port":     {value: "25999", managed: false},
				"rcon.password": {value: "file-secret", managed: false},
			},
		},
		{
			name:     "console switched off keeps RCON managed off",
			mapSaved: true,
			want: map[string]wantField{
				"enable-rcon":   {value: "false", managed: true},
				"rcon.port":     {value: "25999", managed: false},
				"rcon.password": {value: "file-secret", managed: false},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			updateGameConfigSchemasForRemoteParity(t, fixture, minecraftLikeLocalConsoleSchema())
			insertRemoteNodeForParityTests(t, fixture, "node-remote")
			insertRemoteServerForParityTests(t, fixture, "server-local-console")
			if testCase.mapSaved {
				errMap := fixture.conn.UpdateGameServerMinecraftMapConfig(
					"server-local-console",
					testCase.mapEnabled,
					"world",
					true,
					"",
				)
				if errMap != nil {
					t.Fatalf("UpdateGameServerMinecraftMapConfig() error = %v", errMap)
				}
			}

			remoteClient := &nodeclient.FakeNodeClient{
				NodeID:         "node-remote",
				ReadFileResult: []byte("enable-rcon=false\nrcon.port=25999\nrcon.password=file-secret\n"),
			}
			fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

			request := connect.NewRequest(&xylona.GetGameServerConfigFileRequest{
				GameServerId: "server-local-console",
				FilePath:     "server.properties",
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

			response, errGet := fixture.service.GetGameServerConfigFile(context.Background(), request)
			if errGet != nil {
				t.Fatalf("GetGameServerConfigFile() error = %v", errGet)
			}

			fields := configFieldsByKey(response.Msg.GetFields())
			for key, want := range testCase.want {
				field := fields[key]
				if field.GetValue() != want.value || field.GetIsManaged() != want.managed {
					t.Errorf("%s = (%q, managed %t), want (%q, managed %t)",
						key, field.GetValue(), field.GetIsManaged(), want.value, want.managed)
				}
			}
		})
	}
}

func minecraftLikeLocalConsoleSchema() string {
	return `[{"path":"server.properties","format":"properties","category":"Server","managed_fields":{"enable-rcon":"xylona.local_console_enabled","rcon.port":"xylona.local_console_port","rcon.password":"xylona.local_console_password"},"schema":{"type":"object","properties":{"enable-rcon":{"type":"boolean","default":false},"rcon.port":{"type":"integer","default":25575},"rcon.password":{"type":"string","default":""}}}}]`
}
