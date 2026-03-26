package helpers

import (
	"strings"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
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
	ipAddress := ""
	if gsProto.Ip != nil {
		ipAddress = gsProto.Ip.Address
	}

	return &models.GameServer{
		ID:                        gsProto.Id,
		UserID:                    gsProto.UserId,
		Name:                      gsProto.Name,
		GameID:                    gsProto.GameId,
		StartArgsPatches:          gsProto.StartArgsPatches,
		Status:                    gsProto.Status.String(),
		SetPlayers:                gsProto.SetMaxPlayers,
		MaxPlayers:                gsProto.MaxPlayers,
		Map:                       gsProto.Map,
		IP:                        ipAddress,
		Port:                      gsProto.Port,
		QueryPort:                 gsProto.QueryPort,
		Directory:                 gsProto.Directory,
		MaxMemoryMB:               gsProto.MaxMemoryMb,
		BackupsEnabled:            gsProto.BackupsEnabled,
		SteamGameServerLoginToken: gsProto.SteamGameServerLoginToken,
		BackupDirectory:           gsProto.BackupDirectory,
		MaxBackups:                gsProto.MaxBackups,
		NodeID:                    gsProto.NodeId,
		Branch:                    gsProto.SelectedTarget,
		TargetPinned:              gsProto.SelectedTargetPinned,
		ServerSoftware:            null.FromCond(gsProto.SelectedVariantId, gsProto.SelectedVariantId != ""),
		ServerExecutable:          null.FromCond(gsProto.ServerExecutable, gsProto.ServerExecutable != ""),
		CreatedAt:                 gsProto.CreatedAt.AsTime(),
		UpdatedAt:                 gsProto.UpdatedAt.AsTime(),
	}
}

func GameServerModelToProto(gsModel *models.GameServer, vsm *versiontracker.VersionStateMap) *xylona.GameServer {
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
	proto := &xylona.GameServer{
		Id:                        gsModel.ID,
		UserId:                    gsModel.UserID,
		Name:                      gsModel.Name,
		GameId:                    gsModel.GameID,
		StartArgsPatches:          gsModel.StartArgsPatches,
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
		SelectedTarget:            gsModel.Branch,
		SelectedTargetPinned:      gsModel.TargetPinned,
		SelectedVariantId:         gsModel.ServerSoftware.GetOr(""),
		ServerExecutable:          gsModel.ServerExecutable.GetOr(""),
		Game:                      gameProtoFromRelation(gsModel),
	}
	if gsModel.R.Game != nil {
		resolvedConfig, errResolve := updateproviders.ResolveModelConfig(gsModel.R.Game, gsModel)
		if errResolve != nil {
			log.Warn().Err(errResolve).Str("game_server_id", gsModel.ID).Msg("failed to resolve typed game server config")
		} else {
			proto.ResolvedUpdateProvider = updateproviders.ProviderConfigToProto(resolvedConfig.Provider)
			proto.ResolvedModProfile = updateproviders.ModProfileToProto(resolvedConfig.ModProfile)
			proto.ResolvedHasUpdate = resolvedConfig.Provider.Kind != updateproviders.ProviderKindNone
			proto.ResolvedHasModSupport = resolvedConfig.ModProfile != nil
		}
	}
	if vsm != nil {
		proto.VersionInfo = versionStateToVersionInfoProto(vsm.Get(gsModel.ID))
	}
	return proto
}

func versionStateToVersionInfoProto(state versiontracker.VersionState) *xylona.VersionInfo {
	protoStatus := xylona.VersionStatus_VERSION_STATUS_NO_TRACKER
	switch state.Status {
	case versiontracker.VersionStatusUnchecked:
		protoStatus = xylona.VersionStatus_VERSION_STATUS_UNCHECKED
	case versiontracker.VersionStatusChecking:
		protoStatus = xylona.VersionStatus_VERSION_STATUS_CHECKING
	case versiontracker.VersionStatusChecked:
		protoStatus = xylona.VersionStatus_VERSION_STATUS_CHECKED
	case versiontracker.VersionStatusError:
		protoStatus = xylona.VersionStatus_VERSION_STATUS_ERROR
	}

	var lastCheckUnix int64
	if !state.LastCheckTime.IsZero() {
		lastCheckUnix = state.LastCheckTime.Unix()
	}

	return &xylona.VersionInfo{
		InstalledVersion:      state.InstalledVersion,
		LatestVersion:         state.LatestVersion,
		UpdateAvailable:       state.UpdateAvailable,
		LastCheckTime:         lastCheckUnix,
		TrackerType:           state.TrackerType,
		Status:                protoStatus,
		InstalledVersionLabel: state.InstalledVersionLabel,
		LatestVersionLabel:    state.LatestVersionLabel,
		InstalledBranch:       state.InstalledBranch,
		LatestBranch:          state.LatestBranch,
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
		StartArgsPatches:          omit.From(gsModel.StartArgsPatches),
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
		Branch:                    omit.From(gsModel.Branch),
		ServerSoftware:            omitnull.FromNull(gsModel.ServerSoftware),
		ServerExecutable:          omitnull.FromNull(gsModel.ServerExecutable),
		TargetPinned:              omit.From(gsModel.TargetPinned),
		CreatedAt:                 omit.From(gsModel.CreatedAt),
		UpdatedAt:                 omit.From(gsModel.UpdatedAt),
	}
}

func GameModelToProto(gameModel *models.Game) *xylona.Game {
	gameConfig, errConfig := updateproviders.LoadGameConfigFromModel(gameModel)
	if errConfig != nil {
		log.Warn().Err(errConfig).Str("game_id", gameModel.ID).Msg("failed to load typed game config")
	}

	linuxInstallType := modelCommandTypeToProtoCommandType(
		gameModel.LinuxInstallCommandType,
		gameModel.LinuxInstallCommand,
		gameConfig.UpdateProvider.Kind,
	)
	linuxUpdateType := modelCommandTypeToProtoCommandType(
		gameModel.LinuxUpdateCommandType,
		gameModel.LinuxUpdateCommand,
		gameConfig.UpdateProvider.Kind,
	)
	windowsInstallType := modelCommandTypeToProtoCommandType(
		gameModel.WindowsInstallCommandType,
		gameModel.WindowsInstallCommand,
		gameConfig.UpdateProvider.Kind,
	)
	windowsUpdateType := modelCommandTypeToProtoCommandType(
		gameModel.WindowsUpdateCommandType,
		gameModel.WindowsUpdateCommand,
		gameConfig.UpdateProvider.Kind,
	)

	return &xylona.Game{
		Id:                                gameModel.ID,
		Name:                              gameModel.Name,
		DefaultPort:                       gameModel.DefaultPort,
		DefaultQueryPort:                  gameModel.DefaultQueryPort,
		DefaultMaxPlayers:                 gameModel.DefaultMaxPlayers,
		LinuxSupport:                      gameModel.LinuxSupport,
		WindowsSupport:                    gameModel.WindowsSupport,
		LinuxStopCommand:                  gameModel.LinuxStopCommand,
		LinuxInstallCommand:               gameModel.LinuxInstallCommand,
		LinuxInstallCommandProcessor:      commandTypeToCommandProcessor(gameModel.LinuxInstallCommandType),
		LinuxUpdateCommand:                gameModel.LinuxUpdateCommand,
		LinuxUpdateCommandProcessor:       commandTypeToCommandProcessor(gameModel.LinuxUpdateCommandType),
		LinuxWorkingDirectory:             gameModel.LinuxWorkingDirectory,
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
		UsesSteamcmd:                      gameModel.UsesSteamcmd,
		SteamAppid:                        gameModel.SteamAppID,
		ConfigSchemas:                     gameModel.ConfigSchemas.GetOr(""),
		LinuxStartArgsTemplate:            gameModel.LinuxStartArgsTemplate.GetOr(""),
		WindowsStartArgsTemplate:          gameModel.WindowsStartArgsTemplate.GetOr(""),
		LinuxBaseCommand:                  gameModel.LinuxBaseCommand,
		WindowsBaseCommand:                gameModel.WindowsBaseCommand,
		StartArgBlocklist:                 gameModel.StartArgBlocklist,
		AllowStartArgEditing:              gameModel.AllowStartArgEditing,
		UpdateProvider:                    updateproviders.ProviderConfigToProto(gameConfig.UpdateProvider),
		DefaultTarget:                     gameConfig.DefaultTarget,
		ModProfile:                        updateproviders.ModProfileToProto(gameConfig.ModProfile),
		Variants:                          updateproviders.VariantsToProto(gameConfig.Variants),
		LinuxInstallType:                  linuxInstallType,
		LinuxUpdateType:                   linuxUpdateType,
		WindowsInstallType:                windowsInstallType,
		WindowsUpdateType:                 windowsUpdateType,
	}
}

func GameProtoToModel(gameProto *xylona.Game) *models.Game {
	linuxInstallCommand := commandValueForType(
		gameProto.GetLinuxInstallType(),
		gameProto.GetLinuxInstallCommand(),
		gameProto.GetSteamAppid(),
	)
	linuxUpdateCommand := commandValueForType(
		gameProto.GetLinuxUpdateType(),
		gameProto.GetLinuxUpdateCommand(),
		gameProto.GetSteamAppid(),
	)
	windowsInstallCommand := commandValueForType(
		gameProto.GetWindowsInstallType(),
		gameProto.GetWindowsInstallCommand(),
		gameProto.GetSteamAppid(),
	)
	windowsUpdateCommand := commandValueForType(
		gameProto.GetWindowsUpdateType(),
		gameProto.GetWindowsUpdateCommand(),
		gameProto.GetSteamAppid(),
	)

	gameModel := &models.Game{
		ID:                                gameProto.Id,
		Name:                              gameProto.Name,
		DefaultPort:                       gameProto.DefaultPort,
		DefaultQueryPort:                  gameProto.DefaultQueryPort,
		DefaultMaxPlayers:                 gameProto.DefaultMaxPlayers,
		LinuxSupport:                      gameProto.LinuxSupport,
		WindowsSupport:                    gameProto.WindowsSupport,
		LinuxStopCommand:                  gameProto.LinuxStopCommand,
		LinuxInstallCommand:               linuxInstallCommand,
		LinuxInstallCommandType:           protoCommandTypeToModelCommandType(gameProto.GetLinuxInstallType(), gameProto.GetLinuxInstallCommandProcessor()),
		LinuxUpdateCommand:                linuxUpdateCommand,
		LinuxUpdateCommandType:            protoCommandTypeToModelCommandType(gameProto.GetLinuxUpdateType(), gameProto.GetLinuxUpdateCommandProcessor()),
		LinuxWorkingDirectory:             gameProto.LinuxWorkingDirectory,
		WindowsStopCommand:                gameProto.WindowsStopCommand,
		WindowsInstallCommand:             windowsInstallCommand,
		WindowsInstallCommandType:         protoCommandTypeToModelCommandType(gameProto.GetWindowsInstallType(), gameProto.GetWindowsInstallCommandProcessor()),
		WindowsUpdateCommand:              windowsUpdateCommand,
		WindowsUpdateCommandType:          protoCommandTypeToModelCommandType(gameProto.GetWindowsUpdateType(), gameProto.GetWindowsUpdateCommandProcessor()),
		WindowsWorkingDirectory:           gameProto.WindowsWorkingDirectory,
		RequireDedicatedIP:                gameProto.RequireDedicatedIp,
		BindsToAllIps:                     gameProto.BindsToAllIps,
		CreatedAt:                         gameProto.CreatedAt.AsTime(),
		UpdatedAt:                         gameProto.UpdatedAt.AsTime(),
		UsesSourceQuery:                   gameProto.UsesSourceQuery,
		RequiresSteamGameServerLoginToken: gameProto.RequiresSteamGameServerLoginToken,
		UsesSteamcmd:                      gameProto.UsesSteamcmd || hasSteamCommandType(gameProto),
		SteamAppID:                        gameProto.SteamAppid,
		ConfigSchemas:                     null.FromCond(gameProto.ConfigSchemas, gameProto.ConfigSchemas != ""),
		LinuxStartArgsTemplate:            null.FromCond(gameProto.GetLinuxStartArgsTemplate(), gameProto.GetLinuxStartArgsTemplate() != ""),
		WindowsStartArgsTemplate:          null.FromCond(gameProto.GetWindowsStartArgsTemplate(), gameProto.GetWindowsStartArgsTemplate() != ""),
		LinuxBaseCommand:                  gameProto.GetLinuxBaseCommand(),
		WindowsBaseCommand:                gameProto.GetWindowsBaseCommand(),
		StartArgBlocklist:                 gameProto.GetStartArgBlocklist(),
		AllowStartArgEditing:              gameProto.GetAllowStartArgEditing(),
	}

	gameConfig := updateproviders.GameConfig{
		UpdateProvider: updateproviders.ProviderConfigFromProto(gameProto.GetUpdateProvider()),
		DefaultTarget:  strings.TrimSpace(gameProto.GetDefaultTarget()),
		ModProfile:     updateproviders.ModProfileFromProto(gameProto.GetModProfile()),
		Variants:       updateproviders.VariantsFromProto(gameProto.GetVariants()),
	}
	gameConfig = normalizeGameConfigForCommandTypes(gameProto, gameConfig)
	errConfig := updateproviders.SaveGameConfigToModel(gameModel, gameConfig)
	if errConfig != nil {
		log.Warn().Err(errConfig).Str("game_id", gameProto.GetId()).Msg("failed to save typed game config")
	}

	return gameModel
}

func GameModelToGameSetter(gameModel *models.Game) *models.GameSetter {
	gameConfig, errConfig := updateproviders.LoadGameConfigFromModel(gameModel)
	if errConfig != nil {
		log.Warn().Err(errConfig).Str("game_id", gameModel.ID).Msg("failed to load typed game config for setter")
	}
	configModel := &models.Game{ServerSoftware: gameModel.ServerSoftware}
	if errConfig == nil {
		errSave := updateproviders.SaveGameConfigToModel(configModel, gameConfig)
		if errSave != nil {
			log.Warn().Err(errSave).Str("game_id", gameModel.ID).Msg("failed to marshal typed game config for setter")
		}
	}
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
		LinuxStopCommand:                  omit.From(gameModel.LinuxStopCommand),
		LinuxInstallCommand:               omit.From(gameModel.LinuxInstallCommand),
		LinuxInstallCommandType:           omit.From(gameModel.LinuxInstallCommandType),
		LinuxUpdateCommand:                omit.From(gameModel.LinuxUpdateCommand),
		LinuxUpdateCommandType:            omit.From(gameModel.LinuxUpdateCommandType),
		LinuxWorkingDirectory:             omit.From(gameModel.LinuxWorkingDirectory),
		WindowsSupport:                    omit.From(gameModel.WindowsSupport),
		WindowsStopCommand:                omit.From(gameModel.WindowsStopCommand),
		WindowsInstallCommand:             omit.From(gameModel.WindowsInstallCommand),
		WindowsInstallCommandType:         omit.From(gameModel.WindowsInstallCommandType),
		WindowsUpdateCommand:              omit.From(gameModel.WindowsUpdateCommand),
		WindowsUpdateCommandType:          omit.From(gameModel.WindowsUpdateCommandType),
		WindowsWorkingDirectory:           omit.From(gameModel.WindowsWorkingDirectory),
		ConfigSchemas:                     omitnull.FromNull(gameModel.ConfigSchemas),
		ServerSoftware:                    omitnull.FromNull(configModel.ServerSoftware),
		LinuxStartArgsTemplate:            omitnull.FromNull(gameModel.LinuxStartArgsTemplate),
		WindowsStartArgsTemplate:          omitnull.FromNull(gameModel.WindowsStartArgsTemplate),
		LinuxBaseCommand:                  omit.From(gameModel.LinuxBaseCommand),
		WindowsBaseCommand:                omit.From(gameModel.WindowsBaseCommand),
		StartArgBlocklist:                 omit.From(gameModel.StartArgBlocklist),
		AllowStartArgEditing:              omit.From(gameModel.AllowStartArgEditing),
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

func modelCommandTypeToProtoCommandType(commandType string, command string, providerKind updateproviders.ProviderKind) xylona.CommandType {
	normalizedType := strings.TrimSpace(strings.ToLower(commandType))

	switch normalizedType {
	case "bash", "cmd", "powershell":
		return xylona.CommandType_COMMAND
	case "direct":
		if providerKind == updateproviders.ProviderKindSteamCMD && looksLikeSteamCMDCommand(command) {
			return xylona.CommandType_STEAMCMD
		}
		if strings.TrimSpace(command) == "" {
			return xylona.CommandType_NONE
		}
		return xylona.CommandType_COMMAND
	case "internal":
		switch providerKind {
		case updateproviders.ProviderKindSteamCMD:
			return xylona.CommandType_STEAMCMD
		case updateproviders.ProviderKindPaperMC:
			return xylona.CommandType_PAPERMC
		case updateproviders.ProviderKindMojang:
			return xylona.CommandType_MOJANG
		default:
			return xylona.CommandType_COMMAND
		}
	case "steamcmd":
		return xylona.CommandType_STEAMCMD
	case "papermc":
		return xylona.CommandType_PAPERMC
	case "mojang":
		return xylona.CommandType_MOJANG
	case "none":
		return xylona.CommandType_NONE
	default:
		if providerKind == updateproviders.ProviderKindSteamCMD && looksLikeSteamCMDCommand(command) {
			return xylona.CommandType_STEAMCMD
		}
		if strings.TrimSpace(command) == "" {
			return xylona.CommandType_NONE
		}
		return xylona.CommandType_COMMAND
	}
}

func protoCommandTypeToModelCommandType(commandType xylona.CommandType, processor xylona.CommandProcessor) string {
	switch commandType {
	case xylona.CommandType_NONE:
		return "direct"
	case xylona.CommandType_COMMAND:
		return commandProcessorToCommandType(processor)
	case xylona.CommandType_STEAMCMD:
		return "direct"
	case xylona.CommandType_PAPERMC, xylona.CommandType_MOJANG:
		return "internal"
	default:
		return commandProcessorToCommandType(processor)
	}
}

func commandValueForType(commandType xylona.CommandType, existingCommand string, steamAppID string) string {
	switch commandType {
	case xylona.CommandType_NONE:
		return ""
	case xylona.CommandType_STEAMCMD:
		generatedCommand := steamCMDCommand(steamAppID)
		if generatedCommand != "" {
			return generatedCommand
		}
		return strings.TrimSpace(existingCommand)
	default:
		return existingCommand
	}
}

func steamCMDCommand(steamAppID string) string {
	normalizedAppID := strings.TrimSpace(steamAppID)
	if normalizedAppID == "" {
		return ""
	}

	return "steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update " +
		normalizedAppID +
		" validate +quit"
}

func looksLikeSteamCMDCommand(command string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(command)), "steamcmd")
}

func hasSteamCommandType(gameProto *xylona.Game) bool {
	return gameProto.GetLinuxInstallType() == xylona.CommandType_STEAMCMD ||
		gameProto.GetLinuxUpdateType() == xylona.CommandType_STEAMCMD ||
		gameProto.GetWindowsInstallType() == xylona.CommandType_STEAMCMD ||
		gameProto.GetWindowsUpdateType() == xylona.CommandType_STEAMCMD
}

func normalizeGameConfigForCommandTypes(gameProto *xylona.Game, gameConfig updateproviders.GameConfig) updateproviders.GameConfig {
	if len(gameConfig.Variants) > 0 {
		return gameConfig
	}
	if strings.TrimSpace(gameConfig.DefaultTarget) != "" {
		return gameConfig
	}

	updateProviderKind := commandTypeToProviderKind(primaryUpdateCommandType(gameProto))
	updateProvider := updateproviders.ProviderConfig{
		Kind: updateProviderKind,
	}

	switch updateProviderKind {
	case updateproviders.ProviderKindSteamCMD:
		updateProvider.SourceID = strings.TrimSpace(gameProto.GetSteamAppid())
	case updateproviders.ProviderKindPaperMC:
		updateProvider.SourceID = defaultProviderSourceID(updateProviderKind, gameConfig.UpdateProvider.SourceID)
	case updateproviders.ProviderKindMojang:
		updateProvider.SourceID = defaultProviderSourceID(updateProviderKind, gameConfig.UpdateProvider.SourceID)
	}

	gameConfig.UpdateProvider = updateProvider
	gameConfig.DefaultTarget = ""
	return gameConfig
}

func primaryUpdateCommandType(gameProto *xylona.Game) xylona.CommandType {
	if gameProto.GetLinuxSupport() {
		linuxUpdateType := gameProto.GetLinuxUpdateType()
		if linuxUpdateType != xylona.CommandType_NONE {
			return linuxUpdateType
		}
	}

	if gameProto.GetWindowsSupport() {
		windowsUpdateType := gameProto.GetWindowsUpdateType()
		if windowsUpdateType != xylona.CommandType_NONE {
			return windowsUpdateType
		}
	}

	if gameProto.GetLinuxSupport() {
		return gameProto.GetLinuxInstallType()
	}
	if gameProto.GetWindowsSupport() {
		return gameProto.GetWindowsInstallType()
	}
	return xylona.CommandType_NONE
}

func commandTypeToProviderKind(commandType xylona.CommandType) updateproviders.ProviderKind {
	switch commandType {
	case xylona.CommandType_COMMAND:
		return updateproviders.ProviderKindCommand
	case xylona.CommandType_STEAMCMD:
		return updateproviders.ProviderKindSteamCMD
	case xylona.CommandType_PAPERMC:
		return updateproviders.ProviderKindPaperMC
	case xylona.CommandType_MOJANG:
		return updateproviders.ProviderKindMojang
	default:
		return updateproviders.ProviderKindNone
	}
}

func defaultProviderSourceID(kind updateproviders.ProviderKind, current string) string {
	trimmedCurrent := strings.TrimSpace(current)
	if trimmedCurrent != "" {
		return trimmedCurrent
	}

	switch kind {
	case updateproviders.ProviderKindPaperMC:
		return "paper"
	case updateproviders.ProviderKindMojang:
		return "vanilla"
	default:
		return ""
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
		Os:               strings.TrimSpace(nodeProto.Os),
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
		Os:               nodeModel.Os,
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
		Os:               omit.From(nodeModel.Os),
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
