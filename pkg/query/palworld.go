package query

import (
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
		Name string `json:"name"`
	} `json:"players"`
}

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
	client := &http.Client{Timeout: palworldQueryTimeout}
	return palworldWithClient(ctx, client, host, port, username, password)
}

func palworldWithClient(
	ctx context.Context,
	client *http.Client,
	host string,
	port int,
	username string,
	password string,
) (*xylona.PalworldQueryInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return nil, errors.New("palworld query HTTP client is nil")
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("palworld query port %d is invalid", port)
	}
	queryHost := normalizePalworldQueryHost(host)
	if queryHost == "" {
		return nil, errors.New("palworld query host is required")
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return nil, errors.New("palworld query credentials are required")
	}

	baseURL := "http://" + net.JoinHostPort(queryHost, strconv.Itoa(port)) + "/v1/api"
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

	playerNames := make([]string, 0, len(players.Players))
	for _, player := range players.Players {
		name := strings.TrimSpace(player.Name)
		if name == "" {
			continue
		}
		playerNames = append(playerNames, name)
	}

	return &xylona.PalworldQueryInfo{
		Name:              serverInfo.ServerName,
		Description:       serverInfo.Description,
		Version:           serverInfo.Version,
		WorldGuid:         serverInfo.WorldGUID,
		Players:           helpers.ClampUint32FromInt64(metrics.CurrentPlayers),
		MaxPlayers:        helpers.ClampUint32FromInt64(metrics.MaxPlayers),
		PlayerList:        playerNames,
		UptimeSeconds:     clampUint64(metrics.UptimeSeconds),
		ServerFps:         metrics.ServerFPS,
		ServerFrameTimeMs: metrics.ServerFrameTimeMS,
		Days:              helpers.ClampUint32FromInt64(metrics.Days),
		Responded:         true,
	}, nil
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
