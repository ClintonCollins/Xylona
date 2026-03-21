package rpc

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/cfgparse"
	"github.com/ClintonCollins/Xylona/cfgschema"
	"github.com/ClintonCollins/Xylona/db"
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
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}

	schemas, errGet := xs.db.GetGameConfigSchemas(request.Msg.GetGameId())
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
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
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
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
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update config schemas"))
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
	_ context.Context,
	request *connect.Request[xylona.GetGameServerConfigFilesRequest],
) (*connect.Response[xylona.GetGameServerConfigFilesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	serverID := request.Msg.GetGameServerId()

	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GetGameServerConfigFilesResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, db.PermissionGameServerConfig)
			if errPermission != nil {
				return nil, errPermission
			}

			return xs.getGameServerConfigFilesLocal(gameServer)
		},
		func() (*connect.Response[xylona.GetGameServerConfigFilesResponse], error) {
			return nil, connect.NewError(connect.CodeUnimplemented, errors.New("federation not yet supported"))
		},
	)
}

func (xs *XylonaService) getGameServerConfigFilesLocal(
	gameServer *models.GameServer,
) (*connect.Response[xylona.GetGameServerConfigFilesResponse], error) {
	game, errGame := xs.db.GetGameByID(gameServer.GameID)
	if errGame != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game"))
	}

	schemasJSON := game.ConfigSchemas.GetOr("")
	entries, errParse := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse config schemas"))
	}

	var configFiles []*xylona.ConfigFileInfo
	for _, entry := range entries {
		filePath := filepath.Join(gameServer.Directory, entry.Path)
		_, errStat := os.Stat(filePath)
		existsOnDisk := errStat == nil

		fieldCount := int32(len(entry.Schema.Properties))
		managedCount := int32(len(entry.ManagedFields))

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
	_ context.Context,
	request *connect.Request[xylona.GetGameServerConfigFileRequest],
) (*connect.Response[xylona.GetGameServerConfigFileResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	serverID := request.Msg.GetGameServerId()

	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GetGameServerConfigFileResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, db.PermissionGameServerConfig)
			if errPermission != nil {
				return nil, errPermission
			}

			return xs.getGameServerConfigFileLocal(gameServer, request.Msg.GetFilePath())
		},
		func() (*connect.Response[xylona.GetGameServerConfigFileResponse], error) {
			return nil, connect.NewError(connect.CodeUnimplemented, errors.New("federation not yet supported"))
		},
	)
}

func (xs *XylonaService) getGameServerConfigFileLocal(
	gameServer *models.GameServer,
	requestedPath string,
) (*connect.Response[xylona.GetGameServerConfigFileResponse], error) {
	game, errGame := xs.db.GetGameByID(gameServer.GameID)
	if errGame != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game"))
	}

	schemasJSON := game.ConfigSchemas.GetOr("")
	entries, errParse := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse config schemas"))
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

	// Get parser.
	p, errGetParser := cfgparse.GetParser(schemaEntry.Format)
	if errGetParser != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("unsupported format"))
	}

	// Read and parse the file.
	filePath := filepath.Join(gameServer.Directory, requestedPath)
	var parsed []cfgparse.ConfigEntry

	fileData, errRead := os.ReadFile(filePath)
	if errRead == nil && p.IsFlat() {
		parsed, errRead = p.Flat.Parse(fileData)
		if errRead != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse config file"))
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
			IsMissingFromFile: f.IsMissingFromFile,
			EnumOptions:       f.EnumOptions,
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
	_ context.Context,
	request *connect.Request[xylona.UpdateGameServerConfigFileRequest],
) (*connect.Response[xylona.UpdateGameServerConfigFileResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	serverID := request.Msg.GetGameServerId()

	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.UpdateGameServerConfigFileResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, db.PermissionGameServerConfig)
			if errPermission != nil {
				return nil, errPermission
			}

			return xs.updateGameServerConfigFileLocal(gameServer, request.Msg)
		},
		func() (*connect.Response[xylona.UpdateGameServerConfigFileResponse], error) {
			return nil, connect.NewError(connect.CodeUnimplemented, errors.New("federation not yet supported"))
		},
	)
}

func (xs *XylonaService) updateGameServerConfigFileLocal(
	gameServer *models.GameServer,
	msg *xylona.UpdateGameServerConfigFileRequest,
) (*connect.Response[xylona.UpdateGameServerConfigFileResponse], error) {
	game, errGame := xs.db.GetGameByID(gameServer.GameID)
	if errGame != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game"))
	}

	schemasJSON := game.ConfigSchemas.GetOr("")
	entries, errParse := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse config schemas"))
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
			Key:               pf.Key,
			Value:             pf.Value,
			Title:             pf.Title,
			FieldType:         pf.FieldType,
			IsManaged:         pf.IsManaged,
			IsMissingFromFile: pf.IsMissingFromFile,
			AllowMultiple:     pf.AllowMultiple,
			Values:            pf.Values,
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
		return nil, connect.NewError(connect.CodeInternal, errors.New("unsupported format"))
	}

	if !p.IsFlat() {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("structured format editing not yet supported"))
	}

	// Read existing file.
	filePath := filepath.Join(gameServer.Directory, msg.GetFilePath())
	var existingEntries []cfgparse.ConfigEntry

	fileData, errRead := os.ReadFile(filePath)
	if errRead == nil {
		existingEntries, errRead = p.Flat.Parse(fileData)
		if errRead != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse existing config file"))
		}
	}

	// Convert advanced fields.
	var advancedFields []cfgschema.AdvancedFieldData
	for _, af := range msg.GetAdvancedFields() {
		advancedFields = append(advancedFields, cfgschema.AdvancedFieldData{
			Key:     af.Key,
			Value:   af.Value,
			Section: af.Section,
		})
	}

	// Merge and write.
	merged := cfgschema.MergeAndWrite(existingEntries, fields, advancedFields, schemaEntry.Schema)

	output, errWrite := p.Flat.Write(merged)
	if errWrite != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to write config file"))
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(filePath)
	errMkdir := os.MkdirAll(dir, 0o755)
	if errMkdir != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create directory"))
	}

	errWriteFile := os.WriteFile(filePath, output, 0o644)
	if errWriteFile != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to write config file"))
	}

	return &connect.Response[xylona.UpdateGameServerConfigFileResponse]{
		Msg: &xylona.UpdateGameServerConfigFileResponse{
			Success: true,
		},
	}, nil
}

// GenerateGameServerConfigFile creates a config file from schema defaults.
func (xs *XylonaService) GenerateGameServerConfigFile(
	_ context.Context,
	request *connect.Request[xylona.GenerateGameServerConfigFileRequest],
) (*connect.Response[xylona.GenerateGameServerConfigFileResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	serverID := request.Msg.GetGameServerId()

	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GenerateGameServerConfigFileResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, db.PermissionGameServerConfig)
			if errPermission != nil {
				return nil, errPermission
			}

			return xs.generateGameServerConfigFileLocal(gameServer, request.Msg.GetFilePath())
		},
		func() (*connect.Response[xylona.GenerateGameServerConfigFileResponse], error) {
			return nil, connect.NewError(connect.CodeUnimplemented, errors.New("federation not yet supported"))
		},
	)
}

func (xs *XylonaService) generateGameServerConfigFileLocal(
	gameServer *models.GameServer,
	requestedPath string,
) (*connect.Response[xylona.GenerateGameServerConfigFileResponse], error) {
	game, errGame := xs.db.GetGameByID(gameServer.GameID)
	if errGame != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game"))
	}

	schemasJSON := game.ConfigSchemas.GetOr("")
	entries, errParse := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse config schemas"))
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

	// Get resolver.
	resolver := cfgschema.ServerSettingsResolver(gameServer.IP, gameServer.Port, gameServer.QueryPort)

	// Use the pre-start logic to generate the file.
	cfgschema.RunPreStart(gameServer.Directory, schemasJSON, resolver)

	return &connect.Response[xylona.GenerateGameServerConfigFileResponse]{
		Msg: &xylona.GenerateGameServerConfigFileResponse{
			Success: true,
		},
	}, nil
}
