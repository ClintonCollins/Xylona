package main

import (
	"bytes"
	"context"
	"encoding/base64"
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
			wantErrPart: `missing required configuration: COOKIE_HASH_KEY_BASE64, COOKIE_BLOCK_KEY_BASE64, JWT_SECRET_KEY_BASE64`,
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
			name: `invalid timeout`,
			mutate: func(config *Configuration) {
				config.HTTPReadTimeout = 0
			},
			wantErrPart: `HTTP_READ_TIMEOUT must be greater than zero`,
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

func TestValidateConfigurationReturnsDecodedCookieKeys(t *testing.T) {
	config := validConfigurationForTest()
	hashKey := encodeSecretForTest(64)
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

func validConfigurationForTest() Configuration {
	return Configuration{
		CookieHashKey:          encodeSecretForTest(64),
		CookieBlockKey:         encodeSecretForTest(32),
		JWTSecretKey:           encodeSecretForTest(64),
		HTTPReadTimeout:        15 * time.Minute,
		HTTPWriteTimeout:       15 * time.Minute,
		HTTPIdleTimeout:        30 * time.Minute,
		FederationReadTimeout:  15 * time.Minute,
		FederationWriteTimeout: 15 * time.Minute,
		FederationIdleTimeout:  30 * time.Minute,
	}
}

func encodeSecretForTest(size int) string {
	secret := bytes.Repeat([]byte(`a`), size)
	return base64.StdEncoding.EncodeToString(secret)
}
