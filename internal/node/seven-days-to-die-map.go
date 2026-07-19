package node

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	sevenDaysToDieServerConfigName = "serverconfig.xml"
	sevenDaysToDieMapJSONLimit     = 4 << 20
	sevenDaysToDieMapTileLimit     = 4 << 20
)

var errSevenDaysToDieMapUnavailable = errors.New("node: 7 Days to Die map is unavailable")

type sevenDaysToDieServerSettingsXML struct {
	XMLName    xml.Name `xml:"ServerSettings"`
	Properties []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"property"`
}

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

type sevenDaysToDieMarkerJSON struct {
	ID   string  `json:"id"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Name *string `json:"name"`
	Icon *string `json:"icon"`
}

type sevenDaysToDieClaimsJSON struct {
	ClaimSize   int `json:"claimsize"`
	ClaimOwners []struct {
		SteamID    string  `json:"steamid"`
		PlayerName *string `json:"playername"`
		Active     bool    `json:"claimactive"`
		Claims     []struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			Z float64 `json:"z"`
		} `json:"claims"`
	} `json:"claimowners"`
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

	markers := make([]SevenDaysToDieMapMarker, 0)
	markersEnvelope, errMarkers := sevenDaysToDieMapGetJSON(ctx, client, baseURL+"/api/markers", req)
	if errMarkers == nil {
		var rawMarkers []sevenDaysToDieMarkerJSON
		errDecodeMarkers := json.Unmarshal(markersEnvelope.Data, &rawMarkers)
		if errDecodeMarkers == nil {
			for _, marker := range rawMarkers {
				markers = append(markers, SevenDaysToDieMapMarker{
					ID:   strings.TrimSpace(marker.ID),
					X:    marker.X,
					Z:    marker.Y,
					Name: stringPointerValue(marker.Name),
					Icon: stringPointerValue(marker.Icon),
				})
			}
		}
	}

	claims, claimsSupported, errClaims := querySevenDaysToDieLandClaims(ctx, client, baseURL, req)
	if errClaims != nil {
		return nil, fmt.Errorf("%w: read land claims: %w", errSevenDaysToDieMapUnavailable, errClaims)
	}

	return &SevenDaysToDieMapSnapshot{
		Enabled:         config.Enabled,
		TileSize:        int32(tileSize),
		MaxZoom:         int32(config.MaxZoom),
		MapSize:         SevenDaysToDieMapVector{X: config.MapSize.X, Y: config.MapSize.Y, Z: config.MapSize.Z},
		SourceTime:      strings.TrimSpace(configEnvelope.Meta.ServerTime),
		Players:         players,
		Markers:         markers,
		Claims:          claims,
		ClaimsSupported: claimsSupported,
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
	trimmedDirectory := strings.TrimSpace(workingDirectory)
	if trimmedDirectory == "" {
		return "", errors.New("node: 7 Days to Die working directory is empty")
	}
	configPath := filepath.Join(trimmedDirectory, sevenDaysToDieServerConfigName)
	data, errRead := os.ReadFile(configPath) // #nosec G304 -- fixed file under the tracked server directory.
	if errRead != nil {
		return "", fmt.Errorf("node: read 7 Days to Die server config: %w", errRead)
	}
	var settings sevenDaysToDieServerSettingsXML
	errXML := xml.Unmarshal(data, &settings)
	if errXML != nil {
		return "", fmt.Errorf("node: parse 7 Days to Die server config: %w", errXML)
	}
	values := make(map[string]string, len(settings.Properties))
	for _, property := range settings.Properties {
		values[property.Name] = property.Value
	}
	port, errPort := strconv.ParseUint(strings.TrimSpace(values["WebDashboardPort"]), 10, 16)
	if errPort != nil || port == 0 {
		return "", errors.New("node: 7 Days to Die WebDashboardPort is invalid")
	}
	return "http://127.0.0.1:" + strconv.FormatUint(port, 10), nil
}

func sevenDaysToDieMapHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
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

func querySevenDaysToDieLandClaims(ctx context.Context, client *http.Client, baseURL string, req SevenDaysToDieMapQueryRequest) ([]SevenDaysToDieLandClaim, bool, error) {
	envelope, errClaims := sevenDaysToDieMapGetJSON(ctx, client, baseURL+"/api/getlandclaims", req)
	if errors.Is(errClaims, fs.ErrNotExist) {
		return nil, false, nil
	}
	if errClaims != nil {
		return nil, false, errClaims
	}
	var raw sevenDaysToDieClaimsJSON
	errDecode := json.Unmarshal(envelope.Data, &raw)
	if errDecode != nil {
		return nil, false, fmt.Errorf("decode 7 Days to Die land claims: %w", errDecode)
	}
	if raw.ClaimSize <= 0 || raw.ClaimSize > math.MaxInt32 {
		return nil, false, errors.New("7 Days to Die land claim size is invalid")
	}
	claims := make([]SevenDaysToDieLandClaim, 0)
	for _, owner := range raw.ClaimOwners {
		for _, claim := range owner.Claims {
			claims = append(claims, SevenDaysToDieLandClaim{
				OwnerID:   strings.TrimSpace(owner.SteamID),
				OwnerName: stringPointerValue(owner.PlayerName),
				Active:    owner.Active,
				Position:  SevenDaysToDieMapVector{X: claim.X, Y: claim.Y, Z: claim.Z},
				Size:      int32(raw.ClaimSize),
			})
		}
	}
	return claims, true, nil
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
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
