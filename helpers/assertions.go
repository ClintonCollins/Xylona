package helpers

import (
	"github.com/aarondl/opt/omit"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func GameServerModelStatusToProtoStatus(status string) xylona.Status {
	switch status {
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
	}
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
		LinuxUpdateCommand:                gameModel.LinuxUpdateCommand,
		LinuxWorkingDirectory:             gameModel.LinuxWorkingDirectory,
		WindowsStartCommand:               gameModel.WindowsStartCommand,
		WindowsStopCommand:                gameModel.WindowsStopCommand,
		WindowsInstallCommand:             gameModel.WindowsInstallCommand,
		WindowsUpdateCommand:              gameModel.WindowsUpdateCommand,
		WindowsWorkingDirectory:           gameModel.WindowsWorkingDirectory,
		RequireDedicatedIp:                gameModel.RequireDedicatedIP,
		BindsToAllIps:                     gameModel.BindsToAllIps,
		CreatedAt:                         timestamppb.New(gameModel.CreatedAt),
		UpdatedAt:                         timestamppb.New(gameModel.UpdatedAt),
		UsesSourceQuery:                   gameModel.UsesSourceQuery,
		RequiresSteamGameServerLoginToken: gameModel.RequiresSteamGameServerLoginToken,
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
		LinuxUpdateCommand:                gameProto.LinuxUpdateCommand,
		LinuxWorkingDirectory:             gameProto.LinuxWorkingDirectory,
		WindowsStartCommand:               gameProto.WindowsStartCommand,
		WindowsStopCommand:                gameProto.WindowsStopCommand,
		WindowsInstallCommand:             gameProto.WindowsInstallCommand,
		WindowsUpdateCommand:              gameProto.WindowsUpdateCommand,
		WindowsWorkingDirectory:           gameProto.WindowsWorkingDirectory,
		RequireDedicatedIP:                gameProto.RequireDedicatedIp,
		BindsToAllIps:                     gameProto.BindsToAllIps,
		CreatedAt:                         gameProto.CreatedAt.AsTime(),
		UpdatedAt:                         gameProto.UpdatedAt.AsTime(),
		UsesSourceQuery:                   gameProto.UsesSourceQuery,
		RequiresSteamGameServerLoginToken: gameProto.RequiresSteamGameServerLoginToken,
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
