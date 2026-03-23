package rpc

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// systemSMTPConfigKey is the DB key used to store the serialized SMTP config.
const systemSMTPConfigKey = "smtp_config"

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

	return connect.NewResponse(&xylona.TestSystemSMTPResponse{
		Success: false,
		Error:   "SMTP delivery not yet implemented",
	}), nil
}
