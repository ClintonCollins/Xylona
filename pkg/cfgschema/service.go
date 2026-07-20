package cfgschema

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/cfgparse"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
)

// SchemaDefinition represents a parsed JSON Schema for a config file.
type SchemaDefinition struct {
	Type       string                    `json:"type"`
	Groups     []string                  `json:"x-groups,omitempty"`
	Properties map[string]SchemaProperty `json:"properties"`
}

// SchemaProperty represents a single field in the JSON Schema.
type SchemaProperty struct {
	Type          string             `json:"type"`
	Title         string             `json:"title"`
	Description   string             `json:"description"`
	Default       any                `json:"default"`
	Enum          []string           `json:"enum"`
	EnumLabels    []string           `json:"x-enum-labels"`
	Minimum       *int64             `json:"minimum"`
	Maximum       *int64             `json:"maximum"`
	MaxLength     *int32             `json:"maxLength"`
	Required      bool               `json:"required"`
	AllowMultiple bool               `json:"x-allow-multiple"`
	Group         string             `json:"x-group,omitempty"`
	Order         *int32             `json:"x-order,omitempty"`
	Managed       *managedFieldEntry `json:"x-managed,omitempty"`
}

// ConfigSchemaEntry represents a single entry in the config_schemas JSON array,
// used by the schema service for processing.
type ConfigSchemaEntry struct {
	Path                string            `json:"path"`
	PlatformPaths       map[string]string `json:"platform_paths,omitempty"`
	Format              string            `json:"format"`
	Category            string            `json:"category"`
	GenerateBeforeStart bool              `json:"generate_before_start"`
	ManagedFields       map[string]string `json:"managed_fields"`
	XMLKeyMode          *xmlKeyModeEntry  `json:"xml_key_mode"`
	Schema              SchemaDefinition  `json:"schema"`
}

// ResolvePlatformPath returns the platform-specific path for an entry when one
// is configured, otherwise it returns the entry's default path.
func ResolvePlatformPath(entry ConfigSchemaEntry, platform string) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	platformPath := strings.TrimSpace(entry.PlatformPaths[platform])
	if platformPath != "" {
		return platformPath
	}
	return entry.Path
}

// HasPlatformPaths reports whether any entry requires target-platform path
// resolution.
func HasPlatformPaths(entries []ConfigSchemaEntry) bool {
	for _, entry := range entries {
		if len(entry.PlatformPaths) > 0 {
			return true
		}
	}
	return false
}

// ResolvePlatformConfigSchemas rewrites config schema paths for a target node
// platform without changing the stored game definition.
func ResolvePlatformConfigSchemas(schemasJSON string, platform string) (string, error) {
	entries, errParse := ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return "", errParse
	}
	for index := range entries {
		entries[index].Path = ResolvePlatformPath(entries[index], platform)
	}
	data, errMarshal := json.Marshal(entries)
	if errMarshal != nil {
		return "", fmt.Errorf("marshal platform config schemas: %w", errMarshal)
	}
	return string(data), nil
}

type managedFieldEntry struct {
	Source string `json:"source"`
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
	for i := range entries {
		entries[i].Schema = normalizeSchemaManagedSources(entries[i].Schema)
		entries[i].ManagedFields = mergeManagedFields(entries[i].ManagedFields, entries[i].Schema)
	}
	return entries, nil
}

func mergeManagedFields(existing map[string]string, schema SchemaDefinition) map[string]string {
	managedFields := make(map[string]string)
	for key, value := range existing {
		managedFields[key] = normalizeManagedSource(value)
	}

	for key, prop := range schema.Properties {
		if prop.Managed == nil || prop.Managed.Source == "" {
			continue
		}
		if _, exists := managedFields[key]; exists {
			continue
		}
		managedFields[key] = normalizeManagedSource(prop.Managed.Source)
	}

	if len(managedFields) == 0 {
		return nil
	}

	return managedFields
}

func normalizeSchemaManagedSources(schema SchemaDefinition) SchemaDefinition {
	if len(schema.Properties) == 0 {
		return schema
	}

	normalizedSchema := schema
	normalizedSchema.Properties = make(map[string]SchemaProperty, len(schema.Properties))
	for key, prop := range schema.Properties {
		if prop.Managed != nil && prop.Managed.Source != "" {
			prop.Managed = &managedFieldEntry{Source: normalizeManagedSource(prop.Managed.Source)}
		}

		normalizedSchema.Properties[key] = prop
	}

	return normalizedSchema
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
	ManagedSource     string
	IsMissingFromFile bool
	EnumOptions       []string
	EnumLabels        []string
	Minimum           *int64
	Maximum           *int64
	MaxLength         *int32
	Required          bool
	AllowMultiple     bool
	Values            []string
	Group             string
	Order             *int32
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

// GameServerSettings contains server values that can own generated config fields.
type GameServerSettings struct {
	Name                   string
	Directory              string
	IP                     string
	Port                   int64
	QueryPort              int64
	MaxPlayers             int64
	LocalConsoleConfigured bool
	LocalConsoleEnabled    bool
	LocalConsolePort       int64
	LocalConsolePassword   string
}

// GameServerSettingsResolver creates a ManagedFieldResolver from all supported server settings.
func GameServerSettingsResolver(settings GameServerSettings) ManagedFieldResolver {
	localConsoleEnabled := true
	if settings.LocalConsoleConfigured {
		localConsoleEnabled = settings.LocalConsoleEnabled
	}
	sources := map[string]string{
		"game_server.ip":                settings.IP,
		"game_server.port":              strconv.FormatInt(settings.Port, 10),
		"game_server.port_plus_1":       strconv.FormatInt(settings.Port+1, 10),
		"game_server.port_plus_2":       strconv.FormatInt(settings.Port+2, 10),
		"game_server.query_port":        strconv.FormatInt(settings.QueryPort, 10),
		"game_server.query_port_plus_1": strconv.FormatInt(settings.QueryPort+1, 10),
		"game_server.max_players":       strconv.FormatInt(settings.MaxPlayers, 10),
		"game_server.server_name":       settings.Name,
		"game_server.directory":         settings.Directory,
		"xylona.local_console_enabled":  strconv.FormatBool(localConsoleEnabled),
		"xylona.local_console_port":     strconv.FormatInt(settings.LocalConsolePort, 10),
		"xylona.local_console_password": settings.LocalConsolePassword,
	}
	return func(source string) (string, bool) {
		v, ok := sources[normalizeManagedSource(source)]
		return v, ok
	}
}

// WithoutManagedSources removes selected managed ownership rules from a schema
// copy. It is used when a capability is disabled and existing user-owned
// configuration must remain untouched during pre-start processing.
func WithoutManagedSources(schemasJSON string, sources ...string) (string, error) {
	entries, errParse := ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return "", errParse
	}
	removed := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		removed[normalizeManagedSource(source)] = struct{}{}
	}
	for entryIndex := range entries {
		entry := &entries[entryIndex]
		for key, source := range entry.ManagedFields {
			_, shouldRemove := removed[normalizeManagedSource(source)]
			if shouldRemove {
				delete(entry.ManagedFields, key)
			}
		}
		for key, property := range entry.Schema.Properties {
			if property.Managed == nil {
				continue
			}
			_, shouldRemove := removed[normalizeManagedSource(property.Managed.Source)]
			if !shouldRemove {
				continue
			}
			property.Managed = nil
			entry.Schema.Properties[key] = property
		}
	}
	encoded, errMarshal := json.Marshal(entries)
	if errMarshal != nil {
		return "", fmt.Errorf("marshal config schemas without managed sources: %w", errMarshal)
	}
	return string(encoded), nil
}

// ServerSettingsResolver creates a ManagedFieldResolver from server settings.
func ServerSettingsResolver(ip string, port int64, queryPort int64) ManagedFieldResolver {
	return GameServerSettingsResolver(GameServerSettings{
		IP:                     ip,
		Port:                   port,
		QueryPort:              queryPort,
		LocalConsoleConfigured: true,
		LocalConsoleEnabled:    true,
		LocalConsolePort:       queryPort + 1,
	})
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

	// Phase 2: Append schema-only fields sorted by x-order, then alphabetically.
	schemaOnlyKeys := SortedPropertyKeys(schema)

	for _, key := range schemaOnlyKeys {
		if matched[key] {
			continue
		}
		prop := schema.Properties[key]
		fd := buildFieldData(key, prop, managedFields, resolver)
		fd.IsMissingFromFile = true
		if !fd.IsManaged && fd.Value == "" {
			fd.Value = fd.DefaultValue
		}
		fields = append(fields, fd)
	}

	fields = SortFieldsBySchema(fields, schema)

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
		EnumLabels:    prop.EnumLabels,
		Minimum:       prop.Minimum,
		Maximum:       prop.Maximum,
		MaxLength:     prop.MaxLength,
		Required:      prop.Required,
		AllowMultiple: prop.AllowMultiple,
		Group:         prop.Group,
		Order:         prop.Order,
	}

	// Check if this is a managed field.
	source, isManaged := managedFields[key]
	if isManaged {
		source = normalizeManagedSource(source)
		fd.IsManaged = true
		fd.ManagedSource = source
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
	_ SchemaDefinition,
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
		switch {
		case inSchema && !updated[e.Key] && fd.IsManaged:
			// Managed fields keep their resolved values.
			result = append(result, cfgparse.ConfigEntry{
				Key:     e.Key,
				Value:   fd.Value,
				Index:   e.Index,
				Section: e.Section,
				Comment: e.Comment,
			})
			updated[e.Key] = true
		case inSchema && !updated[e.Key] && fd.AllowMultiple && len(fd.Values) > 0:
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
			updated[e.Key] = true
		case inSchema && !updated[e.Key]:
			result = append(result, cfgparse.ConfigEntry{
				Key:     e.Key,
				Value:   fd.Value,
				Index:   e.Index,
				Section: e.Section,
				Comment: e.Comment,
			})
			updated[e.Key] = true
		case inSchema && updated[e.Key]:
			// Skip duplicate entries for keys we've already processed
			// (unless allow-multiple, handled above).
			continue
		default:
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
			if prop.MaxLength != nil && helpers.ClampInt32FromInt(len(value)) > *prop.MaxLength {
				errs = append(errs, ValidationError{
					Field:   f.Key,
					Message: fmt.Sprintf("value length %d exceeds maximum %d", len(value), *prop.MaxLength),
				})
			}
		}

		// Enum values are suggestions, not constraints. Mods and plugins may
		// require values outside the predefined list, so we do not reject them.
	}

	return errs
}
