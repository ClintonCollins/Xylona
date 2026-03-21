package cfgschema

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/cfgparse"
)

// SchemaDefinition represents a parsed JSON Schema for a config file.
type SchemaDefinition struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaProperty `json:"properties"`
}

// SchemaProperty represents a single field in the JSON Schema.
type SchemaProperty struct {
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Default       any      `json:"default"`
	Enum          []string `json:"enum"`
	Minimum       *int64   `json:"minimum"`
	Maximum       *int64   `json:"maximum"`
	MaxLength     *int32   `json:"maxLength"`
	Required      bool     `json:"required"`
	AllowMultiple bool     `json:"x-allow-multiple"`
	Group         string   `json:"x-group,omitempty"`
}

// ConfigSchemaEntry represents a single entry in the config_schemas JSON array,
// used by the schema service for processing.
type ConfigSchemaEntry struct {
	Path                string            `json:"path"`
	Format              string            `json:"format"`
	Category            string            `json:"category"`
	GenerateBeforeStart bool              `json:"generate_before_start"`
	ManagedFields       map[string]string `json:"managed_fields"`
	XMLKeyMode          *xmlKeyModeEntry  `json:"xml_key_mode"`
	Schema              SchemaDefinition  `json:"schema"`
}

// ParseConfigSchemas parses the config_schemas JSON blob into a slice of entries.
func ParseConfigSchemas(schemasJSON string) ([]ConfigSchemaEntry, error) {
	if schemasJSON == "" {
		return nil, nil
	}
	var entries []ConfigSchemaEntry
	errUnmarshal := json.Unmarshal([]byte(schemasJSON), &entries)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parsing config schemas: %w", errUnmarshal)
	}
	return entries, nil
}

// FieldData represents a config field matched to its schema definition.
type FieldData struct {
	Key               string
	Value             string
	Title             string
	Description       string
	FieldType         string
	DefaultValue      string
	IsManaged         bool
	IsMissingFromFile bool
	EnumOptions       []string
	Minimum           *int64
	Maximum           *int64
	MaxLength         *int32
	Required          bool
	AllowMultiple     bool
	Values            []string
	Group             string
}

// AdvancedFieldData represents a config entry not matched by the schema.
type AdvancedFieldData struct {
	Key     string
	Value   string
	Section string
}

// MatchFieldsResult contains the output of MatchFields.
type MatchFieldsResult struct {
	Fields         []FieldData
	AdvancedFields []AdvancedFieldData
}

// ManagedFieldResolver resolves managed field source paths to values.
type ManagedFieldResolver func(source string) (string, bool)

// ServerSettingsResolver creates a ManagedFieldResolver from server settings.
func ServerSettingsResolver(ip string, port int64, queryPort int64) ManagedFieldResolver {
	sources := map[string]string{
		"game_server.ip":         ip,
		"game_server.port":       strconv.FormatInt(port, 10),
		"game_server.query_port": strconv.FormatInt(queryPort, 10),
	}
	return func(source string) (string, bool) {
		v, ok := sources[source]
		return v, ok
	}
}

// MatchFields matches parsed config entries against a JSON Schema and returns
// structured field data. Entries not in the schema become advanced fields.
// Managed fields are resolved via the resolver and marked as read-only.
//
// Fields are returned in two phases:
// Phase 1: Entries from the parsed config in file order (preserving original ordering).
// Phase 2: Schema-only fields (not in config) appended alphabetically.
func MatchFields(
	entries []cfgparse.ConfigEntry,
	schema SchemaDefinition,
	managedFields map[string]string,
	resolver ManagedFieldResolver,
) MatchFieldsResult {
	// Index entries by key for lookup. Track multi-value keys.
	multiValues := map[string][]string{}
	entryMap := map[string]cfgparse.ConfigEntry{}
	for _, e := range entries {
		_, exists := entryMap[e.Key]
		if exists {
			multiValues[e.Key] = append(multiValues[e.Key], e.Value)
		} else {
			entryMap[e.Key] = e
			multiValues[e.Key] = []string{e.Value}
		}
	}

	var fields []FieldData
	matched := map[string]bool{}

	// Phase 1: Iterate parsed entries in file order.
	var advanced []AdvancedFieldData

	for _, e := range entries {
		// Skip duplicate keys (already processed first occurrence).
		if matched[e.Key] {
			continue
		}
		matched[e.Key] = true

		prop, inSchema := schema.Properties[e.Key]
		if !inSchema {
			// Only add first occurrence to advanced fields.
			if e.Index == 0 {
				advanced = append(advanced, AdvancedFieldData{
					Key:     e.Key,
					Value:   e.Value,
					Section: e.Section,
				})
			}
			continue
		}

		fd := buildFieldData(e.Key, prop, managedFields, resolver)

		// Fill value from parsed entry.
		if !fd.IsManaged {
			fd.Value = e.Value
		}
		if prop.AllowMultiple {
			fd.Values = multiValues[e.Key]
		}

		fields = append(fields, fd)
	}

	// Phase 2: Append schema-only fields (not in config file), sorted alphabetically.
	var schemaOnlyKeys []string
	for key := range schema.Properties {
		if !matched[key] {
			schemaOnlyKeys = append(schemaOnlyKeys, key)
		}
	}
	slices.Sort(schemaOnlyKeys)

	for _, key := range schemaOnlyKeys {
		prop := schema.Properties[key]
		fd := buildFieldData(key, prop, managedFields, resolver)
		fd.IsMissingFromFile = true
		if !fd.IsManaged && fd.Value == "" {
			fd.Value = fd.DefaultValue
		}
		fields = append(fields, fd)
	}

	return MatchFieldsResult{
		Fields:         fields,
		AdvancedFields: advanced,
	}
}

// buildFieldData creates a FieldData from a schema property, resolving managed fields.
func buildFieldData(
	key string,
	prop SchemaProperty,
	managedFields map[string]string,
	resolver ManagedFieldResolver,
) FieldData {
	fd := FieldData{
		Key:           key,
		Title:         prop.Title,
		Description:   prop.Description,
		FieldType:     prop.Type,
		DefaultValue:  formatDefault(prop.Default),
		EnumOptions:   prop.Enum,
		Minimum:       prop.Minimum,
		Maximum:       prop.Maximum,
		MaxLength:     prop.MaxLength,
		Required:      prop.Required,
		AllowMultiple: prop.AllowMultiple,
		Group:         prop.Group,
	}

	// Check if this is a managed field.
	source, isManaged := managedFields[key]
	if isManaged {
		fd.IsManaged = true
		resolvedValue, ok := resolver(source)
		if ok {
			fd.Value = resolvedValue
		} else {
			log.Warn().Str("key", key).Str("source", source).Msg("Unknown managed field source")
		}
	}

	return fd
}

// formatDefault converts a JSON Schema default value to a string.
func formatDefault(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// MergeAndWrite merges updated field values into existing parsed entries,
// preserving comments, ordering, and unknown fields.
func MergeAndWrite(
	existingEntries []cfgparse.ConfigEntry,
	updatedFields []FieldData,
	advancedFields []AdvancedFieldData,
	schema SchemaDefinition,
) []cfgparse.ConfigEntry {
	// Build a map of updated field values.
	fieldValues := map[string]FieldData{}
	for _, f := range updatedFields {
		fieldValues[f.Key] = f
	}

	// First pass: update existing entries in place.
	updated := map[string]bool{}
	var result []cfgparse.ConfigEntry

	for _, e := range existingEntries {
		fd, inSchema := fieldValues[e.Key]
		if inSchema && !updated[e.Key] {
			if fd.IsManaged {
				// Managed fields keep their resolved values.
				result = append(result, cfgparse.ConfigEntry{
					Key:     e.Key,
					Value:   fd.Value,
					Index:   e.Index,
					Section: e.Section,
					Comment: e.Comment,
				})
			} else if fd.AllowMultiple && len(fd.Values) > 0 {
				// Write all values for allow-multiple fields.
				for i, v := range fd.Values {
					result = append(result, cfgparse.ConfigEntry{
						Key:     e.Key,
						Value:   v,
						Index:   i,
						Section: e.Section,
						Comment: func() string {
							if i == 0 {
								return e.Comment
							}
							return ""
						}(),
					})
				}
			} else {
				result = append(result, cfgparse.ConfigEntry{
					Key:     e.Key,
					Value:   fd.Value,
					Index:   e.Index,
					Section: e.Section,
					Comment: e.Comment,
				})
			}
			updated[e.Key] = true
		} else if inSchema && updated[e.Key] {
			// Skip duplicate entries for keys we've already processed
			// (unless allow-multiple, handled above).
			continue
		} else {
			// Not in schema — check if it's in advanced fields.
			advFound := false
			for _, af := range advancedFields {
				if af.Key == e.Key {
					result = append(result, cfgparse.ConfigEntry{
						Key:     e.Key,
						Value:   af.Value,
						Index:   e.Index,
						Section: e.Section,
						Comment: e.Comment,
					})
					advFound = true
					break
				}
			}
			if !advFound {
				// Preserve the existing entry as-is.
				result = append(result, e)
			}
		}
	}

	// Append new fields that weren't in the existing entries.
	for _, fd := range updatedFields {
		if updated[fd.Key] || fd.IsManaged {
			continue
		}
		if fd.IsMissingFromFile && fd.Value != "" {
			result = append(result, cfgparse.ConfigEntry{
				Key:   fd.Key,
				Value: fd.Value,
			})
		}
	}

	return result
}

// ValidationError represents a validation failure for a single field.
type ValidationError struct {
	Field   string
	Message string
}

// ValidateFields validates field values against their schema constraints.
func ValidateFields(fields []FieldData, schema SchemaDefinition) []ValidationError {
	var errs []ValidationError

	for _, f := range fields {
		prop, exists := schema.Properties[f.Key]
		if !exists {
			continue
		}

		// Skip managed fields — their values come from the system.
		if f.IsManaged {
			continue
		}

		value := f.Value

		// Required check.
		if prop.Required && value == "" {
			errs = append(errs, ValidationError{
				Field:   f.Key,
				Message: "field is required",
			})
			continue
		}

		if value == "" {
			continue
		}

		// Type-specific validation.
		switch prop.Type {
		case "integer":
			n, errParse := strconv.ParseInt(value, 10, 64)
			if errParse != nil {
				errs = append(errs, ValidationError{
					Field:   f.Key,
					Message: fmt.Sprintf("invalid integer: %s", value),
				})
				continue
			}
			if prop.Minimum != nil && n < *prop.Minimum {
				errs = append(errs, ValidationError{
					Field:   f.Key,
					Message: fmt.Sprintf("value %d is below minimum %d", n, *prop.Minimum),
				})
			}
			if prop.Maximum != nil && n > *prop.Maximum {
				errs = append(errs, ValidationError{
					Field:   f.Key,
					Message: fmt.Sprintf("value %d exceeds maximum %d", n, *prop.Maximum),
				})
			}

		case "string":
			if prop.MaxLength != nil && int32(len(value)) > *prop.MaxLength {
				errs = append(errs, ValidationError{
					Field:   f.Key,
					Message: fmt.Sprintf("value length %d exceeds maximum %d", len(value), *prop.MaxLength),
				})
			}
		}

		// Enum check (applies to any type).
		if len(prop.Enum) > 0 && !slices.Contains(prop.Enum, value) {
			errs = append(errs, ValidationError{
				Field:   f.Key,
				Message: fmt.Sprintf("value %q is not a valid option", value),
			})
		}
	}

	return errs
}
