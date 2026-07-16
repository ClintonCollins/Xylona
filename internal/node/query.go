package node

import (
	"context"
	"fmt"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/pkg/query"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// QueryGameServer executes a game-server query probe from the node host. Probe
// failures are represented as default max-player payloads so the controller can
// keep its existing scheduling and storage flow without moving policy here.
func (n *Node) QueryGameServer(ctx context.Context, req GameServerQueryRequest) (GameServerQueryResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	errCtx := ctx.Err()
	if errCtx != nil {
		return GameServerQueryResult{Kind: req.Kind}, fmt.Errorf("node: query game server canceled: %w", errCtx)
	}

	switch req.Kind {
	case GameServerQueryKindMinecraft:
		return queryMinecraft(req), nil
	case GameServerQueryKindSource:
		return querySource(req), nil
	case GameServerQueryKindPalworld:
		return queryPalworld(ctx, req), nil
	default:
		return GameServerQueryResult{Kind: GameServerQueryKindUnknown}, nil
	}
}

func queryPalworld(ctx context.Context, req GameServerQueryRequest) GameServerQueryResult {
	info, errQuery := query.PalworldDetailed(ctx, req.IP, int(req.QueryPort), req.Username, req.Password)
	if errQuery != nil {
		info = &query.PalworldInfo{MaxPlayers: helpers.ClampUint32FromInt64(req.MaxPlayers)}
	}
	return GameServerQueryResult{
		Kind:     GameServerQueryKindPalworld,
		Palworld: palworldQueryFromDetailed(info),
	}
}

func queryMinecraft(req GameServerQueryRequest) GameServerQueryResult {
	info, errQuery := query.Minecraft(req.IP, int(req.QueryPort))
	if errQuery != nil {
		info = &xylona.MinecraftQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(req.MaxPlayers)}
	}
	return GameServerQueryResult{
		Kind:      GameServerQueryKindMinecraft,
		Minecraft: minecraftQueryFromXylona(info),
	}
}

func querySource(req GameServerQueryRequest) GameServerQueryResult {
	info, errQuery := query.Source(req.IP, int(req.QueryPort))
	if errQuery != nil {
		info = &xylona.SourceQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(req.MaxPlayers)}
	}
	return GameServerQueryResult{
		Kind:   GameServerQueryKindSource,
		Source: sourceQueryFromXylona(info),
	}
}

func minecraftQueryFromXylona(info *xylona.MinecraftQueryInfo) *MinecraftQueryInfo {
	if info == nil {
		return nil
	}
	playerDetails := make([]GameServerPlayer, 0, len(info.GetPlayerList()))
	for _, playerName := range info.GetPlayerList() {
		playerDetails = append(playerDetails, GameServerPlayer{Name: playerName, ID: playerName})
	}
	return &MinecraftQueryInfo{
		MOTD:            info.GetMotd(),
		GameType:        info.GetGameType(),
		Map:             info.GetMap(),
		NumberOfPlayers: info.GetNumberOfPlayers(),
		MaxPlayers:      info.GetMaxPlayers(),
		PlayerList:      append([]string(nil), info.GetPlayerList()...),
		ProtocolVersion: info.GetProtocolVersion(),
		ServerVersion:   info.GetServerVersion(),
		PlayerDetails:   playerDetails,
	}
}

func sourceQueryFromXylona(info *xylona.SourceQueryInfo) *SourceQueryInfo {
	if info == nil {
		return nil
	}
	return &SourceQueryInfo{
		Name:                info.GetName(),
		Map:                 info.GetMap(),
		Game:                info.GetGame(),
		AppID:               info.GetAppId(),
		SteamID:             info.GetSteamId(),
		GameID:              info.GetGameId(),
		Players:             info.GetPlayers(),
		MaxPlayers:          info.GetMaxPlayers(),
		Bots:                info.GetBots(),
		ServerOS:            info.GetServerOs(),
		Visibility:          info.GetVisibility(),
		VAC:                 info.GetVac(),
		Version:             info.GetVersion(),
		Protocol:            info.GetProtocol(),
		PlayerList:          append([]string(nil), info.GetPlayerList()...),
		PlayerListSupported: info.GetPlayerListSupported(),
	}
}

func palworldQueryFromDetailed(info *query.PalworldInfo) *PalworldQueryInfo {
	if info == nil {
		return nil
	}
	playerList := make([]string, 0, len(info.PlayerDetails))
	playerDetails := make([]GameServerPlayer, 0, len(info.PlayerDetails))
	for _, player := range info.PlayerDetails {
		playerList = append(playerList, player.Name)
		playerDetails = append(playerDetails, GameServerPlayer{Name: player.Name, ID: player.UserID})
	}
	return &PalworldQueryInfo{
		Name:              info.Name,
		Description:       info.Description,
		Version:           info.Version,
		WorldGUID:         info.WorldGUID,
		Players:           info.Players,
		MaxPlayers:        info.MaxPlayers,
		PlayerList:        playerList,
		UptimeSeconds:     info.UptimeSeconds,
		ServerFPS:         info.ServerFPS,
		ServerFrameTimeMS: info.ServerFrameTimeMS,
		Days:              info.Days,
		Responded:         info.Responded,
		PlayerDetails:     playerDetails,
	}
}
