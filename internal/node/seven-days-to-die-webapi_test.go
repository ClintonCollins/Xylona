package node

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNodeQuerySevenDaysToDieWebAPIStatus(t *testing.T) {
	t.Run("reports a disabled dashboard without making a request", func(t *testing.T) {
		workingDirectory := t.TempDir()
		writeSevenDaysToDieWebAPIConfig(t, workingDirectory, "false", "", "https://example.com")

		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{
			WorkingDirectory: workingDirectory,
		})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
		}
		if status.ConnectionState != SevenDaysToDieWebAPIConnectionStateDashboardDisabled {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() state = %v, want %v", status.ConnectionState, SevenDaysToDieWebAPIConnectionStateDashboardDisabled)
		}
		if !status.ObservedAt.IsZero() {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() observed at = %v, want zero", status.ObservedAt)
		}
	})

	t.Run("reports malformed dashboard settings", func(t *testing.T) {
		tests := []struct {
			name    string
			enabled string
			port    string
		}{
			{name: "missing enabled setting", enabled: "", port: "8080"},
			{name: "invalid port", enabled: "true", port: "0"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				workingDirectory := t.TempDir()
				writeSevenDaysToDieWebAPIConfig(t, workingDirectory, test.enabled, test.port, "https://example.com")
				status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory})
				if errQuery != nil {
					t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
				}
				if status.ConnectionState != SevenDaysToDieWebAPIConnectionStateMisconfigured {
					t.Errorf("QuerySevenDaysToDieWebAPIStatus() state = %v, want %v", status.ConnectionState, SevenDaysToDieWebAPIConnectionStateMisconfigured)
				}
			})
		}
	})

	t.Run("discovers capabilities and Blood Moon state through fixed loopback GETs", func(t *testing.T) {
		const tokenName = "status-token"
		const tokenSecret = "status-secret"
		var paths []string
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			paths = append(paths, request.URL.Path)
			if request.Method != http.MethodGet {
				http.Error(response, "method", http.StatusMethodNotAllowed)
				return
			}
			if request.Header.Get("X-SDTD-API-TOKENNAME") != tokenName || request.Header.Get("X-SDTD-API-SECRET") != tokenSecret {
				http.Error(response, "credentials", http.StatusUnauthorized)
				return
			}
			switch request.URL.Path {
			case "/api/openapi/openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, fullSevenDaysToDieOpenAPI())
			case "/api/bloodmoon":
				writeSevenDaysToDieTestResponse(t, response, `{"data":{"gameTime":{"days":42,"hours":7,"minutes":5},"bloodmoonActive":true,"nextBloodmoon":{"days":49,"hours":22,"minutes":0},"nextBloodmoonEnd":{"days":50,"hours":4,"minutes":30}}}`)
			default:
				http.NotFound(response, request)
			}
		}, "https://example.com/should-never-be-used")

		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{
			WorkingDirectory: workingDirectory,
			TokenName:        tokenName,
			TokenSecret:      tokenSecret,
		})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
		}
		if status.ConnectionState != SevenDaysToDieWebAPIConnectionStateAvailable || status.APIVersion != "V2.2" {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() = %+v", status)
		}
		wantCapabilities := SevenDaysToDieWebAPICapabilities{
			PlayerData:                true,
			RuntimeSettings:           true,
			NativeLog:                 true,
			WorldPopulation:           true,
			HostileAndAnimalPositions: true,
			AccessControl:             true,
			GamePermissions:           true,
			ReportedMods:              true,
		}
		if status.Capabilities != wantCapabilities {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() capabilities = %+v, want %+v", status.Capabilities, wantCapabilities)
		}
		if status.WorldTimeState != SevenDaysToDieWebAPIValueStateAvailable || status.WorldTime == nil || *status.WorldTime != (SevenDaysToDieGameTime{Day: 42, Hour: 7, Minute: 5}) {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() world time = %v %+v", status.WorldTimeState, status.WorldTime)
		}
		if status.BloodMoonState != SevenDaysToDieWebAPIValueStateAvailable || status.BloodMoonActive == nil || !*status.BloodMoonActive {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() Blood Moon = %v %v", status.BloodMoonState, status.BloodMoonActive)
		}
		if status.NextBloodMoon == nil || *status.NextBloodMoon != (SevenDaysToDieGameTime{Day: 49, Hour: 22}) {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() next Blood Moon = %+v", status.NextBloodMoon)
		}
		if status.NextBloodMoonEnd == nil || *status.NextBloodMoonEnd != (SevenDaysToDieGameTime{Day: 50, Hour: 4, Minute: 30}) {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() next Blood Moon end = %+v", status.NextBloodMoonEnd)
		}
		if status.ObservedAt.IsZero() || time.Since(status.ObservedAt) > time.Minute {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() observed at = %v", status.ObservedAt)
		}
		if fmt.Sprintf("%+v", status) == "" || strings.Contains(fmt.Sprintf("%+v", status), tokenSecret) {
			t.Error("QuerySevenDaysToDieWebAPIStatus() exposed credentials")
		}
		if strings.Join(paths, ",") != "/api/openapi/openapi.yaml,/api/bloodmoon" {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() paths = %v", paths)
		}
	})

	t.Run("falls back to server statistics when Blood Moon access is denied", func(t *testing.T) {
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/openapi/openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, `openapi: 3.1.0
info:
  version: V2.2
paths:
  /api/bloodmoon:
    get: {}
  /api/serverstats:
    get: {}
`)
			case "/api/bloodmoon":
				http.Error(response, "secret upstream body", http.StatusForbidden)
			case "/api/serverstats":
				writeSevenDaysToDieTestResponse(t, response, `{"data":{"gameTime":{"days":8,"hours":13,"minutes":37}}}`)
			default:
				http.NotFound(response, request)
			}
		}, "")
		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
		}
		if status.ConnectionState != SevenDaysToDieWebAPIConnectionStateAvailable || status.BloodMoonState != SevenDaysToDieWebAPIValueStatePermissionDenied {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() states = %v %v", status.ConnectionState, status.BloodMoonState)
		}
		if status.WorldTimeState != SevenDaysToDieWebAPIValueStateAvailable || status.WorldTime == nil || status.WorldTime.Day != 8 {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() fallback world time = %v %+v", status.WorldTimeState, status.WorldTime)
		}
		if strings.Contains(fmt.Sprintf("%+v", status), "secret upstream body") {
			t.Error("QuerySevenDaysToDieWebAPIStatus() exposed an upstream body")
		}
	})

	t.Run("falls back to server statistics when Blood Moon is unsupported", func(t *testing.T) {
		var paths []string
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			paths = append(paths, request.URL.Path)
			switch request.URL.Path {
			case "/api/openapi/openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, `openapi: 3.0.3
info:
  version: V1.0
paths:
  /api/serverstats:
    get: {}
`)
			case "/api/serverstats":
				writeSevenDaysToDieTestResponse(t, response, `{"data":{"gameTime":{"days":3,"hours":1,"minutes":2}}}`)
			default:
				http.NotFound(response, request)
			}
		}, "")
		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
		}
		if status.BloodMoonState != SevenDaysToDieWebAPIValueStateUnsupported || status.WorldTimeState != SevenDaysToDieWebAPIValueStateAvailable {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() states = %v %v", status.BloodMoonState, status.WorldTimeState)
		}
		if strings.Join(paths, ",") != "/api/openapi/openapi.yaml,/api/serverstats" {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() paths = %v", paths)
		}
	})

	t.Run("requires every advertised operation in a capability group", func(t *testing.T) {
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/api/openapi/openapi.yaml" {
				t.Errorf("unexpected request path %q", request.URL.Path)
				http.NotFound(response, request)
				return
			}
			writeSevenDaysToDieTestResponse(t, response, `openapi: 3.1.0
info:
  version: V2.2
paths:
  /api/hostile:
    get: {}
  /api/animal:
    $ref: https://example.com/ignored.yaml
`)
		}, "https://example.com/ignored")
		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
		}
		if status.Capabilities.HostileAndAnimalPositions {
			t.Error("QuerySevenDaysToDieWebAPIStatus() combined position capability = true with no advertised animal GET")
		}
		if status.ConnectionState != SevenDaysToDieWebAPIConnectionStateAvailable || status.ObservedAt.IsZero() {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() = %+v", status)
		}
	})

	t.Run("classifies discovery failures without exposing upstream responses", func(t *testing.T) {
		tests := []struct {
			name      string
			handler   http.HandlerFunc
			wantState SevenDaysToDieWebAPIConnectionState
		}{
			{
				name: "authentication denied",
				handler: func(response http.ResponseWriter, _ *http.Request) {
					http.Error(response, "credential details", http.StatusUnauthorized)
				},
				wantState: SevenDaysToDieWebAPIConnectionStateAuthenticationDenied,
			},
			{
				name: "discovery unsupported",
				handler: func(response http.ResponseWriter, request *http.Request) {
					http.NotFound(response, request)
				},
				wantState: SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported,
			},
			{
				name: "unsupported OpenAPI version",
				handler: func(response http.ResponseWriter, _ *http.Request) {
					writeSevenDaysToDieTestResponse(t, response, "swagger: '2.0'\ninfo:\n  version: old\npaths: {}\n")
				},
				wantState: SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported,
			},
			{
				name: "malformed YAML",
				handler: func(response http.ResponseWriter, _ *http.Request) {
					writeSevenDaysToDieTestResponse(t, response, "openapi: [\n")
				},
				wantState: SevenDaysToDieWebAPIConnectionStateInvalidResponse,
			},
			{
				name: "oversized API version",
				handler: func(response http.ResponseWriter, _ *http.Request) {
					writeSevenDaysToDieTestResponse(t, response, "openapi: 3.1.0\ninfo:\n  version: "+strings.Repeat("v", 129)+"\npaths: {}\n")
				},
				wantState: SevenDaysToDieWebAPIConnectionStateInvalidResponse,
			},
			{
				name: "bounded discovery response",
				handler: func(response http.ResponseWriter, _ *http.Request) {
					writeSevenDaysToDieTestResponse(t, response, strings.Repeat("x", sevenDaysToDieWebAPIResponseLimit+1))
				},
				wantState: SevenDaysToDieWebAPIConnectionStateInvalidResponse,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				workingDirectory := startSevenDaysToDieWebAPITestServer(t, test.handler, "https://203.0.113.1")
				status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory})
				if errQuery != nil {
					t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
				}
				if status.ConnectionState != test.wantState {
					t.Errorf("QuerySevenDaysToDieWebAPIStatus() state = %v, want %v", status.ConnectionState, test.wantState)
				}
				if !status.ObservedAt.IsZero() {
					t.Errorf("QuerySevenDaysToDieWebAPIStatus() observed at = %v, want zero", status.ObservedAt)
				}
			})
		}
	})

	t.Run("does not follow redirects", func(t *testing.T) {
		followed := false
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/redirect-target" {
				followed = true
				writeSevenDaysToDieTestResponse(t, response, "openapi: 3.1.0")
				return
			}
			http.Redirect(response, request, "/redirect-target", http.StatusFound)
		}, "")
		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
		}
		if followed || status.ConnectionState != SevenDaysToDieWebAPIConnectionStateUnreachable {
			t.Errorf("redirect followed = %v, state = %v", followed, status.ConnectionState)
		}
	})

	t.Run("returns request cancellation as an error", func(t *testing.T) {
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(_ http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}, "")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(ctx, SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory})
		if !errors.Is(errQuery, context.Canceled) {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() error = %v, want context.Canceled", errQuery)
		}
	})
}

func writeSevenDaysToDieWebAPIConfig(t *testing.T, workingDirectory string, enabled string, port string, dashboardURL string) {
	t.Helper()
	config := `<ServerSettings>` +
		`<property name="WebDashboardEnabled" value="` + enabled + `" />` +
		`<property name="WebDashboardPort" value="` + port + `" />` +
		`<property name="WebDashboardUrl" value="` + dashboardURL + `" />` +
		`</ServerSettings>`
	errWrite := os.WriteFile(filepath.Join(workingDirectory, sevenDaysToDieServerConfigName), []byte(config), 0o600)
	if errWrite != nil {
		t.Fatalf("write server config: %v", errWrite)
	}
}

func startSevenDaysToDieWebAPITestServer(t *testing.T, handler http.HandlerFunc, dashboardURL string) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	serverURL, errParseURL := url.Parse(server.URL)
	if errParseURL != nil {
		t.Fatalf("parse test server URL: %v", errParseURL)
	}
	host, port, errSplitHostPort := net.SplitHostPort(serverURL.Host)
	if errSplitHostPort != nil {
		t.Fatalf("split test server host: %v", errSplitHostPort)
	}
	if host != "127.0.0.1" {
		t.Fatalf("test server host = %q, want 127.0.0.1", host)
	}
	workingDirectory := t.TempDir()
	writeSevenDaysToDieWebAPIConfig(t, workingDirectory, "true", port, dashboardURL)
	return workingDirectory
}

func fullSevenDaysToDieOpenAPI() string {
	return `openapi: 3.1.0
info:
  version: V2.2
servers:
  - url: https://example.com/ignored
paths:
  /api/player:
    get: {}
  /api/gameprefs:
    get: {}
  /api/gamestats:
    get: {}
  /api/log:
    get: {}
  /api/serverstats:
    get: {}
  /api/hostile:
    get: {}
  /api/animal:
    get: {}
  /api/blacklist:
    get: {}
  /api/blacklist/{id}:
    post: {}
    delete: {}
  /api/whitelist:
    get: {}
  /api/whitelist/user/{id}:
    post: {}
    delete: {}
  /api/userpermissions:
    get: {}
  /api/userpermissions/user/{id}:
    post: {}
    delete: {}
  /api/mods:
    get: {}
  /api/bloodmoon:
    get: {}
`
}
