package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

type e2eClient struct {
	rpc     xylonaconnect.XylonaClient
	baseURL string
	userID  string
}

func newE2EClient(baseURL string) *e2eClient {
	jar, errJar := cookiejar.New(nil)
	if errJar != nil {
		panic(fmt.Sprintf("create cookie jar: %v", errJar))
	}
	httpClient := &http.Client{
		Jar: jar,
	}
	return &e2eClient{
		rpc:     xylonaconnect.NewXylonaClient(httpClient, baseURL),
		baseURL: baseURL,
	}
}

func newAuthenticatedClient(ctx context.Context, baseURL, username, password string) (*e2eClient, error) {
	client := newE2EClient(baseURL)
	resp, errLogin := client.rpc.Login(ctx, connect.NewRequest(&xylona.LoginRequest{
		UserName: username,
		Password: password,
	}))
	if errLogin != nil {
		return nil, fmt.Errorf("login as %s at %s: %w", username, baseURL, errLogin)
	}
	if resp.Msg.GetUser() != nil {
		client.userID = resp.Msg.GetUser().GetId()
	}
	return client, nil
}
