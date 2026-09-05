// Package providerhttp contains shared HTTP helpers for mod provider clients.
package providerhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

type userAgentTransport struct {
	wrapped   http.RoundTripper
	userAgent string
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("User-Agent", t.userAgent)
	resp, errRoundTrip := t.wrapped.RoundTrip(cloned)
	if errRoundTrip != nil {
		return nil, fmt.Errorf("round trip request: %w", errRoundTrip)
	}
	return resp, nil
}

// NewUserAgentClient returns an HTTP client that injects the given User-Agent.
func NewUserAgentClient(userAgent string) *http.Client {
	return &http.Client{
		Transport: &userAgentTransport{
			wrapped:   http.DefaultTransport,
			userAgent: userAgent,
		},
	}
}

// GetJSON fetches endpoint and decodes the JSON response into dest.
func GetJSON(ctx context.Context, client *http.Client, endpoint string, dest any, providerName string) error {
	return GetJSONLimited(ctx, client, endpoint, dest, providerName, 0)
}

// GetJSONLimited fetches endpoint, caps the response size when requested, and decodes JSON into dest.
func GetJSONLimited(ctx context.Context, client *http.Client, endpoint string, dest any, providerName string, maxBytes int64) error {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if errReq != nil {
		return fmt.Errorf("build request for %s: %w", endpoint, errReq)
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return fmt.Errorf("GET %s: %w", endpoint, errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Warn().Err(errClose).Str("url", endpoint).Msg(providerName + ": failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %d", endpoint, resp.StatusCode)
	}

	if maxBytes > 0 {
		limitedBody := io.LimitReader(resp.Body, maxBytes+1)
		body, errRead := io.ReadAll(limitedBody)
		if errRead != nil {
			return fmt.Errorf("read response from %s: %w", endpoint, errRead)
		}
		if int64(len(body)) > maxBytes {
			return fmt.Errorf("response exceeded %d bytes for %s", maxBytes, endpoint)
		}

		errDecode := json.NewDecoder(bytes.NewReader(body)).Decode(dest)
		if errDecode != nil {
			return fmt.Errorf("decode response from %s: %w", endpoint, errDecode)
		}
		return nil
	}

	errDecode := json.NewDecoder(resp.Body).Decode(dest)
	if errDecode != nil {
		return fmt.Errorf("decode response from %s: %w", endpoint, errDecode)
	}
	return nil
}

// DownloadToFile downloads rawURL into destPath and returns bytes written plus SHA-256.
func DownloadToFile(ctx context.Context, client *http.Client, rawURL string, destPath string, providerName string) (int64, string, error) {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if errReq != nil {
		return 0, "", fmt.Errorf("build request: %w", errReq)
	}

	resp, errGet := client.Do(req)
	if errGet != nil {
		return 0, "", fmt.Errorf("GET %s: %w", rawURL, errGet)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Warn().Err(errClose).Msg(providerName + ": failed to close download response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, rawURL)
	}

	outFile, errCreate := os.Create(destPath)
	if errCreate != nil {
		return 0, "", fmt.Errorf("create file %s: %w", destPath, errCreate)
	}
	defer func() {
		if errClose := outFile.Close(); errClose != nil {
			log.Warn().Err(errClose).Str("path", destPath).Msg(providerName + ": failed to close output file")
		}
	}()

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)
	limitedBody := io.LimitReader(resp.Body, modproviders.MaxModDownloadSize+1)
	written, errCopy := io.Copy(writer, limitedBody)
	if errCopy != nil {
		return 0, "", fmt.Errorf("write file %s: %w", destPath, errCopy)
	}
	if written > modproviders.MaxModDownloadSize {
		return 0, "", fmt.Errorf("file %s (%d bytes): %w", destPath, written, modproviders.ErrDownloadTooLarge)
	}

	return written, fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// StringParam returns a string search parameter.
func StringParam(params modproviders.SearchParams, key string) string {
	s, _ := params[key].(string)
	return s
}

// IntParam returns an integer search parameter or defaultValue.
func IntParam(params modproviders.SearchParams, key string, defaultValue int) int {
	n, ok := params[key].(int)
	if !ok {
		return defaultValue
	}
	return n
}

// StringSliceParam returns a string slice search parameter.
func StringSliceParam(params modproviders.SearchParams, key string) []string {
	s, _ := params[key].([]string)
	return s
}
