package rpc

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/internal/mailer"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	// systemSMTPConfigKey retains the existing DB key so current manual SMTP
	// installations migrate without a data rewrite.
	systemSMTPConfigKey = "smtp_config"

	// GoogleMailOAuthCallbackPath is the controller route registered as the
	// authorized redirect URI in Google Cloud.
	GoogleMailOAuthCallbackPath = "/api/oauth/google/mail/callback"

	controllerSettingsPath   = "/admin/settings"
	googleMailOAuthStateLife = 10 * time.Minute
)

type googleMailOAuthState struct {
	userID       string
	clientID     string
	clientSecret string
	redirectURI  string
	verifier     string
	expiresAt    time.Time
}

type googleMailExchangeFunc func(
	ctx context.Context,
	clientID string,
	clientSecret string,
	redirectURI string,
	code string,
	verifier string,
) (*mailer.GoogleAuthorization, error)

type googleMailRevokeFunc func(ctx context.Context, refreshToken string) error

func systemEmailProvider(config *xylona.SystemSMTPConfig) xylona.SystemEmailProvider {
	if config == nil {
		return xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_UNSPECIFIED
	}

	provider := config.GetProvider()
	if provider != xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_UNSPECIFIED {
		return provider
	}

	// Configurations written before provider selection existed are manual SMTP.
	if strings.TrimSpace(config.GetHost()) != "" ||
		config.GetPort() != 0 ||
		strings.TrimSpace(config.GetUser()) != "" ||
		strings.TrimSpace(config.GetFromAddress()) != "" {
		return xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_SMTP
	}

	return xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_UNSPECIFIED
}

func systemSMTPConfigToMailer(config *xylona.SystemSMTPConfig) *mailer.SMTPConfig {
	provider := systemEmailProvider(config)
	method := mailer.DeliveryMethodSMTP
	if provider == xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_GOOGLE {
		method = mailer.DeliveryMethodGoogle
	}

	return &mailer.SMTPConfig{
		Method:     method,
		Host:       config.GetHost(),
		Port:       int(config.GetPort()),
		User:       config.GetUser(),
		Password:   config.GetPassword(),
		From:       config.GetFromAddress(),
		TLSEnabled: config.GetTlsEnabled(),
		Google: &mailer.GoogleOAuthConfig{
			ClientID:     config.GetGoogleClientId(),
			ClientSecret: config.GetGoogleClientSecret(),
			RefreshToken: config.GetGoogleRefreshToken(),
			Email:        config.GetGoogleEmail(),
		},
	}
}

func manualSMTPConfigUsable(config *xylona.SystemSMTPConfig) bool {
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

func googleMailConfigUsable(config *xylona.SystemSMTPConfig) bool {
	if config == nil {
		return false
	}

	return strings.TrimSpace(config.GetGoogleClientId()) != "" &&
		strings.TrimSpace(config.GetGoogleClientSecret()) != "" &&
		strings.TrimSpace(config.GetGoogleRefreshToken()) != "" &&
		strings.TrimSpace(config.GetGoogleEmail()) != ""
}

func systemSMTPConfigUsable(config *xylona.SystemSMTPConfig) bool {
	switch systemEmailProvider(config) {
	case xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_SMTP:
		return manualSMTPConfigUsable(config)
	case xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_GOOGLE:
		return googleMailConfigUsable(config)
	default:
		return false
	}
}

func (xs *XylonaService) readStoredSystemSMTPConfig() (*xylona.SystemSMTPConfig, bool, error) {
	jsonStr, errGet := xs.db.GetSystemConfig(systemSMTPConfigKey)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("rpc: load stored email config: %w", errGet)
	}

	config := &xylona.SystemSMTPConfig{}
	errUnmarshal := protojson.Unmarshal([]byte(jsonStr), config)
	if errUnmarshal != nil {
		return nil, false, fmt.Errorf("rpc: unmarshal stored email config: %w", errUnmarshal)
	}

	return config, true, nil
}

func (xs *XylonaService) writeStoredSystemSMTPConfig(config *xylona.SystemSMTPConfig) error {
	jsonBytes, errMarshal := protojson.Marshal(config)
	if errMarshal != nil {
		return fmt.Errorf("rpc: marshal email config: %w", errMarshal)
	}

	errSet := xs.db.SetSystemConfig(systemSMTPConfigKey, string(jsonBytes))
	if errSet != nil {
		return fmt.Errorf("rpc: save email config: %w", errSet)
	}
	return nil
}

// GetSystemSMTPConfig returns the stored controller email configuration for superusers.
func (xs *XylonaService) GetSystemSMTPConfig(
	_ context.Context,
	request *connect.Request[xylona.GetSystemSMTPConfigRequest],
) (*connect.Response[xylona.GetSystemSMTPConfigResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	config, stored, errGet := xs.readStoredSystemSMTPConfig()
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to get controller email config from DB")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !stored {
		return connect.NewResponse(&xylona.GetSystemSMTPConfigResponse{
			Configured: false,
		}), nil
	}

	config.Provider = systemEmailProvider(config)
	passwordConfigured := strings.TrimSpace(config.GetPassword()) != ""
	googleClientSecretConfigured := strings.TrimSpace(config.GetGoogleClientSecret()) != ""
	googleConnected := googleMailConfigUsable(config)
	configured := systemSMTPConfigUsable(config)

	config.Password = ""
	config.GoogleClientSecret = ""
	config.GoogleRefreshToken = ""

	return connect.NewResponse(&xylona.GetSystemSMTPConfigResponse{
		Config:                       config,
		Configured:                   configured,
		PasswordConfigured:           passwordConfigured,
		GoogleClientSecretConfigured: googleClientSecretConfigured,
		GoogleConnected:              googleConnected,
	}), nil
}

// SetSystemSMTPConfig stores and activates the controller's manual SMTP configuration.
func (xs *XylonaService) SetSystemSMTPConfig(
	_ context.Context,
	request *connect.Request[xylona.SetSystemSMTPConfigRequest],
) (*connect.Response[xylona.SetSystemSMTPConfigResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	config := request.Msg.GetConfig()
	if config == nil {
		return nil, invalidArg("config is required")
	}

	config.Host = strings.TrimSpace(config.GetHost())
	if config.GetHost() == "" {
		return nil, invalidArg("host is required")
	}
	port := config.GetPort()
	if port < 1 || port > 65535 {
		return nil, invalidArg("port must be between 1 and 65535")
	}
	config.User = strings.TrimSpace(config.GetUser())
	if config.GetUser() == "" {
		return nil, invalidArg("user is required")
	}
	config.FromAddress = strings.TrimSpace(config.GetFromAddress())
	if config.GetFromAddress() == "" {
		return nil, invalidArg("from_address is required")
	}
	_, errParseFrom := mail.ParseAddress(config.GetFromAddress())
	if errParseFrom != nil {
		return nil, invalidArg("from_address must be a valid email address")
	}

	existingConfig, stored, errExisting := xs.readStoredSystemSMTPConfig()
	if errExisting != nil {
		log.Error().Err(errExisting).Msg("Failed to read existing controller email config")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	password := strings.TrimSpace(config.GetPassword())
	if password == "" {
		if !stored || strings.TrimSpace(existingConfig.GetPassword()) == "" {
			return nil, invalidArg("password is required")
		}
		config.Password = existingConfig.GetPassword()
	}

	// Google credentials are write-only through the OAuth flow. Preserve any
	// connected account while the administrator switches to manual SMTP.
	if stored {
		config.GoogleClientId = existingConfig.GetGoogleClientId()
		config.GoogleClientSecret = existingConfig.GetGoogleClientSecret()
		config.GoogleRefreshToken = existingConfig.GetGoogleRefreshToken()
		config.GoogleEmail = existingConfig.GetGoogleEmail()
	} else {
		config.GoogleClientId = ""
		config.GoogleClientSecret = ""
		config.GoogleRefreshToken = ""
		config.GoogleEmail = ""
	}
	config.Provider = xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_SMTP

	errSet := xs.writeStoredSystemSMTPConfig(config)
	if errSet != nil {
		log.Error().Err(errSet).Msg("Failed to save manual SMTP config")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	return connect.NewResponse(&xylona.SetSystemSMTPConfigResponse{}), nil
}

// GetLocalSMTPStatus reports whether the controller email configuration is usable.
func (xs *XylonaService) GetLocalSMTPStatus(
	_ context.Context,
	request *connect.Request[xylona.GetLocalSMTPStatusRequest],
) (*connect.Response[xylona.GetLocalSMTPStatusResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	allowed, errPerm := xs.hasGlobalPermission(user)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("Failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, permissionDenied("insufficient permissions")
	}

	config, stored, errGet := xs.readStoredSystemSMTPConfig()
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to get controller email status")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !stored {
		return connect.NewResponse(&xylona.GetLocalSMTPStatusResponse{Configured: false}), nil
	}

	return connect.NewResponse(&xylona.GetLocalSMTPStatusResponse{
		Configured: systemSMTPConfigUsable(config),
	}), nil
}

// TestSystemSMTP sends a test email using the active controller email provider.
func (xs *XylonaService) TestSystemSMTP(
	ctx context.Context,
	request *connect.Request[xylona.TestSystemSMTPRequest],
) (*connect.Response[xylona.TestSystemSMTPResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	toAddress := strings.TrimSpace(request.Msg.GetToAddress())
	if toAddress == "" {
		return nil, invalidArg("to_address is required")
	}
	_, errParseTo := mail.ParseAddress(toAddress)
	if errParseTo != nil {
		return nil, invalidArg("to_address must be a valid email address")
	}

	protoConfig, stored, errGet := xs.readStoredSystemSMTPConfig()
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to get controller email config for test")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !stored || !systemSMTPConfigUsable(protoConfig) {
		return connect.NewResponse(&xylona.TestSystemSMTPResponse{
			Success: false,
			Error:   "Controller email delivery is not configured",
		}), nil
	}

	mailConfig := systemSMTPConfigToMailer(protoConfig)
	errSend := xs.resolvedSendTestEmailFunc()(ctx, mailConfig, toAddress)
	if errSend != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errSend)
	}

	return connect.NewResponse(&xylona.TestSystemSMTPResponse{
		Success: true,
	}), nil
}

// BeginGoogleMailOAuth starts a one-time, superuser-bound Google authorization flow.
func (xs *XylonaService) BeginGoogleMailOAuth(
	_ context.Context,
	request *connect.Request[xylona.BeginGoogleMailOAuthRequest],
) (*connect.Response[xylona.BeginGoogleMailOAuthResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	clientID := strings.TrimSpace(request.Msg.GetClientId())
	if clientID == "" {
		return nil, invalidArg("client_id is required")
	}

	clientSecret := strings.TrimSpace(request.Msg.GetClientSecret())
	if clientSecret == "" {
		existingConfig, stored, errExisting := xs.readStoredSystemSMTPConfig()
		if errExisting != nil {
			log.Error().Err(errExisting).Msg("Failed to read Google OAuth client config")
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		if stored && strings.TrimSpace(existingConfig.GetGoogleClientId()) == clientID {
			clientSecret = existingConfig.GetGoogleClientSecret()
		}
	}
	if strings.TrimSpace(clientSecret) == "" {
		return nil, invalidArg("client_secret is required")
	}

	redirectURI, errRedirect := validateGoogleMailRedirectURI(request.Msg.GetRedirectUri())
	if errRedirect != nil {
		return nil, invalidArg(errRedirect.Error())
	}

	state, errState := generateGoogleMailOAuthState()
	if errState != nil {
		log.Error().Err(errState).Msg("Failed to generate Google OAuth state")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	verifier := oauth2.GenerateVerifier()
	authorizationURL, errAuthorizationURL := mailer.GoogleAuthorizationURL(
		clientID,
		clientSecret,
		redirectURI,
		state,
		verifier,
	)
	if errAuthorizationURL != nil {
		return nil, invalidArg(errAuthorizationURL.Error())
	}

	xs.storeGoogleMailOAuthState(state, googleMailOAuthState{
		userID:       user.ID,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		verifier:     verifier,
		expiresAt:    time.Now().Add(googleMailOAuthStateLife),
	})

	return connect.NewResponse(&xylona.BeginGoogleMailOAuthResponse{
		AuthorizationUrl: authorizationURL,
	}), nil
}

// GoogleMailOAuthCallback completes the server-side Google OAuth flow and
// returns through a same-origin transition page without exposing the
// authorization code to the frontend application.
func (xs *XylonaService) GoogleMailOAuthCallback(response http.ResponseWriter, request *http.Request) {
	stateValue := strings.TrimSpace(request.URL.Query().Get("state"))
	state, validState := xs.consumeGoogleMailOAuthState(stateValue)
	if !validState {
		xs.renderGoogleMailResult(response, "invalid_state")
		return
	}

	user, errUser := xs.db.GetUserByID(state.userID)
	if errUser != nil || !user.SuperUser {
		xs.renderGoogleMailResult(response, "invalid_state")
		return
	}

	oauthError := strings.TrimSpace(request.URL.Query().Get("error"))
	if oauthError != "" {
		xs.renderGoogleMailResult(response, "denied")
		return
	}

	code := strings.TrimSpace(request.URL.Query().Get("code"))
	if code == "" {
		xs.renderGoogleMailResult(response, "error")
		return
	}

	authorization, errExchange := xs.resolvedGoogleMailExchangeFunc()(
		request.Context(),
		state.clientID,
		state.clientSecret,
		state.redirectURI,
		code,
		state.verifier,
	)
	if errExchange != nil {
		log.Warn().Err(errExchange).Str("user_id", user.ID).Msg("Google mail authorization failed")
		xs.renderGoogleMailResult(response, "error")
		return
	}
	if authorization == nil || strings.TrimSpace(authorization.RefreshToken) == "" || strings.TrimSpace(authorization.Email) == "" {
		log.Warn().Str("user_id", user.ID).Msg("Google mail authorization returned incomplete credentials")
		xs.renderGoogleMailResult(response, "error")
		return
	}

	config, stored, errGet := xs.readStoredSystemSMTPConfig()
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to load controller email config during Google callback")
		xs.renderGoogleMailResult(response, "error")
		return
	}
	if !stored {
		config = &xylona.SystemSMTPConfig{}
	}

	config.Provider = xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_GOOGLE
	config.GoogleClientId = state.clientID
	config.GoogleClientSecret = state.clientSecret
	config.GoogleRefreshToken = authorization.RefreshToken
	config.GoogleEmail = authorization.Email

	errSet := xs.writeStoredSystemSMTPConfig(config)
	if errSet != nil {
		log.Error().Err(errSet).Msg("Failed to save Google mail authorization")
		xs.renderGoogleMailResult(response, "error")
		return
	}

	xs.renderGoogleMailResult(response, "connected")
}

// DisconnectGoogleMail revokes and removes the connected Google refresh token.
func (xs *XylonaService) DisconnectGoogleMail(
	ctx context.Context,
	request *connect.Request[xylona.DisconnectGoogleMailRequest],
) (*connect.Response[xylona.DisconnectGoogleMailResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	config, stored, errGet := xs.readStoredSystemSMTPConfig()
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to read Google mail config for disconnect")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !stored || strings.TrimSpace(config.GetGoogleRefreshToken()) == "" {
		return connect.NewResponse(&xylona.DisconnectGoogleMailResponse{}), nil
	}

	errRevoke := xs.resolvedGoogleMailRevokeFunc()(ctx, config.GetGoogleRefreshToken())
	if errRevoke != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errRevoke)
	}

	config.GoogleRefreshToken = ""
	config.GoogleEmail = ""
	if systemEmailProvider(config) == xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_GOOGLE {
		if manualSMTPConfigUsable(config) {
			config.Provider = xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_SMTP
		} else {
			config.Provider = xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_UNSPECIFIED
		}
	}

	errSet := xs.writeStoredSystemSMTPConfig(config)
	if errSet != nil {
		log.Error().Err(errSet).Msg("Failed to clear Google mail authorization")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	return connect.NewResponse(&xylona.DisconnectGoogleMailResponse{}), nil
}

func validateGoogleMailRedirectURI(rawRedirectURI string) (string, error) {
	trimmedRedirectURI := strings.TrimSpace(rawRedirectURI)
	parsedURI, errParse := url.Parse(trimmedRedirectURI)
	if errParse != nil {
		return "", errors.New("redirect_uri must be a valid absolute URL")
	}
	if parsedURI.Scheme == "" || parsedURI.Host == "" || parsedURI.User != nil || parsedURI.Opaque != "" {
		return "", errors.New("redirect_uri must be a valid absolute URL")
	}
	if parsedURI.Path != GoogleMailOAuthCallbackPath || parsedURI.RawQuery != "" || parsedURI.Fragment != "" {
		return "", fmt.Errorf("redirect_uri must use the path %s without a query or fragment", GoogleMailOAuthCallbackPath)
	}

	switch strings.ToLower(parsedURI.Scheme) {
	case "https":
		return parsedURI.String(), nil
	case "http":
		hostname := strings.ToLower(parsedURI.Hostname())
		ip := net.ParseIP(hostname)
		if hostname == "localhost" || (ip != nil && ip.IsLoopback()) {
			return parsedURI.String(), nil
		}
		return "", errors.New("google OAuth requires HTTPS except on localhost")
	default:
		return "", errors.New("redirect_uri must use HTTPS")
	}
}

func generateGoogleMailOAuthState() (string, error) {
	stateBytes := make([]byte, 32)
	_, errRead := rand.Read(stateBytes)
	if errRead != nil {
		return "", fmt.Errorf("generate random OAuth state: %w", errRead)
	}
	return base64.RawURLEncoding.EncodeToString(stateBytes), nil
}

func (xs *XylonaService) storeGoogleMailOAuthState(stateValue string, state googleMailOAuthState) {
	xs.googleMailOAuthMu.Lock()
	defer xs.googleMailOAuthMu.Unlock()

	now := time.Now()
	for key, existingState := range xs.googleMailOAuthStates {
		if !existingState.expiresAt.After(now) {
			delete(xs.googleMailOAuthStates, key)
		}
	}
	if xs.googleMailOAuthStates == nil {
		xs.googleMailOAuthStates = make(map[string]googleMailOAuthState)
	}
	xs.googleMailOAuthStates[stateValue] = state
}

func (xs *XylonaService) consumeGoogleMailOAuthState(stateValue string) (googleMailOAuthState, bool) {
	xs.googleMailOAuthMu.Lock()
	defer xs.googleMailOAuthMu.Unlock()

	state, exists := xs.googleMailOAuthStates[stateValue]
	if exists {
		delete(xs.googleMailOAuthStates, stateValue)
	}
	if !exists || !state.expiresAt.After(time.Now()) {
		return googleMailOAuthState{}, false
	}
	return state, true
}

func (xs *XylonaService) resolvedGoogleMailExchangeFunc() googleMailExchangeFunc {
	if xs.googleMailExchangeFunc != nil {
		return xs.googleMailExchangeFunc
	}
	return mailer.ExchangeGoogleAuthorization
}

func (xs *XylonaService) resolvedGoogleMailRevokeFunc() googleMailRevokeFunc {
	if xs.googleMailRevokeFunc != nil {
		return xs.googleMailRevokeFunc
	}
	return mailer.RevokeGoogleAuthorization
}

func (xs *XylonaService) renderGoogleMailResult(response http.ResponseWriter, result string) {
	query := url.Values{}
	query.Set("google", result)
	target := html.EscapeString(controllerSettingsPath + "?" + query.Encode())

	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	response.Header().Set("Referrer-Policy", "no-referrer")
	_, errWrite := fmt.Fprintf(
		response,
		`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta http-equiv="refresh" content="0; url=%s"><title>Returning to Xylona</title></head><body><p>Returning to Xylona. <a href="%s">Continue to Controller Settings</a>.</p></body></html>`,
		target,
		target,
	)
	if errWrite != nil {
		log.Warn().Err(errWrite).Msg("Failed to write Google OAuth transition page")
	}
}
