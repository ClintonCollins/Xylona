package restinput

import (
	"bytes"
	"context"
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
)

const palworldAPIBasePath = "/v1/api"

// ErrPalworldCommandRejected identifies bounded, sanitized command or API
// rejections that are safe to show to the operator.
var ErrPalworldCommandRejected = errors.New("rest input: Palworld command rejected")

type palworldCommandRejectedError struct {
	detail string
}

func (e *palworldCommandRejectedError) Error() string {
	return e.detail
}

func (e *palworldCommandRejectedError) Unwrap() error {
	return ErrPalworldCommandRejected
}

type palworldCommandRequest struct {
	method       string
	path         string
	body         any
	confirmation string
}

type palworldServerInfoResponse struct {
	version     string
	serverName  string
	description string
	worldGUID   string
}

func (r *palworldServerInfoResponse) UnmarshalJSON(data []byte) error {
	var response struct {
		Version     string `json:"version"`
		ServerName  string `json:"servername"`
		Description string `json:"description"`
		WorldGUID   string `json:"worldguid"`
	}
	errUnmarshal := json.Unmarshal(data, &response)
	if errUnmarshal != nil {
		return fmt.Errorf("unmarshal Palworld server info fields: %w", errUnmarshal)
	}
	r.version = response.Version
	r.serverName = response.ServerName
	r.description = response.Description
	r.worldGUID = response.WorldGUID
	return nil
}

type palworldPlayerResponse struct {
	name        string
	accountName string
	playerID    string
	userID      string
	level       int64
}

type palworldPlayersResponse struct {
	players []palworldPlayerResponse
}

func (r *palworldPlayersResponse) UnmarshalJSON(data []byte) error {
	var response struct {
		Players []struct {
			Name        string `json:"name"`
			AccountName string `json:"accountName"`
			PlayerID    string `json:"playerId"`
			UserID      string `json:"userId"`
			Level       int64  `json:"level"`
		} `json:"players"`
	}
	errUnmarshal := json.Unmarshal(data, &response)
	if errUnmarshal != nil {
		return fmt.Errorf("unmarshal Palworld player fields: %w", errUnmarshal)
	}
	r.players = make([]palworldPlayerResponse, 0, len(response.Players))
	for _, player := range response.Players {
		r.players = append(r.players, palworldPlayerResponse{
			name:        player.Name,
			accountName: player.AccountName,
			playerID:    player.PlayerID,
			userID:      player.UserID,
			level:       player.Level,
		})
	}
	return nil
}

// ExecutePalworld translates Palworld's familiar slash commands to the
// equivalent authenticated administrative REST API calls.
func ExecutePalworld(
	ctx context.Context,
	host string,
	port int,
	password string,
	command string,
) (string, error) {
	requestCommand, errParse := parsePalworldCommand(command)
	if errParse != nil {
		return "", newPalworldCommandRejectedError(errParse)
	}

	responseBody, errCall := callPalworldAPI(ctx, host, port, password, requestCommand)
	if errCall != nil {
		return "", errCall
	}

	switch requestCommand.path {
	case "/info":
		output, errFormat := formatPalworldServerInfo(responseBody)
		if errFormat != nil {
			return "", newPalworldCommandRejectedError(errFormat)
		}
		return output, nil
	case "/players":
		output, errFormat := formatPalworldPlayers(responseBody)
		if errFormat != nil {
			return "", newPalworldCommandRejectedError(errFormat)
		}
		return output, nil
	default:
		return requestCommand.confirmation, nil
	}
}

func parsePalworldCommand(command string) (palworldCommandRequest, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return palworldCommandRequest{}, errors.New("rest input: Palworld command is empty")
	}
	if !strings.HasPrefix(command, "/") {
		return palworldCommandRequest{}, errors.New("rest input: Palworld command must begin with a slash")
	}

	fields := strings.Fields(command)
	commandName := strings.ToLower(fields[0])
	arguments := fields[1:]
	switch commandName {
	case "/info":
		errArguments := requirePalworldArgumentCount(commandName, arguments, 0, 0)
		if errArguments != nil {
			return palworldCommandRequest{}, errArguments
		}
		return palworldCommandRequest{method: http.MethodGet, path: "/info"}, nil
	case "/showplayers":
		errArguments := requirePalworldArgumentCount(commandName, arguments, 0, 0)
		if errArguments != nil {
			return palworldCommandRequest{}, errArguments
		}
		return palworldCommandRequest{method: http.MethodGet, path: "/players"}, nil
	case "/broadcast":
		errArguments := requirePalworldArgumentCount(commandName, arguments, 1, -1)
		if errArguments != nil {
			return palworldCommandRequest{}, errArguments
		}
		return palworldCommandRequest{
			method:       http.MethodPost,
			path:         "/announce",
			body:         map[string]any{"message": strings.Join(arguments, " ")},
			confirmation: "Broadcast sent.",
		}, nil
	case "/kickplayer", "/banplayer":
		errArguments := requirePalworldArgumentCount(commandName, arguments, 1, -1)
		if errArguments != nil {
			return palworldCommandRequest{}, errArguments
		}
		path := "/kick"
		confirmation := "Player kick requested."
		if commandName == "/banplayer" {
			path = "/ban"
			confirmation = "Player ban requested."
		}
		payload := map[string]any{"userid": arguments[0]}
		if len(arguments) > 1 {
			payload["message"] = strings.Join(arguments[1:], " ")
		}
		return palworldCommandRequest{
			method:       http.MethodPost,
			path:         path,
			body:         payload,
			confirmation: confirmation,
		}, nil
	case "/unbanplayer":
		errArguments := requirePalworldArgumentCount(commandName, arguments, 1, 1)
		if errArguments != nil {
			return palworldCommandRequest{}, errArguments
		}
		return palworldCommandRequest{
			method:       http.MethodPost,
			path:         "/unban",
			body:         map[string]any{"userid": arguments[0]},
			confirmation: "Player unban requested.",
		}, nil
	case "/save":
		errArguments := requirePalworldArgumentCount(commandName, arguments, 0, 0)
		if errArguments != nil {
			return palworldCommandRequest{}, errArguments
		}
		return palworldCommandRequest{
			method:       http.MethodPost,
			path:         "/save",
			confirmation: "World save requested.",
		}, nil
	case "/shutdown":
		errArguments := requirePalworldArgumentCount(commandName, arguments, 1, -1)
		if errArguments != nil {
			return palworldCommandRequest{}, errArguments
		}
		waitTime, errWaitTime := strconv.Atoi(arguments[0])
		if errWaitTime != nil || waitTime < 0 {
			return palworldCommandRequest{}, errors.New(
				"rest input: Palworld /Shutdown wait time must be a non-negative integer",
			)
		}
		payload := map[string]any{"waittime": waitTime}
		if len(arguments) > 1 {
			payload["message"] = strings.Join(arguments[1:], " ")
		}
		return palworldCommandRequest{
			method:       http.MethodPost,
			path:         "/shutdown",
			body:         payload,
			confirmation: fmt.Sprintf("Server shutdown requested in %d seconds.", waitTime),
		}, nil
	case "/doexit":
		errArguments := requirePalworldArgumentCount(commandName, arguments, 0, 0)
		if errArguments != nil {
			return palworldCommandRequest{}, errArguments
		}
		return palworldCommandRequest{
			method:       http.MethodPost,
			path:         "/stop",
			confirmation: "Immediate server stop requested.",
		}, nil
	default:
		return palworldCommandRequest{}, fmt.Errorf(
			"rest input: unsupported Palworld command %q",
			fields[0],
		)
	}
}

func requirePalworldArgumentCount(command string, arguments []string, minimum int, maximum int) error {
	if len(arguments) < minimum {
		return fmt.Errorf("rest input: Palworld %s is missing required arguments", command)
	}
	if maximum >= 0 && len(arguments) > maximum {
		return fmt.Errorf("rest input: Palworld %s has unexpected arguments", command)
	}
	return nil
}

func callPalworldAPI(
	ctx context.Context,
	host string,
	port int,
	password string,
	command palworldCommandRequest,
) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	apiHost, errHost := palworldAPIHost(host)
	if errHost != nil {
		return nil, errHost
	}
	if port <= 0 || port > 65535 {
		return nil, errors.New("rest input: Palworld API port is invalid")
	}
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("rest input: Palworld API password is empty")
	}

	var requestBody io.Reader
	if command.body != nil {
		body, errMarshal := json.Marshal(command.body)
		if errMarshal != nil {
			return nil, fmt.Errorf("rest input: encode Palworld request: %w", errMarshal)
		}
		requestBody = bytes.NewReader(body)
	}

	endpoint := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(apiHost, strconv.Itoa(port)),
		Path:   palworldAPIBasePath + command.path,
	}
	request, errRequest := http.NewRequestWithContext(ctx, command.method, endpoint.String(), requestBody)
	if errRequest != nil {
		return nil, fmt.Errorf("rest input: create Palworld request: %w", errRequest)
	}
	request.Header.Set("Accept", "application/json")
	if command.body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.SetBasicAuth("admin", password)

	client := &http.Client{Timeout: defaultTimeout}
	transport, errTransport := palworldHTTPTransport()
	if errTransport != nil {
		return nil, errTransport
	}
	defer transport.CloseIdleConnections()
	client.Transport = transport
	response, errDo := client.Do(request)
	if errDo != nil {
		errContext := ctx.Err()
		if errContext != nil {
			return nil, fmt.Errorf("rest input: call Palworld API: %w", errContext)
		}
		return nil, fmt.Errorf("rest input: call Palworld API: %w", errDo)
	}
	responseBody, errRead := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	errClose := response.Body.Close()
	if errRead != nil {
		errRead = fmt.Errorf("rest input: read Palworld response: %w", errRead)
	}
	if errClose != nil {
		errClose = fmt.Errorf("rest input: close Palworld response: %w", errClose)
	}
	errResponse := errors.Join(errRead, errClose)
	if errResponse != nil {
		return nil, errResponse
	}
	if len(responseBody) > maxResponseBytes {
		return nil, newPalworldCommandRejectedError(
			errors.New("rest input: Palworld response exceeds size limit"),
		)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		detail := palworldErrorDetail(responseBody, password)
		if detail == "" {
			return nil, newPalworldCommandRejectedError(
				fmt.Errorf("rest input: Palworld API returned %s", response.Status),
			)
		}
		return nil, newPalworldCommandRejectedError(
			fmt.Errorf("rest input: Palworld API returned %s: %s", response.Status, detail),
		)
	}
	return responseBody, nil
}

func palworldHTTPTransport() (*http.Transport, error) {
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("rest input: default HTTP transport has an unexpected type")
	}
	transport := defaultTransport.Clone()
	transport.Proxy = nil
	return transport, nil
}

func palworldAPIHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	ip := net.ParseIP(host)
	if ip != nil && ip.IsUnspecified() {
		return "127.0.0.1", nil
	}
	if strings.EqualFold(host, "localhost") {
		return "localhost", nil
	}
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("rest input: Palworld API host is invalid")
	}
	return host, nil
}

func newPalworldCommandRejectedError(err error) error {
	const maxDetailRunes = 512

	detail := strings.TrimSpace(err.Error())
	detail = strings.TrimPrefix(detail, "rest input: ")
	detail = strings.Join(strings.Fields(detail), " ")
	if detail == "" {
		detail = "Palworld command was rejected"
	}
	runes := []rune(detail)
	if len(runes) > maxDetailRunes {
		detail = string(runes[:maxDetailRunes-3]) + "..."
	}
	return &palworldCommandRejectedError{detail: detail}
}

func palworldErrorDetail(body []byte, password string) string {
	detail := strings.Join(strings.Fields(string(body)), " ")
	if password != "" {
		detail = strings.ReplaceAll(detail, password, "[redacted]")
	}
	return detail
}

func formatPalworldServerInfo(body []byte) (string, error) {
	var response palworldServerInfoResponse
	errDecode := json.Unmarshal(body, &response)
	if errDecode != nil {
		return "", fmt.Errorf("rest input: decode Palworld server info: %w", errDecode)
	}
	return strings.Join([]string{
		"Server: " + palworldDisplayValue(response.serverName),
		"Version: " + palworldDisplayValue(response.version),
		"Description: " + palworldDisplayValue(response.description),
		"World GUID: " + palworldDisplayValue(response.worldGUID),
	}, "\n"), nil
}

func formatPalworldPlayers(body []byte) (string, error) {
	var response palworldPlayersResponse
	errDecode := json.Unmarshal(body, &response)
	if errDecode != nil {
		return "", fmt.Errorf("rest input: decode Palworld player list: %w", errDecode)
	}
	slices.SortFunc(response.players, func(left palworldPlayerResponse, right palworldPlayerResponse) int {
		leftKey := strings.ToLower(left.name + "\x00" + left.userID + "\x00" + left.playerID)
		rightKey := strings.ToLower(right.name + "\x00" + right.userID + "\x00" + right.playerID)
		return strings.Compare(leftKey, rightKey)
	})

	lines := make([]string, 1, len(response.players)+1)
	lines[0] = fmt.Sprintf("Players online: %d", len(response.players))
	for _, player := range response.players {
		lines = append(lines, fmt.Sprintf(
			"- %s | Account: %s | Player ID: %s | User ID: %s | Level: %d",
			palworldDisplayValue(player.name),
			palworldDisplayValue(player.accountName),
			palworldDisplayValue(player.playerID),
			palworldDisplayValue(player.userID),
			player.level,
		))
	}
	return strings.Join(lines, "\n"), nil
}

func palworldDisplayValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "(not reported)"
	}
	return value
}
