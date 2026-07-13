package cfgschema

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/cfgparse"
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
// before a game server process starts. It does not block the start on errors.
func RunPreStart(serverDir string, schemasJSON string, resolver ManagedFieldResolver) {
	errRun := RunPreStartStrict(serverDir, schemasJSON, resolver)
	if errRun != nil {
		log.Warn().Err(errRun).Msg("Pre-start: one or more config files could not be processed")
	}
}

// RunPreStartWithStore enforces managed field values using the provided store.
func RunPreStartWithStore(schemasJSON string, resolver ManagedFieldResolver, store PreStartFileStore) {
	errRun := RunPreStartWithStoreStrict(schemasJSON, resolver, store)
	if errRun != nil {
		log.Warn().Err(errRun).Msg("Pre-start: one or more config files could not be processed")
	}
}

// RunPreStartStrict enforces managed fields and reports every processing
// failure to the caller.
func RunPreStartStrict(serverDir string, schemasJSON string, resolver ManagedFieldResolver) error {
	store := localPreStartFileStore{serverDir: serverDir}
	return RunPreStartWithStoreStrict(schemasJSON, resolver, store)
}

// RunPreStartWithStoreStrict is the error-returning store-backed pre-start
// path used when a game cannot safely launch with stale managed values.
func RunPreStartWithStoreStrict(schemasJSON string, resolver ManagedFieldResolver, store PreStartFileStore) error {
	if store == nil {
		return errors.New("cfgschema: pre-start file store is required")
	}

	entries, errParse := ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return fmt.Errorf("cfgschema: parse pre-start config schemas: %w", errParse)
	}

	var result error
	for _, entry := range entries {
		errProcess := processPreStartEntry(store, entry, resolver)
		if errProcess != nil {
			result = errors.Join(result, errProcess)
		}
	}
	return result
}

func processPreStartEntry(store PreStartFileStore, entry ConfigSchemaEntry, resolver ManagedFieldResolver) error {
	if len(entry.ManagedFields) == 0 && !entry.GenerateBeforeStart {
		return nil
	}

	relativePath, errPath := normalizePreStartRelativePath(entry.Path)
	if errPath != nil {
		return fmt.Errorf("cfgschema: invalid pre-start config path %q: %w", entry.Path, errPath)
	}

	fileData, errRead := store.ReadFile(relativePath)
	fileExists := errRead == nil
	if errRead != nil && !errors.Is(errRead, os.ErrNotExist) {
		return fmt.Errorf("cfgschema: read pre-start config %q: %w", entry.Path, errRead)
	}

	if !fileExists && !entry.GenerateBeforeStart {
		return fmt.Errorf("cfgschema: required pre-start config %q does not exist: %w", entry.Path, os.ErrNotExist)
	}

	p, errGetParser := preStartParser(entry)
	if errGetParser != nil {
		return fmt.Errorf("cfgschema: get parser for pre-start config %q: %w", entry.Path, errGetParser)
	}

	errManagedSources := validatePreStartManagedSources(entry, resolver)
	if errManagedSources != nil {
		return errManagedSources
	}

	output, errOutput := preStartOutput(p, entry, fileData, fileExists, resolver)
	if errOutput != nil {
		return errOutput
	}

	// Ensure parent directory exists.
	dir := path.Dir(relativePath)
	errMkdir := store.EnsureDir(dir)
	if errMkdir != nil {
		return fmt.Errorf("cfgschema: create pre-start config directory %q: %w", dir, errMkdir)
	}

	errWriteFile := store.WriteFile(relativePath, output)
	if errWriteFile != nil {
		return fmt.Errorf("cfgschema: write pre-start config %q: %w", entry.Path, errWriteFile)
	}

	log.Debug().Str("path", entry.Path).Msg("Pre-start: processed config file")
	return nil
}

func preStartParser(entry ConfigSchemaEntry) (*cfgparse.Parser, error) {
	if entry.Format != "xml" || entry.XMLKeyMode == nil {
		parser, errGet := cfgparse.GetParser(entry.Format)
		if errGet != nil {
			return nil, fmt.Errorf("cfgschema: get pre-start parser: %w", errGet)
		}
		return parser, nil
	}

	xmlParser := cfgparse.NewXMLParser(cfgparse.XMLKeyMode{
		Mode:      entry.XMLKeyMode.Mode,
		Element:   entry.XMLKeyMode.Element,
		KeyAttr:   entry.XMLKeyMode.KeyAttr,
		ValueAttr: entry.XMLKeyMode.ValueAttr,
	})
	return &cfgparse.Parser{Structured: xmlParser}, nil
}

func preStartOutput(
	p *cfgparse.Parser,
	entry ConfigSchemaEntry,
	fileData []byte,
	fileExists bool,
	resolver ManagedFieldResolver,
) ([]byte, error) {
	if p.IsFlat() {
		return preStartFlatOutput(p, entry, fileData, fileExists, resolver)
	}

	return preStartStructuredOutput(p, entry, fileData, fileExists, resolver)
}

func preStartFlatOutput(
	p *cfgparse.Parser,
	entry ConfigSchemaEntry,
	fileData []byte,
	fileExists bool,
	resolver ManagedFieldResolver,
) ([]byte, error) {
	var parsed []cfgparse.ConfigEntry

	if fileExists {
		var errParseFile error
		parsed, errParseFile = p.Flat.Parse(fileData)
		if errParseFile != nil {
			return nil, fmt.Errorf("cfgschema: parse pre-start config %q: %w", entry.Path, errParseFile)
		}
	}

	if !fileExists && entry.GenerateBeforeStart {
		parsed = generateDefaultEntries(entry)
	}

	parsed = enforceManagedFields(parsed, entry.ManagedFields, resolver)

	output, errWrite := p.Flat.Write(parsed)
	if errWrite != nil {
		return nil, fmt.Errorf("cfgschema: render pre-start config %q: %w", entry.Path, errWrite)
	}

	return output, nil
}

func preStartStructuredOutput(
	p *cfgparse.Parser,
	entry ConfigSchemaEntry,
	fileData []byte,
	fileExists bool,
	resolver ManagedFieldResolver,
) ([]byte, error) {
	var root *cfgparse.ConfigNode

	if fileExists {
		var errParseFile error
		root, errParseFile = p.Structured.Parse(fileData)
		if errParseFile != nil {
			return nil, fmt.Errorf("cfgschema: parse pre-start config %q: %w", entry.Path, errParseFile)
		}
	} else {
		root = structuredDefaultRoot(entry)
	}

	enforceManagedStructuredFields(root, entry.ManagedFields, entry.Schema, resolver)

	output, errWrite := p.Structured.Write(root)
	if errWrite != nil {
		return nil, fmt.Errorf("cfgschema: render pre-start config %q: %w", entry.Path, errWrite)
	}

	return output, nil
}

func validatePreStartManagedSources(entry ConfigSchemaEntry, resolver ManagedFieldResolver) error {
	for key, source := range entry.ManagedFields {
		_, ok := resolver(source)
		if !ok {
			return fmt.Errorf("cfgschema: pre-start config %q field %q has unknown managed source %q", entry.Path, key, source)
		}
	}
	return nil
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
