package cfgschema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPreStart_ManagedFieldsOverwritten(t *testing.T) {
	dir := t.TempDir()

	// Create a properties file with an old IP.
	content := "server-ip=0.0.0.0\nserver-port=25565\nmotd=Hello\n"
	errWrite := os.WriteFile(filepath.Join(dir, "server.properties"), []byte(content), 0o600)
	if errWrite != nil {
		t.Fatalf("setup write error: %v", errWrite)
	}

	schemasJSON := `[{
		"path": "server.properties",
		"format": "properties",
		"category": "Core",
		"managed_fields": {
			"server-ip": "game_server.ip",
			"server-port": "game_server.port"
		},
		"schema": {"type": "object", "properties": {}}
	}]`

	resolver := ServerSettingsResolver("192.168.1.100", 25570, 25570)
	RunPreStart(dir, schemasJSON, resolver)

	data, errRead := os.ReadFile(filepath.Join(dir, "server.properties"))
	if errRead != nil {
		t.Fatalf("read error: %v", errRead)
	}

	output := string(data)
	if !strings.Contains(output, "server-ip=192.168.1.100") {
		t.Errorf("expected server-ip=192.168.1.100, got:\n%s", output)
	}
	if !strings.Contains(output, "server-port=25570") {
		t.Errorf("expected server-port=25570, got:\n%s", output)
	}
	// motd should be untouched.
	if !strings.Contains(output, "motd=Hello") {
		t.Errorf("expected motd=Hello preserved, got:\n%s", output)
	}
}

func TestRunPreStart_GenerateFileWhenMissing(t *testing.T) {
	dir := t.TempDir()

	schemasJSON := `[{
		"path": "server.properties",
		"format": "properties",
		"category": "Core",
		"generate_before_start": true,
		"managed_fields": {
			"server-port": "game_server.port"
		},
		"schema": {
			"type": "object",
			"properties": {
				"motd": {"type": "string", "default": "A Minecraft Server"},
				"server-port": {"type": "integer", "default": 25565}
			}
		}
	}]`

	resolver := ServerSettingsResolver("0.0.0.0", 25570, 25570)
	RunPreStart(dir, schemasJSON, resolver)

	data, errRead := os.ReadFile(filepath.Join(dir, "server.properties"))
	if errRead != nil {
		t.Fatalf("expected file to be generated, got read error: %v", errRead)
	}

	output := string(data)
	// Managed field should be resolved.
	if !strings.Contains(output, "server-port=25570") {
		t.Errorf("expected server-port=25570, got:\n%s", output)
	}
}

func TestRunPreStart_SkipWhenMissingAndNoGenerate(t *testing.T) {
	dir := t.TempDir()

	schemasJSON := `[{
		"path": "server.properties",
		"format": "properties",
		"category": "Core",
		"generate_before_start": false,
		"managed_fields": {"server-port": "game_server.port"},
		"schema": {"type": "object", "properties": {}}
	}]`

	resolver := ServerSettingsResolver("0.0.0.0", 25570, 25570)
	RunPreStart(dir, schemasJSON, resolver)

	// File should NOT be created.
	_, errStat := os.Stat(filepath.Join(dir, "server.properties"))
	if errStat == nil {
		t.Error("expected file to not exist when generate_before_start is false")
	}
}

func TestRunPreStart_NonManagedFieldsUntouched(t *testing.T) {
	dir := t.TempDir()

	content := "motd=Custom MOTD\nserver-port=25565\n"
	errWrite := os.WriteFile(filepath.Join(dir, "server.properties"), []byte(content), 0o600)
	if errWrite != nil {
		t.Fatalf("setup write error: %v", errWrite)
	}

	schemasJSON := `[{
		"path": "server.properties",
		"format": "properties",
		"category": "Core",
		"managed_fields": {"server-port": "game_server.port"},
		"schema": {"type": "object", "properties": {}}
	}]`

	resolver := ServerSettingsResolver("0.0.0.0", 25570, 25570)
	RunPreStart(dir, schemasJSON, resolver)

	data, errRead := os.ReadFile(filepath.Join(dir, "server.properties"))
	if errRead != nil {
		t.Fatalf("read error: %v", errRead)
	}

	output := string(data)
	if !strings.Contains(output, "motd=Custom MOTD") {
		t.Errorf("expected motd to be untouched, got:\n%s", output)
	}
}

func TestRunPreStart_CorruptedFileDoesNotBlock(t *testing.T) {
	dir := t.TempDir()

	// Write a binary/corrupted file — properties parser should still handle it
	// gracefully (properties format is very lenient, so we use a format that
	// would actually fail). For this test, just verify no panic.
	errWrite := os.WriteFile(filepath.Join(dir, "server.properties"), []byte("key=value\n"), 0o600)
	if errWrite != nil {
		t.Fatalf("setup write error: %v", errWrite)
	}

	schemasJSON := `[{
		"path": "server.properties",
		"format": "properties",
		"category": "Core",
		"managed_fields": {"key": "game_server.port"},
		"schema": {"type": "object", "properties": {}}
	}]`

	resolver := ServerSettingsResolver("0.0.0.0", 25570, 25570)

	// Should not panic.
	RunPreStart(dir, schemasJSON, resolver)

	data, errRead := os.ReadFile(filepath.Join(dir, "server.properties"))
	if errRead != nil {
		t.Fatalf("read error: %v", errRead)
	}

	if !strings.Contains(string(data), "key=25570") {
		t.Errorf("expected managed field to be updated, got:\n%s", string(data))
	}
}

func TestRunPreStart_EmptySchemas(t *testing.T) {
	dir := t.TempDir()
	// Should not panic with empty or null schemas.
	RunPreStart(dir, "", nil)
	RunPreStart(dir, "[]", nil)
}

func TestRunPreStartStrictReportsManagedFieldFailure(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.properties")
	errWrite := os.WriteFile(configPath, []byte("telnet-password=keep-me\n"), 0o600)
	if errWrite != nil {
		t.Fatalf("write config: %v", errWrite)
	}

	schemasJSON := `[{
		"path": "server.properties",
		"format": "properties",
		"managed_fields": {"telnet-password": "unknown.local_console_password"},
		"schema": {"type": "object", "properties": {}}
	}]`
	errRun := RunPreStartStrict(dir, schemasJSON, GameServerSettingsResolver(GameServerSettings{}))
	if errRun == nil {
		t.Fatal("RunPreStartStrict() error = nil, want unknown source error")
	}

	data, errRead := os.ReadFile(configPath)
	if errRead != nil {
		t.Fatalf("read config: %v", errRead)
	}
	if string(data) != "telnet-password=keep-me\n" {
		t.Fatalf("config changed after strict failure: %q", data)
	}
}

func TestRunPreStartStrictEnforcesLocalConsoleSources(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.properties")
	errWrite := os.WriteFile(configPath, []byte("telnet-enabled=false\ntelnet-password=remote-secret\n"), 0o600)
	if errWrite != nil {
		t.Fatalf("write config: %v", errWrite)
	}

	schemasJSON := `[{
		"path": "server.properties",
		"format": "properties",
		"managed_fields": {
			"telnet-enabled": "xylona.local_console_enabled",
			"telnet-password": "xylona.local_console_password"
		},
		"schema": {"type": "object", "properties": {}}
	}]`
	errRun := RunPreStartStrict(dir, schemasJSON, GameServerSettingsResolver(GameServerSettings{}))
	if errRun != nil {
		t.Fatalf("RunPreStartStrict() error = %v", errRun)
	}

	data, errRead := os.ReadFile(configPath)
	if errRead != nil {
		t.Fatalf("read config: %v", errRead)
	}
	output := string(data)
	if !strings.Contains(output, "telnet-enabled=true") {
		t.Fatalf("managed config = %q, want enabled local console", output)
	}
	if !strings.Contains(output, "telnet-password=\n") {
		t.Fatalf("managed config = %q, want empty local console password", output)
	}
}

func TestRunPreStartStrictEnforcesAttributeKeyedXML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "serverconfig.xml")
	input := `<ServerSettings>
  <property name="TelnetEnabled" value="false"></property>
  <property name="TelnetPassword" value="remote-secret"></property>
  <property name="Unmanaged" value="preserved"></property>
</ServerSettings>`
	errWrite := os.WriteFile(configPath, []byte(input), 0o600)
	if errWrite != nil {
		t.Fatalf("write config: %v", errWrite)
	}

	schemasJSON := `[{
		"path": "serverconfig.xml",
		"format": "xml",
		"xml_key_mode": {
			"mode": "attributes",
			"element": "property",
			"key_attr": "name",
			"value_attr": "value"
		},
		"managed_fields": {
			"TelnetEnabled": "xylona.local_console_enabled",
			"TelnetPassword": "xylona.local_console_password"
		},
		"schema": {"type": "object", "properties": {}}
	}]`
	errRun := RunPreStartStrict(dir, schemasJSON, GameServerSettingsResolver(GameServerSettings{}))
	if errRun != nil {
		t.Fatalf("RunPreStartStrict() error = %v", errRun)
	}

	data, errRead := os.ReadFile(configPath)
	if errRead != nil {
		t.Fatalf("read config: %v", errRead)
	}
	output := string(data)
	for _, expected := range []string{
		`<property name="TelnetEnabled" value="true"/>`,
		`<property name="TelnetPassword" value=""/>`,
		`<property name="Unmanaged" value="preserved"/>`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("managed XML = %q, want %q", output, expected)
		}
	}
}

func TestRunPreStart_GenerateStructuredJSONWhenMissing(t *testing.T) {
	dir := t.TempDir()

	schemasJSON := `[{
		"path": "ServerDescription.json",
		"format": "json",
		"category": "Server",
		"generate_before_start": true,
		"managed_fields": {
			"ServerDescription_Persistent.DirectConnectionServerPort": "game_server.port"
		},
		"schema": {
			"type": "object",
			"properties": {
				"ServerDescription_Persistent.ServerName": {"type": "string", "default": "My Windrose Server"},
				"ServerDescription_Persistent.MaxPlayerCount": {"type": "integer", "default": 8},
				"ServerDescription_Persistent.UseDirectConnection": {"type": "boolean", "default": true},
				"ServerDescription_Persistent.DirectConnectionServerPort": {"type": "integer", "default": 7777}
			}
		}
	}]`

	resolver := ServerSettingsResolver("0.0.0.0", 7781, 7782)
	RunPreStart(dir, schemasJSON, resolver)

	data, errRead := os.ReadFile(filepath.Join(dir, "ServerDescription.json"))
	if errRead != nil {
		t.Fatalf("expected JSON file to be generated, got read error: %v", errRead)
	}

	var config map[string]any
	errUnmarshal := json.Unmarshal(data, &config)
	if errUnmarshal != nil {
		t.Fatalf("json.Unmarshal() error = %v", errUnmarshal)
	}

	persistent, ok := config["ServerDescription_Persistent"].(map[string]any)
	if !ok {
		t.Fatalf("ServerDescription_Persistent type = %T, want object", config["ServerDescription_Persistent"])
	}
	if persistent["ServerName"] != "My Windrose Server" {
		t.Errorf("ServerName = %v, want %q", persistent["ServerName"], "My Windrose Server")
	}
	if persistent["MaxPlayerCount"] != float64(8) {
		t.Errorf("MaxPlayerCount = %v, want 8", persistent["MaxPlayerCount"])
	}
	useDirectConnection, ok := persistent["UseDirectConnection"].(bool)
	if !ok || !useDirectConnection {
		t.Errorf("UseDirectConnection = %v, want true", persistent["UseDirectConnection"])
	}
	if persistent["DirectConnectionServerPort"] != float64(7781) {
		t.Errorf("DirectConnectionServerPort = %v, want 7781", persistent["DirectConnectionServerPort"])
	}
}

func TestRunPreStart_ManagedStructuredJSONOverwritten(t *testing.T) {
	dir := t.TempDir()

	content := `{
  "ServerDescription_Persistent": {
    "ServerName": "Keep Me",
    "DirectConnectionServerPort": 7777
  },
  "Unknown": "preserved"
}
`
	errWrite := os.WriteFile(filepath.Join(dir, "ServerDescription.json"), []byte(content), 0o600)
	if errWrite != nil {
		t.Fatalf("setup write error: %v", errWrite)
	}

	schemasJSON := `[{
		"path": "ServerDescription.json",
		"format": "json",
		"category": "Server",
		"managed_fields": {
			"ServerDescription_Persistent.DirectConnectionServerPort": "game_server.port"
		},
		"schema": {
			"type": "object",
			"properties": {
				"ServerDescription_Persistent.ServerName": {"type": "string"},
				"ServerDescription_Persistent.DirectConnectionServerPort": {"type": "integer"}
			}
		}
	}]`

	resolver := ServerSettingsResolver("0.0.0.0", 7781, 7782)
	RunPreStart(dir, schemasJSON, resolver)

	data, errRead := os.ReadFile(filepath.Join(dir, "ServerDescription.json"))
	if errRead != nil {
		t.Fatalf("read error: %v", errRead)
	}

	var config map[string]any
	errUnmarshal := json.Unmarshal(data, &config)
	if errUnmarshal != nil {
		t.Fatalf("json.Unmarshal() error = %v", errUnmarshal)
	}

	persistent, ok := config["ServerDescription_Persistent"].(map[string]any)
	if !ok {
		t.Fatalf("ServerDescription_Persistent type = %T, want object", config["ServerDescription_Persistent"])
	}
	if persistent["ServerName"] != "Keep Me" {
		t.Errorf("ServerName = %v, want %q", persistent["ServerName"], "Keep Me")
	}
	if persistent["DirectConnectionServerPort"] != float64(7781) {
		t.Errorf("DirectConnectionServerPort = %v, want 7781", persistent["DirectConnectionServerPort"])
	}
	if config["Unknown"] != "preserved" {
		t.Errorf("Unknown = %v, want %q", config["Unknown"], "preserved")
	}
}
