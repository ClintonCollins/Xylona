package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestValidateConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*Configuration)
		wantErrPart string
	}{
		{
			name: `missing required secrets`,
			mutate: func(config *Configuration) {
				config.CookieHashKey = ``
				config.CookieBlockKey = ``
				config.JWTSecretKey = ``
			},
			wantErrPart: `missing required configuration: COOKIE_HASH_KEY_BASE64, COOKIE_BLOCK_KEY_BASE64`,
		},
		{
			name: `malformed cookie hash key`,
			mutate: func(config *Configuration) {
				config.CookieHashKey = `%%%`
			},
			wantErrPart: `decode COOKIE_HASH_KEY_BASE64`,
		},
		{
			name: `malformed jwt key`,
			mutate: func(config *Configuration) {
				config.JWTSecretKey = `%%%`
			},
			wantErrPart: `decode JWT_SECRET_KEY_BASE64`,
		},
		{
			name: `cookie block key wrong length`,
			mutate: func(config *Configuration) {
				config.CookieBlockKey = encodeSecretForTest(10)
			},
			wantErrPart: `COOKIE_BLOCK_KEY_BASE64 must decode to 16, 24, or 32 bytes, got 10`,
		},
		{
			name: `invalid timeout`,
			mutate: func(config *Configuration) {
				config.HTTPReadTimeout = 0
			},
			wantErrPart: `HTTP_READ_TIMEOUT must be greater than zero`,
		},
		{
			name: `missing encryption key`,
			mutate: func(config *Configuration) {
				config.EncryptionKey = ``
			},
			wantErrPart: `missing required configuration: ENCRYPTION_KEY_BASE64`,
		},
		{
			name: `malformed encryption key`,
			mutate: func(config *Configuration) {
				config.EncryptionKey = `%%%`
			},
			wantErrPart: `decode ENCRYPTION_KEY_BASE64`,
		},
		{
			name: `encryption key too short`,
			mutate: func(config *Configuration) {
				config.EncryptionKey = encodeSecretForTest(16)
			},
			wantErrPart: `ENCRYPTION_KEY_BASE64 must decode to at least 32 bytes, got 16`,
		},
		{
			name: `invalid trusted proxies`,
			mutate: func(config *Configuration) {
				config.TrustedProxies = `not-an-ip`
			},
			wantErrPart: `TRUSTED_PROXIES`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validConfigurationForTest()
			tt.mutate(&config)

			_, errValidate := validateConfiguration(config)
			if errValidate == nil {
				t.Fatal(`validateConfiguration() error = nil, want error`)
			}
			if !strings.Contains(errValidate.Error(), tt.wantErrPart) {
				t.Fatalf(`validateConfiguration() error = %q, want substring %q`, errValidate.Error(), tt.wantErrPart)
			}
		})
	}
}

func TestValidateConfigurationAllowsEmptyJWTSecret(t *testing.T) {
	config := validConfigurationForTest()
	config.JWTSecretKey = ``

	_, errValidate := validateConfiguration(config)
	if errValidate != nil {
		t.Fatalf(`validateConfiguration() error = %v, want nil when JWT secret is empty`, errValidate)
	}
}

func TestValidateConfigurationReturnsDecodedCookieKeys(t *testing.T) {
	config := validConfigurationForTest()
	hashKey := encodeSecretForTest(48)
	blockKey := encodeSecretForTest(32)
	config.CookieHashKey = hashKey
	config.CookieBlockKey = blockKey

	validatedConfig, errValidate := validateConfiguration(config)
	if errValidate != nil {
		t.Fatalf(`validateConfiguration() error = %v`, errValidate)
	}

	wantDecodedHashKey, errDecodeHashKey := base64.StdEncoding.DecodeString(hashKey)
	if errDecodeHashKey != nil {
		t.Fatalf(`DecodeString(hash) error = %v`, errDecodeHashKey)
	}
	wantDecodedBlockKey, errDecodeBlockKey := base64.StdEncoding.DecodeString(blockKey)
	if errDecodeBlockKey != nil {
		t.Fatalf(`DecodeString(block) error = %v`, errDecodeBlockKey)
	}

	if !bytes.Equal(validatedConfig.cookieHashKey, wantDecodedHashKey) {
		t.Fatal(`cookie hash key mismatch`)
	}
	if !bytes.Equal(validatedConfig.cookieBlockKey, wantDecodedBlockKey) {
		t.Fatal(`cookie block key mismatch`)
	}
}

func TestRegisterMetricsRouteDisabledByDefault(t *testing.T) {
	router := chi.NewRouter()
	registerMetricsRoute(router, Configuration{})

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, `/metrics`, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf(`GET /metrics status = %d, want %d`, response.Code, http.StatusNotFound)
	}
}

func TestRegisterMetricsRouteEnabled(t *testing.T) {
	router := chi.NewRouter()
	registerMetricsRoute(router, Configuration{MetricsEnabled: true})

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, `/metrics`, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(`GET /metrics status = %d, want %d`, response.Code, http.StatusOK)
	}
}

func TestSecurityHeadersAllowGoogleFonts(t *testing.T) {
	handler := securityHeaders(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, `/`, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	csp := response.Header().Get(`Content-Security-Policy`)
	if !strings.Contains(csp, `style-src 'self' 'unsafe-inline' https://fonts.googleapis.com`) {
		t.Fatalf(`Content-Security-Policy missing Google Fonts stylesheet source: %q`, csp)
	}
	if !strings.Contains(csp, `font-src 'self' data: https://fonts.gstatic.com`) {
		t.Fatalf(`Content-Security-Policy missing Google Fonts font source: %q`, csp)
	}
}

func TestSecurityHeadersSupportExternalMapTiles(t *testing.T) {
	testCases := []struct {
		name           string
		requestURL     string
		forwardedProto string
		useTLS         bool
		wantImageSrc   string
		wantReferrer   string
	}{
		{
			name:         "HTTP controller permits HTTP and HTTPS tiles",
			requestURL:   "/game-servers/server-1/map",
			wantImageSrc: `img-src 'self' data: blob: http: https:`,
			wantReferrer: "strict-origin-when-cross-origin",
		},
		{
			name:         "HTTPS controller permits only HTTPS tiles",
			requestURL:   "/game-servers/server-1/map",
			useTLS:       true,
			wantImageSrc: `img-src 'self' data: blob: https:`,
			wantReferrer: "strict-origin-when-cross-origin",
		},
		{
			name:           "spoofed forwarded proto is ignored without trusted proxies",
			requestURL:     "/game-servers/server-1/map",
			forwardedProto: "https",
			wantImageSrc:   `img-src 'self' data: blob: http: https:`,
			wantReferrer:   "strict-origin-when-cross-origin",
		},
		{
			name:         "public map does not send referrers",
			requestURL:   "/shared/palworld-map",
			wantImageSrc: `img-src 'self' data: blob: http: https:`,
			wantReferrer: "no-referrer",
		},
		{
			name:         "public status page does not send referrers",
			requestURL:   "/status/Owner_Page",
			wantImageSrc: `img-src 'self' data: blob: http: https:`,
			wantReferrer: "no-referrer",
		},
		{
			name:         "public status stream does not send referrers",
			requestURL:   "/api/public/status-pages/Owner_Page/events",
			wantImageSrc: `img-src 'self' data: blob: http: https:`,
			wantReferrer: "no-referrer",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := securityHeaders(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, testCase.requestURL, nil)
			if testCase.forwardedProto != "" {
				request.Header.Set("X-Forwarded-Proto", testCase.forwardedProto)
			}
			if testCase.useTLS {
				request.TLS = &tls.ConnectionState{}
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			csp := response.Header().Get("Content-Security-Policy")
			if !strings.Contains(csp, testCase.wantImageSrc) {
				t.Fatalf("Content-Security-Policy = %q, want image source %q", csp, testCase.wantImageSrc)
			}
			if gotReferrer := response.Header().Get("Referrer-Policy"); gotReferrer != testCase.wantReferrer {
				t.Fatalf("Referrer-Policy = %q, want %q", gotReferrer, testCase.wantReferrer)
			}
		})
	}
}

func TestStartupFailureReturnsNonZeroAndCleansUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cleanupCalled := false

	exitCode := startupFailure(func() {
		cleanupCalled = true
	}, cancel, errors.New(`boom`), `startup failed`)

	if exitCode != 1 {
		t.Fatalf(`startupFailure() exit code = %d, want 1`, exitCode)
	}
	if !cleanupCalled {
		t.Fatal(`startupFailure() did not call cleanup`)
	}

	select {
	case <-ctx.Done():
	default:
		t.Fatal(`startupFailure() did not cancel context`)
	}
}

func validConfigurationForTest() Configuration {
	return Configuration{
		CookieHashKey:    encodeSecretForTest(64),
		CookieBlockKey:   encodeSecretForTest(32),
		JWTSecretKey:     encodeSecretForTest(64),
		EncryptionKey:    encodeSecretForTest(32),
		HTTPReadTimeout:  15 * time.Minute,
		HTTPWriteTimeout: 15 * time.Minute,
		HTTPIdleTimeout:  30 * time.Minute,
	}
}

func encodeSecretForTest(size int) string {
	secret := bytes.Repeat([]byte(`a`), size)
	return base64.StdEncoding.EncodeToString(secret)
}
