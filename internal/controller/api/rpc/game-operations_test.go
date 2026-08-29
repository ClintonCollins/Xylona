package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
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

func TestExecuteGameServerOperation(t *testing.T) {
	t.Run("executes through controller and node to authoritative read-back", func(t *testing.T) {
		fixture, client, postRequests := newGameOperationVerticalFixture(t)
		request := validPublicAddAdministratorRequest()
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), request)
		if errExecute != nil {
			t.Fatalf("ExecuteGameServerOperation() error = %v", errExecute)
		}
		result := response.Msg.GetResult()
		if result.GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED ||
			!strings.Contains(result.GetMessage(), "confirmed") || postRequests.Load() != 1 {
			t.Fatalf("result = %+v, native POST requests = %d", result, postRequests.Load())
		}
		if len(client.ExecuteGameOperationCalls) != 1 {
			t.Fatalf("node execution calls = %+v", client.ExecuteGameOperationCalls)
		}
		dispatched := client.ExecuteGameOperationCalls[0]
		if dispatched.OperationID != "player_access.add_administrator" || len(dispatched.Values) != 2 ||
			dispatched.WorkingDirectory == "" || dispatched.TokenName == "" || dispatched.TokenSecret == "" {
			t.Fatalf("node request = %+v", dispatched)
		}
		wire, errMarshal := protojson.Marshal(response.Msg)
		if errMarshal != nil {
			t.Fatalf("marshal response: %v", errMarshal)
		}
		wireText := strings.ToLower(string(wire))
		for _, forbidden := range []string{"/api/", dispatched.TokenName, dispatched.TokenSecret} {
			if forbidden != "" && strings.Contains(wireText, strings.ToLower(forbidden)) {
				t.Fatalf("public result exposed node transport detail %q: %s", forbidden, wire)
			}
		}
	})

	t.Run("node validation rejects an unknown structured field", func(t *testing.T) {
		fixture, client, postRequests := newGameOperationVerticalFixture(t)
		request := validPublicAddAdministratorRequest()
		request.Msg.Values = append(request.Msg.Values, &xylona.GameOperationValue{
			FieldId: "endpoint",
			Value:   &xylona.GameOperationValue_StringValue{StringValue: "/unsafe"},
		})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), request)
		if errExecute != nil {
			t.Fatalf("ExecuteGameServerOperation() error = %v", errExecute)
		}
		result := response.Msg.GetResult()
		if result.GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED ||
			!strings.Contains(result.GetMessage(), "Unknown operation field") || postRequests.Load() != 0 ||
			len(client.ExecuteGameOperationCalls) != 1 {
			t.Fatalf("result = %+v, native POST requests = %d, node calls = %d", result, postRequests.Load(), len(client.ExecuteGameOperationCalls))
		}
	})

	t.Run("capability loss fails before node execution", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
			ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
			Capabilities:    node.SevenDaysToDieWebAPICapabilities{GamePermissions: true},
		}
		request := validPublicAddAdministratorRequest()
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), request)
		if errExecute != nil {
			t.Fatalf("ExecuteGameServerOperation() error = %v", errExecute)
		}
		result := response.Msg.GetResult()
		if result.GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED ||
			!strings.Contains(result.GetMessage(), "Update the node") || len(client.ExecuteGameOperationCalls) != 0 {
			t.Fatalf("result = %+v, node calls = %+v", result, client.ExecuteGameOperationCalls)
		}
	})

	t.Run("authorization is rechecked after listing", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		client.RuntimeCapabilitiesResult.GameOperations = []node.GameOperationSupport{
			{GameID: sevenDaysToDieGameID, OperationIDs: []string{"player_access.add_administrator"}},
		}
		client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
			ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
			Capabilities:    node.SevenDaysToDieWebAPICapabilities{GamePermissions: true},
		}
		grantRequest := connect.NewRequest(&xylona.GrantGameServerAccessRequest{
			GameServerId: "server-local-1",
			UserId:       "user-other",
			RoleId:       "operator",
		})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, grantRequest, "user-owner")
		grantResponse, errGrant := fixture.service.GrantGameServerAccess(t.Context(), grantRequest)
		if errGrant != nil {
			t.Fatalf("grant operator access: %v", errGrant)
		}

		listRequest := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listRequest, "user-other")
		listResponse, errList := fixture.service.ListGameServerOperations(t.Context(), listRequest)
		if errList != nil || len(listResponse.Msg.GetOperations()) != 1 {
			t.Fatalf("listed operations = %+v, error = %v", listResponse, errList)
		}

		revokeRequest := connect.NewRequest(&xylona.RevokeGameServerAccessRequest{
			GrantId:      grantResponse.Msg.GetGrant().GetId(),
			GameServerId: "server-local-1",
		})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, revokeRequest, "user-owner")
		_, errRevoke := fixture.service.RevokeGameServerAccess(t.Context(), revokeRequest)
		if errRevoke != nil {
			t.Fatalf("revoke operator access: %v", errRevoke)
		}

		executeRequest := validPublicAddAdministratorRequest()
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, executeRequest, "user-other")
		_, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), executeRequest)
		if connect.CodeOf(errExecute) != connect.CodePermissionDenied || len(client.ExecuteGameOperationCalls) != 0 {
			t.Fatalf("execution error = %v, node calls = %+v", errExecute, client.ExecuteGameOperationCalls)
		}
	})

	t.Run("same-target execution is rejected while a different Player can proceed", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		client.RuntimeCapabilitiesResult.GameOperations = []node.GameOperationSupport{
			{GameID: sevenDaysToDieGameID, OperationIDs: []string{"player_access.add_administrator"}},
		}
		client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
			ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
			Capabilities:    node.SevenDaysToDieWebAPICapabilities{GamePermissions: true},
		}
		started := make(chan struct{})
		release := make(chan struct{})
		defer func() {
			select {
			case <-release:
			default:
				close(release)
			}
		}()
		var executions atomic.Int64
		client.ExecuteGameOperationFunc = func(_ context.Context, _ node.GameOperationRequest) (node.GameOperationResult, error) {
			if executions.Add(1) == 1 {
				close(started)
				<-release
			}
			return node.GameOperationResult{
				Classification: node.GameOperationResultConfirmed,
				Message:        "confirmed",
			}, nil
		}

		firstDone := make(chan error, 1)
		firstRequest := validPublicAddAdministratorRequest()
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, firstRequest, "user-owner")
		go func() {
			_, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), firstRequest)
			firstDone <- errExecute
		}()
		<-started

		differentPlayerRequest := validPublicAddAdministratorRequest()
		differentPlayerRequest.Msg.Values[0].Value = &xylona.GameOperationValue_StringValue{StringValue: "EOS_PLAYER_2"}
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, differentPlayerRequest, "user-owner")
		differentResponse, errDifferent := fixture.service.ExecuteGameServerOperation(t.Context(), differentPlayerRequest)
		if errDifferent != nil ||
			differentResponse.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED ||
			executions.Load() != 2 {
			t.Fatalf("different-target result = %+v, error = %v, node executions = %d", differentResponse, errDifferent, executions.Load())
		}

		duplicateRequest := validPublicAddAdministratorRequest()
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, duplicateRequest, "user-owner")
		response, errDuplicate := fixture.service.ExecuteGameServerOperation(t.Context(), duplicateRequest)
		if errDuplicate != nil {
			t.Fatalf("duplicate ExecuteGameServerOperation() error = %v", errDuplicate)
		}
		if response.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED ||
			!strings.Contains(response.Msg.GetResult().GetMessage(), "already in progress") || executions.Load() != 2 {
			t.Fatalf("duplicate result = %+v, node executions = %d", response.Msg.GetResult(), executions.Load())
		}

		close(release)
		if errFirst := <-firstDone; errFirst != nil {
			t.Fatalf("first ExecuteGameServerOperation() error = %v", errFirst)
		}

		afterCompletionRequest := validPublicAddAdministratorRequest()
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, afterCompletionRequest, "user-owner")
		afterCompletionResponse, errAfterCompletion := fixture.service.ExecuteGameServerOperation(t.Context(), afterCompletionRequest)
		if errAfterCompletion != nil ||
			afterCompletionResponse.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED ||
			executions.Load() != 3 {
			t.Fatalf("post-completion result = %+v, error = %v, node executions = %d", afterCompletionResponse, errAfterCompletion, executions.Load())
		}
	})
}

func validPublicAddAdministratorRequest() *connect.Request[xylona.ExecuteGameServerOperationRequest] {
	return connect.NewRequest(&xylona.ExecuteGameServerOperationRequest{
		GameServerId: "server-local-1",
		OperationId:  "player_access.add_administrator",
		Values: []*xylona.GameOperationValue{
			{FieldId: "player", Value: &xylona.GameOperationValue_StringValue{StringValue: "EOS_PLAYER_1"}},
			{FieldId: "permission_level", Value: &xylona.GameOperationValue_IntegerValue{IntegerValue: 0}},
		},
	})
}

func newGameOperationVerticalFixture(
	t *testing.T,
) (*rbacRPCFixture, *nodeclient.FakeNodeClient, *atomic.Int64) {
	t.Helper()
	postRequests := new(atomic.Int64)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/openapi/openapi.yaml":
			_, errWrite := response.Write([]byte(`openapi: 3.1.0
info:
  version: '1.0.0'
paths:
  /api/userpermissions:
    get: {}
  /api/userpermissions/user/{id}:
    post: {}
`))
			if errWrite != nil {
				t.Errorf("write OpenAPI response: %v", errWrite)
			}
		case "/api/userpermissions/user/EOS_PLAYER_1":
			if request.Method != http.MethodPost {
				http.Error(response, "method", http.StatusMethodNotAllowed)
				return
			}
			postRequests.Add(1)
			if request.Header.Get("X-SDTD-API-TOKENNAME") == "" || request.Header.Get("X-SDTD-API-SECRET") == "" {
				http.Error(response, "credentials", http.StatusUnauthorized)
				return
			}
			var body struct {
				PermissionLevel int64 `json:"permissionLevel"`
			}
			errDecode := json.NewDecoder(request.Body).Decode(&body)
			if errDecode != nil || body.PermissionLevel != 0 {
				http.Error(response, "body", http.StatusBadRequest)
				return
			}
			response.WriteHeader(http.StatusCreated)
		case "/api/userpermissions":
			_, errWrite := response.Write([]byte(`{"data":{"groups":[],"users":[{"name":"Fixture Player","permissionLevel":0,"userId":{"combinedString":"EOS_PLAYER_1"}}]}}`))
			if errWrite != nil {
				t.Errorf("write read-back response: %v", errWrite)
			}
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	serverURL, errURL := url.Parse(server.URL)
	if errURL != nil {
		t.Fatalf("parse native fixture URL: %v", errURL)
	}
	_, port, errPort := net.SplitHostPort(serverURL.Host)
	if errPort != nil {
		t.Fatalf("split native fixture host: %v", errPort)
	}
	workingDirectory := t.TempDir()
	config := fmt.Sprintf(`<ServerSettings><property name="WebDashboardEnabled" value="true"/><property name="WebDashboardPort" value="%s"/></ServerSettings>`, port)
	errWrite := os.WriteFile(filepath.Join(workingDirectory, "serverconfig.xml"), []byte(config), 0o600)
	if errWrite != nil {
		t.Fatalf("write native fixture config: %v", errWrite)
	}

	fixture, _, client := newPrivateReadGateFixture(t)
	_, errUpdate := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		"update game_server set directory = ? where id = ?",
		workingDirectory,
		"server-local-1",
	)
	if errUpdate != nil {
		t.Fatalf("update fixture working directory: %v", errUpdate)
	}
	client.RuntimeCapabilitiesResult.GameOperations = []node.GameOperationSupport{
		{GameID: sevenDaysToDieGameID, OperationIDs: []string{"player_access.add_administrator"}},
	}
	client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
		ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
		Capabilities:    node.SevenDaysToDieWebAPICapabilities{GamePermissions: true},
	}
	nodeInstance := node.New(t.Context(), nil, nil)
	client.ExecuteGameOperationFunc = func(ctx context.Context, request node.GameOperationRequest) (node.GameOperationResult, error) {
		return nodeInstance.ExecuteGameOperation(ctx, request), nil
	}
	return fixture, client, postRequests
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
