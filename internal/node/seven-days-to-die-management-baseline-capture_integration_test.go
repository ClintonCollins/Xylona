//go:build integration

package node

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	sevenDaysToDieBaselineCaptureFlag = "XYLONA_CAPTURE_7DTD_BASELINE"
	sevenDaysToDieBaselineVerifyFlag  = "XYLONA_VERIFY_7DTD_BASELINE"
	sevenDaysToDieBaselineRoot        = "testdata/seven-days-to-die/v2.6-build-22422094"
	sevenDaysToDieFixtureUserID       = "EOS_00000000000000000000000000000000"
)

var (
	sevenDaysToDieOpenAPIReferencePattern = regexp.MustCompile(`\./([^'#/]+\.yaml)#`)
	sevenDaysToDieIPv4Pattern             = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	sevenDaysToDieSteamIDPattern          = regexp.MustCompile(`Steam_[0-9]+`)
	sevenDaysToDieLongIDPattern           = regexp.MustCompile(`\b[0-9]{15,}\b`)
	sevenDaysToDieHexIDPattern            = regexp.MustCompile(`\b[0-9a-fA-F]{24,}\b`)
	sevenDaysToDieWindowsPathPattern      = regexp.MustCompile(`[A-Za-z]:\\[^\r\n"']+`)
	sevenDaysToDieTrailingSpacePattern    = regexp.MustCompile(`(?m)[\t ]+$`)
	errSevenDaysToDieReadbackUnavailable  = errors.New("read-back transport unavailable")
)

type sevenDaysToDieBaselineCapture struct {
	baseURL     string
	tokenName   string
	tokenSecret string
	client      *http.Client
}

type sevenDaysToDieBaselineRoundTripper func(*http.Request) (*http.Response, error)

func (f sevenDaysToDieBaselineRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type sevenDaysToDieBaselineExchange struct {
	Request  sevenDaysToDieBaselineRequest  `json:"request"`
	Response sevenDaysToDieBaselineResponse `json:"response"`
}

type sevenDaysToDieBaselineRequest struct {
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	HeaderNames []string       `json:"headerNames"`
	Body        map[string]any `json:"body,omitzero"`
}

type sevenDaysToDieBaselineResponse struct {
	Status int `json:"status"`
	Body   any `json:"body,omitzero"`
}

func TestIntegrationCaptureSevenDaysToDieManagementBaseline(t *testing.T) {
	if os.Getenv(sevenDaysToDieBaselineCaptureFlag) == "" {
		t.Skip("set XYLONA_CAPTURE_7DTD_BASELINE=1 to capture the live management baseline")
	}

	baseURL := strings.TrimRight(os.Getenv("XYLONA_7DTD_BASE_URL"), "/")
	tokenName := os.Getenv("XYLONA_7DTD_TOKEN_NAME")
	tokenSecret := os.Getenv("XYLONA_7DTD_TOKEN_SECRET")
	if baseURL == "" || tokenName == "" || tokenSecret == "" {
		t.Fatal("live baseline capture requires base URL and dashboard token environment variables")
	}

	capture := &sevenDaysToDieBaselineCapture{
		baseURL:     baseURL,
		tokenName:   tokenName,
		tokenSecret: tokenSecret,
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{DisableKeepAlives: true},
		},
	}
	root := filepath.FromSlash(sevenDaysToDieBaselineRoot)
	errMkdir := os.MkdirAll(root, 0o750)
	if errMkdir != nil {
		t.Fatalf("create baseline directory: %v", errMkdir)
	}

	master := capture.get(t, "/api/openapi/openapi.yaml")
	master = []byte(capture.sanitizeOpenAPI(string(master)))
	writeSevenDaysToDieBaselineFile(t, filepath.Join(root, "openapi", "openapi.yaml"), master)
	for _, fileName := range sevenDaysToDieOpenAPIReferences(master) {
		body := capture.get(t, "/api/OpenAPI/"+url.PathEscape(fileName))
		body = []byte(capture.sanitizeOpenAPI(string(body)))
		writeSevenDaysToDieBaselineFile(t, filepath.Join(root, "openapi", fileName), body)
	}

	commandCatalog := capture.getJSON(t, "/api/command")
	commandNames := sortSevenDaysToDieCommands(t, commandCatalog)
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "commands", "catalog.json"), commandCatalog)
	commandDetails := capture.commandDetails(t, commandCatalog)
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "commands", "details.json"), commandDetails)

	players := capture.getJSON(t, "/api/player")
	sanitizeSevenDaysToDiePlayers(t, players)
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "players", "representative.json"), players)

	userPermissions := capture.getJSON(t, "/api/userpermissions")
	sanitizeSevenDaysToDieUserPermissions(t, userPermissions)
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "users-before.json"), userPermissions)
	commandPermissions := capture.getJSON(t, "/api/commandpermissions")
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "commands-before.json"), commandPermissions)

	capture.captureVersion(t, root)
	capture.captureUserPermissionCases(t, root)
	capture.captureCommandPermissionCases(t, root, commandPermissions)
	capture.captureTimeoutCase(t, root)
	capture.captureUnavailableReadbackCase(t, root)

	manifest := map[string]any{
		"game":                "7 Days to Die",
		"gameVersion":         "2.6",
		"gameBuild":           "b14",
		"release":             "V2.6 Stable",
		"buildId":             "22422094",
		"capturedAt":          "2026-08-28",
		"dashboardAPIVersion": "1.0.0",
		"commandCount":        len(commandNames),
		"source":              "supported local dedicated server",
		"files":               hashSevenDaysToDieBaselineFiles(t, root),
	}
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "manifest.json"), manifest)
}

func TestIntegrationSevenDaysToDieManagementBaselineDrift(t *testing.T) {
	if os.Getenv(sevenDaysToDieBaselineVerifyFlag) == "" {
		t.Skip("set XYLONA_VERIFY_7DTD_BASELINE=1 to compare a live command catalog with the baseline")
	}
	baseURL := strings.TrimRight(os.Getenv("XYLONA_7DTD_BASE_URL"), "/")
	tokenName := os.Getenv("XYLONA_7DTD_TOKEN_NAME")
	tokenSecret := os.Getenv("XYLONA_7DTD_TOKEN_SECRET")
	if baseURL == "" || tokenName == "" || tokenSecret == "" {
		t.Fatal("live baseline verification requires base URL and dashboard token environment variables")
	}
	capture := &sevenDaysToDieBaselineCapture{
		baseURL:     baseURL,
		tokenName:   tokenName,
		tokenSecret: tokenSecret,
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{DisableKeepAlives: true},
		},
	}
	catalog := capture.getJSON(t, "/api/command")
	sortSevenDaysToDieCommands(t, catalog)
	details := capture.commandDetails(t, catalog)
	baseline := readSevenDaysToDieBaselineJSON[map[string]any](
		t,
		filepath.Join(sevenDaysToDieBaselineRoot, "commands", "details.json"),
	)
	drift := detectSevenDaysToDieCommandDrift(baseline, details)
	if len(drift.newCommands) != 0 || len(drift.removedCommands) != 0 || len(drift.changedCommands) != 0 {
		t.Fatalf(
			"live command catalog drifted: new=%v removed=%v changed=%v",
			drift.newCommands,
			drift.removedCommands,
			drift.changedCommands,
		)
	}
	verifySevenDaysToDieLiveOperationsSmoke(t, capture)
}

func verifySevenDaysToDieLiveOperationsSmoke(t *testing.T, capture *sevenDaysToDieBaselineCapture) {
	t.Helper()
	status, response := capture.requestJSON(t, http.MethodPost, "/api/command", map[string]any{
		"command": "version",
		"format":  "Full",
	})
	responseRoot, okRoot := response.(map[string]any)
	responseData, okData := responseRoot["data"].(map[string]any)
	version, okVersion := responseData["result"].(string)
	if status != http.StatusOK || !okRoot || !okData || !okVersion || !strings.Contains(version, "Game version: V 2.6 (b14)") {
		t.Fatal("live command transport does not match the supported V2.6 build b14 baseline")
	}

	permissions := capture.getJSON(t, "/api/userpermissions")
	permissionsRoot, okRoot := permissions.(map[string]any)
	permissionsData, okData := permissionsRoot["data"].(map[string]any)
	_, okGroups := permissionsData["groups"].([]any)
	_, okUsers := permissionsData["users"].([]any)
	if !okRoot || !okData || !okGroups || !okUsers {
		t.Fatal("live native permission read-back does not match the supported baseline")
	}
}

func (c *sevenDaysToDieBaselineCapture) commandDetails(t *testing.T, catalog any) map[string]any {
	t.Helper()
	root, okRoot := catalog.(map[string]any)
	data, okData := root["data"].(map[string]any)
	commands, okCommands := data["commands"].([]any)
	if !okRoot || !okData || !okCommands {
		t.Fatal("captured command catalog has an invalid envelope")
	}
	details := make(map[string]any, len(commands))
	for _, rawCommand := range commands {
		command, okCommand := rawCommand.(map[string]any)
		name, okName := command["command"].(string)
		if !okCommand || !okName || name == "" {
			t.Fatal("captured command catalog contains an invalid command")
		}
		candidates := []string{name}
		overloads, _ := command["overloads"].([]any)
		for _, rawOverload := range overloads {
			overload, okOverload := rawOverload.(string)
			if okOverload && !slices.Contains(candidates, overload) {
				candidates = append(candidates, overload)
			}
		}
		for _, candidate := range candidates {
			status, body := c.request(t, http.MethodGet, "/api/command/"+url.PathEscape(candidate), nil)
			if status == http.StatusNotFound {
				continue
			}
			if status != http.StatusOK {
				t.Fatalf("GET command detail status = %d for %q", status, name)
			}
			details[name] = map[string]any{
				"requestName": candidate,
				"response":    decodeSevenDaysToDieBaselineJSON(t, body, c),
			}
			break
		}
		_, found := details[name]
		if !found {
			t.Fatalf("no live command detail route resolved for %q", name)
		}
	}
	return details
}

func (c *sevenDaysToDieBaselineCapture) captureVersion(t *testing.T, root string) {
	t.Helper()
	body := map[string]any{"command": "version", "format": "Full"}
	status, response := c.requestJSON(t, http.MethodPost, "/api/command", body)
	if status != http.StatusOK {
		t.Fatalf("version command status = %d", status)
	}
	responseRoot, okRoot := response.(map[string]any)
	responseData, okData := responseRoot["data"].(map[string]any)
	versionOutput, okOutput := responseData["result"].(string)
	if !okRoot || !okData || !okOutput || !strings.Contains(versionOutput, "Game version: V 2.6 (b14)") {
		t.Fatal("live version response does not match V2.6 build b14")
	}
	exchange := sevenDaysToDieBaselineExchange{
		Request:  capturedSevenDaysToDieBaselineRequest(http.MethodPost, "/api/command", body),
		Response: sevenDaysToDieBaselineResponse{Status: status, Body: response},
	}
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "results", "version.json"), exchange)
}

func (c *sevenDaysToDieBaselineCapture) captureUserPermissionCases(t *testing.T, root string) {
	t.Helper()

	fixturePath := "/api/userpermissions/user/" + url.PathEscape(sevenDaysToDieFixtureUserID)
	before := c.getJSON(t, "/api/userpermissions")
	if sevenDaysToDieUserPermissionExists(t, before, sevenDaysToDieFixtureUserID) {
		t.Fatal("fixture user already exists; refusing to replace it")
	}

	cleanupNeeded := true
	t.Cleanup(func() {
		if !cleanupNeeded {
			return
		}
		request, errRequest := http.NewRequestWithContext(context.Background(), http.MethodDelete, c.baseURL+fixturePath, nil)
		if errRequest != nil {
			t.Errorf("create fixture user permission cleanup request: %v", errRequest)
			return
		}
		c.addHeaders(request)
		response, errDo := c.client.Do(request)
		if errDo != nil {
			t.Errorf("clean up fixture user permission: %v", errDo)
			return
		}
		errDrain := drainAndCloseSevenDaysToDieBaselineResponse(response)
		if errDrain != nil {
			t.Errorf("drain fixture user permission cleanup response: %v", errDrain)
		}
		if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusNotFound {
			t.Errorf("fixture user permission cleanup status = %d", response.StatusCode)
		}
	})

	createBody := map[string]any{"permissionLevel": 0, "name": "Fixture Player"}
	statusCreate, responseCreate := c.requestJSON(t, http.MethodPost, fixturePath, createBody)
	if statusCreate != http.StatusCreated {
		t.Fatalf("create fixture user permission status = %d", statusCreate)
	}
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "user-create.json"), sevenDaysToDieBaselineExchange{
		Request: capturedSevenDaysToDieBaselineRequest(
			http.MethodPost,
			"/api/userpermissions/user/{playerId}",
			createBody,
		),
		Response: sevenDaysToDieBaselineResponse{Status: statusCreate, Body: responseCreate},
	})

	readback := c.waitForUserPermission(t, sevenDaysToDieFixtureUserID, true)
	sanitizeSevenDaysToDieUserPermissions(t, readback)
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "user-readback.json"), readback)
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "results", "add-administrator-confirmed.json"), map[string]any{
		"classification": "confirmed",
		"execution":      sevenDaysToDieBaselineResponse{Status: statusCreate, Body: responseCreate},
		"readBack":       readback,
	})

	rejectBody := map[string]any{"permissionLevel": "invalid"}
	statusReject, responseReject := c.requestJSON(t, http.MethodPost, "/api/userpermissions/user/invalid", rejectBody)
	if statusReject != http.StatusBadRequest {
		t.Fatalf("invalid user permission status = %d, want 400", statusReject)
	}
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "results", "rejection.json"), sevenDaysToDieBaselineExchange{
		Request: capturedSevenDaysToDieBaselineRequest(
			http.MethodPost,
			"/api/userpermissions/user/{invalidPlayerId}",
			rejectBody,
		),
		Response: sevenDaysToDieBaselineResponse{Status: statusReject, Body: responseReject},
	})

	statusDelete, responseDelete := c.requestJSON(t, http.MethodDelete, fixturePath, nil)
	if statusDelete != http.StatusNoContent {
		t.Fatalf("delete fixture user permission status = %d", statusDelete)
	}
	cleanupNeeded = false
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "user-delete.json"), sevenDaysToDieBaselineExchange{
		Request: capturedSevenDaysToDieBaselineRequest(
			http.MethodDelete,
			"/api/userpermissions/user/{playerId}",
			nil,
		),
		Response: sevenDaysToDieBaselineResponse{Status: statusDelete, Body: responseDelete},
	})

	after := c.waitForUserPermission(t, sevenDaysToDieFixtureUserID, false)
	sanitizeSevenDaysToDieUserPermissions(t, after)
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "users-after-delete.json"), after)
}

func (c *sevenDaysToDieBaselineCapture) captureCommandPermissionCases(t *testing.T, root string, permissions any) {
	t.Helper()

	command, originalLevel, originalDefault := sevenDaysToDieCommandPermission(t, permissions, "version")
	permissionLevel := originalLevel - 1
	if permissionLevel < 0 {
		permissionLevel = originalLevel + 1
	}
	fixturePath := "/api/commandpermissions/" + url.PathEscape(command)
	body := map[string]any{"permissionLevel": permissionLevel}
	cleanupNeeded := true
	t.Cleanup(func() {
		if !cleanupNeeded {
			return
		}
		restoreBody, errEncode := json.Marshal(map[string]any{"permissionLevel": originalLevel})
		if errEncode != nil {
			t.Errorf("encode command permission restore: %v", errEncode)
			return
		}
		request, errRequest := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			c.baseURL+fixturePath,
			bytes.NewReader(restoreBody),
		)
		if errRequest != nil {
			t.Errorf("create command permission cleanup request: %v", errRequest)
			return
		}
		c.addHeaders(request)
		request.Header.Set("Content-Type", "application/json")
		response, errDo := c.client.Do(request)
		if errDo != nil {
			t.Errorf("clean up command permission override: %v", errDo)
			return
		}
		errDrain := drainAndCloseSevenDaysToDieBaselineResponse(response)
		if errDrain != nil {
			t.Errorf("drain command permission cleanup response: %v", errDrain)
		}
		if response.StatusCode != http.StatusCreated {
			t.Errorf("command permission cleanup status = %d", response.StatusCode)
		}
	})
	statusCreate, responseCreate := c.requestJSON(t, http.MethodPost, fixturePath, body)
	if statusCreate != http.StatusCreated {
		t.Fatalf("create command permission override status = %d", statusCreate)
	}
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "command-create.json"), sevenDaysToDieBaselineExchange{
		Request: capturedSevenDaysToDieBaselineRequest(
			http.MethodPost,
			"/api/commandpermissions/{command}",
			body,
		),
		Response: sevenDaysToDieBaselineResponse{Status: statusCreate, Body: responseCreate},
	})

	readback := c.waitForCommandPermission(t, command, permissionLevel, false)
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "command-readback.json"), readback)

	statusDelete, responseDelete := c.requestJSON(t, http.MethodDelete, fixturePath, nil)
	if statusDelete != http.StatusNoContent {
		t.Fatalf("delete command permission override status = %d", statusDelete)
	}
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "command-delete.json"), sevenDaysToDieBaselineExchange{
		Request: capturedSevenDaysToDieBaselineRequest(
			http.MethodDelete,
			"/api/commandpermissions/{command}",
			nil,
		),
		Response: sevenDaysToDieBaselineResponse{Status: statusDelete, Body: responseDelete},
	})
	after := c.waitForCommandPermission(t, command, originalLevel, originalDefault)
	cleanupNeeded = false
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "permissions", "commands-after-delete.json"), after)
}

func (c *sevenDaysToDieBaselineCapture) waitForUserPermission(t *testing.T, userID string, wanted bool) any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		readback := c.getJSON(t, "/api/userpermissions")
		if sevenDaysToDieUserPermissionExists(t, readback, userID) == wanted {
			return readback
		}
		if time.Now().After(deadline) {
			t.Fatalf("fixture user permission presence did not become %t", wanted)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (c *sevenDaysToDieBaselineCapture) waitForCommandPermission(
	t *testing.T,
	command string,
	permissionLevel int,
	isDefault bool,
) any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		readback := c.getJSON(t, "/api/commandpermissions")
		if sevenDaysToDieCommandPermissionMatches(readback, command, permissionLevel, isDefault) {
			return readback
		}
		if time.Now().After(deadline) {
			t.Fatal("command permission read-back did not match the requested state")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func drainAndCloseSevenDaysToDieBaselineResponse(response *http.Response) error {
	_, errDrain := io.Copy(io.Discard, response.Body)
	errClose := response.Body.Close()
	return errors.Join(errDrain, errClose)
}

func (c *sevenDaysToDieBaselineCapture) captureTimeoutCase(t *testing.T, root string) {
	t.Helper()

	client := &http.Client{Timeout: time.Nanosecond}
	request, errRequest := http.NewRequestWithContext(t.Context(), http.MethodGet, c.baseURL+"/api/command", nil)
	if errRequest != nil {
		t.Fatalf("create timeout request: %v", errRequest)
	}
	c.addHeaders(request)
	response, errDo := client.Do(request)
	if response != nil {
		errDrain := drainAndCloseSevenDaysToDieBaselineResponse(response)
		if errDrain != nil {
			t.Errorf("drain unexpected timeout response: %v", errDrain)
		}
	}
	if !errors.Is(errDo, context.DeadlineExceeded) {
		t.Fatalf("timeout request error type = %T", errDo)
	}
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "results", "timeout.json"), map[string]any{
		"request": capturedSevenDaysToDieBaselineRequest(http.MethodGet, "/api/command", nil),
		"outcome": "timeout",
		"source":  "transport-simulation",
	})
}

func (c *sevenDaysToDieBaselineCapture) captureUnavailableReadbackCase(t *testing.T, root string) {
	t.Helper()

	client := &http.Client{Transport: sevenDaysToDieBaselineRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errSevenDaysToDieReadbackUnavailable
	})}
	request, errRequest := http.NewRequestWithContext(t.Context(), http.MethodGet, c.baseURL+"/api/userpermissions", nil)
	if errRequest != nil {
		t.Fatalf("create unavailable read-back request: %v", errRequest)
	}
	c.addHeaders(request)
	response, errDo := client.Do(request)
	if response != nil {
		errDrain := drainAndCloseSevenDaysToDieBaselineResponse(response)
		if errDrain != nil {
			t.Errorf("drain unexpected unavailable read-back response: %v", errDrain)
		}
	}
	if !errors.Is(errDo, errSevenDaysToDieReadbackUnavailable) {
		t.Fatalf("unavailable read-back error type = %T", errDo)
	}
	writeSevenDaysToDieBaselineJSON(t, filepath.Join(root, "results", "unavailable-readback.json"), map[string]any{
		"request": capturedSevenDaysToDieBaselineRequest(http.MethodGet, "/api/userpermissions", nil),
		"outcome": "unavailable",
		"source":  "transport-simulation",
	})
}

func (c *sevenDaysToDieBaselineCapture) get(t *testing.T, path string) []byte {
	t.Helper()
	status, body := c.request(t, http.MethodGet, path, nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s status = %d", path, status)
	}
	return body
}

func (c *sevenDaysToDieBaselineCapture) getJSON(t *testing.T, path string) any {
	t.Helper()
	return decodeSevenDaysToDieBaselineJSON(t, c.get(t, path), c)
}

func (c *sevenDaysToDieBaselineCapture) requestJSON(t *testing.T, method string, path string, body map[string]any) (int, any) {
	t.Helper()
	var encoded []byte
	if body != nil {
		var errEncode error
		encoded, errEncode = json.Marshal(body)
		if errEncode != nil {
			t.Fatalf("encode %s %s body: %v", method, path, errEncode)
		}
	}
	status, response := c.request(t, method, path, encoded)
	if len(response) == 0 {
		return status, nil
	}
	return status, decodeSevenDaysToDieBaselineJSON(t, response, c)
}

func (c *sevenDaysToDieBaselineCapture) request(t *testing.T, method string, path string, body []byte) (int, []byte) {
	t.Helper()
	request, errRequest := http.NewRequestWithContext(t.Context(), method, c.baseURL+path, bytes.NewReader(body))
	if errRequest != nil {
		t.Fatalf("create %s %s request: %v", method, path, errRequest)
	}
	c.addHeaders(request)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, errDo := c.client.Do(request)
	if errDo != nil {
		t.Fatalf("%s %s failed with %T", method, path, errDo)
	}
	responseBody, errRead := io.ReadAll(io.LimitReader(response.Body, sevenDaysToDieWebAPIResponseLimit+1))
	errClose := response.Body.Close()
	if errRead != nil || errClose != nil {
		t.Fatalf("read %s %s response: %v", method, path, errors.Join(errRead, errClose))
	}
	if len(responseBody) > sevenDaysToDieWebAPIResponseLimit {
		t.Fatalf("%s %s response exceeds limit", method, path)
	}
	return response.StatusCode, responseBody
}

func (c *sevenDaysToDieBaselineCapture) addHeaders(request *http.Request) {
	request.Header.Set("Accept", "application/json, application/yaml, text/yaml")
	request.Header.Set("X-SDTD-API-TOKENNAME", c.tokenName)
	request.Header.Set("X-SDTD-API-SECRET", c.tokenSecret)
}

func (c *sevenDaysToDieBaselineCapture) sanitizeText(value string) string {
	if c.tokenName != "" {
		value = strings.ReplaceAll(value, c.tokenName, "<redacted>")
	}
	if c.tokenSecret != "" {
		value = strings.ReplaceAll(value, c.tokenSecret, "<redacted>")
	}
	value = sevenDaysToDieWindowsPathPattern.ReplaceAllString(value, "<path>")
	value = sevenDaysToDieIPv4Pattern.ReplaceAllString(value, "192.0.2.1")
	value = sevenDaysToDieSteamIDPattern.ReplaceAllString(value, "Steam_PLAYER_1")
	value = sevenDaysToDieLongIDPattern.ReplaceAllString(value, "00000000000000000")
	value = sevenDaysToDieHexIDPattern.ReplaceAllString(value, "PLAYER_ID")
	value = strings.ReplaceAll(value, "TheFunPimp", "Fixture Player")
	return value
}

func (c *sevenDaysToDieBaselineCapture) sanitizeOpenAPI(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return sevenDaysToDieTrailingSpacePattern.ReplaceAllString(c.sanitizeText(value), "")
}

func capturedSevenDaysToDieBaselineRequest(method string, path string, body map[string]any) sevenDaysToDieBaselineRequest {
	headerNames := []string{"Accept", "X-SDTD-API-SECRET", "X-SDTD-API-TOKENNAME"}
	if body != nil {
		headerNames = slices.Insert(headerNames, 1, "Content-Type")
	}
	return sevenDaysToDieBaselineRequest{
		Method:      method,
		Path:        path,
		HeaderNames: headerNames,
		Body:        body,
	}
}

func decodeSevenDaysToDieBaselineJSON(t *testing.T, data []byte, capture *sevenDaysToDieBaselineCapture) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	errDecode := decoder.Decode(&value)
	if errDecode != nil {
		t.Fatalf("decode captured JSON: %v", errDecode)
	}
	if capture != nil {
		return normalizeSevenDaysToDieBaselineJSON(value, capture)
	}
	return value
}

func normalizeSevenDaysToDieBaselineJSON(value any, capture *sevenDaysToDieBaselineCapture) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "serverTime" {
				typed[key] = "2000-01-01T00:00:00Z"
				continue
			}
			typed[key] = normalizeSevenDaysToDieBaselineJSON(child, capture)
		}
	case []any:
		for index, child := range typed {
			typed[index] = normalizeSevenDaysToDieBaselineJSON(child, capture)
		}
	case string:
		return capture.sanitizeText(typed)
	}
	return value
}

func sevenDaysToDieOpenAPIReferences(master []byte) []string {
	references := sevenDaysToDieOpenAPIReferencePattern.FindAllSubmatch(master, -1)
	fileNames := make([]string, 0, len(references))
	for _, reference := range references {
		fileNames = append(fileNames, string(reference[1]))
	}
	slices.Sort(fileNames)
	return slices.Compact(fileNames)
}

func sortSevenDaysToDieCommands(t *testing.T, catalog any) []string {
	t.Helper()
	root, okRoot := catalog.(map[string]any)
	data, okData := root["data"].(map[string]any)
	commands, okCommands := data["commands"].([]any)
	if !okRoot || !okData || !okCommands {
		t.Fatal("captured command catalog has an invalid envelope")
	}
	slices.SortFunc(commands, func(left any, right any) int {
		leftCommand, _ := left.(map[string]any)["command"].(string)
		rightCommand, _ := right.(map[string]any)["command"].(string)
		return strings.Compare(leftCommand, rightCommand)
	})
	result := make([]string, 0, len(commands))
	for _, rawCommand := range commands {
		command, okCommand := rawCommand.(map[string]any)
		name, okName := command["command"].(string)
		if !okCommand || !okName || name == "" {
			t.Fatal("captured command catalog contains an invalid command")
		}
		result = append(result, name)
	}
	return result
}

func sanitizeSevenDaysToDiePlayers(t *testing.T, envelope any) {
	t.Helper()
	root, okRoot := envelope.(map[string]any)
	data, okData := root["data"].(map[string]any)
	players, okPlayers := data["players"].([]any)
	if !okRoot || !okData || !okPlayers {
		t.Fatal("captured Player response has an invalid envelope")
	}
	for index, rawPlayer := range players {
		player, okPlayer := rawPlayer.(map[string]any)
		if !okPlayer {
			t.Fatal("captured Player response contains an invalid Player")
		}
		fixtureIndex := index + 1
		player["entityId"] = json.Number(fmt.Sprintf("%d", fixtureIndex))
		player["name"] = fmt.Sprintf("Player %d", fixtureIndex)
		player["platformId"] = sanitizeSevenDaysToDieUserID(player["platformId"], fixtureIndex)
		player["crossplatformId"] = sanitizeSevenDaysToDieUserID(player["crossplatformId"], fixtureIndex)
		player["ip"] = nil
		player["position"] = nil
		player["ping"] = nil
		player["health"] = json.Number("0")
		player["stamina"] = json.Number("0")
		player["score"] = json.Number("0")
		player["deaths"] = json.Number("0")
		player["kills"] = map[string]any{"players": json.Number("0"), "zombies": json.Number("0")}
		player["banned"] = map[string]any{"banActive": false, "reason": nil, "until": nil}
	}
}

func sanitizeSevenDaysToDieUserPermissions(t *testing.T, envelope any) {
	t.Helper()
	root, okRoot := envelope.(map[string]any)
	data, okData := root["data"].(map[string]any)
	users, okUsers := data["users"].([]any)
	groups, okGroups := data["groups"].([]any)
	if !okRoot || !okData || !okUsers || !okGroups {
		t.Fatal("captured user permission response has an invalid envelope")
	}
	for index, rawUser := range users {
		user, okUser := rawUser.(map[string]any)
		if !okUser {
			t.Fatal("captured user permission response contains an invalid user")
		}
		user["name"] = fmt.Sprintf("Fixture Player %d", index+1)
		user["userId"] = sanitizeSevenDaysToDieUserID(user["userId"], index+1)
	}
	for index, rawGroup := range groups {
		group, okGroup := rawGroup.(map[string]any)
		if !okGroup {
			t.Fatal("captured user permission response contains an invalid group")
		}
		group["name"] = fmt.Sprintf("Fixture Group %d", index+1)
		group["groupId"] = fmt.Sprintf("GROUP_%d", index+1)
	}
}

func sanitizeSevenDaysToDieUserID(value any, index int) any {
	identifier, okIdentifier := value.(map[string]any)
	if !okIdentifier {
		return nil
	}
	platform, _ := identifier["platformId"].(string)
	if platform == "" {
		platform = "Platform"
	}
	return map[string]any{
		"combinedString": fmt.Sprintf("%s_PLAYER_%d", platform, index),
		"platformId":     platform,
		"userId":         fmt.Sprintf("PLAYER_%d", index),
	}
}

func sevenDaysToDieUserPermissionExists(t *testing.T, envelope any, userID string) bool {
	t.Helper()
	root, okRoot := envelope.(map[string]any)
	data, okData := root["data"].(map[string]any)
	users, okUsers := data["users"].([]any)
	if !okRoot || !okData || !okUsers {
		t.Fatal("captured user permission response has an invalid envelope")
	}
	for _, rawUser := range users {
		user, okUser := rawUser.(map[string]any)
		identifier, okIdentifier := user["userId"].(map[string]any)
		combined, _ := identifier["combinedString"].(string)
		if okUser && okIdentifier && combined == userID {
			return true
		}
	}
	return false
}

func sevenDaysToDieCommandPermission(t *testing.T, envelope any, wanted string) (string, int, bool) {
	t.Helper()
	root, okRoot := envelope.(map[string]any)
	permissions, okPermissions := root["data"].([]any)
	if !okRoot || !okPermissions {
		t.Fatal("captured command permission response has an invalid envelope")
	}
	for _, rawPermission := range permissions {
		permission, okPermission := rawPermission.(map[string]any)
		command, _ := permission["command"].(string)
		isDefault, _ := permission["default"].(bool)
		level, okLevel := permission["permissionLevel"].(json.Number)
		if okPermission && command == wanted && okLevel {
			parsedLevel, errParse := strconv.Atoi(level.String())
			if errParse != nil {
				t.Fatalf("parse %q default permission: %v", wanted, errParse)
			}
			return command, parsedLevel, isDefault
		}
	}
	t.Fatalf("command %q does not have a default permission", wanted)
	return "", 0, false
}

func sevenDaysToDieCommandPermissionMatches(envelope any, wanted string, level int, isDefault bool) bool {
	root, okRoot := envelope.(map[string]any)
	permissions, okPermissions := root["data"].([]any)
	if !okRoot || !okPermissions {
		return false
	}
	for _, rawPermission := range permissions {
		permission, okPermission := rawPermission.(map[string]any)
		command, _ := permission["command"].(string)
		permissionDefault, _ := permission["default"].(bool)
		permissionNumber, okLevel := permission["permissionLevel"].(json.Number)
		if !okPermission || command != wanted || permissionDefault != isDefault || !okLevel {
			continue
		}
		permissionLevel, errParse := strconv.Atoi(permissionNumber.String())
		return errParse == nil && permissionLevel == level
	}
	return false
}

func writeSevenDaysToDieBaselineJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, errMarshal := json.MarshalIndent(value, "", "  ")
	if errMarshal != nil {
		t.Fatalf("encode baseline fixture %s: %v", filepath.Base(path), errMarshal)
	}
	data = append(data, '\n')
	writeSevenDaysToDieBaselineFile(t, path, data)
}

func writeSevenDaysToDieBaselineFile(t *testing.T, path string, data []byte) {
	t.Helper()
	errMkdir := os.MkdirAll(filepath.Dir(path), 0o750)
	if errMkdir != nil {
		t.Fatalf("create baseline fixture directory: %v", errMkdir)
	}
	errWrite := os.WriteFile(path, data, 0o600)
	if errWrite != nil {
		t.Fatalf("write baseline fixture %s: %v", filepath.Base(path), errWrite)
	}
}

func hashSevenDaysToDieBaselineFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	rootDirectory, errOpen := os.OpenRoot(root)
	if errOpen != nil {
		t.Fatalf("open baseline root for hashing: %v", errOpen)
	}
	t.Cleanup(func() {
		errClose := rootDirectory.Close()
		if errClose != nil {
			t.Errorf("close baseline root after hashing: %v", errClose)
		}
	})
	hashes := make(map[string]string)
	errWalk := fs.WalkDir(rootDirectory.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk baseline fixture: %w", err)
		}
		if entry.IsDir() || filepath.Base(path) == "manifest.json" {
			return nil
		}
		data, errRead := rootDirectory.ReadFile(path)
		if errRead != nil {
			return fmt.Errorf("read baseline fixture: %w", errRead)
		}
		digest := sha256.Sum256(data)
		hashes[filepath.ToSlash(path)] = hex.EncodeToString(digest[:])
		return nil
	})
	if errWalk != nil {
		t.Fatalf("hash baseline fixtures: %v", errWalk)
	}
	return hashes
}
