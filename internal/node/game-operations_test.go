package node

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNodeExecuteGameOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		modifyRequest      func(*GameOperationRequest)
		postStatus         int
		readBackStatus     int
		readBackBody       string
		waitForPostTimeout bool
		wantClassification GameOperationResultClassification
		wantMessage        string
		wantPost           bool
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			postRequests := 0
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
					postRequests++
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
						<-request.Context().Done()
						return
					}
					response.WriteHeader(test.postStatus)
				case request.Method == http.MethodGet && request.URL.Path == "/api/userpermissions":
					response.WriteHeader(test.readBackStatus)
					writeSevenDaysToDieTestResponse(t, response, test.readBackBody)
				default:
					http.NotFound(response, request)
				}
			}, "")

			operationRequest := validAddAdministratorRequest(workingDirectory)
			if test.modifyRequest != nil {
				test.modifyRequest(&operationRequest)
			}

			ctx := t.Context()
			if test.waitForPostTimeout {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			result := new(Node).ExecuteGameOperation(ctx, operationRequest)

			if result.Classification != test.wantClassification || !strings.Contains(result.Message, test.wantMessage) {
				t.Fatalf("ExecuteGameOperation() = %+v, want classification %v and message containing %q", result, test.wantClassification, test.wantMessage)
			}
			if test.wantPost != (postRequests == 1) {
				t.Fatalf("native POST requests = %d, wantPost = %v", postRequests, test.wantPost)
			}
			if len(result.Message) > 512 || strings.Contains(result.Message, "fixture-secret") || strings.Contains(result.Message, "/api/") {
				t.Fatalf("result was not bounded and redacted: %+v", result)
			}
			if test.wantPost && (result.TransportDetails.Method == "" || strings.Contains(result.TransportDetails.Method, "/api/")) {
				t.Fatalf("transport details = %+v", result.TransportDetails)
			}
		})
	}
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

func readGameOperationFixture(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"testdata", "seven-days-to-die", "v2.6-build-22422094"}, parts...)...)
	data, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("read fixture %s: %v", path, errRead)
	}
	return string(data)
}
