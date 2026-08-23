package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	sevenDaysToDieMapJSONLimit           = 4 << 20
	sevenDaysToDieMapTileLimit           = 4 << 20
	sevenDaysToDieMapItemLimit           = SevenDaysToDieMapItemLimit
	sevenDaysToDieMapTextLimit           = 256
	sevenDaysToDieMapDepthLimit          = 12
	sevenDaysToDieMapTokenLimit          = 32768
	sevenDaysToDieMapContainerEntryLimit = 4096
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
	Ping            *int32          `json:"ping"`
	Level           *int32          `json:"level"`
	Health          *int32          `json:"health"`
	Stamina         *float32        `json:"stamina"`
	Score           *int32          `json:"score"`
	Deaths          *int32          `json:"deaths"`
	SteamID         string          `json:"steamid"`
	CrossPlatformID string          `json:"crossplatformid"`
	PlatformID      struct {
		CombinedString string `json:"combinedString"`
	} `json:"platformId"`
	CrossPlatformIDObject struct {
		CombinedString string `json:"combinedString"`
	} `json:"crossplatformId"`
	Kills *struct {
		Zombies *int32 `json:"zombies"`
		Players *int32 `json:"players"`
	} `json:"kills"`
	Banned *struct {
		Active *bool `json:"banActive"`
	} `json:"banned"`
	Position *struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"position"`
}

type sevenDaysToDieNativeMarkerJSON struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	X    *float64 `json:"x"`
	Y    *float64 `json:"y"`
}

type sevenDaysToDieLandClaimsJSON struct {
	ClaimSize   *int32 `json:"claimsize"`
	ClaimOwners []struct {
		SteamID    string `json:"steamid"`
		Active     *bool  `json:"claimactive"`
		PlayerName string `json:"playername"`
		Claims     []struct {
			X *float64 `json:"x"`
			Y *float64 `json:"y"`
			Z *float64 `json:"z"`
		} `json:"claims"`
	} `json:"claimowners"`
}

type sevenDaysToDieMapEntityJSON struct {
	ID       json.RawMessage `json:"id"`
	Name     string          `json:"name"`
	Position *struct {
		X *float64 `json:"x"`
		Y *float64 `json:"y"`
		Z *float64 `json:"z"`
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
	if tileSize < 2 || tileSize > 4096 || config.MaxZoom < 0 || config.MaxZoom > 30 ||
		!validSevenDaysToDieMapDimension(config.MapSize.X) || !validSevenDaysToDieMapDimension(config.MapSize.Z) {
		return nil, fmt.Errorf("%w: invalid native map dimensions", errSevenDaysToDieMapUnavailable)
	}

	playersEnvelope, errPlayers := sevenDaysToDieMapGetJSON(ctx, client, baseURL+"/api/player", req)
	if errPlayers != nil && errors.Is(errPlayers, fs.ErrNotExist) {
		playersEnvelope, errPlayers = sevenDaysToDieMapGetJSON(ctx, client, baseURL+"/api/getplayerslocation?offline=true", req)
	}
	if errPlayers != nil {
		return nil, fmt.Errorf("%w: read players: %w", errSevenDaysToDieMapUnavailable, errPlayers)
	}
	players, errDecodePlayers := decodeSevenDaysToDieMapPlayers(playersEnvelope.Data, SevenDaysToDieMapVector{
		X: config.MapSize.X, Y: config.MapSize.Y, Z: config.MapSize.Z,
	})
	if errDecodePlayers != nil {
		return nil, fmt.Errorf("%w: decode players: %w", errSevenDaysToDieMapUnavailable, errDecodePlayers)
	}

	snapshot := &SevenDaysToDieMapSnapshot{
		Enabled:    config.Enabled,
		TileSize:   int32(tileSize),
		MaxZoom:    int32(config.MaxZoom),
		MapSize:    SevenDaysToDieMapVector{X: config.MapSize.X, Y: config.MapSize.Y, Z: config.MapSize.Z},
		SourceTime: strings.TrimSpace(configEnvelope.Meta.ServerTime),
		Players:    players,
	}
	if req.IncludeTactical {
		querySevenDaysToDieTacticalOverlays(ctx, req, snapshot)
	}
	return snapshot, nil
}

func validSevenDaysToDieMapDimension(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0 && value <= 2_000_000
}

func querySevenDaysToDieTacticalOverlays(ctx context.Context, req SevenDaysToDieMapQueryRequest, snapshot *SevenDaysToDieMapSnapshot) {
	snapshot.NativeMarkerState = SevenDaysToDieWebAPIValueStateUnavailable
	snapshot.ClaimsState = SevenDaysToDieWebAPIValueStateUnavailable
	snapshot.BloodMoonState = SevenDaysToDieWebAPIValueStateUnavailable
	snapshot.HostileState = SevenDaysToDieWebAPIValueStateUnavailable
	snapshot.AnimalState = SevenDaysToDieWebAPIValueStateUnavailable

	discovery, errDiscovery := discoverSevenDaysToDieWebAPI(ctx, req.WorkingDirectory, req.TokenName, req.TokenSecret)
	if errDiscovery != nil {
		return
	}
	defer discovery.cancel()
	if discovery.connectionState != SevenDaysToDieWebAPIConnectionStateAvailable {
		return
	}

	snapshot.NativeMarkerState = querySevenDaysToDieMapOverlay(
		ctx, discovery, req, "/api/markers", sevenDaysToDieWebAPIEndpointMarkers,
		func(body []byte) error {
			markers, errDecode := decodeSevenDaysToDieNativeMarkers(body, snapshot.MapSize)
			if errDecode == nil {
				snapshot.NativeMarkers = markers
			}
			return errDecode
		},
	)
	snapshot.ClaimsState = querySevenDaysToDieMapOverlay(
		ctx, discovery, req, "/api/getlandclaims", sevenDaysToDieWebAPIEndpointLandClaims,
		func(body []byte) error {
			claims, errDecode := decodeSevenDaysToDieLandClaims(body, snapshot.MapSize)
			if errDecode == nil {
				snapshot.Claims = claims
			}
			return errDecode
		},
	)
	snapshot.BloodMoonState = querySevenDaysToDieMapOverlay(
		ctx, discovery, req, "/api/bloodmoon", sevenDaysToDieWebAPIEndpointBloodMoon,
		func(body []byte) error {
			bloodMoon, errDecode := decodeSevenDaysToDieMapBloodMoon(body)
			if errDecode == nil {
				snapshot.BloodMoon = bloodMoon
			}
			return errDecode
		},
	)
	snapshot.HostileState = querySevenDaysToDieMapOverlay(
		ctx, discovery, req, "/api/hostile", sevenDaysToDieWebAPIEndpointHostile,
		func(body []byte) error {
			entities, errDecode := decodeSevenDaysToDieMapEntities(body, snapshot.MapSize)
			if errDecode == nil {
				snapshot.Hostiles = entities
			}
			return errDecode
		},
	)
	snapshot.AnimalState = querySevenDaysToDieMapOverlay(
		ctx, discovery, req, "/api/animal", sevenDaysToDieWebAPIEndpointAnimal,
		func(body []byte) error {
			entities, errDecode := decodeSevenDaysToDieMapEntities(body, snapshot.MapSize)
			if errDecode == nil {
				snapshot.Animals = entities
			}
			return errDecode
		},
	)
}

func querySevenDaysToDieMapOverlay(
	ctx context.Context,
	discovery *sevenDaysToDieWebAPIDiscovery,
	req SevenDaysToDieMapQueryRequest,
	path string,
	endpoint sevenDaysToDieWebAPIEndpoint,
	decode func([]byte) error,
) SevenDaysToDieWebAPIValueState {
	if !discovery.resolver.supports(sevenDaysToDieOpenAPIOperation{path: path, method: http.MethodGet}) {
		if discovery.resolver.failed {
			return SevenDaysToDieWebAPIValueStateUnavailable
		}
		return SevenDaysToDieWebAPIValueStateUnsupported
	}
	queryCtx, cancel := context.WithTimeout(ctx, sevenDaysToDieWebAPIQueryTimeout)
	defer cancel()
	statusCode, body, errQuery := getSevenDaysToDieWebAPI(
		queryCtx, discovery.settings, endpoint, req.TokenName, req.TokenSecret,
	)
	if errQuery != nil {
		return SevenDaysToDieWebAPIValueStateUnavailable
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return SevenDaysToDieWebAPIValueStatePermissionDenied
	case http.StatusNotFound:
		return SevenDaysToDieWebAPIValueStateUnsupported
	case http.StatusOK:
	default:
		return SevenDaysToDieWebAPIValueStateUnavailable
	}
	if validateSevenDaysToDieMapJSONStructure(body) != nil || decode(body) != nil {
		return SevenDaysToDieWebAPIValueStateUnavailable
	}
	return SevenDaysToDieWebAPIValueStateAvailable
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
	errStructure := validateSevenDaysToDieMapJSONStructure(data)
	if errStructure != nil {
		return sevenDaysToDieMapEnvelope{}, errStructure
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

func decodeSevenDaysToDieMapPlayers(data json.RawMessage, mapSize SevenDaysToDieMapVector) ([]SevenDaysToDieMapPlayer, error) {
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
	if len(rawPlayers) > sevenDaysToDieMapItemLimit {
		return nil, errors.New("decode 7 Days to Die players: player count exceeds limit")
	}
	players := make([]SevenDaysToDieMapPlayer, 0, len(rawPlayers))
	for index, rawPlayer := range rawPlayers {
		name := strings.TrimSpace(rawPlayer.Name)
		if name == "" || rawPlayer.Position == nil {
			continue
		}
		if len(name) > sevenDaysToDieMapTextLimit || !validSevenDaysToDieMapPosition(
			rawPlayer.Position.X, rawPlayer.Position.Y, rawPlayer.Position.Z, mapSize,
		) {
			return nil, errors.New("decode 7 Days to Die players: invalid player")
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

func validateSevenDaysToDieMapJSONStructure(data []byte) error {
	type frame struct {
		delimiter    json.Delim
		entries      int
		expectingKey bool
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	frames := make([]frame, 0, sevenDaysToDieMapDepthLimit)
	tokens := 0
	rootComplete := false
	for {
		token, errToken := decoder.Token()
		if errors.Is(errToken, io.EOF) {
			if !rootComplete || len(frames) != 0 {
				return errors.New("decode 7 Days to Die map response: incomplete JSON")
			}
			return nil
		}
		if errToken != nil || rootComplete {
			return errors.New("decode 7 Days to Die map response: invalid JSON")
		}
		tokens++
		if tokens > sevenDaysToDieMapTokenLimit {
			return errors.New("decode 7 Days to Die map response: token count exceeds limit")
		}

		closing, isDelimiter := token.(json.Delim)
		if isDelimiter && (closing == '}' || closing == ']') {
			if len(frames) == 0 || (closing == '}' && frames[len(frames)-1].delimiter != '{') ||
				(closing == ']' && frames[len(frames)-1].delimiter != '[') ||
				(frames[len(frames)-1].delimiter == '{' && !frames[len(frames)-1].expectingKey) {
				return errors.New("decode 7 Days to Die map response: invalid container")
			}
			frames = frames[:len(frames)-1]
			rootComplete = len(frames) == 0
			continue
		}

		if len(frames) > 0 {
			current := &frames[len(frames)-1]
			if current.delimiter == '{' && current.expectingKey {
				key, validKey := token.(string)
				if !validKey || len(key) > sevenDaysToDieMapTextLimit {
					return errors.New("decode 7 Days to Die map response: invalid object key")
				}
				current.entries++
				current.expectingKey = false
				if current.entries > sevenDaysToDieMapContainerEntryLimit {
					return errors.New("decode 7 Days to Die map response: object entry count exceeds limit")
				}
				continue
			}
			if current.delimiter == '[' {
				current.entries++
				if current.entries > sevenDaysToDieMapContainerEntryLimit {
					return errors.New("decode 7 Days to Die map response: array item count exceeds limit")
				}
			} else {
				current.expectingKey = true
			}
		}

		if text, isText := token.(string); isText && len(text) > sevenDaysToDieMapTextLimit {
			return errors.New("decode 7 Days to Die map response: string exceeds limit")
		}
		if opening, opensContainer := token.(json.Delim); opensContainer && (opening == '{' || opening == '[') {
			if len(frames) == sevenDaysToDieMapDepthLimit {
				return errors.New("decode 7 Days to Die map response: depth exceeds limit")
			}
			frames = append(frames, frame{delimiter: opening, expectingKey: opening == '{'})
			continue
		}
		if len(frames) == 0 {
			rootComplete = true
		}
	}
}

func decodeSevenDaysToDieNativeMarkers(body []byte, mapSize SevenDaysToDieMapVector) ([]SevenDaysToDieMapMarker, error) {
	var envelope struct {
		Data []sevenDaysToDieNativeMarkerJSON `json:"data"`
	}
	errDecode := json.Unmarshal(body, &envelope)
	if errDecode != nil || envelope.Data == nil || len(envelope.Data) > sevenDaysToDieMapItemLimit {
		return nil, errors.New("decode 7 Days to Die native markers: invalid marker list")
	}
	markers := make([]SevenDaysToDieMapMarker, 0, len(envelope.Data))
	for _, rawMarker := range envelope.Data {
		id := strings.TrimSpace(rawMarker.ID)
		name := strings.TrimSpace(rawMarker.Name)
		if name == "" {
			name = "Native marker"
		}
		if uuid.Validate(id) != nil || len(name) > sevenDaysToDieMapTextLimit || rawMarker.X == nil || rawMarker.Y == nil ||
			math.Trunc(*rawMarker.X) != *rawMarker.X || math.Trunc(*rawMarker.Y) != *rawMarker.Y ||
			!validSevenDaysToDieMapPoint(*rawMarker.X, *rawMarker.Y, mapSize) {
			return nil, errors.New("decode 7 Days to Die native markers: invalid marker")
		}
		markers = append(markers, SevenDaysToDieMapMarker{
			ID: id, Name: name, Position: SevenDaysToDieMapVector{X: *rawMarker.X, Z: *rawMarker.Y},
		})
	}
	return markers, nil
}

func decodeSevenDaysToDieLandClaims(body []byte, mapSize SevenDaysToDieMapVector) ([]SevenDaysToDieLandClaim, error) {
	var envelope struct {
		Data sevenDaysToDieLandClaimsJSON `json:"data"`
	}
	errDecode := json.Unmarshal(body, &envelope)
	if errDecode != nil || envelope.Data.ClaimSize == nil || *envelope.Data.ClaimSize <= 0 ||
		*envelope.Data.ClaimSize > 10000 || envelope.Data.ClaimOwners == nil {
		return nil, errors.New("decode 7 Days to Die land claims: invalid claim list")
	}
	claims := make([]SevenDaysToDieLandClaim, 0)
	for _, owner := range envelope.Data.ClaimOwners {
		ownerID := strings.TrimSpace(owner.SteamID)
		ownerName := strings.TrimSpace(owner.PlayerName)
		if ownerID == "" || len(ownerID) > sevenDaysToDieMapTextLimit || owner.Active == nil ||
			len(ownerName) > sevenDaysToDieMapTextLimit {
			return nil, errors.New("decode 7 Days to Die land claims: invalid owner")
		}
		for _, rawClaim := range owner.Claims {
			if len(claims) == sevenDaysToDieMapItemLimit || rawClaim.X == nil || rawClaim.Y == nil || rawClaim.Z == nil ||
				!validSevenDaysToDieMapPosition(*rawClaim.X, *rawClaim.Y, *rawClaim.Z, mapSize) {
				return nil, errors.New("decode 7 Days to Die land claims: invalid claim")
			}
			claims = append(claims, SevenDaysToDieLandClaim{
				OwnerID: ownerID, OwnerName: ownerName, Active: *owner.Active,
				Position: SevenDaysToDieMapVector{X: *rawClaim.X, Y: *rawClaim.Y, Z: *rawClaim.Z}, Size: *envelope.Data.ClaimSize,
			})
		}
	}
	return claims, nil
}

func decodeSevenDaysToDieMapBloodMoon(body []byte) (*SevenDaysToDieBloodMoon, error) {
	var envelope sevenDaysToDieBloodMoonEnvelope
	errDecode := json.Unmarshal(body, &envelope)
	if errDecode != nil || envelope.Data.BloodMoonActive == nil || !validSevenDaysToDieGameTime(envelope.Data.GameTime) ||
		!validSevenDaysToDieGameTime(envelope.Data.NextBloodMoon) || !validSevenDaysToDieGameTime(envelope.Data.NextBloodMoonEnd) {
		return nil, errors.New("decode 7 Days to Die Blood Moon: invalid state")
	}
	return &SevenDaysToDieBloodMoon{
		GameTime: *gameTimeFromSevenDaysToDieJSON(envelope.Data.GameTime), Active: *envelope.Data.BloodMoonActive,
		NextBloodMoon:    *gameTimeFromSevenDaysToDieJSON(envelope.Data.NextBloodMoon),
		NextBloodMoonEnd: *gameTimeFromSevenDaysToDieJSON(envelope.Data.NextBloodMoonEnd),
	}, nil
}

func decodeSevenDaysToDieMapEntities(body []byte, mapSize SevenDaysToDieMapVector) ([]SevenDaysToDieMapEntity, error) {
	var envelope struct {
		Data []sevenDaysToDieMapEntityJSON `json:"data"`
	}
	errDecode := json.Unmarshal(body, &envelope)
	if errDecode != nil || envelope.Data == nil || len(envelope.Data) > sevenDaysToDieMapItemLimit {
		return nil, errors.New("decode 7 Days to Die entities: invalid entity list")
	}
	entities := make([]SevenDaysToDieMapEntity, 0, len(envelope.Data))
	for _, rawEntity := range envelope.Data {
		name := strings.TrimSpace(rawEntity.Name)
		id := rawJSONIdentifier(rawEntity.ID)
		if id == "" || len(id) > sevenDaysToDieMapTextLimit || name == "" || len(name) > sevenDaysToDieMapTextLimit ||
			rawEntity.Position == nil || rawEntity.Position.X == nil || rawEntity.Position.Y == nil || rawEntity.Position.Z == nil ||
			!validSevenDaysToDieMapPosition(*rawEntity.Position.X, *rawEntity.Position.Y, *rawEntity.Position.Z, mapSize) {
			return nil, errors.New("decode 7 Days to Die entities: invalid entity")
		}
		entities = append(entities, SevenDaysToDieMapEntity{
			Name: name, Position: SevenDaysToDieMapVector{
				X: *rawEntity.Position.X, Y: *rawEntity.Position.Y, Z: *rawEntity.Position.Z,
			},
		})
	}
	return entities, nil
}

func validSevenDaysToDieMapPoint(x float64, z float64, mapSize SevenDaysToDieMapVector) bool {
	return validSevenDaysToDieMapPosition(x, 0, z, mapSize)
}

func validSevenDaysToDieMapPosition(x float64, y float64, z float64, mapSize SevenDaysToDieMapVector) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0) && !math.IsNaN(y) && !math.IsInf(y, 0) &&
		!math.IsNaN(z) && !math.IsInf(z, 0) && math.Abs(x) <= mapSize.X/2 && math.Abs(z) <= mapSize.Z/2 &&
		y >= -1_000_000 && y <= 1_000_000
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
