// Package node is the thin layer the Xylona node binary will expose. It owns
// process supervision, file operations, and host metrics for a single host.
// The package contains no HTTP/proto types; it is consumed by the controller
// in-process and (in later steps) wrapped by a gRPC implementation.
package node

import (
	"errors"
	"strings"
	"time"
)

// Sentinel errors returned by node operations.
var (
	// ErrInvalidPath is returned when a relative path escapes the resolved root.
	ErrInvalidPath = errors.New("node: invalid path")
	// ErrProtectedPath is returned when a write targets a protected file path.
	// pkg/node never sets this directly; callers (the controller) may layer
	// protected-path policy on top of node operations.
	ErrProtectedPath = errors.New("node: path is protected")
)

// defaultStopTimeout mirrors supervisor's default graceful stop window.
const defaultStopTimeout = 15 * time.Second

// ProcessConfig describes a process to launch on the node. It is the
// transport-agnostic input for StartProcess and is translated into a
// supervisor.PreparedCommand internally.
type ProcessConfig struct {
	ID               string
	Name             string
	BaseCommand      string
	Args             []string
	WorkingDirectory string
	User             string
	NodeID           string
	ServiceID        string
	StopTimeout      time.Duration

	// InputTelnet, when non-zero, configures the process to receive console
	// input over telnet (used by games like 7 Days to Die that don't accept
	// stdin). Ignored for remote nodes until the node-side supervisor gains
	// telnet support — the current default is stdin.
	InputTelnet *TelnetInput

	// InternalCommand marks the process as an "internal" supervisor command
	// (a Go function that runs as part of install/update rather than a
	// shell process). Only meaningful for the embedded in-process path;
	// ignored by remote nodes since Go-based internal installers run in
	// the controller's process space.
	InternalCommand bool
	// InternalGameServerID is used by the supervisor to reconstruct the
	// game server context for internal commands. Only set when
	// InternalCommand is true.
	InternalGameServerID string
	// InternalGameID is the game ID associated with the internal command.
	InternalGameID string
	// InternalGameServer is a direct pass-through for the model used by
	// internal commands. When set, the in-process client skips the DB
	// lookup by InternalGameServerID. Ignored by remote nodes (internal
	// commands run as Go functions in the controller process space).
	// Typed as interface{} to avoid pulling the models package into
	// pkg/node; the in-process client type-asserts.
	InternalGameServer any
}

// TelnetInput configures telnet-based console input for a process.
type TelnetInput struct {
	Port     int
	Password string
}

// normalize returns a copy of the config with whitespace trimmed from the base
// command, args duplicated, and the stop timeout defaulted.
func (p ProcessConfig) normalize() ProcessConfig {
	out := p
	out.BaseCommand = strings.TrimSpace(p.BaseCommand)
	if len(p.Args) > 0 {
		out.Args = append([]string(nil), p.Args...)
	}
	if out.StopTimeout <= 0 {
		out.StopTimeout = defaultStopTimeout
	}
	return out
}

// FileEntry describes a single directory entry returned by ListFiles.
type FileEntry struct {
	Name         string
	Size         int64
	IsDirectory  bool
	LastModified time.Time
}

// ProtectionPolicy is the per-request game-server context the node uses to
// run the same protected-path check the controller runs. Both fields may be
// empty, in which case the node skips the check (the controller only
// populates this for game-server-bound write requests).
type ProtectionPolicy struct {
	// ServerExecutable is the game's configured server executable (e.g.
	// "server.jar"). Writes targeting this path are rejected.
	ServerExecutable string
	// BaseCommand is the game's configured launch command (e.g. "./run.sh"
	// on Linux). Writes targeting this path are rejected when BaseCommand
	// looks like a server-relative path.
	BaseCommand string
}

// IsConfigured reports whether the policy has any fields worth checking. If
// both fields are empty the node will skip the protected-path check.
func (p ProtectionPolicy) IsConfigured() bool {
	return p.ServerExecutable != "" || p.BaseCommand != ""
}

// NewFileEntry builds a FileEntry from raw fields. Provided for callers that
// need to build entries directly (tests, in-process bridges).
func NewFileEntry(name string, size int64, isDirectory bool, modTime time.Time) FileEntry {
	return FileEntry{
		Name:         name,
		Size:         size,
		IsDirectory:  isDirectory,
		LastModified: modTime,
	}
}

// ProcessSnapshot is a point-in-time view of a single supervised process.
type ProcessSnapshot struct {
	ID              string
	Name            string
	Status          string
	UnixStartedAt   int64
	CPUPercent      float64
	CPUCores        int32
	MemoryRSS       uint64
	MemoryVMS       uint64
	MemoryPercent   float32
	NumThreads      int32
	DiskUsageBytes  uint64
	IOReadRate      float64
	IOWriteRate     float64
	ConnectionCount int32
	WorkingDir      string
}

// NodeSnapshot bundles host info, point-in-time host resource usage, and the
// per-process metrics for everything the supervisor is currently tracking.
// The "Node" prefix is intentional: this is the on-the-wire shape that the
// controller consumes from each node, where "Snapshot" alone would be
// ambiguous against ProcessSnapshot, ResourceSnapshot, etc.
//
//nolint:revive // explicit Node prefix matches the migration plan terminology
type NodeSnapshot struct {
	CPUModel      string
	CPUCores      int
	CPUThreads    int
	TotalMemory   uint64
	OS            string
	OSVersion     string
	Architecture  string
	XylonaVersion string

	CPUPercent    float64
	MemoryUsed    uint64
	MemoryPercent float64
	DiskUsed      uint64
	DiskTotal     uint64
	DiskPercent   float64

	// DefaultInstallPath is the node-resolved root directory under which the
	// controller should place managed game-server directories for this node.
	// Resolved on the node because it depends on the node's own OS and
	// HOME/USERPROFILE env vars (which are not knowable from the controller
	// in a hub-spoke deployment).
	DefaultInstallPath string

	Processes []ProcessSnapshot
	Collected time.Time
}

// ConsoleChunk is a slice of buffered console output for a process.
type ConsoleChunk struct {
	ProcessID string
	Data      string
}

// EventType identifies the kind of node event.
type EventType string

// Known node event types. The list is intentionally small for Step 1; it will
// grow when StreamEvents is wired up in Step 9.
const (
	EventTypeProcessStatus EventType = "process_status"
	EventTypeConsoleOutput EventType = "console_output"
	EventTypeMetrics       EventType = "metrics"
)

// Event is a typed event emitted by the node and consumed by the controller.
type Event struct {
	Type      EventType
	ProcessID string
	Status    string
	Payload   any
	Timestamp time.Time
}
