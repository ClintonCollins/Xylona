package restinput

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestExecuteSatisfactory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		command      string
		wantFunction string
		wantCommand  string
		wantResponse string
		errorCode    string
		errorMessage string
		wantErr      string
	}{
		{
			name:         "runs console command",
			command:      "server.SaveGame",
			wantFunction: "RunCommand",
			wantCommand:  "server.SaveGame",
			wantResponse: "saved",
		},
		{
			name:         "uses shutdown API for quit",
			command:      "quit",
			wantFunction: "Shutdown",
		},
		{
			name:         "reports API error envelope",
			command:      "unknown.command",
			wantFunction: "RunCommand",
			wantCommand:  "unknown.command",
			errorCode:    "invalid_request",
			errorMessage: "command is not available",
			wantErr:      "invalid_request",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != "/api/v1" {
					t.Errorf("request = %s %s, want POST /api/v1", request.Method, request.URL.Path)
				}
				var payload struct {
					Function string         `json:"function"`
					Data     map[string]any `json:"data"`
				}
				errDecode := json.NewDecoder(request.Body).Decode(&payload)
				if errDecode != nil {
					t.Errorf("decode request: %v", errDecode)
				}
				if payload.Function == "PasswordLogin" {
					if payload.Data["Password"] != "custom-admin-password" ||
						payload.Data["MinimumPrivilegeLevel"] != "Administrator" {
						t.Errorf("PasswordLogin data = %+v", payload.Data)
					}
					encodeTestJSON(t, writer, map[string]any{
						"data": map[string]any{"authenticationToken": "admin-token"},
					})
					return
				}
				if request.Header.Get("Authorization") != "Bearer admin-token" {
					t.Errorf("Authorization = %q, want Bearer admin-token", request.Header.Get("Authorization"))
				}
				command, _ := payload.Data["Command"].(string)
				if payload.Function != tc.wantFunction || command != tc.wantCommand {
					t.Errorf("payload = %+v", payload)
				}
				if tc.wantFunction == "Shutdown" {
					writer.WriteHeader(http.StatusNoContent)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				if tc.errorCode != "" {
					errEncode := json.NewEncoder(writer).Encode(map[string]any{
						"errorCode":    tc.errorCode,
						"errorMessage": tc.errorMessage,
					})
					if errEncode != nil {
						t.Errorf("write error response: %v", errEncode)
					}
					return
				}
				errEncode := json.NewEncoder(writer).Encode(map[string]any{
					"data": map[string]any{"CommandResult": tc.wantResponse},
				})
				if errEncode != nil {
					t.Errorf("write response: %v", errEncode)
				}
			}))
			t.Cleanup(server.Close)

			endpoint, errParse := url.Parse(server.URL)
			if errParse != nil {
				t.Fatalf("url.Parse() error = %v", errParse)
			}
			host, portValue, errSplit := net.SplitHostPort(endpoint.Host)
			if errSplit != nil {
				t.Fatalf("net.SplitHostPort() error = %v", errSplit)
			}
			port, errPort := strconv.Atoi(portValue)
			if errPort != nil {
				t.Fatalf("strconv.Atoi() error = %v", errPort)
			}

			response, errExecute := ExecuteSatisfactory(
				t.Context(),
				host,
				port,
				"custom-admin-password",
				tc.command,
			)
			if tc.wantErr != "" {
				if errExecute == nil || !strings.Contains(errExecute.Error(), tc.wantErr) {
					t.Fatalf("ExecuteSatisfactory() error = %v, want containing %q", errExecute, tc.wantErr)
				}
				return
			}
			if errExecute != nil {
				t.Fatalf("ExecuteSatisfactory() error = %v", errExecute)
			}
			if response != tc.wantResponse {
				t.Fatalf("ExecuteSatisfactory() response = %q, want %q", response, tc.wantResponse)
			}
		})
	}
}

func TestConfigureSatisfactoryAdminPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		claimed           bool
		initialPassword   string
		previousPasswords []string
		wantFunctions     string
	}{
		{
			name:            "already configured password authenticates",
			claimed:         true,
			initialPassword: "custom-admin-password",
			wantFunctions:   "PasswordLogin",
		},
		{
			name:          "unclaimed server is claimed",
			wantFunctions: "PasswordLogin,PasswordlessLogin,ClaimServer",
		},
		{
			name:              "claimed server rotates from newest matching prior password",
			claimed:           true,
			initialPassword:   "previous-admin-password",
			previousPasswords: []string{"older-admin-password", "previous-admin-password"},
			wantFunctions:     "PasswordLogin,PasswordlessLogin,PasswordLogin,PasswordLogin,SetAdminPassword,PasswordLogin",
		},
		{
			name:            "claimed server rotates from password older than four offline changes",
			claimed:         true,
			initialPassword: "password-zero",
			previousPasswords: []string{
				"password-zero",
				"password-one",
				"password-two",
				"password-three",
				"password-four",
			},
			wantFunctions: "PasswordLogin,PasswordlessLogin,PasswordLogin,PasswordLogin,PasswordLogin," +
				"PasswordLogin,PasswordLogin,PasswordLogin,SetAdminPassword,PasswordLogin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			claimed := tc.claimed
			currentPassword := tc.initialPassword
			successfulLogins := 0
			var functions []string
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var payload struct {
					Function string         `json:"function"`
					Data     map[string]any `json:"data"`
				}
				errDecode := json.NewDecoder(request.Body).Decode(&payload)
				if errDecode != nil {
					t.Errorf("decode request: %v", errDecode)
				}
				functions = append(functions, payload.Function)

				writer.Header().Set("Content-Type", "application/json")
				switch payload.Function {
				case "PasswordLogin":
					password, _ := payload.Data["Password"].(string)
					if claimed && password == currentPassword {
						successfulLogins++
						encodeTestJSON(t, writer, map[string]any{
							"data": map[string]any{
								"authenticationToken": "admin-token-" + strconv.Itoa(successfulLogins),
							},
						})
						return
					}
					encodeTestJSON(t, writer, map[string]any{"errorCode": "wrong_password"})
				case "PasswordlessLogin":
					if claimed {
						encodeTestJSON(t, writer, map[string]any{
							"errorCode": "passwordless_login_not_possible",
						})
						return
					}
					encodeTestJSON(t, writer, map[string]any{
						"data": map[string]any{"authenticationToken": "initial-token"},
					})
				case "ClaimServer":
					if request.Header.Get("Authorization") != "Bearer initial-token" ||
						payload.Data["ServerName"] != "Factory One" ||
						payload.Data["AdminPassword"] != "custom-admin-password" {
						t.Errorf(
							"ClaimServer request header = %q, data = %+v",
							request.Header.Get("Authorization"),
							payload.Data,
						)
					}
					claimed = true
					currentPassword = "custom-admin-password"
					writer.WriteHeader(http.StatusNoContent)
				case "SetAdminPassword":
					if request.Header.Get("Authorization") != "Bearer admin-token-1" ||
						payload.Data["AuthenticationToken"] != "admin-token-2" ||
						payload.Data["Password"] != "custom-admin-password" {
						t.Errorf(
							"SetAdminPassword request header = %q, data = %+v",
							request.Header.Get("Authorization"),
							payload.Data,
						)
					}
					currentPassword = "custom-admin-password"
					writer.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected function %q", payload.Function)
				}
			}))
			t.Cleanup(server.Close)

			endpoint, errParse := url.Parse(server.URL)
			if errParse != nil {
				t.Fatalf("url.Parse() error = %v", errParse)
			}
			host, portValue, errSplit := net.SplitHostPort(endpoint.Host)
			if errSplit != nil {
				t.Fatalf("net.SplitHostPort() error = %v", errSplit)
			}
			port, errPort := strconv.Atoi(portValue)
			if errPort != nil {
				t.Fatalf("strconv.Atoi() error = %v", errPort)
			}

			errConfigure := ConfigureSatisfactoryAdminPassword(
				t.Context(),
				host,
				port,
				"Factory One",
				"custom-admin-password",
				tc.previousPasswords,
			)
			if errConfigure != nil {
				t.Fatalf("ConfigureSatisfactoryAdminPassword() error = %v", errConfigure)
			}
			if strings.Join(functions, ",") != tc.wantFunctions {
				t.Fatalf("functions = %v, want %q", functions, tc.wantFunctions)
			}
		})
	}
}

func encodeTestJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	errEncode := json.NewEncoder(writer).Encode(value)
	if errEncode != nil {
		t.Errorf("write response: %v", errEncode)
	}
}
