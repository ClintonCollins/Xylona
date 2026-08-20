package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/controller/protomap"
	"github.com/ClintonCollins/Xylona/internal/placeholder"
	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
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
	gameServer.NodeHost = ""

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

// platformForGOOS normalizes a GOOS-like value into the two platform families
// supported by the structured start-args templates.
func platformForGOOS(goos string) string {
	if strings.EqualFold(strings.TrimSpace(goos), "windows") {
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

func normalizeStartArgsPatchesJSON(patches string) string {
	trimmed := strings.TrimSpace(patches)
	if trimmed == "" {
		return emptyStartArgsPatchesJSON
	}

	return trimmed
}

func definitionStartArgsConfig(game *models.Game) startargs.DefinitionConfig {
	return startargs.DefinitionConfig{
		LinuxTemplateJSON:   game.LinuxStartArgsTemplate.GetOr(""),
		LinuxBaseCommand:    game.LinuxBaseCommand,
		WindowsTemplateJSON: game.WindowsStartArgsTemplate.GetOr(""),
		WindowsBaseCommand:  game.WindowsBaseCommand,
		BlocklistJSON:       game.StartArgBlocklist,
	}
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
	updatedGame := *gameModel
	if platform == "windows" {
		updatedGame.WindowsStartArgsTemplate = null.FromCond(templateJSON, templateJSON != "")
		updatedGame.WindowsBaseCommand = baseCommand
	} else {
		updatedGame.LinuxStartArgsTemplate = null.FromCond(templateJSON, templateJSON != "")
		updatedGame.LinuxBaseCommand = baseCommand
	}
	updatedGame.AllowStartArgEditing = request.Msg.GetAllowStartArgEditing()
	errValidate := startargs.ValidateDefinition(definitionStartArgsConfig(&updatedGame))
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}
	if updatedGame.XylonaOfficial {
		updatedGame.OfficialDefinitionDiverged = true
	}

	gameSetter := protomap.GameModelToGameSetter(&updatedGame)
	gameSetter.ID = omit.From(gameModel.ID)
	updated, errUpdate := xs.db.UpdateGame(xs.db.DB, gameModel, gameSetter)
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, errUpdate)
	}

	return connect.NewResponse(&xylona.UpdateGameStartArgsTemplateResponse{
		Game: protomap.GameModelToProto(updated),
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
	updatedGame := *gameModel
	updatedGame.StartArgBlocklist = blocklistJSON
	errValidate := startargs.ValidateDefinition(definitionStartArgsConfig(&updatedGame))
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}

	if updatedGame.XylonaOfficial {
		updatedGame.OfficialDefinitionDiverged = true
	}

	gameSetter := protomap.GameModelToGameSetter(&updatedGame)
	gameSetter.ID = omit.From(gameModel.ID)
	updated, errUpdate := xs.db.UpdateGame(xs.db.DB, gameModel, gameSetter)
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, errUpdate)
	}

	return connect.NewResponse(&xylona.UpdateGameStartArgBlocklistResponse{
		Game: protomap.GameModelToProto(updated),
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

	platform := platformForGOOS(xs.resolveNodeGOOS(gameServer.NodeID))
	errValidate := startargs.ValidateServerUpdate(startargs.ServerConfig{
		TemplateJSON:  templateJSONForPlatform(gameServer.R.Game, platform),
		PatchesJSON:   normalizedPatches,
		BlocklistJSON: gameServer.R.Game.StartArgBlocklist,
		Variables:     placeholder.BuildVarsFromGameServer(gameServer),
	})
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

	gameServerProto := protomap.GameServerModelToProto(updated, xs.versionState)
	if !user.SuperUser {
		redactGameServerForNonSuperuser(gameServerProto)
	}

	return connect.NewResponse(&xylona.UpdateGameServerStartArgsResponse{
		GameServer: gameServerProto,
	}), nil
}
