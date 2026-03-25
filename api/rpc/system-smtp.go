package rpc

import (
	"context"
	"database/sql"
	"errors"
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

// maskedPasswordPlaceholder is the value returned by GetSystemSMTPConfig in
// place of the real password. If SetSystemSMTPConfig receives this value, it
// preserves the previously stored password rather than overwriting it.
const maskedPasswordPlaceholder = "********"

func (xs *XylonaService) GetSystemSMTPConfig(
	ctx context.Context,
	request *connect.Request[xylona.GetSystemSMTPConfigRequest],
) (*connect.Response[xylona.GetSystemSMTPConfigResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser access required"))
	}

	jsonStr, errGet := xs.db.GetSystemConfig(systemSMTPConfigKey)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return connect.NewResponse(&xylona.GetSystemSMTPConfigResponse{
				Configured: false,
			}), nil
		}
		log.Error().Err(errGet).Msg("failed to get SMTP config from DB")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	config := &xylona.SystemSMTPConfig{}
	errUnmarshal := protojson.Unmarshal([]byte(jsonStr), config)
	if errUnmarshal != nil {
		log.Error().Err(errUnmarshal).Msg("failed to unmarshal SMTP config")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if config.GetPassword() != "" {
		config.Password = "********"
	}

	return connect.NewResponse(&xylona.GetSystemSMTPConfigResponse{
		Config:     config,
		Configured: true,
	}), nil
}

func (xs *XylonaService) SetSystemSMTPConfig(
	ctx context.Context,
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
	fromAddress := strings.TrimSpace(config.GetFromAddress())
	if fromAddress == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("from_address is required"))
	}
	_, errParseFrom := mail.ParseAddress(fromAddress)
	if errParseFrom != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("from_address must be a valid email address"))
	}

	// If the password is the masked placeholder, read the existing stored
	// config and preserve the original password so a save-without-edit does
	// not overwrite the real credential.
	if config.GetPassword() == maskedPasswordPlaceholder {
		existingJSON, errExisting := xs.db.GetSystemConfig(systemSMTPConfigKey)
		if errExisting != nil && !errors.Is(errExisting, sql.ErrNoRows) {
			log.Error().Err(errExisting).Msg("failed to read existing SMTP config for password preservation")
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		if errExisting == nil {
			existing := &xylona.SystemSMTPConfig{}
			errUnmarshal := protojson.Unmarshal([]byte(existingJSON), existing)
			if errUnmarshal == nil {
				config.Password = existing.GetPassword()
			}
		}
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

	// Read the stored SMTP config from DB.
	jsonStr, errGet := xs.db.GetSystemConfig(systemSMTPConfigKey)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return connect.NewResponse(&xylona.TestSystemSMTPResponse{
				Success: false,
				Error:   "SMTP is not configured",
			}), nil
		}
		log.Error().Err(errGet).Msg("failed to get SMTP config for test")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	protoConfig := &xylona.SystemSMTPConfig{}
	errUnmarshal := protojson.Unmarshal([]byte(jsonStr), protoConfig)
	if errUnmarshal != nil {
		log.Error().Err(errUnmarshal).Msg("failed to unmarshal SMTP config for test")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	smtpCfg := &mailer.SMTPConfig{
		Host:       protoConfig.GetHost(),
		Port:       int(protoConfig.GetPort()),
		User:       protoConfig.GetUser(),
		Password:   protoConfig.GetPassword(),
		From:       protoConfig.GetFromAddress(),
		TLSEnabled: protoConfig.GetTlsEnabled(),
	}

	// Use injected send function for testing, or the real one in production.
	sendFn := mailer.SendTestEmail
	if xs.testEmailSendFunc != nil {
		sendFn = func(ctx context.Context, cfg *mailer.SMTPConfig, to string) error {
			return xs.testEmailSendFunc(ctx, cfg, to, "Xylona SMTP Test",
				"This is a test email from Xylona to verify your SMTP configuration.")
		}
	}

	errSend := sendFn(ctx, smtpCfg, toAddress)
	if errSend != nil {
		return connect.NewResponse(&xylona.TestSystemSMTPResponse{
			Success: false,
			Error:   errSend.Error(),
		}), nil
	}

	return connect.NewResponse(&xylona.TestSystemSMTPResponse{
		Success: true,
	}), nil
}
