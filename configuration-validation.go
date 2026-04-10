package main

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type validatedConfiguration struct {
	cookieHashKey  []byte
	cookieBlockKey []byte
}

func validateConfiguration(config Configuration) (*validatedConfiguration, error) {
	missingVars := make([]string, 0, 3)

	if config.CookieHashKey == `` {
		missingVars = append(missingVars, `COOKIE_HASH_KEY_BASE64`)
	}
	if config.CookieBlockKey == `` {
		missingVars = append(missingVars, `COOKIE_BLOCK_KEY_BASE64`)
	}
	if config.JWTSecretKey == `` {
		missingVars = append(missingVars, `JWT_SECRET_KEY_BASE64`)
	}

	if len(missingVars) > 0 {
		return nil, fmt.Errorf(`missing required configuration: %s`, strings.Join(missingVars, `, `))
	}

	cookieHashKey, errDecodeHashKey := decodeBase64ConfigurationValue(`COOKIE_HASH_KEY_BASE64`, config.CookieHashKey)
	if errDecodeHashKey != nil {
		return nil, errDecodeHashKey
	}
	cookieBlockKey, errDecodeBlockKey := decodeBase64ConfigurationValue(`COOKIE_BLOCK_KEY_BASE64`, config.CookieBlockKey)
	if errDecodeBlockKey != nil {
		return nil, errDecodeBlockKey
	}
	if len(cookieBlockKey) != 16 && len(cookieBlockKey) != 24 && len(cookieBlockKey) != 32 {
		return nil, fmt.Errorf(`COOKIE_BLOCK_KEY_BASE64 must decode to 16, 24, or 32 bytes, got %d`, len(cookieBlockKey))
	}

	_, errDecodeJWTKey := decodeBase64ConfigurationValue(`JWT_SECRET_KEY_BASE64`, config.JWTSecretKey)
	if errDecodeJWTKey != nil {
		return nil, errDecodeJWTKey
	}

	errValidateTimeouts := validateServerTimeoutConfiguration(config)
	if errValidateTimeouts != nil {
		return nil, errValidateTimeouts
	}

	return &validatedConfiguration{
		cookieHashKey:  cookieHashKey,
		cookieBlockKey: cookieBlockKey,
	}, nil
}

func decodeBase64ConfigurationValue(name string, value string) ([]byte, error) {
	decodedValue, errDecode := base64.StdEncoding.DecodeString(value)
	if errDecode != nil {
		return nil, fmt.Errorf(`decode %s: %w`, name, errDecode)
	}
	return decodedValue, nil
}

func validateServerTimeoutConfiguration(config Configuration) error {
	errHTTPReadTimeout := validatePositiveDuration(`HTTP_READ_TIMEOUT`, config.HTTPReadTimeout)
	if errHTTPReadTimeout != nil {
		return errHTTPReadTimeout
	}

	errHTTPWriteTimeout := validatePositiveDuration(`HTTP_WRITE_TIMEOUT`, config.HTTPWriteTimeout)
	if errHTTPWriteTimeout != nil {
		return errHTTPWriteTimeout
	}

	errHTTPIdleTimeout := validatePositiveDuration(`HTTP_IDLE_TIMEOUT`, config.HTTPIdleTimeout)
	if errHTTPIdleTimeout != nil {
		return errHTTPIdleTimeout
	}

	errFederationReadTimeout := validatePositiveDuration(`FEDERATION_READ_TIMEOUT`, config.FederationReadTimeout)
	if errFederationReadTimeout != nil {
		return errFederationReadTimeout
	}

	errFederationWriteTimeout := validatePositiveDuration(`FEDERATION_WRITE_TIMEOUT`, config.FederationWriteTimeout)
	if errFederationWriteTimeout != nil {
		return errFederationWriteTimeout
	}

	errFederationIdleTimeout := validatePositiveDuration(`FEDERATION_IDLE_TIMEOUT`, config.FederationIdleTimeout)
	if errFederationIdleTimeout != nil {
		return errFederationIdleTimeout
	}

	return nil
}

func validatePositiveDuration(name string, value time.Duration) error {
	if value <= 0 {
		return fmt.Errorf(`%s must be greater than zero`, name)
	}
	return nil
}

func registerMetricsRoute(router chi.Router, config Configuration) {
	if !config.MetricsEnabled {
		return
	}
	router.Handle(`/metrics`, promhttp.Handler())
}

func newHTTPServer(config Configuration, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(`%s:%d`, config.Host, config.HTTPPort),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       config.HTTPReadTimeout,
		WriteTimeout:      config.HTTPWriteTimeout,
		IdleTimeout:       config.HTTPIdleTimeout,
	}
}

func newFederationServer(config Configuration, handler http.Handler, tlsConfig *tls.Config) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(`%s:%d`, config.Host, config.FederationPort),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       config.FederationReadTimeout,
		WriteTimeout:      config.FederationWriteTimeout,
		IdleTimeout:       config.FederationIdleTimeout,
		TLSConfig:         tlsConfig,
	}
}
