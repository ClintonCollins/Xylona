package cfgschema

import (
	"strings"
	"testing"
)

func TestValidate_ValidSchema(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","properties":{}}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) != 0 {
		t.Errorf("ValidateConfigSchemas() returned %d errors, want 0: %v", len(errs), errs)
	}
}

func TestValidate_CommandCFGFormat(t *testing.T) {
	input := `[{"path":"server.cfg","format":"commandcfg","category":"Core","schema":{"type":"object","properties":{"hostname":{"type":"string"}}}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) != 0 {
		t.Errorf("ValidateConfigSchemas() returned %d errors, want 0: %v", len(errs), errs)
	}
}

func TestValidate_EmptyArray(t *testing.T) {
	errs := ValidateConfigSchemas("[]")
	if len(errs) != 0 {
		t.Errorf("ValidateConfigSchemas([]) returned %d errors, want 0: %v", len(errs), errs)
	}
}

func TestValidate_MalformedJSON(t *testing.T) {
	errs := ValidateConfigSchemas("{bad json")
	if len(errs) == 0 {
		t.Error("ValidateConfigSchemas(malformed) expected errors, got none")
	}
}

func TestValidate_MissingPath(t *testing.T) {
	input := `[{"format":"properties","category":"Core"}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for missing path")
	}
}

func TestValidate_MissingFormat(t *testing.T) {
	input := `[{"path":"server.properties","category":"Core"}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for missing format")
	}
}

func TestValidate_MissingCategory(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties"}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for missing category")
	}
}

func TestValidate_InvalidFormat(t *testing.T) {
	input := `[{"path":"test.cfg","format":"lua","category":"Core"}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for invalid format")
	}
}

func TestValidate_PathTraversal(t *testing.T) {
	input := `[{"path":"../etc/passwd","format":"properties","category":"Core"}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for path traversal")
	}
}

func TestValidate_UnknownManagedSource(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","managed_fields":{"key":"unknown.source"}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for unknown managed field source")
	}
}

func TestValidate_ValidManagedSources(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","managed_fields":{"server-ip":"game_server.ip","server-port":"game_server.port","query-port":"game_server.query_port"}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) != 0 {
		t.Errorf("ValidateConfigSchemas() returned %d errors for valid managed sources: %v", len(errs), errs)
	}
}

func TestValidate_ValidManagedSourceAliases(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","managed_fields":{"server-ip":"ip","server-port":"server_port","query-port":"query_port"}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) != 0 {
		t.Errorf("ValidateConfigSchemas() returned %d errors for valid managed source aliases: %v", len(errs), errs)
	}
}

func TestValidate_UnknownManagedSourceInSchemaProperty(t *testing.T) {
	input := `[{
		"path":"server.properties",
		"format":"properties",
		"category":"Core",
		"schema":{
			"type":"object",
			"properties":{
				"server-ip":{"type":"string","x-managed":{"source":"unknown.source"}}
			}
		}
	}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for unknown x-managed source")
	}
}

func TestValidate_ValidManagedSourceAliasesInSchemaProperty(t *testing.T) {
	input := `[{
		"path":"server.properties",
		"format":"properties",
		"category":"Core",
		"schema":{
			"type":"object",
			"properties":{
				"server-ip":{"type":"string","x-managed":{"source":"ip"}},
				"server-port":{"type":"integer","x-managed":{"source":"server_port"}},
				"query-port":{"type":"integer","x-managed":{"source":"query_port"}}
			}
		}
	}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) != 0 {
		t.Errorf("ValidateConfigSchemas() returned %d errors for valid x-managed aliases: %v", len(errs), errs)
	}
}

func TestValidate_XMLKeyModeOnNonXML(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","xml_key_mode":{"mode":"elements"}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for xml_key_mode on non-XML format")
	}
}

func TestValidate_XMLAttributesModeValid(t *testing.T) {
	input := `[{"path":"server.xml","format":"xml","category":"Core","xml_key_mode":{"mode":"attributes","element":"property","key_attr":"name","value_attr":"value"}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) != 0 {
		t.Errorf("ValidateConfigSchemas() returned %d errors for valid XML attributes mode: %v", len(errs), errs)
	}
}

func TestValidate_XMLAttributesModeMissingFields(t *testing.T) {
	input := `[{"path":"server.xml","format":"xml","category":"Core","xml_key_mode":{"mode":"attributes"}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for attributes mode without element/key_attr/value_attr")
	}
}

func TestValidate_XMLElementsModeValid(t *testing.T) {
	input := `[{"path":"server.xml","format":"xml","category":"Core","xml_key_mode":{"mode":"elements"}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) != 0 {
		t.Errorf("ValidateConfigSchemas() returned %d errors for valid XML elements mode: %v", len(errs), errs)
	}
}

func TestValidate_DuplicatePaths(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core"},{"path":"server.properties","format":"properties","category":"Core"}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for duplicate paths")
	}
}

func TestValidateConfigSchemas_AcceptsXGroup(t *testing.T) {
	schemasJSON := `[{
		"path": "server.properties",
		"format": "properties",
		"category": "Core",
		"schema": {
			"type": "object",
			"properties": {
				"port": {"type": "integer", "x-group": "network"},
				"motd": {"type": "string", "x-group": "gameplay"}
			}
		}
	}]`
	errs := ValidateConfigSchemas(schemasJSON)
	if len(errs) > 0 {
		t.Errorf("expected no validation errors, got: %v", errs)
	}
}

func TestValidateConfigSchemas_EnumLabels(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantMatch string
	}{
		{
			name: "valid enum labels matching enum length",
			input: `[{
				"path": "server.properties",
				"format": "properties",
				"category": "server",
				"schema": {
					"type": "object",
					"properties": {
						"level": {
							"type": "string",
							"enum": ["1", "2", "3"],
							"x-enum-labels": ["Low", "Medium", "High"]
						}
					}
				}
			}]`,
			wantCount: 0,
		},
		{
			name: "enum labels length mismatch",
			input: `[{
				"path": "server.properties",
				"format": "properties",
				"category": "server",
				"schema": {
					"type": "object",
					"properties": {
						"level": {
							"type": "string",
							"enum": ["1", "2", "3"],
							"x-enum-labels": ["Low", "High"]
						}
					}
				}
			}]`,
			wantCount: 1,
			wantMatch: "x-enum-labels",
		},
		{
			name: "enum labels without enum is an error",
			input: `[{
				"path": "server.properties",
				"format": "properties",
				"category": "server",
				"schema": {
					"type": "object",
					"properties": {
						"level": {
							"type": "string",
							"x-enum-labels": ["Low", "High"]
						}
					}
				}
			}]`,
			wantCount: 1,
			wantMatch: "x-enum-labels",
		},
		{
			name: "enum without labels is valid",
			input: `[{
				"path": "server.properties",
				"format": "properties",
				"category": "server",
				"schema": {
					"type": "object",
					"properties": {
						"level": {
							"type": "string",
							"enum": ["1", "2", "3"]
						}
					}
				}
			}]`,
			wantCount: 0,
		},
		{
			name: "empty enum labels is valid",
			input: `[{
				"path": "server.properties",
				"format": "properties",
				"category": "server",
				"schema": {
					"type": "object",
					"properties": {
						"level": {
							"type": "string",
							"enum": ["1", "2"],
							"x-enum-labels": []
						}
					}
				}
			}]`,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateConfigSchemas(tt.input)
			if len(errs) != tt.wantCount {
				t.Fatalf("ValidateConfigSchemas() returned %d errors, want %d: %v", len(errs), tt.wantCount, errs)
			}
			if tt.wantMatch != "" && tt.wantCount > 0 {
				found := false
				for _, e := range errs {
					if strings.Contains(e, tt.wantMatch) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", tt.wantMatch, errs)
				}
			}
		})
	}
}

func TestBuildFieldData_EnumLabels(t *testing.T) {
	tests := []struct {
		name           string
		prop           SchemaProperty
		wantEnumLabels []string
	}{
		{
			name: "copies enum labels from property",
			prop: SchemaProperty{
				Type:       "string",
				Enum:       []string{"1", "2", "3"},
				EnumLabels: []string{"Low", "Medium", "High"},
			},
			wantEnumLabels: []string{"Low", "Medium", "High"},
		},
		{
			name: "nil enum labels stays nil",
			prop: SchemaProperty{
				Type: "string",
				Enum: []string{"a", "b"},
			},
			wantEnumLabels: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			noopResolver := func(_ string) (string, bool) { return "", false }
			fd := buildFieldData("testkey", tt.prop, nil, noopResolver)
			if len(fd.EnumLabels) != len(tt.wantEnumLabels) {
				t.Fatalf("EnumLabels length = %d, want %d", len(fd.EnumLabels), len(tt.wantEnumLabels))
			}
			for i, label := range fd.EnumLabels {
				if label != tt.wantEnumLabels[i] {
					t.Errorf("EnumLabels[%d] = %q, want %q", i, label, tt.wantEnumLabels[i])
				}
			}
		})
	}
}

func TestValidate_AcceptsXOrderAndXGroups(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","x-groups":["network","gameplay"],"properties":{"port":{"type":"integer","x-group":"network","x-order":0},"motd":{"type":"string","x-group":"gameplay","x-order":1}}}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidate_NegativeXOrder(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","properties":{"port":{"type":"integer","x-order":-1}}}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for negative x-order")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e, "x-order") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error mentioning x-order, got: %v", errs)
	}
}

func TestValidate_DuplicateXGroups(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","x-groups":["network","network"],"properties":{}}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for duplicate x-groups")
	}
}

func TestValidate_EmptyStringInXGroups(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","x-groups":["network",""],"properties":{}}}]`
	errs := ValidateConfigSchemas(input)
	if len(errs) == 0 {
		t.Error("expected error for empty string in x-groups")
	}
}
