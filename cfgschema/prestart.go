package cfgschema

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/cfgparse"
)

// PreStartFileStore abstracts config file IO so pre-start enforcement can run
// against either controller-local files or node-hosted remote files.
type PreStartFileStore interface {
	ReadFile(relativePath string) ([]byte, error)
	EnsureDir(relativePath string) error
	WriteFile(relativePath string, data []byte) error
}

type localPreStartFileStore struct {
	serverDir string
}

// RunPreStart enforces managed field values and generates missing config files
// before a game server process starts. It does not block the start on errors —
// parse failures are logged as warnings and skipped.
func RunPreStart(serverDir string, schemasJSON string, resolver ManagedFieldResolver) {
	store := localPreStartFileStore{serverDir: serverDir}
	RunPreStartWithStore(schemasJSON, resolver, store)
}

// RunPreStartWithStore enforces managed field values using the provided store.
func RunPreStartWithStore(schemasJSON string, resolver ManagedFieldResolver, store PreStartFileStore) {
	if store == nil {
		return
	}

	entries, errParse := ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		log.Warn().Err(errParse).Msg("Pre-start: failed to parse config schemas")
		return
	}

	for _, entry := range entries {
		processPreStartEntry(store, entry, resolver)
	}
}

func processPreStartEntry(store PreStartFileStore, entry ConfigSchemaEntry, resolver ManagedFieldResolver) {
	if len(entry.ManagedFields) == 0 && !entry.GenerateBeforeStart {
		return
	}

	relativePath, errPath := normalizePreStartRelativePath(entry.Path)
	if errPath != nil {
		log.Warn().Str("path", entry.Path).Msg("Pre-start: invalid config path, skipping")
		return
	}

	fileData, errRead := store.ReadFile(relativePath)
	fileExists := errRead == nil
	if errRead != nil && !errors.Is(errRead, os.ErrNotExist) {
		log.Warn().Err(errRead).Str("path", entry.Path).
			Msg("Pre-start: failed to read config file, skipping")
		return
	}

	if !fileExists && !entry.GenerateBeforeStart {
		return
	}

	p, errGetParser := cfgparse.GetParser(entry.Format)
	if errGetParser != nil {
		log.Warn().Err(errGetParser).Str("format", entry.Format).Str("path", entry.Path).
			Msg("Pre-start: unknown format, skipping")
		return
	}

	output, ok := preStartOutput(p, entry, fileData, fileExists, resolver)
	if !ok {
		return
	}

	// Ensure parent directory exists.
	dir := path.Dir(relativePath)
	errMkdir := store.EnsureDir(dir)
	if errMkdir != nil {
		log.Warn().Err(errMkdir).Str("path", dir).
			Msg("Pre-start: failed to create directory")
		return
	}

	errWriteFile := store.WriteFile(relativePath, output)
	if errWriteFile != nil {
		log.Warn().Err(errWriteFile).Str("path", entry.Path).
			Msg("Pre-start: failed to write config file")
		return
	}

	log.Debug().Str("path", entry.Path).Msg("Pre-start: processed config file")
}

func preStartOutput(
	p *cfgparse.Parser,
	entry ConfigSchemaEntry,
	fileData []byte,
	fileExists bool,
	resolver ManagedFieldResolver,
) ([]byte, bool) {
	if p.IsFlat() {
		output, ok := preStartFlatOutput(p, entry, fileData, fileExists, resolver)
		return output, ok
	}

	output, ok := preStartStructuredOutput(p, entry, fileData, fileExists, resolver)
	return output, ok
}

func preStartFlatOutput(
	p *cfgparse.Parser,
	entry ConfigSchemaEntry,
	fileData []byte,
	fileExists bool,
	resolver ManagedFieldResolver,
) ([]byte, bool) {
	var parsed []cfgparse.ConfigEntry

	if fileExists {
		var errParseFile error
		parsed, errParseFile = p.Flat.Parse(fileData)
		if errParseFile != nil {
			log.Warn().Err(errParseFile).Str("path", entry.Path).
				Msg("Pre-start: failed to parse config file, skipping")
			return nil, false
		}
	}

	if !fileExists && entry.GenerateBeforeStart {
		parsed = generateDefaultEntries(entry)
	}

	parsed = enforceManagedFields(parsed, entry.ManagedFields, resolver)

	output, errWrite := p.Flat.Write(parsed)
	if errWrite != nil {
		log.Warn().Err(errWrite).Str("path", entry.Path).
			Msg("Pre-start: failed to write config file")
		return nil, false
	}

	return output, true
}

func preStartStructuredOutput(
	p *cfgparse.Parser,
	entry ConfigSchemaEntry,
	fileData []byte,
	fileExists bool,
	resolver ManagedFieldResolver,
) ([]byte, bool) {
	var root *cfgparse.ConfigNode

	if fileExists {
		var errParseFile error
		root, errParseFile = p.Structured.Parse(fileData)
		if errParseFile != nil {
			log.Warn().Err(errParseFile).Str("path", entry.Path).
				Msg("Pre-start: failed to parse config file, skipping")
			return nil, false
		}
	} else {
		root = structuredDefaultRoot(entry)
	}

	enforceManagedStructuredFields(root, entry.ManagedFields, entry.Schema, resolver)

	output, errWrite := p.Structured.Write(root)
	if errWrite != nil {
		log.Warn().Err(errWrite).Str("path", entry.Path).
			Msg("Pre-start: failed to write config file")
		return nil, false
	}

	return output, true
}

func normalizePreStartRelativePath(entryPath string) (string, error) {
	normalizedPath := strings.ReplaceAll(strings.TrimSpace(entryPath), `\`, "/")
	if strings.HasPrefix(normalizedPath, "//") {
		return "", os.ErrInvalid
	}

	normalizedPath = strings.TrimPrefix(normalizedPath, "/")
	cleanedPath := path.Clean(normalizedPath)
	if cleanedPath == "." {
		return "", nil
	}
	if cleanedPath == ".." || strings.HasPrefix(cleanedPath, "../") {
		return "", os.ErrInvalid
	}
	if strings.HasPrefix(cleanedPath, "/") {
		return "", os.ErrInvalid
	}
	if hasPreStartWindowsDrivePrefix(cleanedPath) {
		return "", os.ErrInvalid
	}

	return cleanedPath, nil
}

func hasPreStartWindowsDrivePrefix(pathValue string) bool {
	if len(pathValue) < 2 {
		return false
	}
	if (pathValue[0] < 'A' || pathValue[0] > 'Z') && (pathValue[0] < 'a' || pathValue[0] > 'z') {
		return false
	}
	return pathValue[1] == ':'
}

func (store localPreStartFileStore) ReadFile(relativePath string) ([]byte, error) {
	filePath, errPath := store.resolve(relativePath)
	if errPath != nil {
		return nil, errPath
	}
	data, errRead := os.ReadFile(filePath)
	if errRead != nil {
		return nil, fmt.Errorf("cfgschema: read pre-start file: %w", errRead)
	}
	return data, nil
}

func (store localPreStartFileStore) EnsureDir(relativePath string) error {
	if relativePath == "" || relativePath == "." {
		return nil
	}

	dirPath, errPath := store.resolve(relativePath)
	if errPath != nil {
		return errPath
	}
	errMkdir := os.MkdirAll(dirPath, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("cfgschema: create pre-start directory: %w", errMkdir)
	}
	return nil
}

func (store localPreStartFileStore) WriteFile(relativePath string, data []byte) error {
	filePath, errPath := store.resolve(relativePath)
	if errPath != nil {
		return errPath
	}
	errWrite := os.WriteFile(filePath, data, 0o600)
	if errWrite != nil {
		return fmt.Errorf("cfgschema: write pre-start file: %w", errWrite)
	}
	return nil
}

func (store localPreStartFileStore) resolve(relativePath string) (string, error) {
	cleanServerDir := filepath.Clean(store.serverDir)
	filePath := filepath.Clean(filepath.Join(cleanServerDir, filepath.FromSlash(relativePath)))
	if filePath == cleanServerDir {
		return filePath, nil
	}

	serverDirPrefix := cleanServerDir + string(filepath.Separator)
	if !strings.HasPrefix(filePath, serverDirPrefix) {
		return "", os.ErrInvalid
	}
	return filePath, nil
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
