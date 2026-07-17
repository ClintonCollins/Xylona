// Package restinput implements game-specific REST console transports.
package restinput

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout   = 10 * time.Second
	maxResponseBytes = 1 << 20
)

type satisfactoryAPIError struct {
	code    string
	message string
}

func (e *satisfactoryAPIError) Error() string {
	return fmt.Sprintf("rest input: Satisfactory API error %s: %s", e.code, strings.TrimSpace(e.message))
}

// ConfigureSatisfactoryAdminPassword claims an unclaimed server, verifies the
// configured password, or rotates a claimed server from a prior password.
func ConfigureSatisfactoryAdminPassword(
	ctx context.Context,
	host string,
	port int,
	serverName string,
	password string,
	previousPasswords []string,
) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return errors.New("rest input: Satisfactory admin password is empty")
	}
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		serverName = "Xylona Server"
	}

	_, errCurrent := satisfactoryPasswordLogin(ctx, host, port, password)
	if errCurrent == nil {
		return nil
	}
	if !satisfactoryAPIErrorCodeIs(errCurrent, "wrong_password", "server_not_claimed") {
		return fmt.Errorf("verify Satisfactory admin password: %w", errCurrent)
	}

	loginData, errLogin := callSatisfactoryAPI(ctx, host, port, "PasswordlessLogin", map[string]any{
		"MinimumPrivilegeLevel": "InitialAdmin",
	}, "")
	if errLogin == nil {
		initialToken, errToken := authenticationToken(loginData)
		if errToken != nil {
			return errToken
		}

		_, errClaim := callSatisfactoryAPI(ctx, host, port, "ClaimServer", map[string]any{
			"ServerName":    serverName,
			"AdminPassword": password,
		}, initialToken)
		if errClaim != nil {
			return fmt.Errorf("claim Satisfactory server: %w", errClaim)
		}
		return nil
	}
	if !satisfactoryAPIErrorCodeIs(errLogin, "passwordless_login_not_possible") {
		return fmt.Errorf("acquire Satisfactory initial-admin token: %w", errLogin)
	}

	for index := len(previousPasswords) - 1; index >= 0; index-- {
		previousPassword := previousPasswords[index]
		if previousPassword == password {
			continue
		}
		previousToken, errPrevious := satisfactoryPasswordLogin(ctx, host, port, previousPassword)
		if errPrevious != nil {
			if satisfactoryAPIErrorCodeIs(errPrevious, "wrong_password") {
				continue
			}
			return fmt.Errorf("authenticate prior Satisfactory admin password: %w", errPrevious)
		}
		replacementToken, errReplacement := satisfactoryPasswordLogin(ctx, host, port, previousPassword)
		if errReplacement != nil {
			return fmt.Errorf("acquire replacement Satisfactory admin token: %w", errReplacement)
		}
		_, errSet := callSatisfactoryAPI(ctx, host, port, "SetAdminPassword", map[string]any{
			"Password":            password,
			"AuthenticationToken": replacementToken,
		}, previousToken)
		if errSet != nil {
			return fmt.Errorf("update Satisfactory admin password: %w", errSet)
		}
		_, errVerify := satisfactoryPasswordLogin(ctx, host, port, password)
		if errVerify != nil {
			return fmt.Errorf("verify updated Satisfactory admin password: %w", errVerify)
		}
		return nil
	}
	return errors.New("configure Satisfactory admin password: configured and prior passwords were rejected")
}

// ExecuteSatisfactory authenticates and sends a command through the dedicated
// server's HTTPS API. The standard stop command maps to Shutdown.
func ExecuteSatisfactory(
	ctx context.Context,
	host string,
	port int,
	password string,
	command string,
) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("rest input: command is empty")
	}
	authenticationToken, errLogin := satisfactoryPasswordLogin(ctx, host, port, password)
	if errLogin != nil {
		return "", fmt.Errorf("rest input: authenticate Satisfactory API: %w", errLogin)
	}

	isShutdown := strings.EqualFold(command, "quit")
	function := "RunCommand"
	data := map[string]any{"Command": command}
	if isShutdown {
		function = "Shutdown"
		data = map[string]any{}
	}
	resultData, errCall := callSatisfactoryAPI(ctx, host, port, function, data, authenticationToken)
	if errCall != nil {
		return "", errCall
	}
	if isShutdown {
		return "", nil
	}

	var result struct {
		CommandResult string `json:"CommandResult"`
	}
	errDecode := json.Unmarshal(resultData, &result)
	if errDecode != nil {
		return "", fmt.Errorf("rest input: decode Satisfactory response data: %w", errDecode)
	}
	return strings.TrimSpace(result.CommandResult), nil
}

func callSatisfactoryAPI(
	ctx context.Context,
	host string,
	port int,
	function string,
	data map[string]any,
	authenticationToken string,
) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	host = strings.TrimSpace(host)
	if host == "" || (net.ParseIP(host) == nil && !strings.EqualFold(host, "localhost")) {
		return nil, errors.New("rest input: Satisfactory API host is invalid")
	}
	if port <= 0 || port > 65535 {
		return nil, errors.New("rest input: Satisfactory API port is invalid")
	}

	payload := struct {
		Function string         `json:"function"`
		Data     map[string]any `json:"data"`
	}{Function: function, Data: data}
	body, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, fmt.Errorf("rest input: encode Satisfactory request: %w", errMarshal)
	}

	endpoint := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/api/v1",
	}
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if errRequest != nil {
		return nil, fmt.Errorf("rest input: create Satisfactory request: %w", errRequest)
	}
	request.Header.Set("Content-Type", "application/json")
	if authenticationToken != "" {
		request.Header.Set("Authorization", "Bearer "+authenticationToken)
	}

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("rest input: default HTTP transport has an unexpected type")
	}
	transport := defaultTransport.Clone()
	defer transport.CloseIdleConnections()
	transport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // #nosec G402 -- Satisfactory uses a generated self-signed certificate by default.
	}
	client := &http.Client{Transport: transport, Timeout: defaultTimeout}
	response, errDo := client.Do(request)
	if errDo != nil {
		return nil, fmt.Errorf("rest input: call Satisfactory API: %w", errDo)
	}
	responseBody, errRead := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	errClose := response.Body.Close()
	if errRead != nil {
		return nil, errors.Join(
			fmt.Errorf("rest input: read Satisfactory response: %w", errRead),
			wrapCloseError(errClose),
		)
	}
	if errClose != nil {
		return nil, fmt.Errorf("rest input: close Satisfactory response: %w", errClose)
	}
	if len(responseBody) > maxResponseBytes {
		return nil, errors.New("rest input: Satisfactory response exceeds size limit")
	}

	var envelope struct {
		Data         json.RawMessage `json:"data"`
		ErrorCode    string          `json:"errorCode"`
		ErrorMessage string          `json:"errorMessage"`
	}
	if len(responseBody) > 0 {
		errDecode := json.Unmarshal(responseBody, &envelope)
		if errDecode != nil {
			return nil, fmt.Errorf("rest input: decode Satisfactory response: %w", errDecode)
		}
	}
	if envelope.ErrorCode != "" {
		return nil, &satisfactoryAPIError{code: envelope.ErrorCode, message: envelope.ErrorMessage}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("rest input: Satisfactory API returned %s", response.Status)
	}
	if len(envelope.Data) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return envelope.Data, nil
}

func authenticationToken(data json.RawMessage) (string, error) {
	var tokens map[string]string
	errDecode := json.Unmarshal(data, &tokens)
	if errDecode != nil {
		return "", fmt.Errorf("rest input: decode Satisfactory authentication token: %w", errDecode)
	}
	for key, value := range tokens {
		if strings.EqualFold(key, "authenticationToken") && strings.TrimSpace(value) != "" {
			return value, nil
		}
	}
	return "", errors.New("rest input: Satisfactory authentication token is missing")
}

func satisfactoryPasswordLogin(ctx context.Context, host string, port int, password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", errors.New("rest input: Satisfactory admin password is empty")
	}
	data, errLogin := callSatisfactoryAPI(ctx, host, port, "PasswordLogin", map[string]any{
		"MinimumPrivilegeLevel": "Administrator",
		"Password":              password,
	}, "")
	if errLogin != nil {
		return "", errLogin
	}
	return authenticationToken(data)
}

func satisfactoryAPIErrorCodeIs(err error, codes ...string) bool {
	var apiErr *satisfactoryAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return slices.Contains(codes, apiErr.code)
}

func wrapCloseError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("rest input: close Satisfactory response: %w", err)
}
