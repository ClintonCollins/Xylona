package node

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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
		fragments := fullSevenDaysToDieOpenAPIFragments()
		pathCounts := make(map[string]int)
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			pathCounts[request.URL.Path]++
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
				fragment, found := fragments[request.URL.Path]
				if !found {
					http.NotFound(response, request)
					return
				}
				writeSevenDaysToDieTestResponse(t, response, fragment)
			}
		}, "https://example.com/should-never-be-used")

		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{
			WorkingDirectory: workingDirectory,
			TokenName:        tokenName,
			TokenSecret:      tokenSecret,
			IncludeTactical:  true,
		})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
		}
		if status.ConnectionState != SevenDaysToDieWebAPIConnectionStateAvailable || status.APIVersion != "1.0.0" {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() = %+v", status)
		}
		wantCapabilities := SevenDaysToDieWebAPICapabilities{
			PlayerData:                true,
			RuntimeSettings:           true,
			NativeLog:                 true,
			WorldPopulation:           true,
			HostileAndAnimalPositions: true,
			HostilePositions:          true,
			AnimalPositions:           true,
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
		wantPathCounts := map[string]int{
			"/api/openapi/openapi.yaml":                 1,
			"/api/OpenAPI/Animal.openapi.yaml":          1,
			"/api/OpenAPI/Blacklist.openapi.yaml":       1,
			"/api/OpenAPI/Bloodmoon.openapi.yaml":       1,
			"/api/OpenAPI/GamePrefs.openapi.yaml":       1,
			"/api/OpenAPI/GameStats.openapi.yaml":       1,
			"/api/OpenAPI/Hostile.openapi.yaml":         1,
			"/api/OpenAPI/Log.openapi.yaml":             1,
			"/api/OpenAPI/Mods.openapi.yaml":            1,
			"/api/OpenAPI/Player.openapi.yaml":          1,
			"/api/OpenAPI/ServerStats.openapi.yaml":     1,
			"/api/OpenAPI/UserPermissions.openapi.yaml": 1,
			"/api/OpenAPI/Whitelist.openapi.yaml":       1,
			"/api/bloodmoon":                            1,
		}
		if !maps.Equal(pathCounts, wantPathCounts) {
			t.Errorf("QuerySevenDaysToDieWebAPIStatus() request counts = %v, want %v", pathCounts, wantPathCounts)
		}
	})

	t.Run("keeps viewer diagnostics without fetching tactical status", func(t *testing.T) {
		paths := make([]string, 0, 3)
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			paths = append(paths, request.URL.Path)
			switch request.URL.Path {
			case "/api/openapi/openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, `openapi: 3.1.0
info:
  version: V2.2
paths:
  /api/player:
    get: {}
  /api/bloodmoon:
    get: {}
  /api/hostile:
    get: {}
  /api/animal:
    get: {}
  /api/serverstats:
    get: {}
`)
			case "/api/bloodmoon":
				t.Fatal("view-only status requested the Blood Moon endpoint")
			case "/api/serverstats":
				writeSevenDaysToDieTestResponse(t, response, `{"data":{"gameTime":{"days":8,"hours":13,"minutes":37}}}`)
			default:
				http.NotFound(response, request)
			}
		}, "")

		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{
			WorkingDirectory: workingDirectory,
		})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
		}
		if status.WorldTimeState != SevenDaysToDieWebAPIValueStateAvailable || status.WorldTime == nil || status.WorldTime.Day != 8 ||
			!status.Capabilities.PlayerData || status.Capabilities.HostileAndAnimalPositions || status.Capabilities.HostilePositions ||
			status.Capabilities.AnimalPositions || status.BloodMoonState != SevenDaysToDieWebAPIValueStateUnspecified ||
			status.BloodMoonActive != nil || status.NextBloodMoon != nil || status.NextBloodMoonEnd != nil {
			t.Fatalf("view-only status = %+v", status)
		}
		if slices.Contains(paths, "/api/bloodmoon") {
			t.Fatalf("view-only request paths = %v", paths)
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
		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory, IncludeTactical: true})
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
		status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory, IncludeTactical: true})
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

	t.Run("rejects unsafe references without requesting them", func(t *testing.T) {
		tests := []struct {
			name      string
			reference string
		}{
			{name: "external URL", reference: "https://example.com/Player.openapi.yaml#/paths/~1api~1player"},
			{name: "external authority", reference: "//example.com/Player.openapi.yaml#/paths/~1api~1player"},
			{name: "traversal", reference: "./../Player.openapi.yaml#/paths/~1api~1player"},
			{name: "nested path", reference: "./nested/Player.openapi.yaml#/paths/~1api~1player"},
			{name: "encoded traversal", reference: "./%2e%2e%2fPlayer.openapi.yaml#/paths/~1api~1player"},
			{name: "encoded forward slash", reference: "./nested%2fPlayer.openapi.yaml#/paths/~1api~1player"},
			{name: "encoded backslash", reference: "./nested%5cPlayer.openapi.yaml#/paths/~1api~1player"},
			{name: "query", reference: "./Player.openapi.yaml?cache=1#/paths/~1api~1player"},
			{name: "backslash", reference: `.\Player.openapi.yaml#/paths/~1api~1player`},
			{name: "wrong pointer", reference: "./Player.openapi.yaml#/paths/~1api~1animal"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				var unexpectedPaths []string
				workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
					if request.URL.Path != "/api/openapi/openapi.yaml" {
						unexpectedPaths = append(unexpectedPaths, request.URL.Path)
						http.NotFound(response, request)
						return
					}
					writeSevenDaysToDieTestResponse(t, response, fmt.Sprintf(`openapi: 3.1.0
info:
  version: '1.0.0'
paths:
  /api/player:
    $ref: %q
`, test.reference))
				}, "")

				status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory})
				if errQuery != nil {
					t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
				}
				if status.ConnectionState != SevenDaysToDieWebAPIConnectionStateAvailable || status.Capabilities.PlayerData {
					t.Errorf("QuerySevenDaysToDieWebAPIStatus() = %+v", status)
				}
				if len(unexpectedPaths) != 0 {
					t.Errorf("QuerySevenDaysToDieWebAPIStatus() requested unsafe paths %v", unexpectedPaths)
				}
			})
		}
	})

	t.Run("isolates fragment failures to the affected capability", func(t *testing.T) {
		tests := []struct {
			name    string
			handler http.HandlerFunc
		}{
			{
				name: "missing fragment",
				handler: func(response http.ResponseWriter, request *http.Request) {
					http.NotFound(response, request)
				},
			},
			{
				name: "malformed fragment",
				handler: func(response http.ResponseWriter, _ *http.Request) {
					writeSevenDaysToDieTestResponse(t, response, "openapi: [\n")
				},
			},
			{
				name: "oversized fragment",
				handler: func(response http.ResponseWriter, _ *http.Request) {
					writeSevenDaysToDieTestResponse(t, response, strings.Repeat("x", sevenDaysToDieWebAPIResponseLimit+1))
				},
			},
			{
				name: "redirected fragment",
				handler: func(response http.ResponseWriter, request *http.Request) {
					http.Redirect(response, request, "/redirect-target", http.StatusFound)
				},
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				fragmentRequests := 0
				redirectFollowed := false
				workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
					switch request.URL.Path {
					case "/api/openapi/openapi.yaml":
						writeSevenDaysToDieTestResponse(t, response, `openapi: 3.1.0
info:
  version: '1.0.0'
paths:
  /api/log:
    get: {}
  /api/player:
    $ref: './Shared.openapi.yaml#/paths/~1api~1player'
  /api/mods:
    $ref: './Shared.openapi.yaml#/paths/~1api~1mods'
`)
					case "/api/OpenAPI/Shared.openapi.yaml":
						fragmentRequests++
						test.handler(response, request)
					case "/redirect-target":
						redirectFollowed = true
						writeSevenDaysToDieTestResponse(t, response, "paths:\n  /api/mods:\n    get: {}\n")
					default:
						http.NotFound(response, request)
					}
				}, "")

				status, errQuery := new(Node).QuerySevenDaysToDieWebAPIStatus(t.Context(), SevenDaysToDieWebAPIStatusQueryRequest{WorkingDirectory: workingDirectory})
				if errQuery != nil {
					t.Fatalf("QuerySevenDaysToDieWebAPIStatus() error = %v", errQuery)
				}
				if status.ConnectionState != SevenDaysToDieWebAPIConnectionStateAvailable || !status.Capabilities.NativeLog ||
					status.Capabilities.PlayerData || status.Capabilities.ReportedMods {
					t.Errorf("QuerySevenDaysToDieWebAPIStatus() = %+v", status)
				}
				if fragmentRequests != 1 || redirectFollowed {
					t.Errorf("fragment requests = %d, redirect followed = %v", fragmentRequests, redirectFollowed)
				}
			})
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

func TestProjectSevenDaysToDieWebAPICapabilitiesKeepsEntityEndpointsIndependent(t *testing.T) {
	resolver := &sevenDaysToDieOpenAPIResolver{document: sevenDaysToDieOpenAPI{
		Paths: map[string]map[string]yaml.Node{
			"/api/hostile": {"get": {}},
		},
	}}
	capabilities := projectSevenDaysToDieWebAPICapabilities(resolver)
	if !capabilities.HostilePositions || capabilities.AnimalPositions || capabilities.HostileAndAnimalPositions {
		t.Fatalf("independent entity capabilities = %+v", capabilities)
	}
}

func TestNodeQuerySevenDaysToDiePlayers(t *testing.T) {
	t.Parallel()

	t.Run("decodes identifiers and optional zero values from the fixed player endpoint", func(t *testing.T) {
		t.Parallel()
		paths := make([]string, 0, 3)
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			paths = append(paths, request.URL.Path)
			if request.Header.Get("X-SDTD-API-TOKENNAME") != "xylona" || request.Header.Get("X-SDTD-API-SECRET") != "secret" {
				t.Fatal("QuerySevenDaysToDiePlayers() did not send the node-held credentials")
			}
			switch request.URL.Path {
			case "/api/openapi/openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, fullSevenDaysToDieOpenAPI())
			case "/api/OpenAPI/Player.openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, fullSevenDaysToDieOpenAPIFragments()[request.URL.Path])
			case "/api/player":
				writeSevenDaysToDieTestResponse(t, response, `{"data":{"players":[
					{"entityId":42,"name":"Platform","platformId":{"combinedString":"Steam_100"},"crossplatformId":{"combinedString":"EOS_200"},"online":false,"ping":0,"level":0,"health":0,"stamina":0,"score":0,"deaths":0,"kills":{"zombies":0,"players":0},"banned":{"banActive":false},"ip":"203.0.113.10","position":{"x":1,"y":2,"z":3}},
					{"entityId":"43","name":"Cross-platform","crossplatformId":{"combinedString":"EOS_201"}},
					{"entityId":44,"name":"Entity only"},
					{"name":"Read only"}
				]},"meta":{}}`)
			default:
				http.NotFound(response, request)
			}
		}, "")

		result, errQuery := new(Node).QuerySevenDaysToDiePlayers(t.Context(), SevenDaysToDiePlayersQueryRequest{
			WorkingDirectory: workingDirectory,
			TokenName:        "xylona",
			TokenSecret:      "secret",
		})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDiePlayers() error = %v", errQuery)
		}
		if result.ConnectionState != SevenDaysToDieWebAPIConnectionStateAvailable || result.State != SevenDaysToDieWebAPIValueStateAvailable {
			t.Fatalf("QuerySevenDaysToDiePlayers() states = %v, %v", result.ConnectionState, result.State)
		}
		if len(result.Players) != 4 {
			t.Fatalf("QuerySevenDaysToDiePlayers() players = %+v", result.Players)
		}
		platform := result.Players[0]
		if platform.ActionID != "Steam_100" || platform.EntityID != "42" || platform.PlatformID != "Steam_100" || platform.CrossPlatformID != "EOS_200" {
			t.Fatalf("platform identifiers = %+v", platform)
		}
		if platform.Online == nil || *platform.Online || platform.Ping == nil || *platform.Ping != 0 ||
			platform.Level == nil || *platform.Level != 0 || platform.Health == nil || *platform.Health != 0 ||
			platform.Stamina == nil || *platform.Stamina != 0 || platform.Score == nil || *platform.Score != 0 ||
			platform.Deaths == nil || *platform.Deaths != 0 || platform.ZombieKills == nil || *platform.ZombieKills != 0 ||
			platform.PlayerKills == nil || *platform.PlayerKills != 0 || platform.Banned == nil || *platform.Banned {
			t.Fatalf("optional zero values = %+v", platform)
		}
		if result.Players[1].ActionID != "EOS_201" || result.Players[2].ActionID != "44" || result.Players[3].ActionID != "" {
			t.Fatalf("identifier precedence = %+v", result.Players)
		}
		wantPaths := []string{"/api/openapi/openapi.yaml", "/api/OpenAPI/Player.openapi.yaml", "/api/player"}
		if !slices.Equal(paths, wantPaths) {
			t.Fatalf("QuerySevenDaysToDiePlayers() paths = %v, want %v", paths, wantPaths)
		}
	})

	tests := []struct {
		name             string
		master           string
		masterStatusCode int
		statusCode       int
		body             string
		waitForTimeout   bool
		wantConnection   SevenDaysToDieWebAPIConnectionState
		wantState        SevenDaysToDieWebAPIValueState
		wantCount        int
	}{
		{name: "confirmed empty roster", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusOK, body: `{"data":{"players":[]},"meta":{}}`, wantState: SevenDaysToDieWebAPIValueStateAvailable},
		{name: "missing capability", master: "openapi: 3.1.0\ninfo:\n  version: '1'\npaths: {}\n", wantState: SevenDaysToDieWebAPIValueStateUnsupported},
		{name: "discovery unauthorized", masterStatusCode: http.StatusUnauthorized, wantConnection: SevenDaysToDieWebAPIConnectionStateAuthenticationDenied, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "discovery forbidden", masterStatusCode: http.StatusForbidden, wantConnection: SevenDaysToDieWebAPIConnectionStateAuthenticationDenied, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "endpoint unauthorized", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusUnauthorized, wantState: SevenDaysToDieWebAPIValueStatePermissionDenied},
		{name: "endpoint forbidden", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusForbidden, wantState: SevenDaysToDieWebAPIValueStatePermissionDenied},
		{name: "internal query timeout", master: fullSevenDaysToDieOpenAPI(), waitForTimeout: true, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "malformed result", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusOK, body: `{"data":{"players":`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "oversized result", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusOK, body: strings.Repeat("x", sevenDaysToDieWebAPIResponseLimit+1), wantState: SevenDaysToDieWebAPIValueStateUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/openapi/openapi.yaml":
					if test.masterStatusCode != 0 {
						response.WriteHeader(test.masterStatusCode)
						return
					}
					writeSevenDaysToDieTestResponse(t, response, test.master)
				case "/api/OpenAPI/Player.openapi.yaml":
					writeSevenDaysToDieTestResponse(t, response, fullSevenDaysToDieOpenAPIFragments()[request.URL.Path])
				case "/api/player":
					if test.waitForTimeout {
						<-request.Context().Done()
						return
					}
					response.WriteHeader(test.statusCode)
					if test.body != "" {
						writeSevenDaysToDieTestResponse(t, response, test.body)
					}
				default:
					http.NotFound(response, request)
				}
			}, "")
			result, errQuery := new(Node).QuerySevenDaysToDiePlayers(t.Context(), SevenDaysToDiePlayersQueryRequest{WorkingDirectory: workingDirectory})
			if errQuery != nil {
				t.Fatalf("QuerySevenDaysToDiePlayers() error = %v", errQuery)
			}
			wantConnection := test.wantConnection
			if wantConnection == SevenDaysToDieWebAPIConnectionStateUnspecified {
				wantConnection = SevenDaysToDieWebAPIConnectionStateAvailable
			}
			if result.ConnectionState != wantConnection || result.State != test.wantState || len(result.Players) != test.wantCount {
				t.Fatalf("QuerySevenDaysToDiePlayers() = %+v, want state %v and %d players", result, test.wantState, test.wantCount)
			}
		})
	}

	t.Run("returns parent cancellation as an error", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, errQuery := new(Node).QuerySevenDaysToDiePlayers(ctx, SevenDaysToDiePlayersQueryRequest{})
		if !errors.Is(errQuery, context.Canceled) {
			t.Fatalf("QuerySevenDaysToDiePlayers() error = %v, want context.Canceled", errQuery)
		}
	})
}

func TestNodeQuerySevenDaysToDieReportedMods(t *testing.T) {
	t.Parallel()

	t.Run("decodes only approved text fields from the fixed mods endpoint", func(t *testing.T) {
		t.Parallel()
		paths := make([]string, 0, 3)
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			paths = append(paths, request.URL.Path)
			switch request.URL.Path {
			case "/api/openapi/openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, fullSevenDaysToDieOpenAPI())
			case "/api/OpenAPI/Mods.openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, fullSevenDaysToDieOpenAPIFragments()[request.URL.Path])
			case "/api/mods":
				if request.Header.Get("X-SDTD-API-TOKENNAME") != "xylona" || request.Header.Get("X-SDTD-API-SECRET") != "secret" {
					t.Fatal("QuerySevenDaysToDieReportedMods() did not send the node-held credentials")
				}
				writeSevenDaysToDieTestResponse(t, response, `{"data":[{"name":"Example","displayName":"<b>Example</b>","description":"Plain text","author":"Operator","version":"1.2.3","website":"https://example.invalid","web":{"baseUrl":"/webmods/Example/","bundle":"/webmods/Example/bundle.js"}}],"meta":{}}`)
			default:
				http.NotFound(response, request)
			}
		}, "")

		result, errQuery := new(Node).QuerySevenDaysToDieReportedMods(t.Context(), SevenDaysToDieReportedModsQueryRequest{
			WorkingDirectory: workingDirectory,
			TokenName:        "xylona",
			TokenSecret:      "secret",
		})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieReportedMods() error = %v", errQuery)
		}
		if result.ConnectionState != SevenDaysToDieWebAPIConnectionStateAvailable || result.State != SevenDaysToDieWebAPIValueStateAvailable || len(result.Mods) != 1 {
			t.Fatalf("QuerySevenDaysToDieReportedMods() = %+v", result)
		}
		want := SevenDaysToDieReportedMod{Name: "Example", DisplayName: "<b>Example</b>", Description: "Plain text", Author: "Operator", Version: "1.2.3"}
		if result.Mods[0] != want {
			t.Fatalf("reported mod = %+v, want %+v", result.Mods[0], want)
		}
		wantPaths := []string{"/api/openapi/openapi.yaml", "/api/OpenAPI/Mods.openapi.yaml", "/api/mods"}
		if !slices.Equal(paths, wantPaths) {
			t.Fatalf("QuerySevenDaysToDieReportedMods() paths = %v, want %v", paths, wantPaths)
		}
	})

	tests := []struct {
		name             string
		master           string
		masterStatusCode int
		statusCode       int
		body             string
		waitForTimeout   bool
		wantConnection   SevenDaysToDieWebAPIConnectionState
		wantState        SevenDaysToDieWebAPIValueState
	}{
		{name: "confirmed empty list", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusOK, body: `{"data":[],"meta":{}}`, wantState: SevenDaysToDieWebAPIValueStateAvailable},
		{name: "missing capability", master: "openapi: 3.1.0\ninfo:\n  version: '1'\npaths: {}\n", wantState: SevenDaysToDieWebAPIValueStateUnsupported},
		{name: "discovery unauthorized", masterStatusCode: http.StatusUnauthorized, wantConnection: SevenDaysToDieWebAPIConnectionStateAuthenticationDenied, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "discovery forbidden", masterStatusCode: http.StatusForbidden, wantConnection: SevenDaysToDieWebAPIConnectionStateAuthenticationDenied, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "endpoint unauthorized", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusUnauthorized, wantState: SevenDaysToDieWebAPIValueStatePermissionDenied},
		{name: "endpoint forbidden", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusForbidden, wantState: SevenDaysToDieWebAPIValueStatePermissionDenied},
		{name: "internal query timeout", master: fullSevenDaysToDieOpenAPI(), waitForTimeout: true, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "malformed result", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusOK, body: `{"data":`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "oversized result", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusOK, body: strings.Repeat("x", sevenDaysToDieWebAPIResponseLimit+1), wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "over-count result", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusOK, body: `{"data":[` + strings.Repeat(`{},`, SevenDaysToDieReportedModCountLimit) + `{}` + `]}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "over-limit field result", master: fullSevenDaysToDieOpenAPI(), statusCode: http.StatusOK, body: `{"data":[{"description":"` + strings.Repeat("x", SevenDaysToDieReportedModFieldByteLimit+1) + `"}]}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/openapi/openapi.yaml":
					if test.masterStatusCode != 0 {
						response.WriteHeader(test.masterStatusCode)
						return
					}
					writeSevenDaysToDieTestResponse(t, response, test.master)
				case "/api/OpenAPI/Mods.openapi.yaml":
					writeSevenDaysToDieTestResponse(t, response, fullSevenDaysToDieOpenAPIFragments()[request.URL.Path])
				case "/api/mods":
					if test.waitForTimeout {
						<-request.Context().Done()
						return
					}
					response.WriteHeader(test.statusCode)
					if test.body != "" {
						writeSevenDaysToDieTestResponse(t, response, test.body)
					}
				default:
					http.NotFound(response, request)
				}
			}, "")
			result, errQuery := new(Node).QuerySevenDaysToDieReportedMods(t.Context(), SevenDaysToDieReportedModsQueryRequest{WorkingDirectory: workingDirectory})
			if errQuery != nil {
				t.Fatalf("QuerySevenDaysToDieReportedMods() error = %v", errQuery)
			}
			wantConnection := test.wantConnection
			if wantConnection == SevenDaysToDieWebAPIConnectionStateUnspecified {
				wantConnection = SevenDaysToDieWebAPIConnectionStateAvailable
			}
			if result.ConnectionState != wantConnection || result.State != test.wantState || len(result.Mods) != 0 {
				t.Fatalf("QuerySevenDaysToDieReportedMods() = %+v, want state %v", result, test.wantState)
			}
		})
	}
}

func TestDecodeSevenDaysToDieSandboxSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCode  string
		wantKey   string
		wantGroup string
		wantValue string
		wantLabel string
		wantErr   bool
		buildBody func() string
	}{
		{
			name:     "decodes categorized native metadata",
			body:     `{"data":{"sandboxCode":"AAAE","categories":[{"name":"Player","settings":[{"enumName":"RangedDamage","localizedName":"Ranged damage","description":"<b>untrusted</b>","currentValue":0.85,"localizedValue":"85%"}]}]}}`,
			wantCode: "AAAE", wantKey: "RangedDamage", wantGroup: "Player", wantValue: "0.85", wantLabel: "85%",
		},
		{
			name:     "decodes tolerant scalar aliases",
			body:     `{"data":{"code":"A","settings":[{"key":"EnemySpawn","label":"Enemy spawning","group":"Entities","value":true,"valueLabel":"Enabled"}]}}`,
			wantCode: "A", wantKey: "EnemySpawn", wantGroup: "Entities", wantValue: "true", wantLabel: "Enabled",
		},
		{name: "rejects a missing data envelope", body: `{"settings":[]}`, wantErr: true},
		{name: "rejects conflicting validity aliases", body: `{"data":{"valid":false,"recognized":true}}`, wantErr: true},
		{name: "rejects conflicting code aliases", body: `{"data":{"code":"A","sandboxCode":"B","settings":[{"key":"x","value":"1"}]}}`, wantErr: true},
		{name: "rejects a non-scalar setting value", body: `{"data":{"settings":[{"key":"EnemySpawn","value":{"nested":true}}]}}`, wantErr: true},
		{
			name: "rejects oversized text",
			buildBody: func() string {
				return `{"data":{"settings":[{"key":"EnemySpawn","value":"` + strings.Repeat("x", SevenDaysToDieSandboxTextByteLimit+1) + `"}]}}`
			},
			wantErr: true,
		},
		{
			name: "rejects too many settings",
			buildBody: func() string {
				var body strings.Builder
				body.WriteString(`{"data":{"settings":[`)
				for index := range SevenDaysToDieSandboxSettingCountLimit + 1 {
					if index > 0 {
						body.WriteByte(',')
					}
					fmt.Fprintf(&body, `{"key":"setting-%d","value":%d}`, index, index)
				}
				body.WriteString(`]}}`)
				return body.String()
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := test.body
			if test.buildBody != nil {
				body = test.buildBody()
			}
			snapshot, errDecode := decodeSevenDaysToDieSandboxSnapshot([]byte(body))
			if (errDecode != nil) != test.wantErr {
				t.Fatalf("decodeSevenDaysToDieSandboxSnapshot() error = %v, wantErr %v", errDecode, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if snapshot.code != test.wantCode || len(snapshot.settings) != 1 {
				t.Fatalf("decodeSevenDaysToDieSandboxSnapshot() = %+v", snapshot)
			}
			setting := snapshot.settings[0]
			if setting.Key != test.wantKey || setting.Group != test.wantGroup || setting.EffectiveValue != test.wantValue || setting.EffectiveLabel != test.wantLabel {
				t.Fatalf("setting = %+v", setting)
			}
		})
	}

	t.Run("accepts explicit invalidity without settings", func(t *testing.T) {
		snapshot, errDecode := decodeSevenDaysToDieSandboxSnapshot([]byte(`{"data":{"valid":false}}`))
		if errDecode != nil {
			t.Fatalf("decodeSevenDaysToDieSandboxSnapshot() error = %v", errDecode)
		}
		if !snapshot.validityKnown || snapshot.valid || len(snapshot.settings) != 0 {
			t.Fatalf("decodeSevenDaysToDieSandboxSnapshot() = %+v", snapshot)
		}
	})
}

func TestValidateSevenDaysToDieSandboxJSONStructure(t *testing.T) {
	deep := strings.Repeat("[", sevenDaysToDieSandboxJSONDepthLimit+1) + "null" +
		strings.Repeat("]", sevenDaysToDieSandboxJSONDepthLimit+1)
	var wide strings.Builder
	wide.WriteString(`{"data":{`)
	for index := range sevenDaysToDieSandboxJSONContainerEntryLimit + 1 {
		if index > 0 {
			wide.WriteByte(',')
		}
		fmt.Fprintf(&wide, `"ignored-%d":null`, index)
	}
	wide.WriteString(`}}`)
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{name: "bounded aliases", body: `{"data":{"code":"A","settings":[{"key":"EnemySpawn","value":true}],"future":null}}`},
		{name: "over-limit nesting", body: deep, wantErr: errSevenDaysToDieSandboxJSONStructure},
		{name: "wide valid object", body: wide.String(), wantErr: errSevenDaysToDieSandboxJSONStructure},
		{name: "duplicate case-insensitive key", body: `{"data":{"code":"A","Code":"B"}}`, wantErr: errors.New("duplicate")},
		{name: "multiple roots", body: `{} {}`, wantErr: errors.New("multiple")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			errValidate := validateSevenDaysToDieSandboxJSONStructure([]byte(test.body))
			if test.wantErr == nil && errValidate != nil {
				t.Fatalf("validateSevenDaysToDieSandboxJSONStructure() error = %v", errValidate)
			}
			if test.wantErr != nil && errValidate == nil {
				t.Fatal("validateSevenDaysToDieSandboxJSONStructure() accepted invalid structure")
			}
			if errors.Is(test.wantErr, errSevenDaysToDieSandboxJSONStructure) && !errors.Is(errValidate, errSevenDaysToDieSandboxJSONStructure) {
				t.Fatalf("validateSevenDaysToDieSandboxJSONStructure() error = %v, want %v", errValidate, errSevenDaysToDieSandboxJSONStructure)
			}
		})
	}
}

func TestNodeQuerySevenDaysToDieSandboxSettings(t *testing.T) {
	const configuredCode = "AAAJABJACJADJARFBNC"

	t.Run("marks matching codes and settings", func(t *testing.T) {
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/openapi/openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, "openapi: 3.1.0\ninfo:\n  version: '3.0'\npaths:\n  /api/sandboxsettings:\n    get: {}\n")
			case "/api/sandboxsettings":
				writeSevenDaysToDieTestResponse(t, response, `{"data":{"code":"AAAJABJACJADJARFBNC","settings":[{"key":"RangedDamage","value":"1"}]}}`)
			default:
				http.NotFound(response, request)
			}
		}, "")

		result, errQuery := new(Node).QuerySevenDaysToDieSandboxSettings(t.Context(), SevenDaysToDieSandboxSettingsQueryRequest{WorkingDirectory: workingDirectory})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieSandboxSettings() error = %v", errQuery)
		}
		if result.ComparisonState != SevenDaysToDieSandboxComparisonStateMatch || len(result.Settings) != 1 || result.Settings[0].EffectiveValue != "1" {
			t.Fatalf("QuerySevenDaysToDieSandboxSettings() = %+v", result)
		}
	})

	t.Run("reports a truthful mismatch from one current GET", func(t *testing.T) {
		var methods []string
		sandboxRequests := 0
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			methods = append(methods, request.Method)
			switch request.URL.Path {
			case "/api/openapi/openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, "openapi: 3.1.0\ninfo:\n  version: '3.0'\npaths:\n  /api/sandboxsettings:\n    get: {}\n")
			case "/api/sandboxsettings":
				sandboxRequests++
				if request.URL.RawQuery != "" {
					t.Errorf("sandbox settings query = %q, want none", request.URL.RawQuery)
				}
				writeSevenDaysToDieTestResponse(t, response, `{"data":{"code":"AAAE","groups":[{"title":"Player","options":[{"name":"RangedDamage","title":"Ranged damage","description":"<script>text only</script>","value":"0.85","displayValue":"85%"}]}]}}`)
			default:
				http.NotFound(response, request)
			}
		}, "")
		configPath := filepath.Join(workingDirectory, sevenDaysToDieServerConfigName)
		before, errRead := os.ReadFile(configPath)
		if errRead != nil {
			t.Fatalf("read config before query: %v", errRead)
		}

		result, errQuery := new(Node).QuerySevenDaysToDieSandboxSettings(t.Context(), SevenDaysToDieSandboxSettingsQueryRequest{WorkingDirectory: workingDirectory})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieSandboxSettings() error = %v", errQuery)
		}
		if result.State != SevenDaysToDieWebAPIValueStateAvailable || result.ComparisonState != SevenDaysToDieSandboxComparisonStateMismatch ||
			result.ConfiguredCode != configuredCode || result.EffectiveCode != "AAAE" || len(result.Settings) != 1 {
			t.Fatalf("QuerySevenDaysToDieSandboxSettings() = %+v", result)
		}
		setting := result.Settings[0]
		if setting.EffectiveLabel != "85%" || setting.Description != "<script>text only</script>" {
			t.Fatalf("observed setting = %+v", setting)
		}
		if sandboxRequests != 1 {
			t.Fatalf("sandbox settings requests = %d, want 1", sandboxRequests)
		}
		if slices.ContainsFunc(methods, func(method string) bool { return method != http.MethodGet }) {
			t.Fatalf("request methods = %v, want GET only", methods)
		}
		after, errRead := os.ReadFile(configPath)
		if errRead != nil {
			t.Fatalf("read config after query: %v", errRead)
		}
		if string(after) != string(before) {
			t.Fatal("sandbox settings query modified serverconfig.xml")
		}
	})

	tests := []struct {
		name           string
		openAPI        string
		responseStatus int
		responseBody   string
		wantState      SevenDaysToDieWebAPIValueState
		wantComparison SevenDaysToDieSandboxComparisonState
	}{
		{name: "unsupported", openAPI: "openapi: 3.1.0\ninfo:\n  version: '2.6'\npaths: {}\n", wantState: SevenDaysToDieWebAPIValueStateUnsupported},
		{name: "upstream unauthorized", responseStatus: http.StatusForbidden, wantState: SevenDaysToDieWebAPIValueStatePermissionDenied},
		{name: "missing effective code", responseStatus: http.StatusOK, responseBody: `{"data":{"settings":[{"key":"x","value":"1"}]}}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "malformed upstream data", responseStatus: http.StatusOK, responseBody: `{"data":{"settings":[{"key":"x","value":{}}]}}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "oversized upstream data", responseStatus: http.StatusOK, responseBody: strings.Repeat("x", sevenDaysToDieWebAPIResponseLimit+1), wantState: SevenDaysToDieWebAPIValueStateUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/openapi/openapi.yaml":
					openAPI := test.openAPI
					if openAPI == "" {
						openAPI = "openapi: 3.1.0\ninfo:\n  version: '3.0'\npaths:\n  /api/sandboxsettings:\n    get: {}\n"
					}
					writeSevenDaysToDieTestResponse(t, response, openAPI)
				case "/api/sandboxsettings":
					response.WriteHeader(test.responseStatus)
					_, errWrite := response.Write([]byte(test.responseBody))
					if errWrite != nil {
						t.Errorf("write response: %v", errWrite)
					}
				default:
					http.NotFound(response, request)
				}
			}, "")

			result, errQuery := new(Node).QuerySevenDaysToDieSandboxSettings(t.Context(), SevenDaysToDieSandboxSettingsQueryRequest{WorkingDirectory: workingDirectory})
			if errQuery != nil {
				t.Fatalf("QuerySevenDaysToDieSandboxSettings() error = %v", errQuery)
			}
			if result.State != test.wantState || result.ComparisonState != test.wantComparison {
				t.Fatalf("QuerySevenDaysToDieSandboxSettings() = %+v", result)
			}
		})
	}

	t.Run("maps explicit invalid current data to stale without settings", func(t *testing.T) {
		sandboxRequests := 0
		workingDirectory := startSevenDaysToDieWebAPITestServer(t, func(response http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/openapi/openapi.yaml":
				writeSevenDaysToDieTestResponse(t, response, "openapi: 3.1.0\ninfo:\n  version: '3.0'\npaths:\n  /api/sandboxsettings:\n    get: {}\n")
			case "/api/sandboxsettings":
				sandboxRequests++
				if request.URL.RawQuery != "" {
					t.Errorf("sandbox settings query = %q, want none", request.URL.RawQuery)
				}
				writeSevenDaysToDieTestResponse(t, response, `{"data":{"valid":false}}`)
			default:
				http.NotFound(response, request)
			}
		}, "")

		result, errQuery := new(Node).QuerySevenDaysToDieSandboxSettings(t.Context(), SevenDaysToDieSandboxSettingsQueryRequest{WorkingDirectory: workingDirectory})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieSandboxSettings() error = %v", errQuery)
		}
		if result.State != SevenDaysToDieWebAPIValueStateAvailable || result.ComparisonState != SevenDaysToDieSandboxComparisonStateStale || len(result.Settings) != 0 {
			t.Fatalf("QuerySevenDaysToDieSandboxSettings() = %+v", result)
		}
		if sandboxRequests != 1 {
			t.Fatalf("sandbox settings requests = %d, want 1", sandboxRequests)
		}
	})
}

func writeSevenDaysToDieWebAPIConfig(t *testing.T, workingDirectory string, enabled string, port string, dashboardURL string) {
	t.Helper()
	config := `<ServerSettings>` +
		`<property name="WebDashboardEnabled" value="` + enabled + `" />` +
		`<property name="WebDashboardPort" value="` + port + `" />` +
		`<property name="WebDashboardUrl" value="` + dashboardURL + `" />` +
		`<property name="SandboxCode" value="AAAJABJACJADJARFBNC" />` +
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
  version: '1.0.0'
servers:
  - url: https://example.com/ignored
paths:
  /api/player:
    $ref: './Player.openapi.yaml#/paths/~1api~1player'
  /api/gameprefs:
    $ref: './GamePrefs.openapi.yaml#/paths/~1api~1gameprefs'
  /api/gamestats:
    $ref: './GameStats.openapi.yaml#/paths/~1api~1gamestats'
  /api/log:
    $ref: './Log.openapi.yaml#/paths/~1api~1log'
  /api/serverstats:
    $ref: './ServerStats.openapi.yaml#/paths/~1api~1serverstats'
  /api/hostile:
    $ref: './Hostile.openapi.yaml#/paths/~1api~1hostile'
  /api/animal:
    $ref: './Animal.openapi.yaml#/paths/~1api~1animal'
  /api/blacklist:
    $ref: './Blacklist.openapi.yaml#/paths/~1api~1blacklist'
  /api/blacklist/{id}:
    $ref: './Blacklist.openapi.yaml#/paths/~1api~1blacklist~1{id}'
  /api/whitelist:
    $ref: './Whitelist.openapi.yaml#/paths/~1api~1whitelist'
  /api/whitelist/user/{id}:
    $ref: './Whitelist.openapi.yaml#/paths/~1api~1whitelist~1user~1{id}'
  /api/userpermissions:
    $ref: './UserPermissions.openapi.yaml#/paths/~1api~1userpermissions'
  /api/userpermissions/user/{id}:
    $ref: './UserPermissions.openapi.yaml#/paths/~1api~1userpermissions~1user~1{id}'
  /api/mods:
    $ref: './Mods.openapi.yaml#/paths/~1api~1mods'
  /api/bloodmoon:
    $ref: './Bloodmoon.openapi.yaml#/paths/~1api~1bloodmoon'
`
}

func fullSevenDaysToDieOpenAPIFragments() map[string]string {
	return map[string]string{
		"/api/OpenAPI/Player.openapi.yaml": `paths:
  /api/player:
    get: {}
`,
		"/api/OpenAPI/GamePrefs.openapi.yaml": `paths:
  /api/gameprefs:
    get: {}
`,
		"/api/OpenAPI/GameStats.openapi.yaml": `paths:
  /api/gamestats:
    get: {}
`,
		"/api/OpenAPI/Log.openapi.yaml": `paths:
  /api/log:
    get: {}
`,
		"/api/OpenAPI/ServerStats.openapi.yaml": `paths:
  /api/serverstats:
    get: {}
`,
		"/api/OpenAPI/Hostile.openapi.yaml": `paths:
  /api/hostile:
    get: {}
`,
		"/api/OpenAPI/Animal.openapi.yaml": `paths:
  /api/animal:
    get: {}
`,
		"/api/OpenAPI/Blacklist.openapi.yaml": `paths:
  /api/blacklist:
    get: {}
  /api/blacklist/{id}:
    post: {}
    delete: {}
`,
		"/api/OpenAPI/Whitelist.openapi.yaml": `paths:
  /api/whitelist:
    get: {}
  /api/whitelist/user/{id}:
    post: {}
    delete: {}
`,
		"/api/OpenAPI/UserPermissions.openapi.yaml": `paths:
  /api/userpermissions:
    get: {}
  /api/userpermissions/user/{id}:
    post: {}
    delete: {}
`,
		"/api/OpenAPI/Mods.openapi.yaml": `paths:
  /api/mods:
    get: {}
`,
		"/api/OpenAPI/Bloodmoon.openapi.yaml": `paths:
  /api/bloodmoon:
    get: {}
`,
	}
}
