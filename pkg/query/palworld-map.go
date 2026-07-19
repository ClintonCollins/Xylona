package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// PalworldMapSnapshot is one sanitized live-world snapshot.
type PalworldMapSnapshot struct {
	SourceTime  string
	CollectedAt time.Time
	Source      string
	Partial     bool
	Truncated   bool
	Actors      []PalworldMapActor
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
	client := &http.Client{Timeout: palworldQueryTimeout}
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

	var gameData palworldGameDataResponse
	errGameData := getPalworldJSONWithLimit(
		ctx,
		client,
		baseURL+"/game-data",
		username,
		password,
		&gameData,
		palworldMapMaxResponseBodyLen,
	)
	if errGameData == nil {
		return sanitizePalworldGameData(gameData, time.Now().UTC()), nil
	}

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
		return nil, fmt.Errorf(
			"query palworld world actors: %w",
			errors.Join(errGameData, fmt.Errorf("player fallback: %w", errPlayers)),
		)
	}
	return sanitizePalworldPlayers(players, time.Now().UTC()), nil
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
