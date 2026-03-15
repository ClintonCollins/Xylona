package rpc

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

func performHandshake(ctx context.Context, baseURL string, secretKey string) (*xylona.FederationHandshakeResponse, error) {
	httpClient := &http.Client{
		Timeout: 15 * time.Second,
	}
	client := xylonaconnect.NewFederationClient(httpClient, baseURL)
	handshakeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.Handshake(handshakeCtx, connect.NewRequest(&xylona.FederationHandshakeRequest{
		SecretKey: secretKey,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}
