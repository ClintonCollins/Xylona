package rpc

import (
	"context"
	"errors"
	"net"
	"strings"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/admininterface"
	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetGameServerAdminInterface returns safe endpoint and password-state metadata.
func (xs *XylonaService) GetGameServerAdminInterface(
	_ context.Context,
	request *connect.Request[xylona.GetGameServerAdminInterfaceRequest],
) (*connect.Response[xylona.GetGameServerAdminInterfaceResponse], error) {
	_, gameServer, errAccess := xs.getEnvironmentGameServer(request.Header(), request.Msg.GetServerId())
	if errAccess != nil {
		return nil, errAccess
	}

	adminInterface, errView := xs.gameServerAdminInterfaceView(gameServer)
	if errView != nil {
		return nil, errView
	}
	return connect.NewResponse(&xylona.GetGameServerAdminInterfaceResponse{
		AdminInterface: adminInterface,
	}), nil
}

// SetGameServerAdminInterfacePassword replaces the encrypted write-only
// password used by the supported game's administration interface.
func (xs *XylonaService) SetGameServerAdminInterfacePassword(
	_ context.Context,
	request *connect.Request[xylona.SetGameServerAdminInterfacePasswordRequest],
) (*connect.Response[xylona.SetGameServerAdminInterfacePasswordResponse], error) {
	user, gameServer, errAccess := xs.getEnvironmentGameServer(request.Header(), request.Msg.GetServerId())
	if errAccess != nil {
		return nil, errAccess
	}
	profile, supported := effectiveGameServerAdminInterfaceProfile(gameServer)
	if !supported {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errAdminInterfaceUnsupported)
	}

	password := request.Msg.GetPassword()
	errPassword := admininterface.ValidatePassword(gameServer.GameID, password)
	if errPassword != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errPassword)
	}
	errHistory := xs.retainSatisfactoryAdminPasswordHistory(gameServer, profile, password, user.ID)
	if errHistory != nil {
		return nil, internalErrf("failed to retain admin interface password history")
	}
	errSet := xs.db.SetGameServerSecret(
		gameServer.ID,
		profile.SecretKind,
		profile.SecretName,
		password,
		user.ID,
	)
	if errSet != nil {
		return nil, internalErrf("failed to save admin interface password")
	}

	adminInterface, errView := xs.gameServerAdminInterfaceView(gameServer)
	if errView != nil {
		return nil, errView
	}
	return connect.NewResponse(&xylona.SetGameServerAdminInterfacePasswordResponse{
		AdminInterface: adminInterface,
	}), nil
}

func (xs *XylonaService) gameServerAdminInterfaceView(
	gameServer *models.GameServer,
) (*xylona.GameServerAdminInterface, error) {
	profile, supported := effectiveGameServerAdminInterfaceProfile(gameServer)
	if !supported {
		return &xylona.GameServerAdminInterface{}, nil
	}
	configured, errConfigured := xs.db.HasGameServerSecret(
		gameServer.ID,
		profile.SecretKind,
		profile.SecretName,
	)
	if errConfigured != nil {
		return nil, internalErr()
	}

	bindAddress := "0.0.0.0"
	if profile.BindToGameServerIP {
		bindAddress = strings.TrimSpace(gameServer.IP)
		if bindAddress == "" {
			bindAddress = "0.0.0.0"
		}
	}
	remoteAccess := configured && !isLoopbackAdminInterfaceAddress(bindAddress)
	return &xylona.GameServerAdminInterface{
		Supported:             true,
		Transport:             profile.Transport,
		BindAddress:           bindAddress,
		Port:                  profile.Port,
		Username:              profile.Username,
		PasswordConfigured:    configured,
		RemoteAccess:          remoteAccess,
		RemoteAccessNote:      profile.RemoteAccessNote,
		TransportSecurityNote: profile.TransportSecurityNote,
	}, nil
}

func effectiveGameServerAdminInterfaceProfile(gameServer *models.GameServer) (admininterface.Profile, bool) {
	if gameServer == nil {
		return admininterface.Profile{}, false
	}
	profile, supported := admininterface.Lookup(gameServer.GameID, gameServer.Port, gameServer.QueryPort)
	if !supported {
		return admininterface.Profile{}, false
	}
	if gameServer.GameID != "palworld" && !actions.GameServerDefinitionSupportsAdminInput(gameServer) {
		return admininterface.Profile{}, false
	}
	return profile, true
}

func (xs *XylonaService) retainSatisfactoryAdminPasswordHistory(
	gameServer *models.GameServer,
	profile admininterface.Profile,
	newPassword string,
	userID string,
) error {
	if gameServer.GameID != "satisfactory" {
		return nil
	}
	currentPassword, configured, errCurrent := xs.db.DecryptGameServerSecret(
		gameServer.ID,
		profile.SecretKind,
		profile.SecretName,
	)
	if errCurrent != nil {
		return errors.Join(errors.New("load current Satisfactory admin password"), errCurrent)
	}
	if !configured || currentPassword == "" || currentPassword == newPassword {
		return nil
	}
	rawHistory, historyConfigured, errHistory := xs.db.DecryptGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindAdminInterface,
		db.GameServerSecretNameAdminInterfacePasswordHistory,
	)
	if errHistory != nil {
		return errors.Join(errors.New("load Satisfactory admin password history"), errHistory)
	}
	if !historyConfigured {
		rawHistory = ""
	}
	updatedHistory, errAppend := admininterface.AppendPasswordHistory(rawHistory, currentPassword)
	if errAppend != nil {
		return errors.Join(errors.New("append Satisfactory admin password history"), errAppend)
	}
	errSet := xs.db.SetGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindAdminInterface,
		db.GameServerSecretNameAdminInterfacePasswordHistory,
		updatedHistory,
		userID,
	)
	if errSet != nil {
		return errors.Join(errors.New("store Satisfactory admin password history"), errSet)
	}
	return nil
}

func isLoopbackAdminInterfaceAddress(address string) bool {
	if strings.EqualFold(strings.TrimSpace(address), "localhost") {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(address))
	return ip != nil && ip.IsLoopback()
}

var errAdminInterfaceUnsupported = errors.New("this game does not have a supported password-protected admin interface")
