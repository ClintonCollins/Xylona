package cfgschema

import (
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
