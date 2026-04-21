package cfgschema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// validFormats lists the supported config file format strings.
var validFormats = map[string]bool{
	"properties": true,
	"ini":        true,
	"json":       true,
	"yaml":       true,
	"toml":       true,
	"xml":        true,
	"commandcfg": true,
}

// configSchemaEntry represents a single entry in the config_schemas JSON array.
type configSchemaEntry struct {
	Path                string            `json:"path"`
	Format              string            `json:"format"`
	Category            string            `json:"category"`
	GenerateBeforeStart bool              `json:"generate_before_start"`
	ManagedFields       map[string]string `json:"managed_fields"`
	XMLKeyMode          *xmlKeyModeEntry  `json:"xml_key_mode"`
	Schema              json.RawMessage   `json:"schema"`
}

type xmlKeyModeEntry struct {
	Mode      string `json:"mode"`
	Element   string `json:"element"`
	KeyAttr   string `json:"key_attr"`
	ValueAttr string `json:"value_attr"`
}

// ValidateConfigSchemas validates the config_schemas JSON blob stored on a game
// definition. It returns a slice of human-readable error strings. An empty slice
// means the input is valid.
func ValidateConfigSchemas(schemasJSON string) []string {
	var entries []configSchemaEntry
	errUnmarshal := json.Unmarshal([]byte(schemasJSON), &entries)
	if errUnmarshal != nil {
		return []string{fmt.Sprintf("invalid JSON: %v", errUnmarshal)}
	}

	var errs []string
	seenPaths := map[string]bool{}

	for i, entry := range entries {
		prefix := fmt.Sprintf("entry[%d]", i)

		errs = append(errs, validateEntryBasics(prefix, entry, seenPaths)...)
		errs = append(errs, validateManagedFields(prefix, entry.ManagedFields)...)
		errs = append(errs, validateXMLKeyMode(prefix, entry)...)
		errs = append(errs, validateSchemaProperties(prefix, entry.Schema)...)
		errs = append(errs, validateSchemaGroups(prefix, entry.Schema)...)
		errs = append(errs, validateSchemaOrder(prefix, entry.Schema)...)
	}

	return errs
}

func validateEntryBasics(prefix string, entry configSchemaEntry, seenPaths map[string]bool) []string {
	var errs []string
	if entry.Path == "" {
		errs = append(errs, fmt.Sprintf("%s: path is required", prefix))
	} else {
		if strings.Contains(entry.Path, "..") {
			errs = append(errs, fmt.Sprintf("%s: path must not contain '..' traversal", prefix))
		}
		if seenPaths[entry.Path] {
			errs = append(errs, fmt.Sprintf("%s: duplicate path %q", prefix, entry.Path))
		}
		seenPaths[entry.Path] = true
	}

	if entry.Format == "" {
		errs = append(errs, fmt.Sprintf("%s: format is required", prefix))
	} else if !validFormats[entry.Format] {
		errs = append(errs, fmt.Sprintf("%s: invalid format %q", prefix, entry.Format))
	}

	if entry.Category == "" {
		errs = append(errs, fmt.Sprintf("%s: category is required", prefix))
	}
	return errs
}

func validateManagedFields(prefix string, managedFields map[string]string) []string {
	var errs []string
	for key, source := range managedFields {
		if !isKnownManagedSource(source) {
			errs = append(errs, fmt.Sprintf("%s: unknown managed field source %q for key %q", prefix, source, key))
		}
	}
	return errs
}

func validateXMLKeyMode(prefix string, entry configSchemaEntry) []string {
	if entry.XMLKeyMode == nil {
		return nil
	}
	if entry.Format != "xml" {
		return []string{fmt.Sprintf("%s: xml_key_mode is only valid for xml format", prefix)}
	}
	if entry.XMLKeyMode.Mode != "attributes" {
		return nil
	}
	if entry.XMLKeyMode.Element == "" || entry.XMLKeyMode.KeyAttr == "" || entry.XMLKeyMode.ValueAttr == "" {
		return []string{fmt.Sprintf("%s: xml_key_mode attributes mode requires element, key_attr, and value_attr", prefix)}
	}
	return nil
}

func validateSchemaProperties(prefix string, schema json.RawMessage) []string {
	if len(schema) == 0 {
		return nil
	}

	var schemaDef struct {
		Properties map[string]struct {
			Enum       []any    `json:"enum"`
			EnumLabels []string `json:"x-enum-labels"`
			Managed    *struct {
				Source string `json:"source"`
			} `json:"x-managed"`
		} `json:"properties"`
	}
	errSchemaParse := json.Unmarshal(schema, &schemaDef)
	if errSchemaParse != nil || schemaDef.Properties == nil {
		return nil
	}

	var errs []string
	for propKey, prop := range schemaDef.Properties {
		if prop.Managed != nil && !isKnownManagedSource(prop.Managed.Source) {
			errs = append(errs, fmt.Sprintf(
				"%s: unknown managed field source %q for key %q",
				prefix, prop.Managed.Source, propKey))
		}
		if len(prop.EnumLabels) > 0 && len(prop.Enum) == 0 {
			errs = append(errs, fmt.Sprintf(
				"%s: property %q has x-enum-labels but no enum values",
				prefix, propKey))
		}
		if len(prop.EnumLabels) > 0 && len(prop.Enum) > 0 && len(prop.EnumLabels) != len(prop.Enum) {
			errs = append(errs, fmt.Sprintf(
				"%s: property %q x-enum-labels length (%d) must match enum length (%d)",
				prefix, propKey, len(prop.EnumLabels), len(prop.Enum)))
		}
	}
	return errs
}

func validateSchemaGroups(prefix string, schema json.RawMessage) []string {
	var schemaWithGroups struct {
		Groups []string `json:"x-groups"`
	}
	if len(schema) > 0 {
		_ = json.Unmarshal(schema, &schemaWithGroups)
	}

	var errs []string
	seenGroups := map[string]bool{}
	for _, group := range schemaWithGroups.Groups {
		if group == "" {
			errs = append(errs, fmt.Sprintf("%s: x-groups must not contain empty strings", prefix))
		}
		if seenGroups[group] {
			errs = append(errs, fmt.Sprintf("%s: duplicate group %q in x-groups", prefix, group))
		}
		seenGroups[group] = true
	}
	return errs
}

func validateSchemaOrder(prefix string, schema json.RawMessage) []string {
	var schemaWithOrder struct {
		Properties map[string]struct {
			Order *int32 `json:"x-order"`
		} `json:"properties"`
	}
	if len(schema) > 0 {
		_ = json.Unmarshal(schema, &schemaWithOrder)
	}

	var errs []string
	for propKey, prop := range schemaWithOrder.Properties {
		if prop.Order != nil && *prop.Order < 0 {
			errs = append(errs, fmt.Sprintf(
				"%s: property %q x-order must be non-negative, got %d",
				prefix, propKey, *prop.Order))
		}
	}
	return errs
}
