package cfgschema

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/cfgparse"
)

func TestSchemaProperty_GroupDeserialization(t *testing.T) {
	jsonStr := `{
		"type": "object",
		"properties": {
			"port": {"type": "integer", "title": "Port", "x-group": "network"},
			"motd": {"type": "string", "title": "MOTD"}
		}
	}`
	var schema SchemaDefinition
	errUnmarshal := json.Unmarshal([]byte(jsonStr), &schema)
	if errUnmarshal != nil {
		t.Fatalf("unmarshal error: %v", errUnmarshal)
	}

	portProp := schema.Properties["port"]
	if portProp.Group != "network" {
		t.Errorf("port group = %q, want %q", portProp.Group, "network")
	}

	motdProp := schema.Properties["motd"]
	if motdProp.Group != "" {
		t.Errorf("motd group = %q, want empty", motdProp.Group)
	}
}

func TestSchemaProperty_OrderDeserialization(t *testing.T) {
	jsonStr := `{
		"type": "object",
		"x-groups": ["network", "gameplay"],
		"properties": {
			"port": {"type": "integer", "title": "Port", "x-group": "network", "x-order": 1},
			"motd": {"type": "string", "title": "MOTD", "x-order": 5},
			"pvp":  {"type": "boolean", "title": "PvP"}
		}
	}`
	var schema SchemaDefinition
	errUnmarshal := json.Unmarshal([]byte(jsonStr), &schema)
	if errUnmarshal != nil {
		t.Fatalf("unmarshal error: %v", errUnmarshal)
	}

	// Verify x-groups on SchemaDefinition.
	if len(schema.Groups) != 2 {
		t.Fatalf("groups length = %d, want 2", len(schema.Groups))
	}
	if schema.Groups[0] != "network" || schema.Groups[1] != "gameplay" {
		t.Errorf("groups = %v, want [network, gameplay]", schema.Groups)
	}

	// Verify x-order on properties.
	portProp := schema.Properties["port"]
	if portProp.Order == nil {
		t.Fatal("port order is nil, want 1")
	}
	if *portProp.Order != 1 {
		t.Errorf("port order = %d, want 1", *portProp.Order)
	}

	motdProp := schema.Properties["motd"]
	if motdProp.Order == nil {
		t.Fatal("motd order is nil, want 5")
	}
	if *motdProp.Order != 5 {
		t.Errorf("motd order = %d, want 5", *motdProp.Order)
	}

	// Verify nil order when x-order is absent.
	pvpProp := schema.Properties["pvp"]
	if pvpProp.Order != nil {
		t.Errorf("pvp order = %v, want nil", pvpProp.Order)
	}
}

func testResolver(source string) (string, bool) {
	sources := map[string]string{
		"game_server.ip":         "192.168.1.1",
		"game_server.port":       "25565",
		"game_server.query_port": "25565",
	}
	v, ok := sources[source]
	return v, ok
}

func TestMatchFields_MatchesSchemaFields(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "motd", Value: "Hello World"},
		{Key: "port", Value: "25565"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"motd": {Type: "string", Title: "MOTD"},
			"port": {Type: "integer", Title: "Port"},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	if len(result.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(result.Fields))
	}

	foundMotd := false
	for _, f := range result.Fields {
		if f.Key == "motd" {
			foundMotd = true
			if f.Value != "Hello World" {
				t.Errorf("motd value = %q, want %q", f.Value, "Hello World")
			}
			if f.Title != "MOTD" {
				t.Errorf("motd title = %q, want %q", f.Title, "MOTD")
			}
		}
	}
	if !foundMotd {
		t.Error("motd field not found in results")
	}
}

func TestMatchFields_UnmatchedEntriesBecomeAdvanced(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "motd", Value: "Hello"},
		{Key: "custom-key", Value: "custom-value"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"motd": {Type: "string", Title: "MOTD"},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	if len(result.AdvancedFields) != 1 {
		t.Fatalf("expected 1 advanced field, got %d", len(result.AdvancedFields))
	}
	if result.AdvancedFields[0].Key != "custom-key" {
		t.Errorf("advanced field key = %q, want %q", result.AdvancedFields[0].Key, "custom-key")
	}
}

func TestMatchFields_MissingFieldsGetDefaults(t *testing.T) {
	entries := []cfgparse.ConfigEntry{}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"motd": {Type: "string", Title: "MOTD", Default: "A Minecraft Server"},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	if len(result.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(result.Fields))
	}
	if !result.Fields[0].IsMissingFromFile {
		t.Error("expected IsMissingFromFile = true")
	}
	if result.Fields[0].Value != "A Minecraft Server" {
		t.Errorf("value = %q, want default %q", result.Fields[0].Value, "A Minecraft Server")
	}
}

func TestMatchFields_ManagedFieldsResolved(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "server-ip", Value: "0.0.0.0"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"server-ip": {Type: "string", Title: "Server IP"},
		},
	}
	managed := map[string]string{
		"server-ip": "game_server.ip",
	}

	result := MatchFields(entries, schema, managed, testResolver)

	if len(result.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(result.Fields))
	}
	f := result.Fields[0]
	if !f.IsManaged {
		t.Error("expected IsManaged = true")
	}
	if f.Value != "192.168.1.1" {
		t.Errorf("managed value = %q, want %q", f.Value, "192.168.1.1")
	}
}

func TestMatchFields_ManagedFieldAliasesResolved(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "server-port", Value: "25565"},
		{Key: "query-port", Value: "25565"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"server-port": {Type: "integer", Title: "Server Port"},
			"query-port":  {Type: "integer", Title: "Query Port"},
		},
	}
	managed := map[string]string{
		"server-port": "server_port",
		"query-port":  "query_port",
	}

	result := MatchFields(entries, schema, managed, ServerSettingsResolver("127.0.0.1", 25567, 25567))

	if len(result.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(result.Fields))
	}

	if got := result.Fields[0].Value; got != "25567" {
		t.Errorf("server-port value = %q, want %q", got, "25567")
	}
	if got := result.Fields[0].ManagedSource; got != "game_server.port" {
		t.Errorf("server-port managed source = %q, want %q", got, "game_server.port")
	}
	if got := result.Fields[1].Value; got != "25567" {
		t.Errorf("query-port value = %q, want %q", got, "25567")
	}
	if got := result.Fields[1].ManagedSource; got != "game_server.query_port" {
		t.Errorf("query-port managed source = %q, want %q", got, "game_server.query_port")
	}
}

func TestGameServerSettingsResolver(t *testing.T) {
	resolver := GameServerSettingsResolver(GameServerSettings{
		Name:                "Example Server",
		Directory:           "/srv/example",
		IP:                  "127.0.0.1",
		Port:                25565,
		QueryPort:           25566,
		MaxPlayers:          24,
		LocalConsoleEnabled: true,
		LocalConsolePort:    25567,
	})

	testCases := []struct {
		name       string
		source     string
		want       string
		wantExists bool
	}{
		{name: "name", source: "game_server.server_name", want: "Example Server", wantExists: true},
		{name: "name alias", source: "server_name", want: "Example Server", wantExists: true},
		{name: "directory", source: "game_server.directory", want: "/srv/example", wantExists: true},
		{name: "directory alias", source: "directory", want: "/srv/example", wantExists: true},
		{name: "IP", source: "game_server.ip", want: "127.0.0.1", wantExists: true},
		{name: "port", source: "game_server.port", want: "25565", wantExists: true},
		{name: "port plus one", source: "game_server.port_plus_1", want: "25566", wantExists: true},
		{name: "port plus one alias", source: "server_port_plus_1", want: "25566", wantExists: true},
		{name: "port plus two", source: "game_server.port_plus_2", want: "25567", wantExists: true},
		{name: "port plus two alias", source: "server_port_plus_2", want: "25567", wantExists: true},
		{name: "query port", source: "game_server.query_port", want: "25566", wantExists: true},
		{name: "query port plus one", source: "game_server.query_port_plus_1", want: "25567", wantExists: true},
		{name: "query port plus one alias", source: "query_port_plus_1", want: "25567", wantExists: true},
		{name: "max players", source: "game_server.max_players", want: "24", wantExists: true},
		{name: "max players alias", source: "max_players", want: "24", wantExists: true},
		{name: "local console enabled", source: "xylona.local_console_enabled", want: "true", wantExists: true},
		{name: "local console port", source: "xylona.local_console_port", want: "25567", wantExists: true},
		{name: "local console password", source: "xylona.local_console_password", want: "", wantExists: true},
		{name: "unknown", source: "game_server.unknown", wantExists: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, exists := resolver(testCase.source)
			if exists != testCase.wantExists {
				t.Fatalf("resolver(%q) exists = %t, want %t", testCase.source, exists, testCase.wantExists)
			}
			if got != testCase.want {
				t.Errorf("resolver(%q) = %q, want %q", testCase.source, got, testCase.want)
			}
		})
	}

	disabledResolver := GameServerSettingsResolver(GameServerSettings{
		LocalConsoleConfigured: true,
		LocalConsoleEnabled:    false,
	})
	disabled, exists := disabledResolver("xylona.local_console_enabled")
	if !exists || disabled != "false" {
		t.Fatalf("explicitly disabled local console = %q, %t, want false, true", disabled, exists)
	}
}

func TestWithoutManagedSourcesRemovesExplicitAndSchemaOwnership(t *testing.T) {
	input := `[{
		"path":"server.properties",
		"format":"properties",
		"managed_fields":{
			"enable-rcon":"xylona.local_console_enabled",
			"rcon.password":"xylona.local_console_password"
		},
		"schema":{"type":"object","properties":{
			"enable-rcon":{"type":"boolean","x-managed":{"source":"xylona.local_console_enabled"}},
			"rcon.password":{"type":"string","x-managed":{"source":"xylona.local_console_password"}},
			"server-port":{"type":"integer","x-managed":{"source":"game_server.port"}}
		}}
	}]`
	filtered, errFilter := WithoutManagedSources(
		input,
		"xylona.local_console_enabled",
		"xylona.local_console_password",
	)
	if errFilter != nil {
		t.Fatalf("WithoutManagedSources() error = %v", errFilter)
	}
	entries, errParse := ParseConfigSchemas(filtered)
	if errParse != nil {
		t.Fatalf("ParseConfigSchemas() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("ParseConfigSchemas() entries = %d, want 1", len(entries))
	}
	managed := entries[0].ManagedFields
	if _, exists := managed["enable-rcon"]; exists {
		t.Fatal("enable-rcon remained managed")
	}
	if _, exists := managed["rcon.password"]; exists {
		t.Fatal("rcon.password remained managed")
	}
	if managed["server-port"] != "game_server.port" {
		t.Fatalf("server-port managed source = %q", managed["server-port"])
	}
}

func TestParseConfigSchemas_DerivesManagedFieldsFromSchemaProperties(t *testing.T) {
	input := `[{
		"path": "server.properties",
		"format": "properties",
		"category": "Core",
		"schema": {
			"type": "object",
			"properties": {
				"server-ip": {
					"type": "string",
					"title": "Server IP",
					"x-managed": {"source": "game_server.ip"}
				},
				"server-port": {
					"type": "integer",
					"title": "Server Port",
					"x-managed": {"source": "game_server.port"}
				}
			}
		}
	}]`

	entries, errParse := ParseConfigSchemas(input)
	if errParse != nil {
		t.Fatalf("ParseConfigSchemas() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	if len(entries[0].ManagedFields) != 2 {
		t.Fatalf("len(ManagedFields) = %d, want 2", len(entries[0].ManagedFields))
	}
	if got := entries[0].ManagedFields["server-ip"]; got != "game_server.ip" {
		t.Errorf("ManagedFields[server-ip] = %q, want %q", got, "game_server.ip")
	}
	if got := entries[0].ManagedFields["server-port"]; got != "game_server.port" {
		t.Errorf("ManagedFields[server-port] = %q, want %q", got, "game_server.port")
	}
}

func TestParseConfigSchemas_CanonicalizesManagedSourceAliases(t *testing.T) {
	input := `[{
		"path": "server.properties",
		"format": "properties",
		"category": "Core",
		"managed_fields": {
			"server-ip": "ip"
		},
		"schema": {
			"type": "object",
			"properties": {
				"server-port": {
					"type": "integer",
					"title": "Server Port",
					"x-managed": {"source": "server_port"}
				},
				"query-port": {
					"type": "integer",
					"title": "Query Port",
					"x-managed": {"source": "query_port"}
				}
			}
		}
	}]`

	entries, errParse := ParseConfigSchemas(input)
	if errParse != nil {
		t.Fatalf("ParseConfigSchemas() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	if got := entries[0].ManagedFields["server-ip"]; got != "game_server.ip" {
		t.Errorf("ManagedFields[server-ip] = %q, want %q", got, "game_server.ip")
	}
	if got := entries[0].ManagedFields["server-port"]; got != "game_server.port" {
		t.Errorf("ManagedFields[server-port] = %q, want %q", got, "game_server.port")
	}
	if got := entries[0].ManagedFields["query-port"]; got != "game_server.query_port" {
		t.Errorf("ManagedFields[query-port] = %q, want %q", got, "game_server.query_port")
	}

	if got := entries[0].Schema.Properties["server-port"].Managed.Source; got != "game_server.port" {
		t.Errorf("Schema.Properties[server-port].Managed.Source = %q, want %q", got, "game_server.port")
	}
	if got := entries[0].Schema.Properties["query-port"].Managed.Source; got != "game_server.query_port" {
		t.Errorf("Schema.Properties[query-port].Managed.Source = %q, want %q", got, "game_server.query_port")
	}
}

func TestMatchFields_AllowMultipleCollectsValues(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "level", Value: "world1", Index: 0},
		{Key: "level", Value: "world2", Index: 1},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"level": {Type: "string", Title: "Level", AllowMultiple: true},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	if len(result.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(result.Fields))
	}
	if len(result.Fields[0].Values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(result.Fields[0].Values))
	}
	if result.Fields[0].Values[0] != "world1" || result.Fields[0].Values[1] != "world2" {
		t.Errorf("values = %v, want [world1, world2]", result.Fields[0].Values)
	}
}

func TestMatchFields_IntegerDefault(t *testing.T) {
	entries := []cfgparse.ConfigEntry{}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"port": {Type: "integer", Title: "Port", Default: float64(25565)},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)
	if result.Fields[0].DefaultValue != "25565" {
		t.Errorf("default = %q, want %q", result.Fields[0].DefaultValue, "25565")
	}
}

func TestMatchFields_BoolDefault(t *testing.T) {
	entries := []cfgparse.ConfigEntry{}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"pvp": {Type: "boolean", Title: "PvP", Default: true},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)
	if result.Fields[0].DefaultValue != "true" {
		t.Errorf("default = %q, want %q", result.Fields[0].DefaultValue, "true")
	}
}

// MergeAndWrite tests

func TestMergeAndWrite_UpdateExistingValue(t *testing.T) {
	existing := []cfgparse.ConfigEntry{
		{Key: "motd", Value: "Old MOTD", Comment: "# Server message"},
		{Key: "port", Value: "25565"},
	}
	updated := []FieldData{
		{Key: "motd", Value: "New MOTD"},
		{Key: "port", Value: "25565"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"motd": {Type: "string"},
			"port": {Type: "integer"},
		},
	}

	result := MergeAndWrite(existing, updated, nil, schema)

	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result[0].Value != "New MOTD" {
		t.Errorf("motd value = %q, want %q", result[0].Value, "New MOTD")
	}
	if result[0].Comment != "# Server message" {
		t.Errorf("comment = %q, want preserved", result[0].Comment)
	}
}

func TestMergeAndWrite_PreservesUnknownFields(t *testing.T) {
	existing := []cfgparse.ConfigEntry{
		{Key: "motd", Value: "Hello"},
		{Key: "unknown-key", Value: "unknown-value"},
	}
	updated := []FieldData{
		{Key: "motd", Value: "Updated"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"motd": {Type: "string"},
		},
	}

	result := MergeAndWrite(existing, updated, nil, schema)

	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result[1].Key != "unknown-key" || result[1].Value != "unknown-value" {
		t.Errorf("unknown field not preserved: %+v", result[1])
	}
}

func TestMergeAndWrite_AppendNewSchemaFields(t *testing.T) {
	existing := []cfgparse.ConfigEntry{
		{Key: "motd", Value: "Hello"},
	}
	updated := []FieldData{
		{Key: "motd", Value: "Hello"},
		{Key: "new-field", Value: "new-value", IsMissingFromFile: true},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"motd":      {Type: "string"},
			"new-field": {Type: "string"},
		},
	}

	result := MergeAndWrite(existing, updated, nil, schema)

	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result[1].Key != "new-field" || result[1].Value != "new-value" {
		t.Errorf("new field not appended: %+v", result[1])
	}
}

func TestMergeAndWrite_AllowMultipleDuplicates(t *testing.T) {
	existing := []cfgparse.ConfigEntry{
		{Key: "level", Value: "world1", Index: 0},
	}
	updated := []FieldData{
		{Key: "level", Value: "world1", AllowMultiple: true, Values: []string{"world1", "world2", "world3"}},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"level": {Type: "string", AllowMultiple: true},
		},
	}

	result := MergeAndWrite(existing, updated, nil, schema)

	if len(result) != 3 {
		t.Fatalf("expected 3 entries for allow-multiple, got %d", len(result))
	}
	for i, expected := range []string{"world1", "world2", "world3"} {
		if result[i].Value != expected {
			t.Errorf("result[%d].Value = %q, want %q", i, result[i].Value, expected)
		}
	}
}

// ValidateFields tests

func TestValidateFields_ValidValues(t *testing.T) {
	fields := []FieldData{
		{Key: "port", Value: "25565"},
		{Key: "motd", Value: "Hello"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"port": {Type: "integer", Minimum: new(int64(1)), Maximum: new(int64(65535))},
			"motd": {Type: "string", MaxLength: new(int32(59))},
		},
	}

	errs := ValidateFields(fields, schema)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateFields_IntegerOutOfRange(t *testing.T) {
	fields := []FieldData{
		{Key: "port", Value: "99999"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"port": {Type: "integer", Minimum: new(int64(1)), Maximum: new(int64(65535))},
		},
	}

	errs := ValidateFields(fields, schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "port" {
		t.Errorf("error field = %q, want %q", errs[0].Field, "port")
	}
}

func TestValidateFields_IntegerBelowMinimum(t *testing.T) {
	fields := []FieldData{
		{Key: "port", Value: "0"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"port": {Type: "integer", Minimum: new(int64(1)), Maximum: new(int64(65535))},
		},
	}

	errs := ValidateFields(fields, schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateFields_NonEnumValueAllowed(t *testing.T) {
	fields := []FieldData{
		{Key: "gamemode", Value: "modded_mode"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"gamemode": {Type: "string", Enum: []string{"survival", "creative", "adventure", "spectator"}},
		},
	}

	errs := ValidateFields(fields, schema)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors (enum values are suggestions), got %d: %v", len(errs), errs)
	}
}

func TestValidateFields_MissingRequired(t *testing.T) {
	fields := []FieldData{
		{Key: "name", Value: ""},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"name": {Type: "string", Required: true},
		},
	}

	errs := ValidateFields(fields, schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateFields_StringExceedsMaxLength(t *testing.T) {
	longString := strings.Repeat("x", 60)
	fields := []FieldData{
		{Key: "motd", Value: longString},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"motd": {Type: "string", MaxLength: new(int32(59))},
		},
	}

	errs := ValidateFields(fields, schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateFields_ManagedFieldSkipped(t *testing.T) {
	fields := []FieldData{
		{Key: "port", Value: "99999", IsManaged: true},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"port": {Type: "integer", Minimum: new(int64(1)), Maximum: new(int64(65535))},
		},
	}

	errs := ValidateFields(fields, schema)
	if len(errs) != 0 {
		t.Errorf("expected no errors for managed field, got %d: %v", len(errs), errs)
	}
}

func TestMatchFields_PreservesFileOrder(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "port", Value: "25565"},
		{Key: "motd", Value: "Hello"},
		{Key: "difficulty", Value: "hard"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"difficulty": {Type: "string", Title: "Difficulty"},
			"motd":       {Type: "string", Title: "MOTD"},
			"port":       {Type: "integer", Title: "Port"},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	if len(result.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(result.Fields))
	}

	expectedOrder := []string{"port", "motd", "difficulty"}
	for i, expected := range expectedOrder {
		if result.Fields[i].Key != expected {
			t.Errorf("field[%d].Key = %q, want %q", i, result.Fields[i].Key, expected)
		}
	}
}

func TestMatchFields_SchemaOnlyFieldsAppendedAlphabetically(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "port", Value: "25565"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"port":       {Type: "integer", Title: "Port"},
			"motd":       {Type: "string", Title: "MOTD", Default: "A Server"},
			"difficulty": {Type: "string", Title: "Difficulty", Default: "normal"},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	if len(result.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(result.Fields))
	}

	expectedOrder := []string{"port", "difficulty", "motd"}
	for i, expected := range expectedOrder {
		if result.Fields[i].Key != expected {
			t.Errorf("field[%d].Key = %q, want %q", i, result.Fields[i].Key, expected)
		}
	}

	if !result.Fields[1].IsMissingFromFile {
		t.Error("difficulty should be IsMissingFromFile")
	}
	if !result.Fields[2].IsMissingFromFile {
		t.Error("motd should be IsMissingFromFile")
	}
}

func TestMatchFields_PassesGroupFromSchema(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "port", Value: "25565"},
		{Key: "motd", Value: "Hello"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"port": {Type: "integer", Title: "Port", Group: "network"},
			"motd": {Type: "string", Title: "MOTD", Group: "gameplay"},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	for _, f := range result.Fields {
		switch f.Key {
		case "port":
			if f.Group != "network" {
				t.Errorf("port group = %q, want %q", f.Group, "network")
			}
		case "motd":
			if f.Group != "gameplay" {
				t.Errorf("motd group = %q, want %q", f.Group, "gameplay")
			}
		}
	}
}

func TestValidateFields_InvalidInteger(t *testing.T) {
	fields := []FieldData{
		{Key: "port", Value: "not_a_number"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"port": {Type: "integer"},
		},
	}

	errs := ValidateFields(fields, schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestMatchFields_RespectsXOrderAndXGroups(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "motd", Value: "Hello"},
		{Key: "port", Value: "25565"},
		{Key: "ip", Value: "0.0.0.0"},
	}
	schema := SchemaDefinition{
		Type:   "object",
		Groups: []string{"network", "gameplay"},
		Properties: map[string]SchemaProperty{
			"motd": {Type: "string", Title: "MOTD", Group: "gameplay", Order: new(int32(0))},
			"port": {Type: "integer", Title: "Port", Group: "network", Order: new(int32(1))},
			"ip":   {Type: "string", Title: "IP", Group: "network", Order: new(int32(0))},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	wantKeys := []string{"ip", "port", "motd"}
	if len(result.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(result.Fields))
	}
	for i, f := range result.Fields {
		if f.Key != wantKeys[i] {
			t.Errorf("result.Fields[%d].Key = %q, want %q", i, f.Key, wantKeys[i])
		}
	}
}

func TestMatchFields_SchemaOnlyFieldsSortedByOrder(t *testing.T) {
	entries := []cfgparse.ConfigEntry{}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"z-field": {Type: "string", Order: new(int32(2))},
			"a-field": {Type: "string", Order: new(int32(0))},
			"m-field": {Type: "string", Order: new(int32(1))},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	wantKeys := []string{"a-field", "m-field", "z-field"}
	if len(result.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(result.Fields))
	}
	for i, f := range result.Fields {
		if f.Key != wantKeys[i] {
			t.Errorf("result.Fields[%d].Key = %q, want %q", i, f.Key, wantKeys[i])
		}
	}
}

func TestMatchFields_OrderPropagatedToFieldData(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "motd", Value: "Hello World"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"motd": {Type: "string", Title: "MOTD", Order: new(int32(5))},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	if len(result.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(result.Fields))
	}
	if result.Fields[0].Order == nil {
		t.Fatal("expected Order to be non-nil")
	}
	if *result.Fields[0].Order != 5 {
		t.Errorf("Order = %d, want 5", *result.Fields[0].Order)
	}
}

func TestMatchFields_MixedOrderedAndUnorderedFields(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "motd", Value: "Hello"},
		{Key: "port", Value: "25565"},
		{Key: "pvp", Value: "true"},
		{Key: "difficulty", Value: "hard"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"motd":       {Type: "string", Title: "MOTD"},
			"port":       {Type: "integer", Title: "Port", Order: new(int32(0))},
			"pvp":        {Type: "boolean", Title: "PvP"},
			"difficulty": {Type: "string", Title: "Difficulty", Order: new(int32(1))},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	if len(result.Fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(result.Fields))
	}

	// Ordered fields (port=0, difficulty=1) should come before unordered fields.
	// Unordered fields preserve their relative order from the file.
	wantKeys := []string{"port", "difficulty", "motd", "pvp"}
	for i, want := range wantKeys {
		if result.Fields[i].Key != want {
			t.Errorf("result.Fields[%d].Key = %q, want %q", i, result.Fields[i].Key, want)
		}
	}
}

func TestMatchFields_GroupOrderWithMixedFileAndSchemaOnlyFields(t *testing.T) {
	entries := []cfgparse.ConfigEntry{
		{Key: "motd", Value: "Hello"},
		{Key: "port", Value: "25565"},
	}
	schema := SchemaDefinition{
		Type:   "object",
		Groups: []string{"network", "gameplay"},
		Properties: map[string]SchemaProperty{
			"port":       {Type: "integer", Title: "Port", Group: "network", Order: new(int32(0))},
			"ip":         {Type: "string", Title: "IP", Group: "network", Order: new(int32(1)), Default: "0.0.0.0"},
			"motd":       {Type: "string", Title: "MOTD", Group: "gameplay", Order: new(int32(0))},
			"difficulty": {Type: "string", Title: "Difficulty", Group: "gameplay", Order: new(int32(1)), Default: "normal"},
		},
	}

	result := MatchFields(entries, schema, nil, testResolver)

	if len(result.Fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(result.Fields))
	}

	// Network group first (port from file, ip schema-only), then gameplay group
	// (motd from file, difficulty schema-only).
	wantKeys := []string{"port", "ip", "motd", "difficulty"}
	for i, want := range wantKeys {
		if result.Fields[i].Key != want {
			t.Errorf("result.Fields[%d].Key = %q, want %q", i, result.Fields[i].Key, want)
		}
	}

	// Verify schema-only fields are marked correctly.
	for _, f := range result.Fields {
		switch f.Key {
		case "ip", "difficulty":
			if !f.IsMissingFromFile {
				t.Errorf("%q should be IsMissingFromFile", f.Key)
			}
		case "port", "motd":
			if f.IsMissingFromFile {
				t.Errorf("%q should NOT be IsMissingFromFile", f.Key)
			}
		}
	}
}

func TestParseConfigSchemas_RoundTripWithOrdering(t *testing.T) {
	jsonStr := `[{
		"path": "server.properties",
		"format": "properties",
		"category": "server",
		"generate_before_start": false,
		"schema": {
			"type": "object",
			"x-groups": ["network", "gameplay"],
			"properties": {
				"port": {"type": "integer", "title": "Port", "x-group": "network", "x-order": 0},
				"motd": {"type": "string", "title": "MOTD", "x-group": "gameplay", "x-order": 1}
			}
		}
	}]`

	entries, errParse := ParseConfigSchemas(jsonStr)
	if errParse != nil {
		t.Fatalf("ParseConfigSchemas error: %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]

	// Verify Groups parsed correctly.
	if len(entry.Schema.Groups) != 2 {
		t.Fatalf("groups length = %d, want 2", len(entry.Schema.Groups))
	}
	if entry.Schema.Groups[0] != "network" || entry.Schema.Groups[1] != "gameplay" {
		t.Errorf("groups = %v, want [network, gameplay]", entry.Schema.Groups)
	}

	// Verify Order parsed correctly.
	portProp := entry.Schema.Properties["port"]
	if portProp.Order == nil || *portProp.Order != 0 {
		t.Errorf("port order = %v, want 0", portProp.Order)
	}
	motdProp := entry.Schema.Properties["motd"]
	if motdProp.Order == nil || *motdProp.Order != 1 {
		t.Errorf("motd order = %v, want 1", motdProp.Order)
	}

	// Verify Group parsed correctly.
	if portProp.Group != "network" {
		t.Errorf("port group = %q, want %q", portProp.Group, "network")
	}
	if motdProp.Group != "gameplay" {
		t.Errorf("motd group = %q, want %q", motdProp.Group, "gameplay")
	}

	// Round-trip: marshal back to JSON and re-parse.
	marshaled, errMarshal := json.Marshal(entries)
	if errMarshal != nil {
		t.Fatalf("json.Marshal error: %v", errMarshal)
	}

	roundTripped, errRoundTrip := ParseConfigSchemas(string(marshaled))
	if errRoundTrip != nil {
		t.Fatalf("round-trip ParseConfigSchemas error: %v", errRoundTrip)
	}
	if len(roundTripped) != 1 {
		t.Fatalf("round-trip expected 1 entry, got %d", len(roundTripped))
	}

	rtSchema := roundTripped[0].Schema
	if len(rtSchema.Groups) != 2 {
		t.Fatalf("round-trip groups length = %d, want 2", len(rtSchema.Groups))
	}
	if rtSchema.Groups[0] != "network" || rtSchema.Groups[1] != "gameplay" {
		t.Errorf("round-trip groups = %v, want [network, gameplay]", rtSchema.Groups)
	}

	rtPort := rtSchema.Properties["port"]
	if rtPort.Order == nil || *rtPort.Order != 0 {
		t.Errorf("round-trip port order = %v, want 0", rtPort.Order)
	}
	rtMotd := rtSchema.Properties["motd"]
	if rtMotd.Order == nil || *rtMotd.Order != 1 {
		t.Errorf("round-trip motd order = %v, want 1", rtMotd.Order)
	}
}

func TestGenerateDefaultEntries_RespectsOrder(t *testing.T) {
	entry := ConfigSchemaEntry{
		Path:   "test.properties",
		Format: "properties",
		Schema: SchemaDefinition{
			Type: "object",
			Properties: map[string]SchemaProperty{
				"z-field": {Type: "string", Default: "z", Order: new(int32(2))},
				"a-field": {Type: "string", Default: "a", Order: new(int32(0))},
				"m-field": {Type: "string", Default: "m", Order: new(int32(1))},
			},
		},
	}

	result := generateDefaultEntries(entry)

	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}

	wantKeys := []string{"a-field", "m-field", "z-field"}
	wantValues := []string{"a", "m", "z"}
	for i, want := range wantKeys {
		if result[i].Key != want {
			t.Errorf("result[%d].Key = %q, want %q", i, result[i].Key, want)
		}
		if result[i].Value != wantValues[i] {
			t.Errorf("result[%d].Value = %q, want %q", i, result[i].Value, wantValues[i])
		}
	}
}
