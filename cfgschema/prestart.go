package cfgschema

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/cfgparse"
)

// RunPreStart enforces managed field values and generates missing config files
// before a game server process starts. It does not block the start on errors —
// parse failures are logged as warnings and skipped.
func RunPreStart(serverDir string, schemasJSON string, resolver ManagedFieldResolver) {
	entries, errParse := ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		log.Warn().Err(errParse).Msg("Pre-start: failed to parse config schemas")
		return
	}

	for _, entry := range entries {
		processPreStartEntry(serverDir, entry, resolver)
	}
}

func processPreStartEntry(serverDir string, entry ConfigSchemaEntry, resolver ManagedFieldResolver) {
	if len(entry.ManagedFields) == 0 && !entry.GenerateBeforeStart {
		return
	}

	relativePath := strings.TrimPrefix(entry.Path, string(filepath.Separator))
	if relativePath != "" && !filepath.IsLocal(relativePath) {
		log.Warn().Str("path", entry.Path).Msg("Pre-start: invalid config path, skipping")
		return
	}

	cleanServerDir := filepath.Clean(serverDir)
	filePath := filepath.Clean(filepath.Join(cleanServerDir, relativePath))
	if filePath != cleanServerDir && !strings.HasPrefix(filePath, cleanServerDir+string(filepath.Separator)) {
		log.Warn().Str("path", entry.Path).Msg("Pre-start: config path escapes server directory, skipping")
		return
	}

	fileData, errRead := os.ReadFile(filePath)
	fileExists := errRead == nil

	if !fileExists && !entry.GenerateBeforeStart {
		return
	}

	p, errGetParser := cfgparse.GetParser(entry.Format)
	if errGetParser != nil {
		log.Warn().Err(errGetParser).Str("format", entry.Format).Str("path", entry.Path).
			Msg("Pre-start: unknown format, skipping")
		return
	}

	if !p.IsFlat() {
		// For structured formats, we'd need the dot-path adapter. For now,
		// skip structured format pre-start enforcement.
		log.Debug().Str("path", entry.Path).Msg("Pre-start: structured format support pending")
		return
	}

	var parsed []cfgparse.ConfigEntry

	if fileExists {
		var errParseFile error
		parsed, errParseFile = p.Flat.Parse(fileData)
		if errParseFile != nil {
			log.Warn().Err(errParseFile).Str("path", entry.Path).
				Msg("Pre-start: failed to parse config file, skipping")
			return
		}
	}

	if !fileExists && entry.GenerateBeforeStart {
		// Generate file from defaults.
		parsed = generateDefaultEntries(entry)
	}

	// Enforce managed fields.
	parsed = enforceManagedFields(parsed, entry.ManagedFields, resolver)

	// Write the file back.
	output, errWrite := p.Flat.Write(parsed)
	if errWrite != nil {
		log.Warn().Err(errWrite).Str("path", entry.Path).
			Msg("Pre-start: failed to write config file")
		return
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(filePath)
	errMkdir := os.MkdirAll(dir, 0o750)
	if errMkdir != nil {
		log.Warn().Err(errMkdir).Str("path", dir).
			Msg("Pre-start: failed to create directory")
		return
	}

	errWriteFile := os.WriteFile(filePath, output, 0o600) //nolint:gosec // filePath is validated as a local path and constrained to serverDir above.
	if errWriteFile != nil {
		log.Warn().Err(errWriteFile).Str("path", entry.Path).
			Msg("Pre-start: failed to write config file")
		return
	}

	log.Debug().Str("path", entry.Path).Msg("Pre-start: processed config file")
}

// generateDefaultEntries creates entries from schema defaults for a new file.
func generateDefaultEntries(entry ConfigSchemaEntry) []cfgparse.ConfigEntry {
	keys := SortedPropertyKeys(entry.Schema)
	result := make([]cfgparse.ConfigEntry, 0, len(keys))
	for i, key := range keys {
		prop := entry.Schema.Properties[key]
		result = append(result, cfgparse.ConfigEntry{
			Key:   key,
			Value: formatDefault(prop.Default),
			Index: i,
		})
	}
	return result
}

// enforceManagedFields overwrites managed field values in the parsed entries.
func enforceManagedFields(
	entries []cfgparse.ConfigEntry,
	managedFields map[string]string,
	resolver ManagedFieldResolver,
) []cfgparse.ConfigEntry {
	managed := map[string]string{}
	for key, source := range managedFields {
		value, ok := resolver(source)
		if !ok {
			log.Warn().Str("key", key).Str("source", source).
				Msg("Pre-start: unknown managed field source, skipping")
			continue
		}
		managed[key] = value
	}

	// Update existing entries.
	found := map[string]bool{}
	for i, e := range entries {
		val, isManaged := managed[e.Key]
		if isManaged {
			entries[i].Value = val
			found[e.Key] = true
		}
	}

	// Append managed fields that don't exist in the file.
	idx := len(entries)
	for key, val := range managed {
		if found[key] {
			continue
		}
		entries = append(entries, cfgparse.ConfigEntry{
			Key:   key,
			Value: val,
			Index: idx,
		})
		idx++
	}

	return entries
}
