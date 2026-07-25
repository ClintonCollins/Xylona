package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
)

const (
	palworldMapMaxResponseBodyLen int64 = 16 << 20
	palworldMapMaxActors                = 25_000

	// palworldMapQueryTimeout is deliberately far larger than the timeout used
	// for the small status endpoints. A populated world makes /game-data
	// serialize every actor, which routinely takes several seconds.
	palworldMapQueryTimeout = 15 * time.Second

	// palworldGameDataErrorBodyLen bounds how much of a rejected /game-data
	// response is read so the failure can be classified.
	palworldGameDataErrorBodyLen int64 = 4 << 10

	// palworldGameDataLaunchOption unlocks GET /v1/api/game-data. Palworld
	// disables the endpoint unless the dedicated server is started with it.
	palworldGameDataLaunchOption = "-enable-gamedata-api"
)

// PalworldMapActorKind is the sanitized category used by the live map.
type PalworldMapActorKind int

// PalworldMapActorKind values classify sanitized world actors.
const (
	PalworldMapActorKindUnknown PalworldMapActorKind = iota
	PalworldMapActorKindPlayer
	PalworldMapActorKindBase
	PalworldMapActorKindBaseWorker
	PalworldMapActorKindCompanionPal
	PalworldMapActorKindWildPal
	PalworldMapActorKindNPC
	PalworldMapActorKindOther
)

// PalworldMapActor contains only display-safe world actor fields. Palworld
// user IDs, platform IDs, IP addresses, and raw instance IDs are discarded
// while decoding and never leave the node.
type PalworldMapActor struct {
	Key         string
	Kind        PalworldMapActorKind
	Name        string
	GuildKey    string
	GuildName   string
	TrainerName string
	ClassName   string
	LocationX   float64
	LocationY   float64
	LocationZ   float64
	RotationZ   float64
	Level       uint32
	HP          uint32
	MaxHP       uint32
	Action      string
	AIAction    string
	Active      bool
}

// PalworldMapHealth contains display-safe operational telemetry from the
// official Palworld metrics endpoint.
type PalworldMapHealth struct {
	ServerFPS         float64
	ServerFrameTimeMS float64
	CurrentPlayers    uint32
	MaxPlayers        uint32
	UptimeSeconds     uint64
	BaseCampCount     uint32
	Days              uint32
}

// PalworldMapSnapshot is one sanitized live-world snapshot.
type PalworldMapSnapshot struct {
	SourceTime  string
	CollectedAt time.Time
	Source      string
	Partial     bool
	Truncated   bool
	// PartialReason explains, in operator-facing terms, why the world snapshot
	// was unavailable and the map fell back to player positions. It never
	// repeats server-supplied text because it also reaches public shared maps.
	PartialReason string
	// PartialDetail is the raw failure text for node-local logging only. It can
	// name the game server's address, so it must not be transported or shown.
	PartialDetail string
	Actors        []PalworldMapActor
	Health        *PalworldMapHealth
}

type palworldGameDataResponse struct {
	Time      string                          `json:"Time"`
	ActorData []palworldGameDataActorResponse `json:"ActorData"`
}

type palworldGameDataActorResponse struct {
	Type              string          `json:"Type"`
	UnitType          string          `json:"UnitType"`
	InstanceID        string          `json:"InstanceID"`
	NickName          string          `json:"NickName"`
	TrainerInstanceID string          `json:"TrainerInstanceID"`
	TrainerNickName   string          `json:"TrainerNickName"`
	GuildID           string          `json:"GuildID"`
	GuildName         string          `json:"GuildName"`
	Class             string          `json:"Class"`
	Action            string          `json:"Action"`
	AIAction          string          `json:"AI_Action"`
	LocationX         float64         `json:"LocationX"`
	LocationY         float64         `json:"LocationY"`
	LocationZ         float64         `json:"LocationZ"`
	RotationZ         float64         `json:"RotationZ"`
	Level             int64           `json:"level"`
	HP                int64           `json:"HP"`
	MaxHP             int64           `json:"MaxHP"`
	IsActive          json.RawMessage `json:"IsActive"`
}

type palworldMapPlayersResponse struct {
	Players []struct {
		Name      string  `json:"name"`
		UserID    string  `json:"userId"`
		LocationX float64 `json:"location_x"`
		LocationY float64 `json:"location_y"`
		Level     int64   `json:"level"`
	} `json:"players"`
}

// PalworldMap queries the Palworld 1.0 world actor snapshot. Older servers
// without /game-data fall back to the standard player list so the map remains
// useful while clearly reporting a partial source.
func PalworldMap(
	ctx context.Context,
	host string,
	port int,
	username string,
	password string,
) (*PalworldMapSnapshot, error) {
	client := &http.Client{Timeout: palworldMapQueryTimeout}
	return palworldMapWithClient(ctx, client, host, port, username, password)
}

func palworldMapWithClient(
	ctx context.Context,
	client *http.Client,
	host string,
	port int,
	username string,
	password string,
) (*PalworldMapSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return nil, errors.New("palworld map HTTP client is nil")
	}
	baseURL, errBaseURL := palworldBaseURL(host, port, username, password)
	if errBaseURL != nil {
		return nil, errBaseURL
	}

	type palworldMapMetricsResult struct {
		metrics palworldMetricsResponse
		err     error
	}
	metricsResultChannel := make(chan palworldMapMetricsResult, 1)
	go func() {
		var metrics palworldMetricsResponse
		errMetrics := getPalworldJSON(
			ctx,
			client,
			baseURL+"/metrics",
			username,
			password,
			&metrics,
		)
		metricsResultChannel <- palworldMapMetricsResult{metrics: metrics, err: errMetrics}
	}()

	var snapshot *PalworldMapSnapshot
	var gameData palworldGameDataResponse
	errGameData := getPalworldGameData(ctx, client, baseURL+"/game-data", username, password, &gameData)
	if errGameData == nil {
		snapshot = sanitizePalworldGameData(gameData, time.Now().UTC())
	} else {
		var players palworldMapPlayersResponse
		errPlayers := getPalworldJSON(
			ctx,
			client,
			baseURL+"/players",
			username,
			password,
			&players,
		)
		if errPlayers != nil {
			<-metricsResultChannel
			return nil, fmt.Errorf(
				"query palworld world actors: %w",
				errors.Join(errGameData, fmt.Errorf("player fallback: %w", errPlayers)),
			)
		}
		snapshot = sanitizePalworldPlayers(players, time.Now().UTC())
		snapshot.PartialReason = palworldMapPartialReason(errGameData)
		snapshot.PartialDetail = errGameData.Error()
	}

	metricsOutcome := <-metricsResultChannel
	if metricsOutcome.err == nil {
		snapshot.Health = sanitizePalworldMapHealth(metricsOutcome.metrics)
	}
	return snapshot, nil
}

// palworldGameDataFailure classifies why GET /game-data could not be used so
// the caller can turn it into operator guidance without echoing server text.
type palworldGameDataFailure struct {
	// statusCode is the rejected HTTP status, or zero when the request never
	// produced a response.
	statusCode int
	// disabled reports that Palworld answered with its GameData-API-disabled
	// rejection rather than a transport or protocol failure.
	disabled bool
	// decodeFailed reports that the server answered 200 with a body this
	// client could not parse.
	decodeFailed bool
	// oversize reports that the body reached the read cap, which truncates the
	// JSON and would otherwise be indistinguishable from a malformed body.
	oversize bool
	err      error
}

func (failure *palworldGameDataFailure) Error() string {
	if failure.statusCode != 0 {
		return fmt.Sprintf("palworld game-data returned HTTP %d", failure.statusCode)
	}
	return fmt.Sprintf("palworld game-data request failed: %v", failure.err)
}

func (failure *palworldGameDataFailure) Unwrap() error {
	return failure.err
}

// getPalworldGameData streams the world snapshot rather than buffering it. The
// decoded actors are already the largest allocation on this path, so holding
// the raw multi-megabyte body at the same time doubles peak memory for nothing.
func getPalworldGameData(
	ctx context.Context,
	client *http.Client,
	url string,
	username string,
	password string,
	destination *palworldGameDataResponse,
) error {
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errRequest != nil {
		return &palworldGameDataFailure{err: fmt.Errorf("create request: %w", errRequest)}
	}
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(username, password)

	response, errDo := client.Do(request)
	if errDo != nil {
		return &palworldGameDataFailure{err: fmt.Errorf("send request: %w", errDo)}
	}
	if response.StatusCode != http.StatusOK {
		detail, errDetail := io.ReadAll(io.LimitReader(response.Body, palworldGameDataErrorBodyLen))
		errClose := response.Body.Close()
		return &palworldGameDataFailure{
			statusCode: response.StatusCode,
			disabled:   palworldGameDataDisabled(string(detail)),
			err:        errors.Join(errDetail, errClose),
		}
	}

	limited := &io.LimitedReader{R: response.Body, N: palworldMapMaxResponseBodyLen + 1}
	errDecode := json.NewDecoder(limited).Decode(destination)
	if errDecode != nil {
		errClose := response.Body.Close()
		return &palworldGameDataFailure{
			decodeFailed: true,
			oversize:     limited.N <= 0,
			err:          errors.Join(fmt.Errorf("decode response: %w", errDecode), errClose),
		}
	}
	// Drain the short remainder so the transport can reuse the connection for
	// the next poll instead of reconnecting every five seconds.
	_, errDrain := io.Copy(io.Discard, io.LimitReader(response.Body, palworldGameDataErrorBodyLen))
	errClose := response.Body.Close()
	if errClose != nil {
		return &palworldGameDataFailure{err: errors.Join(errDrain, fmt.Errorf("close response: %w", errClose))}
	}
	return nil
}

func palworldGameDataDisabled(body string) bool {
	return strings.Contains(strings.ToLower(body), "gamedata api is not enabled")
}

// palworldMapPartialReason renders operator guidance for a failed world
// snapshot. It intentionally omits the server's own error text and address
// because the same reason is served to anonymous public map viewers.
func palworldMapPartialReason(errGameData error) string {
	const playersOnly = "the map is showing player positions only"

	var failure *palworldGameDataFailure
	if !errors.As(errGameData, &failure) {
		return "Palworld's world snapshot could not be read, so " + playersOnly + "."
	}
	switch {
	case failure.disabled:
		return "Palworld refused the world snapshot because its GameData API is disabled. Add " +
			palworldGameDataLaunchOption + " to this server's start arguments and restart it to map bases, Pals, and NPCs."
	case failure.statusCode == http.StatusNotFound:
		return "This server has no /v1/api/game-data endpoint (HTTP 404). Bases, Pals, and NPCs need a Palworld 1.0 or newer server started with " +
			palworldGameDataLaunchOption + "."
	case failure.statusCode == http.StatusUnauthorized || failure.statusCode == http.StatusForbidden:
		return fmt.Sprintf("Palworld rejected the world snapshot request (HTTP %d). Check this server's REST API credentials.", failure.statusCode)
	case failure.statusCode != 0:
		return fmt.Sprintf("Palworld returned HTTP %d for the world snapshot, so %s.", failure.statusCode, playersOnly)
	case failure.oversize:
		return fmt.Sprintf(
			"Palworld's world snapshot exceeded the %d MB Xylona will read, so %s.",
			palworldMapMaxResponseBodyLen>>20,
			playersOnly,
		)
	case failure.decodeFailed:
		return "Palworld's world snapshot could not be decoded, so " + playersOnly + "."
	default:
		return "Palworld's world snapshot request did not complete, so " + playersOnly + "."
	}
}

func sanitizePalworldGameData(response palworldGameDataResponse, collectedAt time.Time) *PalworldMapSnapshot {
	limit := len(response.ActorData)
	truncated := false
	if limit > palworldMapMaxActors {
		limit = palworldMapMaxActors
		truncated = true
	}

	actors := make([]PalworldMapActor, 0, limit)
	for i := 0; i < limit; i++ {
		rawActor := response.ActorData[i]
		actor, ok := sanitizePalworldGameDataActor(rawActor)
		if !ok {
			continue
		}
		actors = append(actors, actor)
	}
	sortPalworldMapActors(actors)
	return &PalworldMapSnapshot{
		SourceTime:  strings.TrimSpace(response.Time),
		CollectedAt: collectedAt,
		Source:      "game-data",
		Truncated:   truncated,
		Actors:      actors,
	}
}

func sanitizePalworldGameDataActor(rawActor palworldGameDataActorResponse) (PalworldMapActor, bool) {
	actorType := strings.TrimSpace(rawActor.Type)
	unitType := strings.TrimSpace(rawActor.UnitType)
	kind := palworldMapActorKind(actorType, unitType)
	if kind == PalworldMapActorKindUnknown {
		return PalworldMapActor{}, false
	}

	name := strings.TrimSpace(rawActor.NickName)
	if kind == PalworldMapActorKindBase {
		name = strings.TrimSpace(rawActor.GuildName)
	}
	if name == "" {
		name = strings.TrimSpace(rawActor.Class)
	}
	if name == "" {
		name = palworldMapActorFallbackName(kind)
	}

	identity := strings.TrimSpace(rawActor.InstanceID)
	if kind == PalworldMapActorKindBase {
		// PalBox actors have no instance ID, and one guild may own multiple
		// bases. Include the fixed world location so every palbox gets its own
		// stable marker key instead of collapsing to the shared guild ID.
		identity = strings.Join([]string{
			actorType,
			strings.TrimSpace(rawActor.GuildID),
			strings.TrimSpace(rawActor.Class),
			strconv.FormatFloat(rawActor.LocationX, 'f', 3, 64),
			strconv.FormatFloat(rawActor.LocationY, 'f', 3, 64),
			strconv.FormatFloat(rawActor.LocationZ, 'f', 3, 64),
		}, "|")
	}
	if identity == "" {
		identity = strings.TrimSpace(rawActor.GuildID)
	}
	if identity == "" {
		identity = strings.Join([]string{
			actorType,
			unitType,
			name,
			strconv.FormatFloat(rawActor.LocationX, 'f', 3, 64),
			strconv.FormatFloat(rawActor.LocationY, 'f', 3, 64),
		}, "|")
	}

	return PalworldMapActor{
		Key:         hashPalworldMapActorKey(identity),
		Kind:        kind,
		Name:        name,
		GuildKey:    hashPalworldMapGuildKey(rawActor.GuildID),
		GuildName:   strings.TrimSpace(rawActor.GuildName),
		TrainerName: strings.TrimSpace(rawActor.TrainerNickName),
		ClassName:   strings.TrimSpace(rawActor.Class),
		LocationX:   rawActor.LocationX,
		LocationY:   rawActor.LocationY,
		LocationZ:   rawActor.LocationZ,
		RotationZ:   rawActor.RotationZ,
		Level:       helpers.ClampUint32FromInt64(rawActor.Level),
		HP:          helpers.ClampUint32FromInt64(rawActor.HP),
		MaxHP:       helpers.ClampUint32FromInt64(rawActor.MaxHP),
		Action:      strings.TrimSpace(rawActor.Action),
		AIAction:    strings.TrimSpace(rawActor.AIAction),
		Active:      palworldMapActorActive(rawActor.IsActive),
	}, true
}

func sanitizePalworldMapHealth(metrics palworldMetricsResponse) *PalworldMapHealth {
	return &PalworldMapHealth{
		ServerFPS:         clampPalworldMapMetric(metrics.ServerFPS),
		ServerFrameTimeMS: clampPalworldMapMetric(metrics.ServerFrameTimeMS),
		CurrentPlayers:    helpers.ClampUint32FromInt64(metrics.CurrentPlayers),
		MaxPlayers:        helpers.ClampUint32FromInt64(metrics.MaxPlayers),
		UptimeSeconds:     clampUint64(metrics.UptimeSeconds),
		BaseCampCount:     helpers.ClampUint32FromInt64(metrics.BaseCampCount),
		Days:              helpers.ClampUint32FromInt64(metrics.Days),
	}
}

func sanitizePalworldPlayers(response palworldMapPlayersResponse, collectedAt time.Time) *PalworldMapSnapshot {
	actors := make([]PalworldMapActor, 0, len(response.Players))
	for _, player := range response.Players {
		name := strings.TrimSpace(player.Name)
		identity := strings.TrimSpace(player.UserID)
		if name == "" && identity == "" {
			continue
		}
		if name == "" {
			name = "Unnamed player"
		}
		if identity == "" {
			identity = strings.Join([]string{
				name,
				strconv.FormatFloat(player.LocationX, 'f', 3, 64),
				strconv.FormatFloat(player.LocationY, 'f', 3, 64),
			}, "|")
		}
		actors = append(actors, PalworldMapActor{
			Key:       hashPalworldMapActorKey(identity),
			Kind:      PalworldMapActorKindPlayer,
			Name:      name,
			LocationX: player.LocationX,
			LocationY: player.LocationY,
			Level:     helpers.ClampUint32FromInt64(player.Level),
			Active:    true,
		})
	}
	sortPalworldMapActors(actors)
	return &PalworldMapSnapshot{
		CollectedAt: collectedAt,
		Source:      "players",
		Partial:     true,
		Actors:      actors,
	}
}

func palworldMapActorKind(actorType string, unitType string) PalworldMapActorKind {
	if strings.EqualFold(actorType, "PalBox") {
		return PalworldMapActorKindBase
	}
	if !strings.EqualFold(actorType, "Character") {
		if actorType == "" {
			return PalworldMapActorKindUnknown
		}
		return PalworldMapActorKindOther
	}
	switch {
	case strings.EqualFold(unitType, "Player"):
		return PalworldMapActorKindPlayer
	case strings.EqualFold(unitType, "BaseCampPal"):
		return PalworldMapActorKindBaseWorker
	case strings.EqualFold(unitType, "OtomoPal"):
		return PalworldMapActorKindCompanionPal
	case strings.EqualFold(unitType, "WildPal"):
		return PalworldMapActorKindWildPal
	case strings.EqualFold(unitType, "NPC"):
		return PalworldMapActorKindNPC
	case unitType != "":
		return PalworldMapActorKindOther
	default:
		return PalworldMapActorKindUnknown
	}
}

func palworldMapActorFallbackName(kind PalworldMapActorKind) string {
	switch kind {
	case PalworldMapActorKindPlayer:
		return "Unnamed player"
	case PalworldMapActorKindBase:
		return "Unnamed base"
	case PalworldMapActorKindBaseWorker:
		return "Unnamed base worker"
	case PalworldMapActorKindCompanionPal:
		return "Unnamed companion Pal"
	case PalworldMapActorKindWildPal:
		return "Unnamed wild Pal"
	case PalworldMapActorKindNPC:
		return "Unnamed NPC"
	default:
		return "Unknown actor"
	}
}

func palworldMapActorActive(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return true
	}
	value = strings.Trim(value, `"`)
	active, errParse := strconv.ParseBool(value)
	if errParse != nil {
		return true
	}
	return active
}

func hashPalworldMapActorKey(identity string) string {
	hash := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(hash[:16])
}

func hashPalworldMapGuildKey(guildID string) string {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ""
	}
	return hashPalworldMapActorKey("palworld-map-guild\x00" + guildID)
}

func clampPalworldMapMetric(value float64) float64 {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func sortPalworldMapActors(actors []PalworldMapActor) {
	sort.SliceStable(actors, func(i int, j int) bool {
		if actors[i].Kind != actors[j].Kind {
			return actors[i].Kind < actors[j].Kind
		}
		if actors[i].Name != actors[j].Name {
			return actors[i].Name < actors[j].Name
		}
		return actors[i].Key < actors[j].Key
	})
}
