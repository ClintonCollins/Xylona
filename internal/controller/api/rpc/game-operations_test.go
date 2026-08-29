package rpc

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestListGameServerOperations(t *testing.T) {
	t.Run("omits operations the viewer cannot authorize", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		grantGameServerRole(t, fixture, "user-other", "viewer")
		request := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

		response, errList := fixture.service.ListGameServerOperations(t.Context(), request)
		if errList != nil {
			t.Fatalf("ListGameServerOperations() error = %v", errList)
		}
		if len(response.Msg.GetOperations()) != 0 {
			t.Fatalf("operations = %+v, want none", response.Msg.GetOperations())
		}
		if client.RuntimeCapabilitiesCalls != 0 || len(client.QuerySevenDaysToDieWebAPIStatusCalls) != 0 {
			t.Fatal("unauthorized operation listing queried node capabilities")
		}
	})

	tests := []struct {
		name                  string
		configure             func(*node.RuntimeCapabilities, *node.SevenDaysToDieWebAPIStatus)
		wantAvailable         bool
		wantReason            xylona.GameOperationAvailabilityReason
		wantPlayerIdentities  bool
		wantNativeStatusCalls int
	}{
		{
			name: "available operation intersects controller node and native support",
			configure: func(capabilities *node.RuntimeCapabilities, status *node.SevenDaysToDieWebAPIStatus) {
				capabilities.GameOperations = []node.GameOperationSupport{
					{GameID: "7_days_to_die", OperationIDs: []string{"player_access.add_administrator"}},
				}
				status.ConnectionState = node.SevenDaysToDieWebAPIConnectionStateAvailable
				status.Capabilities.GamePermissions = true
				status.Capabilities.PlayerData = true
			},
			wantAvailable:         true,
			wantPlayerIdentities:  true,
			wantNativeStatusCalls: 1,
		},
		{
			name: "older node disables the operation before native discovery",
			configure: func(capabilities *node.RuntimeCapabilities, _ *node.SevenDaysToDieWebAPIStatus) {
				capabilities.ProtocolVersion = node.RuntimeProtocolVersion - 1
				capabilities.GameOperations = []node.GameOperationSupport{
					{GameID: "7_days_to_die", OperationIDs: []string{"player_access.add_administrator"}},
				}
			},
			wantReason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNSUPPORTED,
		},
		{
			name: "node without operation support disables the operation",
			configure: func(capabilities *node.RuntimeCapabilities, _ *node.SevenDaysToDieWebAPIStatus) {
				capabilities.GameOperations = nil
			},
			wantReason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNSUPPORTED,
		},
		{
			name: "missing native permission capability gives a concrete reason",
			configure: func(capabilities *node.RuntimeCapabilities, status *node.SevenDaysToDieWebAPIStatus) {
				capabilities.GameOperations = []node.GameOperationSupport{
					{GameID: "7_days_to_die", OperationIDs: []string{"player_access.add_administrator"}},
				}
				status.ConnectionState = node.SevenDaysToDieWebAPIConnectionStateAvailable
				status.Capabilities.GamePermissions = false
			},
			wantReason:            xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_GAME_PERMISSION_UNSUPPORTED,
			wantNativeStatusCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, _, client := newPrivateReadGateFixture(t)
			client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{}
			client.QuerySevenDaysToDiePlayersResult = &node.SevenDaysToDiePlayers{
				ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
				State:           node.SevenDaysToDieWebAPIValueStateAvailable,
				Players: []node.SevenDaysToDiePlayer{
					{
						Name:            "Player One",
						ActionID:        "Steam_PLAYER_1",
						PlatformID:      "Steam_PLAYER_1",
						CrossPlatformID: "EOS_PLAYER_1",
					},
				},
			}
			test.configure(&client.RuntimeCapabilitiesResult, client.QuerySevenDaysToDieWebAPIStatusResult)

			request := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			response, errList := fixture.service.ListGameServerOperations(t.Context(), request)
			if errList != nil {
				t.Fatalf("ListGameServerOperations() error = %v", errList)
			}
			operations := response.Msg.GetOperations()
			if len(operations) != 1 {
				t.Fatalf("operation count = %d, want 1", len(operations))
			}
			operation := operations[0]
			if operation.GetId() != "player_access.add_administrator" || operation.GetAvailable() != test.wantAvailable ||
				operation.GetAvailabilityReason() != test.wantReason {
				t.Fatalf("operation availability = %+v", operation)
			}
			if !test.wantAvailable && strings.TrimSpace(operation.GetAvailabilityReasonText()) == "" {
				t.Fatal("disabled operation omitted its human-readable reason")
			}
			if len(client.QuerySevenDaysToDieWebAPIStatusCalls) != test.wantNativeStatusCalls {
				t.Fatalf("native status calls = %d, want %d", len(client.QuerySevenDaysToDieWebAPIStatusCalls), test.wantNativeStatusCalls)
			}
			if test.wantPlayerIdentities {
				options := operation.GetFields()[0].GetOptions()
				if len(options) != 1 || options[0].GetValue() != "Steam_PLAYER_1" ||
					!strings.Contains(options[0].GetDescription(), "EOS_PLAYER_1") {
					t.Fatalf("Player identity options = %+v", options)
				}
			}

			wire, errMarshal := protojson.Marshal(response.Msg)
			if errMarshal != nil {
				t.Fatalf("marshal response: %v", errMarshal)
			}
			wireText := strings.ToLower(string(wire))
			for _, forbidden := range []string{"/api/", "userpermissions", "command template", "token secret"} {
				if strings.Contains(wireText, forbidden) {
					t.Fatalf("response exposed native transport detail %q: %s", forbidden, wire)
				}
			}
		})
	}
}

func grantGameServerRole(t *testing.T, fixture *rbacRPCFixture, userID string, roleID string) {
	t.Helper()
	request := connect.NewRequest(&xylona.GrantGameServerAccessRequest{
		GameServerId: "server-local-1",
		UserId:       userID,
		RoleId:       roleID,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	_, errGrant := fixture.service.GrantGameServerAccess(t.Context(), request)
	if errGrant != nil {
		t.Fatalf("grant %s role: %v", roleID, errGrant)
	}
}
