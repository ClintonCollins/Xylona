package rpc

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// permissionAlertsManage is the RBAC permission key for managing alert rules
// and notification channels.
const permissionAlertsManage = "alerts.manage"

// hasGlobalPermission returns true if the user is a superuser or holds the
// specified permission via a globally-scoped role assignment (game_server_id IS NULL).
func (xs *XylonaService) hasGlobalPermission(user *models.User, permissionID string) (bool, error) {
	if user.SuperUser {
		return true, nil
	}
	hasPerm, errCheck := xs.db.UserHasGlobalPermission(user.ID, permissionID)
	if errCheck != nil {
		return false, errCheck
	}
	return hasPerm, nil
}

// notificationChannelToProto converts a DB model notification channel to its
// protobuf representation.
func notificationChannelToProto(ch *models.NotificationChannel) *xylona.NotificationChannel {
	channelTypeInt, ok := xylona.NotificationChannelType_value[ch.ChannelType]
	channelType := xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_UNSPECIFIED
	if ok {
		channelType = xylona.NotificationChannelType(channelTypeInt)
	}

	return &xylona.NotificationChannel{
		Id:          ch.ID,
		UserId:      ch.UserID,
		Name:        ch.Name,
		ChannelType: channelType,
		Config:      ch.Config,
		Enabled:     ch.Enabled != 0,
		CreatedAt:   timestamppb.New(ch.CreatedAt),
		UpdatedAt:   timestamppb.New(ch.UpdatedAt),
	}
}

func (xs *XylonaService) CreateNotificationChannel(
	ctx context.Context,
	request *connect.Request[xylona.CreateNotificationChannelRequest],
) (*connect.Response[xylona.CreateNotificationChannelResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	name := strings.TrimSpace(request.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	channelType := request.Msg.GetChannelType()
	if channelType == xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("channel_type is required"))
	}

	channel, errInsert := xs.db.InsertNotificationChannel(
		user.ID,
		name,
		channelType.String(),
		request.Msg.GetConfig(),
		request.Msg.GetEnabled(),
	)
	if errInsert != nil {
		log.Error().Err(errInsert).Str("user_id", user.ID).Msg("failed to create notification channel")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create notification channel"))
	}

	return connect.NewResponse(&xylona.CreateNotificationChannelResponse{
		Channel: notificationChannelToProto(channel),
	}), nil
}

func (xs *XylonaService) UpdateNotificationChannel(
	ctx context.Context,
	request *connect.Request[xylona.UpdateNotificationChannelRequest],
) (*connect.Response[xylona.UpdateNotificationChannelResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	id := strings.TrimSpace(request.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	name := strings.TrimSpace(request.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	errUpdate := xs.db.UpdateNotificationChannel(
		id,
		user.ID,
		name,
		request.Msg.GetConfig(),
		request.Msg.GetEnabled(),
	)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("notification_channel_id", id).Msg("failed to update notification channel")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update notification channel"))
	}

	channel, errGet := xs.db.GetNotificationChannelByID(id)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("notification channel not found"))
		}
		log.Error().Err(errGet).Str("notification_channel_id", id).Msg("failed to fetch updated notification channel")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch updated notification channel"))
	}
	// Verify the re-fetched channel belongs to the caller. The UPDATE was
	// scoped by user_id, so if the channel belongs to another user the
	// update was a no-op but GetNotificationChannelByID ignores ownership.
	if channel.UserID != user.ID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("notification channel not found"))
	}

	return connect.NewResponse(&xylona.UpdateNotificationChannelResponse{
		Channel: notificationChannelToProto(channel),
	}), nil
}

func (xs *XylonaService) DeleteNotificationChannel(
	ctx context.Context,
	request *connect.Request[xylona.DeleteNotificationChannelRequest],
) (*connect.Response[xylona.DeleteNotificationChannelResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	id := strings.TrimSpace(request.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	errDelete := xs.db.DeleteNotificationChannel(id, user.ID)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("notification_channel_id", id).Msg("failed to delete notification channel")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete notification channel"))
	}

	return connect.NewResponse(&xylona.DeleteNotificationChannelResponse{}), nil
}

func (xs *XylonaService) ListNotificationChannels(
	ctx context.Context,
	request *connect.Request[xylona.ListNotificationChannelsRequest],
) (*connect.Response[xylona.ListNotificationChannelsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	channels, errGet := xs.db.GetNotificationChannelsByUserID(user.ID)
	if errGet != nil {
		log.Error().Err(errGet).Str("user_id", user.ID).Msg("failed to list notification channels")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list notification channels"))
	}

	resp := &xylona.ListNotificationChannelsResponse{}
	for _, ch := range channels {
		resp.Channels = append(resp.Channels, notificationChannelToProto(ch))
	}

	return connect.NewResponse(resp), nil
}

func (xs *XylonaService) TestNotificationChannel(
	ctx context.Context,
	request *connect.Request[xylona.TestNotificationChannelRequest],
) (*connect.Response[xylona.TestNotificationChannelResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	id := strings.TrimSpace(request.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	channel, errGet := xs.db.GetNotificationChannelByID(id)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("notification channel not found"))
		}
		log.Error().Err(errGet).Str("notification_channel_id", id).Msg("failed to fetch notification channel for test")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if channel.UserID != user.ID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("notification channel not found"))
	}

	// Test delivery is not yet implemented. Return a stub response.
	return connect.NewResponse(&xylona.TestNotificationChannelResponse{
		Success: false,
		Error:   "test delivery not yet implemented",
	}), nil
}
