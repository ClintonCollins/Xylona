package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/alerts"
	"github.com/ClintonCollins/Xylona/internal/mailer"
	"github.com/ClintonCollins/Xylona/internal/webhooks"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// permissionAlertsManage is the RBAC permission key for managing alert rules
// and notification channels.
const permissionAlertsManage = "alerts.manage"

// permissionAlertsViewHistory is the RBAC permission key for read-only access
// to alert rules, history, and notification channel names.
const permissionAlertsViewHistory = "alerts.view_history"

// hasGlobalPermission returns true if the user is a superuser or holds
// alerts.manage via a globally-scoped role assignment (game_server_id IS NULL).
func (xs *XylonaService) hasGlobalPermission(user *models.User) (bool, error) {
	if user.SuperUser {
		return true, nil
	}
	hasPerm, errCheck := xs.db.UserHasGlobalPermission(user.ID, permissionAlertsManage)
	if errCheck != nil {
		return false, fmt.Errorf("rpc: check global alerts.manage permission: %w", errCheck)
	}
	return hasPerm, nil
}

// hasAnyGlobalPermission returns true if the user is a superuser or holds at
// least one of the specified permissions via globally-scoped role assignments.
func (xs *XylonaService) hasAnyGlobalPermission(user *models.User, permissionIDs ...string) (bool, error) {
	if user.SuperUser {
		return true, nil
	}
	hasPerm, errCheck := xs.db.UserHasAnyGlobalPermission(user.ID, permissionIDs)
	if errCheck != nil {
		return false, fmt.Errorf("rpc: check global alert permissions: %w", errCheck)
	}
	return hasPerm, nil
}

// notificationChannelToProto converts a DB model notification channel to its
// protobuf representation.
func notificationChannelToProto(ch *models.NotificationChannel, includeSensitiveConfig bool) *xylona.NotificationChannel {
	channelTypeInt, ok := xylona.NotificationChannelType_value[ch.ChannelType]
	channelType := xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_UNSPECIFIED
	if ok {
		channelType = xylona.NotificationChannelType(channelTypeInt)
	}

	config := sanitizeNotificationChannelConfig(ch.ChannelType, ch.Config, includeSensitiveConfig)

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

func sanitizeNotificationChannelConfig(channelType string, config string, includeSensitiveConfig bool) string {
	if channelType == xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL.String() {
		emailConfig, errParse := alerts.ParseEmailChannelConfig(config)
		if errParse != nil {
			return maskNotificationChannelConfig(config)
		}

		sanitizedJSON, errMarshal := emailConfig.Sanitized().Marshal()
		if errMarshal != nil {
			return maskNotificationChannelConfig(config)
		}

		if includeSensitiveConfig {
			return sanitizedJSON
		}

		return maskNotificationChannelConfig(sanitizedJSON)
	}

	if includeSensitiveConfig {
		return config
	}

	return maskNotificationChannelConfig(config)
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

func normalizeNotificationChannelConfig(
	channelType string,
	rawConfig string,
	existingChannel *models.NotificationChannel,
) (string, error) {
	switch channelType {
	case xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD.String(),
		xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_SLACK.String(),
		xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_GENERIC.String():
		var config webhooks.ChannelConfig
		errUnmarshal := json.Unmarshal([]byte(rawConfig), &config)
		if errUnmarshal != nil {
			return "", errors.New("config must be valid JSON")
		}
		errValidate := webhooks.ValidateChannelConfig(config)
		if errValidate != nil {
			return "", fmt.Errorf("rpc: validate webhook channel config: %w", errValidate)
		}
		errTarget := webhooks.ValidateWebhookTarget(strings.TrimSpace(config.URL))
		if errTarget != nil {
			return "", fmt.Errorf("rpc: validate webhook target: %w", errTarget)
		}
		return rawConfig, nil
	case xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL.String():
		emailConfig, errParse := alerts.ParseEmailChannelConfig(rawConfig)
		if errParse != nil {
			return "", fmt.Errorf("rpc: parse email notification channel config: %w", errParse)
		}

		var existingEmailConfig alerts.EmailChannelConfig
		if existingChannel != nil && existingChannel.ChannelType == channelType {
			existingEmailConfig, errParse = alerts.ParseEmailChannelConfig(existingChannel.Config)
			if errParse != nil {
				return "", fmt.Errorf("rpc: parse existing email notification channel config: %w", errParse)
			}
		}

		if emailConfig.SMTPSource == alerts.SMTPSourceNode {
			emailConfig.SMTPHost = ""
			emailConfig.SMTPPort = 0
			emailConfig.SMTPUser = ""
			emailConfig.SMTPPassword = ""
			emailConfig.SMTPFrom = ""
			emailConfig.SMTPTLSEnabled = false
		}

		requirePassword := emailConfig.SMTPSource == alerts.SMTPSourceCustom
		if existingChannel != nil &&
			emailConfig.SMTPSource == alerts.SMTPSourceCustom &&
			emailConfig.SMTPPassword == "" &&
			existingEmailConfig.SMTPPassword != "" {
			emailConfig.SMTPPassword = existingEmailConfig.SMTPPassword
			requirePassword = false
		}

		errValidate := emailConfig.Validate(requirePassword)
		if errValidate != nil {
			return "", fmt.Errorf("rpc: validate email notification channel config: %w", errValidate)
		}

		emailConfig.SMTPPasswordConfigured = false
		normalizedJSON, errMarshal := emailConfig.Marshal()
		if errMarshal != nil {
			return "", errors.New("config must be valid JSON")
		}
		return normalizedJSON, nil
	default:
		return rawConfig, nil
	}
}

// CreateNotificationChannel creates a new alert notification channel.
func (xs *XylonaService) CreateNotificationChannel(
	_ context.Context,
	request *connect.Request[xylona.CreateNotificationChannelRequest],
) (*connect.Response[xylona.CreateNotificationChannelResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	allowed, errPerm := xs.hasGlobalPermission(user)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, permissionDenied("insufficient permissions")
	}

	name := strings.TrimSpace(request.Msg.GetName())
	if name == "" {
		return nil, invalidArg("name is required")
	}

	channelType := request.Msg.GetChannelType()
	if channelType == xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_UNSPECIFIED {
		return nil, invalidArg("channel_type is required")
	}
	normalizedConfig, errValidate := normalizeNotificationChannelConfig(channelType.String(), request.Msg.GetConfig(), nil)
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}

	channel, errInsert := xs.db.InsertNotificationChannel(
		user.ID,
		name,
		channelType.String(),
		normalizedConfig,
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

// UpdateNotificationChannel updates an existing alert notification channel.
func (xs *XylonaService) UpdateNotificationChannel(
	_ context.Context,
	request *connect.Request[xylona.UpdateNotificationChannelRequest],
) (*connect.Response[xylona.UpdateNotificationChannelResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	allowed, errPerm := xs.hasGlobalPermission(user)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, permissionDenied("insufficient permissions")
	}

	id := strings.TrimSpace(request.Msg.GetId())
	if id == "" {
		return nil, invalidArg("id is required")
	}

	name := strings.TrimSpace(request.Msg.GetName())
	if name == "" {
		return nil, invalidArg("name is required")
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

	normalizedConfig, errValidate := normalizeNotificationChannelConfig(existingChannel.ChannelType, request.Msg.GetConfig(), existingChannel)
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}

	errUpdate := xs.db.UpdateNotificationChannel(
		id,
		user.ID,
		name,
		normalizedConfig,
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

// DeleteNotificationChannel removes an alert notification channel.
func (xs *XylonaService) DeleteNotificationChannel(
	_ context.Context,
	request *connect.Request[xylona.DeleteNotificationChannelRequest],
) (*connect.Response[xylona.DeleteNotificationChannelResponse], error) {
	errDelete := xs.deleteOwnedAlertResource(request.Header(), request.Msg.GetId(), alertResourceDeleteConfig{
		logIDField:       "notification_channel_id",
		deleteLogMessage: "failed to delete notification channel",
		internalMessage:  "failed to delete notification channel",
		deleteFn:         xs.db.DeleteNotificationChannel,
	})
	if errDelete != nil {
		return nil, errDelete
	}

	return connect.NewResponse(&xylona.DeleteNotificationChannelResponse{}), nil
}

// ListNotificationChannels lists the caller's alert notification channels.
func (xs *XylonaService) ListNotificationChannels(
	_ context.Context,
	request *connect.Request[xylona.ListNotificationChannelsRequest],
) (*connect.Response[xylona.ListNotificationChannelsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	allowed, errPerm := xs.hasAnyGlobalPermission(user, permissionAlertsManage, permissionAlertsViewHistory)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alert permissions")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, permissionDenied("insufficient permissions")
	}

	includeSensitiveConfig, errManagePerm := xs.hasGlobalPermission(user)
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

// TestNotificationChannel sends a test message through a notification channel.
func (xs *XylonaService) TestNotificationChannel(
	ctx context.Context,
	request *connect.Request[xylona.TestNotificationChannelRequest],
) (*connect.Response[xylona.TestNotificationChannelResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	allowed, errPerm := xs.hasGlobalPermission(user)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, permissionDenied("insufficient permissions")
	}

	id := strings.TrimSpace(request.Msg.GetId())
	if id == "" {
		return nil, invalidArg("id is required")
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

	if channel.ChannelType != xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL.String() {
		return connect.NewResponse(&xylona.TestNotificationChannelResponse{
			Success: false,
			Error:   "test delivery not yet implemented",
		}), nil
	}

	limiterKey := user.ID + ":" + channel.ID
	allowedByRateLimit := xs.getNotificationChannelTestLimiter().Allow(limiterKey)
	if !allowedByRateLimit {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("test notification channel rate limit exceeded"))
	}

	emailConfig, errParse := alerts.ParseEmailChannelConfig(channel.Config)
	if errParse != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("rpc: parse stored email notification channel config: %w", errParse))
	}

	var smtpCfg *mailer.SMTPConfig
	if emailConfig.SMTPSource == alerts.SMTPSourceNode {
		systemConfig, configured, errSystem := xs.readStoredSystemSMTPConfig()
		if errSystem != nil {
			log.Error().Err(errSystem).Msg("failed to resolve local SMTP config for notification channel test")
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		if !configured || !systemSMTPConfigUsable(systemConfig) {
			return connect.NewResponse(&xylona.TestNotificationChannelResponse{
				Success: false,
				Error:   "SMTP is not configured",
			}), nil
		}
		smtpCfg = systemSMTPConfigToMailer(systemConfig)
	} else {
		smtpCfg = emailConfig.EffectiveSMTPConfig()
	}

	errSend := xs.resolvedSendTestEmailFunc()(ctx, smtpCfg, emailConfig.To)
	if errSend != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errSend)
	}

	return connect.NewResponse(&xylona.TestNotificationChannelResponse{
		Success: true,
	}), nil
}
