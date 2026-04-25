package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/cfgparse"
	"github.com/ClintonCollins/Xylona/cfgschema"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetGameConfigSchemas returns the config_schemas JSON for a game definition.
// Requires superuser auth.
func (xs *XylonaService) GetGameConfigSchemas(
	_ context.Context,
	request *connect.Request[xylona.GetGameConfigSchemasRequest],
) (*connect.Response[xylona.GetGameConfigSchemasResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	schemas, errGet := xs.db.GetGameConfigSchemas(request.Msg.GetGameId())
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, internalErr()
	}

	return &connect.Response[xylona.GetGameConfigSchemasResponse]{
		Msg: &xylona.GetGameConfigSchemasResponse{
			ConfigSchemasJson: schemas,
		},
	}, nil
}

// UpdateGameConfigSchemas validates and persists config_schemas JSON for a game.
// Requires superuser auth.
func (xs *XylonaService) UpdateGameConfigSchemas(
	_ context.Context,
	request *connect.Request[xylona.UpdateGameConfigSchemasRequest],
) (*connect.Response[xylona.UpdateGameConfigSchemasResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	schemasJSON := request.Msg.GetConfigSchemasJson()

	// Validate the JSON structure.
	validationErrors := cfgschema.ValidateConfigSchemas(schemasJSON)
	if len(validationErrors) > 0 {
		return &connect.Response[xylona.UpdateGameConfigSchemasResponse]{
			Msg: &xylona.UpdateGameConfigSchemasResponse{
				Success:          false,
				ValidationErrors: validationErrors,
			},
		}, nil
	}

	errUpdate := xs.db.UpdateGameConfigSchemas(request.Msg.GetGameId(), schemasJSON)
	if errUpdate != nil {
		return nil, internalErrf("failed to update config schemas")
	}

	return &connect.Response[xylona.UpdateGameConfigSchemasResponse]{
		Msg: &xylona.UpdateGameConfigSchemasResponse{
			Success: true,
		},
	}, nil
}

// GetGameServerConfigFiles returns the list of config files defined by the
// game's schema, with existence status for each file.
func (xs *XylonaService) GetGameServerConfigFiles(
	ctx context.Context,
	request *connect.Request[xylona.GetGameServerConfigFilesRequest],
) (*connect.Response[xylona.GetGameServerConfigFilesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, db.PermissionGameServerConfig)
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}

	return getGameServerConfigFiles(ctx, xs.db, gameServer, client)
}

func getGameServerConfigFiles(
	ctx context.Context,
	dbInst *db.Connection,
	gameServer *models.GameServer,
	client nodeclient.NodeClient,
) (*connect.Response[xylona.GetGameServerConfigFilesResponse], error) {
	game, errGame := dbInst.GetGameByID(gameServer.GameID)
	if errGame != nil {
		return nil, internalErrf("failed to get game")
	}

	schemasJSON := game.ConfigSchemas.GetOr("")
	entries, errParse := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return nil, internalErrf("failed to parse config schemas")
	}

	var configFiles []*xylona.ConfigFileInfo
	for _, entry := range entries {
		relativePath, errPath := sanitizeConfigRelativePath(entry.Path)
		if errPath != nil {
			return nil, errPath
		}
		existsOnDisk := configFileExists(ctx, client, gameServer.Directory, relativePath)

		fieldCount := helpers.ClampInt32FromInt(len(entry.Schema.Properties))
		managedCount := helpers.ClampInt32FromInt(len(entry.ManagedFields))

		configFiles = append(configFiles, &xylona.ConfigFileInfo{
			Path:                entry.Path,
			Format:              entry.Format,
			Category:            entry.Category,
			FieldCount:          fieldCount,
			ManagedFieldCount:   managedCount,
			ExistsOnDisk:        existsOnDisk,
			GenerateBeforeStart: entry.GenerateBeforeStart,
		})
	}

	return &connect.Response[xylona.GetGameServerConfigFilesResponse]{
		Msg: &xylona.GetGameServerConfigFilesResponse{
			ConfigFiles: configFiles,
			GameName:    game.Name,
		},
	}, nil
}

// GetGameServerConfigFile reads a config file and returns structured field data.
func (xs *XylonaService) GetGameServerConfigFile(
	ctx context.Context,
	request *connect.Request[xylona.GetGameServerConfigFileRequest],
) (*connect.Response[xylona.GetGameServerConfigFileResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, db.PermissionGameServerConfig)
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}

	return getGameServerConfigFile(ctx, xs.db, gameServer, client, request.Msg.GetFilePath())
}

func getGameServerConfigFile(
	ctx context.Context,
	dbInst *db.Connection,
	gameServer *models.GameServer,
	client nodeclient.NodeClient,
	requestedPath string,
) (*connect.Response[xylona.GetGameServerConfigFileResponse], error) {
	game, errGame := dbInst.GetGameByID(gameServer.GameID)
	if errGame != nil {
		return nil, internalErrf("failed to get game")
	}

	schemasJSON := game.ConfigSchemas.GetOr("")
	entries, errParse := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return nil, internalErrf("failed to parse config schemas")
	}

	// Find the matching schema entry.
	var schemaEntry *cfgschema.ConfigSchemaEntry
	for i, e := range entries {
		if e.Path == requestedPath {
			schemaEntry = &entries[i]
			break
		}
	}
	if schemaEntry == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("config file not found in schema"))
	}
	relativePath, errPath := sanitizeConfigRelativePath(requestedPath)
	if errPath != nil {
		return nil, errPath
	}

	// Get parser.
	p, errGetParser := cfgparse.GetParser(schemaEntry.Format)
	if errGetParser != nil {
		return nil, internalErrf("unsupported format")
	}

	// Read and parse the file.
	var parsed []cfgparse.ConfigEntry

	fileData, errRead := client.ReadFile(ctx, gameServer.Directory, relativePath)
	if errRead == nil {
		if p.IsFlat() {
			parsed, errRead = p.Flat.Parse(fileData)
			if errRead != nil {
				return nil, internalErrf("failed to parse config file")
			}
		} else {
			root, errParseStructured := p.Structured.Parse(fileData)
			if errParseStructured != nil {
				return nil, internalErrf("failed to parse config file")
			}
			parsed = cfgparse.Flatten(root)
		}
	}

	// Create resolver for managed fields.
	resolver := cfgschema.ServerSettingsResolver(gameServer.IP, gameServer.Port, gameServer.QueryPort)

	// Match fields.
	result := cfgschema.MatchFields(parsed, schemaEntry.Schema, schemaEntry.ManagedFields, resolver)

	// Convert to proto.
	var protoFields []*xylona.ConfigFieldData
	for _, f := range result.Fields {
		pf := &xylona.ConfigFieldData{
			Key:               f.Key,
			Value:             f.Value,
			Title:             f.Title,
			Description:       f.Description,
			FieldType:         f.FieldType,
			DefaultValue:      f.DefaultValue,
			IsManaged:         f.IsManaged,
			ManagedSource:     f.ManagedSource,
			IsMissingFromFile: f.IsMissingFromFile,
			EnumOptions:       f.EnumOptions,
			EnumLabels:        f.EnumLabels,
			Required:          f.Required,
			AllowMultiple:     f.AllowMultiple,
			Values:            f.Values,
			Group:             f.Group,
		}
		if f.Minimum != nil {
			pf.Minimum = f.Minimum
		}
		if f.Maximum != nil {
			pf.Maximum = f.Maximum
		}
		if f.MaxLength != nil {
			pf.MaxLength = f.MaxLength
		}
		if f.Order != nil {
			pf.Order = f.Order
		}
		protoFields = append(protoFields, pf)
	}

	var protoAdvanced []*xylona.AdvancedField
	for _, af := range result.AdvancedFields {
		protoAdvanced = append(protoAdvanced, &xylona.AdvancedField{
			Key:     af.Key,
			Value:   af.Value,
			Section: af.Section,
		})
	}

	return &connect.Response[xylona.GetGameServerConfigFileResponse]{
		Msg: &xylona.GetGameServerConfigFileResponse{
			Fields:         protoFields,
			AdvancedFields: protoAdvanced,
			FilePath:       requestedPath,
			Format:         schemaEntry.Format,
			Category:       schemaEntry.Category,
		},
	}, nil
}

// UpdateGameServerConfigFile validates and writes config field values.
func (xs *XylonaService) UpdateGameServerConfigFile(
	ctx context.Context,
	request *connect.Request[xylona.UpdateGameServerConfigFileRequest],
) (*connect.Response[xylona.UpdateGameServerConfigFileResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, db.PermissionGameServerConfig)
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}

	return updateGameServerConfigFile(ctx, xs.db, gameServer, client, request.Msg)
}

func updateGameServerConfigFile(
	ctx context.Context,
	dbInst *db.Connection,
	gameServer *models.GameServer,
	client nodeclient.NodeClient,
	msg *xylona.UpdateGameServerConfigFileRequest,
) (*connect.Response[xylona.UpdateGameServerConfigFileResponse], error) {
	game, errGame := dbInst.GetGameByID(gameServer.GameID)
	if errGame != nil {
		return nil, internalErrf("failed to get game")
	}

	schemasJSON := game.ConfigSchemas.GetOr("")
	entries, errParse := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return nil, internalErrf("failed to parse config schemas")
	}

	// Find the matching schema entry.
	var schemaEntry *cfgschema.ConfigSchemaEntry
	for i, e := range entries {
		if e.Path == msg.GetFilePath() {
			schemaEntry = &entries[i]
			break
		}
	}
	if schemaEntry == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("config file not found in schema"))
	}

	// Convert proto fields to service types.
	var fields []cfgschema.FieldData
	for _, pf := range msg.GetFields() {
		fd := cfgschema.FieldData{
			Key:               pf.GetKey(),
			Value:             pf.GetValue(),
			Title:             pf.GetTitle(),
			FieldType:         pf.GetFieldType(),
			IsManaged:         pf.GetIsManaged(),
			IsMissingFromFile: pf.GetIsMissingFromFile(),
			AllowMultiple:     pf.GetAllowMultiple(),
			Values:            pf.GetValues(),
		}
		if pf.Minimum != nil {
			fd.Minimum = pf.Minimum
		}
		if pf.Maximum != nil {
			fd.Maximum = pf.Maximum
		}
		if pf.MaxLength != nil {
			fd.MaxLength = pf.MaxLength
		}
		fields = append(fields, fd)
	}

	// Validate fields.
	validationErrors := cfgschema.ValidateFields(fields, schemaEntry.Schema)
	if len(validationErrors) > 0 {
		var protoErrors []*xylona.ConfigValidationError
		for _, ve := range validationErrors {
			protoErrors = append(protoErrors, &xylona.ConfigValidationError{
				Field:   ve.Field,
				Message: ve.Message,
			})
		}
		return &connect.Response[xylona.UpdateGameServerConfigFileResponse]{
			Msg: &xylona.UpdateGameServerConfigFileResponse{
				Success: false,
				Errors:  protoErrors,
			},
		}, nil
	}

	// Get parser.
	p, errGetParser := cfgparse.GetParser(schemaEntry.Format)
	if errGetParser != nil {
		return nil, internalErrf("unsupported format")
	}

	relativePath, errPath := sanitizeConfigRelativePath(schemaEntry.Path)
	if errPath != nil {
		return nil, errPath
	}

	fileData, errRead := client.ReadFile(ctx, gameServer.Directory, relativePath)

	// Convert advanced fields.
	var advancedFields []cfgschema.AdvancedFieldData
	for _, af := range msg.GetAdvancedFields() {
		advancedFields = append(advancedFields, cfgschema.AdvancedFieldData{
			Key:     af.GetKey(),
			Value:   af.GetValue(),
			Section: af.GetSection(),
		})
	}

	output, errBuildOutput := configFileOutput(p, fileData, errRead, fields, advancedFields, schemaEntry.Schema)
	if errors.Is(errBuildOutput, errParseConfigFile) {
		return nil, internalErrf("failed to parse existing config file")
	}
	if errBuildOutput != nil {
		return nil, internalErrf("failed to write config file")
	}

	errEnsureDir := ensureConfigParentDirectory(ctx, client, gameServer.Directory, relativePath)
	if errEnsureDir != nil {
		return nil, internalErrf("failed to create directory")
	}

	errWriteFile := client.WriteFile(ctx, gameServer.Directory, relativePath, output, node.ProtectionPolicy{})
	if errWriteFile != nil {
		return nil, internalErrf("failed to write config file")
	}

	return &connect.Response[xylona.UpdateGameServerConfigFileResponse]{
		Msg: &xylona.UpdateGameServerConfigFileResponse{
			Success: true,
		},
	}, nil
}

var errParseConfigFile = errors.New("parse config file")

func configFileOutput(
	p *cfgparse.Parser,
	fileData []byte,
	errRead error,
	fields []cfgschema.FieldData,
	advancedFields []cfgschema.AdvancedFieldData,
	schema cfgschema.SchemaDefinition,
) ([]byte, error) {
	if p.IsFlat() {
		return flatConfigFileOutput(p, fileData, errRead, fields, advancedFields, schema)
	}

	return structuredConfigFileOutput(p, fileData, errRead, fields, advancedFields, schema)
}

func flatConfigFileOutput(
	p *cfgparse.Parser,
	fileData []byte,
	errRead error,
	fields []cfgschema.FieldData,
	advancedFields []cfgschema.AdvancedFieldData,
	schema cfgschema.SchemaDefinition,
) ([]byte, error) {
	var existingEntries []cfgparse.ConfigEntry

	if errRead == nil {
		var errParseFile error
		existingEntries, errParseFile = p.Flat.Parse(fileData)
		if errParseFile != nil {
			return nil, fmt.Errorf("%w: %w", errParseConfigFile, errParseFile)
		}
	}

	merged := cfgschema.MergeAndWrite(existingEntries, fields, advancedFields, schema)
	output, errWrite := p.Flat.Write(merged)
	if errWrite != nil {
		return nil, fmt.Errorf("write flat config: %w", errWrite)
	}

	return output, nil
}

func structuredConfigFileOutput(
	p *cfgparse.Parser,
	fileData []byte,
	errRead error,
	fields []cfgschema.FieldData,
	advancedFields []cfgschema.AdvancedFieldData,
	schema cfgschema.SchemaDefinition,
) ([]byte, error) {
	root := &cfgparse.ConfigNode{Type: cfgparse.NodeObject}

	if errRead == nil {
		var errParseFile error
		root, errParseFile = p.Structured.Parse(fileData)
		if errParseFile != nil {
			return nil, fmt.Errorf("%w: %w", errParseConfigFile, errParseFile)
		}
	}

	cfgschema.MergeStructuredFields(root, fields, advancedFields, schema)
	output, errWrite := p.Structured.Write(root)
	if errWrite != nil {
		return nil, fmt.Errorf("write structured config: %w", errWrite)
	}

	return output, nil
}

// GenerateGameServerConfigFile creates a config file from schema defaults.
func (xs *XylonaService) GenerateGameServerConfigFile(
	ctx context.Context,
	request *connect.Request[xylona.GenerateGameServerConfigFileRequest],
) (*connect.Response[xylona.GenerateGameServerConfigFileResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, db.PermissionGameServerConfig)
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}

	return generateGameServerConfigFile(ctx, xs.db, gameServer, client, request.Msg.GetFilePath())
}

func generateGameServerConfigFile(
	ctx context.Context,
	dbInst *db.Connection,
	gameServer *models.GameServer,
	client nodeclient.NodeClient,
	requestedPath string,
) (*connect.Response[xylona.GenerateGameServerConfigFileResponse], error) {
	game, errGame := dbInst.GetGameByID(gameServer.GameID)
	if errGame != nil {
		return nil, internalErrf("failed to get game")
	}

	schemasJSON := game.ConfigSchemas.GetOr("")
	entries, errParse := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return nil, internalErrf("failed to parse config schemas")
	}

	var schemaEntry *cfgschema.ConfigSchemaEntry
	for i, e := range entries {
		if e.Path == requestedPath {
			schemaEntry = &entries[i]
			break
		}
	}
	if schemaEntry == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("config file not found in schema"))
	}
	relativePath, errPath := sanitizeConfigRelativePath(requestedPath)
	if errPath != nil {
		return nil, errPath
	}

	// Get resolver.
	resolver := cfgschema.ServerSettingsResolver(gameServer.IP, gameServer.Port, gameServer.QueryPort)

	stagingDir, errMkdirTemp := os.MkdirTemp("", "xylona-config-generate-*")
	if errMkdirTemp != nil {
		return nil, internalErrf("failed to create config staging directory")
	}
	defer func() {
		errRemoveAll := os.RemoveAll(stagingDir)
		if errRemoveAll != nil {
			log.Warn().Err(errRemoveAll).Str("path", stagingDir).Msg("failed to remove config staging directory")
		}
	}()

	generationSchemasJSON, errGenerationSchemas := configSchemasJSONForGeneration(schemaEntry)
	if errGenerationSchemas != nil {
		return nil, internalErrf("failed to generate config file")
	}

	cfgschema.RunPreStart(stagingDir, generationSchemasJSON, resolver)

	stagedFilePath := filepath.Join(stagingDir, filepath.FromSlash(relativePath))
	fileData, errRead := os.ReadFile(stagedFilePath)
	if errRead != nil {
		return nil, internalErrf("failed to generate config file")
	}

	errEnsureDir := ensureConfigParentDirectory(ctx, client, gameServer.Directory, relativePath)
	if errEnsureDir != nil {
		return nil, internalErrf("failed to create directory")
	}

	errWrite := client.WriteFile(ctx, gameServer.Directory, relativePath, fileData, node.ProtectionPolicy{})
	if errWrite != nil {
		return nil, internalErrf("failed to write config file")
	}

	return &connect.Response[xylona.GenerateGameServerConfigFileResponse]{
		Msg: &xylona.GenerateGameServerConfigFileResponse{
			Success: true,
		},
	}, nil
}

func configSchemasJSONForGeneration(schemaEntry *cfgschema.ConfigSchemaEntry) (string, error) {
	generationEntry := *schemaEntry
	generationEntry.GenerateBeforeStart = true

	data, errMarshal := json.Marshal([]cfgschema.ConfigSchemaEntry{generationEntry})
	if errMarshal != nil {
		return "", fmt.Errorf("marshal generation config schema: %w", errMarshal)
	}

	return string(data), nil
}

func sanitizeConfigRelativePath(relativePath string) (string, error) {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(relativePath))
	normalizedPath = strings.TrimPrefix(normalizedPath, "/")
	if normalizedPath != "" && !filepath.IsLocal(normalizedPath) {
		return "", invalidArg("invalid config file path")
	}
	return normalizedPath, nil
}

func configFileExists(ctx context.Context, client nodeclient.NodeClient, directory string, relativePath string) bool {
	_, errRead := client.ReadFile(ctx, directory, relativePath)
	return errRead == nil
}

func ensureConfigParentDirectory(ctx context.Context, client nodeclient.NodeClient, directory string, relativePath string) error {
	parentDir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relativePath)))
	if parentDir == "." || parentDir == "" {
		return nil
	}
	errCreate := client.CreateFileOrDirectory(ctx, directory, parentDir, "", true, node.ProtectionPolicy{})
	if errCreate != nil {
		return fmt.Errorf("create config parent directory %q: %w", parentDir, errCreate)
	}
	return nil
}
