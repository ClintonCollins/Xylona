package cfgschema

import (
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/cfgparse"
)

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

func TestValidateFields_InvalidEnum(t *testing.T) {
	fields := []FieldData{
		{Key: "gamemode", Value: "invalid_mode"},
	}
	schema := SchemaDefinition{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"gamemode": {Type: "string", Enum: []string{"survival", "creative", "adventure", "spectator"}},
		},
	}

	errs := ValidateFields(fields, schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
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
