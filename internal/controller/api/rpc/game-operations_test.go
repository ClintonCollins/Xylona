package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestListGameServerOperations(t *testing.T) {
	t.Run("omits operations outside the viewer permission", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		grantGameServerRole(t, fixture, "user-other", "viewer")
		client.RuntimeCapabilitiesResult.GameOperations = allGameOperationSupport()
		client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
			ConnectionState:         node.SevenDaysToDieWebAPIConnectionStateAvailable,
			CommandOperationsState:  node.SevenDaysToDieWebAPIValueStateAvailable,
			SupportedGameOperations: allCommandGameOperationIDs(),
			AllowedGameOperations:   allCommandGameOperationIDs(),
			Capabilities: node.SevenDaysToDieWebAPICapabilities{
				CommandExecution: true,
				GamePermissions:  true,
			},
		}
		request := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

		response, errList := fixture.service.ListGameServerOperations(t.Context(), request)
		if errList != nil {
			t.Fatalf("ListGameServerOperations() error = %v", errList)
		}
		operations := response.Msg.GetOperations()
		if len(operations) != 6 {
			t.Fatalf("operations = %+v, want six read-only diagnostics", operations)
		}
		for _, operation := range operations {
			if operation.GetCategory() != "Server information" || operation.GetPermissionId() != "game_server.view" {
				t.Fatalf("viewer received unauthorized operation %+v", operation)
			}
		}
		if client.RuntimeCapabilitiesCalls != 1 || len(client.QuerySevenDaysToDieWebAPIStatusCalls) != 1 ||
			len(client.QuerySevenDaysToDiePlayersCalls) != 0 {
			t.Fatalf("viewer listing node calls: runtime=%d status=%d players=%d", client.RuntimeCapabilitiesCalls, len(client.QuerySevenDaysToDieWebAPIStatusCalls), len(client.QuerySevenDaysToDiePlayersCalls))
		}
	})

	tests := []struct {
		name                  string
		configure             func(*node.RuntimeCapabilities, *node.SevenDaysToDieWebAPIStatus)
		operationID           string
		wantAvailable         bool
		wantReason            xylona.GameOperationAvailabilityReason
		wantReasonText        string
		wantPlayerIdentities  bool
		wantNativeStatusCalls int
	}{
		{
			name: "available operation intersects controller node and native support",
			configure: func(_ *node.RuntimeCapabilities, status *node.SevenDaysToDieWebAPIStatus) {
				status.Capabilities.PlayerData = true
			},
			operationID:           "player_access.add_administrator",
			wantAvailable:         true,
			wantPlayerIdentities:  true,
			wantNativeStatusCalls: 1,
		},
		{
			name: "older node disables the operation before native discovery",
			configure: func(capabilities *node.RuntimeCapabilities, _ *node.SevenDaysToDieWebAPIStatus) {
				capabilities.ProtocolVersion = gameOperationsNodeProtocol - 1
			},
			operationID: "player_access.add_administrator",
			wantReason:  xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNSUPPORTED,
		},
		{
			name: "node without operation support disables the operation",
			configure: func(capabilities *node.RuntimeCapabilities, _ *node.SevenDaysToDieWebAPIStatus) {
				capabilities.GameOperations = nil
			},
			operationID: "player_access.add_administrator",
			wantReason:  xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNSUPPORTED,
		},
		{
			name: "missing native permission capability gives a concrete reason",
			configure: func(_ *node.RuntimeCapabilities, status *node.SevenDaysToDieWebAPIStatus) {
				status.Capabilities.GamePermissions = false
			},
			operationID:           "player_access.add_administrator",
			wantReason:            xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_GAME_PERMISSION_UNSUPPORTED,
			wantNativeStatusCalls: 1,
		},
		{
			name: "missing command capability gives a concrete reason",
			configure: func(_ *node.RuntimeCapabilities, status *node.SevenDaysToDieWebAPIStatus) {
				status.Capabilities.CommandExecution = false
			},
			operationID:           "communication.broadcast_message",
			wantReason:            xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED,
			wantReasonText:        "command execution",
			wantNativeStatusCalls: 1,
		},
		{
			name: "missing native command gives a concrete capability reason",
			configure: func(_ *node.RuntimeCapabilities, status *node.SevenDaysToDieWebAPIStatus) {
				status.SupportedGameOperations = []string{"server_information.version"}
			},
			operationID:           "communication.broadcast_message",
			wantReason:            xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED,
			wantReasonText:        "does not expose",
			wantNativeStatusCalls: 1,
		},
		{
			name: "configured token without command permission gives a concrete reason",
			configure: func(_ *node.RuntimeCapabilities, status *node.SevenDaysToDieWebAPIStatus) {
				status.AllowedGameOperations = []string{"server_information.version"}
			},
			operationID:           "communication.broadcast_message",
			wantReason:            xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED,
			wantReasonText:        "not allowed",
			wantNativeStatusCalls: 1,
		},
		{
			name: "give item requires native player lookup",
			configure: func(_ *node.RuntimeCapabilities, status *node.SevenDaysToDieWebAPIStatus) {
				status.Capabilities.PlayerData = false
			},
			operationID:           "player_assistance.give_item",
			wantReason:            xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED,
			wantReasonText:        "player lookup",
			wantNativeStatusCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, _, client := newPrivateReadGateFixture(t)
			client.RuntimeCapabilitiesResult.GameOperations = allGameOperationSupport()
			client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
				ConnectionState:         node.SevenDaysToDieWebAPIConnectionStateAvailable,
				CommandOperationsState:  node.SevenDaysToDieWebAPIValueStateAvailable,
				SupportedGameOperations: allCommandGameOperationIDs(),
				AllowedGameOperations:   allCommandGameOperationIDs(),
				Capabilities: node.SevenDaysToDieWebAPICapabilities{
					CommandExecution: true,
					GamePermissions:  true,
				},
			}
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
			if len(operations) != 29 {
				t.Fatalf("operation count = %d, want 29", len(operations))
			}
			operation := publicOperationByID(t, operations, test.operationID)
			if operation.GetAvailable() != test.wantAvailable ||
				operation.GetAvailabilityReason() != test.wantReason {
				t.Fatalf("operation availability = %+v", operation)
			}
			if !test.wantAvailable && strings.TrimSpace(operation.GetAvailabilityReasonText()) == "" {
				t.Fatal("disabled operation omitted its human-readable reason")
			}
			if len(client.QuerySevenDaysToDieWebAPIStatusCalls) != test.wantNativeStatusCalls {
				t.Fatalf("native status calls = %d, want %d", len(client.QuerySevenDaysToDieWebAPIStatusCalls), test.wantNativeStatusCalls)
			}
			if test.wantReasonText != "" && !strings.Contains(strings.ToLower(operation.GetAvailabilityReasonText()), test.wantReasonText) {
				t.Fatalf("operation reason = %q, want text containing %q", operation.GetAvailabilityReasonText(), test.wantReasonText)
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

	t.Run("native command capability controls Server control availability", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		client.RuntimeCapabilitiesResult.GameOperations = []node.GameOperationSupport{
			{
				GameID: sevenDaysToDieGameID,
				OperationIDs: []string{
					"player_access.add_administrator",
					"server_control.save_world",
					"server_control.set_game_time",
					"server_control.set_temperature_unit",
					"server_control.shutdown",
				},
			},
		}
		client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
			ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
			Capabilities: node.SevenDaysToDieWebAPICapabilities{
				GamePermissions:  true,
				CommandExecution: false,
			},
		}

		request := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
		response, errList := fixture.service.ListGameServerOperations(t.Context(), request)
		if errList != nil {
			t.Fatalf("ListGameServerOperations() error = %v", errList)
		}
		if len(client.GetProcessSnapshotCalls) != 1 || client.RuntimeCapabilitiesCalls != 1 ||
			len(client.QuerySevenDaysToDieWebAPIStatusCalls) != 1 {
			t.Fatalf(
				"shared availability calls = process %d, runtime %d, native %d; want 1 each",
				len(client.GetProcessSnapshotCalls),
				client.RuntimeCapabilitiesCalls,
				len(client.QuerySevenDaysToDieWebAPIStatusCalls),
			)
		}
		addAdministrator := publicOperationByID(t, response.Msg.GetOperations(), "player_access.add_administrator")
		saveWorld := publicOperationByID(t, response.Msg.GetOperations(), "server_control.save_world")
		setTime := publicOperationByID(t, response.Msg.GetOperations(), "server_control.set_game_time")
		setTemperatureUnit := publicOperationByID(t, response.Msg.GetOperations(), "server_control.set_temperature_unit")
		shutdown := publicOperationByID(t, response.Msg.GetOperations(), "server_control.shutdown")
		if !addAdministrator.GetAvailable() || saveWorld.GetAvailable() || setTime.GetAvailable() ||
			setTemperatureUnit.GetAvailable() || shutdown.GetAvailable() {
			t.Fatalf(
				"operation availability = add %t, save %t, set time %t, set temperature %t, shutdown %t",
				addAdministrator.GetAvailable(),
				saveWorld.GetAvailable(),
				setTime.GetAvailable(),
				setTemperatureUnit.GetAvailable(),
				shutdown.GetAvailable(),
			)
		}
		for _, operation := range []*xylona.GameOperationDescriptor{saveWorld, setTime, setTemperatureUnit, shutdown} {
			if operation.GetAvailabilityReason() != xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED ||
				!strings.Contains(operation.GetAvailabilityReasonText(), "command") {
				t.Fatalf("command operation availability = %+v", operation)
			}
		}
	})

	t.Run("offline server disables every modeled operation", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		client.GetProcessSnapshotResult = &node.ProcessSnapshot{Status: xylona.Status_OFFLINE.String()}
		request := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errList := fixture.service.ListGameServerOperations(t.Context(), request)
		if errList != nil {
			t.Fatalf("ListGameServerOperations() error = %v", errList)
		}
		if len(response.Msg.GetOperations()) != 29 || client.RuntimeCapabilitiesCalls != 1 ||
			len(client.QuerySevenDaysToDieWebAPIStatusCalls) != 0 {
			t.Fatalf(
				"offline operation count = %d, runtime calls = %d, native calls = %d",
				len(response.Msg.GetOperations()),
				client.RuntimeCapabilitiesCalls,
				len(client.QuerySevenDaysToDieWebAPIStatusCalls),
			)
		}
		for _, operation := range response.Msg.GetOperations() {
			if operation.GetAvailable() ||
				operation.GetAvailabilityReason() != xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_OFFLINE ||
				!strings.Contains(operation.GetAvailabilityReasonText(), "Start") {
				t.Fatalf("offline operation availability = %+v", operation)
			}
		}
	})

	t.Run("offline server still exposes file-derived operation choices", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		client.GetProcessSnapshotResult = &node.ProcessSnapshot{Status: xylona.Status_OFFLINE.String()}
		client.QuerySevenDaysToDieOperationMetadataResult = &node.SevenDaysToDieOperationMetadata{
			Players: []node.SevenDaysToDieOperationOption{{Label: "Alice", Value: "EOS_abc123", Description: "Saved player"}},
			Items: []node.SevenDaysToDieOperationOption{{
				Label: "Wood", Value: "resourceWood", IconName: "resourceWood", Category: "Resources",
			}},
			Buffs:    []node.SevenDaysToDieOperationOption{{Label: "Warmth", Value: "buffWarm", AccentColor: "#ff8000"}},
			Commands: []node.SevenDaysToDieOperationOption{{Label: "teleport", Value: "teleport"}},
		}
		request := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errList := fixture.service.ListGameServerOperations(t.Context(), request)
		if errList != nil {
			t.Fatalf("ListGameServerOperations() error = %v", errList)
		}
		if len(client.QuerySevenDaysToDieOperationMetadataCalls) != 1 {
			t.Fatalf("metadata calls = %d, want 1", len(client.QuerySevenDaysToDieOperationMetadataCalls))
		}
		addAdministrator := publicOperationByID(t, response.Msg.GetOperations(), gameintegrations.OperationIDAddAdministrator)
		giveItem := publicOperationByID(t, response.Msg.GetOperations(), gameintegrations.OperationIDGiveItem)
		applyBuff := publicOperationByID(t, response.Msg.GetOperations(), gameintegrations.OperationIDApplyBuff)
		setPermission := publicOperationByID(t, response.Msg.GetOperations(), gameintegrations.OperationIDSetCommandPermission)
		assertPublicOperationFieldOption(t, addAdministrator, "player", "EOS_abc123", "")
		itemOption := assertPublicOperationFieldOption(t, giveItem, "item", "resourceWood", SevenDaysToDieOperationItemIconPathPrefix+"/server-local-1/resourceWood.png")
		if itemOption.GetCategory() != "Resources" {
			t.Fatalf("item option = %+v", itemOption)
		}
		buffOption := assertPublicOperationFieldOption(t, applyBuff, "buff", "buffWarm", "")
		if buffOption.GetAccentColor() != "#ff8000" {
			t.Fatalf("buff option = %+v", buffOption)
		}
		assertPublicOperationFieldOption(t, setPermission, "command", "teleport", "")
	})

	t.Run("online catalogs distinguish saved players from teleport targets", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		client.QuerySevenDaysToDieOperationMetadataResult = &node.SevenDaysToDieOperationMetadata{
			Players:  []node.SevenDaysToDieOperationOption{{Label: "Alice", Value: "EOS_OFFLINE", Description: "Saved player"}},
			Commands: []node.SevenDaysToDieOperationOption{{Label: "saveworld", Value: "saveworld", Description: "Configured command"}},
		}
		client.RuntimeCapabilitiesResult.GameOperations = allGameOperationSupport()
		client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
			ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
			Capabilities: node.SevenDaysToDieWebAPICapabilities{
				PlayerData: true, GamePermissions: true, CommandExecution: true, CommandPermissions: true,
			},
			CommandOperationsState:  node.SevenDaysToDieWebAPIValueStateAvailable,
			SupportedGameOperations: allCommandGameOperationIDs(),
			AllowedGameOperations:   allCommandGameOperationIDs(),
			KnownCommands:           []string{"saveworld", "teleport", "version"},
		}
		client.QuerySevenDaysToDiePlayersResult = &node.SevenDaysToDiePlayers{
			State: node.SevenDaysToDieWebAPIValueStateAvailable,
			Players: []node.SevenDaysToDiePlayer{{
				Name: "Bob", ActionID: "EOS_ONLINE", PlatformID: "EOS_ONLINE",
			}},
		}
		request := connect.NewRequest(&xylona.ListGameServerOperationsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errList := fixture.service.ListGameServerOperations(t.Context(), request)
		if errList != nil {
			t.Fatalf("ListGameServerOperations() error = %v", errList)
		}
		addAdministrator := publicOperationByID(t, response.Msg.GetOperations(), gameintegrations.OperationIDAddAdministrator)
		teleport := publicOperationByID(t, response.Msg.GetOperations(), gameintegrations.OperationIDTeleportPlayer)
		setPermission := publicOperationByID(t, response.Msg.GetOperations(), gameintegrations.OperationIDSetCommandPermission)
		assertPublicOperationFieldOption(t, addAdministrator, "player", "EOS_OFFLINE", "")
		assertPublicOperationFieldOption(t, addAdministrator, "player", "EOS_ONLINE", "")
		assertPublicOperationFieldOption(t, setPermission, "command", "saveworld", "")
		assertPublicOperationFieldOption(t, setPermission, "command", "teleport", "")
		assertPublicOperationFieldOption(t, setPermission, "command", "version", "")
		for _, field := range teleport.GetFields() {
			if field.GetId() != "destination" {
				continue
			}
			if len(field.GetOptions()) != 1 || field.GetOptions()[0].GetValue() != "EOS_ONLINE" {
				t.Fatalf("teleport destination options = %+v", field.GetOptions())
			}
			return
		}
		t.Fatal("teleport destination field was not listed")
	})
}

func TestExecuteGameServerOperation(t *testing.T) {
	t.Run("executes through controller and node to authoritative read-back", func(t *testing.T) {
		fixture, client, transport := newGameOperationVerticalFixture(t)
		request := validPublicAddAdministratorRequest()
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), request)
		if errExecute != nil {
			t.Fatalf("ExecuteGameServerOperation() error = %v", errExecute)
		}
		result := response.Msg.GetResult()
		if result.GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED ||
			!strings.Contains(result.GetMessage(), "confirmed") || transport.postRequests.Load() != 1 {
			t.Fatalf("result = %+v, native POST requests = %d", result, transport.postRequests.Load())
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

	t.Run("executes remaining permission operations through authoritative read-back", func(t *testing.T) {
		fixture, client, transport := newGameOperationVerticalFixture(t)
		requests := []*connect.Request[xylona.ExecuteGameServerOperationRequest]{
			validPublicOperationRequest(
				"player_access.remove_administrator",
				&xylona.GameOperationValue{FieldId: "player", Value: &xylona.GameOperationValue_StringValue{StringValue: "EOS_PLAYER_1"}},
			),
			validPublicOperationRequest(
				"permissions.set_command_permission",
				&xylona.GameOperationValue{FieldId: "command", Value: &xylona.GameOperationValue_StringValue{StringValue: "version"}},
				&xylona.GameOperationValue{FieldId: "permission_level", Value: &xylona.GameOperationValue_IntegerValue{IntegerValue: 7}},
			),
			validPublicOperationRequest(
				"permissions.reset_command_permission",
				&xylona.GameOperationValue{FieldId: "command", Value: &xylona.GameOperationValue_StringValue{StringValue: "version"}},
			),
		}
		for _, request := range requests {
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			response, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), request)
			if errExecute != nil ||
				response.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED {
				t.Fatalf("operation %q result = %+v, error = %v", request.Msg.GetOperationId(), response, errExecute)
			}
		}
		if transport.postRequests.Load() != 3 || len(client.ExecuteGameOperationCalls) != 3 {
			t.Fatalf("native mutations = %d, node calls = %d", transport.postRequests.Load(), len(client.ExecuteGameOperationCalls))
		}
	})

	t.Run("executes the demonstrated Server control operations through the native command transport", func(t *testing.T) {
		fixture, _, transport := newGameOperationVerticalFixture(t)
		tests := []struct {
			name        string
			request     *connect.Request[xylona.ExecuteGameServerOperationRequest]
			wantCommand string
		}{
			{name: "save world", request: validPublicOperationRequest("server_control.save_world"), wantCommand: "saveworld"},
			{
				name: "set temperature unit",
				request: validPublicOperationRequest(
					"server_control.set_temperature_unit",
					&xylona.GameOperationValue{FieldId: "unit", Value: &xylona.GameOperationValue_StringValue{StringValue: "C"}},
				),
				wantCommand: "settempunit C",
			},
			{name: "shutdown", request: validPublicOperationRequest("server_control.shutdown"), wantCommand: "shutdown"},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, test.request, "user-owner")
				response, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), test.request)
				if errExecute != nil {
					t.Fatalf("ExecuteGameServerOperation() error = %v", errExecute)
				}
				result := response.Msg.GetResult()
				if result.GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_ACCEPTED_BUT_UNVERIFIED ||
					!strings.Contains(result.GetMessage(), "cannot be verified") {
					t.Fatalf("result = %+v", result)
				}
				if got := transport.lastCommand(); got != test.wantCommand {
					t.Fatalf("native command = %q, want %q", got, test.wantCommand)
				}
			})
		}
	})

	t.Run("executes communication and diagnostics through the native command API", func(t *testing.T) {
		tests := []struct {
			name               string
			operationID        string
			values             []*xylona.GameOperationValue
			wantClassification xylona.GameOperationResultClassification
			wantMessage        string
		}{
			{
				name:        "broadcast accepted without delivery read-back",
				operationID: "communication.broadcast_message",
				values: []*xylona.GameOperationValue{
					{FieldId: "message", Value: &xylona.GameOperationValue_StringValue{StringValue: "Server restart soon"}},
				},
				wantClassification: xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_ACCEPTED_BUT_UNVERIFIED,
				wantMessage:        "delivery could not be verified",
			},
			{
				name:               "version confirmed from diagnostic output",
				operationID:        "server_information.version",
				wantClassification: xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED,
				wantMessage:        "Version: V2.6 b22",
			},
			{
				name:        "experience grant stays honest without Player read-back",
				operationID: "player_assistance.give_experience",
				values: []*xylona.GameOperationValue{
					{FieldId: "player", Value: &xylona.GameOperationValue_StringValue{StringValue: "EOS_PLAYER_1"}},
					{FieldId: "experience", Value: &xylona.GameOperationValue_IntegerValue{IntegerValue: 2500}},
				},
				wantClassification: xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_ACCEPTED_BUT_UNVERIFIED,
				wantMessage:        "delivery could not be verified",
			},
			{
				name:        "weather preset stays honest without world-state read-back",
				operationID: "world_events.set_weather",
				values: []*xylona.GameOperationValue{
					{FieldId: "weather", Value: &xylona.GameOperationValue_StringValue{StringValue: "rain"}},
				},
				wantClassification: xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_ACCEPTED_BUT_UNVERIFIED,
				wantMessage:        "delivery could not be verified",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				fixture, client, transport := newGameOperationVerticalFixture(t)
				request := connect.NewRequest(&xylona.ExecuteGameServerOperationRequest{
					GameServerId: "server-local-1",
					OperationId:  test.operationID,
					Values:       test.values,
				})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

				response, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), request)
				if errExecute != nil {
					t.Fatalf("ExecuteGameServerOperation() error = %v", errExecute)
				}
				result := response.Msg.GetResult()
				if result.GetClassification() != test.wantClassification ||
					!strings.Contains(result.GetMessage(), test.wantMessage) || transport.postRequests.Load() != 1 {
					t.Fatalf("result = %+v, native POST requests = %d", result, transport.postRequests.Load())
				}
				if len(client.ExecuteGameOperationCalls) != 1 ||
					client.ExecuteGameOperationCalls[0].OperationID != test.operationID {
					t.Fatalf("node execution calls = %+v", client.ExecuteGameOperationCalls)
				}
			})
		}
	})

	t.Run("confirms set game time from authoritative world time read-back", func(t *testing.T) {
		fixture, _, transport := newGameOperationVerticalFixture(t)
		request := validPublicSetTimeRequest("night")
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), request)
		if errExecute != nil {
			t.Fatalf("ExecuteGameServerOperation() error = %v", errExecute)
		}
		result := response.Msg.GetResult()
		if result.GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED ||
			!strings.Contains(result.GetMessage(), "confirmed") ||
			result.GetTransportDetails().GetVerification() != "World time read-back" ||
			transport.serverStatsRequests.Load() != 1 {
			t.Fatalf("result = %+v, ServerStats requests = %d", result, transport.serverStatsRequests.Load())
		}
		if got := transport.lastCommand(); got != "settime night" {
			t.Fatalf("native command = %q, want %q", got, "settime night")
		}
	})
	t.Run("node validation rejects an unknown structured field", func(t *testing.T) {
		fixture, client, transport := newGameOperationVerticalFixture(t)
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
			!strings.Contains(result.GetMessage(), "Unknown operation field") || transport.postRequests.Load() != 0 ||
			len(client.ExecuteGameOperationCalls) != 1 {
			t.Fatalf("result = %+v, native POST requests = %d, node calls = %d", result, transport.postRequests.Load(), len(client.ExecuteGameOperationCalls))
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
		if errList != nil {
			t.Fatalf("listed operations = %+v, error = %v", listResponse, errList)
		}
		publicOperationByID(t, listResponse.Msg.GetOperations(), "player_access.add_administrator")

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

	t.Run("independent operations proceed while duplicate target and exact conflicts fail", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		configureGameOperationExecution(client)
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
		client.ExecuteGameOperationFunc = func(_ context.Context, request node.GameOperationRequest) (node.GameOperationResult, error) {
			executions.Add(1)
			if request.OperationID == "server_control.set_game_time" {
				close(started)
				<-release
			}
			return node.GameOperationResult{Classification: node.GameOperationResultConfirmed, Message: "confirmed"}, nil
		}

		firstDone := make(chan error, 1)
		firstRequest := validPublicSetTimeRequest("day")
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, firstRequest, "user-owner")
		go func() {
			_, errExecute := fixture.service.ExecuteGameServerOperation(t.Context(), firstRequest)
			firstDone <- errExecute
		}()
		<-started

		independentRequests := []*connect.Request[xylona.ExecuteGameServerOperationRequest]{
			validPublicAddAdministratorRequest(),
			validPublicOperationRequest("server_control.save_world"),
			validPublicOperationRequest(
				"server_control.set_temperature_unit",
				&xylona.GameOperationValue{FieldId: "unit", Value: &xylona.GameOperationValue_StringValue{StringValue: "F"}},
			),
		}
		for _, independentRequest := range independentRequests {
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, independentRequest, "user-owner")
			independentResponse, errIndependent := fixture.service.ExecuteGameServerOperation(t.Context(), independentRequest)
			if errIndependent != nil || independentResponse.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED {
				t.Fatalf("independent result = %+v, error = %v", independentResponse, errIndependent)
			}
		}

		duplicateRequest := validPublicSetTimeRequest("day")
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, duplicateRequest, "user-owner")
		duplicateResponse, errDuplicate := fixture.service.ExecuteGameServerOperation(t.Context(), duplicateRequest)
		if errDuplicate != nil ||
			duplicateResponse.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED ||
			!strings.Contains(duplicateResponse.Msg.GetResult().GetMessage(), "identical") {
			t.Fatalf("duplicate result = %+v, error = %v", duplicateResponse, errDuplicate)
		}

		sameTargetRequest := validPublicSetTimeRequest("night")
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, sameTargetRequest, "user-owner")
		sameTargetResponse, errSameTarget := fixture.service.ExecuteGameServerOperation(t.Context(), sameTargetRequest)
		if errSameTarget != nil ||
			sameTargetResponse.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED ||
			!strings.Contains(sameTargetResponse.Msg.GetResult().GetMessage(), "same target") {
			t.Fatalf("same-target result = %+v, error = %v", sameTargetResponse, errSameTarget)
		}

		conflictingRequest := validPublicOperationRequest("server_control.shutdown")
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, conflictingRequest, "user-owner")
		conflictingResponse, errConflict := fixture.service.ExecuteGameServerOperation(t.Context(), conflictingRequest)
		if errConflict != nil ||
			conflictingResponse.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED ||
			!strings.Contains(conflictingResponse.Msg.GetResult().GetMessage(), "Set game time") {
			t.Fatalf("conflicting result = %+v, error = %v", conflictingResponse, errConflict)
		}
		if executions.Load() != 4 {
			t.Fatalf("node executions = %d, want only independent requests", executions.Load())
		}

		close(release)
		if errFirst := <-firstDone; errFirst != nil {
			t.Fatalf("first ExecuteGameServerOperation() error = %v", errFirst)
		}
	})

	t.Run("locks release after node success and failure", func(t *testing.T) {
		fixture, _, client := newPrivateReadGateFixture(t)
		configureGameOperationExecution(client)
		var executions atomic.Int64
		client.ExecuteGameOperationFunc = func(_ context.Context, _ node.GameOperationRequest) (node.GameOperationResult, error) {
			if executions.Add(1) == 1 {
				return node.GameOperationResult{}, errors.New("fixture node failure")
			}
			return node.GameOperationResult{Classification: node.GameOperationResultConfirmed, Message: "confirmed"}, nil
		}

		firstRequest := validPublicSetTimeRequest("day")
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, firstRequest, "user-owner")
		firstResponse, errFirst := fixture.service.ExecuteGameServerOperation(t.Context(), firstRequest)
		if errFirst != nil || firstResponse.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED {
			t.Fatalf("failed node result = %+v, error = %v", firstResponse, errFirst)
		}

		secondRequest := validPublicSetTimeRequest("day")
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, secondRequest, "user-owner")
		secondResponse, errSecond := fixture.service.ExecuteGameServerOperation(t.Context(), secondRequest)
		if errSecond != nil || secondResponse.Msg.GetResult().GetClassification() != xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED || executions.Load() != 2 {
			t.Fatalf("retry result = %+v, error = %v, executions = %d", secondResponse, errSecond, executions.Load())
		}
	})
}

func publicOperationByID(t *testing.T, operations []*xylona.GameOperationDescriptor, operationID string) *xylona.GameOperationDescriptor {
	t.Helper()
	for _, operation := range operations {
		if operation.GetId() == operationID {
			return operation
		}
	}
	t.Fatalf("operation %q was not listed", operationID)
	return nil
}

func assertPublicOperationFieldOption(
	t *testing.T,
	operation *xylona.GameOperationDescriptor,
	fieldID string,
	value string,
	iconURL string,
) *xylona.GameOperationFieldOption {
	t.Helper()
	for _, field := range operation.GetFields() {
		if field.GetId() != fieldID {
			continue
		}
		for _, option := range field.GetOptions() {
			if option.GetValue() == value && option.GetIconUrl() == iconURL {
				return option
			}
		}
		t.Fatalf("operation %q field %q options = %+v", operation.GetId(), fieldID, field.GetOptions())
	}
	t.Fatalf("operation %q field %q was not listed", operation.GetId(), fieldID)
	return nil
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

func validPublicSetTimeRequest(timeValue string) *connect.Request[xylona.ExecuteGameServerOperationRequest] {
	return validPublicOperationRequest(
		"server_control.set_game_time",
		&xylona.GameOperationValue{FieldId: "time", Value: &xylona.GameOperationValue_StringValue{StringValue: timeValue}},
	)
}

func validPublicOperationRequest(operationID string, values ...*xylona.GameOperationValue) *connect.Request[xylona.ExecuteGameServerOperationRequest] {
	return connect.NewRequest(&xylona.ExecuteGameServerOperationRequest{
		GameServerId: "server-local-1",
		OperationId:  operationID,
		Values:       values,
	})
}

func configureGameOperationExecution(client *nodeclient.FakeNodeClient) {
	client.RuntimeCapabilitiesResult.GameOperations = []node.GameOperationSupport{
		{
			GameID: sevenDaysToDieGameID,
			OperationIDs: []string{
				"player_access.add_administrator",
				"server_control.save_world",
				"server_control.set_game_time",
				"server_control.set_temperature_unit",
				"server_control.shutdown",
			},
		},
	}
	client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
		ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
		Capabilities: node.SevenDaysToDieWebAPICapabilities{
			GamePermissions:  true,
			CommandExecution: true,
		},
		CommandOperationsState:  node.SevenDaysToDieWebAPIValueStateAvailable,
		SupportedGameOperations: allCommandGameOperationIDs(),
		AllowedGameOperations:   allCommandGameOperationIDs(),
	}
}

func newGameOperationVerticalFixture(
	t *testing.T,
) (*rbacRPCFixture, *nodeclient.FakeNodeClient, *gameOperationTransportRecorder) {
	t.Helper()
	transport := new(gameOperationTransportRecorder)
	worldTimeReadBack, errReadWorldTime := os.ReadFile(filepath.Join(
		"..", "..", "..", "node", "testdata", "seven-days-to-die", "v2.6-build-22422094", "results", "set-game-time-readback.json",
	))
	if errReadWorldTime != nil {
		t.Fatalf("read Set game time fixture: %v", errReadWorldTime)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/openapi/openapi.yaml":
			_, errWrite := response.Write([]byte(`openapi: 3.1.0
info:
  version: '1.0.0'
paths:
  /api/serverstats:
    get: {}
  /api/userpermissions:
    get: {}
  /api/userpermissions/user/{id}:
    post: {}
    delete: {}
  /api/commandpermissions:
    get: {}
  /api/commandpermissions/{command}:
    post: {}
    delete: {}
  /api/command:
    get: {}
    post: {}
`))
			if errWrite != nil {
				t.Errorf("write OpenAPI response: %v", errWrite)
			}
		case "/api/userpermissions/user/EOS_PLAYER_1":
			if request.Method == http.MethodDelete {
				transport.postRequests.Add(1)
				transport.administratorRemoved.Store(true)
				response.WriteHeader(http.StatusNoContent)
				return
			}
			if request.Method != http.MethodPost {
				http.Error(response, "method", http.StatusMethodNotAllowed)
				return
			}
			transport.postRequests.Add(1)
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
		case "/api/command":
			if request.Method == http.MethodGet {
				_, errWrite := response.Write([]byte(`{"data":{"commands":[{"command":"say","allowed":true},{"command":"sayplayer","allowed":true},{"command":"teleportplayer","allowed":true},{"command":"give","allowed":true},{"command":"givexp","allowed":true},{"command":"buffplayer","allowed":true},{"command":"debuffplayer","allowed":true},{"command":"spawnairdrop","allowed":true},{"command":"spawnwandering","allowed":true},{"command":"weather","allowed":true},{"command":"getgamepref","allowed":true},{"command":"getgamestat","allowed":true},{"command":"gettime","allowed":true},{"command":"listdlc","allowed":true},{"command":"listitems","allowed":true},{"command":"version","allowed":true},{"command":"saveworld","allowed":true},{"command":"settempunit","allowed":true},{"command":"settime","allowed":true},{"command":"shutdown","allowed":true}]}}`))
				if errWrite != nil {
					t.Errorf("write command catalog response: %v", errWrite)
				}
				return
			}
			if request.Method != http.MethodPost {
				http.Error(response, "method", http.StatusMethodNotAllowed)
				return
			}
			transport.postRequests.Add(1)
			var body struct {
				Command string `json:"command"`
				Format  string `json:"format"`
			}
			errDecode := json.NewDecoder(request.Body).Decode(&body)
			if errDecode != nil || (body.Format != "Simple" && body.Format != "Full") {
				http.Error(response, "body", http.StatusBadRequest)
				return
			}
			transport.recordCommand(body.Command)
			if body.Format == "Simple" {
				response.WriteHeader(http.StatusOK)
				return
			}
			switch body.Command {
			case `say "Server restart soon"`:
				_, errWrite := response.Write([]byte(`{"data":{"command":"say","parameters":"\"Server restart soon\"","result":"Message delivered"}}`))
				if errWrite != nil {
					t.Errorf("write command response: %v", errWrite)
				}
			case "version":
				_, errWrite := response.Write([]byte(`{"data":{"command":"version","parameters":"","result":"Version: V2.6 b22"}}`))
				if errWrite != nil {
					t.Errorf("write command response: %v", errWrite)
				}
			case `givexp "EOS_PLAYER_1" 2500`:
				_, errWrite := response.Write([]byte(`{"data":{"command":"givexp","parameters":"\"EOS_PLAYER_1\" 2500","result":"Experience granted"}}`))
				if errWrite != nil {
					t.Errorf("write command response: %v", errWrite)
				}
			case "weather Rain 1":
				_, errWrite := response.Write([]byte(`{"data":{"command":"weather","parameters":"Rain 1","result":"Weather changed"}}`))
				if errWrite != nil {
					t.Errorf("write command response: %v", errWrite)
				}
			default:
				http.Error(response, "command", http.StatusBadRequest)
			}
		case "/api/serverstats":
			if request.Method != http.MethodGet {
				http.Error(response, "method", http.StatusMethodNotAllowed)
				return
			}
			if request.Header.Get("X-SDTD-API-TOKENNAME") == "" || request.Header.Get("X-SDTD-API-SECRET") == "" {
				http.Error(response, "credentials", http.StatusUnauthorized)
				return
			}
			transport.serverStatsRequests.Add(1)
			_, errWrite := response.Write(worldTimeReadBack)
			if errWrite != nil {
				t.Errorf("write ServerStats response: %v", errWrite)
			}
		case "/api/userpermissions":
			body := `{"data":{"groups":[],"users":[{"name":"Fixture Player","permissionLevel":0,"userId":{"combinedString":"EOS_PLAYER_1"}}]}}`
			if transport.administratorRemoved.Load() {
				body = `{"data":{"groups":[],"users":[]}}`
			}
			_, errWrite := response.Write([]byte(body))
			if errWrite != nil {
				t.Errorf("write read-back response: %v", errWrite)
			}
		case "/api/commandpermissions":
			body := fmt.Sprintf(`{"data":[{"command":"version","default":%t,"permissionLevel":%d}]}`,
				transport.commandPermissionDefault.Load(), transport.commandPermissionLevel.Load())
			_, errWrite := response.Write([]byte(body))
			if errWrite != nil {
				t.Errorf("write command permission response: %v", errWrite)
			}
		case "/api/commandpermissions/version":
			transport.postRequests.Add(1)
			if request.Method == http.MethodDelete {
				transport.commandPermissionDefault.Store(true)
				transport.commandPermissionLevel.Store(0)
				response.WriteHeader(http.StatusNoContent)
				return
			}
			if request.Method != http.MethodPost {
				http.Error(response, "method", http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				PermissionLevel int64 `json:"permissionLevel"`
			}
			errDecode := json.NewDecoder(request.Body).Decode(&body)
			if errDecode != nil {
				http.Error(response, "body", http.StatusBadRequest)
				return
			}
			transport.commandPermissionDefault.Store(false)
			transport.commandPermissionLevel.Store(body.PermissionLevel)
			response.WriteHeader(http.StatusCreated)
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
	client.RuntimeCapabilitiesResult.GameOperations = allGameOperationSupport()
	client.QuerySevenDaysToDieWebAPIStatusResult = &node.SevenDaysToDieWebAPIStatus{
		ConnectionState:         node.SevenDaysToDieWebAPIConnectionStateAvailable,
		CommandOperationsState:  node.SevenDaysToDieWebAPIValueStateAvailable,
		SupportedGameOperations: allCommandGameOperationIDs(),
		AllowedGameOperations:   allCommandGameOperationIDs(),
		Capabilities: node.SevenDaysToDieWebAPICapabilities{
			CommandExecution:   true,
			CommandPermissions: true,
			GamePermissions:    true,
		},
	}
	nodeInstance := node.New(t.Context(), nil, nil)
	client.ExecuteGameOperationFunc = func(ctx context.Context, request node.GameOperationRequest) (node.GameOperationResult, error) {
		return nodeInstance.ExecuteGameOperation(ctx, request), nil
	}
	return fixture, client, transport
}

type gameOperationTransportRecorder struct {
	postRequests             atomic.Int64
	serverStatsRequests      atomic.Int64
	administratorRemoved     atomic.Bool
	commandPermissionDefault atomic.Bool
	commandPermissionLevel   atomic.Int64
	mu                       sync.Mutex
	commands                 []string
}

func (r *gameOperationTransportRecorder) recordCommand(command string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, command)
}

func (r *gameOperationTransportRecorder) lastCommand() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.commands) == 0 {
		return ""
	}
	return r.commands[len(r.commands)-1]
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

func allGameOperationSupport() []node.GameOperationSupport {
	return new(node.Node).RuntimeCapabilities().GameOperations
}

func allCommandGameOperationIDs() []string {
	return []string{
		"communication.broadcast_message",
		"communication.message_player",
		"player_assistance.teleport_to_player",
		"player_assistance.give_item",
		"player_assistance.give_experience",
		"player_assistance.apply_buff",
		"player_assistance.remove_buff",
		"server_information.game_preferences",
		"server_information.game_statistics",
		"server_information.game_time",
		"server_information.dlc_status",
		"server_information.item_search",
		"server_information.version",
		"server_control.save_world",
		"server_control.set_temperature_unit",
		"server_control.set_game_time",
		"server_control.shutdown",
		"world_events.spawn_airdrop",
		"world_events.spawn_wandering_horde",
		"world_events.set_weather",
	}
}
