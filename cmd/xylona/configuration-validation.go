package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
)

type validatedConfiguration struct {
	cookieHashKey  []byte
	cookieBlockKey []byte
	trustedProxies *gatekeeper.ProxyTrust
}

func validateConfiguration(config Configuration) (*validatedConfiguration, error) {
	missingVars := make([]string, 0, 4)

	if config.CookieHashKey == `` {
		missingVars = append(missingVars, `COOKIE_HASH_KEY_BASE64`)
	}
	if config.CookieBlockKey == `` {
		missingVars = append(missingVars, `COOKIE_BLOCK_KEY_BASE64`)
	}
	if config.EncryptionKey == `` {
		missingVars = append(missingVars, `ENCRYPTION_KEY_BASE64`)
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

	if config.JWTSecretKey != `` {
		_, errDecodeJWTKey := decodeBase64ConfigurationValue(`JWT_SECRET_KEY_BASE64`, config.JWTSecretKey)
		if errDecodeJWTKey != nil {
			return nil, errDecodeJWTKey
		}
	}

	_, errDecodeEncryptionKey := decodeDatabaseEncryptionKey(config.EncryptionKey)
	if errDecodeEncryptionKey != nil {
		return nil, errDecodeEncryptionKey
	}

	errValidateTimeouts := validateServerTimeoutConfiguration(config)
	if errValidateTimeouts != nil {
		return nil, errValidateTimeouts
	}

	trustedProxies, errTrustedProxies := gatekeeper.ParseTrustedProxies(config.TrustedProxies)
	if errTrustedProxies != nil {
		return nil, fmt.Errorf("TRUSTED_PROXIES: %w", errTrustedProxies)
	}

	return &validatedConfiguration{
		cookieHashKey:  cookieHashKey,
		cookieBlockKey: cookieBlockKey,
		trustedProxies: trustedProxies,
	}, nil
}

func decodeBase64ConfigurationValue(name string, value string) ([]byte, error) {
	decodedValue, errDecode := base64.StdEncoding.DecodeString(value)
	if errDecode != nil {
		return nil, fmt.Errorf(`decode %s: %w`, name, errDecode)
	}
	return decodedValue, nil
}

func decodeDatabaseEncryptionKey(encodedKey string) ([]byte, error) {
	if encodedKey == `` {
		return nil, errors.New(`ENCRYPTION_KEY_BASE64 must be set`)
	}

	decodedKey, errDecode := decodeBase64ConfigurationValue(`ENCRYPTION_KEY_BASE64`, encodedKey)
	if errDecode != nil {
		return nil, errDecode
	}
	if len(decodedKey) < xycrypt.EncryptionKeySize {
		return nil, fmt.Errorf(
			`ENCRYPTION_KEY_BASE64 must decode to at least %d bytes, got %d`,
			xycrypt.EncryptionKeySize,
			len(decodedKey),
		)
	}

	// Preserve compatibility with older deployments that supplied longer
	// secrets while still pinning AES-256 to a 32-byte key.
	return decodedKey[:xycrypt.EncryptionKeySize], nil
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
