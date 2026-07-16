package mailer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	// GoogleMailSendScope grants only permission to send mail through the
	// Gmail API. It intentionally avoids the full-mail scope required by
	// Gmail's SMTP XOAUTH2 mechanism.
	GoogleMailSendScope = "https://www.googleapis.com/auth/gmail.send"

	googleAuthorizationEndpoint   = "https://accounts.google.com/o/oauth2/v2/auth"
	googleOAuthExchangeEndpoint   = "https://oauth2.googleapis.com/token"
	googleUserInfoEndpoint        = "https://openidconnect.googleapis.com/v1/userinfo"
	googleMailSendEndpoint        = "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"
	googleOAuthRevocationEndpoint = "https://oauth2.googleapis.com/revoke"
	googleRequestTimeout          = 15 * time.Second
	googleErrorBodyLimit          = 4096
)

// GoogleAuthorization contains the durable data returned after a Google OAuth
// flow. Access tokens are deliberately not persisted; they are refreshed only
// when mail needs to be sent.
type GoogleAuthorization struct {
	RefreshToken string
	Email        string
}

type googleUserInfo struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

type googleAPIError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func googleOAuthConfig(clientID string, clientSecret string, redirectURI string, endpoint oauth2.Endpoint) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     endpoint,
		Scopes: []string{
			"openid",
			"email",
			GoogleMailSendScope,
		},
	}
}

// GoogleAuthorizationURL builds the Google consent URL for controller mail.
// The caller must retain state and verifier until the callback is completed.
func GoogleAuthorizationURL(clientID string, clientSecret string, redirectURI string, state string, verifier string) (string, error) {
	if strings.TrimSpace(clientID) == "" {
		return "", errors.New("mailer: Google OAuth client ID is required")
	}
	if strings.TrimSpace(clientSecret) == "" {
		return "", errors.New("mailer: Google OAuth client secret is required")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return "", errors.New("mailer: Google OAuth redirect URI is required")
	}
	if strings.TrimSpace(state) == "" {
		return "", errors.New("mailer: Google OAuth state is required")
	}
	if strings.TrimSpace(verifier) == "" {
		return "", errors.New("mailer: Google OAuth PKCE verifier is required")
	}

	endpoint := oauth2.Endpoint{
		AuthURL:   googleAuthorizationEndpoint,
		TokenURL:  googleOAuthExchangeEndpoint,
		AuthStyle: oauth2.AuthStyleInParams,
	}
	config := googleOAuthConfig(clientID, clientSecret, redirectURI, endpoint)
	return config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent select_account"),
		oauth2.S256ChallengeOption(verifier),
	), nil
}

// ExchangeGoogleAuthorization exchanges an authorization code and resolves the
// connected Google account's verified email address.
func ExchangeGoogleAuthorization(
	ctx context.Context,
	clientID string,
	clientSecret string,
	redirectURI string,
	code string,
	verifier string,
) (*GoogleAuthorization, error) {
	endpoint := oauth2.Endpoint{
		AuthURL:   googleAuthorizationEndpoint,
		TokenURL:  googleOAuthExchangeEndpoint,
		AuthStyle: oauth2.AuthStyleInParams,
	}
	return exchangeGoogleAuthorizationWithEndpoints(
		ctx,
		clientID,
		clientSecret,
		redirectURI,
		code,
		verifier,
		endpoint,
		googleUserInfoEndpoint,
	)
}

func exchangeGoogleAuthorizationWithEndpoints(
	ctx context.Context,
	clientID string,
	clientSecret string,
	redirectURI string,
	code string,
	verifier string,
	endpoint oauth2.Endpoint,
	userInfoEndpoint string,
) (*GoogleAuthorization, error) {
	config := googleOAuthConfig(clientID, clientSecret, redirectURI, endpoint)
	token, errExchange := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if errExchange != nil {
		return nil, fmt.Errorf("mailer: exchange Google authorization code: %w", errExchange)
	}

	refreshToken := strings.TrimSpace(token.RefreshToken)
	if refreshToken == "" {
		return nil, errors.New("mailer: Google did not return an offline refresh token; reconnect and approve access")
	}

	grantedScopes, _ := token.Extra("scope").(string)
	if grantedScopes != "" && !slices.Contains(strings.Fields(grantedScopes), GoogleMailSendScope) {
		return nil, errors.New("mailer: Google mail send permission was not granted")
	}

	httpClient := config.Client(ctx, token)
	httpClient.Timeout = googleRequestTimeout
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, userInfoEndpoint, nil)
	if errRequest != nil {
		return nil, fmt.Errorf("mailer: create Google user info request: %w", errRequest)
	}
	request.Header.Set("Accept", "application/json")

	response, errDo := httpClient.Do(request)
	if errDo != nil {
		return nil, fmt.Errorf("mailer: get Google account information: %w", errDo)
	}
	responseBody, errBody := readGoogleResponseBody(response)
	if errBody != nil {
		return nil, errBody
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, googleHTTPError("get Google account information", response.StatusCode, responseBody)
	}

	var userInfo googleUserInfo
	errDecode := json.Unmarshal(responseBody, &userInfo)
	if errDecode != nil {
		return nil, fmt.Errorf("mailer: decode Google account information: %w", errDecode)
	}
	userInfo.Email = strings.TrimSpace(userInfo.Email)
	if userInfo.Email == "" || !userInfo.EmailVerified {
		return nil, errors.New("mailer: Google account did not provide a verified email address")
	}

	return &GoogleAuthorization{
		RefreshToken: refreshToken,
		Email:        userInfo.Email,
	}, nil
}

// RevokeGoogleAuthorization revokes a stored Google refresh token before the
// controller removes it from local storage.
func RevokeGoogleAuthorization(ctx context.Context, refreshToken string) error {
	return revokeGoogleAuthorizationWithEndpoint(ctx, refreshToken, googleOAuthRevocationEndpoint, &http.Client{
		Timeout: googleRequestTimeout,
	})
}

func revokeGoogleAuthorizationWithEndpoint(ctx context.Context, refreshToken string, endpoint string, httpClient *http.Client) error {
	trimmedToken := strings.TrimSpace(refreshToken)
	if trimmedToken == "" {
		return nil
	}

	form := url.Values{}
	form.Set("token", trimmedToken)
	request, errRequest := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		strings.NewReader(form.Encode()),
	)
	if errRequest != nil {
		return fmt.Errorf("mailer: create Google token revocation request: %w", errRequest)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, errDo := httpClient.Do(request)
	if errDo != nil {
		return fmt.Errorf("mailer: revoke Google authorization: %w", errDo)
	}
	responseBody, errBody := readGoogleResponseBody(response)
	if errBody != nil {
		return errBody
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return googleHTTPError("revoke Google authorization", response.StatusCode, responseBody)
	}

	return nil
}

func sendGoogleAPI(ctx context.Context, config *SMTPConfig, to string, subject string, body string) error {
	googleConfig := config.Google
	if googleConfig == nil {
		return errors.New("mailer: Google mail is not configured")
	}
	if strings.TrimSpace(googleConfig.ClientID) == "" ||
		strings.TrimSpace(googleConfig.ClientSecret) == "" ||
		strings.TrimSpace(googleConfig.RefreshToken) == "" ||
		strings.TrimSpace(googleConfig.Email) == "" {
		return errors.New("mailer: Google mail credentials are incomplete")
	}

	endpoint := oauth2.Endpoint{
		AuthURL:   googleAuthorizationEndpoint,
		TokenURL:  googleOAuthExchangeEndpoint,
		AuthStyle: oauth2.AuthStyleInParams,
	}
	return sendGoogleAPIWithEndpoints(ctx, config, to, subject, body, endpoint, googleMailSendEndpoint)
}

func sendGoogleAPIWithEndpoints(
	ctx context.Context,
	config *SMTPConfig,
	to string,
	subject string,
	body string,
	oauthEndpoint oauth2.Endpoint,
	mailEndpoint string,
) error {
	googleConfig := config.Google
	fromAddress, errFrom := mail.ParseAddress(strings.TrimSpace(googleConfig.Email))
	if errFrom != nil {
		return fmt.Errorf("mailer: invalid connected Google email address: %w", errFrom)
	}
	toAddress, errTo := mail.ParseAddress(strings.TrimSpace(to))
	if errTo != nil {
		return fmt.Errorf("mailer: invalid recipient address: %w", errTo)
	}

	message := buildMessage(fromAddress.Address, toAddress.Address, subject, body)
	payload, errMarshal := json.Marshal(map[string]string{
		"raw": base64.RawURLEncoding.EncodeToString([]byte(message)),
	})
	if errMarshal != nil {
		return fmt.Errorf("mailer: encode Gmail API request: %w", errMarshal)
	}

	oauthConfig := googleOAuthConfig(
		googleConfig.ClientID,
		googleConfig.ClientSecret,
		"",
		oauthEndpoint,
	)
	token := &oauth2.Token{RefreshToken: googleConfig.RefreshToken}
	httpClient := oauthConfig.Client(ctx, token)
	httpClient.Timeout = googleRequestTimeout

	request, errRequest := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		mailEndpoint,
		bytes.NewReader(payload),
	)
	if errRequest != nil {
		return fmt.Errorf("mailer: create Gmail API request: %w", errRequest)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")

	response, errDo := httpClient.Do(request)
	if errDo != nil {
		return fmt.Errorf("mailer: send through Gmail API: %w", errDo)
	}
	responseBody, errBody := readGoogleResponseBody(response)
	if errBody != nil {
		return errBody
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return googleHTTPError("send through Gmail API", response.StatusCode, responseBody)
	}

	return nil
}

func readGoogleResponseBody(response *http.Response) ([]byte, error) {
	responseBody, errRead := io.ReadAll(io.LimitReader(response.Body, googleErrorBodyLimit))
	errClose := response.Body.Close()
	if errRead != nil {
		return nil, fmt.Errorf("mailer: read Google response: %w", errRead)
	}
	if errClose != nil {
		return nil, fmt.Errorf("mailer: close Google response: %w", errClose)
	}
	return responseBody, nil
}

func googleHTTPError(action string, statusCode int, responseBody []byte) error {
	var apiError googleAPIError
	errDecode := json.Unmarshal(responseBody, &apiError)
	message := strings.TrimSpace(apiError.Error.Message)
	if errDecode == nil && message != "" {
		return fmt.Errorf("mailer: %s failed with HTTP %d: %s", action, statusCode, message)
	}
	return fmt.Errorf("mailer: %s failed with HTTP %d", action, statusCode)
}
