package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// ListNodeApiKeys returns all node API keys with masked values.
// Requires superuser.
//
//revive:disable-next-line:var-naming // Matches the generated ConnectRPC method name from the proto schema.
func (xs *XylonaService) ListNodeApiKeys(
	_ context.Context,
	request *connect.Request[xylona.ListNodeApiKeysRequest],
) (*connect.Response[xylona.ListNodeApiKeysResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}

	keys, errGet := xs.db.GetNodeAPIKeys()
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to list node API keys")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list API keys"))
	}

	var protoKeys []*xylona.NodeApiKey
	for _, k := range keys {
		protoKeys = append(protoKeys, helpers.NodeAPIKeyModelToProto(k))
	}

	return &connect.Response[xylona.ListNodeApiKeysResponse]{
		Msg: &xylona.ListNodeApiKeysResponse{
			ApiKeys: protoKeys,
		},
	}, nil
}

// SetNodeApiKey creates or updates a node API key.
// Requires superuser.
//
//revive:disable-next-line:var-naming // Matches the generated ConnectRPC method name from the proto schema.
func (xs *XylonaService) SetNodeApiKey(
	_ context.Context,
	request *connect.Request[xylona.SetNodeApiKeyRequest],
) (*connect.Response[xylona.SetNodeApiKeyResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}

	serviceName := strings.TrimSpace(request.Msg.GetServiceName())
	apiKey := strings.TrimSpace(request.Msg.GetApiKey())

	if serviceName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("service_name is required"))
	}
	if apiKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("api_key is required"))
	}

	now := time.Now().UTC()
	setter := &models.NodeAPIKeySetter{
		ID:          omit.From(uuid.NewString()),
		ServiceName: omit.From(serviceName),
		APIKey:      omit.From(apiKey),
		CreatedAt:   omit.From(now),
		UpdatedAt:   omit.From(now),
	}

	key, errUpsert := xs.db.InsertOrUpdateNodeAPIKey(xs.db.DB, setter)
	if errUpsert != nil {
		log.Error().Err(errUpsert).Msg("Failed to upsert node API key")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save API key"))
	}

	return &connect.Response[xylona.SetNodeApiKeyResponse]{
		Msg: &xylona.SetNodeApiKeyResponse{
			ApiKey: helpers.NodeAPIKeyModelToProto(key),
		},
	}, nil
}

// DeleteNodeApiKey deletes a node API key by service name.
// Requires superuser.
//
//revive:disable-next-line:var-naming // Matches the generated ConnectRPC method name from the proto schema.
func (xs *XylonaService) DeleteNodeApiKey(
	_ context.Context,
	request *connect.Request[xylona.DeleteNodeApiKeyRequest],
) (*connect.Response[xylona.DeleteNodeApiKeyResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}

	serviceName := strings.TrimSpace(request.Msg.GetServiceName())
	if serviceName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("service_name is required"))
	}

	errDelete := xs.db.DeleteNodeAPIKeyByServiceName(serviceName)
	if errDelete != nil {
		log.Error().Err(errDelete).Msg("Failed to delete node API key")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete API key"))
	}

	return &connect.Response[xylona.DeleteNodeApiKeyResponse]{
		Msg: &xylona.DeleteNodeApiKeyResponse{},
	}, nil
}
