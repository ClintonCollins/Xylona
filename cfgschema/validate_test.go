package cfgschema

import "testing"

func TestValidate_ValidSchema(t *testing.T) {
	input := `[{"path":"server.properties","format":"properties","category":"Core","schema":{"type":"object","properties":{}}}]`
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
