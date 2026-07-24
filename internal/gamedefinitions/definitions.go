// Package gamedefinitions handles bundled and imported game definition JSON.
package gamedefinitions

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aarondl/opt/omit"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/internal/controller/protomap"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/pkg/cfgschema"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	// DocumentType identifies Xylona game definition JSON documents.
	DocumentType = "xylona.game_definition"
	// SchemaVersion is the current supported game definition JSON schema.
	SchemaVersion = int64(1)

	emptyJSONArray         = "[]"
	officialDefinitionsDir = "official"
)

var (
	// FS contains bundled official game definition JSON files.
	//
	//go:embed official/*.json
	FS embed.FS

	errMissingGameDefinition = errors.New("game definition is required")

	// ErrGameNotOfficial reports a reset attempt on a game that is not an official definition.
	ErrGameNotOfficial = errors.New("game is not an official definition")
	// ErrNoBundledDefinition reports a reset attempt for a game without a bundled definition.
	ErrNoBundledDefinition = errors.New("no bundled definition for game")
	importIDSuffixPattern  = regexp.MustCompile(`[^a-z0-9_]+`)
	validGameIDPattern     = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

// Document is the stable, human-editable game definition envelope.
type Document struct {
	DocumentType          string                     `json:"document_type"`
	SchemaVersion         int64                      `json:"schema_version"`
	ExportedAt            string                     `json:"exported_at,omitempty"`
	ExportedFromVersion   string                     `json:"exported_from_version,omitempty"`
	ContentHash           string                     `json:"content_hash,omitempty"`
	Game                  json.RawMessage            `json:"game"`
	ConfigSchemas         json.RawMessage            `json:"config_schemas"`
	LinuxStartArgs        json.RawMessage            `json:"linux_start_args_template"`
	WindowsStartArgs      json.RawMessage            `json:"windows_start_args_template"`
	StartArgBlocklist     json.RawMessage            `json:"start_arg_blocklist"`
	DefaultEnvVars        json.RawMessage            `json:"default_env_vars,omitempty"`
	UpdateConfig          updateproviders.GameConfig `json:"update_config"`
	OfficialSource        string                     `json:"official_source,omitempty"`
	OfficialSchemaVersion int64                      `json:"official_schema_version,omitempty"`
}

// ParsedDefinition is a validated document with DB-ready model data.
type ParsedDefinition struct {
	Document Document
	Game     *xylona.Game
	Model    *models.Game
	Hash     string
	Warnings []string
}

// SyncResult summarizes bundled official definition synchronization.
type SyncResult struct {
	Inserted int
	Updated  int
	Diverged int
	Skipped  int
}

// ExportModel serializes a game model to a versioned JSON definition.
func ExportModel(game *models.Game, exportedFromVersion string, exportedAt time.Time) (string, string, error) {
	if game == nil {
		return "", "", errMissingGameDefinition
	}

	doc, errDocument := documentFromModel(game, exportedFromVersion, exportedAt)
	if errDocument != nil {
		return "", "", errDocument
	}

	hash, errHash := HashDocument(doc)
	if errHash != nil {
		return "", "", errHash
	}
	doc.ContentHash = hash

	out, errMarshal := json.MarshalIndent(doc, "", "  ")
	if errMarshal != nil {
		return "", "", fmt.Errorf("marshal game definition: %w", errMarshal)
	}
	return string(out), hash, nil
}

// Parse parses a game definition JSON document.
func Parse(data []byte) (*ParsedDefinition, error) {
	var doc Document
	errUnmarshal := json.Unmarshal(data, &doc)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parse game definition JSON: %w", errUnmarshal)
	}
	if doc.DocumentType != DocumentType {
		return nil, fmt.Errorf("document_type must be %q", DocumentType)
	}
	if doc.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported game definition schema_version %d", doc.SchemaVersion)
	}
	if len(doc.Game) == 0 {
		return nil, errMissingGameDefinition
	}

	hash, errHash := HashDocument(doc)
	if errHash != nil {
		return nil, errHash
	}

	warnings := []string{}
	if strings.TrimSpace(doc.ContentHash) != "" && doc.ContentHash != hash {
		warnings = append(warnings, "Content hash does not match the game definition body.")
	}

	game := &xylona.Game{}
	errGame := protojson.Unmarshal(doc.Game, game)
	if errGame != nil {
		return nil, fmt.Errorf("parse game fields: %w", errGame)
	}

	errSections := applyStructuredSections(doc, game)
	if errSections != nil {
		return nil, errSections
	}

	model := protomap.GameProtoToModel(game)
	defaultEnvVars, errDefaultEnv := defaultEnvFromDocument(doc)
	if errDefaultEnv != nil {
		return nil, errDefaultEnv
	}
	model.DefaultEnvVars = defaultEnvVars
	errSaveConfig := updateconfig.SaveGameConfigToModel(model, doc.UpdateConfig)
	if errSaveConfig != nil {
		return nil, fmt.Errorf("update_config: %w", errSaveConfig)
	}
	model.XylonaOfficial = game.GetXylonaOfficial()
	model.OfficialDefinitionHash = ""
	model.OfficialDefinitionSource = ""
	model.OfficialDefinitionSchemaVersion = 0
	model.OfficialDefinitionDiverged = false

	return &ParsedDefinition{
		Document: doc,
		Game:     game,
		Model:    model,
		Hash:     hash,
		Warnings: warnings,
	}, nil
}

// ValidateModel validates a DB-ready game definition.
func ValidateModel(game *models.Game) []string {
	if game == nil {
		return []string{errMissingGameDefinition.Error()}
	}

	validationErrors := []string{}
	if strings.TrimSpace(game.ID) == "" {
		validationErrors = append(validationErrors, "game id is required")
	} else if !validGameIDPattern.MatchString(game.ID) {
		validationErrors = append(validationErrors, "game id may only contain lowercase letters, numbers, hyphens, and underscores")
	}
	if strings.TrimSpace(game.Name) == "" {
		validationErrors = append(validationErrors, "game name is required")
	}
	configSchemas := game.ConfigSchemas.GetOr("")
	if strings.TrimSpace(configSchemas) != "" {
		schemaErrors := cfgschema.ValidateConfigSchemas(configSchemas)
		validationErrors = append(validationErrors, schemaErrors...)
	}

	errStartArgs := validateStructuredStartArgsGameConfig(game)
	if errStartArgs != nil {
		validationErrors = append(validationErrors, errStartArgs.Error())
	}

	defaultEnv, errDefaultEnv := launchenv.ParseStored(game.DefaultEnvVars)
	if errDefaultEnv != nil {
		validationErrors = append(validationErrors, fmt.Sprintf("default_env_vars: %v", errDefaultEnv))
	} else {
		for _, issue := range launchenv.ValidateVariables(defaultEnv) {
			validationErrors = append(validationErrors, "default_env_vars: "+issue.Message)
		}
	}

	_, errConfig := updateconfig.LoadGameConfigFromModel(game)
	if errConfig != nil {
		validationErrors = append(validationErrors, fmt.Sprintf("update config: %v", errConfig))
	}

	return validationErrors
}

// LoadBundled parses all embedded official game definition documents.
func LoadBundled() ([]*ParsedDefinition, error) {
	entries, errEntries := FS.ReadDir(officialDefinitionsDir)
	if errEntries != nil {
		return nil, fmt.Errorf("read bundled game definitions: %w", errEntries)
	}

	definitions := make([]*ParsedDefinition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}

		definitionPath := officialDefinitionsDir + "/" + entry.Name()
		data, errRead := FS.ReadFile(definitionPath)
		if errRead != nil {
			return nil, fmt.Errorf("read bundled game definition %q: %w", entry.Name(), errRead)
		}
		definition, errParse := Parse(data)
		if errParse != nil {
			return nil, fmt.Errorf("parse bundled game definition %q: %w", entry.Name(), errParse)
		}
		definition.Model.XylonaOfficial = true
		definition.Model.OfficialDefinitionHash = definition.Hash
		definition.Model.OfficialDefinitionSource = entry.Name()
		definition.Model.OfficialDefinitionSchemaVersion = SchemaVersion
		definition.Model.OfficialDefinitionDiverged = false
		definitions = append(definitions, definition)
	}

	sort.Slice(definitions, func(i int, j int) bool {
		return definitions[i].Model.ID < definitions[j].Model.ID
	})
	return definitions, nil
}

// SyncOfficialDefinitions syncs bundled official definitions into the game table.
func SyncOfficialDefinitions(conn *db.Connection) (SyncResult, error) {
	var result SyncResult
	definitions, errLoad := LoadBundled()
	if errLoad != nil {
		return result, errLoad
	}

	for _, definition := range definitions {
		validationErrors := ValidateModel(definition.Model)
		if len(validationErrors) > 0 {
			return result, fmt.Errorf("validate bundled game definition %q: %s", definition.Model.ID, strings.Join(validationErrors, "; "))
		}

		existing, errGet := conn.GetGameByID(definition.Model.ID)
		if errGet != nil {
			if errors.Is(errGet, sql.ErrNoRows) {
				_, errInsert := conn.InsertGame(conn.DB, GameSetterForModel(definition.Model))
				if errInsert != nil {
					return result, fmt.Errorf("insert bundled game definition %q: %w", definition.Model.ID, errInsert)
				}
				result.Inserted++
				continue
			}
			return result, fmt.Errorf("load game %q for official sync: %w", definition.Model.ID, errGet)
		}

		if !existing.XylonaOfficial {
			result.Skipped++
			continue
		}

		currentHash, errCurrentHash := HashModel(existing)
		if errCurrentHash != nil {
			return result, fmt.Errorf("hash current game %q: %w", existing.ID, errCurrentHash)
		}

		var cleanOfficialRow bool
		if existing.OfficialDefinitionHash == "" {
			cleanOfficialRow = currentHash == definition.Hash
		} else {
			cleanOfficialRow = currentHash == existing.OfficialDefinitionHash
		}
		if !cleanOfficialRow {
			errMark := MarkOfficialDiverged(conn, existing)
			if errMark != nil {
				return result, fmt.Errorf("mark game %q diverged: %w", existing.ID, errMark)
			}
			result.Diverged++
			continue
		}

		metadataCurrent := existing.OfficialDefinitionHash == definition.Hash &&
			existing.OfficialDefinitionSource == definition.Model.OfficialDefinitionSource &&
			existing.OfficialDefinitionSchemaVersion == SchemaVersion &&
			!existing.OfficialDefinitionDiverged
		if metadataCurrent && currentHash == definition.Hash {
			continue
		}

		definition.Model.ID = existing.ID
		definition.Model.XylonaOfficial = true
		definition.Model.OfficialDefinitionHash = definition.Hash
		definition.Model.OfficialDefinitionSchemaVersion = SchemaVersion
		definition.Model.OfficialDefinitionDiverged = false
		_, errUpdate := conn.UpdateGame(conn.DB, existing, GameSetterForModel(definition.Model))
		if errUpdate != nil {
			return result, fmt.Errorf("update bundled game definition %q: %w", definition.Model.ID, errUpdate)
		}
		result.Updated++
	}

	return result, nil
}

// ResetGameToOfficialDefinition force-applies the bundled official definition
// to an official game row, discarding local edits and restamping sync metadata
// so future startup syncs track the row again.
func ResetGameToOfficialDefinition(conn *db.Connection, gameID string) (*models.Game, error) {
	existing, errGet := conn.GetGameByID(gameID)
	if errGet != nil {
		return nil, fmt.Errorf("load game %q: %w", gameID, errGet)
	}
	if !existing.XylonaOfficial {
		return nil, fmt.Errorf("reset game %q: %w", gameID, ErrGameNotOfficial)
	}

	definitions, errLoad := LoadBundled()
	if errLoad != nil {
		return nil, errLoad
	}

	var definition *ParsedDefinition
	for _, candidate := range definitions {
		if candidate.Model.ID == existing.ID {
			definition = candidate
			break
		}
	}
	if definition == nil {
		return nil, fmt.Errorf("reset game %q: %w", gameID, ErrNoBundledDefinition)
	}

	updated, errUpdate := conn.UpdateGame(conn.DB, existing, GameSetterForModel(definition.Model))
	if errUpdate != nil {
		return nil, fmt.Errorf("apply bundled definition to game %q: %w", gameID, errUpdate)
	}
	return updated, nil
}

// MarkOfficialDiverged records that an official game has local edits.
func MarkOfficialDiverged(conn *db.Connection, game *models.Game) error {
	setter := &models.GameSetter{
		ID:                         omit.From(game.ID),
		OfficialDefinitionDiverged: omit.From(true),
		UpdatedAt:                  omit.From(time.Now()),
	}
	_, errUpdate := conn.UpdateGame(conn.DB, game, setter)
	if errUpdate != nil {
		return fmt.Errorf("update official divergence marker: %w", errUpdate)
	}
	return nil
}

// GameSetterForModel converts a model to a setter including definition metadata.
func GameSetterForModel(game *models.Game) *models.GameSetter {
	setter := protomap.GameModelToGameSetter(game)
	setter.XylonaOfficial = omit.From(game.XylonaOfficial)
	setter.OfficialDefinitionHash = omit.From(game.OfficialDefinitionHash)
	setter.OfficialDefinitionSource = omit.From(game.OfficialDefinitionSource)
	setter.OfficialDefinitionSchemaVersion = omit.From(game.OfficialDefinitionSchemaVersion)
	setter.OfficialDefinitionDiverged = omit.From(game.OfficialDefinitionDiverged)
	return setter
}

// ClearOfficialMetadata marks a model as a custom/imported game definition.
func ClearOfficialMetadata(game *models.Game) {
	game.XylonaOfficial = false
	game.OfficialDefinitionHash = ""
	game.OfficialDefinitionSource = ""
	game.OfficialDefinitionSchemaVersion = 0
	game.OfficialDefinitionDiverged = false
}

// MarkImportedOfficialEdit preserves official identity while marking divergence.
func MarkImportedOfficialEdit(game *models.Game, existing *models.Game) {
	game.XylonaOfficial = true
	game.OfficialDefinitionHash = existing.OfficialDefinitionHash
	game.OfficialDefinitionSource = existing.OfficialDefinitionSource
	game.OfficialDefinitionSchemaVersion = existing.OfficialDefinitionSchemaVersion
	game.OfficialDefinitionDiverged = true
}

// ExportFileName returns a safe default filename for a game definition.
func ExportFileName(game *models.Game) string {
	id := strings.TrimSpace(game.ID)
	if id == "" {
		id = strings.TrimSpace(game.Name)
	}
	id = strings.ToLower(id)
	id = strings.ReplaceAll(id, "-", "_")
	id = importIDSuffixPattern.ReplaceAllString(id, "_")
	id = strings.Trim(id, "_")
	if id == "" {
		id = "game"
	}
	return id + ".json"
}

// CopyID returns a deterministic available copy/import ID for an imported game.
func CopyID(conn *db.Connection, sourceID string) (string, error) {
	baseID := strings.ToLower(strings.TrimSpace(sourceID))
	baseID = strings.ReplaceAll(baseID, "-", "_")
	baseID = importIDSuffixPattern.ReplaceAllString(baseID, "_")
	baseID = strings.Trim(baseID, "_")
	if baseID == "" {
		baseID = "imported_game"
	}
	baseID = baseID + "_import"

	for i := 1; i < 1000; i++ {
		candidate := baseID
		if i > 1 {
			candidate = fmt.Sprintf("%s_%d", baseID, i)
		}
		_, errGet := conn.GetGameByID(candidate)
		if errGet != nil {
			if errors.Is(errGet, sql.ErrNoRows) {
				return candidate, nil
			}
			return "", fmt.Errorf("load game %q while choosing import id: %w", candidate, errGet)
		}
	}
	return "", errors.New("could not find an available import id")
}

// HashModel computes the canonical definition hash for a persisted game model.
func HashModel(game *models.Game) (string, error) {
	doc, errDocument := documentFromModel(game, "", time.Time{})
	if errDocument != nil {
		return "", errDocument
	}
	return HashDocument(doc)
}

// HashDocument computes the canonical content hash for a definition document.
func HashDocument(doc Document) (string, error) {
	payload, errPayload := canonicalPayload(doc)
	if errPayload != nil {
		return "", errPayload
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func documentFromModel(game *models.Game, exportedFromVersion string, exportedAt time.Time) (Document, error) {
	gameProto := protomap.GameModelToProto(game)
	gameProto.ConfigSchemas = ""
	gameProto.LinuxStartArgsTemplate = ""
	gameProto.WindowsStartArgsTemplate = ""
	gameProto.StartArgBlocklist = ""
	gameProto.UpdateProvider = nil
	gameProto.DefaultTarget = ""
	gameProto.ModProfile = nil
	gameProto.Variants = nil
	gameProto.CreatedAt = nil
	gameProto.UpdatedAt = nil
	gameProto.OfficialDefinitionHash = ""
	gameProto.OfficialDefinitionSource = ""
	gameProto.OfficialDefinitionSchemaVersion = 0
	gameProto.OfficialDefinitionDiverged = false

	gameJSON, errGame := protojson.MarshalOptions{
		EmitUnpopulated: true,
		Indent:          "  ",
	}.Marshal(gameProto)
	if errGame != nil {
		return Document{}, fmt.Errorf("marshal game fields: %w", errGame)
	}

	updateConfig, errConfig := updateconfig.LoadGameConfigFromModel(game)
	if errConfig != nil {
		return Document{}, fmt.Errorf("load update config: %w", errConfig)
	}

	doc := Document{
		DocumentType:          DocumentType,
		SchemaVersion:         SchemaVersion,
		ExportedFromVersion:   exportedFromVersion,
		Game:                  gameJSON,
		ConfigSchemas:         rawArrayFromString(game.ConfigSchemas.GetOr("")),
		LinuxStartArgs:        rawArrayFromString(game.LinuxStartArgsTemplate.GetOr("")),
		WindowsStartArgs:      rawArrayFromString(game.WindowsStartArgsTemplate.GetOr("")),
		StartArgBlocklist:     rawArrayFromString(game.StartArgBlocklist),
		UpdateConfig:          updateConfig,
		OfficialSource:        game.OfficialDefinitionSource,
		OfficialSchemaVersion: game.OfficialDefinitionSchemaVersion,
	}
	defaultEnvVars, errDefaultEnv := defaultEnvRawFromModel(game)
	if errDefaultEnv != nil {
		return Document{}, errDefaultEnv
	}
	doc.DefaultEnvVars = defaultEnvVars
	if !exportedAt.IsZero() {
		doc.ExportedAt = exportedAt.UTC().Format(time.RFC3339)
	}
	return doc, nil
}

func rawArrayFromString(value string) json.RawMessage {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(emptyJSONArray)
	}
	return json.RawMessage(trimmed)
}

func applyStructuredSections(doc Document, game *xylona.Game) error {
	configSchemas, errConfigSchemas := compactRawSection(doc.ConfigSchemas)
	if errConfigSchemas != nil {
		return fmt.Errorf("config_schemas: %w", errConfigSchemas)
	}
	linuxStartArgs, errLinuxStartArgs := compactRawSection(doc.LinuxStartArgs)
	if errLinuxStartArgs != nil {
		return fmt.Errorf("linux_start_args_template: %w", errLinuxStartArgs)
	}
	windowsStartArgs, errWindowsStartArgs := compactRawSection(doc.WindowsStartArgs)
	if errWindowsStartArgs != nil {
		return fmt.Errorf("windows_start_args_template: %w", errWindowsStartArgs)
	}
	blocklist, errBlocklist := compactRawSection(doc.StartArgBlocklist)
	if errBlocklist != nil {
		return fmt.Errorf("start_arg_blocklist: %w", errBlocklist)
	}

	game.ConfigSchemas = emptyToBlank(configSchemas)
	game.LinuxStartArgsTemplate = emptyToBlank(linuxStartArgs)
	game.WindowsStartArgsTemplate = emptyToBlank(windowsStartArgs)
	game.StartArgBlocklist = blocklist
	game.UpdateProvider = protomap.ProviderConfigToProto(doc.UpdateConfig.UpdateProvider)
	game.DefaultTarget = doc.UpdateConfig.DefaultTarget
	game.ModProfile = protomap.ModProfileToProto(doc.UpdateConfig.ModProfile)
	game.Variants = protomap.VariantsToProto(doc.UpdateConfig.Variants)
	return nil
}

func defaultEnvFromDocument(doc Document) (string, error) {
	defaultEnv, errParse := launchenv.ParseStored(string(doc.DefaultEnvVars))
	if errParse != nil {
		return "", fmt.Errorf("default_env_vars: %w", errParse)
	}
	encoded, errMarshal := launchenv.MarshalStored(defaultEnv)
	if errMarshal != nil {
		return "", fmt.Errorf("default_env_vars: %w", errMarshal)
	}
	return encoded, nil
}

func defaultEnvRawFromModel(game *models.Game) (json.RawMessage, error) {
	defaultEnv, errParse := launchenv.ParseStored(game.DefaultEnvVars)
	if errParse != nil {
		return nil, fmt.Errorf("default_env_vars: %w", errParse)
	}
	if len(defaultEnv) == 0 {
		return nil, nil
	}
	encoded, errMarshal := launchenv.MarshalStored(defaultEnv)
	if errMarshal != nil {
		return nil, fmt.Errorf("default_env_vars: %w", errMarshal)
	}
	return json.RawMessage(encoded), nil
}

func compactRawSection(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return emptyJSONArray, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return emptyJSONArray, nil
	}

	var buffer bytes.Buffer
	errCompact := json.Compact(&buffer, raw)
	if errCompact != nil {
		return "", fmt.Errorf("compact JSON section: %w", errCompact)
	}
	return buffer.String(), nil
}

func emptyToBlank(value string) string {
	if value == emptyJSONArray {
		return ""
	}
	return value
}

func canonicalPayload(doc Document) ([]byte, error) {
	gameBody, errGame := canonicalJSONValue(doc.Game)
	if errGame != nil {
		return nil, fmt.Errorf("canonicalize game: %w", errGame)
	}
	configSchemas, errSchemas := canonicalJSONValue(rawArrayOrDefault(doc.ConfigSchemas))
	if errSchemas != nil {
		return nil, fmt.Errorf("canonicalize config_schemas: %w", errSchemas)
	}
	linuxStartArgs, errLinux := canonicalJSONValue(rawArrayOrDefault(doc.LinuxStartArgs))
	if errLinux != nil {
		return nil, fmt.Errorf("canonicalize linux_start_args_template: %w", errLinux)
	}
	windowsStartArgs, errWindows := canonicalJSONValue(rawArrayOrDefault(doc.WindowsStartArgs))
	if errWindows != nil {
		return nil, fmt.Errorf("canonicalize windows_start_args_template: %w", errWindows)
	}
	blocklist, errBlocklist := canonicalJSONValue(rawArrayOrDefault(doc.StartArgBlocklist))
	if errBlocklist != nil {
		return nil, fmt.Errorf("canonicalize start_arg_blocklist: %w", errBlocklist)
	}
	defaultEnvRaw, errDefaultEnvRaw := defaultEnvRawForHash(doc.DefaultEnvVars)
	if errDefaultEnvRaw != nil {
		return nil, errDefaultEnvRaw
	}

	payload := map[string]any{
		"document_type":               doc.DocumentType,
		"schema_version":              doc.SchemaVersion,
		"game":                        gameBody,
		"config_schemas":              configSchemas,
		"linux_start_args_template":   linuxStartArgs,
		"windows_start_args_template": windowsStartArgs,
		"start_arg_blocklist":         blocklist,
		"update_config":               doc.UpdateConfig,
	}
	if len(defaultEnvRaw) > 0 {
		defaultEnv, errDefaultEnv := canonicalJSONValue(defaultEnvRaw)
		if errDefaultEnv != nil {
			return nil, fmt.Errorf("canonicalize default_env_vars: %w", errDefaultEnv)
		}
		payload["default_env_vars"] = defaultEnv
	}

	out, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, fmt.Errorf("marshal canonical payload: %w", errMarshal)
	}
	return out, nil
}

func defaultEnvRawForHash(raw json.RawMessage) (json.RawMessage, error) {
	defaultEnv, errParse := launchenv.ParseStored(string(raw))
	if errParse != nil {
		return nil, fmt.Errorf("default_env_vars: %w", errParse)
	}
	if len(defaultEnv) == 0 {
		return nil, nil
	}
	encoded, errMarshal := launchenv.MarshalStored(defaultEnv)
	if errMarshal != nil {
		return nil, fmt.Errorf("default_env_vars: %w", errMarshal)
	}
	return json.RawMessage(encoded), nil
}

func rawArrayOrDefault(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(emptyJSONArray)
	}
	return raw
}

func canonicalJSONValue(raw json.RawMessage) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	errDecode := decoder.Decode(&value)
	if errDecode != nil {
		return nil, fmt.Errorf("decode JSON value: %w", errDecode)
	}
	return value, nil
}

func validateStructuredStartArgsGameConfig(game *models.Game) error {
	if game == nil {
		return errMissingGameDefinition
	}

	errValidateLinux := validateGameTemplateUpdate(
		game,
		"linux",
		templateJSONForPlatform(game, "linux"),
		baseCommandForPlatform(game, "linux"),
	)
	if errValidateLinux != nil {
		return fmt.Errorf("linux start args: %w", errValidateLinux)
	}

	errValidateWindows := validateGameTemplateUpdate(
		game,
		"windows",
		templateJSONForPlatform(game, "windows"),
		baseCommandForPlatform(game, "windows"),
	)
	if errValidateWindows != nil {
		return fmt.Errorf("windows start args: %w", errValidateWindows)
	}

	errValidateBlocklist := validateGameBlocklistUpdate(game.StartArgBlocklist)
	if errValidateBlocklist != nil {
		return fmt.Errorf("start arg blocklist: %w", errValidateBlocklist)
	}

	return nil
}

func templateJSONForPlatform(game *models.Game, platform string) string {
	if platform == "windows" {
		return game.WindowsStartArgsTemplate.GetOr("")
	}
	return game.LinuxStartArgsTemplate.GetOr("")
}

func baseCommandForPlatform(game *models.Game, platform string) string {
	if platform == "windows" {
		return game.WindowsBaseCommand
	}
	return game.LinuxBaseCommand
}

func validateGameTemplateUpdate(game *models.Game, platform string, templateJSON string, baseCommand string) error {
	templateBlocks, errTemplate := startargs.ParseTemplate(templateJSON)
	if errTemplate != nil {
		return fmt.Errorf("parse start args template: %w", errTemplate)
	}
	errValidateTemplate := validateTemplateBlocks(templateBlocks)
	if errValidateTemplate != nil {
		return errValidateTemplate
	}

	if len(templateBlocks) > 0 && strings.TrimSpace(baseCommand) == "" {
		return errors.New("base command is required when a start args template is configured")
	}

	otherPlatform := "linux"
	if platform == "linux" {
		otherPlatform = "windows"
	}

	otherBlocks, errOther := startargs.ParseTemplate(templateJSONForPlatform(game, otherPlatform))
	if errOther != nil {
		return fmt.Errorf("parse other platform start args template: %w", errOther)
	}

	return validateSharedTemplateIDs(templateBlocks, otherBlocks)
}

func validateTemplateBlocks(blocks []startargs.ArgBlock) error {
	seenIDs := make(map[string]struct{}, len(blocks))

	for _, block := range blocks {
		blockID := strings.TrimSpace(block.ID)
		if blockID == "" {
			return errors.New("template block id is required")
		}
		_, exists := seenIDs[blockID]
		if exists {
			return fmt.Errorf("duplicate template block id %q", blockID)
		}
		seenIDs[blockID] = struct{}{}

		if len(block.Tokens) == 0 {
			return fmt.Errorf("template block %q must contain at least one token", blockID)
		}

		switch block.Ownership {
		case startargs.OwnershipSystem, startargs.OwnershipLocked, startargs.OwnershipEditable:
		default:
			return fmt.Errorf("template block %q has invalid ownership %q", blockID, block.Ownership)
		}

		if block.ManagedSource != "" && !startargs.IsValidManagedSource(block.ManagedSource) {
			return fmt.Errorf("template block %q has invalid managed source %q", blockID, block.ManagedSource)
		}
	}

	return nil
}

func validateSharedTemplateIDs(primary []startargs.ArgBlock, secondary []startargs.ArgBlock) error {
	secondaryByID := make(map[string]startargs.ArgBlock, len(secondary))
	for _, block := range secondary {
		secondaryByID[block.ID] = block
	}

	for _, block := range primary {
		other, exists := secondaryByID[block.ID]
		if !exists {
			continue
		}
		if block.Ownership != other.Ownership {
			return fmt.Errorf("shared template block %q must use the same ownership on both platforms", block.ID)
		}
		if block.Label != other.Label {
			return fmt.Errorf("shared template block %q must use the same label on both platforms", block.ID)
		}
		if block.ManagedSource != other.ManagedSource {
			return fmt.Errorf("shared template block %q must use the same managed source on both platforms", block.ID)
		}
		if len(block.Tokens) != len(other.Tokens) {
			return fmt.Errorf("shared template block %q must use the same token arity on both platforms", block.ID)
		}
	}

	return nil
}

func validateGameBlocklistUpdate(blocklistJSON string) error {
	blocklistEntries, errParse := startargs.ParseBlocklist(blocklistJSON)
	if errParse != nil {
		return fmt.Errorf("parse start arg blocklist: %w", errParse)
	}

	_, errCompile := startargs.CompileBlocklist(blocklistEntries)
	if errCompile != nil {
		return fmt.Errorf("compile start arg blocklist: %w", errCompile)
	}
	return nil
}

// DefinitionPathName normalizes a bundled definition filename.
func DefinitionPathName(path string) string {
	return filepath.Base(path)
}
