package rpc

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"

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

// platformForNode returns the OS family for a node's runtime. Hub-spoke keeps
// this helper so existing call sites can be updated later to use the remote
// node's reported OS via NodeClient.GetNodeSnapshot; for now we fall back to
// the controller's own GOOS.
func platformForNode(_ *models.Node) string {
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
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	platform, errPlatform := normalizeStartArgsPlatform(request.Msg.GetPlatform())
	if errPlatform != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errPlatform)
	}

	gameModel, errGetGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGetGame != nil {
		return nil, dbLookup(errGetGame)
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
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	gameModel, errGetGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGetGame != nil {
		return nil, dbLookup(errGetGame)
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
	_ context.Context,
	request *connect.Request[xylona.UpdateGameServerStartArgsRequest],
) (*connect.Response[xylona.UpdateGameServerStartArgsResponse], error) {
	serverID := request.Msg.GetServerId()
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	normalizedPatches := normalizeStartArgsPatchesJSON(request.Msg.GetStartArgsPatches())

	gameServer, errLookup := xs.db.GetGameServerByID(serverID)
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	if gameServer.R.Game == nil {
		return nil, internalErrf("game relation missing")
	}
	if !user.SuperUser && !gameServer.R.Game.AllowStartArgEditing {
		return nil, permissionDenied("start arg editing is disabled for this game")
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
		return nil, internalErrf("failed to update game server")
	}

	gameServerProto := helpers.GameServerModelToProto(updated, xs.versionState)
	if !user.SuperUser {
		redactGameServerForNonSuperuser(gameServerProto)
	}

	return connect.NewResponse(&xylona.UpdateGameServerStartArgsResponse{
		GameServer: gameServerProto,
	}), nil
}
