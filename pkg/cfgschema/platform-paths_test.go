package cfgschema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/cfgparse"
)

func TestResolvePlatformPath(t *testing.T) {
	t.Parallel()

	entry := ConfigSchemaEntry{
		Path: "Pal/Saved/Config/LinuxServer/PalWorldSettings.ini",
		PlatformPaths: map[string]string{
			"windows": "Pal/Saved/Config/WindowsServer/PalWorldSettings.ini",
		},
	}
	tests := []struct {
		name     string
		platform string
		want     string
	}{
		{
			name:     "platform override",
			platform: " WINDOWS ",
			want:     "Pal/Saved/Config/WindowsServer/PalWorldSettings.ini",
		},
		{
			name:     "default path",
			platform: "linux",
			want:     "Pal/Saved/Config/LinuxServer/PalWorldSettings.ini",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := ResolvePlatformPath(entry, test.platform)
			if got != test.want {
				t.Fatalf("ResolvePlatformPath() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolvePlatformConfigSchemas(t *testing.T) {
	t.Parallel()

	schemasJSON := `[{
		"path":"Pal/Saved/Config/LinuxServer/PalWorldSettings.ini",
		"platform_paths":{"windows":"Pal/Saved/Config/WindowsServer/PalWorldSettings.ini"},
		"format":"palworld",
		"category":"Server",
		"generate_before_start":true,
		"managed_fields":{"ServerName":"game_server.server_name"},
		"schema":{"type":"object","properties":{"ServerName":{"type":"string","default":"Palworld"}}}
	}]`

	resolvedJSON, errResolve := ResolvePlatformConfigSchemas(schemasJSON, "windows")
	if errResolve != nil {
		t.Fatalf("ResolvePlatformConfigSchemas() error = %v", errResolve)
	}
	entries, errParse := ParseConfigSchemas(resolvedJSON)
	if errParse != nil {
		t.Fatalf("ParseConfigSchemas() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("resolved entry count = %d, want 1", len(entries))
	}
	if entries[0].Path != "Pal/Saved/Config/WindowsServer/PalWorldSettings.ini" {
		t.Fatalf("resolved path = %q", entries[0].Path)
	}
	if entries[0].Format != "palworld" || !entries[0].GenerateBeforeStart {
		t.Fatalf("resolved entry lost config metadata: %#v", entries[0])
	}
	if entries[0].ManagedFields["ServerName"] != "game_server.server_name" {
		t.Fatalf("resolved managed field = %q", entries[0].ManagedFields["ServerName"])
	}
}

func TestValidateConfigSchemasPlatformPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pathsJSON string
		wantError string
	}{
		{
			name:      "valid paths",
			pathsJSON: `{"windows":"Pal/Saved/Config/WindowsServer/PalWorldSettings.ini"}`,
		},
		{
			name:      "unsupported platform",
			pathsJSON: `{"plan9":"PalWorldSettings.ini"}`,
			wantError: "unsupported platform path",
		},
		{
			name:      "traversal",
			pathsJSON: `{"windows":"../PalWorldSettings.ini"}`,
			wantError: "must not contain '..' traversal",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := `[{"path":"PalWorldSettings.ini","platform_paths":` + test.pathsJSON +
				`,"format":"palworld","category":"Server","schema":{"type":"object","properties":{}}}]`
			errs := ValidateConfigSchemas(input)
			if test.wantError == "" {
				if len(errs) > 0 {
					t.Fatalf("ValidateConfigSchemas() errors = %v", errs)
				}
				return
			}
			if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), test.wantError) {
				t.Fatalf("ValidateConfigSchemas() errors = %v, want %q", errs, test.wantError)
			}
		})
	}
}

func TestValidateConfigSchemasEffectivePlatformPathCollisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name: "distinct effective paths",
			input: `[
				{"path":"linux-a.ini","platform_paths":{"windows":"windows-a.ini"},"format":"ini","category":"Server"},
				{"path":"linux-b.ini","platform_paths":{"windows":"windows-b.ini"},"format":"ini","category":"Server"}
			]`,
		},
		{
			name: "matching overrides",
			input: `[
				{"path":"linux-a.ini","platform_paths":{"windows":"shared.ini"},"format":"ini","category":"Server"},
				{"path":"linux-b.ini","platform_paths":{"windows":"shared.ini"},"format":"ini","category":"Server"}
			]`,
			wantError: true,
		},
		{
			name: "override matches another default",
			input: `[
				{"path":"linux-a.ini","platform_paths":{"windows":"server.ini"},"format":"ini","category":"Server"},
				{"path":"server.ini","format":"ini","category":"Server"}
			]`,
			wantError: true,
		},
		{
			name: "windows paths are case insensitive",
			input: `[
				{"path":"Config/Server.ini","format":"ini","category":"Server"},
				{"path":"config/server.ini","format":"ini","category":"Server"}
			]`,
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			errs := ValidateConfigSchemas(test.input)
			errorText := strings.Join(errs, "\n")
			if test.wantError {
				if !strings.Contains(errorText, "effective windows path") {
					t.Fatalf("ValidateConfigSchemas() errors = %v, want effective Windows path collision", errs)
				}
				return
			}
			if len(errs) != 0 {
				t.Fatalf("ValidateConfigSchemas() errors = %v, want none", errs)
			}
		})
	}
}

func TestRunPreStartGeneratesPlatformPalworldSettings(t *testing.T) {
	t.Parallel()

	schemasJSON := `[{
		"path":"Pal/Saved/Config/LinuxServer/PalWorldSettings.ini",
		"platform_paths":{"windows":"Pal/Saved/Config/WindowsServer/PalWorldSettings.ini"},
		"format":"palworld",
		"category":"Server",
		"generate_before_start":true,
		"managed_fields":{
			"ServerName":"game_server.server_name",
			"ServerPlayerMaxNum":"game_server.max_players",
			"RESTAPIPort":"game_server.query_port"
		},
		"schema":{"type":"object","properties":{
			"ServerName":{"type":"string","default":"Default Palworld Server"},
			"ServerPlayerMaxNum":{"type":"integer","default":32},
			"RESTAPIEnabled":{"type":"boolean","default":true},
			"RESTAPIPort":{"type":"integer","default":8212},
			"CrossplayPlatforms":{"type":"string","default":"Steam,Xbox,PS5,Mac"}
		}}
	}]`
	resolvedJSON, errResolve := ResolvePlatformConfigSchemas(schemasJSON, "windows")
	if errResolve != nil {
		t.Fatalf("ResolvePlatformConfigSchemas() error = %v", errResolve)
	}
	directory := t.TempDir()
	resolver := GameServerSettingsResolver(GameServerSettings{
		Name:       "Xylona Palworld",
		QueryPort:  38212,
		MaxPlayers: 48,
	})
	errRun := RunPreStartStrict(directory, resolvedJSON, resolver)
	if errRun != nil {
		t.Fatalf("RunPreStartStrict() error = %v", errRun)
	}

	windowsPath := filepath.Join(directory, "Pal", "Saved", "Config", "WindowsServer", "PalWorldSettings.ini")
	data, errRead := os.ReadFile(windowsPath)
	if errRead != nil {
		t.Fatalf("ReadFile() error = %v", errRead)
	}
	entries, errParse := (&cfgparse.PalworldParser{}).Parse(data)
	if errParse != nil {
		t.Fatalf("PalworldParser.Parse() error = %v", errParse)
	}
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		values[entry.Key] = entry.Value
	}
	for key, want := range map[string]string{
		"ServerName":         "Xylona Palworld",
		"ServerPlayerMaxNum": "48",
		"RESTAPIEnabled":     "true",
		"RESTAPIPort":        "38212",
		"CrossplayPlatforms": "Steam,Xbox,PS5,Mac",
	} {
		if values[key] != want {
			t.Errorf("generated value for %s = %q, want %q", key, values[key], want)
		}
	}
	linuxPath := filepath.Join(directory, "Pal", "Saved", "Config", "LinuxServer", "PalWorldSettings.ini")
	_, errLinux := os.Stat(linuxPath)
	if !os.IsNotExist(errLinux) {
		t.Fatalf("Linux settings Stat() error = %v, want os.ErrNotExist", errLinux)
	}
}
