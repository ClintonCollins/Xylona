package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/placeholder"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/startargs"
)

const emptyStartArgsPatchesJSON = "[]"

func redactGameForNonSuperuser(game *xylona.Game) {
	if game == nil || game.GetAllowStartArgEditing() {
		return
	}

	game.LinuxStartArgsTemplate = ""
	game.WindowsStartArgsTemplate = ""
	game.StartArgBlocklist = ""
	game.LinuxBaseCommand = ""
	game.WindowsBaseCommand = ""
}

func redactGameServerForNonSuperuser(gameServer *xylona.GameServer) {
	if gameServer == nil {
		return
	}

	gameServer.BackupDirectory = ""

	redactGameForNonSuperuser(gameServer.GetGame())
}

func normalizeStartArgsPlatform(platform string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(platform))
	switch normalized {
	case "linux", "windows":
		return normalized, nil
	default:
		return "", errors.New("platform must be linux or windows")
	}
}

func platformForNode(node *models.Node) string {
	if node != nil {
		nodeOS := strings.ToLower(strings.TrimSpace(node.Os))
		if strings.Contains(nodeOS, "windows") {
			return "windows"
		}
		if nodeOS != "" {
			return "linux"
		}
	}

	if runtime.GOOS == "windows" {
		return "windows"
	}

	return "linux"
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

func normalizeStartArgsPatchesJSON(patches string) string {
	trimmed := strings.TrimSpace(patches)
	if trimmed == "" {
		return emptyStartArgsPatchesJSON
	}

	return trimmed
}

func validateTemplateBlocks(blocks []startargs.ArgBlock) error {
	seenIDs := make(map[string]struct{}, len(blocks))

	for _, block := range blocks {
		blockID := strings.TrimSpace(block.ID)
		if blockID == "" {
			return errors.New("template block id is required")
		}
		if _, exists := seenIDs[blockID]; exists {
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

func validateGameTemplateUpdate(game *models.Game, platform string, templateJSON string, baseCommand string) error {
	templateBlocks, errTemplate := startargs.ParseTemplate(templateJSON)
	if errTemplate != nil {
		return fmt.Errorf("rpc: parse start args template: %w", errTemplate)
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
		return fmt.Errorf("rpc: parse other platform start args template: %w", errOther)
	}

	return validateSharedTemplateIDs(templateBlocks, otherBlocks)
}

func validateGameBlocklistUpdate(blocklistJSON string) error {
	blocklistEntries, errParse := startargs.ParseBlocklist(blocklistJSON)
	if errParse != nil {
		return fmt.Errorf("rpc: parse start arg blocklist: %w", errParse)
	}

	_, errCompile := startargs.CompileBlocklist(blocklistEntries)
	if errCompile != nil {
		return fmt.Errorf("rpc: compile start arg blocklist: %w", errCompile)
	}
	return nil
}

func validateStructuredStartArgsGameConfig(game *models.Game) error {
	if game == nil {
		return errors.New("game is required")
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

func validateServerPatchStructure(template []startargs.ArgBlock, patches []startargs.Patch) error {
	templateByID := make(map[string]startargs.ArgBlock, len(template))
	referenceableIDs := make(map[string]struct{}, len(template))
	addedIDs := make(map[string]struct{})

	for _, block := range template {
		templateByID[block.ID] = block
		referenceableIDs[block.ID] = struct{}{}
	}

	for _, patch := range patches {
		patchID := strings.TrimSpace(patch.ID)
		if patchID == "" {
			return errors.New("patch id is required")
		}

		switch patch.Op {
		case startargs.PatchOpAdd:
			if len(patch.Tokens) == 0 {
				return fmt.Errorf("add patch %q must contain at least one token", patchID)
			}
			if _, exists := templateByID[patchID]; exists {
				return fmt.Errorf("add patch %q collides with an existing template block", patchID)
			}
			if _, exists := addedIDs[patchID]; exists {
				return fmt.Errorf("duplicate add patch id %q", patchID)
			}
			if patch.AfterID != nil {
				afterID := strings.TrimSpace(*patch.AfterID)
				if afterID == "" {
					return fmt.Errorf("add patch %q has an empty afterId", patchID)
				}
				if _, exists := referenceableIDs[afterID]; !exists {
					return fmt.Errorf("add patch %q references unknown afterId %q", patchID, afterID)
				}
			}
			addedIDs[patchID] = struct{}{}
			referenceableIDs[patchID] = struct{}{}
		case startargs.PatchOpEdit:
			if len(patch.Tokens) == 0 {
				return fmt.Errorf("edit patch %q must contain at least one token", patchID)
			}
			if block, exists := templateByID[patchID]; exists && block.Ownership != startargs.OwnershipEditable {
				return fmt.Errorf("patch %q targets a non-editable template block", patchID)
			}
		case startargs.PatchOpRemove:
			if block, exists := templateByID[patchID]; exists && block.Ownership != startargs.OwnershipEditable {
				return fmt.Errorf("patch %q targets a non-editable template block", patchID)
			}
		default:
			return fmt.Errorf("patch %q has invalid operation %q", patchID, patch.Op)
		}
	}

	return nil
}

func validateGameServerStartArgsUpdate(gameServer *models.GameServer, patchesJSON string) error {
	if gameServer.R.Game == nil {
		return errors.New("game relation is required")
	}

	platform := platformForNode(gameServer.R.Node)
	templateJSON := templateJSONForPlatform(gameServer.R.Game, platform)
	templateBlocks, errTemplate := startargs.ParseTemplate(templateJSON)
	if errTemplate != nil {
		return fmt.Errorf("rpc: parse game server start args template: %w", errTemplate)
	}
	if len(templateBlocks) == 0 && normalizeStartArgsPatchesJSON(patchesJSON) == emptyStartArgsPatchesJSON {
		return nil
	}
	if len(templateBlocks) == 0 {
		return errors.New("this game does not have a start args template for the server platform")
	}

	patches, errPatches := startargs.ParsePatches(normalizeStartArgsPatchesJSON(patchesJSON))
	if errPatches != nil {
		return fmt.Errorf("rpc: parse game server start args patches: %w", errPatches)
	}
	errValidateStructure := validateServerPatchStructure(templateBlocks, patches)
	if errValidateStructure != nil {
		return errValidateStructure
	}

	blocklistEntries, errBlocklist := startargs.ParseBlocklist(gameServer.R.Game.StartArgBlocklist)
	if errBlocklist != nil {
		return fmt.Errorf("rpc: parse game server start arg blocklist: %w", errBlocklist)
	}
	compiledBlocklist, errCompile := startargs.CompileBlocklist(blocklistEntries)
	if errCompile != nil {
		return fmt.Errorf("rpc: compile game server start arg blocklist: %w", errCompile)
	}

	vars := placeholder.BuildVarsFromGameServer(gameServer)
	resolvedArgs, _, errResolve := startargs.ResolveArgs(templateBlocks, patches, vars)
	if errResolve != nil {
		return fmt.Errorf("rpc: resolve game server start args: %w", errResolve)
	}

	violation := compiledBlocklist.Validate(resolvedArgs)
	if violation != nil {
		return fmt.Errorf("blocked start argument %q: %s", violation.Token, violation.Reason)
	}

	return nil
}

// UpdateGameStartArgsTemplate updates the structured start args template for a game.
func (xs *XylonaService) UpdateGameStartArgsTemplate(
	_ context.Context,
	request *connect.Request[xylona.UpdateGameStartArgsTemplateRequest],
) (*connect.Response[xylona.UpdateGameStartArgsTemplateResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}

	platform, errPlatform := normalizeStartArgsPlatform(request.Msg.GetPlatform())
	if errPlatform != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errPlatform)
	}

	gameModel, errGetGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	templateJSON := strings.TrimSpace(request.Msg.GetStartArgsTemplate())
	baseCommand := strings.TrimSpace(request.Msg.GetBaseCommand())
	errValidate := validateGameTemplateUpdate(gameModel, platform, templateJSON, baseCommand)
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}

	updatedGame := *gameModel
	if platform == "windows" {
		updatedGame.WindowsStartArgsTemplate = null.FromCond(templateJSON, templateJSON != "")
		updatedGame.WindowsBaseCommand = baseCommand
	} else {
		updatedGame.LinuxStartArgsTemplate = null.FromCond(templateJSON, templateJSON != "")
		updatedGame.LinuxBaseCommand = baseCommand
	}
	updatedGame.AllowStartArgEditing = request.Msg.GetAllowStartArgEditing()

	gameSetter := helpers.GameModelToGameSetter(&updatedGame)
	gameSetter.ID = omit.From(gameModel.ID)
	updated, errUpdate := xs.db.UpdateGame(xs.db.DB, gameModel, gameSetter)
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, errUpdate)
	}

	return connect.NewResponse(&xylona.UpdateGameStartArgsTemplateResponse{
		Game: helpers.GameModelToProto(updated),
	}), nil
}

// UpdateGameStartArgBlocklist updates the blocked start argument list for a game.
func (xs *XylonaService) UpdateGameStartArgBlocklist(
	_ context.Context,
	request *connect.Request[xylona.UpdateGameStartArgBlocklistRequest],
) (*connect.Response[xylona.UpdateGameStartArgBlocklistResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}

	gameModel, errGetGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	blocklistJSON := strings.TrimSpace(request.Msg.GetStartArgBlocklist())
	errValidate := validateGameBlocklistUpdate(blocklistJSON)
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}

	updatedGame := *gameModel
	updatedGame.StartArgBlocklist = blocklistJSON

	gameSetter := helpers.GameModelToGameSetter(&updatedGame)
	gameSetter.ID = omit.From(gameModel.ID)
	updated, errUpdate := xs.db.UpdateGame(xs.db.DB, gameModel, gameSetter)
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, errUpdate)
	}

	return connect.NewResponse(&xylona.UpdateGameStartArgBlocklistResponse{
		Game: helpers.GameModelToProto(updated),
	}), nil
}

// UpdateGameServerStartArgs updates structured start args patches for a server.
func (xs *XylonaService) UpdateGameServerStartArgs(
	ctx context.Context,
	request *connect.Request[xylona.UpdateGameServerStartArgsRequest],
) (*connect.Response[xylona.UpdateGameServerStartArgsResponse], error) {
	serverID := request.Msg.GetServerId()
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	normalizedPatches := normalizeStartArgsPatchesJSON(request.Msg.GetStartArgsPatches())

	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.UpdateGameServerStartArgsResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
			if errPermission != nil {
				return nil, errPermission
			}

			if gameServer.R.Game == nil {
				return nil, connect.NewError(connect.CodeInternal, errors.New("game relation missing"))
			}
			if !user.SuperUser && !gameServer.R.Game.AllowStartArgEditing {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("start arg editing is disabled for this game"))
			}

			errValidate := validateGameServerStartArgsUpdate(gameServer, normalizedPatches)
			if errValidate != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
			}

			setter := &models.GameServerSetter{
				ID:               omit.From(gameServer.ID),
				StartArgsPatches: omit.From(normalizedPatches),
			}
			updated, errUpdate := xs.db.UpdateGameServer(xs.db.DB, setter)
			if errUpdate != nil {
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update game server"))
			}

			gameServerProto := helpers.GameServerModelToProto(updated, xs.versionState)
			if !user.SuperUser {
				redactGameServerForNonSuperuser(gameServerProto)
			}

			return connect.NewResponse(&xylona.UpdateGameServerStartArgsResponse{
				GameServer: gameServerProto,
			}), nil
		},
		func() (*connect.Response[xylona.UpdateGameServerStartArgsResponse], error) {
			return xs.updateRemoteGameServerStartArgs(ctx, serverID, normalizedPatches, user)
		},
	)
}

func (xs *XylonaService) updateRemoteGameServerStartArgs(
	ctx context.Context,
	serverID string,
	startArgsPatches string,
	actingUser *models.User,
) (*connect.Response[xylona.UpdateGameServerStartArgsResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationUpdateServerStartArgsRequest{
		ServerId:         serverID,
		StartArgsPatches: startArgsPatches,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update remote server start args"))
	}

	resp, errUpdate := client.UpdateRemoteServerStartArgs(ctx, req)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to update remote game server start args")
		return nil, wrapRemoteRPCError(errUpdate, "failed to update remote server start args")
	}

	if !resp.Msg.GetSuccess() {
		return nil, connect.NewError(connect.CodeInternal, errors.New(resp.Msg.GetError()))
	}

	gameServerProto := resp.Msg.GetGameServer()
	if actingUser != nil && !actingUser.SuperUser {
		redactGameServerForNonSuperuser(gameServerProto)
	}

	return connect.NewResponse(&xylona.UpdateGameServerStartArgsResponse{
		GameServer: gameServerProto,
	}), nil
}

// UpdateRemoteServerStartArgs updates structured start args patches over federation.
func (fs FederationService) UpdateRemoteServerStartArgs(
	ctx context.Context,
	request *connect.Request[xylona.FederationUpdateServerStartArgsRequest],
) (*connect.Response[xylona.FederationUpdateServerStartArgsResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.settings",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gameServer, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game server"))
	}

	if gameServer.R.Game == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("game relation missing"))
	}
	if !helpers.FederatedActingIsSuperUser(request.Header()) && !gameServer.R.Game.AllowStartArgEditing {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("start arg editing is disabled for this game"))
	}

	normalizedPatches := normalizeStartArgsPatchesJSON(request.Msg.GetStartArgsPatches())
	errValidate := validateGameServerStartArgsUpdate(gameServer, normalizedPatches)
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}

	setter := &models.GameServerSetter{
		ID:               omit.From(gameServer.ID),
		StartArgsPatches: omit.From(normalizedPatches),
	}
	updated, errUpdate := fs.db.UpdateGameServer(fs.db.DB, setter)
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update game server"))
	}

	gameServerProto := helpers.GameServerModelToProto(updated, fs.versionState)
	if !helpers.FederatedActingIsSuperUser(request.Header()) {
		redactGameServerForNonSuperuser(gameServerProto)
	}

	return connect.NewResponse(&xylona.FederationUpdateServerStartArgsResponse{
		Success:    true,
		GameServer: gameServerProto,
	}), nil
}
