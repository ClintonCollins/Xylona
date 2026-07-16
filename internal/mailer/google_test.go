package mailer

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestGoogleAuthorizationURL(t *testing.T) {
	t.Parallel()

	authorizationURL, errBuild := GoogleAuthorizationURL(
		"client-id",
		"client-secret",
		"https://xylona.example.com/api/oauth/google/mail/callback",
		"state-value",
		"pkce-verifier",
	)
	if errBuild != nil {
		t.Fatalf("GoogleAuthorizationURL() error = %v", errBuild)
	}

	parsedURL, errParse := url.Parse(authorizationURL)
	if errParse != nil {
		t.Fatalf("url.Parse() error = %v", errParse)
	}
	query := parsedURL.Query()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "state", key: "state", value: "state-value"},
		{name: "offline access", key: "access_type", value: "offline"},
		{name: "consent and account selection", key: "prompt", value: "consent select_account"},
		{name: "PKCE method", key: "code_challenge_method", value: "S256"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if query.Get(testCase.key) != testCase.value {
				t.Errorf("query %s = %q, want %q", testCase.key, query.Get(testCase.key), testCase.value)
			}
		})
	}

	if query.Get("code_challenge") == "" {
		t.Error("code_challenge is empty")
	}
	if query.Get("client_secret") != "" {
		t.Error("authorization URL exposed the OAuth client secret")
	}
	if !slices.Contains(strings.Fields(query.Get("scope")), GoogleMailSendScope) {
		t.Errorf("scope = %q, want to include %q", query.Get("scope"), GoogleMailSendScope)
	}
	if strings.Contains(query.Get("scope"), "https://mail.google.com/") {
		t.Errorf("scope = %q, must not request full mailbox access", query.Get("scope"))
	}
}

func TestExchangeGoogleAuthorization(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
			errParseForm := request.ParseForm()
			if errParseForm != nil {
				t.Errorf("ParseForm() error = %v", errParseForm)
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			if request.Form.Get("code") != "authorization-code" {
				t.Errorf("code = %q, want authorization-code", request.Form.Get("code"))
			}
			if request.Form.Get("code_verifier") != "pkce-verifier" {
				t.Errorf("code_verifier = %q, want pkce-verifier", request.Form.Get("code_verifier"))
			}
			response.Header().Set("Content-Type", "application/json")
			_, errWrite := response.Write([]byte(`{"access_token":"access-token","refresh_token":"refresh-token","token_type":"Bearer","expires_in":3600,"scope":"openid email https://www.googleapis.com/auth/gmail.send"}`))
			if errWrite != nil {
				t.Errorf("Write() error = %v", errWrite)
			}
		case "/userinfo":
			if request.Header.Get("Authorization") != "Bearer access-token" {
				t.Errorf("Authorization = %q, want Bearer access-token", request.Header.Get("Authorization"))
			}
			response.Header().Set("Content-Type", "application/json")
			_, errWrite := response.Write([]byte(`{"email":"admin@example.com","email_verified":true}`))
			if errWrite != nil {
				t.Errorf("Write() error = %v", errWrite)
			}
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	endpoint := oauth2.Endpoint{
		AuthURL:   server.URL + "/authorize",
		TokenURL:  server.URL + "/token",
		AuthStyle: oauth2.AuthStyleInParams,
	}
	authorization, errExchange := exchangeGoogleAuthorizationWithEndpoints(
		t.Context(),
		"client-id",
		"client-secret",
		"https://xylona.example.com/api/oauth/google/mail/callback",
		"authorization-code",
		"pkce-verifier",
		endpoint,
		server.URL+"/userinfo",
	)
	if errExchange != nil {
		t.Fatalf("exchangeGoogleAuthorization() error = %v", errExchange)
	}
	if authorization.RefreshToken != "refresh-token" {
		t.Errorf("RefreshToken = %q, want refresh-token", authorization.RefreshToken)
	}
	if authorization.Email != "admin@example.com" {
		t.Errorf("Email = %q, want admin@example.com", authorization.Email)
	}
}

func TestSendGoogleAPIWithEndpoints(t *testing.T) {
	t.Parallel()

	var sentMessage string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
			errParseForm := request.ParseForm()
			if errParseForm != nil {
				t.Errorf("ParseForm() error = %v", errParseForm)
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			if request.Form.Get("grant_type") != "refresh_token" {
				t.Errorf("grant_type = %q, want refresh_token", request.Form.Get("grant_type"))
			}
			if request.Form.Get("refresh_token") != "refresh-token" {
				t.Errorf("refresh_token = %q, want refresh-token", request.Form.Get("refresh_token"))
			}
			response.Header().Set("Content-Type", "application/json")
			_, errWrite := response.Write([]byte(`{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`))
			if errWrite != nil {
				t.Errorf("Write() error = %v", errWrite)
			}
		case "/send":
			if request.Header.Get("Authorization") != "Bearer access-token" {
				t.Errorf("Authorization = %q, want Bearer access-token", request.Header.Get("Authorization"))
			}
			body, errRead := io.ReadAll(request.Body)
			if errRead != nil {
				t.Errorf("ReadAll() error = %v", errRead)
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			var payload map[string]string
			errDecode := json.Unmarshal(body, &payload)
			if errDecode != nil {
				t.Errorf("json.Unmarshal() error = %v", errDecode)
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			decodedMessage, errBase64 := base64.RawURLEncoding.DecodeString(payload["raw"])
			if errBase64 != nil {
				t.Errorf("DecodeString() error = %v", errBase64)
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			sentMessage = string(decodedMessage)
			response.Header().Set("Content-Type", "application/json")
			_, errWrite := response.Write([]byte(`{"id":"message-id"}`))
			if errWrite != nil {
				t.Errorf("Write() error = %v", errWrite)
			}
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := &SMTPConfig{
		Method: DeliveryMethodGoogle,
		Google: &GoogleOAuthConfig{
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			RefreshToken: "refresh-token",
			Email:        "sender@example.com",
		},
	}
	endpoint := oauth2.Endpoint{
		AuthURL:   server.URL + "/authorize",
		TokenURL:  server.URL + "/token",
		AuthStyle: oauth2.AuthStyleInParams,
	}
	errSend := sendGoogleAPIWithEndpoints(
		t.Context(),
		config,
		"recipient@example.com",
		"Controller alert",
		"Alert body",
		endpoint,
		server.URL+"/send",
	)
	if errSend != nil {
		t.Fatalf("sendGoogleAPIWithEndpoints() error = %v", errSend)
	}

	for _, expected := range []string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Subject: Controller alert",
		"Alert body",
	} {
		if !strings.Contains(sentMessage, expected) {
			t.Errorf("sent message missing %q:\n%s", expected, sentMessage)
		}
	}
}

func TestRevokeGoogleAuthorization(t *testing.T) {
	t.Parallel()

	var revokedToken string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
		errParseForm := request.ParseForm()
		if errParseForm != nil {
			t.Errorf("ParseForm() error = %v", errParseForm)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		revokedToken = request.Form.Get("token")
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	errRevoke := revokeGoogleAuthorizationWithEndpoint(t.Context(), "refresh-token", server.URL, server.Client())
	if errRevoke != nil {
		t.Fatalf("revokeGoogleAuthorization() error = %v", errRevoke)
	}
	if revokedToken != "refresh-token" {
		t.Errorf("revoked token = %q, want refresh-token", revokedToken)
	}
}
