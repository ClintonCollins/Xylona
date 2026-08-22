package node

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	sevenDaysToDieServerConfigName    = "serverconfig.xml"
	sevenDaysToDieWebAPIResponseLimit = 4 << 20
	sevenDaysToDieWebAPIQueryTimeout  = 10 * time.Second
)

var errSevenDaysToDieWebAPIResponseTooLarge = errors.New("node: 7 Days to Die WebAPI response is too large")

type sevenDaysToDieWebAPISettings struct {
	enabled bool
	port    uint64
}

type sevenDaysToDieServerSettingsXML struct {
	XMLName    xml.Name `xml:"ServerSettings"`
	Properties []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"property"`
}

type sevenDaysToDieWebAPIEndpoint uint8

const (
	sevenDaysToDieWebAPIEndpointOpenAPI sevenDaysToDieWebAPIEndpoint = iota
	sevenDaysToDieWebAPIEndpointBloodMoon
	sevenDaysToDieWebAPIEndpointServerStats
	sevenDaysToDieWebAPIEndpointPlayer
	sevenDaysToDieWebAPIEndpointMods
)

type sevenDaysToDieOpenAPI struct {
	OpenAPI string `yaml:"openapi"`
	Info    struct {
		Version string `yaml:"version"`
	} `yaml:"info"`
	Paths map[string]map[string]yaml.Node `yaml:"paths"`
}

type sevenDaysToDieOpenAPIOperation struct {
	path   string
	method string
}

type sevenDaysToDieOpenAPIResolver struct {
	ctx         context.Context
	settings    sevenDaysToDieWebAPISettings
	tokenName   string
	tokenSecret string
	document    sevenDaysToDieOpenAPI
	fragments   map[string]*sevenDaysToDieOpenAPI
	failed      bool
}

type sevenDaysToDieWebAPIDiscovery struct {
	ctx             context.Context
	cancel          context.CancelFunc
	settings        sevenDaysToDieWebAPISettings
	resolver        *sevenDaysToDieOpenAPIResolver
	connectionState SevenDaysToDieWebAPIConnectionState
	apiVersion      string
}

type sevenDaysToDieGameTimeJSON struct {
	Days    *int32 `json:"days"`
	Hours   *int32 `json:"hours"`
	Minutes *int32 `json:"minutes"`
}

type sevenDaysToDieBloodMoonEnvelope struct {
	Data struct {
		GameTime         *sevenDaysToDieGameTimeJSON `json:"gameTime"`
		BloodMoonActive  *bool                       `json:"bloodmoonActive"`
		NextBloodMoon    *sevenDaysToDieGameTimeJSON `json:"nextBloodmoon"`
		NextBloodMoonEnd *sevenDaysToDieGameTimeJSON `json:"nextBloodmoonEnd"`
	} `json:"data"`
}

type sevenDaysToDieServerStatsEnvelope struct {
	Data struct {
		GameTime *sevenDaysToDieGameTimeJSON `json:"gameTime"`
	} `json:"data"`
}

type sevenDaysToDiePlayersEnvelope struct {
	Data struct {
		Players []sevenDaysToDiePlayerJSON `json:"players"`
	} `json:"data"`
}

type sevenDaysToDieReportedModJSON struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Version     string `json:"version"`
}

// QuerySevenDaysToDieWebAPIStatus returns bounded diagnostics from the native
// WebAPI exposed by a managed 7 Days to Die server.
func (*Node) QuerySevenDaysToDieWebAPIStatus(ctx context.Context, req SevenDaysToDieWebAPIStatusQueryRequest) (*SevenDaysToDieWebAPIStatus, error) {
	discovery, errDiscovery := discoverSevenDaysToDieWebAPI(ctx, req.WorkingDirectory, req.TokenName, req.TokenSecret)
	if errDiscovery != nil {
		return nil, fmt.Errorf("node: query 7 Days to Die WebAPI status: %w", errDiscovery)
	}
	defer discovery.cancel()
	if discovery.connectionState != SevenDaysToDieWebAPIConnectionStateAvailable {
		return &SevenDaysToDieWebAPIStatus{ConnectionState: discovery.connectionState}, nil
	}
	status := &SevenDaysToDieWebAPIStatus{
		ConnectionState: SevenDaysToDieWebAPIConnectionStateAvailable,
		APIVersion:      discovery.apiVersion,
		Capabilities:    projectSevenDaysToDieWebAPICapabilities(discovery.resolver),
		WorldTimeState:  SevenDaysToDieWebAPIValueStateUnsupported,
		BloodMoonState:  SevenDaysToDieWebAPIValueStateUnsupported,
		ObservedAt:      time.Now().UTC(),
	}
	errContext := ctx.Err()
	if errContext != nil {
		return nil, fmt.Errorf("node: query 7 Days to Die WebAPI discovery: %w", errContext)
	}

	bloodMoonAdvertised := discovery.resolver.supports(sevenDaysToDieOpenAPIOperation{path: "/api/bloodmoon", method: http.MethodGet})
	if bloodMoonAdvertised {
		errBloodMoon := querySevenDaysToDieBloodMoon(discovery.ctx, ctx, discovery.settings, req, status)
		if errBloodMoon != nil {
			return nil, errBloodMoon
		}
	}
	if status.WorldTimeState == SevenDaysToDieWebAPIValueStateAvailable {
		return status, nil
	}

	serverStatsAdvertised := discovery.resolver.supports(sevenDaysToDieOpenAPIOperation{path: "/api/serverstats", method: http.MethodGet})
	if serverStatsAdvertised {
		errServerStats := querySevenDaysToDieServerStats(discovery.ctx, ctx, discovery.settings, req, status)
		if errServerStats != nil {
			return nil, errServerStats
		}
	}
	errContext = ctx.Err()
	if errContext != nil {
		return nil, fmt.Errorf("node: query 7 Days to Die WebAPI discovery: %w", errContext)
	}
	return status, nil
}

// QuerySevenDaysToDiePlayers returns the native management roster without
// exposing it through the broad game-server query path.
func (*Node) QuerySevenDaysToDiePlayers(ctx context.Context, req SevenDaysToDiePlayersQueryRequest) (*SevenDaysToDiePlayers, error) {
	discovery, errDiscovery := discoverSevenDaysToDieWebAPI(ctx, req.WorkingDirectory, req.TokenName, req.TokenSecret)
	if errDiscovery != nil {
		return nil, fmt.Errorf("node: query 7 Days to Die players: %w", errDiscovery)
	}
	defer discovery.cancel()
	result := &SevenDaysToDiePlayers{
		ConnectionState: discovery.connectionState,
		State:           SevenDaysToDieWebAPIValueStateUnavailable,
		Players:         make([]SevenDaysToDiePlayer, 0),
	}
	if discovery.connectionState != SevenDaysToDieWebAPIConnectionStateAvailable {
		return result, nil
	}
	advertised := discovery.resolver.supports(sevenDaysToDieOpenAPIOperation{path: "/api/player", method: http.MethodGet})
	if !advertised {
		errContext := ctx.Err()
		if errContext != nil {
			return nil, fmt.Errorf("node: query 7 Days to Die players: %w", errContext)
		}
		if discovery.resolver.failed {
			return result, nil
		}
		result.State = SevenDaysToDieWebAPIValueStateUnsupported
		return result, nil
	}
	state, body, errQuery := querySevenDaysToDieWebAPIResource(ctx, discovery, sevenDaysToDieWebAPIEndpointPlayer, req.TokenName, req.TokenSecret)
	if errQuery != nil {
		return nil, fmt.Errorf("node: query 7 Days to Die players: %w", errQuery)
	}
	result.State = state
	if state != SevenDaysToDieWebAPIValueStateAvailable {
		return result, nil
	}
	players, errDecode := decodeSevenDaysToDiePlayers(body)
	if errDecode != nil {
		result.State = SevenDaysToDieWebAPIValueStateUnavailable
		return result, nil
	}
	result.Players = players
	return result, nil
}

// QuerySevenDaysToDieReportedMods returns the game server's ephemeral loaded-mod list.
func (*Node) QuerySevenDaysToDieReportedMods(ctx context.Context, req SevenDaysToDieReportedModsQueryRequest) (*SevenDaysToDieReportedMods, error) {
	discovery, errDiscovery := discoverSevenDaysToDieWebAPI(ctx, req.WorkingDirectory, req.TokenName, req.TokenSecret)
	if errDiscovery != nil {
		return nil, fmt.Errorf("node: query 7 Days to Die reported mods: %w", errDiscovery)
	}
	defer discovery.cancel()
	result := &SevenDaysToDieReportedMods{
		ConnectionState: discovery.connectionState,
		State:           SevenDaysToDieWebAPIValueStateUnavailable,
		Mods:            make([]SevenDaysToDieReportedMod, 0),
	}
	if discovery.connectionState != SevenDaysToDieWebAPIConnectionStateAvailable {
		return result, nil
	}
	advertised := discovery.resolver.supports(sevenDaysToDieOpenAPIOperation{path: "/api/mods", method: http.MethodGet})
	if !advertised {
		errContext := ctx.Err()
		if errContext != nil {
			return nil, fmt.Errorf("node: query 7 Days to Die reported mods: %w", errContext)
		}
		if discovery.resolver.failed {
			return result, nil
		}
		result.State = SevenDaysToDieWebAPIValueStateUnsupported
		return result, nil
	}
	state, body, errQuery := querySevenDaysToDieWebAPIResource(ctx, discovery, sevenDaysToDieWebAPIEndpointMods, req.TokenName, req.TokenSecret)
	if errQuery != nil {
		return nil, fmt.Errorf("node: query 7 Days to Die reported mods: %w", errQuery)
	}
	result.State = state
	if state != SevenDaysToDieWebAPIValueStateAvailable {
		return result, nil
	}
	mods, errDecode := decodeSevenDaysToDieReportedMods(body)
	if errDecode != nil {
		result.State = SevenDaysToDieWebAPIValueStateUnavailable
		return result, nil
	}
	result.Mods = mods
	return result, nil
}

func discoverSevenDaysToDieWebAPI(
	ctx context.Context,
	workingDirectory string,
	tokenName string,
	tokenSecret string,
) (*sevenDaysToDieWebAPIDiscovery, error) {
	errContext := ctx.Err()
	if errContext != nil {
		return nil, fmt.Errorf("node: discover 7 Days to Die WebAPI: %w", errContext)
	}
	queryCtx, cancel := context.WithTimeout(ctx, sevenDaysToDieWebAPIQueryTimeout)
	discovery := &sevenDaysToDieWebAPIDiscovery{
		ctx:             queryCtx,
		cancel:          cancel,
		connectionState: SevenDaysToDieWebAPIConnectionStateMisconfigured,
	}
	settings, errSettings := readSevenDaysToDieWebAPISettings(workingDirectory)
	if errSettings != nil {
		return discovery, nil //nolint:nilerr // Invalid server settings are represented as a misconfigured connection state.
	}
	discovery.settings = settings
	if !settings.enabled {
		discovery.connectionState = SevenDaysToDieWebAPIConnectionStateDashboardDisabled
		return discovery, nil
	}
	statusCode, body, errQuery := getSevenDaysToDieWebAPI(queryCtx, settings, sevenDaysToDieWebAPIEndpointOpenAPI, tokenName, tokenSecret)
	if errQuery != nil {
		errContext = ctx.Err()
		if errContext != nil {
			cancel()
			return nil, fmt.Errorf("node: discover 7 Days to Die WebAPI: %w", errContext)
		}
		discovery.connectionState = SevenDaysToDieWebAPIConnectionStateUnreachable
		if errors.Is(errQuery, errSevenDaysToDieWebAPIResponseTooLarge) {
			discovery.connectionState = SevenDaysToDieWebAPIConnectionStateInvalidResponse
		}
		return discovery, nil
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		discovery.connectionState = SevenDaysToDieWebAPIConnectionStateAuthenticationDenied
		return discovery, nil
	case http.StatusNotFound:
		discovery.connectionState = SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported
		return discovery, nil
	case http.StatusOK:
	default:
		discovery.connectionState = SevenDaysToDieWebAPIConnectionStateUnreachable
		return discovery, nil
	}
	var document sevenDaysToDieOpenAPI
	errYAML := yaml.Unmarshal(body, &document)
	if errYAML != nil {
		discovery.connectionState = SevenDaysToDieWebAPIConnectionStateInvalidResponse
		return discovery, nil //nolint:nilerr // Invalid discovery data is represented as an invalid-response state.
	}
	if !strings.HasPrefix(strings.TrimSpace(document.OpenAPI), "3.") {
		discovery.connectionState = SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported
		return discovery, nil
	}
	apiVersion := strings.TrimSpace(document.Info.Version)
	if apiVersion == "" || len(apiVersion) > 128 {
		discovery.connectionState = SevenDaysToDieWebAPIConnectionStateInvalidResponse
		return discovery, nil
	}
	discovery.connectionState = SevenDaysToDieWebAPIConnectionStateAvailable
	discovery.apiVersion = apiVersion
	discovery.resolver = &sevenDaysToDieOpenAPIResolver{
		ctx:         queryCtx,
		settings:    settings,
		tokenName:   tokenName,
		tokenSecret: tokenSecret,
		document:    document,
		fragments:   make(map[string]*sevenDaysToDieOpenAPI),
	}
	return discovery, nil
}

func querySevenDaysToDieWebAPIResource(
	callerCtx context.Context,
	discovery *sevenDaysToDieWebAPIDiscovery,
	endpoint sevenDaysToDieWebAPIEndpoint,
	tokenName string,
	tokenSecret string,
) (SevenDaysToDieWebAPIValueState, []byte, error) {
	statusCode, body, errQuery := getSevenDaysToDieWebAPI(discovery.ctx, discovery.settings, endpoint, tokenName, tokenSecret)
	if errQuery != nil {
		errContext := callerCtx.Err()
		if errContext != nil {
			return SevenDaysToDieWebAPIValueStateUnavailable, nil, fmt.Errorf("query 7 Days to Die WebAPI resource: %w", errContext)
		}
		return SevenDaysToDieWebAPIValueStateUnavailable, nil, nil
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return SevenDaysToDieWebAPIValueStatePermissionDenied, nil, nil
	case http.StatusOK:
		return SevenDaysToDieWebAPIValueStateAvailable, body, nil
	default:
		return SevenDaysToDieWebAPIValueStateUnavailable, nil, nil
	}
}

func readSevenDaysToDieWebAPISettings(workingDirectory string) (sevenDaysToDieWebAPISettings, error) {
	values, errValues := readSevenDaysToDieServerSettings(workingDirectory)
	if errValues != nil {
		return sevenDaysToDieWebAPISettings{}, errValues
	}
	enabled, errEnabled := strconv.ParseBool(strings.TrimSpace(values["WebDashboardEnabled"]))
	if errEnabled != nil {
		return sevenDaysToDieWebAPISettings{}, errors.New("node: 7 Days to Die WebDashboardEnabled is invalid")
	}
	if !enabled {
		return sevenDaysToDieWebAPISettings{enabled: false}, nil
	}
	port, errPort := strconv.ParseUint(strings.TrimSpace(values["WebDashboardPort"]), 10, 16)
	if errPort != nil || port == 0 {
		return sevenDaysToDieWebAPISettings{}, errors.New("node: 7 Days to Die WebDashboardPort is invalid")
	}
	return sevenDaysToDieWebAPISettings{enabled: enabled, port: port}, nil
}

func readSevenDaysToDieServerSettings(workingDirectory string) (map[string]string, error) {
	trimmedDirectory := strings.TrimSpace(workingDirectory)
	if trimmedDirectory == "" {
		return nil, errors.New("node: 7 Days to Die working directory is empty")
	}
	configPath := filepath.Join(trimmedDirectory, sevenDaysToDieServerConfigName)
	data, errRead := os.ReadFile(configPath) // #nosec G304 -- fixed file under the tracked server directory.
	if errRead != nil {
		return nil, fmt.Errorf("node: read 7 Days to Die server config: %w", errRead)
	}
	var settingsXML sevenDaysToDieServerSettingsXML
	errXML := xml.Unmarshal(data, &settingsXML)
	if errXML != nil {
		return nil, fmt.Errorf("node: parse 7 Days to Die server config: %w", errXML)
	}
	values := make(map[string]string, len(settingsXML.Properties))
	for _, property := range settingsXML.Properties {
		values[property.Name] = property.Value
	}
	return values, nil
}

func sevenDaysToDieWebAPIHTTPClient() *http.Client {
	return &http.Client{
		Timeout: sevenDaysToDieWebAPIQueryTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func getSevenDaysToDieWebAPI(
	ctx context.Context,
	settings sevenDaysToDieWebAPISettings,
	endpoint sevenDaysToDieWebAPIEndpoint,
	tokenName string,
	tokenSecret string,
) (int, []byte, error) {
	var path string
	accept := "application/json"
	switch endpoint {
	case sevenDaysToDieWebAPIEndpointOpenAPI:
		path = "/api/openapi/openapi.yaml"
		accept = "application/yaml, text/yaml, application/json"
	case sevenDaysToDieWebAPIEndpointBloodMoon:
		path = "/api/bloodmoon"
	case sevenDaysToDieWebAPIEndpointServerStats:
		path = "/api/serverstats"
	case sevenDaysToDieWebAPIEndpointPlayer:
		path = "/api/player"
	case sevenDaysToDieWebAPIEndpointMods:
		path = "/api/mods"
	default:
		return 0, nil, errors.New("node: invalid 7 Days to Die WebAPI endpoint")
	}
	return getSevenDaysToDieWebAPIPath(ctx, settings, path, accept, tokenName, tokenSecret)
}

func getSevenDaysToDieOpenAPIFragment(
	ctx context.Context,
	settings sevenDaysToDieWebAPISettings,
	fileName string,
	tokenName string,
	tokenSecret string,
) (int, []byte, error) {
	return getSevenDaysToDieWebAPIPath(
		ctx,
		settings,
		"/api/OpenAPI/"+fileName,
		"application/yaml, text/yaml, application/json",
		tokenName,
		tokenSecret,
	)
}

func getSevenDaysToDieWebAPIPath(
	ctx context.Context,
	settings sevenDaysToDieWebAPISettings,
	path string,
	accept string,
	tokenName string,
	tokenSecret string,
) (int, []byte, error) {
	endpointURL := "http://127.0.0.1:" + strconv.FormatUint(settings.port, 10) + path
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if errRequest != nil {
		return 0, nil, fmt.Errorf("node: create 7 Days to Die WebAPI request: %w", errRequest)
	}
	setSevenDaysToDieMapHeaders(request, tokenName, tokenSecret)
	request.Header.Set("Accept", accept)
	response, errDo := sevenDaysToDieWebAPIHTTPClient().Do(request)
	if errDo != nil {
		return 0, nil, fmt.Errorf("node: query 7 Days to Die WebAPI: %w", errDo)
	}
	if response.StatusCode != http.StatusOK {
		errClose := response.Body.Close()
		if errClose != nil {
			return response.StatusCode, nil, fmt.Errorf("node: close 7 Days to Die WebAPI response: %w", errClose)
		}
		return response.StatusCode, []byte{}, nil
	}
	body, errRead := io.ReadAll(io.LimitReader(response.Body, sevenDaysToDieWebAPIResponseLimit+1))
	errClose := response.Body.Close()
	if errRead != nil || errClose != nil {
		return response.StatusCode, nil, fmt.Errorf("node: read 7 Days to Die WebAPI response: %w", errors.Join(errRead, errClose))
	}
	if len(body) > sevenDaysToDieWebAPIResponseLimit {
		return response.StatusCode, nil, errSevenDaysToDieWebAPIResponseTooLarge
	}
	return response.StatusCode, body, nil
}

func projectSevenDaysToDieWebAPICapabilities(resolver *sevenDaysToDieOpenAPIResolver) SevenDaysToDieWebAPICapabilities {
	return SevenDaysToDieWebAPICapabilities{
		PlayerData: resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: "/api/player", method: http.MethodGet}),
		RuntimeSettings: resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: "/api/gameprefs", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/gamestats", method: http.MethodGet}),
		NativeLog: resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: "/api/log", method: http.MethodGet}),
		WorldPopulation: resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: "/api/serverstats", method: http.MethodGet}),
		HostileAndAnimalPositions: resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: "/api/hostile", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/animal", method: http.MethodGet}),
		AccessControl: resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: "/api/blacklist", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/blacklist/{id}", method: http.MethodPost},
			sevenDaysToDieOpenAPIOperation{path: "/api/blacklist/{id}", method: http.MethodDelete},
			sevenDaysToDieOpenAPIOperation{path: "/api/whitelist", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/whitelist/user/{id}", method: http.MethodPost},
			sevenDaysToDieOpenAPIOperation{path: "/api/whitelist/user/{id}", method: http.MethodDelete}),
		GamePermissions: resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions/user/{id}", method: http.MethodPost},
			sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions/user/{id}", method: http.MethodDelete}),
		ReportedMods: resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: "/api/mods", method: http.MethodGet}),
	}
}

func (r *sevenDaysToDieOpenAPIResolver) supports(operations ...sevenDaysToDieOpenAPIOperation) bool {
	for _, operation := range operations {
		methods, pathFound := r.document.Paths[operation.path]
		if !pathFound {
			return false
		}
		_, methodFound := methods[strings.ToLower(operation.method)]
		if methodFound {
			continue
		}
		fileName, validReference := sevenDaysToDieOpenAPIReferenceFile(methods, operation.path)
		if !validReference {
			return false
		}
		fragment := r.fragment(fileName)
		if fragment == nil {
			return false
		}
		fragmentMethods, fragmentPathFound := fragment.Paths[operation.path]
		if !fragmentPathFound {
			return false
		}
		_, methodFound = fragmentMethods[strings.ToLower(operation.method)]
		if !methodFound {
			return false
		}
	}
	return true
}

func (r *sevenDaysToDieOpenAPIResolver) fragment(fileName string) *sevenDaysToDieOpenAPI {
	fragment, cached := r.fragments[fileName]
	if cached {
		return fragment
	}
	statusCode, body, errGet := getSevenDaysToDieOpenAPIFragment(r.ctx, r.settings, fileName, r.tokenName, r.tokenSecret)
	if errGet != nil || statusCode != http.StatusOK {
		r.failed = true
		r.fragments[fileName] = nil
		return nil
	}
	fragment = new(sevenDaysToDieOpenAPI)
	errYAML := yaml.Unmarshal(body, fragment)
	if errYAML != nil {
		r.failed = true
		fragment = nil
	}
	r.fragments[fileName] = fragment
	return fragment
}

func decodeSevenDaysToDiePlayers(body []byte) ([]SevenDaysToDiePlayer, error) {
	var envelope sevenDaysToDiePlayersEnvelope
	errDecode := json.Unmarshal(body, &envelope)
	if errDecode != nil {
		return nil, fmt.Errorf("decode 7 Days to Die players: %w", errDecode)
	}
	if envelope.Data.Players == nil {
		return nil, errors.New("decode 7 Days to Die players: missing player list")
	}
	players := make([]SevenDaysToDiePlayer, 0, len(envelope.Data.Players))
	for _, rawPlayer := range envelope.Data.Players {
		entityID := rawJSONIdentifier(rawPlayer.EntityID)
		platformID := strings.TrimSpace(rawPlayer.PlatformID.CombinedString)
		if platformID == "" {
			platformID = strings.TrimSpace(rawPlayer.SteamID)
		}
		crossPlatformID := strings.TrimSpace(rawPlayer.CrossPlatformIDObject.CombinedString)
		if crossPlatformID == "" {
			crossPlatformID = strings.TrimSpace(rawPlayer.CrossPlatformID)
		}
		actionID := platformID
		if actionID == "" {
			actionID = crossPlatformID
		}
		if actionID == "" {
			actionID = entityID
		}
		player := SevenDaysToDiePlayer{
			Name:            strings.TrimSpace(rawPlayer.Name),
			ActionID:        actionID,
			EntityID:        entityID,
			PlatformID:      platformID,
			CrossPlatformID: crossPlatformID,
			Online:          rawPlayer.Online,
			Ping:            rawPlayer.Ping,
			Level:           rawPlayer.Level,
			Health:          rawPlayer.Health,
			Stamina:         rawPlayer.Stamina,
			Score:           rawPlayer.Score,
			Deaths:          rawPlayer.Deaths,
		}
		if rawPlayer.Kills != nil {
			player.ZombieKills = rawPlayer.Kills.Zombies
			player.PlayerKills = rawPlayer.Kills.Players
		}
		if rawPlayer.Banned != nil {
			player.Banned = rawPlayer.Banned.Active
		}
		players = append(players, player)
	}
	return players, nil
}

func decodeSevenDaysToDieReportedMods(body []byte) ([]SevenDaysToDieReportedMod, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	opening, errOpening := decoder.Token()
	if errOpening != nil {
		return nil, fmt.Errorf("decode 7 Days to Die reported mods: invalid envelope: %w", errOpening)
	}
	if opening != json.Delim('{') {
		return nil, errors.New("decode 7 Days to Die reported mods: invalid envelope")
	}

	mods := make([]SevenDaysToDieReportedMod, 0)
	foundMods := false
	for decoder.More() {
		fieldToken, errField := decoder.Token()
		if errField != nil {
			return nil, fmt.Errorf("decode 7 Days to Die reported mods: read envelope field: %w", errField)
		}
		field, validField := fieldToken.(string)
		if !validField {
			return nil, errors.New("decode 7 Days to Die reported mods: invalid envelope field")
		}
		if field != "data" {
			var ignored json.RawMessage
			errIgnored := decoder.Decode(&ignored)
			if errIgnored != nil {
				return nil, fmt.Errorf("decode 7 Days to Die reported mods: skip envelope field: %w", errIgnored)
			}
			continue
		}
		if foundMods {
			return nil, errors.New("decode 7 Days to Die reported mods: duplicate mod list")
		}
		foundMods = true
		openingMods, errOpeningMods := decoder.Token()
		if errOpeningMods != nil {
			return nil, fmt.Errorf("decode 7 Days to Die reported mods: invalid mod list: %w", errOpeningMods)
		}
		if openingMods != json.Delim('[') {
			return nil, errors.New("decode 7 Days to Die reported mods: invalid mod list")
		}
		for decoder.More() {
			if len(mods) == SevenDaysToDieReportedModCountLimit {
				return nil, errors.New("decode 7 Days to Die reported mods: mod count exceeds limit")
			}
			var rawMod sevenDaysToDieReportedModJSON
			errMod := decoder.Decode(&rawMod)
			if errMod != nil {
				return nil, fmt.Errorf("decode 7 Days to Die reported mods: decode mod: %w", errMod)
			}
			mod := SevenDaysToDieReportedMod(rawMod)
			errMod = validateSevenDaysToDieReportedMod(mod)
			if errMod != nil {
				return nil, fmt.Errorf("decode 7 Days to Die reported mods: %w", errMod)
			}
			mods = append(mods, mod)
		}
		closingMods, errClosingMods := decoder.Token()
		if errClosingMods != nil {
			return nil, fmt.Errorf("decode 7 Days to Die reported mods: invalid mod list end: %w", errClosingMods)
		}
		if closingMods != json.Delim(']') {
			return nil, errors.New("decode 7 Days to Die reported mods: invalid mod list end")
		}
	}
	closing, errClosing := decoder.Token()
	if errClosing != nil {
		return nil, fmt.Errorf("decode 7 Days to Die reported mods: invalid envelope end: %w", errClosing)
	}
	if closing != json.Delim('}') {
		return nil, errors.New("decode 7 Days to Die reported mods: invalid envelope end")
	}
	if !foundMods {
		return nil, errors.New("decode 7 Days to Die reported mods: missing mod list")
	}
	var trailing json.RawMessage
	errTrailing := decoder.Decode(&trailing)
	if !errors.Is(errTrailing, io.EOF) {
		return nil, errors.New("decode 7 Days to Die reported mods: trailing data")
	}
	return mods, nil
}

func sevenDaysToDieOpenAPIReferenceFile(pathItem map[string]yaml.Node, operationPath string) (string, bool) {
	referenceNode, found := pathItem["$ref"]
	if !found || referenceNode.Kind != yaml.ScalarNode {
		return "", false
	}
	filePart, pointer, found := strings.Cut(referenceNode.Value, "#")
	if !found || !strings.HasPrefix(filePart, "./") {
		return "", false
	}
	fileName := strings.TrimPrefix(filePart, "./")
	if !filepath.IsLocal(fileName) || filepath.Base(fileName) != fileName ||
		strings.ContainsAny(fileName, `:/\?#%`) || !strings.HasSuffix(fileName, ".openapi.yaml") {
		return "", false
	}
	expectedPointer := "/paths/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(operationPath)
	return fileName, pointer == expectedPointer
}

func querySevenDaysToDieBloodMoon(
	queryCtx context.Context,
	callerCtx context.Context,
	settings sevenDaysToDieWebAPISettings,
	req SevenDaysToDieWebAPIStatusQueryRequest,
	status *SevenDaysToDieWebAPIStatus,
) error {
	statusCode, body, errQuery := getSevenDaysToDieWebAPI(queryCtx, settings, sevenDaysToDieWebAPIEndpointBloodMoon, req.TokenName, req.TokenSecret)
	if errQuery != nil {
		errContext := callerCtx.Err()
		if errContext != nil {
			return fmt.Errorf("node: query 7 Days to Die Blood Moon status: %w", errContext)
		}
		status.WorldTimeState = SevenDaysToDieWebAPIValueStateUnavailable
		status.BloodMoonState = SevenDaysToDieWebAPIValueStateUnavailable
		return nil
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		status.WorldTimeState = SevenDaysToDieWebAPIValueStatePermissionDenied
		status.BloodMoonState = SevenDaysToDieWebAPIValueStatePermissionDenied
		return nil
	case http.StatusNotFound:
		status.WorldTimeState = SevenDaysToDieWebAPIValueStateUnsupported
		status.BloodMoonState = SevenDaysToDieWebAPIValueStateUnsupported
		return nil
	case http.StatusOK:
	default:
		status.WorldTimeState = SevenDaysToDieWebAPIValueStateUnavailable
		status.BloodMoonState = SevenDaysToDieWebAPIValueStateUnavailable
		return nil
	}

	var envelope sevenDaysToDieBloodMoonEnvelope
	errDecode := json.Unmarshal(body, &envelope)
	if errDecode != nil || envelope.Data.BloodMoonActive == nil || !validSevenDaysToDieGameTime(envelope.Data.GameTime) ||
		!validSevenDaysToDieGameTime(envelope.Data.NextBloodMoon) || !validSevenDaysToDieGameTime(envelope.Data.NextBloodMoonEnd) {
		status.WorldTimeState = SevenDaysToDieWebAPIValueStateUnavailable
		status.BloodMoonState = SevenDaysToDieWebAPIValueStateUnavailable
		//nolint:nilerr // An invalid optional upstream body is reported as an unavailable value state.
		return nil
	}
	status.WorldTimeState = SevenDaysToDieWebAPIValueStateAvailable
	status.WorldTime = gameTimeFromSevenDaysToDieJSON(envelope.Data.GameTime)
	status.BloodMoonState = SevenDaysToDieWebAPIValueStateAvailable
	status.BloodMoonActive = envelope.Data.BloodMoonActive
	status.NextBloodMoon = gameTimeFromSevenDaysToDieJSON(envelope.Data.NextBloodMoon)
	status.NextBloodMoonEnd = gameTimeFromSevenDaysToDieJSON(envelope.Data.NextBloodMoonEnd)
	return nil
}

func querySevenDaysToDieServerStats(
	queryCtx context.Context,
	callerCtx context.Context,
	settings sevenDaysToDieWebAPISettings,
	req SevenDaysToDieWebAPIStatusQueryRequest,
	status *SevenDaysToDieWebAPIStatus,
) error {
	statusCode, body, errQuery := getSevenDaysToDieWebAPI(queryCtx, settings, sevenDaysToDieWebAPIEndpointServerStats, req.TokenName, req.TokenSecret)
	if errQuery != nil {
		errContext := callerCtx.Err()
		if errContext != nil {
			return fmt.Errorf("node: query 7 Days to Die server statistics: %w", errContext)
		}
		status.WorldTimeState = SevenDaysToDieWebAPIValueStateUnavailable
		return nil
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		status.WorldTimeState = SevenDaysToDieWebAPIValueStatePermissionDenied
		return nil
	case http.StatusNotFound:
		status.WorldTimeState = SevenDaysToDieWebAPIValueStateUnsupported
		return nil
	case http.StatusOK:
	default:
		status.WorldTimeState = SevenDaysToDieWebAPIValueStateUnavailable
		return nil
	}
	var envelope sevenDaysToDieServerStatsEnvelope
	errDecode := json.Unmarshal(body, &envelope)
	if errDecode != nil || !validSevenDaysToDieGameTime(envelope.Data.GameTime) {
		status.WorldTimeState = SevenDaysToDieWebAPIValueStateUnavailable
		//nolint:nilerr // An invalid optional upstream body is reported as an unavailable value state.
		return nil
	}
	status.WorldTimeState = SevenDaysToDieWebAPIValueStateAvailable
	status.WorldTime = gameTimeFromSevenDaysToDieJSON(envelope.Data.GameTime)
	return nil
}

func validSevenDaysToDieGameTime(gameTime *sevenDaysToDieGameTimeJSON) bool {
	return gameTime != nil && gameTime.Days != nil && gameTime.Hours != nil && gameTime.Minutes != nil &&
		*gameTime.Days >= 0 && *gameTime.Hours >= 0 && *gameTime.Hours < 24 && *gameTime.Minutes >= 0 && *gameTime.Minutes < 60
}

func gameTimeFromSevenDaysToDieJSON(gameTime *sevenDaysToDieGameTimeJSON) *SevenDaysToDieGameTime {
	return &SevenDaysToDieGameTime{Day: *gameTime.Days, Hour: *gameTime.Hours, Minute: *gameTime.Minutes}
}
