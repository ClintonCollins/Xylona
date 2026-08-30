package node

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
)

const sevenDaysToDieManagementBaselineRoot = "testdata/seven-days-to-die/v2.6-build-22422094"

var (
	baselineIPv4Pattern        = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	baselineWindowsPathPattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])[a-z]:\\[^nrt"\\]`)
	baselineSteamIDPattern     = regexp.MustCompile(`Steam_[0-9]{10,}`)
	baselineEOSIDPattern       = regexp.MustCompile(`EOS_[0-9a-fA-F]{24,}`)
)

type sevenDaysToDieManagementManifest struct {
	Game                string            `json:"game"`
	GameVersion         string            `json:"gameVersion"`
	GameBuild           string            `json:"gameBuild"`
	Release             string            `json:"release"`
	BuildID             string            `json:"buildId"`
	DashboardAPIVersion string            `json:"dashboardAPIVersion"`
	CommandCount        int               `json:"commandCount"`
	Files               map[string]string `json:"files"`
}

type sevenDaysToDieManagementInventory struct {
	BaselineCommandCount   int                                      `json:"baselineCommandCount"`
	BaselineOperationCount int                                      `json:"baselineOperationCount"`
	Supported              []sevenDaysToDieManagementInventoryGroup `json:"supported"`
	Excluded               []sevenDaysToDieManagementInventoryGroup `json:"excluded"`
}

type sevenDaysToDieManagementInventoryGroup struct {
	Category    string   `json:"category"`
	Reason      string   `json:"reason"`
	EscapeHatch string   `json:"escapeHatch"`
	Commands    []string `json:"commands"`
	Operations  []string `json:"operations"`
}

type sevenDaysToDieOpenAPIDocument struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

type sevenDaysToDieOperationCoverage struct {
	Commands  []sevenDaysToDieOperationCoverageEntry `json:"commands"`
	NativeAPI []sevenDaysToDieOperationCoverageEntry `json:"nativeApi"`
}

type sevenDaysToDieOperationCoverageEntry struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	OperationIDs []string `json:"operationIds"`
	Reason       string   `json:"reason"`
	Alternative  string   `json:"alternative"`
}

type sevenDaysToDieCommandDrift struct {
	newCommands     []string
	removedCommands []string
	changedCommands []string
}

func TestSevenDaysToDieManagementBaseline(t *testing.T) {
	root := filepath.FromSlash(sevenDaysToDieManagementBaselineRoot)
	manifest := readSevenDaysToDieBaselineJSON[sevenDaysToDieManagementManifest](t, filepath.Join(root, "manifest.json"))
	if manifest.Game != "7 Days to Die" || manifest.GameVersion != "2.6" || manifest.GameBuild != "b14" ||
		manifest.Release != "V2.6 Stable" ||
		manifest.BuildID != "22422094" || manifest.DashboardAPIVersion != "1.0.0" {
		t.Fatalf("unexpected management baseline identity: %+v", manifest)
	}

	t.Run("fixtures are complete and sanitized", func(t *testing.T) {
		validateSevenDaysToDieBaselineFiles(t, root, manifest.Files)
	})
	t.Run("OpenAPI operations have an exhaustive inventory", func(t *testing.T) {
		operations := sevenDaysToDieBaselineOpenAPIOperations(t, root)
		inventory := readSevenDaysToDieBaselineJSON[sevenDaysToDieManagementInventory](t, filepath.Join(root, "inventory", "native-api.json"))
		if inventory.BaselineOperationCount != len(operations) {
			t.Fatalf("native API inventory count = %d, OpenAPI operations = %d", inventory.BaselineOperationCount, len(operations))
		}
		validateSevenDaysToDieOperationInventory(t, operations, inventory, false)
	})
	t.Run("command help has an exhaustive categorized inventory", func(t *testing.T) {
		catalog := readSevenDaysToDieBaselineJSON[struct {
			Data struct {
				Commands []struct {
					Command   string          `json:"command"`
					Help      json.RawMessage `json:"help"`
					Overloads []string        `json:"overloads"`
				} `json:"commands"`
			} `json:"data"`
		}](t, filepath.Join(root, "commands", "catalog.json"))
		if len(catalog.Data.Commands) != manifest.CommandCount {
			t.Fatalf("command catalog count = %d, manifest = %d", len(catalog.Data.Commands), manifest.CommandCount)
		}
		commands := make([]string, 0, len(catalog.Data.Commands))
		for _, command := range catalog.Data.Commands {
			if command.Command == "" || len(command.Help) == 0 || len(command.Overloads) == 0 {
				t.Fatalf("command %q is missing catalog, overload, or help evidence", command.Command)
			}
			commands = append(commands, command.Command)
		}
		details := readSevenDaysToDieBaselineJSON[map[string]struct {
			RequestName string          `json:"requestName"`
			Response    json.RawMessage `json:"response"`
		}](t, filepath.Join(root, "commands", "details.json"))
		if len(details) != len(commands) {
			t.Fatalf("command detail count = %d, catalog = %d", len(details), len(commands))
		}
		for _, command := range commands {
			detail, found := details[command]
			if !found || detail.RequestName == "" || len(detail.Response) == 0 {
				t.Fatalf("command %q has no captured live detail response", command)
			}
		}
		inventory := readSevenDaysToDieBaselineJSON[sevenDaysToDieManagementInventory](t, filepath.Join(root, "inventory", "commands.json"))
		if inventory.BaselineCommandCount != len(commands) {
			t.Fatalf("command inventory count = %d, catalog = %d", inventory.BaselineCommandCount, len(commands))
		}
		validateSevenDaysToDieOperationInventory(t, commands, inventory, true)
	})
	t.Run("approved Server control commands are modeled", func(t *testing.T) {
		inventory := readSevenDaysToDieBaselineJSON[sevenDaysToDieManagementInventory](t, filepath.Join(root, "inventory", "commands.json"))
		var approvedCommands []string
		for _, group := range inventory.Supported {
			if group.Category == "Server control" {
				approvedCommands = group.Commands
				break
			}
		}
		modeled := map[string]string{
			"saveworld":   gameintegrations.OperationIDSaveWorld,
			"settempunit": gameintegrations.OperationIDSetTemperatureUnit,
			"settime":     gameintegrations.OperationIDSetGameTime,
			"shutdown":    gameintegrations.OperationIDShutdown,
		}
		operations := gameintegrations.OperationsForGame("7_days_to_die")
		if len(approvedCommands) != len(modeled) {
			t.Fatalf("approved Server control command count = %d, modeled = %d", len(approvedCommands), len(modeled))
		}
		for command, operationID := range modeled {
			if !slices.Contains(approvedCommands, command) ||
				!slices.ContainsFunc(operations, func(operation gameintegrations.OperationDescriptor) bool {
					return operation.ID == operationID
				}) {
				t.Fatalf("approved Server control command %q is not modeled as %q", command, operationID)
			}
		}
	})
	t.Run("approved management inventory is modeled or explicitly excluded", func(t *testing.T) {
		coverage := readSevenDaysToDieBaselineJSON[sevenDaysToDieOperationCoverage](
			t,
			filepath.Join("testdata", "seven-days-to-die", "operations-coverage.json"),
		)
		commandInventory := readSevenDaysToDieBaselineJSON[sevenDaysToDieManagementInventory](t, filepath.Join(root, "inventory", "commands.json"))
		nativeInventory := readSevenDaysToDieBaselineJSON[sevenDaysToDieManagementInventory](t, filepath.Join(root, "inventory", "native-api.json"))
		wantCommands := sevenDaysToDieManagementEntries(
			commandInventory,
			true,
			"Player access",
			"Players",
			"Communication",
			"Server information",
			"Server control",
			"Player assistance",
			"World events",
		)
		wantNativeAPI := sevenDaysToDieManagementEntries(
			nativeInventory,
			false,
			"Console",
			"Player access",
			"Players",
			"Server state",
		)
		operationIDs := make(map[string]struct{})
		for _, operation := range gameintegrations.OperationsForGame("7_days_to_die") {
			operationIDs[operation.ID] = struct{}{}
		}
		coveredOperationIDs := make(map[string]struct{})
		validateSevenDaysToDieOperationCoverage(t, wantCommands, coverage.Commands, operationIDs, coveredOperationIDs)
		validateSevenDaysToDieOperationCoverage(t, wantNativeAPI, coverage.NativeAPI, operationIDs, coveredOperationIDs)
		for operationID := range operationIDs {
			_, covered := coveredOperationIDs[operationID]
			if !covered {
				t.Fatalf("operation %q is missing from the approved baseline coverage", operationID)
			}
		}
	})
	t.Run("Player identities are representative and neutral", func(t *testing.T) {
		players := readSevenDaysToDieBaselineJSON[struct {
			Data struct {
				Players []map[string]any `json:"players"`
			} `json:"data"`
		}](t, filepath.Join(root, "players", "representative.json"))
		if len(players.Data.Players) == 0 {
			t.Fatal("Player fixture has no live Player response")
		}
		platforms := make(map[string]struct{})
		for index, player := range players.Data.Players {
			wantName := fmt.Sprintf("Player %d", index+1)
			if player["name"] != wantName || player["ip"] != nil || player["position"] != nil {
				t.Fatalf("Player %d is not deterministically sanitized", index+1)
			}
			for _, key := range []string{"platformId", "crossplatformId"} {
				identity, okIdentity := player[key].(map[string]any)
				if !okIdentity {
					continue
				}
				platform, _ := identity["platformId"].(string)
				combined, _ := identity["combinedString"].(string)
				if platform == "" || !strings.Contains(combined, "_PLAYER_") {
					t.Fatalf("Player %d %s is not a neutral platform identity", index+1, key)
				}
				platforms[platform] = struct{}{}
			}
		}
		if len(platforms) < 2 {
			t.Fatalf("Player fixture contains %d platform identity form, want at least 2 when available", len(platforms))
		}
	})
	t.Run("permission and result evidence identifies Add administrator", func(t *testing.T) {
		validateSevenDaysToDiePermissionEvidence(t, root)
	})
	t.Run("command drift identifies new removed and changed commands", func(t *testing.T) {
		baseline := readSevenDaysToDieBaselineJSON[map[string]any](t, filepath.Join(root, "commands", "details.json"))
		tests := []struct {
			name   string
			mutate func(map[string]any)
			want   sevenDaysToDieCommandDrift
		}{
			{
				name: "new command",
				mutate: func(live map[string]any) {
					live["future-command"] = map[string]any{"response": "new"}
				},
				want: sevenDaysToDieCommandDrift{newCommands: []string{"future-command"}},
			},
			{
				name: "removed command",
				mutate: func(live map[string]any) {
					delete(live, "version")
				},
				want: sevenDaysToDieCommandDrift{removedCommands: []string{"version"}},
			},
			{
				name: "changed command",
				mutate: func(live map[string]any) {
					live["version"] = map[string]any{"response": "changed"}
				},
				want: sevenDaysToDieCommandDrift{changedCommands: []string{"version"}},
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				live := maps.Clone(baseline)
				test.mutate(live)
				if drift := detectSevenDaysToDieCommandDrift(baseline, live); !reflect.DeepEqual(drift, test.want) {
					t.Fatalf("command drift = %+v, want %+v", drift, test.want)
				}
			})
		}
	})
}

func detectSevenDaysToDieCommandDrift(baseline map[string]any, live map[string]any) sevenDaysToDieCommandDrift {
	var drift sevenDaysToDieCommandDrift
	for command, baselineDetail := range baseline {
		liveDetail, found := live[command]
		if !found {
			drift.removedCommands = append(drift.removedCommands, command)
			continue
		}
		if !reflect.DeepEqual(baselineDetail, liveDetail) {
			drift.changedCommands = append(drift.changedCommands, command)
		}
	}
	for command := range live {
		_, found := baseline[command]
		if !found {
			drift.newCommands = append(drift.newCommands, command)
		}
	}
	slices.Sort(drift.newCommands)
	slices.Sort(drift.removedCommands)
	slices.Sort(drift.changedCommands)
	return drift
}

func sevenDaysToDieManagementEntries(
	inventory sevenDaysToDieManagementInventory,
	commands bool,
	categories ...string,
) []string {
	entries := make([]string, 0)
	for _, group := range inventory.Supported {
		if !slices.Contains(categories, group.Category) {
			continue
		}
		if commands {
			entries = append(entries, group.Commands...)
		} else {
			entries = append(entries, group.Operations...)
		}
	}
	slices.Sort(entries)
	return entries
}

func validateSevenDaysToDieOperationCoverage(
	t *testing.T,
	want []string,
	coverage []sevenDaysToDieOperationCoverageEntry,
	operationIDs map[string]struct{},
	coveredOperationIDs map[string]struct{},
) {
	t.Helper()
	got := make([]string, 0, len(coverage))
	for _, entry := range coverage {
		got = append(got, entry.Name)
		switch entry.Status {
		case "modeled", "supporting":
			if len(entry.OperationIDs) == 0 || entry.Reason != "" || entry.Alternative != "" {
				t.Fatalf("coverage entry %q has incomplete %s metadata", entry.Name, entry.Status)
			}
			for _, operationID := range entry.OperationIDs {
				_, found := operationIDs[operationID]
				if !found {
					t.Fatalf("coverage entry %q references unknown operation %q", entry.Name, operationID)
				}
				coveredOperationIDs[operationID] = struct{}{}
			}
		case "excluded":
			if len(entry.OperationIDs) != 0 || entry.Reason == "" || entry.Alternative == "" {
				t.Fatalf("coverage entry %q has incomplete exclusion metadata", entry.Name)
			}
		default:
			t.Fatalf("coverage entry %q has unknown status %q", entry.Name, entry.Status)
		}
	}
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("approved management coverage mismatch: want=%v got=%v", want, got)
	}
}

func validateSevenDaysToDieBaselineFiles(t *testing.T, root string, hashes map[string]string) {
	t.Helper()
	rootDirectory, errOpen := os.OpenRoot(root)
	if errOpen != nil {
		t.Fatalf("open management baseline root: %v", errOpen)
	}
	t.Cleanup(func() {
		errClose := rootDirectory.Close()
		if errClose != nil {
			t.Errorf("close management baseline root: %v", errClose)
		}
	})
	found := make(map[string]string)
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
		validateSevenDaysToDieBaselineSanitization(t, path, data)
		digest := sha256.Sum256(data)
		found[filepath.ToSlash(path)] = hex.EncodeToString(digest[:])
		return nil
	})
	if errWalk != nil {
		t.Fatalf("walk management baseline: %v", errWalk)
	}
	if !maps.Equal(found, hashes) {
		t.Fatal("management baseline file hashes do not match manifest")
	}
}

func validateSevenDaysToDieBaselineSanitization(t *testing.T, path string, data []byte) {
	t.Helper()
	text := string(data)
	for _, address := range baselineIPv4Pattern.FindAllString(text, -1) {
		if address != "192.0.2.1" {
			t.Fatalf("fixture %s contains a non-documentation IPv4 address", filepath.Base(path))
		}
	}
	if baselineWindowsPathPattern.MatchString(text) || baselineSteamIDPattern.MatchString(text) || baselineEOSIDPattern.MatchString(text) {
		t.Fatalf("fixture %s contains an unsanitized path or Player identifier", filepath.Base(path))
	}
}

func sevenDaysToDieBaselineOpenAPIOperations(t *testing.T, root string) []string {
	t.Helper()
	master := readSevenDaysToDieBaselineYAML(t, filepath.Join(root, "openapi", "openapi.yaml"))
	operations := make([]string, 0, len(master.Paths))
	for publicPath, pathItem := range master.Paths {
		ref, okRef := pathItem["$ref"].(string)
		if !okRef {
			t.Fatalf("OpenAPI path %s has no captured reference", publicPath)
		}
		parts := strings.SplitN(ref, "#", 2)
		if len(parts) != 2 {
			t.Fatalf("OpenAPI path %s has invalid reference", publicPath)
		}
		fragmentName := strings.TrimPrefix(parts[0], "./")
		fragment := readSevenDaysToDieBaselineYAML(t, filepath.Join(root, "openapi", fragmentName))
		fragmentPath := strings.TrimPrefix(parts[1], "/paths/")
		fragmentPath = strings.ReplaceAll(strings.ReplaceAll(fragmentPath, "~1", "/"), "~0", "~")
		fragmentItem, found := fragment.Paths[fragmentPath]
		if !found {
			t.Fatalf("OpenAPI reference for %s has no fragment path", publicPath)
		}
		for method := range fragmentItem {
			upperMethod := strings.ToUpper(method)
			if slices.Contains([]string{"DELETE", "GET", "PATCH", "POST", "PUT"}, upperMethod) {
				operations = append(operations, upperMethod+" "+publicPath)
			}
		}
	}
	slices.Sort(operations)
	return operations
}

func readSevenDaysToDieBaselineYAML(t *testing.T, path string) sevenDaysToDieOpenAPIDocument {
	t.Helper()
	data, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("read OpenAPI fixture %s: %v", filepath.Base(path), errRead)
	}
	var document sevenDaysToDieOpenAPIDocument
	errDecode := yaml.Unmarshal(data, &document)
	if errDecode != nil {
		t.Fatalf("decode OpenAPI fixture %s: %v", filepath.Base(path), errDecode)
	}
	return document
}

func validateSevenDaysToDieOperationInventory(t *testing.T, want []string, inventory sevenDaysToDieManagementInventory, commands bool) {
	t.Helper()
	covered := make([]string, 0, len(want))
	for _, group := range inventory.Supported {
		if group.Category == "" {
			t.Fatal("supported operation inventory group has no game-defined category")
		}
		if commands {
			covered = append(covered, group.Commands...)
		} else {
			covered = append(covered, group.Operations...)
		}
	}
	for _, group := range inventory.Excluded {
		if group.Reason == "" || group.EscapeHatch != "Console" {
			t.Fatal("excluded operation inventory group lacks a concrete reason or Console escape hatch")
		}
		if commands {
			covered = append(covered, group.Commands...)
		} else {
			covered = append(covered, group.Operations...)
		}
	}
	slices.Sort(want)
	slices.Sort(covered)
	if !slices.Equal(want, covered) {
		missing := make([]string, 0)
		unknown := make([]string, 0)
		for _, operation := range want {
			if !slices.Contains(covered, operation) {
				missing = append(missing, operation)
			}
		}
		for _, operation := range covered {
			if !slices.Contains(want, operation) {
				unknown = append(unknown, operation)
			}
		}
		t.Fatalf("operation inventory mismatch: missing=%v unknown=%v", missing, unknown)
	}
}

func validateSevenDaysToDiePermissionEvidence(t *testing.T, root string) {
	t.Helper()
	addAdministrator := readSevenDaysToDieBaselineJSON[struct {
		Operation string `json:"operation"`
		Category  string `json:"category"`
		Execution struct {
			Method                string `json:"method"`
			Path                  string `json:"path"`
			VerifiedSuccessStatus int    `json:"verifiedSuccessStatus"`
		} `json:"execution"`
		ReadBack struct {
			Method         string   `json:"method"`
			Path           string   `json:"path"`
			Match          []string `json:"match"`
			VerifiedResult string   `json:"verifiedResult"`
		} `json:"readBack"`
		UnavailableReadBack struct {
			VerifiedResult string `json:"verifiedResult"`
		} `json:"unavailableReadBack"`
	}](t, filepath.Join(root, "inventory", "add-administrator.json"))
	if addAdministrator.Operation != "Add administrator" || addAdministrator.Category == "" ||
		addAdministrator.Execution.Method != http.MethodPost || addAdministrator.Execution.Path != "/api/userpermissions/user/{id}" ||
		addAdministrator.Execution.VerifiedSuccessStatus != http.StatusCreated || addAdministrator.ReadBack.Method != http.MethodGet ||
		addAdministrator.ReadBack.Path != "/api/userpermissions" || len(addAdministrator.ReadBack.Match) == 0 ||
		addAdministrator.ReadBack.VerifiedResult != "Confirmed" ||
		addAdministrator.UnavailableReadBack.VerifiedResult != "Accepted but unverified" {
		t.Fatal("Add administrator inventory does not identify verified execution and read-back options")
	}

	cases := []struct {
		path   string
		check  func(map[string]any) bool
		result string
	}{
		{path: "results/version.json", check: func(value map[string]any) bool {
			return baselineJSONNumber(value, "response", "status") == "200"
		}, result: "successful command execution"},
		{path: "results/rejection.json", check: func(value map[string]any) bool {
			return baselineJSONNumber(value, "response", "status") == "400"
		}, result: "rejected permission request"},
		{path: "results/timeout.json", check: func(value map[string]any) bool {
			return value["outcome"] == "timeout" && value["source"] == "transport-simulation"
		}, result: "timeout simulation"},
		{path: "results/unavailable-readback.json", check: func(value map[string]any) bool {
			return value["outcome"] == "unavailable" && value["source"] == "transport-simulation"
		}, result: "unavailable read-back simulation"},
		{path: "results/add-administrator-confirmed.json", check: func(value map[string]any) bool {
			readBack, okReadBack := value["readBack"].(map[string]any)
			data, okData := readBack["data"].(map[string]any)
			users, okUsers := data["users"].([]any)
			return value["classification"] == "confirmed" && baselineJSONNumber(value, "execution", "status") == "201" &&
				okReadBack && okData && okUsers && len(users) > 0
		}, result: "confirmed Add administrator read-back"},
	}
	for _, testCase := range cases {
		value := readSevenDaysToDieBaselineJSON[map[string]any](t, filepath.Join(root, filepath.FromSlash(testCase.path)))
		if !testCase.check(value) {
			t.Fatalf("fixture %s does not contain %s evidence", testCase.path, testCase.result)
		}
	}

	for _, path := range []string{
		"permissions/user-create.json", "permissions/user-readback.json", "permissions/user-delete.json",
		"permissions/command-create.json", "permissions/command-readback.json", "permissions/command-delete.json",
	} {
		readSevenDaysToDieBaselineJSON[map[string]any](t, filepath.Join(root, filepath.FromSlash(path)))
	}
}

func baselineJSONNumber(value map[string]any, keys ...string) string {
	current := any(value)
	for _, key := range keys {
		object, okObject := current.(map[string]any)
		if !okObject {
			return ""
		}
		current = object[key]
	}
	number, _ := current.(json.Number)
	return number.String()
}

func readSevenDaysToDieBaselineJSON[T any](t *testing.T, path string) T {
	t.Helper()
	data, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("read baseline fixture %s: %v", filepath.Base(path), errRead)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value T
	errDecode := decoder.Decode(&value)
	if errDecode != nil {
		t.Fatalf("decode baseline fixture %s: %v", filepath.Base(path), errDecode)
	}
	return value
}
