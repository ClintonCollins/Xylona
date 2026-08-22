package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	sevenDaysToDieMapJSONLimit = 4 << 20
	sevenDaysToDieMapTileLimit = 4 << 20
)

var errSevenDaysToDieMapUnavailable = errors.New("node: 7 Days to Die map is unavailable")

type sevenDaysToDieMapEnvelope struct {
	Data json.RawMessage `json:"data"`
	Meta struct {
		ServerTime string `json:"serverTime"`
	} `json:"meta"`
}

type sevenDaysToDieMapConfigJSON struct {
	Enabled      bool `json:"enabled"`
	MapBlockSize int  `json:"mapBlockSize"`
	BlockSize    int  `json:"blockSize"`
	MaxZoom      int  `json:"maxZoom"`
	MapSize      struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"mapSize"`
}

type sevenDaysToDiePlayerJSON struct {
	EntityID        json.RawMessage `json:"entityId"`
	Name            string          `json:"name"`
	Online          *bool           `json:"online"`
	SteamID         string          `json:"steamid"`
	CrossPlatformID string          `json:"crossplatformid"`
	PlatformID      struct {
		CombinedString string `json:"combinedString"`
	} `json:"platformId"`
	CrossPlatformIDObject struct {
		CombinedString string `json:"combinedString"`
	} `json:"crossplatformId"`
	Position *struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"position"`
}

// QuerySevenDaysToDieMap reads the fixed native WebAPI endpoints from the
// owning node. It intentionally dials loopback and never accepts a caller URL.
func (n *Node) QuerySevenDaysToDieMap(ctx context.Context, req SevenDaysToDieMapQueryRequest) (*SevenDaysToDieMapSnapshot, error) {
	baseURL, errBaseURL := sevenDaysToDieMapBaseURL(req.WorkingDirectory)
	if errBaseURL != nil {
		return nil, errBaseURL
	}

	client := sevenDaysToDieMapHTTPClient()
	configEnvelope, errConfig := sevenDaysToDieMapGetJSON(ctx, client, baseURL+"/api/map/config", req)
	if errConfig != nil {
		return nil, fmt.Errorf("%w: read config: %w", errSevenDaysToDieMapUnavailable, errConfig)
	}
	var config sevenDaysToDieMapConfigJSON
	errDecodeConfig := json.Unmarshal(configEnvelope.Data, &config)
	if errDecodeConfig != nil {
		return nil, fmt.Errorf("%w: decode config: %w", errSevenDaysToDieMapUnavailable, errDecodeConfig)
	}
	tileSize := config.MapBlockSize
	if tileSize == 0 {
		tileSize = config.BlockSize
	}
	if tileSize < 2 || tileSize > 4096 || config.MaxZoom < 0 || config.MaxZoom > 30 || config.MapSize.X <= 0 || config.MapSize.Z <= 0 {
		return nil, fmt.Errorf("%w: invalid native map dimensions", errSevenDaysToDieMapUnavailable)
	}

	playersEnvelope, errPlayers := sevenDaysToDieMapGetJSON(ctx, client, baseURL+"/api/player", req)
	if errPlayers != nil && errors.Is(errPlayers, fs.ErrNotExist) {
		playersEnvelope, errPlayers = sevenDaysToDieMapGetJSON(ctx, client, baseURL+"/api/getplayerslocation?offline=true", req)
	}
	if errPlayers != nil {
		return nil, fmt.Errorf("%w: read players: %w", errSevenDaysToDieMapUnavailable, errPlayers)
	}
	players, errDecodePlayers := decodeSevenDaysToDieMapPlayers(playersEnvelope.Data)
	if errDecodePlayers != nil {
		return nil, fmt.Errorf("%w: decode players: %w", errSevenDaysToDieMapUnavailable, errDecodePlayers)
	}

	return &SevenDaysToDieMapSnapshot{
		Enabled:    config.Enabled,
		TileSize:   int32(tileSize),
		MaxZoom:    int32(config.MaxZoom),
		MapSize:    SevenDaysToDieMapVector{X: config.MapSize.X, Y: config.MapSize.Y, Z: config.MapSize.Z},
		SourceTime: strings.TrimSpace(configEnvelope.Meta.ServerTime),
		Players:    players,
	}, nil
}

// GetSevenDaysToDieMapTile fetches one bounded native PNG tile.
func (n *Node) GetSevenDaysToDieMapTile(ctx context.Context, req SevenDaysToDieMapTileRequest) ([]byte, error) {
	if req.Zoom < 0 || req.Zoom > 30 || req.X < -1_000_000 || req.X > 1_000_000 || req.Y < -1_000_000 || req.Y > 1_000_000 {
		return nil, errors.New("node: invalid 7 Days to Die map tile coordinates")
	}
	baseURL, errBaseURL := sevenDaysToDieMapBaseURL(req.WorkingDirectory)
	if errBaseURL != nil {
		return nil, errBaseURL
	}
	tileURL := fmt.Sprintf("%s/map/%d/%d/%d.png", baseURL, req.Zoom, req.X, req.Y)
	httpRequest, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, tileURL, nil)
	if errRequest != nil {
		return nil, fmt.Errorf("node: create 7 Days to Die tile request: %w", errRequest)
	}
	setSevenDaysToDieMapHeaders(httpRequest, req.TokenName, req.TokenSecret)
	response, errDo := sevenDaysToDieMapHTTPClient().Do(httpRequest)
	if errDo != nil {
		return nil, fmt.Errorf("%w: fetch tile: %w", errSevenDaysToDieMapUnavailable, errDo)
	}
	if response.StatusCode == http.StatusNotFound {
		errClose := response.Body.Close()
		if errClose != nil {
			return nil, fmt.Errorf("node: close missing 7 Days to Die map tile response: %w", errClose)
		}
		return nil, fs.ErrNotExist
	}
	if response.StatusCode != http.StatusOK {
		errClose := response.Body.Close()
		if errClose != nil {
			return nil, fmt.Errorf("%w: tile returned status %d and close response: %w", errSevenDaysToDieMapUnavailable, response.StatusCode, errClose)
		}
		return nil, fmt.Errorf("%w: tile returned status %d", errSevenDaysToDieMapUnavailable, response.StatusCode)
	}
	content, errRead := io.ReadAll(io.LimitReader(response.Body, sevenDaysToDieMapTileLimit+1))
	errClose := response.Body.Close()
	if errRead != nil || errClose != nil {
		return nil, fmt.Errorf("node: read 7 Days to Die map tile: %w", errors.Join(errRead, errClose))
	}
	if len(content) > sevenDaysToDieMapTileLimit || !bytes.HasPrefix(content, []byte("\x89PNG\r\n\x1a\n")) {
		return nil, errors.New("node: invalid 7 Days to Die map tile")
	}
	return content, nil
}

func sevenDaysToDieMapBaseURL(workingDirectory string) (string, error) {
	values, errValues := readSevenDaysToDieServerSettings(workingDirectory)
	if errValues != nil {
		return "", errValues
	}
	port, errPort := strconv.ParseUint(strings.TrimSpace(values["WebDashboardPort"]), 10, 16)
	if errPort != nil || port == 0 {
		return "", errors.New("node: 7 Days to Die WebDashboardPort is invalid")
	}
	return "http://127.0.0.1:" + strconv.FormatUint(port, 10), nil
}

func sevenDaysToDieMapHTTPClient() *http.Client {
	return sevenDaysToDieWebAPIHTTPClient()
}

func sevenDaysToDieMapGetJSON(ctx context.Context, client *http.Client, endpoint string, req SevenDaysToDieMapQueryRequest) (sevenDaysToDieMapEnvelope, error) {
	parsedURL, errURL := url.Parse(endpoint)
	if errURL != nil || parsedURL.Scheme != "http" || parsedURL.Hostname() != "127.0.0.1" {
		return sevenDaysToDieMapEnvelope{}, errors.New("node: invalid 7 Days to Die map endpoint")
	}
	httpRequest, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if errRequest != nil {
		return sevenDaysToDieMapEnvelope{}, fmt.Errorf("node: create 7 Days to Die map request: %w", errRequest)
	}
	setSevenDaysToDieMapHeaders(httpRequest, req.TokenName, req.TokenSecret)
	response, errDo := client.Do(httpRequest)
	if errDo != nil {
		return sevenDaysToDieMapEnvelope{}, fmt.Errorf("perform 7 Days to Die map request: %w", errDo)
	}
	if response.StatusCode == http.StatusNotFound {
		errClose := response.Body.Close()
		if errClose != nil {
			return sevenDaysToDieMapEnvelope{}, fmt.Errorf("close missing response: %w", errClose)
		}
		return sevenDaysToDieMapEnvelope{}, fs.ErrNotExist
	}
	if response.StatusCode != http.StatusOK {
		errClose := response.Body.Close()
		if errClose != nil {
			return sevenDaysToDieMapEnvelope{}, fmt.Errorf("status %d and close response: %w", response.StatusCode, errClose)
		}
		return sevenDaysToDieMapEnvelope{}, fmt.Errorf("status %d", response.StatusCode)
	}
	data, errRead := io.ReadAll(io.LimitReader(response.Body, sevenDaysToDieMapJSONLimit+1))
	errClose := response.Body.Close()
	if errRead != nil || errClose != nil {
		return sevenDaysToDieMapEnvelope{}, errors.Join(errRead, errClose)
	}
	if len(data) > sevenDaysToDieMapJSONLimit {
		return sevenDaysToDieMapEnvelope{}, fmt.Errorf("response exceeds %d bytes", sevenDaysToDieMapJSONLimit)
	}
	var envelope sevenDaysToDieMapEnvelope
	errDecode := json.Unmarshal(data, &envelope)
	if errDecode != nil || len(envelope.Data) == 0 {
		return sevenDaysToDieMapEnvelope{}, fmt.Errorf("decode response: %w", errDecode)
	}
	return envelope, nil
}

func setSevenDaysToDieMapHeaders(request *http.Request, tokenName string, tokenSecret string) {
	request.Header.Set("Accept", "application/json, image/png")
	if strings.TrimSpace(tokenName) != "" && strings.TrimSpace(tokenSecret) != "" {
		request.Header.Set("X-SDTD-API-TOKENNAME", tokenName)
		request.Header.Set("X-SDTD-API-SECRET", tokenSecret)
	}
}

func decodeSevenDaysToDieMapPlayers(data json.RawMessage) ([]SevenDaysToDieMapPlayer, error) {
	var current struct {
		Players []sevenDaysToDiePlayerJSON `json:"players"`
	}
	errCurrent := json.Unmarshal(data, &current)
	if errCurrent != nil {
		return nil, fmt.Errorf("decode current 7 Days to Die player response: %w", errCurrent)
	}
	rawPlayers := current.Players
	if rawPlayers == nil {
		errLegacy := json.Unmarshal(data, &rawPlayers)
		if errLegacy != nil {
			return nil, fmt.Errorf("decode legacy 7 Days to Die player response: %w", errLegacy)
		}
	}
	players := make([]SevenDaysToDieMapPlayer, 0, len(rawPlayers))
	for index, rawPlayer := range rawPlayers {
		name := strings.TrimSpace(rawPlayer.Name)
		if name == "" || rawPlayer.Position == nil {
			continue
		}
		id := strings.TrimSpace(rawPlayer.PlatformID.CombinedString)
		if id == "" {
			id = strings.TrimSpace(rawPlayer.CrossPlatformIDObject.CombinedString)
		}
		if id == "" {
			id = strings.TrimSpace(rawPlayer.SteamID)
		}
		if id == "" {
			id = strings.TrimSpace(rawPlayer.CrossPlatformID)
		}
		if id == "" {
			id = rawJSONIdentifier(rawPlayer.EntityID)
		}
		if id == "" {
			id = name + ":" + strconv.Itoa(index)
		}
		online := true
		if rawPlayer.Online != nil {
			online = *rawPlayer.Online
		}
		players = append(players, SevenDaysToDieMapPlayer{
			ID:     id,
			Name:   name,
			Online: online,
			Position: SevenDaysToDieMapVector{
				X: rawPlayer.Position.X,
				Y: rawPlayer.Position.Y,
				Z: rawPlayer.Position.Z,
			},
		})
	}
	return players, nil
}

func rawJSONIdentifier(value json.RawMessage) string {
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return ""
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return strings.TrimSpace(text)
	}
	var number json.Number
	if json.Unmarshal(value, &number) == nil {
		return number.String()
	}
	return ""
}
