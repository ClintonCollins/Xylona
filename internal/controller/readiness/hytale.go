package readiness

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	// HytaleClientID is the OAuth client identifier used by Hytale dedicated servers.
	HytaleClientID = "hytale-server"
	// HytaleScope requests offline account access and server authentication.
	HytaleScope = "openid offline auth:server"

	hytaleOAuthBaseURL    = "https://oauth.accounts.hytale.com"
	hytaleAccountBaseURL  = "https://account-data.hytale.com"
	hytaleSessionsBaseURL = "https://sessions.hytale.com"

	// HytaleSessionTokenEnv is the launch-only environment variable name for the Hytale session token.
	// #nosec G101 -- This is an environment variable name, not a secret value.
	HytaleSessionTokenEnv = "HYTALE_SERVER_SESSION_TOKEN"
	// HytaleIdentityTokenEnv is the launch-only environment variable name for the Hytale identity token.
	// #nosec G101 -- This is an environment variable name, not a secret value.
	HytaleIdentityTokenEnv = "HYTALE_SERVER_IDENTITY_TOKEN"
)

// ErrHytaleRelinkRequired means the saved account link can no longer be refreshed.
var ErrHytaleRelinkRequired = errors.New("hytale account relink required")

// HytaleDeviceAuthorization contains the user-facing device-code login instructions.
type HytaleDeviceAuthorization struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               int64
	Interval                int64
}

// HytaleTokenSet is the OAuth token response needed during account linking or refresh.
type HytaleTokenSet struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// HytaleProfile is one selectable Hytale game profile for a linked account.
type HytaleProfile struct {
	UUID     string
	Username string
}

// HytaleGameSession contains launch-only credentials for one Hytale server start.
type HytaleGameSession struct {
	SessionToken  string
	IdentityToken string
	ExpiresAt     time.Time
}

// HytaleDevicePollStatus is the status returned while polling device authorization.
type HytaleDevicePollStatus string

const (
	// HytaleDevicePollPending means the user has not completed authorization yet.
	HytaleDevicePollPending HytaleDevicePollStatus = "pending"
	// HytaleDevicePollReady means authorization succeeded and profiles are available.
	HytaleDevicePollReady HytaleDevicePollStatus = "ready"
	// HytaleDevicePollDenied means the user rejected authorization.
	HytaleDevicePollDenied HytaleDevicePollStatus = "denied"
	// HytaleDevicePollExpired means the device-code flow expired.
	HytaleDevicePollExpired HytaleDevicePollStatus = "expired"
	// HytaleDevicePollSlowDown means the client should poll less often.
	HytaleDevicePollSlowDown HytaleDevicePollStatus = "slow_down"
)

// HytaleDevicePollResult is the public state returned to the frontend during linking.
type HytaleDevicePollResult struct {
	Status           HytaleDevicePollStatus
	Message          string
	Profiles         []HytaleProfile
	PollAfterSeconds int64
}

// HytaleClient provides the Hytale account and session API used by readiness.
type HytaleClient interface {
	StartDeviceAuth(ctx context.Context) (HytaleDeviceAuthorization, error)
	PollDeviceAuth(ctx context.Context, deviceCode string) (HytaleTokenSet, HytaleDevicePollStatus, error)
	ListProfiles(ctx context.Context, accessToken string) ([]HytaleProfile, error)
	RefreshOAuth(ctx context.Context, refreshToken string) (HytaleTokenSet, error)
	CreateGameSession(ctx context.Context, accessToken string, profileUUID string) (HytaleGameSession, error)
}

// HytaleHTTPClient talks to the public Hytale OAuth, account, and session endpoints.
type HytaleHTTPClient struct {
	httpClient      *http.Client
	oauthBaseURL    string
	accountBaseURL  string
	sessionsBaseURL string
}

// NewHytaleHTTPClient creates a Hytale API client with a conservative default timeout.
func NewHytaleHTTPClient(httpClient *http.Client) *HytaleHTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &HytaleHTTPClient{
		httpClient:      httpClient,
		oauthBaseURL:    hytaleOAuthBaseURL,
		accountBaseURL:  hytaleAccountBaseURL,
		sessionsBaseURL: hytaleSessionsBaseURL,
	}
}

// StartDeviceAuth starts the Hytale OAuth device-code flow.
func (c *HytaleHTTPClient) StartDeviceAuth(ctx context.Context) (HytaleDeviceAuthorization, error) {
	form := url.Values{}
	form.Set("client_id", HytaleClientID)
	form.Set("scope", HytaleScope)

	var out struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int64  `json:"expires_in"`
		Interval                int64  `json:"interval"`
	}
	errPost := c.postForm(ctx, c.oauthBaseURL+"/oauth2/device/auth", form, "", &out)
	if errPost != nil {
		return HytaleDeviceAuthorization{}, fmt.Errorf("start hytale device authorization: %w", errPost)
	}
	if strings.TrimSpace(out.DeviceCode) == "" || strings.TrimSpace(out.UserCode) == "" {
		return HytaleDeviceAuthorization{}, errors.New("hytale device auth response missing device or user code")
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 900
	}
	return HytaleDeviceAuthorization{
		DeviceCode:              out.DeviceCode,
		UserCode:                out.UserCode,
		VerificationURI:         out.VerificationURI,
		VerificationURIComplete: out.VerificationURIComplete,
		ExpiresIn:               out.ExpiresIn,
		Interval:                out.Interval,
	}, nil
}

// PollDeviceAuth exchanges a completed Hytale device-code authorization for tokens.
func (c *HytaleHTTPClient) PollDeviceAuth(ctx context.Context, deviceCode string) (HytaleTokenSet, HytaleDevicePollStatus, error) {
	form := url.Values{}
	form.Set("client_id", HytaleClientID)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)

	token, status, errToken := c.exchangeToken(ctx, form, true)
	return token, status, errToken
}

// RefreshOAuth refreshes a linked Hytale account token.
func (c *HytaleHTTPClient) RefreshOAuth(ctx context.Context, refreshToken string) (HytaleTokenSet, error) {
	form := url.Values{}
	form.Set("client_id", HytaleClientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	token, _, errToken := c.exchangeToken(ctx, form, false)
	if errToken != nil {
		return HytaleTokenSet{}, fmt.Errorf("refresh hytale OAuth token: %w", errToken)
	}
	return token, nil
}

// ListProfiles lists game profiles available to the linked Hytale account.
func (c *HytaleHTTPClient) ListProfiles(ctx context.Context, accessToken string) ([]HytaleProfile, error) {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, c.accountBaseURL+"/my-account/get-profiles", nil)
	if errReq != nil {
		return nil, fmt.Errorf("create hytale profiles request: %w", errReq)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	var out struct {
		Profiles []struct {
			UUID     string `json:"uuid"`
			Username string `json:"username"`
		} `json:"profiles"`
	}
	errDo := c.doJSON(req, &out)
	if errDo != nil {
		return nil, fmt.Errorf("list hytale profiles: %w", errDo)
	}

	profiles := make([]HytaleProfile, 0, len(out.Profiles))
	for _, profile := range out.Profiles {
		normalized := HytaleProfile{
			UUID:     strings.TrimSpace(profile.UUID),
			Username: strings.TrimSpace(profile.Username),
		}
		errValidate := validateHytaleProfile(normalized)
		if errValidate != nil {
			return nil, errValidate
		}
		profiles = append(profiles, normalized)
	}
	return profiles, nil
}

// CreateGameSession creates the launch-only credentials for a Hytale server start.
func (c *HytaleHTTPClient) CreateGameSession(ctx context.Context, accessToken string, profileUUID string) (HytaleGameSession, error) {
	body, errBody := json.Marshal(struct {
		UUID string `json:"uuid"`
	}{UUID: profileUUID})
	if errBody != nil {
		return HytaleGameSession{}, fmt.Errorf("marshal hytale game session request: %w", errBody)
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, c.sessionsBaseURL+"/game-session/new", bytes.NewReader(body))
	if errReq != nil {
		return HytaleGameSession{}, fmt.Errorf("create hytale game session request: %w", errReq)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	var out struct {
		SessionToken  string `json:"sessionToken"`
		IdentityToken string `json:"identityToken"`
		ExpiresAt     string `json:"expiresAt"`
	}
	errDo := c.doJSON(req, &out)
	if errDo != nil {
		return HytaleGameSession{}, fmt.Errorf("create hytale game session: %w", errDo)
	}
	if strings.TrimSpace(out.SessionToken) == "" || strings.TrimSpace(out.IdentityToken) == "" {
		return HytaleGameSession{}, errors.New("hytale game session response missing session or identity token")
	}

	var expiresAt time.Time
	if strings.TrimSpace(out.ExpiresAt) != "" {
		var errParse error
		expiresAt, errParse = time.Parse(time.RFC3339, out.ExpiresAt)
		if errParse != nil {
			return HytaleGameSession{}, fmt.Errorf("parse hytale game session expiry: %w", errParse)
		}
	}
	return HytaleGameSession{
		SessionToken:  out.SessionToken,
		IdentityToken: out.IdentityToken,
		ExpiresAt:     expiresAt,
	}, nil
}

func (c *HytaleHTTPClient) exchangeToken(ctx context.Context, form url.Values, devicePoll bool) (HytaleTokenSet, HytaleDevicePollStatus, error) {
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	status, errPost := c.postFormWithOAuthStatus(ctx, c.oauthBaseURL+"/oauth2/token", form, "", &out)
	if errPost != nil {
		if devicePoll && status == HytaleDevicePollPending {
			return HytaleTokenSet{}, status, nil
		}
		if devicePoll && status != "" {
			return HytaleTokenSet{}, status, nil
		}
		return HytaleTokenSet{}, "", errPost
	}
	if strings.TrimSpace(out.AccessToken) == "" || strings.TrimSpace(out.RefreshToken) == "" {
		return HytaleTokenSet{}, "", errors.New("hytale token response missing access or refresh token")
	}
	return HytaleTokenSet{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		ExpiresIn:    out.ExpiresIn,
	}, HytaleDevicePollReady, nil
}

func (c *HytaleHTTPClient) postForm(ctx context.Context, rawURL string, form url.Values, bearerToken string, out any) error {
	_, errPost := c.postFormWithOAuthStatus(ctx, rawURL, form, bearerToken, out)
	return errPost
}

func (c *HytaleHTTPClient) postFormWithOAuthStatus(ctx context.Context, rawURL string, form url.Values, bearerToken string, out any) (HytaleDevicePollStatus, error) {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if errReq != nil {
		return "", fmt.Errorf("create hytale form request: %w", errReq)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	// #nosec G704 -- Hytale requests use package-owned endpoint URLs.
	resp, errDo := c.httpClient.Do(req)
	if errDo != nil {
		return "", fmt.Errorf("send hytale form request: %w", errDo)
	}
	defer closeHytaleResponseBody(resp.Body, "hytale form response")

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		errDecode := decodeLimitedJSON(resp.Body, out)
		if errDecode != nil {
			return "", errDecode
		}
		return "", nil
	}

	status, errOAuth := decodeHytaleOAuthError(resp.Body)
	if errOAuth != nil {
		return "", errOAuth
	}
	if status == HytaleDevicePollPending || status == HytaleDevicePollSlowDown {
		return status, errors.New("hytale device authorization pending")
	}
	if status == HytaleDevicePollDenied || status == HytaleDevicePollExpired {
		return status, ErrHytaleRelinkRequired
	}
	return status, fmt.Errorf("hytale OAuth request failed with HTTP %d", resp.StatusCode)
}

func (c *HytaleHTTPClient) doJSON(req *http.Request, out any) error {
	// #nosec G704 -- Hytale requests use package-owned endpoint URLs.
	resp, errDo := c.httpClient.Do(req)
	if errDo != nil {
		return fmt.Errorf("send hytale JSON request: %w", errDo)
	}
	defer closeHytaleResponseBody(resp.Body, "hytale JSON response")

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		status, errOAuth := decodeHytaleOAuthError(resp.Body)
		if errOAuth != nil {
			return errOAuth
		}
		if status == HytaleDevicePollDenied || status == HytaleDevicePollExpired {
			return ErrHytaleRelinkRequired
		}
		return fmt.Errorf("hytale JSON request failed with HTTP %d", resp.StatusCode)
	}
	return decodeLimitedJSON(resp.Body, out)
}

func decodeLimitedJSON(reader io.Reader, out any) error {
	limited := io.LimitReader(reader, 1<<20)
	decoder := json.NewDecoder(limited)
	errDecode := decoder.Decode(out)
	if errDecode != nil {
		return fmt.Errorf("decode hytale JSON response: %w", errDecode)
	}
	return nil
}

func closeHytaleResponseBody(body io.Closer, label string) {
	errClose := body.Close()
	if errClose != nil {
		log.Warn().Err(errClose).Str("response", label).Msg("Failed to close Hytale response body")
	}
}

func decodeHytaleOAuthError(reader io.Reader) (HytaleDevicePollStatus, error) {
	var body struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	errDecode := decodeLimitedJSON(reader, &body)
	if errDecode != nil {
		return "", errDecode
	}

	switch body.Error {
	case "authorization_pending":
		return HytaleDevicePollPending, nil
	case "slow_down":
		return HytaleDevicePollSlowDown, nil
	case "access_denied":
		return HytaleDevicePollDenied, nil
	case "expired_token", "invalid_grant":
		return HytaleDevicePollExpired, nil
	default:
		if body.ErrorDescription != "" {
			return "", errors.New(body.ErrorDescription)
		}
		if body.Error != "" {
			return "", errors.New(body.Error)
		}
		return "", errors.New("hytale OAuth request failed")
	}
}

// HytaleDeviceAuthManager keeps in-progress device-code flows in memory.
type HytaleDeviceAuthManager struct {
	mu     sync.Mutex
	client HytaleClient
	flows  map[string]*hytaleDeviceFlow
	now    func() time.Time
}

type hytaleDeviceFlow struct {
	ID           string
	ServerID     string
	UserID       string
	DeviceCode   string
	UserCode     string
	VerifyURI    string
	VerifyFull   string
	ExpiresAt    time.Time
	Interval     time.Duration
	NextPollAt   time.Time
	RefreshToken string
	Profiles     []HytaleProfile
	Status       HytaleDevicePollStatus
}

// NewHytaleDeviceAuthManager creates an in-memory Hytale device-code flow manager.
func NewHytaleDeviceAuthManager(client HytaleClient) *HytaleDeviceAuthManager {
	if client == nil {
		client = NewHytaleHTTPClient(nil)
	}
	return &HytaleDeviceAuthManager{
		client: client,
		flows:  map[string]*hytaleDeviceFlow{},
		now:    time.Now,
	}
}

// Start creates a device-code flow tied to a server and initiating user.
func (m *HytaleDeviceAuthManager) Start(ctx context.Context, serverID string, userID string) (string, HytaleDeviceAuthorization, time.Time, error) {
	auth, errStart := m.client.StartDeviceAuth(ctx)
	if errStart != nil {
		return "", HytaleDeviceAuthorization{}, time.Time{}, fmt.Errorf("start hytale device authorization: %w", errStart)
	}

	id := helpers.GenerateUniqueID()
	now := m.now().UTC()
	expiresAt := now.Add(time.Duration(auth.ExpiresIn) * time.Second)
	interval := time.Duration(auth.Interval) * time.Second

	m.mu.Lock()
	defer m.mu.Unlock()
	m.flows[id] = &hytaleDeviceFlow{
		ID:         id,
		ServerID:   serverID,
		UserID:     userID,
		DeviceCode: auth.DeviceCode,
		UserCode:   auth.UserCode,
		VerifyURI:  auth.VerificationURI,
		VerifyFull: auth.VerificationURIComplete,
		ExpiresAt:  expiresAt,
		Interval:   interval,
		NextPollAt: now.Add(interval),
		Status:     HytaleDevicePollPending,
	}
	return id, auth, expiresAt, nil
}

// Poll advances a device-code flow and returns profile options once authorization succeeds.
func (m *HytaleDeviceAuthManager) Poll(ctx context.Context, flowID string, userID string) (HytaleDevicePollResult, error) {
	flow, errFlow := m.getFlow(flowID, userID)
	if errFlow != nil {
		return HytaleDevicePollResult{}, errFlow
	}

	now := m.now().UTC()
	if !flow.ExpiresAt.IsZero() && now.After(flow.ExpiresAt) {
		m.deleteFlow(flowID)
		return HytaleDevicePollResult{Status: HytaleDevicePollExpired, Message: "Hytale device authorization expired"}, nil
	}
	if flow.Status == HytaleDevicePollReady {
		return HytaleDevicePollResult{Status: HytaleDevicePollReady, Profiles: cloneHytaleProfiles(flow.Profiles)}, nil
	}
	if now.Before(flow.NextPollAt) {
		wait := flow.NextPollAt.Sub(now)
		wait = max(wait, time.Second)
		return HytaleDevicePollResult{
			Status:           HytaleDevicePollPending,
			Message:          "Waiting for Hytale account authorization",
			PollAfterSeconds: int64(wait.Round(time.Second).Seconds()),
		}, nil
	}

	token, status, errPoll := m.client.PollDeviceAuth(ctx, flow.DeviceCode)
	if errPoll != nil {
		return HytaleDevicePollResult{}, fmt.Errorf("poll hytale device authorization: %w", errPoll)
	}
	if status == HytaleDevicePollPending {
		m.bumpPoll(flowID, flow.Interval)
		return HytaleDevicePollResult{
			Status:           HytaleDevicePollPending,
			Message:          "Waiting for Hytale account authorization",
			PollAfterSeconds: int64(flow.Interval.Seconds()),
		}, nil
	}
	if status == HytaleDevicePollSlowDown {
		interval := flow.Interval + 5*time.Second
		m.bumpPoll(flowID, interval)
		return HytaleDevicePollResult{
			Status:           HytaleDevicePollPending,
			Message:          "Waiting for Hytale account authorization",
			PollAfterSeconds: int64(interval.Seconds()),
		}, nil
	}
	if status == HytaleDevicePollDenied || status == HytaleDevicePollExpired {
		m.deleteFlow(flowID)
		return HytaleDevicePollResult{Status: status, Message: "Hytale account authorization was not completed"}, nil
	}

	profiles, errProfiles := m.client.ListProfiles(ctx, token.AccessToken)
	if errProfiles != nil {
		return HytaleDevicePollResult{}, fmt.Errorf("list hytale profiles: %w", errProfiles)
	}
	m.markReady(flowID, token.RefreshToken, profiles)
	return HytaleDevicePollResult{Status: HytaleDevicePollReady, Profiles: profiles}, nil
}

// SelectProfile stores the chosen Hytale profile and refresh token for a server.
func (m *HytaleDeviceAuthManager) SelectProfile(database *db.Connection, flowID string, serverID string, userID string, profileUUID string) (HytaleProfile, error) {
	flow, errFlow := m.getFlow(flowID, userID)
	if errFlow != nil {
		return HytaleProfile{}, errFlow
	}
	if flow.ServerID != serverID {
		return HytaleProfile{}, errors.New("hytale device authorization flow does not belong to this server")
	}
	if flow.Status != HytaleDevicePollReady {
		return HytaleProfile{}, errors.New("hytale device authorization is not ready")
	}

	var selected HytaleProfile
	for _, profile := range flow.Profiles {
		if strings.EqualFold(profile.UUID, strings.TrimSpace(profileUUID)) {
			selected = profile
			break
		}
	}
	if selected.UUID == "" {
		return HytaleProfile{}, errors.New("selected hytale profile was not found")
	}

	errPersist := PersistHytaleAccount(database, serverID, selected, flow.RefreshToken, userID)
	if errPersist != nil {
		return HytaleProfile{}, fmt.Errorf("persist hytale account link: %w", errPersist)
	}
	m.deleteFlow(flowID)
	return selected, nil
}

func (m *HytaleDeviceAuthManager) getFlow(flowID string, userID string) (*hytaleDeviceFlow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	flow := m.flows[strings.TrimSpace(flowID)]
	if flow == nil {
		return nil, errors.New("hytale device authorization flow not found")
	}
	if flow.UserID != userID {
		return nil, errors.New("hytale device authorization flow does not belong to this user")
	}
	copied := *flow
	copied.Profiles = cloneHytaleProfiles(flow.Profiles)
	return &copied, nil
}

func (m *HytaleDeviceAuthManager) bumpPoll(flowID string, interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	flow := m.flows[flowID]
	if flow == nil {
		return
	}
	flow.NextPollAt = m.now().UTC().Add(interval)
}

func (m *HytaleDeviceAuthManager) markReady(flowID string, refreshToken string, profiles []HytaleProfile) {
	m.mu.Lock()
	defer m.mu.Unlock()
	flow := m.flows[flowID]
	if flow == nil {
		return
	}
	flow.RefreshToken = refreshToken
	flow.Profiles = cloneHytaleProfiles(profiles)
	flow.Status = HytaleDevicePollReady
}

func (m *HytaleDeviceAuthManager) deleteFlow(flowID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.flows, strings.TrimSpace(flowID))
}

func cloneHytaleProfiles(profiles []HytaleProfile) []HytaleProfile {
	if len(profiles) == 0 {
		return nil
	}
	cloned := make([]HytaleProfile, len(profiles))
	copy(cloned, profiles)
	return cloned
}

// HytaleAccountPublicData is the non-secret state shown for a linked Hytale account.
type HytaleAccountPublicData struct {
	ProfileUUID     string `json:"profileUuid"`
	ProfileUsername string `json:"profileUsername"`
}

// PersistHytaleAccount saves a linked Hytale profile and encrypted refresh token.
func PersistHytaleAccount(database *db.Connection, gameServerID string, profile HytaleProfile, refreshToken string, userID string) error {
	if database == nil {
		return errors.New("database is missing")
	}
	errProfile := validateHytaleProfile(profile)
	if errProfile != nil {
		return errProfile
	}
	trimmedRefreshToken := strings.TrimSpace(refreshToken)
	if trimmedRefreshToken == "" {
		return errors.New("hytale refresh token is required")
	}

	errSecret := database.SetGameServerSecret(
		gameServerID,
		db.GameServerSecretKindHytaleRefreshToken,
		db.GameServerSecretNameHytaleRefreshToken,
		trimmedRefreshToken,
		userID,
	)
	if errSecret != nil {
		return fmt.Errorf("store hytale refresh token: %w", errSecret)
	}

	data, errData := hytaleAccountData(profile)
	if errData != nil {
		return errData
	}
	errReadiness := database.UpsertGameServerReadiness(gameServerID, KindHytaleAccount, data, userID)
	if errReadiness != nil {
		return fmt.Errorf("store hytale account readiness: %w", errReadiness)
	}
	return nil
}

// ClearHytaleAccount removes a linked Hytale account from a server.
func ClearHytaleAccount(database *db.Connection, gameServerID string) error {
	if database == nil {
		return errors.New("database is missing")
	}
	errSecret := database.ClearGameServerSecret(
		gameServerID,
		db.GameServerSecretKindHytaleRefreshToken,
		db.GameServerSecretNameHytaleRefreshToken,
	)
	if errSecret != nil {
		return fmt.Errorf("clear hytale refresh token: %w", errSecret)
	}
	errReadiness := database.DeleteGameServerReadiness(gameServerID, KindHytaleAccount)
	if errReadiness != nil {
		return fmt.Errorf("clear hytale account readiness: %w", errReadiness)
	}
	return nil
}

// RequiresHytaleAccount reports whether this server needs Hytale account linking.
func RequiresHytaleAccount(gameServer *models.GameServer) bool {
	if gameServer == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(gameServer.GameID), "hytale")
}

// RequiresLaunchEnv reports whether readiness needs launch-only environment support.
func RequiresLaunchEnv(gameServer *models.GameServer) bool {
	return RequiresHytaleAccount(gameServer)
}

// HytaleAccountItem returns the public readiness state for Hytale account linking.
func HytaleAccountItem(ctx context.Context, database *db.Connection, gameServer *models.GameServer, client nodeclient.NodeClient, checkNode bool) (Item, error) {
	item := Item{
		Kind:     KindHytaleAccount,
		Required: true,
		Complete: false,
		Blocking: true,
		Message:  "Hytale account link required",
	}
	if database == nil {
		return item, errors.New("database is missing")
	}
	if gameServer == nil {
		return item, errors.New("game server is missing")
	}

	data, hasData, errData := loadHytaleAccountData(database, gameServer.ID)
	if errData != nil {
		return item, errData
	}
	if hasData {
		publicData, errPublicData := hytaleAccountData(HytaleProfile{
			UUID:     data.ProfileUUID,
			Username: data.ProfileUsername,
		})
		if errPublicData != nil {
			return item, errPublicData
		}
		item.PublicData = publicData
	}

	hasRefreshToken, errSecret := database.HasGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindHytaleRefreshToken,
		db.GameServerSecretNameHytaleRefreshToken,
	)
	if errSecret != nil {
		return item, fmt.Errorf("check hytale refresh token: %w", errSecret)
	}
	if !hasData || !hasRefreshToken {
		return item, nil
	}

	if checkNode {
		if client == nil {
			item.Message = "Target node is unavailable"
			return item, nil
		}
		caps, errCaps := client.GetRuntimeCapabilities(ctx)
		if errCaps != nil {
			return item, fmt.Errorf("target node runtime capabilities unavailable: %w", errCaps)
		}
		if !caps.LaunchEnv {
			item.Message = "Target node does not support launch-only Hytale credentials"
			return item, nil
		}
	}

	item.Complete = true
	item.Blocking = false
	if data.ProfileUsername != "" {
		item.Message = "Hytale profile linked: " + data.ProfileUsername
	} else {
		item.Message = "Hytale account linked"
	}
	return item, nil
}

// PrepareHytaleLaunchSecrets refreshes the linked account and appends launch-only Hytale tokens.
func PrepareHytaleLaunchSecrets(ctx context.Context, database *db.Connection, gameServer *models.GameServer, client HytaleClient, launchEnv map[string]string) (map[string]string, error) {
	prepared := cloneLaunchEnv(launchEnv)
	if !RequiresHytaleAccount(gameServer) {
		return prepared, nil
	}
	if database == nil {
		return nil, errors.New("database is missing")
	}
	if client == nil {
		client = NewHytaleHTTPClient(nil)
	}

	account, hasAccount, errAccount := loadHytaleAccountData(database, gameServer.ID)
	if errAccount != nil {
		return nil, fmt.Errorf("load hytale account link: %w", errAccount)
	}
	if !hasAccount {
		return nil, errors.New("hytale account link required")
	}

	refreshToken, ok, errRefreshToken := database.DecryptGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindHytaleRefreshToken,
		db.GameServerSecretNameHytaleRefreshToken,
	)
	if errRefreshToken != nil {
		return nil, fmt.Errorf("decrypt hytale refresh token: %w", errRefreshToken)
	}
	if !ok || strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("hytale account refresh token is missing")
	}

	tokens, errRefresh := client.RefreshOAuth(ctx, refreshToken)
	if errRefresh != nil {
		errClear := clearHytaleOnRelinkRequired(database, gameServer.ID, errRefresh)
		if errClear != nil {
			return nil, errClear
		}
		return nil, fmt.Errorf("refresh Hytale OAuth token: %w", errRefresh)
	}
	if strings.TrimSpace(tokens.AccessToken) == "" || strings.TrimSpace(tokens.RefreshToken) == "" {
		return nil, errors.New("hytale refresh response missing access or refresh token")
	}

	errPersist := database.SetGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindHytaleRefreshToken,
		db.GameServerSecretNameHytaleRefreshToken,
		tokens.RefreshToken,
		"",
	)
	if errPersist != nil {
		errDelete := database.DeleteGameServerReadiness(gameServer.ID, KindHytaleAccount)
		if errDelete != nil {
			return nil, errors.Join(fmt.Errorf("persist rotated Hytale refresh token: %w", errPersist), errDelete)
		}
		return nil, fmt.Errorf("persist rotated Hytale refresh token: %w", errPersist)
	}

	session, errSession := client.CreateGameSession(ctx, tokens.AccessToken, account.ProfileUUID)
	if errSession != nil {
		errClear := clearHytaleOnRelinkRequired(database, gameServer.ID, errSession)
		if errClear != nil {
			return nil, errClear
		}
		return nil, fmt.Errorf("create Hytale game session: %w", errSession)
	}
	if strings.TrimSpace(session.SessionToken) == "" || strings.TrimSpace(session.IdentityToken) == "" {
		return nil, errors.New("hytale game session response missing session or identity token")
	}

	errReserved := ensureHytaleLaunchEnvAvailable(prepared, HytaleSessionTokenEnv)
	if errReserved != nil {
		return nil, errReserved
	}
	errReserved = ensureHytaleLaunchEnvAvailable(prepared, HytaleIdentityTokenEnv)
	if errReserved != nil {
		return nil, errReserved
	}
	prepared[HytaleSessionTokenEnv] = session.SessionToken
	prepared[HytaleIdentityTokenEnv] = session.IdentityToken
	return prepared, nil
}

func clearHytaleOnRelinkRequired(database *db.Connection, gameServerID string, cause error) error {
	if !errors.Is(cause, ErrHytaleRelinkRequired) {
		return nil
	}
	errClear := ClearHytaleAccount(database, gameServerID)
	if errClear != nil {
		return errors.Join(ErrHytaleRelinkRequired, errClear)
	}
	return ErrHytaleRelinkRequired
}

func ensureHytaleLaunchEnvAvailable(launchEnv map[string]string, name string) error {
	for existingName := range launchEnv {
		if strings.EqualFold(existingName, name) {
			return fmt.Errorf("%s is reserved for Hytale launch credentials", name)
		}
	}
	return nil
}

func cloneLaunchEnv(launchEnv map[string]string) map[string]string {
	if len(launchEnv) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(launchEnv))
	maps.Copy(cloned, launchEnv)
	return cloned
}

func hytaleAccountData(profile HytaleProfile) (string, error) {
	data, errMarshal := json.Marshal(HytaleAccountPublicData{
		ProfileUUID:     strings.TrimSpace(profile.UUID),
		ProfileUsername: strings.TrimSpace(profile.Username),
	})
	if errMarshal != nil {
		return "", fmt.Errorf("marshal hytale account readiness data: %w", errMarshal)
	}
	return string(data), nil
}

func loadHytaleAccountData(database *db.Connection, gameServerID string) (HytaleAccountPublicData, bool, error) {
	if database == nil {
		return HytaleAccountPublicData{}, false, nil
	}
	state, errGet := database.GetGameServerReadiness(gameServerID, KindHytaleAccount)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return HytaleAccountPublicData{}, false, nil
		}
		return HytaleAccountPublicData{}, false, fmt.Errorf("load hytale account readiness: %w", errGet)
	}

	var data HytaleAccountPublicData
	errUnmarshal := json.Unmarshal([]byte(state.PublicData), &data)
	if errUnmarshal != nil {
		return HytaleAccountPublicData{}, true, fmt.Errorf("parse hytale account readiness data: %w", errUnmarshal)
	}
	if strings.TrimSpace(data.ProfileUUID) == "" {
		return HytaleAccountPublicData{}, true, errors.New("hytale account readiness is missing profile UUID")
	}
	return data, true, nil
}

func validateHytaleProfile(profile HytaleProfile) error {
	if strings.TrimSpace(profile.UUID) == "" {
		return errors.New("hytale profile UUID is required")
	}
	if len(profile.UUID) > 128 {
		return errors.New("hytale profile UUID is too long")
	}
	if len(profile.Username) > 128 {
		return errors.New("hytale profile username is too long")
	}
	return nil
}

// HytaleLaunchLocks serializes launch-time token refresh for a server.
type HytaleLaunchLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewHytaleLaunchLocks creates per-server launch locks for Hytale token refresh.
func NewHytaleLaunchLocks() *HytaleLaunchLocks {
	return &HytaleLaunchLocks{locks: map[string]*sync.Mutex{}}
}

// Lock locks Hytale launch-secret preparation for a single server until the returned unlock function is called.
func (l *HytaleLaunchLocks) Lock(serverID string) func() {
	l.mu.Lock()
	lock := l.locks[serverID]
	if lock == nil {
		lock = &sync.Mutex{}
		l.locks[serverID] = lock
	}
	l.mu.Unlock()

	lock.Lock()
	return lock.Unlock
}
