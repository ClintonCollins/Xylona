package query

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	palworldQueryTimeout       = 4 * time.Second
	palworldMaxResponseBodyLen = 1 << 20
)

type palworldServerInfoResponse struct {
	Version     string `json:"version"`
	ServerName  string `json:"servername"`
	Description string `json:"description"`
	WorldGUID   string `json:"worldguid"`
}

type palworldMetricsResponse struct {
	ServerFPS         float64 `json:"serverfps"`
	CurrentPlayers    int64   `json:"currentplayernum"`
	ServerFrameTimeMS float64 `json:"serverframetime"`
	MaxPlayers        int64   `json:"maxplayernum"`
	UptimeSeconds     int64   `json:"uptime"`
	Days              int64   `json:"days"`
}

type palworldPlayersResponse struct {
	Players []struct {
		Name   string `json:"name"`
		UserID string `json:"userId"`
	} `json:"players"`
}

// PalworldPlayer is a player identity returned by Palworld's administrative
// REST API. UserID is the stable identifier accepted by kick and ban APIs.
type PalworldPlayer struct {
	Name   string
	UserID string
}

// PalworldInfo is the complete Palworld REST query result, including stable
// player identifiers that are intentionally excluded from the broad public
// server-query feed.
type PalworldInfo struct {
	Name              string
	Description       string
	Version           string
	WorldGUID         string
	Players           uint32
	MaxPlayers        uint32
	PlayerDetails     []PalworldPlayer
	UptimeSeconds     uint64
	ServerFPS         float64
	ServerFrameTimeMS float64
	Days              uint32
	Responded         bool
}

// PalworldPlayerAction is a typed Palworld administrative REST action.
type PalworldPlayerAction int

const (
	// PalworldPlayerActionUnknown is invalid and sends no request.
	PalworldPlayerActionUnknown PalworldPlayerAction = iota
	// PalworldPlayerActionKick disconnects an online player.
	PalworldPlayerActionKick
	// PalworldPlayerActionBan blocks a player from joining.
	PalworldPlayerActionBan
	// PalworldPlayerActionUnban removes a player ban.
	PalworldPlayerActionUnban
)

// Palworld queries the authenticated Palworld REST API exposed on the node
// host. The API is intentionally queried from the node because Palworld warns
// against exposing its administrative API directly to the Internet.
func Palworld(
	ctx context.Context,
	host string,
	port int,
	username string,
	password string,
) (*xylona.PalworldQueryInfo, error) {
	info, errQuery := PalworldDetailed(ctx, host, port, username, password)
	if errQuery != nil {
		return nil, errQuery
	}
	return palworldInfoToProto(info), nil
}

// PalworldDetailed queries Palworld and retains the stable user IDs required
// for typed administrative actions.
func PalworldDetailed(
	ctx context.Context,
	host string,
	port int,
	username string,
	password string,
) (*PalworldInfo, error) {
	client := &http.Client{Timeout: palworldQueryTimeout}
	return palworldDetailedWithClient(ctx, client, host, port, username, password)
}

func palworldDetailedWithClient(
	ctx context.Context,
	client *http.Client,
	host string,
	port int,
	username string,
	password string,
) (*PalworldInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return nil, errors.New("palworld query HTTP client is nil")
	}
	baseURL, errBaseURL := palworldBaseURL(host, port, username, password)
	if errBaseURL != nil {
		return nil, errBaseURL
	}
	var serverInfo palworldServerInfoResponse
	errInfo := getPalworldJSON(ctx, client, baseURL+"/info", username, password, &serverInfo)
	if errInfo != nil {
		return nil, fmt.Errorf("query palworld server info: %w", errInfo)
	}

	var metrics palworldMetricsResponse
	errMetrics := getPalworldJSON(ctx, client, baseURL+"/metrics", username, password, &metrics)
	if errMetrics != nil {
		return nil, fmt.Errorf("query palworld server metrics: %w", errMetrics)
	}

	var players palworldPlayersResponse
	errPlayers := getPalworldJSON(ctx, client, baseURL+"/players", username, password, &players)
	if errPlayers != nil {
		return nil, fmt.Errorf("query palworld players: %w", errPlayers)
	}

	playerDetails := make([]PalworldPlayer, 0, len(players.Players))
	for _, player := range players.Players {
		name := strings.TrimSpace(player.Name)
		userID := strings.TrimSpace(player.UserID)
		if name == "" && userID == "" {
			continue
		}
		if name == "" {
			name = userID
		}
		playerDetails = append(playerDetails, PalworldPlayer{Name: name, UserID: userID})
	}

	return &PalworldInfo{
		Name:              serverInfo.ServerName,
		Description:       serverInfo.Description,
		Version:           serverInfo.Version,
		WorldGUID:         serverInfo.WorldGUID,
		Players:           helpers.ClampUint32FromInt64(metrics.CurrentPlayers),
		MaxPlayers:        helpers.ClampUint32FromInt64(metrics.MaxPlayers),
		PlayerDetails:     playerDetails,
		UptimeSeconds:     clampUint64(metrics.UptimeSeconds),
		ServerFPS:         metrics.ServerFPS,
		ServerFrameTimeMS: metrics.ServerFrameTimeMS,
		Days:              helpers.ClampUint32FromInt64(metrics.Days),
		Responded:         true,
	}, nil
}

// PerformPalworldPlayerAction sends a typed kick, ban, or unban request to the
// official Palworld administrative REST API.
func PerformPalworldPlayerAction(
	ctx context.Context,
	host string,
	port int,
	username string,
	password string,
	action PalworldPlayerAction,
	userID string,
	message string,
) error {
	client := &http.Client{Timeout: palworldQueryTimeout}
	return performPalworldPlayerActionWithClient(
		ctx,
		client,
		host,
		port,
		username,
		password,
		action,
		userID,
		message,
	)
}

func performPalworldPlayerActionWithClient(
	ctx context.Context,
	client *http.Client,
	host string,
	port int,
	username string,
	password string,
	action PalworldPlayerAction,
	userID string,
	message string,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return errors.New("palworld action HTTP client is nil")
	}
	baseURL, errBaseURL := palworldBaseURL(host, port, username, password)
	if errBaseURL != nil {
		return errBaseURL
	}

	var endpoint string
	switch action {
	case PalworldPlayerActionKick:
		endpoint = "/kick"
	case PalworldPlayerActionBan:
		endpoint = "/ban"
	case PalworldPlayerActionUnban:
		endpoint = "/unban"
		message = ""
	default:
		return errors.New("palworld player action is unsupported")
	}

	payload := struct {
		UserID  string `json:"userid"`
		Message string `json:"message,omitempty"`
	}{
		UserID:  userID,
		Message: message,
	}
	body, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return fmt.Errorf("marshal palworld player action: %w", errMarshal)
	}
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+endpoint, bytes.NewReader(body))
	if errRequest != nil {
		return fmt.Errorf("create palworld player action request: %w", errRequest)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(username, password)

	response, errDo := client.Do(request)
	if errDo != nil {
		return fmt.Errorf("send palworld player action: %w", errDo)
	}
	responseBody, errRead := io.ReadAll(io.LimitReader(response.Body, palworldMaxResponseBodyLen+1))
	errClose := response.Body.Close()
	if errRead != nil || errClose != nil {
		return fmt.Errorf("read palworld player action response: %w", errors.Join(errRead, errClose))
	}
	if len(responseBody) > palworldMaxResponseBodyLen {
		return fmt.Errorf("palworld player action response exceeds %d bytes", palworldMaxResponseBodyLen)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("palworld player action returned unexpected HTTP status %s", response.Status)
	}
	return nil
}

func palworldInfoToProto(info *PalworldInfo) *xylona.PalworldQueryInfo {
	if info == nil {
		return nil
	}
	playerNames := make([]string, 0, len(info.PlayerDetails))
	for _, player := range info.PlayerDetails {
		playerNames = append(playerNames, player.Name)
	}
	return &xylona.PalworldQueryInfo{
		Name:              info.Name,
		Description:       info.Description,
		Version:           info.Version,
		WorldGuid:         info.WorldGUID,
		Players:           info.Players,
		MaxPlayers:        info.MaxPlayers,
		PlayerList:        playerNames,
		UptimeSeconds:     info.UptimeSeconds,
		ServerFps:         info.ServerFPS,
		ServerFrameTimeMs: info.ServerFrameTimeMS,
		Days:              info.Days,
		Responded:         info.Responded,
	}
}

func palworldBaseURL(host string, port int, username string, password string) (string, error) {
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("palworld query port %d is invalid", port)
	}
	queryHost := normalizePalworldQueryHost(host)
	if queryHost == "" {
		return "", errors.New("palworld query host is required")
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return "", errors.New("palworld query credentials are required")
	}
	return "http://" + net.JoinHostPort(queryHost, strconv.Itoa(port)) + "/v1/api", nil
}

func normalizePalworldQueryHost(host string) string {
	host = strings.TrimSpace(host)
	ip := net.ParseIP(host)
	if ip != nil && ip.IsUnspecified() {
		return "127.0.0.1"
	}
	return host
}

func getPalworldJSON(
	ctx context.Context,
	client *http.Client,
	url string,
	username string,
	password string,
	destination any,
) error {
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errRequest != nil {
		return fmt.Errorf("create request: %w", errRequest)
	}
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(username, password)

	response, errDo := client.Do(request)
	if errDo != nil {
		return fmt.Errorf("send request: %w", errDo)
	}
	body, errRead := io.ReadAll(io.LimitReader(response.Body, palworldMaxResponseBodyLen+1))
	errClose := response.Body.Close()
	if errRead != nil || errClose != nil {
		return fmt.Errorf("read response: %w", errors.Join(errRead, errClose))
	}
	if len(body) > palworldMaxResponseBodyLen {
		return fmt.Errorf("response exceeds %d bytes", palworldMaxResponseBodyLen)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	errDecode := json.Unmarshal(body, destination)
	if errDecode != nil {
		return fmt.Errorf("decode response: %w", errDecode)
	}
	return nil
}

func clampUint64(value int64) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}
