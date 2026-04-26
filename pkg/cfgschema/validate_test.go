package cfgschema

import (
	"strings"
	"testing"
)

func TestValidateConfigSchemas_BasicCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantMatch string
	}{
		{
			name:  "valid properties schema",
			input: `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","properties":{}}}]`,
		},
		{
			name:  "valid commandcfg schema",
			input: `[{"path":"server.cfg","format":"commandcfg","category":"Core","schema":{"type":"object","properties":{"hostname":{"type":"string"}}}}]`,
		},
		{
			name:  "empty array",
			input: `[]`,
		},
		{
			name:    "malformed json",
			input:   `{bad json`,
			wantErr: true,
		},
		{
			name:    "missing path",
			input:   `[{"format":"properties","category":"Core"}]`,
			wantErr: true,
		},
		{
			name:    "missing format",
			input:   `[{"path":"server.properties","category":"Core"}]`,
			wantErr: true,
		},
		{
			name:    "missing category",
			input:   `[{"path":"server.properties","format":"properties"}]`,
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   `[{"path":"test.cfg","format":"lua","category":"Core"}]`,
			wantErr: true,
		},
		{
			name:    "path traversal",
			input:   `[{"path":"../etc/passwd","format":"properties","category":"Core"}]`,
			wantErr: true,
		},
		{
			name:    "unknown managed source",
			input:   `[{"path":"server.properties","format":"properties","category":"Core","managed_fields":{"key":"unknown.source"}}]`,
			wantErr: true,
		},
		{
			name:  "valid managed sources",
			input: `[{"path":"server.properties","format":"properties","category":"Core","managed_fields":{"server-ip":"game_server.ip","server-port":"game_server.port","query-port":"game_server.query_port"}}]`,
		},
		{
			name:  "valid managed source aliases",
			input: `[{"path":"server.properties","format":"properties","category":"Core","managed_fields":{"server-ip":"ip","server-port":"server_port","query-port":"query_port"}}]`,
		},
		{
			name: "unknown managed source in schema property",
			input: `[{
				"path":"server.properties",
				"format":"properties",
				"category":"Core",
				"schema":{
					"type":"object",
					"properties":{
						"server-ip":{"type":"string","x-managed":{"source":"unknown.source"}}
					}
				}
			}]`,
			wantErr: true,
		},
		{
			name: "valid managed source aliases in schema property",
			input: `[{
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
			}]`,
		},
		{
			name:    "xml key mode on non xml",
			input:   `[{"path":"server.properties","format":"properties","category":"Core","xml_key_mode":{"mode":"elements"}}]`,
			wantErr: true,
		},
		{
			name:  "valid xml attributes mode",
			input: `[{"path":"server.xml","format":"xml","category":"Core","xml_key_mode":{"mode":"attributes","element":"property","key_attr":"name","value_attr":"value"}}]`,
		},
		{
			name:    "xml attributes mode missing fields",
			input:   `[{"path":"server.xml","format":"xml","category":"Core","xml_key_mode":{"mode":"attributes"}}]`,
			wantErr: true,
		},
		{
			name:  "valid xml elements mode",
			input: `[{"path":"server.xml","format":"xml","category":"Core","xml_key_mode":{"mode":"elements"}}]`,
		},
		{
			name:    "duplicate paths",
			input:   `[{"path":"server.properties","format":"properties","category":"Core"},{"path":"server.properties","format":"properties","category":"Core"}]`,
			wantErr: true,
		},
		{
			name: "accepts x-group",
			input: `[{
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
			}]`,
		},
		{
			name:  "accepts x-order and x-groups",
			input: `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","x-groups":["network","gameplay"],"properties":{"port":{"type":"integer","x-group":"network","x-order":0},"motd":{"type":"string","x-group":"gameplay","x-order":1}}}}]`,
		},
		{
			name:      "negative x-order",
			input:     `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","properties":{"port":{"type":"integer","x-order":-1}}}}]`,
			wantErr:   true,
			wantMatch: "x-order",
		},
		{
			name:    "duplicate x-groups",
			input:   `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","x-groups":["network","network"],"properties":{}}}]`,
			wantErr: true,
		},
		{
			name:    "empty string in x-groups",
			input:   `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","x-groups":["network",""],"properties":{}}}]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateConfigSchemas(tt.input)
			if tt.wantErr && len(errs) == 0 {
				t.Fatal("ValidateConfigSchemas() errors = 0, want at least 1")
			}
			if !tt.wantErr && len(errs) != 0 {
				t.Fatalf("ValidateConfigSchemas() returned %d errors, want 0: %v", len(errs), errs)
			}
			if tt.wantMatch == "" {
				return
			}

			found := false
			for _, errText := range errs {
				if strings.Contains(errText, tt.wantMatch) {
					found = true
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got: %v", tt.wantMatch, errs)
			}
		})
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
