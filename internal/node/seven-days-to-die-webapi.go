package node

import (
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

// QuerySevenDaysToDieWebAPIStatus returns bounded diagnostics from the native
// WebAPI exposed by a managed 7 Days to Die server.
func (*Node) QuerySevenDaysToDieWebAPIStatus(ctx context.Context, req SevenDaysToDieWebAPIStatusQueryRequest) (*SevenDaysToDieWebAPIStatus, error) {
	errContext := ctx.Err()
	if errContext != nil {
		return nil, fmt.Errorf("node: query 7 Days to Die WebAPI status: %w", errContext)
	}
	settings, errSettings := readSevenDaysToDieWebAPISettings(req.WorkingDirectory)
	if errSettings != nil {
		//nolint:nilerr // Configuration failures are expected operational states for diagnostics.
		return &SevenDaysToDieWebAPIStatus{ConnectionState: SevenDaysToDieWebAPIConnectionStateMisconfigured}, nil
	}
	if !settings.enabled {
		return &SevenDaysToDieWebAPIStatus{ConnectionState: SevenDaysToDieWebAPIConnectionStateDashboardDisabled}, nil
	}

	statusCode, body, errDiscovery := getSevenDaysToDieWebAPI(ctx, settings, sevenDaysToDieWebAPIEndpointOpenAPI, req.TokenName, req.TokenSecret)
	if errDiscovery != nil {
		errContext = ctx.Err()
		if errContext != nil {
			return nil, fmt.Errorf("node: query 7 Days to Die WebAPI discovery: %w", errContext)
		}
		connectionState := SevenDaysToDieWebAPIConnectionStateUnreachable
		if errors.Is(errDiscovery, errSevenDaysToDieWebAPIResponseTooLarge) {
			connectionState = SevenDaysToDieWebAPIConnectionStateInvalidResponse
		}
		return &SevenDaysToDieWebAPIStatus{ConnectionState: connectionState}, nil
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &SevenDaysToDieWebAPIStatus{ConnectionState: SevenDaysToDieWebAPIConnectionStateAuthenticationDenied}, nil
	case http.StatusNotFound:
		return &SevenDaysToDieWebAPIStatus{ConnectionState: SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported}, nil
	case http.StatusOK:
	default:
		return &SevenDaysToDieWebAPIStatus{ConnectionState: SevenDaysToDieWebAPIConnectionStateUnreachable}, nil
	}

	var document sevenDaysToDieOpenAPI
	errYAML := yaml.Unmarshal(body, &document)
	if errYAML != nil {
		//nolint:nilerr // An unreadable upstream document is reported as a typed response state.
		return &SevenDaysToDieWebAPIStatus{ConnectionState: SevenDaysToDieWebAPIConnectionStateInvalidResponse}, nil
	}
	openAPIVersion := strings.TrimSpace(document.OpenAPI)
	if !strings.HasPrefix(openAPIVersion, "3.") {
		return &SevenDaysToDieWebAPIStatus{ConnectionState: SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported}, nil
	}
	apiVersion := strings.TrimSpace(document.Info.Version)
	if apiVersion == "" || len(apiVersion) > 128 {
		return &SevenDaysToDieWebAPIStatus{ConnectionState: SevenDaysToDieWebAPIConnectionStateInvalidResponse}, nil
	}

	status := &SevenDaysToDieWebAPIStatus{
		ConnectionState: SevenDaysToDieWebAPIConnectionStateAvailable,
		APIVersion:      apiVersion,
		Capabilities:    projectSevenDaysToDieWebAPICapabilities(document),
		WorldTimeState:  SevenDaysToDieWebAPIValueStateUnsupported,
		BloodMoonState:  SevenDaysToDieWebAPIValueStateUnsupported,
		ObservedAt:      time.Now().UTC(),
	}

	bloodMoonAdvertised := supportsSevenDaysToDieWebAPIOperations(document, sevenDaysToDieOpenAPIOperation{path: "/api/bloodmoon", method: http.MethodGet})
	if bloodMoonAdvertised {
		errBloodMoon := querySevenDaysToDieBloodMoon(ctx, settings, req, status)
		if errBloodMoon != nil {
			return nil, errBloodMoon
		}
	}
	if status.WorldTimeState == SevenDaysToDieWebAPIValueStateAvailable {
		return status, nil
	}

	serverStatsAdvertised := supportsSevenDaysToDieWebAPIOperations(document, sevenDaysToDieOpenAPIOperation{path: "/api/serverstats", method: http.MethodGet})
	if serverStatsAdvertised {
		errServerStats := querySevenDaysToDieServerStats(ctx, settings, req, status)
		if errServerStats != nil {
			return nil, errServerStats
		}
	}
	return status, nil
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
		Timeout: 10 * time.Second,
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
	switch endpoint {
	case sevenDaysToDieWebAPIEndpointOpenAPI:
		path = "/api/openapi/openapi.yaml"
	case sevenDaysToDieWebAPIEndpointBloodMoon:
		path = "/api/bloodmoon"
	case sevenDaysToDieWebAPIEndpointServerStats:
		path = "/api/serverstats"
	default:
		return 0, nil, errors.New("node: invalid 7 Days to Die WebAPI endpoint")
	}
	endpointURL := "http://127.0.0.1:" + strconv.FormatUint(settings.port, 10) + path
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if errRequest != nil {
		return 0, nil, fmt.Errorf("node: create 7 Days to Die WebAPI request: %w", errRequest)
	}
	setSevenDaysToDieMapHeaders(request, tokenName, tokenSecret)
	if endpoint == sevenDaysToDieWebAPIEndpointOpenAPI {
		request.Header.Set("Accept", "application/yaml, text/yaml, application/json")
	} else {
		request.Header.Set("Accept", "application/json")
	}
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

func projectSevenDaysToDieWebAPICapabilities(document sevenDaysToDieOpenAPI) SevenDaysToDieWebAPICapabilities {
	return SevenDaysToDieWebAPICapabilities{
		PlayerData: supportsSevenDaysToDieWebAPIOperations(document,
			sevenDaysToDieOpenAPIOperation{path: "/api/player", method: http.MethodGet}),
		RuntimeSettings: supportsSevenDaysToDieWebAPIOperations(document,
			sevenDaysToDieOpenAPIOperation{path: "/api/gameprefs", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/gamestats", method: http.MethodGet}),
		NativeLog: supportsSevenDaysToDieWebAPIOperations(document,
			sevenDaysToDieOpenAPIOperation{path: "/api/log", method: http.MethodGet}),
		WorldPopulation: supportsSevenDaysToDieWebAPIOperations(document,
			sevenDaysToDieOpenAPIOperation{path: "/api/serverstats", method: http.MethodGet}),
		HostileAndAnimalPositions: supportsSevenDaysToDieWebAPIOperations(document,
			sevenDaysToDieOpenAPIOperation{path: "/api/hostile", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/animal", method: http.MethodGet}),
		AccessControl: supportsSevenDaysToDieWebAPIOperations(document,
			sevenDaysToDieOpenAPIOperation{path: "/api/blacklist", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/blacklist/{id}", method: http.MethodPost},
			sevenDaysToDieOpenAPIOperation{path: "/api/blacklist/{id}", method: http.MethodDelete},
			sevenDaysToDieOpenAPIOperation{path: "/api/whitelist", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/whitelist/user/{id}", method: http.MethodPost},
			sevenDaysToDieOpenAPIOperation{path: "/api/whitelist/user/{id}", method: http.MethodDelete}),
		GamePermissions: supportsSevenDaysToDieWebAPIOperations(document,
			sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions", method: http.MethodGet},
			sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions/user/{id}", method: http.MethodPost},
			sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions/user/{id}", method: http.MethodDelete}),
		ReportedMods: supportsSevenDaysToDieWebAPIOperations(document,
			sevenDaysToDieOpenAPIOperation{path: "/api/mods", method: http.MethodGet}),
	}
}

func supportsSevenDaysToDieWebAPIOperations(document sevenDaysToDieOpenAPI, operations ...sevenDaysToDieOpenAPIOperation) bool {
	for _, operation := range operations {
		methods, pathFound := document.Paths[operation.path]
		if !pathFound {
			return false
		}
		_, methodFound := methods[strings.ToLower(operation.method)]
		if !methodFound {
			return false
		}
	}
	return true
}

func querySevenDaysToDieBloodMoon(
	ctx context.Context,
	settings sevenDaysToDieWebAPISettings,
	req SevenDaysToDieWebAPIStatusQueryRequest,
	status *SevenDaysToDieWebAPIStatus,
) error {
	statusCode, body, errQuery := getSevenDaysToDieWebAPI(ctx, settings, sevenDaysToDieWebAPIEndpointBloodMoon, req.TokenName, req.TokenSecret)
	if errQuery != nil {
		errContext := ctx.Err()
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
	ctx context.Context,
	settings sevenDaysToDieWebAPISettings,
	req SevenDaysToDieWebAPIStatusQueryRequest,
	status *SevenDaysToDieWebAPIStatus,
) error {
	statusCode, body, errQuery := getSevenDaysToDieWebAPI(ctx, settings, sevenDaysToDieWebAPIEndpointServerStats, req.TokenName, req.TokenSecret)
	if errQuery != nil {
		errContext := ctx.Err()
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
