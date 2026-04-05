package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/pkg/mailer"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// systemSMTPConfigKey is the DB key used to store the serialized SMTP config.
const systemSMTPConfigKey = "smtp_config"

func systemSMTPConfigToMailer(config *xylona.SystemSMTPConfig) *mailer.SMTPConfig {
	return &mailer.SMTPConfig{
		Host:       config.GetHost(),
		Port:       int(config.GetPort()),
		User:       config.GetUser(),
		Password:   config.GetPassword(),
		From:       config.GetFromAddress(),
		TLSEnabled: config.GetTlsEnabled(),
	}
}

func systemSMTPConfigUsable(config *xylona.SystemSMTPConfig) bool {
	if config == nil {
		return false
	}

	return strings.TrimSpace(config.GetHost()) != "" &&
		config.GetPort() >= 1 &&
		config.GetPort() <= 65535 &&
		strings.TrimSpace(config.GetUser()) != "" &&
		strings.TrimSpace(config.GetPassword()) != "" &&
		strings.TrimSpace(config.GetFromAddress()) != ""
}

func (xs *XylonaService) readStoredSystemSMTPConfig() (*xylona.SystemSMTPConfig, bool, error) {
	jsonStr, errGet := xs.db.GetSystemConfig(systemSMTPConfigKey)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("rpc: load stored SMTP config: %w", errGet)
	}

	config := &xylona.SystemSMTPConfig{}
	errUnmarshal := protojson.Unmarshal([]byte(jsonStr), config)
	if errUnmarshal != nil {
		return nil, false, fmt.Errorf("rpc: unmarshal stored SMTP config: %w", errUnmarshal)
	}

	return config, true, nil
}

// GetSystemSMTPConfig returns the stored system SMTP configuration for superusers.
func (xs *XylonaService) GetSystemSMTPConfig(
	_ context.Context,
	request *connect.Request[xylona.GetSystemSMTPConfigRequest],
) (*connect.Response[xylona.GetSystemSMTPConfigResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser access required"))
	}

	config, configured, errGet := xs.readStoredSystemSMTPConfig()
	if errGet != nil {
		log.Error().Err(errGet).Msg("failed to get SMTP config from DB")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !configured {
		return connect.NewResponse(&xylona.GetSystemSMTPConfigResponse{
			Configured: false,
		}), nil
	}

	passwordConfigured := strings.TrimSpace(config.GetPassword()) != ""
	config.Password = ""

	return connect.NewResponse(&xylona.GetSystemSMTPConfigResponse{
		Config:             config,
		Configured:         true,
		PasswordConfigured: passwordConfigured,
	}), nil
}

// SetSystemSMTPConfig stores the system SMTP configuration for superusers.
func (xs *XylonaService) SetSystemSMTPConfig(
	_ context.Context,
	request *connect.Request[xylona.SetSystemSMTPConfigRequest],
) (*connect.Response[xylona.SetSystemSMTPConfigResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser access required"))
	}

	config := request.Msg.GetConfig()
	if config == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("config is required"))
	}

	host := strings.TrimSpace(config.GetHost())
	if host == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host is required"))
	}
	port := config.GetPort()
	if port < 1 || port > 65535 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("port must be between 1 and 65535"))
	}
	userName := strings.TrimSpace(config.GetUser())
	if userName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user is required"))
	}
	fromAddress := strings.TrimSpace(config.GetFromAddress())
	if fromAddress == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("from_address is required"))
	}
	_, errParseFrom := mail.ParseAddress(fromAddress)
	if errParseFrom != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("from_address must be a valid email address"))
	}

	password := strings.TrimSpace(config.GetPassword())
	if password == "" {
		existingConfig, configured, errExisting := xs.readStoredSystemSMTPConfig()
		if errExisting != nil {
			log.Error().Err(errExisting).Msg("failed to read existing SMTP config for password preservation")
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		if !configured || strings.TrimSpace(existingConfig.GetPassword()) == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("password is required"))
		}
		config.Password = existingConfig.GetPassword()
	}

	jsonBytes, errMarshal := protojson.Marshal(config)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("failed to marshal SMTP config")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	errSet := xs.db.SetSystemConfig(systemSMTPConfigKey, string(jsonBytes))
	if errSet != nil {
		log.Error().Err(errSet).Msg("failed to save SMTP config")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	return connect.NewResponse(&xylona.SetSystemSMTPConfigResponse{}), nil
}

// GetLocalSMTPStatus reports whether the node SMTP configuration is usable.
func (xs *XylonaService) GetLocalSMTPStatus(
	_ context.Context,
	request *connect.Request[xylona.GetLocalSMTPStatusRequest],
) (*connect.Response[xylona.GetLocalSMTPStatusResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	config, configured, errGet := xs.readStoredSystemSMTPConfig()
	if errGet != nil {
		log.Error().Err(errGet).Msg("failed to get local SMTP status")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !configured {
		return connect.NewResponse(&xylona.GetLocalSMTPStatusResponse{Configured: false}), nil
	}

	return connect.NewResponse(&xylona.GetLocalSMTPStatusResponse{
		Configured: systemSMTPConfigUsable(config),
	}), nil
}

// TestSystemSMTP sends a test email using the stored system SMTP configuration.
func (xs *XylonaService) TestSystemSMTP(
	ctx context.Context,
	request *connect.Request[xylona.TestSystemSMTPRequest],
) (*connect.Response[xylona.TestSystemSMTPResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser access required"))
	}

	toAddress := strings.TrimSpace(request.Msg.GetToAddress())
	if toAddress == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("to_address is required"))
	}
	_, errParseTo := mail.ParseAddress(toAddress)
	if errParseTo != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("to_address must be a valid email address"))
	}

	protoConfig, configured, errGet := xs.readStoredSystemSMTPConfig()
	if errGet != nil {
		log.Error().Err(errGet).Msg("failed to get SMTP config for test")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !configured || !systemSMTPConfigUsable(protoConfig) {
		return connect.NewResponse(&xylona.TestSystemSMTPResponse{
			Success: false,
			Error:   "SMTP is not configured",
		}), nil
	}

	smtpCfg := systemSMTPConfigToMailer(protoConfig)

	errSend := xs.resolvedSendTestEmailFunc()(ctx, smtpCfg, toAddress)
	if errSend != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errSend)
	}

	return connect.NewResponse(&xylona.TestSystemSMTPResponse{
		Success: true,
	}), nil
}
