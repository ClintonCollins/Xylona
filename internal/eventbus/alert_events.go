package eventbus

import (
	"time"
)

// Alert event topics — server events.
const (
	TopicGameServerCrashed         = "game_server.crashed"
	TopicGameServerStatusChanged   = "game_server.status_changed"
	TopicGameServerCPUThreshold    = "game_server.cpu_threshold"
	TopicGameServerMemoryThreshold = "game_server.memory_threshold"
	TopicGameServerDiskThreshold   = "game_server.disk_threshold"
	TopicGameServerPlayerThreshold = "game_server.player_threshold"
)

// Alert event topics — node events.
const (
	TopicNodeCPUThreshold    = "node.cpu_threshold"
	TopicNodeMemoryThreshold = "node.memory_threshold"
	TopicNodeDiskThreshold   = "node.disk_threshold"
)

// ThresholdDirection indicates whether a threshold was entered or resolved.
type ThresholdDirection string

// ThresholdDirection values describe whether a threshold condition was entered
// or resolved.
const (
	ThresholdEntered  ThresholdDirection = "entered"
	ThresholdResolved ThresholdDirection = "resolved"
)

// ServerCrashedEvent is published when a game server process exits unexpectedly.
type ServerCrashedEvent struct {
	ServerID     string
	ServerNodeID string
	ExitCode     int
	Timestamp    time.Time
}

// StatusChangedEvent is published when a game server's status transitions.
// IntentionalStop distinguishes user-initiated stops from crashes/unexpected
// exits and is consumed by the auto-restart subscriber. ExitCode is set when
// NewStatus is OFFLINE and the process exited with a known code; zero
// otherwise (including graceful stops).
type StatusChangedEvent struct {
	ServerID           string
	ServerName         string
	ServerNodeID       string
	OldStatus          string
	NewStatus          string
	ExecutionID        string
	TransitionSequence uint64
	IntentionalStop    bool
	ExitCode           int
	ExitCodeKnown      bool
	Replayed           bool
	// OccurredAt is authoritative for remote/replayed events. Local publishers
	// may leave it zero, in which case immediate consumers use observation time.
	OccurredAt time.Time
}

// ThresholdEvent is published when a server metric crosses a threshold boundary.
type ThresholdEvent struct {
	ServerID     string
	ServerNodeID string
	CurrentValue float64
	Threshold    float64
	Direction    ThresholdDirection
}

// NodeThresholdEvent is published when a node-level metric crosses a threshold.
type NodeThresholdEvent struct {
	NodeID       string
	CurrentValue float64
	Threshold    float64
	Direction    ThresholdDirection
}
