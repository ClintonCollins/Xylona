package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/pkg/webhooks"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// permissionAlertsManage is the RBAC permission key for managing alert rules
// and notification channels.
const permissionAlertsManage = "alerts.manage"

// permissionAlertsViewHistory is the RBAC permission key for read-only access
// to alert rules, history, and notification channel names.
const permissionAlertsViewHistory = "alerts.view_history"

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

// hasAnyGlobalPermission returns true if the user is a superuser or holds at
// least one of the specified permissions via globally-scoped role assignments.
func (xs *XylonaService) hasAnyGlobalPermission(user *models.User, permissionIDs ...string) (bool, error) {
	if user.SuperUser {
		return true, nil
	}
	return xs.db.UserHasAnyGlobalPermission(user.ID, permissionIDs)
}

// notificationChannelToProto converts a DB model notification channel to its
// protobuf representation.
func notificationChannelToProto(ch *models.NotificationChannel, includeSensitiveConfig bool) *xylona.NotificationChannel {
	channelTypeInt, ok := xylona.NotificationChannelType_value[ch.ChannelType]
	channelType := xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_UNSPECIFIED
	if ok {
		channelType = xylona.NotificationChannelType(channelTypeInt)
	}

	config := ch.Config
	if !includeSensitiveConfig {
		config = maskNotificationChannelConfig(ch.Config)
	}

	return &xylona.NotificationChannel{
		Id:          ch.ID,
		UserId:      ch.UserID,
		Name:        ch.Name,
		ChannelType: channelType,
		Config:      config,
		Enabled:     ch.Enabled != 0,
		CreatedAt:   timestamppb.New(ch.CreatedAt),
		UpdatedAt:   timestamppb.New(ch.UpdatedAt),
	}
}

func maskNotificationChannelConfig(config string) string {
	trimmed := strings.TrimSpace(config)
	if trimmed == "" {
		return config
	}

	var parsed any
	errUnmarshal := json.Unmarshal([]byte(config), &parsed)
	if errUnmarshal != nil {
		return `"********"`
	}

	masked := maskSensitiveConfigValue(parsed)
	maskedJSON, errMarshal := json.Marshal(masked)
	if errMarshal != nil {
		return `"********"`
	}

	return string(maskedJSON)
}

func maskSensitiveConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		masked := make(map[string]any, len(typed))
		for key, nested := range typed {
			if isSensitiveConfigKey(key) {
				masked[key] = "********"
				continue
			}
			masked[key] = maskSensitiveConfigValue(nested)
		}
		return masked
	case []any:
		masked := make([]any, len(typed))
		for idx, nested := range typed {
			masked[idx] = maskSensitiveConfigValue(nested)
		}
		return masked
	default:
		return value
	}
}

func isSensitiveConfigKey(key string) bool {
	lowered := strings.ToLower(strings.TrimSpace(key))
	sensitiveKeys := []string{"url", "webhook", "token", "secret", "password", "api_key", "apikey", "authorization"}
	for _, sensitiveKey := range sensitiveKeys {
		if lowered == sensitiveKey || strings.Contains(lowered, sensitiveKey) {
			return true
		}
	}
	return false
}

type notificationChannelEmailConfig struct {
	To       string `json:"to"`
	SMTPFrom string `json:"smtp_from"`
}

func validateNotificationChannelConfig(channelType string, rawConfig string) error {
	switch channelType {
	case xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD.String(),
		xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_SLACK.String(),
		xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_GENERIC.String():
		var config webhooks.ChannelConfig
		errUnmarshal := json.Unmarshal([]byte(rawConfig), &config)
		if errUnmarshal != nil {
			return errors.New("config must be valid JSON")
		}
		errValidate := webhooks.ValidateChannelConfig(config)
		if errValidate != nil {
			return errValidate
		}
		return webhooks.ValidateWebhookTarget(strings.TrimSpace(config.URL))
	case xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL.String():
		var config notificationChannelEmailConfig
		errUnmarshal := json.Unmarshal([]byte(rawConfig), &config)
		if errUnmarshal != nil {
			return errors.New("config must be valid JSON")
		}
		toAddress := strings.TrimSpace(config.To)
		if toAddress == "" {
			return errors.New("email notification channels require a recipient address")
		}
		_, errParseTo := mail.ParseAddress(toAddress)
		if errParseTo != nil {
			return errors.New("email notification channels require a valid recipient address")
		}
		smtpFrom := strings.TrimSpace(config.SMTPFrom)
		if smtpFrom != "" {
			_, errParseFrom := mail.ParseAddress(smtpFrom)
			if errParseFrom != nil {
				return errors.New("email notification channels require a valid smtp_from address")
			}
		}
		return nil
	default:
		return nil
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
	errValidate := validateNotificationChannelConfig(channelType.String(), request.Msg.GetConfig())
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
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
		Channel: notificationChannelToProto(channel, true),
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

	existingChannel, errGetExisting := xs.db.GetNotificationChannelByID(id)
	if errGetExisting != nil {
		if errors.Is(errGetExisting, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("notification channel not found"))
		}
		log.Error().Err(errGetExisting).Str("notification_channel_id", id).Msg("failed to fetch notification channel before update")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update notification channel"))
	}
	if existingChannel.UserID != user.ID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("notification channel not found"))
	}

	errValidate := validateNotificationChannelConfig(existingChannel.ChannelType, request.Msg.GetConfig())
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
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
		Channel: notificationChannelToProto(channel, true),
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

	allowed, errPerm := xs.hasAnyGlobalPermission(user, permissionAlertsManage, permissionAlertsViewHistory)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alert permissions")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	includeSensitiveConfig, errManagePerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errManagePerm != nil {
		log.Error().Err(errManagePerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission for config masking")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	channels, errGet := xs.db.GetNotificationChannelsByUserID(user.ID)
	if errGet != nil {
		log.Error().Err(errGet).Str("user_id", user.ID).Msg("failed to list notification channels")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list notification channels"))
	}

	resp := &xylona.ListNotificationChannelsResponse{}
	for _, ch := range channels {
		resp.Channels = append(resp.Channels, notificationChannelToProto(ch, includeSensitiveConfig))
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
