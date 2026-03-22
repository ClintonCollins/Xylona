package helpers

import (
	"strings"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func GameServerModelStatusToProtoStatus(status string) xylona.Status {
	statusNormalized := strings.TrimSpace(strings.ToUpper(status))
	switch statusNormalized {
	case xylona.Status_UNKNOWN.String():
		return xylona.Status_UNKNOWN
	case xylona.Status_OFFLINE.String():
		return xylona.Status_OFFLINE
	case xylona.Status_ONLINE.String():
		return xylona.Status_ONLINE
	case xylona.Status_INSTALLING.String():
		return xylona.Status_INSTALLING
	case xylona.Status_UPDATING.String():
		return xylona.Status_UPDATING
	}

	if len(status) == 1 {
		statusFromRune := xylona.Status(status[0])
		switch statusFromRune {
		case xylona.Status_UNKNOWN, xylona.Status_OFFLINE, xylona.Status_ONLINE, xylona.Status_INSTALLING, xylona.Status_UPDATING:
			return statusFromRune
		}
	}

	return xylona.Status_UNKNOWN
}

// GameServerProtoToModel converts a *xylona.GameServer to a *models.GameServer
func GameServerProtoToModel(gsProto *xylona.GameServer) *models.GameServer {
	return &models.GameServer{
		ID:                        gsProto.Id,
		UserID:                    gsProto.UserId,
		Name:                      gsProto.Name,
		GameID:                    gsProto.GameId,
		StartCommand:              gsProto.StartCommand,
		Status:                    gsProto.Status.String(),
		SetPlayers:                gsProto.SetMaxPlayers,
		MaxPlayers:                gsProto.MaxPlayers,
		Map:                       gsProto.Map,
		IP:                        gsProto.Ip.Address,
		Port:                      gsProto.Port,
		QueryPort:                 gsProto.QueryPort,
		Directory:                 gsProto.Directory,
		MaxMemoryMB:               gsProto.MaxMemoryMb,
		BackupsEnabled:            gsProto.BackupsEnabled,
		SteamGameServerLoginToken: gsProto.SteamGameServerLoginToken,
		BackupDirectory:           gsProto.BackupDirectory,
		MaxBackups:                gsProto.MaxBackups,
		NodeID:                    gsProto.NodeId,
		CreatedAt:                 gsProto.CreatedAt.AsTime(),
		UpdatedAt:                 gsProto.UpdatedAt.AsTime(),
	}
}

func GameServerModelToProto(gsModel *models.GameServer) *xylona.GameServer {
	gameName := ""
	if gsModel.R.Game != nil {
		gameName = gsModel.R.Game.Name
	}
	userName := ""
	if gsModel.R.User != nil {
		userName = gsModel.R.User.UserName
	}
	if gsModel.R.Node == nil {
		log.Debug().Msgf("Node is nil for GameServer %s", gsModel.ID)
		gsModel.R.Node = &models.Node{}
	}
	return &xylona.GameServer{
		Id:                        gsModel.ID,
		UserId:                    gsModel.UserID,
		Name:                      gsModel.Name,
		GameId:                    gsModel.GameID,
		StartCommand:              gsModel.StartCommand,
		Status:                    GameServerModelStatusToProtoStatus(gsModel.Status),
		SetMaxPlayers:             gsModel.SetPlayers,
		MaxPlayers:                gsModel.MaxPlayers,
		Map:                       gsModel.Map,
		Ip:                        &xylona.IP{Address: gsModel.IP},
		Port:                      gsModel.Port,
		QueryPort:                 gsModel.QueryPort,
		Directory:                 gsModel.Directory,
		MaxMemoryMb:               gsModel.MaxMemoryMB,
		BackupsEnabled:            gsModel.BackupsEnabled,
		BackupDirectory:           gsModel.BackupDirectory,
		MaxBackups:                gsModel.MaxBackups,
		CreatedAt:                 timestamppb.New(gsModel.CreatedAt),
		UpdatedAt:                 timestamppb.New(gsModel.UpdatedAt),
		SteamGameServerLoginToken: gsModel.SteamGameServerLoginToken,
		UserName:                  userName,
		GameName:                  gameName,
		NodeId:                    gsModel.NodeID,
		NodeName:                  gsModel.R.Node.Name,
		NodeHost:                  gsModel.R.Node.Host,
		NodePort:                  gsModel.R.Node.Port,
		Version:                   gsModel.Version,
		ServerSoftware:            gsModel.ServerSoftware.GetOr(""),
		Game:                      gameProtoFromRelation(gsModel),
	}
}

func gameProtoFromRelation(gsModel *models.GameServer) *xylona.Game {
	if gsModel.R.Game == nil {
		return nil
	}
	return GameModelToProto(gsModel.R.Game)
}

func GameServerModelToSetter(gsModel *models.GameServer) *models.GameServerSetter {
	return &models.GameServerSetter{
		ID:                        omit.From(gsModel.ID),
		UserID:                    omit.From(gsModel.UserID),
		Name:                      omit.From(gsModel.Name),
		GameID:                    omit.From(gsModel.GameID),
		StartCommand:              omit.From(gsModel.StartCommand),
		Status:                    omit.From(gsModel.Status),
		SetPlayers:                omit.From(gsModel.SetPlayers),
		MaxPlayers:                omit.From(gsModel.MaxPlayers),
		Map:                       omit.From(gsModel.Map),
		IP:                        omit.From(gsModel.IP),
		Port:                      omit.From(gsModel.Port),
		QueryPort:                 omit.From(gsModel.QueryPort),
		Directory:                 omit.From(gsModel.Directory),
		MaxMemoryMB:               omit.From(gsModel.MaxMemoryMB),
		BackupsEnabled:            omit.From(gsModel.BackupsEnabled),
		SteamGameServerLoginToken: omit.From(gsModel.SteamGameServerLoginToken),
		BackupDirectory:           omit.From(gsModel.BackupDirectory),
		MaxBackups:                omit.From(gsModel.MaxBackups),
		NodeID:                    omit.From(gsModel.NodeID),
		CreatedAt:                 omit.From(gsModel.CreatedAt),
		UpdatedAt:                 omit.From(gsModel.UpdatedAt),
	}
}

func GameModelToProto(gameModel *models.Game) *xylona.Game {
	return &xylona.Game{
		Id:                                gameModel.ID,
		Name:                              gameModel.Name,
		DefaultPort:                       gameModel.DefaultPort,
		DefaultQueryPort:                  gameModel.DefaultQueryPort,
		DefaultMaxPlayers:                 gameModel.DefaultMaxPlayers,
		LinuxSupport:                      gameModel.LinuxSupport,
		WindowsSupport:                    gameModel.WindowsSupport,
		LinuxStartCommand:                 gameModel.LinuxStartCommand,
		LinuxStopCommand:                  gameModel.LinuxStopCommand,
		LinuxInstallCommand:               gameModel.LinuxInstallCommand,
		LinuxInstallCommandProcessor:      commandTypeToCommandProcessor(gameModel.LinuxInstallCommandType),
		LinuxUpdateCommand:                gameModel.LinuxUpdateCommand,
		LinuxUpdateCommandProcessor:       commandTypeToCommandProcessor(gameModel.LinuxUpdateCommandType),
		LinuxWorkingDirectory:             gameModel.LinuxWorkingDirectory,
		WindowsStartCommand:               gameModel.WindowsStartCommand,
		WindowsStopCommand:                gameModel.WindowsStopCommand,
		WindowsInstallCommand:             gameModel.WindowsInstallCommand,
		WindowsInstallCommandProcessor:    commandTypeToCommandProcessor(gameModel.WindowsInstallCommandType),
		WindowsUpdateCommand:              gameModel.WindowsUpdateCommand,
		WindowsUpdateCommandProcessor:     commandTypeToCommandProcessor(gameModel.WindowsUpdateCommandType),
		WindowsWorkingDirectory:           gameModel.WindowsWorkingDirectory,
		RequireDedicatedIp:                gameModel.RequireDedicatedIP,
		BindsToAllIps:                     gameModel.BindsToAllIps,
		CreatedAt:                         timestamppb.New(gameModel.CreatedAt),
		UpdatedAt:                         timestamppb.New(gameModel.UpdatedAt),
		UsesSourceQuery:                   gameModel.UsesSourceQuery,
		RequiresSteamGameServerLoginToken: gameModel.RequiresSteamGameServerLoginToken,
		ConfigSchemas:                     gameModel.ConfigSchemas.GetOr(""),
		ServerSoftware:                    gameModel.ServerSoftware.GetOr(""),
	}
}

func GameProtoToModel(gameProto *xylona.Game) *models.Game {
	return &models.Game{
		ID:                                gameProto.Id,
		Name:                              gameProto.Name,
		DefaultPort:                       gameProto.DefaultPort,
		DefaultQueryPort:                  gameProto.DefaultQueryPort,
		DefaultMaxPlayers:                 gameProto.DefaultMaxPlayers,
		LinuxSupport:                      gameProto.LinuxSupport,
		WindowsSupport:                    gameProto.WindowsSupport,
		LinuxStartCommand:                 gameProto.LinuxStartCommand,
		LinuxStopCommand:                  gameProto.LinuxStopCommand,
		LinuxInstallCommand:               gameProto.LinuxInstallCommand,
		LinuxInstallCommandType:           commandProcessorToCommandType(gameProto.LinuxInstallCommandProcessor),
		LinuxUpdateCommand:                gameProto.LinuxUpdateCommand,
		LinuxUpdateCommandType:            commandProcessorToCommandType(gameProto.LinuxUpdateCommandProcessor),
		LinuxWorkingDirectory:             gameProto.LinuxWorkingDirectory,
		WindowsStartCommand:               gameProto.WindowsStartCommand,
		WindowsStopCommand:                gameProto.WindowsStopCommand,
		WindowsInstallCommand:             gameProto.WindowsInstallCommand,
		WindowsInstallCommandType:         commandProcessorToCommandType(gameProto.WindowsInstallCommandProcessor),
		WindowsUpdateCommand:              gameProto.WindowsUpdateCommand,
		WindowsUpdateCommandType:          commandProcessorToCommandType(gameProto.WindowsUpdateCommandProcessor),
		WindowsWorkingDirectory:           gameProto.WindowsWorkingDirectory,
		RequireDedicatedIP:                gameProto.RequireDedicatedIp,
		BindsToAllIps:                     gameProto.BindsToAllIps,
		CreatedAt:                         gameProto.CreatedAt.AsTime(),
		UpdatedAt:                         gameProto.UpdatedAt.AsTime(),
		UsesSourceQuery:                   gameProto.UsesSourceQuery,
		RequiresSteamGameServerLoginToken: gameProto.RequiresSteamGameServerLoginToken,
		ConfigSchemas:                     null.FromCond(gameProto.ConfigSchemas, gameProto.ConfigSchemas != ""),
		ServerSoftware:                    null.FromCond(gameProto.ServerSoftware, gameProto.ServerSoftware != ""),
	}
}

func GameModelToGameSetter(gameModel *models.Game) *models.GameSetter {
	return &models.GameSetter{
		ID:                                omit.From(gameModel.ID),
		Name:                              omit.From(gameModel.Name),
		DefaultPort:                       omit.From(gameModel.DefaultPort),
		DefaultQueryPort:                  omit.From(gameModel.DefaultQueryPort),
		DefaultMaxPlayers:                 omit.From(gameModel.DefaultMaxPlayers),
		RequireDedicatedIP:                omit.From(gameModel.RequireDedicatedIP),
		BindsToAllIps:                     omit.From(gameModel.BindsToAllIps),
		UsesSourceQuery:                   omit.From(gameModel.UsesSourceQuery),
		UsesSteamcmd:                      omit.From(gameModel.UsesSteamcmd),
		SteamAppID:                        omit.From(gameModel.SteamAppID),
		RequiresSteamGameServerLoginToken: omit.From(gameModel.RequiresSteamGameServerLoginToken),
		LinuxSupport:                      omit.From(gameModel.LinuxSupport),
		LinuxStartCommand:                 omit.From(gameModel.LinuxStartCommand),
		LinuxStopCommand:                  omit.From(gameModel.LinuxStopCommand),
		LinuxInstallCommand:               omit.From(gameModel.LinuxInstallCommand),
		LinuxInstallCommandType:           omit.From(gameModel.LinuxInstallCommandType),
		LinuxUpdateCommand:                omit.From(gameModel.LinuxUpdateCommand),
		LinuxUpdateCommandType:            omit.From(gameModel.LinuxUpdateCommandType),
		LinuxWorkingDirectory:             omit.From(gameModel.LinuxWorkingDirectory),
		WindowsSupport:                    omit.From(gameModel.WindowsSupport),
		WindowsStartCommand:               omit.From(gameModel.WindowsStartCommand),
		WindowsStopCommand:                omit.From(gameModel.WindowsStopCommand),
		WindowsInstallCommand:             omit.From(gameModel.WindowsInstallCommand),
		WindowsInstallCommandType:         omit.From(gameModel.WindowsInstallCommandType),
		WindowsUpdateCommand:              omit.From(gameModel.WindowsUpdateCommand),
		WindowsUpdateCommandType:          omit.From(gameModel.WindowsUpdateCommandType),
		WindowsWorkingDirectory:           omit.From(gameModel.WindowsWorkingDirectory),
		ConfigSchemas:                     omitnull.FromNull(gameModel.ConfigSchemas),
		ServerSoftware:                    omitnull.FromNull(gameModel.ServerSoftware),
		CreatedAt:                         omit.From(time.Now()),
		UpdatedAt:                         omit.From(time.Now()),
	}
}

func UserModelToProto(userModel *models.User) *xylona.User {
	return &xylona.User{
		Id:        userModel.ID,
		UserName:  userModel.UserName,
		Email:     userModel.Email,
		FirstName: userModel.FirstName,
		LastName:  userModel.LastName,
		SuperUser: userModel.SuperUser,
		LastLogin: timestamppb.New(userModel.LastLoginAt),
		CreatedAt: timestamppb.New(userModel.CreatedAt),
	}
}

func UserProtoToModel(userProto *xylona.User) *models.User {
	return &models.User{
		ID:           userProto.Id,
		UserName:     userProto.UserName,
		Email:        userProto.Email,
		FirstName:    userProto.FirstName,
		LastName:     userProto.LastName,
		SuperUser:    userProto.SuperUser,
		LastLoginAt:  userProto.LastLogin.AsTime(),
		CreatedAt:    userProto.CreatedAt.AsTime(),
		PasswordHash: "",
	}
}

func IPModelToProto(ipModel *models.IP) *xylona.IP {
	return &xylona.IP{
		Address:  ipModel.Address,
		Usable:   ipModel.Usable,
		External: ipModel.External,
	}
}

func commandTypeToCommandProcessor(commandType string) xylona.CommandProcessor {
	switch commandType {
	case "direct":
		return xylona.CommandProcessor_DIRECT
	case "bash":
		return xylona.CommandProcessor_BASH
	case "cmd":
		return xylona.CommandProcessor_CMD
	case "powershell":
		return xylona.CommandProcessor_POWERSHELL
	case "internal":
		return xylona.CommandProcessor_XYLONA_INTERNAL
	default:
		return xylona.CommandProcessor_DIRECT
	}
}

func commandProcessorToCommandType(commandProcessor xylona.CommandProcessor) string {
	switch commandProcessor {
	case xylona.CommandProcessor_DIRECT:
		return "direct"
	case xylona.CommandProcessor_BASH:
		return "bash"
	case xylona.CommandProcessor_CMD:
		return "cmd"
	case xylona.CommandProcessor_POWERSHELL:
		return "powershell"
	case xylona.CommandProcessor_XYLONA_INTERNAL:
		return "internal"
	default:
		return "direct"
	}
}

func NodeProtoToModel(nodeProto *xylona.Node) *models.Node {
	secretKey := strings.TrimSpace(nodeProto.SecretKey)

	return &models.Node{
		ID:               nodeProto.Id,
		Name:             strings.TrimSpace(nodeProto.Name),
		Host:             strings.TrimSpace(nodeProto.Host),
		Port:             nodeProto.Port,
		SecretKey:        null.FromCond(secretKey, secretKey != ""),
		IsLocal:          nodeProto.Local,
		BaseURL:          strings.TrimSpace(nodeProto.BaseUrl),
		AllowInsecureTLS: nodeProto.AllowInsecureTls,
	}
}

func NodeModelToProto(nodeModel *models.Node) *xylona.Node {
	return &xylona.Node{
		Id:               nodeModel.ID,
		Name:             nodeModel.Name,
		Host:             nodeModel.Host,
		Port:             nodeModel.Port,
		Local:            nodeModel.IsLocal,
		SecretKey:        nodeModel.SecretKey.GetOr(""),
		BaseUrl:          nodeModel.BaseURL,
		Enabled:          nodeModel.Enabled,
		LastSeenAt:       timestamppb.New(nodeModel.LastSeenAt.GetOr(time.Time{})),
		LastSyncAt:       timestamppb.New(nodeModel.LastSyncAt.GetOr(time.Time{})),
		LastSyncStatus:   nodeModel.LastSyncStatus,
		HealthStatus:     nodeModel.HealthStatus,
		Version:          nodeModel.Version,
		ProtocolVersion:  nodeModel.ProtocolVersion,
		Capabilities:     nodeModel.Capabilities,
		CreatedAt:        timestamppb.New(nodeModel.CreatedAt.GetOr(time.Time{})),
		UpdatedAt:        timestamppb.New(nodeModel.UpdatedAt.GetOr(time.Time{})),
		AllowInsecureTls: nodeModel.AllowInsecureTLS,
		Departed:         nodeModel.Departed,
		AutoPaired:       nodeModel.AutoPaired,
	}
}

func NodeModelToSetter(nodeModel *models.Node) *models.NodeSetter {
	return &models.NodeSetter{
		ID:               omit.From(nodeModel.ID),
		Name:             omit.From(nodeModel.Name),
		SecretKey:        omitnull.FromNull(nodeModel.SecretKey),
		Host:             omit.From(nodeModel.Host),
		Port:             omit.From(nodeModel.Port),
		BaseURL:          omit.From(nodeModel.BaseURL),
		AllowInsecureTLS: omit.From(nodeModel.AllowInsecureTLS),
	}
}

// RemoteServerCacheToProto converts a cached remote server record into a GameServer proto,
// suitable for use in GetGameServerResponse when the peer is unreachable.
func RemoteServerCacheToProto(rsc *models.RemoteServerCache, node *models.Node) *xylona.GameServer {
	return &xylona.GameServer{
		Id:                 rsc.RemoteServerID,
		Name:               rsc.DisplayName,
		GameId:             rsc.GameID,
		GameName:           rsc.GameName,
		Status:             GameServerModelStatusToProtoStatus(rsc.Status),
		Ip:                 &xylona.IP{Address: rsc.IPAddress},
		Port:               rsc.Port,
		QueryPort:          rsc.QueryPort,
		SetMaxPlayers:      rsc.MaxPlayers,
		MaxPlayers:         rsc.MaxPlayers,
		CurrentPlayerCount: rsc.CurrentPlayers,
		Map:                rsc.MapName,
		Version:            rsc.Version,
		NodeId:             node.ID,
		NodeName:           node.Name,
		NodeHost:           node.BaseURL,
	}
}

func InstalledModModelToProto(mod *models.InstalledMod) *xylona.InstalledMod {
	return &xylona.InstalledMod{
		Id:                 mod.ID,
		GameServerId:       mod.GameServerID,
		Source:             mod.Source,
		SourceId:           mod.SourceID,
		ModName:            mod.ModName,
		ModAuthor:          mod.ModAuthor,
		InstalledVersion:   mod.InstalledVersion,
		InstalledVersionId: mod.InstalledVersionID,
		FileHash:           mod.FileHash,
		AutoUpdate:         mod.AutoUpdate != 0,
		Enabled:            mod.Enabled != 0,
		PinnedVersion:      mod.PinnedVersion.GetOr(""),
		UpdateAvailable:    false,
		LatestVersion:      "",
		CreatedAt:          timestamppb.New(mod.CreatedAt),
		UpdatedAt:          timestamppb.New(mod.UpdatedAt),
	}
}

func InstalledModFileModelToProto(file *models.InstalledModFile) *xylona.InstalledModFile {
	return &xylona.InstalledModFile{
		Id:             file.ID,
		InstalledModId: file.InstalledModID,
		FilePath:       file.FilePath,
		FileHash:       file.FileHash,
		FileSize:       file.FileSize,
		IsPrimary:      file.IsPrimary != 0,
	}
}

func NodeApiKeyModelToProto(key *models.NodeAPIKey) *xylona.NodeApiKey {
	maskedKey := "****"
	if len(key.APIKey) >= 4 {
		maskedKey = key.APIKey[:4] + "****"
	}
	return &xylona.NodeApiKey{
		Id:          key.ID,
		ServiceName: key.ServiceName,
		MaskedKey:   maskedKey,
		CreatedAt:   timestamppb.New(key.CreatedAt),
		UpdatedAt:   timestamppb.New(key.UpdatedAt),
	}
}

func RemoteServerCacheModelToProto(rsc *models.RemoteServerCache) *xylona.RemoteServerSummary {
	return &xylona.RemoteServerSummary{
		Id:               rsc.ID,
		SourceNodeId:     rsc.SourceNodeID,
		NodeId:           rsc.NodeID,
		RemoteServerId:   rsc.RemoteServerID,
		DisplayName:      rsc.DisplayName,
		Status:           GameServerModelStatusToProtoStatus(rsc.Status),
		GameName:         rsc.GameName,
		GameId:           rsc.GameID,
		IpAddress:        rsc.IPAddress,
		Port:             rsc.Port,
		QueryPort:        rsc.QueryPort,
		MaxPlayers:       rsc.MaxPlayers,
		CurrentPlayers:   rsc.CurrentPlayers,
		MapName:          rsc.MapName,
		Version:          rsc.Version,
		NodeName:         rsc.NodeName,
		NodeHost:         rsc.NodeHost,
		LastRemoteUpdate: timestamppb.New(rsc.LastRemoteUpdate.GetOr(time.Time{})),
		LastSyncedAt:     timestamppb.New(rsc.LastSyncedAt.GetOr(time.Time{})),
		IsStale:          rsc.IsStale,
	}
}
