package actions

import (
	"sync"
	"time"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// GameServerQueryTelemetryStatus describes the latest attempt to query a
// game server. Unknown values are deliberately represented by valid flags in
// GameServerQueryTelemetrySnapshot rather than zero values.
type GameServerQueryTelemetryStatus string

const (
	// GameServerQueryTelemetryStatusNotYetQueried means no query attempt has completed.
	GameServerQueryTelemetryStatusNotYetQueried GameServerQueryTelemetryStatus = "not_yet_queried"
	// GameServerQueryTelemetryStatusUnsupported means the game has no typed query integration.
	GameServerQueryTelemetryStatusUnsupported GameServerQueryTelemetryStatus = "unsupported"
	// GameServerQueryTelemetryStatusSuccess means the latest query returned authoritative data.
	GameServerQueryTelemetryStatusSuccess GameServerQueryTelemetryStatus = "success"
	// GameServerQueryTelemetryStatusFailure means the latest supported query attempt failed.
	GameServerQueryTelemetryStatusFailure GameServerQueryTelemetryStatus = "failure"
	// GameServerQueryTelemetryStatusUnavailable means the supported query was skipped because the server was offline.
	GameServerQueryTelemetryStatusUnavailable GameServerQueryTelemetryStatus = "unavailable"
)

// GameServerQueryTelemetrySnapshot is the most recent authoritative game
// query attempt for one server. It is safe for callers to retain and use.
// Numeric values are meaningful only when their corresponding Valid flag is
// true.
type GameServerQueryTelemetrySnapshot struct {
	Status        GameServerQueryTelemetryStatus
	QueryType     xylona.ServerQuery_Type
	CheckedAt     time.Time
	LastSuccessAt time.Time
	Duration      time.Duration
	DurationValid bool

	PlayerCount         uint32
	PlayerCountValid    bool
	PlayerCapacity      uint32
	PlayerCapacityValid bool

	PalworldServerFPS          float64
	PalworldServerFPSValid     bool
	PalworldFrameTimeMS        float64
	PalworldFrameTimeMSValid   bool
	PalworldUptimeSeconds      uint64
	PalworldUptimeSecondsValid bool
}

// GameServerQueryTelemetryProvider is implemented by Instance. Metrics
// consumers can use it without depending on the query cache's presentation
// payload or its fallback values.
type GameServerQueryTelemetryProvider interface {
	GetGameServerQueryTelemetry(gameServerID string) GameServerQueryTelemetrySnapshot
}

type gameServerQueryTelemetryStore struct {
	mu        sync.RWMutex
	snapshots map[string]GameServerQueryTelemetrySnapshot
}

// GetGameServerQueryTelemetry returns the latest telemetry for a server. A
// server with no completed query attempt is explicitly not-yet-queried.
func (inst *Instance) GetGameServerQueryTelemetry(gameServerID string) GameServerQueryTelemetrySnapshot {
	inst.queryTelemetry.mu.RLock()
	snapshot, ok := inst.queryTelemetry.snapshots[gameServerID]
	inst.queryTelemetry.mu.RUnlock()
	if !ok {
		return GameServerQueryTelemetrySnapshot{Status: GameServerQueryTelemetryStatusNotYetQueried}
	}
	return snapshot
}

func (inst *Instance) storeGameServerQueryTelemetry(gameServerID string, snapshot GameServerQueryTelemetrySnapshot) {
	inst.queryTelemetry.mu.Lock()
	if inst.queryTelemetry.snapshots == nil {
		inst.queryTelemetry.snapshots = make(map[string]GameServerQueryTelemetrySnapshot)
	}
	inst.queryTelemetry.snapshots[gameServerID] = snapshot
	inst.queryTelemetry.mu.Unlock()
}

func (inst *Instance) recordUnsupportedGameServerQuery(gameServerID string) {
	inst.storeGameServerQueryTelemetry(gameServerID, GameServerQueryTelemetrySnapshot{
		Status: GameServerQueryTelemetryStatusUnsupported,
	})
}

func (inst *Instance) recordFailedGameServerQuery(gameServerID string, queryType xylona.ServerQuery_Type, _ time.Time) {
	previous := inst.GetGameServerQueryTelemetry(gameServerID)
	inst.storeGameServerQueryTelemetry(gameServerID, GameServerQueryTelemetrySnapshot{
		Status:        GameServerQueryTelemetryStatusFailure,
		QueryType:     queryType,
		CheckedAt:     time.Now().UTC(),
		LastSuccessAt: previous.LastSuccessAt,
	})
}

func (inst *Instance) recordUnavailableGameServerQuery(gameServerID string, queryType xylona.ServerQuery_Type) {
	previous := inst.GetGameServerQueryTelemetry(gameServerID)
	inst.storeGameServerQueryTelemetry(gameServerID, GameServerQueryTelemetrySnapshot{
		Status:        GameServerQueryTelemetryStatusUnavailable,
		QueryType:     queryType,
		CheckedAt:     time.Now().UTC(),
		LastSuccessAt: previous.LastSuccessAt,
	})
}

func (inst *Instance) recordSuccessfulGameServerQuery(gameServerID string, queryType xylona.ServerQuery_Type, startedAt time.Time, result *xylona.ServerQuery) {
	now := time.Now().UTC()
	snapshot := GameServerQueryTelemetrySnapshot{
		Status:        GameServerQueryTelemetryStatusSuccess,
		QueryType:     queryType,
		CheckedAt:     now,
		LastSuccessAt: now,
		Duration:      time.Since(startedAt),
		DurationValid: true,
	}
	if result != nil {
		switch queryType {
		case xylona.ServerQuery_Minecraft:
			info := result.GetMinecraft()
			if info != nil {
				snapshot.PlayerCount = info.GetNumberOfPlayers()
				snapshot.PlayerCountValid = true
				snapshot.PlayerCapacity = info.GetMaxPlayers()
				snapshot.PlayerCapacityValid = true
			}
		case xylona.ServerQuery_Source:
			info := result.GetSource()
			if info != nil {
				snapshot.PlayerCount = info.GetPlayers()
				snapshot.PlayerCountValid = true
				snapshot.PlayerCapacity = info.GetMaxPlayers()
				snapshot.PlayerCapacityValid = true
			}
		case xylona.ServerQuery_Palworld:
			info := result.GetPalworld()
			if info != nil {
				snapshot.PlayerCount = info.GetPlayers()
				snapshot.PlayerCountValid = true
				snapshot.PlayerCapacity = info.GetMaxPlayers()
				snapshot.PlayerCapacityValid = true
				snapshot.PalworldServerFPS = info.GetServerFps()
				snapshot.PalworldServerFPSValid = true
				snapshot.PalworldFrameTimeMS = info.GetServerFrameTimeMs()
				snapshot.PalworldFrameTimeMSValid = true
				snapshot.PalworldUptimeSeconds = info.GetUptimeSeconds()
				snapshot.PalworldUptimeSecondsValid = true
			}
		}
	}
	inst.storeGameServerQueryTelemetry(gameServerID, snapshot)
}
