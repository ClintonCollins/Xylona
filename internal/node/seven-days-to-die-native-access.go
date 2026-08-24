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
	"net"
	"net/http"
	"net/url"
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

	sevenDaysToDieWebAPIEndpointOpenAPI         = "/api/openapi/openapi.yaml"
	sevenDaysToDieWebAPIEndpointBloodMoon       = "/api/bloodmoon"
	sevenDaysToDieWebAPIEndpointServerStats     = "/api/serverstats"
	sevenDaysToDieWebAPIEndpointPlayer          = "/api/player"
	sevenDaysToDieWebAPIEndpointMods            = "/api/mods"
	sevenDaysToDieWebAPIEndpointSandboxSettings = "/api/sandboxsettings"
	sevenDaysToDieWebAPIEndpointMarkers         = "/api/markers"
	sevenDaysToDieWebAPIEndpointLandClaims      = "/api/getlandclaims"
	sevenDaysToDieWebAPIEndpointHostile         = "/api/hostile"
	sevenDaysToDieWebAPIEndpointAnimal          = "/api/animal"
)

var errSevenDaysToDieWebAPIResponseTooLarge = errors.New("node: 7 Days to Die WebAPI response is too large")

type sevenDaysToDieNativeAccess struct {
	workingDirectory string
	tokenName        string
	tokenSecret      string
	client           *http.Client
	settings         map[string]string
	errSettings      error
	settingsLoaded   bool
}

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
	ctx       context.Context
	access    *sevenDaysToDieNativeAccess
	settings  sevenDaysToDieWebAPISettings
	document  sevenDaysToDieOpenAPI
	fragments map[string]*sevenDaysToDieOpenAPI
	failed    bool
}

type sevenDaysToDieWebAPIDiscovery struct {
	ctx             context.Context
	cancel          context.CancelFunc
	settings        sevenDaysToDieWebAPISettings
	resolver        *sevenDaysToDieOpenAPIResolver
	connectionState SevenDaysToDieWebAPIConnectionState
	apiVersion      string
}

func newSevenDaysToDieNativeAccess(workingDirectory string, tokenName string, tokenSecret string) *sevenDaysToDieNativeAccess {
	return &sevenDaysToDieNativeAccess{
		workingDirectory: workingDirectory,
		tokenName:        tokenName,
		tokenSecret:      tokenSecret,
		client: &http.Client{
			Timeout: sevenDaysToDieWebAPIQueryTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (a *sevenDaysToDieNativeAccess) serverSettings() (map[string]string, error) {
	if a.settingsLoaded {
		return a.settings, a.errSettings
	}
	a.settingsLoaded = true
	trimmedDirectory := strings.TrimSpace(a.workingDirectory)
	if trimmedDirectory == "" {
		a.errSettings = errors.New("node: 7 Days to Die working directory is empty")
		return nil, a.errSettings
	}
	configPath := filepath.Join(trimmedDirectory, sevenDaysToDieServerConfigName)
	data, errRead := os.ReadFile(configPath) // #nosec G304 -- fixed file under the tracked server directory.
	if errRead != nil {
		a.errSettings = fmt.Errorf("node: read 7 Days to Die server config: %w", errRead)
		return nil, a.errSettings
	}
	var settingsXML sevenDaysToDieServerSettingsXML
	errXML := xml.Unmarshal(data, &settingsXML)
	if errXML != nil {
		a.errSettings = fmt.Errorf("node: parse 7 Days to Die server config: %w", errXML)
		return nil, a.errSettings
	}
	a.settings = make(map[string]string, len(settingsXML.Properties))
	for _, property := range settingsXML.Properties {
		a.settings[property.Name] = property.Value
	}
	return a.settings, nil
}

func (a *sevenDaysToDieNativeAccess) webAPISettings() (sevenDaysToDieWebAPISettings, error) {
	values, errValues := a.serverSettings()
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
	port, errPort := sevenDaysToDieWebDashboardPort(values)
	if errPort != nil {
		return sevenDaysToDieWebAPISettings{}, errPort
	}
	return sevenDaysToDieWebAPISettings{enabled: true, port: port}, nil
}

func (a *sevenDaysToDieNativeAccess) mapPort() (uint64, error) {
	values, errValues := a.serverSettings()
	if errValues != nil {
		return 0, errValues
	}
	return sevenDaysToDieWebDashboardPort(values)
}

func sevenDaysToDieWebDashboardPort(values map[string]string) (uint64, error) {
	port, errPort := strconv.ParseUint(strings.TrimSpace(values["WebDashboardPort"]), 10, 16)
	if errPort != nil || port == 0 {
		return 0, errors.New("node: 7 Days to Die WebDashboardPort is invalid")
	}
	return port, nil
}

func (a *sevenDaysToDieNativeAccess) request(ctx context.Context, port uint64, path string, accept string) (*http.Request, error) {
	parsedPath, errPath := url.ParseRequestURI(path)
	if errPath != nil || parsedPath.IsAbs() || parsedPath.Host != "" || !strings.HasPrefix(parsedPath.Path, "/") {
		return nil, errors.New("node: invalid 7 Days to Die native path")
	}
	endpoint := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("127.0.0.1", strconv.FormatUint(port, 10)),
		Path:     parsedPath.Path,
		RawQuery: parsedPath.RawQuery,
	}
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if errRequest != nil {
		return nil, fmt.Errorf("create 7 Days to Die native request: %w", errRequest)
	}
	request.Header.Set("Accept", accept)
	if strings.TrimSpace(a.tokenName) != "" && strings.TrimSpace(a.tokenSecret) != "" {
		request.Header.Set("X-SDTD-API-TOKENNAME", a.tokenName)
		request.Header.Set("X-SDTD-API-SECRET", a.tokenSecret)
	}
	return request, nil
}

func (a *sevenDaysToDieNativeAccess) mapJSON(ctx context.Context, path string) (sevenDaysToDieMapEnvelope, error) {
	port, errPort := a.mapPort()
	if errPort != nil {
		return sevenDaysToDieMapEnvelope{}, errPort
	}
	request, errRequest := a.request(ctx, port, path, "application/json, image/png")
	if errRequest != nil {
		return sevenDaysToDieMapEnvelope{}, fmt.Errorf("node: create 7 Days to Die map request: %w", errRequest)
	}
	response, errDo := a.client.Do(request)
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

func (a *sevenDaysToDieNativeAccess) mapTile(ctx context.Context, path string) ([]byte, error) {
	port, errPort := a.mapPort()
	if errPort != nil {
		return nil, errPort
	}
	request, errRequest := a.request(ctx, port, path, "application/json, image/png")
	if errRequest != nil {
		return nil, fmt.Errorf("node: create 7 Days to Die tile request: %w", errRequest)
	}
	response, errDo := a.client.Do(request)
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

func (a *sevenDaysToDieNativeAccess) getWebAPI(
	ctx context.Context,
	settings sevenDaysToDieWebAPISettings,
	path string,
) (int, []byte, error) {
	accept := "application/json"
	if path == sevenDaysToDieWebAPIEndpointOpenAPI {
		accept = "application/yaml, text/yaml, application/json"
	}
	return a.getWebAPIPath(ctx, settings, path, accept)
}

func (a *sevenDaysToDieNativeAccess) getOpenAPIFragment(
	ctx context.Context,
	settings sevenDaysToDieWebAPISettings,
	fileName string,
) (int, []byte, error) {
	return a.getWebAPIPath(
		ctx,
		settings,
		"/api/OpenAPI/"+fileName,
		"application/yaml, text/yaml, application/json",
	)
}

func (a *sevenDaysToDieNativeAccess) getWebAPIPath(
	ctx context.Context,
	settings sevenDaysToDieWebAPISettings,
	path string,
	accept string,
) (int, []byte, error) {
	request, errRequest := a.request(ctx, settings.port, path, accept)
	if errRequest != nil {
		return 0, nil, fmt.Errorf("node: create 7 Days to Die WebAPI request: %w", errRequest)
	}
	response, errDo := a.client.Do(request)
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

func (a *sevenDaysToDieNativeAccess) discover(ctx context.Context) (*sevenDaysToDieWebAPIDiscovery, error) {
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
	settings, errSettings := a.webAPISettings()
	if errSettings != nil {
		return discovery, nil //nolint:nilerr // Invalid server settings are represented as a misconfigured connection state.
	}
	discovery.settings = settings
	if !settings.enabled {
		discovery.connectionState = SevenDaysToDieWebAPIConnectionStateDashboardDisabled
		return discovery, nil
	}
	statusCode, body, errQuery := a.getWebAPI(queryCtx, settings, sevenDaysToDieWebAPIEndpointOpenAPI)
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
		ctx:       queryCtx,
		access:    a,
		settings:  settings,
		document:  document,
		fragments: make(map[string]*sevenDaysToDieOpenAPI),
	}
	return discovery, nil
}

func (a *sevenDaysToDieNativeAccess) queryWebAPIResource(
	callerCtx context.Context,
	discovery *sevenDaysToDieWebAPIDiscovery,
	endpoint string,
) (SevenDaysToDieWebAPIValueState, []byte, error) {
	statusCode, body, errQuery := a.getWebAPI(discovery.ctx, discovery.settings, endpoint)
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
	statusCode, body, errGet := r.access.getOpenAPIFragment(r.ctx, r.settings, fileName)
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
