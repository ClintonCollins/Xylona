package node

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNodeExecuteGameOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		modifyRequest          func(*GameOperationRequest)
		operationID            string
		values                 []GameOperationValue
		postStatus             int
		commandStatus          int
		wantCommand            string
		readBackStatus         int
		readBackBody           string
		worldTimeStatus        int
		worldTimeBody          string
		waitForPostTimeout     bool
		waitForReadBackTimeout bool
		wantClassification     GameOperationResultClassification
		wantMessage            string
		wantPost               bool
		wantWorldTimeReadBack  bool
	}{
		{
			name:               "authoritative read-back confirms the native operation",
			postStatus:         http.StatusCreated,
			readBackStatus:     http.StatusOK,
			readBackBody:       readGameOperationFixture(t, "permissions", "user-readback.json"),
			wantClassification: GameOperationResultConfirmed,
			wantMessage:        "confirmed",
			wantPost:           true,
		},
		{
			name:               "accepted request with unavailable read-back stays unverified",
			postStatus:         http.StatusCreated,
			readBackStatus:     http.StatusServiceUnavailable,
			wantClassification: GameOperationResultAcceptedButUnverified,
			wantMessage:        "could not be verified",
			wantPost:           true,
		},
		{
			name:               "server rejection fails without exposing its response",
			postStatus:         http.StatusBadRequest,
			wantClassification: GameOperationResultFailed,
			wantMessage:        "rejected",
			wantPost:           true,
		},
		{
			name: "unknown operation is rejected before transport",
			modifyRequest: func(request *GameOperationRequest) {
				request.OperationID = "player_access.unknown"
			},
			wantClassification: GameOperationResultFailed,
			wantMessage:        "Unknown game operation",
		},
		{
			name: "unknown field is rejected before transport",
			modifyRequest: func(request *GameOperationRequest) {
				request.Values = append(request.Values, GameOperationValue{FieldID: "endpoint", StringValue: new("/unsafe")})
			},
			wantClassification: GameOperationResultFailed,
			wantMessage:        "Unknown operation field",
		},
		{
			name: "malformed Player identity is rejected before transport",
			modifyRequest: func(request *GameOperationRequest) {
				request.Values[0].StringValue = new("../../admin")
			},
			wantClassification: GameOperationResultFailed,
			wantMessage:        "Player identity",
		},
		{
			name: "out-of-range permission is rejected before transport",
			modifyRequest: func(request *GameOperationRequest) {
				request.Values[1].IntegerValue = new(int64(1001))
			},
			wantClassification: GameOperationResultFailed,
			wantMessage:        "between 0 and 1000",
		},
		{
			name:               "native transport timeout fails with a useful reason",
			waitForPostTimeout: true,
			wantClassification: GameOperationResultFailed,
			wantMessage:        "timed out",
			wantPost:           true,
		},
		{
			name:                  "set game time uses the captured bounded command",
			operationID:           "server_control.set_game_time",
			values:                []GameOperationValue{{FieldID: "time", StringValue: new("night")}},
			commandStatus:         http.StatusOK,
			wantCommand:           "settime night",
			worldTimeStatus:       http.StatusOK,
			worldTimeBody:         readGameOperationFixture(t, "results", "set-game-time-readback.json"),
			wantClassification:    GameOperationResultConfirmed,
			wantMessage:           "confirmed",
			wantPost:              true,
			wantWorldTimeReadBack: true,
		},
		{
			name:                  "set game time accepts an exact day and clock value",
			operationID:           "server_control.set_game_time",
			values:                []GameOperationValue{{FieldID: "time", StringValue: new("42 7 5")}},
			commandStatus:         http.StatusOK,
			wantCommand:           "settime 42 7 5",
			worldTimeStatus:       http.StatusOK,
			worldTimeBody:         `{"data":{"gameTime":{"days":42,"hours":7,"minutes":5}}}`,
			wantClassification:    GameOperationResultConfirmed,
			wantMessage:           "confirmed",
			wantPost:              true,
			wantWorldTimeReadBack: true,
		},
		{
			name:                  "set game time stays unverified when world time read-back is malformed",
			operationID:           "server_control.set_game_time",
			values:                []GameOperationValue{{FieldID: "time", StringValue: new("night")}},
			commandStatus:         http.StatusOK,
			wantCommand:           "settime night",
			worldTimeStatus:       http.StatusOK,
			worldTimeBody:         `{"data":{"gameTime":{"days":2,"hours":0}}}`,
			wantClassification:    GameOperationResultAcceptedButUnverified,
			wantMessage:           "cannot be verified",
			wantPost:              true,
			wantWorldTimeReadBack: true,
		},
		{
			name:                   "set game time stays unverified when world time read-back times out",
			operationID:            "server_control.set_game_time",
			values:                 []GameOperationValue{{FieldID: "time", StringValue: new("night")}},
			commandStatus:          http.StatusOK,
			wantCommand:            "settime night",
			waitForReadBackTimeout: true,
			wantClassification:     GameOperationResultAcceptedButUnverified,
			wantMessage:            "cannot be verified",
			wantPost:               true,
			wantWorldTimeReadBack:  true,
		},
		{
			name:                  "set game time stays unverified when world time read-back is unavailable",
			operationID:           "server_control.set_game_time",
			values:                []GameOperationValue{{FieldID: "time", StringValue: new("day")}},
			commandStatus:         http.StatusOK,
			wantCommand:           "settime day",
			worldTimeStatus:       http.StatusServiceUnavailable,
			wantClassification:    GameOperationResultAcceptedButUnverified,
			wantMessage:           "cannot be verified",
			wantPost:              true,
			wantWorldTimeReadBack: true,
		},
		{
			name:                  "set game time fails when authoritative read-back does not match",
			operationID:           "server_control.set_game_time",
			values:                []GameOperationValue{{FieldID: "time", StringValue: new("day")}},
			commandStatus:         http.StatusOK,
			wantCommand:           "settime day",
			worldTimeStatus:       http.StatusOK,
			worldTimeBody:         `{"data":{"gameTime":{"days":8,"hours":13,"minutes":37}}}`,
			wantClassification:    GameOperationResultFailed,
			wantMessage:           "did not report",
			wantPost:              true,
			wantWorldTimeReadBack: true,
		},
		{
			name:               "shutdown uses the captured command without values",
			operationID:        "server_control.shutdown",
			commandStatus:      http.StatusOK,
			wantCommand:        "shutdown",
			wantClassification: GameOperationResultAcceptedButUnverified,
			wantMessage:        "accepted",
			wantPost:           true,
		},
		{
			name:               "set game time reports native command timeout",
			operationID:        "server_control.set_game_time",
			values:             []GameOperationValue{{FieldID: "time", StringValue: new("day")}},
			waitForPostTimeout: true,
			wantCommand:        "settime day",
			wantClassification: GameOperationResultFailed,
			wantMessage:        "timed out",
			wantPost:           true,
		},
		{
			name:               "shutdown reports native command rejection",
			operationID:        "server_control.shutdown",
			commandStatus:      http.StatusConflict,
			wantCommand:        "shutdown",
			wantClassification: GameOperationResultFailed,
			wantMessage:        "rejected",
			wantPost:           true,
		},
		{
			name:               "shutdown reports native command timeout",
			operationID:        "server_control.shutdown",
			waitForPostTimeout: true,
			wantCommand:        "shutdown",
			wantClassification: GameOperationResultFailed,
			wantMessage:        "timed out",
			wantPost:           true,
		},
		{
			name:               "save world uses the captured command without values",
			operationID:        "server_control.save_world",
			commandStatus:      http.StatusOK,
			wantCommand:        "saveworld",
			wantClassification: GameOperationResultAcceptedButUnverified,
			wantMessage:        "accepted",
			wantPost:           true,
		},
		{
			name:               "save world rejects unknown fields before transport",
			operationID:        "server_control.save_world",
			values:             []GameOperationValue{{FieldID: "slot", StringValue: new("main")}},
			wantClassification: GameOperationResultFailed,
			wantMessage:        "does not accept fields",
		},
		{
			name:               "save world reports native command rejection",
			operationID:        "server_control.save_world",
			commandStatus:      http.StatusServiceUnavailable,
			wantCommand:        "saveworld",
			wantClassification: GameOperationResultFailed,
			wantMessage:        "rejected",
			wantPost:           true,
		},
		{
			name:               "save world reports native command timeout",
			operationID:        "server_control.save_world",
			waitForPostTimeout: true,
			wantCommand:        "saveworld",
			wantClassification: GameOperationResultFailed,
			wantMessage:        "timed out",
			wantPost:           true,
		},
		{
			name:               "set temperature unit uses the captured bounded command",
			operationID:        "server_control.set_temperature_unit",
			values:             []GameOperationValue{{FieldID: "unit", StringValue: new("C")}},
			commandStatus:      http.StatusOK,
			wantCommand:        "settempunit C",
			wantClassification: GameOperationResultAcceptedButUnverified,
			wantMessage:        "accepted",
			wantPost:           true,
		},
		{
			name:               "set temperature unit rejects values outside the captured options",
			operationID:        "server_control.set_temperature_unit",
			values:             []GameOperationValue{{FieldID: "unit", StringValue: new("Kelvin")}},
			wantClassification: GameOperationResultFailed,
			wantMessage:        "F or C",
		},
		{
			name:               "set temperature unit reports native command rejection",
			operationID:        "server_control.set_temperature_unit",
			values:             []GameOperationValue{{FieldID: "unit", StringValue: new("F")}},
			commandStatus:      http.StatusBadRequest,
			wantCommand:        "settempunit F",
			wantClassification: GameOperationResultFailed,
			wantMessage:        "rejected",
			wantPost:           true,
		},
		{
			name:               "set temperature unit reports native command timeout",
			operationID:        "server_control.set_temperature_unit",
			values:             []GameOperationValue{{FieldID: "unit", StringValue: new("F")}},
			waitForPostTimeout: true,
			wantCommand:        "settempunit F",
			wantClassification: GameOperationResultFailed,
			wantMessage:        "timed out",
			wantPost:           true,
		},
		{
			name:               "set game time rejects invalid exact values",
			operationID:        "server_control.set_game_time",
			values:             []GameOperationValue{{FieldID: "time", StringValue: new("42 24 0")}},
			wantClassification: GameOperationResultFailed,
			wantMessage:        "hour from 0 to 23",
		},
		{
			name:               "shutdown rejects unknown fields before transport",
			operationID:        "server_control.shutdown",
			values:             []GameOperationValue{{FieldID: "delay", IntegerValue: new(int64(30))}},
			wantClassification: GameOperationResultFailed,
			wantMessage:        "does not accept fields",
		},
		{
			name:               "native command rejection is a failed result",
			operationID:        "server_control.set_game_time",
			values:             []GameOperationValue{{FieldID: "time", StringValue: new("day")}},
			commandStatus:      http.StatusForbidden,
			wantCommand:        "settime day",
			wantClassification: GameOperationResultFailed,
			wantMessage:        "rejected",
			wantPost:           true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var postRequests atomic.Int32
			var serverStatsRequests atomic.Int32
			var observedCommand atomic.Value
			timeoutRequestStarted := make(chan struct{})
			fragments := fullSevenDaysToDieOpenAPIFragments()
			workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
				switch {
				case request.URL.Path == sevenDaysToDieWebAPIEndpointOpenAPI:
					writeSevenDaysToDieTestResponse(t, response, fullSevenDaysToDieOpenAPI())
				case strings.HasPrefix(request.URL.Path, "/api/OpenAPI/"):
					fragment, found := fragments[request.URL.Path]
					if !found {
						http.NotFound(response, request)
						return
					}
					writeSevenDaysToDieTestResponse(t, response, fragment)
				case request.Method == http.MethodPost && request.URL.Path == "/api/userpermissions/user/EOS_PLAYER_1":
					postRequests.Add(1)
					if request.Header.Get("X-SDTD-API-TOKENNAME") != "operator" || request.Header.Get("X-SDTD-API-SECRET") != "fixture-secret" {
						t.Error("native request omitted node-owned credentials")
					}
					var body struct {
						PermissionLevel int64 `json:"permissionLevel"`
					}
					errDecode := json.NewDecoder(io.LimitReader(request.Body, 1024)).Decode(&body)
					if errDecode != nil || body.PermissionLevel != 0 {
						t.Errorf("native request body = %+v, error = %v", body, errDecode)
					}
					if test.waitForPostTimeout {
						close(timeoutRequestStarted)
						<-request.Context().Done()
						return
					}
					response.WriteHeader(test.postStatus)
				case request.Method == http.MethodPost && request.URL.Path == "/api/command":
					postRequests.Add(1)
					if request.Header.Get("X-SDTD-API-TOKENNAME") != "operator" || request.Header.Get("X-SDTD-API-SECRET") != "fixture-secret" {
						t.Error("native command request omitted node-owned credentials")
					}
					var body struct {
						Command string `json:"command"`
						Format  string `json:"format"`
					}
					errDecode := json.NewDecoder(io.LimitReader(request.Body, 1024)).Decode(&body)
					if errDecode != nil || body.Format != "Simple" {
						t.Errorf("native command body = %+v, error = %v", body, errDecode)
					}
					observedCommand.Store(body.Command)
					if test.waitForPostTimeout {
						close(timeoutRequestStarted)
						<-request.Context().Done()
						return
					}
					response.WriteHeader(test.commandStatus)
				case request.Method == http.MethodGet && request.URL.Path == "/api/userpermissions":
					response.WriteHeader(test.readBackStatus)
					writeSevenDaysToDieTestResponse(t, response, test.readBackBody)
				case request.Method == http.MethodGet && request.URL.Path == sevenDaysToDieWebAPIEndpointServerStats:
					serverStatsRequests.Add(1)
					if test.waitForReadBackTimeout {
						close(timeoutRequestStarted)
						<-request.Context().Done()
						return
					}
					response.WriteHeader(test.worldTimeStatus)
					writeSevenDaysToDieTestResponse(t, response, test.worldTimeBody)
				default:
					http.NotFound(response, request)
				}
			}, "")

			operationRequest := validAddAdministratorRequest(workingDirectory)
			if test.operationID != "" {
				operationRequest.OperationID = test.operationID
				operationRequest.Values = test.values
			}
			if test.modifyRequest != nil {
				test.modifyRequest(&operationRequest)
			}

			var result GameOperationResult
			if test.waitForPostTimeout || test.waitForReadBackTimeout {
				timeoutContext := newGameOperationDeadlineContext(t.Context())
				resultReady := make(chan GameOperationResult, 1)
				go func() {
					resultReady <- new(Node).ExecuteGameOperation(timeoutContext, operationRequest)
				}()
				select {
				case result = <-resultReady:
					t.Fatalf("ExecuteGameOperation() returned before the timed native request started: %+v", result)
				case <-timeoutRequestStarted:
				}
				timeoutContext.expire()
				result = <-resultReady
			} else {
				result = new(Node).ExecuteGameOperation(t.Context(), operationRequest)
			}

			if result.Classification != test.wantClassification || !strings.Contains(result.Message, test.wantMessage) {
				t.Fatalf("ExecuteGameOperation() = %+v, want classification %v and message containing %q", result, test.wantClassification, test.wantMessage)
			}
			if test.wantPost != (postRequests.Load() == 1) {
				t.Fatalf("native POST requests = %d, wantPost = %v", postRequests.Load(), test.wantPost)
			}
			if test.wantWorldTimeReadBack != (serverStatsRequests.Load() == 1) {
				t.Fatalf("ServerStats requests = %d, wantWorldTimeReadBack = %v", serverStatsRequests.Load(), test.wantWorldTimeReadBack)
			}
			gotCommand, _ := observedCommand.Load().(string)
			if gotCommand != test.wantCommand {
				t.Fatalf("native command = %q, want %q", gotCommand, test.wantCommand)
			}
			if len(result.Message) > 512 || strings.Contains(result.Message, "fixture-secret") || strings.Contains(result.Message, "/api/") {
				t.Fatalf("result was not bounded and redacted: %+v", result)
			}
			if test.wantPost && (result.TransportDetails.Method == "" || strings.Contains(result.TransportDetails.Method, "/api/")) {
				t.Fatalf("transport details = %+v", result.TransportDetails)
			}
		})
	}

	commandTests := []struct {
		name               string
		request            GameOperationRequest
		wantCommand        string
		wantNativeName     string
		wantParameters     string
		invalidValues      []GameOperationValue
		wantClassification GameOperationResultClassification
	}{
		{
			name: "broadcasts an announcement",
			request: commandOperationRequest("communication.broadcast_message",
				GameOperationValue{FieldID: "message", StringValue: new("Server restart in 5 minutes")}),
			wantCommand:        `say "Server restart in 5 minutes"`,
			wantNativeName:     "say",
			wantParameters:     `"Server restart in 5 minutes"`,
			invalidValues:      []GameOperationValue{{FieldID: "message", StringValue: new("first\nversion")}},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name: "sends a private Player message",
			request: commandOperationRequest("communication.message_player",
				GameOperationValue{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				GameOperationValue{FieldID: "message", StringValue: new("Meet at the trader")}),
			wantCommand:    `sayplayer "EOS_PLAYER_1" "Meet at the trader"`,
			wantNativeName: "sayplayer",
			wantParameters: `"EOS_PLAYER_1" "Meet at the trader"`,
			invalidValues: []GameOperationValue{
				{FieldID: "player", StringValue: new("../admin")},
				{FieldID: "message", StringValue: new("Meet at the trader")},
			},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name: "teleports one Player to another",
			request: commandOperationRequest("player_assistance.teleport_to_player",
				GameOperationValue{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				GameOperationValue{FieldID: "destination", StringValue: new("Steam_PLAYER_2")}),
			wantCommand:    `teleportplayer "EOS_PLAYER_1" "Steam_PLAYER_2"`,
			wantNativeName: "teleportplayer",
			wantParameters: `"EOS_PLAYER_1" "Steam_PLAYER_2"`,
			invalidValues: []GameOperationValue{
				{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				{FieldID: "destination", StringValue: new("../unsafe")},
			},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name: "gives an item stack to a Player",
			request: commandOperationRequest("player_assistance.give_item",
				GameOperationValue{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				GameOperationValue{FieldID: "item", StringValue: new("resourceWood")},
				GameOperationValue{FieldID: "amount", IntegerValue: new(int64(50))}),
			wantCommand:    `give "1" "resourceWood" 50`,
			wantNativeName: "give",
			wantParameters: `"1" "resourceWood" 50`,
			invalidValues: []GameOperationValue{
				{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				{FieldID: "item", StringValue: new("resourceWood")},
				{FieldID: "amount", IntegerValue: new(int64(0))},
			},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name: "gives experience to a Player",
			request: commandOperationRequest("player_assistance.give_experience",
				GameOperationValue{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				GameOperationValue{FieldID: "experience", IntegerValue: new(int64(2500))}),
			wantCommand:    `givexp "EOS_PLAYER_1" 2500`,
			wantNativeName: "givexp",
			wantParameters: `"EOS_PLAYER_1" 2500`,
			invalidValues: []GameOperationValue{
				{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				{FieldID: "experience", IntegerValue: new(int64(1000001))},
			},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name: "applies a buff to a Player",
			request: commandOperationRequest("player_assistance.apply_buff",
				GameOperationValue{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				GameOperationValue{FieldID: "buff", StringValue: new("buffDrugPainkillers")}),
			wantCommand:    `buffplayer "EOS_PLAYER_1" "buffDrugPainkillers"`,
			wantNativeName: "buffplayer",
			wantParameters: `"EOS_PLAYER_1" "buffDrugPainkillers"`,
			invalidValues: []GameOperationValue{
				{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				{FieldID: "buff", StringValue: new("unsafe\nbuff")},
			},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name: "removes a buff from a Player",
			request: commandOperationRequest("player_assistance.remove_buff",
				GameOperationValue{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
				GameOperationValue{FieldID: "buff", StringValue: new("buffDrugPainkillers")}),
			wantCommand:    `debuffplayer "EOS_PLAYER_1" "buffDrugPainkillers"`,
			wantNativeName: "debuffplayer",
			wantParameters: `"EOS_PLAYER_1" "buffDrugPainkillers"`,
			invalidValues: []GameOperationValue{
				{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
			},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name:               "spawns an airdrop",
			request:            commandOperationRequest("world_events.spawn_airdrop"),
			wantCommand:        "spawnairdrop",
			wantNativeName:     "spawnairdrop",
			invalidValues:      []GameOperationValue{{FieldID: "count", IntegerValue: new(int64(2))}},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name:               "spawns a wandering horde",
			request:            commandOperationRequest("world_events.spawn_wandering_horde"),
			wantCommand:        "spawnwandering h",
			wantNativeName:     "spawnwandering",
			wantParameters:     "h",
			invalidValues:      []GameOperationValue{{FieldID: "kind", StringValue: new("bandits")}},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name: "sets a bounded weather preset",
			request: commandOperationRequest("world_events.set_weather",
				GameOperationValue{FieldID: "weather", StringValue: new("rain")}),
			wantCommand:        "weather Rain 1",
			wantNativeName:     "weather",
			wantParameters:     "Rain 1",
			invalidValues:      []GameOperationValue{{FieldID: "weather", StringValue: new("storm")}},
			wantClassification: GameOperationResultAcceptedButUnverified,
		},
		{
			name: "filters game preferences",
			request: commandOperationRequest("server_information.game_preferences",
				GameOperationValue{FieldID: "filter", StringValue: new("LandClaim")}),
			wantCommand:        `getgamepref "LandClaim"`,
			wantNativeName:     "getgamepref",
			wantParameters:     `"LandClaim"`,
			invalidValues:      []GameOperationValue{{FieldID: "filter", StringValue: new("Land\nClaim")}},
			wantClassification: GameOperationResultConfirmed,
		},
		{
			name:               "reads all game statistics",
			request:            commandOperationRequest("server_information.game_statistics"),
			wantCommand:        "getgamestat",
			wantNativeName:     "getgamestat",
			invalidValues:      []GameOperationValue{{FieldID: "filter", IntegerValue: new(int64(1))}},
			wantClassification: GameOperationResultConfirmed,
		},
		{
			name:               "reads game time",
			request:            commandOperationRequest("server_information.game_time"),
			wantCommand:        "gettime",
			wantNativeName:     "gettime",
			invalidValues:      []GameOperationValue{{FieldID: "timezone", StringValue: new("UTC")}},
			wantClassification: GameOperationResultConfirmed,
		},
		{
			name:               "reads DLC status",
			request:            commandOperationRequest("server_information.dlc_status"),
			wantCommand:        "listdlc",
			wantNativeName:     "listdlc",
			invalidValues:      []GameOperationValue{{FieldID: "refresh", BooleanValue: new(true)}},
			wantClassification: GameOperationResultConfirmed,
		},
		{
			name: "searches item definitions",
			request: commandOperationRequest("server_information.item_search",
				GameOperationValue{FieldID: "search", StringValue: new("medical")}),
			wantCommand:        `listitems "medical"`,
			wantNativeName:     "listitems",
			wantParameters:     `"medical"`,
			invalidValues:      []GameOperationValue{{FieldID: "search", StringValue: new("")}},
			wantClassification: GameOperationResultConfirmed,
		},
		{
			name:               "reads version and loaded mods",
			request:            commandOperationRequest("server_information.version"),
			wantCommand:        "version",
			wantNativeName:     "version",
			invalidValues:      []GameOperationValue{{FieldID: "format", StringValue: new("raw")}},
			wantClassification: GameOperationResultConfirmed,
		},
	}

	for _, test := range commandTests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, postedCommand := executeCommandOperationFixtureWithResponseCommand(
				t,
				test.request,
				true,
				http.StatusOK,
				test.wantNativeName,
				test.wantNativeName,
				test.wantParameters,
				"fixture diagnostic output",
				true,
			)
			if postedCommand != test.wantCommand || result.Classification != test.wantClassification {
				t.Fatalf("ExecuteGameOperation() command = %q, result = %+v", postedCommand, result)
			}
			if test.wantClassification == GameOperationResultConfirmed && !strings.Contains(result.Message, "fixture diagnostic output") {
				t.Fatalf("confirmed diagnostic result omitted native output: %+v", result)
			}

			invalidRequest := test.request
			invalidRequest.Values = test.invalidValues
			invalidResult, invalidPostedCommand := executeCommandOperationFixture(
				t,
				invalidRequest,
				true,
				http.StatusOK,
				test.wantNativeName,
				test.wantParameters,
				"unused",
			)
			if invalidResult.Classification != GameOperationResultFailed || invalidPostedCommand != "" {
				t.Fatalf("invalid ExecuteGameOperation() command = %q, result = %+v", invalidPostedCommand, invalidResult)
			}

			unsupportedResult, unsupportedPostedCommand := executeCommandOperationFixture(
				t,
				test.request,
				false,
				http.StatusOK,
				test.wantNativeName,
				test.wantParameters,
				"unused",
			)
			if unsupportedResult.Classification != GameOperationResultFailed || unsupportedPostedCommand != "" ||
				!strings.Contains(unsupportedResult.Message, "command") {
				t.Fatalf("unsupported ExecuteGameOperation() command = %q, result = %+v", unsupportedPostedCommand, unsupportedResult)
			}

			rejectedResult, rejectedCommand := executeCommandOperationFixture(
				t,
				test.request,
				true,
				http.StatusForbidden,
				test.wantNativeName,
				test.wantParameters,
				"fixture-secret should stay private",
			)
			if rejectedResult.Classification != GameOperationResultFailed || rejectedCommand != test.wantCommand ||
				strings.Contains(rejectedResult.Message, "fixture-secret") {
				t.Fatalf("rejected ExecuteGameOperation() command = %q, result = %+v", rejectedCommand, rejectedResult)
			}

			mismatchedClassification := GameOperationResultFailed
			if test.wantClassification == GameOperationResultAcceptedButUnverified {
				mismatchedClassification = GameOperationResultAcceptedButUnverified
			}
			mismatchedResult, mismatchedCommand := executeCommandOperationFixtureWithResponseCommand(
				t,
				test.request,
				true,
				http.StatusOK,
				test.wantNativeName,
				"mismatched",
				test.wantParameters,
				"untrusted output",
				true,
			)
			if mismatchedResult.Classification != mismatchedClassification || mismatchedCommand != test.wantCommand {
				t.Fatalf("mismatched ExecuteGameOperation() command = %q, result = %+v", mismatchedCommand, mismatchedResult)
			}
		})
	}

	t.Run("bounds and redacts diagnostic output", func(t *testing.T) {
		t.Parallel()

		result, _ := executeCommandOperationFixture(
			t,
			commandOperationRequest("server_information.version"),
			true,
			http.StatusOK,
			"version",
			"",
			strings.Repeat("output ", 200)+"fixture-secret",
		)
		if result.Classification != GameOperationResultConfirmed || len(result.Message) > 512 ||
			strings.Contains(result.Message, "fixture-secret") {
			t.Fatalf("diagnostic result was not bounded and redacted: %+v", result)
		}
	})

	t.Run("unsupported discovery reports an operation-neutral failure", func(t *testing.T) {
		t.Parallel()

		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			http.NotFound(response, request)
		}, "")
		request := commandOperationRequest("server_information.version")
		request.WorkingDirectory = workingDirectory

		result := new(Node).ExecuteGameOperation(t.Context(), request)
		if result.Classification != GameOperationResultFailed ||
			!strings.Contains(result.Message, "required game operation support") ||
			strings.Contains(result.Message, "Add administrator") {
			t.Fatalf("unsupported discovery result = %+v", result)
		}
	})

	t.Run("configured token without command permission fails before execution", func(t *testing.T) {
		t.Parallel()

		result, postedCommand := executeCommandOperationFixtureWithAllowed(
			t,
			commandOperationRequest("communication.broadcast_message",
				GameOperationValue{FieldID: "message", StringValue: new("Server restart soon")}),
			true,
			http.StatusOK,
			"say",
			`"Server restart soon"`,
			"unused",
			false,
		)
		if result.Classification != GameOperationResultFailed || postedCommand != "" ||
			!strings.Contains(result.Message, "not allowed") {
			t.Fatalf("permission-denied command result = %+v, posted command = %q", result, postedCommand)
		}
	})

	t.Run("missing native command fails as unsupported before execution", func(t *testing.T) {
		t.Parallel()

		result, postedCommand := executeCommandOperationFixtureWithResponseCommand(
			t,
			commandOperationRequest("communication.broadcast_message",
				GameOperationValue{FieldID: "message", StringValue: new("Server restart soon")}),
			true,
			http.StatusOK,
			"version",
			"version",
			"",
			"unused",
			true,
		)
		if result.Classification != GameOperationResultFailed || postedCommand != "" ||
			!strings.Contains(result.Message, "does not expose") {
			t.Fatalf("unsupported command result = %+v, posted command = %q", result, postedCommand)
		}
	})
}

type gameOperationDeadlineContext struct {
	context.Context
	done chan struct{}
}

func newGameOperationDeadlineContext(parent context.Context) *gameOperationDeadlineContext {
	return &gameOperationDeadlineContext{
		Context: parent,
		done:    make(chan struct{}),
	}
}

func (ctx *gameOperationDeadlineContext) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *gameOperationDeadlineContext) Err() error {
	select {
	case <-ctx.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func (ctx *gameOperationDeadlineContext) expire() {
	close(ctx.done)
}

func validAddAdministratorRequest(workingDirectory string) GameOperationRequest {
	return GameOperationRequest{
		WorkingDirectory: workingDirectory,
		TokenName:        "operator",
		TokenSecret:      "fixture-secret",
		OperationID:      "player_access.add_administrator",
		Values: []GameOperationValue{
			{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
			{FieldID: "permission_level", IntegerValue: new(int64(0))},
		},
	}
}

func commandOperationRequest(operationID string, values ...GameOperationValue) GameOperationRequest {
	return GameOperationRequest{
		TokenName:   "operator",
		TokenSecret: "fixture-secret",
		OperationID: operationID,
		Values:      values,
	}
}

func executeCommandOperationFixture(
	t *testing.T,
	request GameOperationRequest,
	commandCapability bool,
	statusCode int,
	nativeName string,
	parameters string,
	output string,
) (GameOperationResult, string) {
	return executeCommandOperationFixtureWithResponseCommand(
		t,
		request,
		commandCapability,
		statusCode,
		nativeName,
		nativeName,
		parameters,
		output,
		true,
	)
}

func executeCommandOperationFixtureWithAllowed(
	t *testing.T,
	request GameOperationRequest,
	commandCapability bool,
	statusCode int,
	nativeName string,
	parameters string,
	output string,
	commandAllowed bool,
) (GameOperationResult, string) {
	return executeCommandOperationFixtureWithResponseCommand(
		t,
		request,
		commandCapability,
		statusCode,
		nativeName,
		nativeName,
		parameters,
		output,
		commandAllowed,
	)
}

func executeCommandOperationFixtureWithResponseCommand(
	t *testing.T,
	request GameOperationRequest,
	commandCapability bool,
	statusCode int,
	catalogCommand string,
	responseCommand string,
	parameters string,
	output string,
	commandAllowed bool,
) (GameOperationResult, string) {
	t.Helper()
	postedCommand := ""
	fragments := fullSevenDaysToDieOpenAPIFragments()
	openAPI := fullSevenDaysToDieOpenAPI()
	if !commandCapability {
		openAPI = "openapi: 3.1.0\ninfo:\n  version: '1.0.0'\npaths: {}\n"
	}
	workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, nativeRequest *http.Request) {
		switch {
		case nativeRequest.URL.Path == sevenDaysToDieWebAPIEndpointOpenAPI:
			writeSevenDaysToDieTestResponse(t, response, openAPI)
		case strings.HasPrefix(nativeRequest.URL.Path, "/api/OpenAPI/"):
			fragment, found := fragments[nativeRequest.URL.Path]
			if !found {
				http.NotFound(response, nativeRequest)
				return
			}
			writeSevenDaysToDieTestResponse(t, response, fragment)
		case nativeRequest.Method == http.MethodGet && nativeRequest.URL.Path == "/api/command":
			encoded, errEncode := json.Marshal(map[string]any{
				"data": map[string]any{
					"commands": []map[string]any{{"command": catalogCommand, "allowed": commandAllowed}},
				},
			})
			if errEncode != nil {
				t.Errorf("encode native command catalog: %v", errEncode)
				return
			}
			writeSevenDaysToDieTestResponse(t, response, string(encoded))
		case nativeRequest.Method == http.MethodGet && nativeRequest.URL.Path == sevenDaysToDieWebAPIEndpointPlayer:
			writeSevenDaysToDieTestResponse(
				t,
				response,
				readGameOperationFixture(t, "players", "representative.json"),
			)
		case nativeRequest.Method == http.MethodPost && nativeRequest.URL.Path == "/api/command":
			var body struct {
				Command string `json:"command"`
				Format  string `json:"format"`
			}
			errDecode := json.NewDecoder(io.LimitReader(nativeRequest.Body, 4096)).Decode(&body)
			if errDecode != nil || body.Format != "Full" {
				t.Errorf("native command request = %+v, error = %v", body, errDecode)
			}
			postedCommand = body.Command
			response.WriteHeader(statusCode)
			if statusCode == http.StatusOK {
				encoded, errEncode := json.Marshal(map[string]any{
					"data": map[string]string{
						"command":    responseCommand,
						"parameters": parameters,
						"result":     output,
					},
				})
				if errEncode != nil {
					t.Errorf("encode native command response: %v", errEncode)
					return
				}
				writeSevenDaysToDieTestResponse(t, response, string(encoded))
			}
		default:
			http.NotFound(response, nativeRequest)
		}
	}, "")
	request.WorkingDirectory = workingDirectory
	return new(Node).ExecuteGameOperation(t.Context(), request), postedCommand
}

func readGameOperationFixture(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"testdata", "seven-days-to-die", "v2.6-build-22422094"}, parts...)...)
	data, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("read fixture %s: %v", path, errRead)
	}
	return string(data)
}
