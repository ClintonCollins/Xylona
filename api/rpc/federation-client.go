package rpc

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

func performHandshake(
	ctx context.Context,
	federationMTLS *helpers.FederationMTLS,
	baseURL string,
	expectedPeerFederationPort int,
	expectedPeerFingerprint string,
	expectedPeerNodeID string,
) (*xylona.FederationHandshakeResponse, error) {
	if federationMTLS == nil {
		return nil, errors.New("federation mTLS is not configured")
	}

	httpClient, federationBaseURL, errClient := federationMTLS.NewNodeHTTPClientWithPort(
		15*time.Second,
		baseURL,
		expectedPeerFederationPort,
		expectedPeerFingerprint,
		expectedPeerNodeID,
	)
	if errClient != nil {
		return nil, errClient
	}

	client := xylonaconnect.NewFederationClient(httpClient, federationBaseURL)
	handshakeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.Handshake(handshakeCtx, connect.NewRequest(&xylona.FederationHandshakeRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}
