// Package node is the thin layer the Xylona node binary will expose. It owns
// process supervision, file operations, and host metrics for a single host.
// Its public APIs avoid HTTP/proto transport types; it is consumed by the
// controller in-process and wrapped by a gRPC implementation.
package node

import (
	"errors"
	"maps"
	"strings"
	"time"
)

// Sentinel errors returned by node operations.
var (
	// ErrInvalidPath is returned when a relative path escapes the resolved root.
	ErrInvalidPath = errors.New("node: invalid path")
	// ErrProtectedPath is returned when a write targets a protected file path.
	// internal/node never sets this directly; callers (the controller) may layer
	// protected-path policy on top of node operations.
	ErrProtectedPath = errors.New("node: path is protected")
	// ErrProcessNotFound is returned when a process command is not currently
	// tracked by the node.
	ErrProcessNotFound = errors.New("node: process not found")
	// ErrConsoleInputUnavailable is returned while a process input transport
	// is starting or reconnecting. Callers may retry the command.
	ErrConsoleInputUnavailable = errors.New("node: console input temporarily unavailable")
	// ErrUnexpectedHTTPStatus is returned when a node download receives a
	// non-success HTTP status.
	ErrUnexpectedHTTPStatus = errors.New("node: unexpected download HTTP status")
	// ErrDownloadIntegrityMismatch is returned when a node download does not
	// match the expected size or hash supplied by the controller.
	ErrDownloadIntegrityMismatch = errors.New("node: download integrity verification failed")
	// ErrInvalidPlayerAction is returned when an action, identifier, or reason
	// cannot be represented safely by the target game's management protocol.
	ErrInvalidPlayerAction = errors.New("node: invalid player action")
	// ErrPlayerActionUnsupported is returned when a game does not expose a safe
	// player-management protocol for the requested action.
	ErrPlayerActionUnsupported = errors.New("node: player action is unsupported")
	// ErrPlayerActionUnavailable is returned when a supported action cannot be
	// completed because the game server or its management API is unavailable.
	ErrPlayerActionUnavailable = errors.New("node: player action is unavailable")
)

// defaultStopTimeout mirrors supervisor's default graceful stop window.
const defaultStopTimeout = 15 * time.Second

// ProcessConfig describes a process to launch on the node. It is the
// transport-agnostic input for StartProcess and is translated into a
// supervisor.PreparedCommand internally.
type ProcessConfig struct {
	ID               string
	ExecutionID      string
	Name             string
	BaseCommand      string
	Args             []string
	WorkingDirectory string
	User             string
	NodeID           string
	ServiceID        string
	StopTimeout      time.Duration
	LaunchEnv        map[string]string

	// InputTelnet, when non-zero, configures the process to receive console
	// input over telnet (used by games like 7 Days to Die that don't accept
	// stdin). Remote controllers must verify the node's TelnetInput runtime
	// capability before sending it. The default remains stdin.
	InputTelnet *TelnetInput

	// InputRCON configures authenticated RCON console input. The credentials
	// stay in node memory and are never exposed through process snapshots.
	InputRCON *RCONInput

	// InputREST configures a game-specific REST command transport. Only
	// explicitly supported local APIs may be selected.
	InputREST *RESTInput

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
	// internal/node; the in-process client type-asserts.
	InternalGameServer any
}

// TelnetInput configures telnet-based console input for a process.
type TelnetInput struct {
	Port     int
	Password string
}

// RCONProtocol identifies the packet protocol spoken by an RCON server.
type RCONProtocol int

const (
	// RCONProtocolUnknown is invalid.
	RCONProtocolUnknown RCONProtocol = iota
	// RCONProtocolSource is used by Source-engine servers, Factorio, and
	// V Rising.
	RCONProtocolSource
	// RCONProtocolMinecraft is used by Minecraft-compatible RCON servers such
	// as Conan Exiles.
	RCONProtocolMinecraft
	// RCONProtocolRustWeb is Rust's WebSocket-based RCON protocol.
	RCONProtocolRustWeb
)

// RCONInput configures authenticated RCON console input for a process.
type RCONInput struct {
	Host     string
	Port     int
	Password string
	Protocol RCONProtocol
}

// RESTInputKind identifies a supported game-specific REST command API.
type RESTInputKind int

const (
	// RESTInputKindUnknown is invalid.
	RESTInputKindUnknown RESTInputKind = iota
	// RESTInputKindSatisfactory sends commands through the dedicated server
	// HTTPS API's RunCommand function.
	RESTInputKindSatisfactory
)

// RESTInput configures game-specific REST console input for a process.
type RESTInput struct {
	Host              string
	Port              int
	Kind              RESTInputKind
	Password          string
	PreviousPasswords []string
}

// normalize returns a copy of the config with whitespace trimmed from the base
// command, args duplicated, and the stop timeout defaulted.
func (p ProcessConfig) normalize() ProcessConfig {
	out := p
	out.BaseCommand = strings.TrimSpace(p.BaseCommand)
	if len(p.Args) > 0 {
		out.Args = append([]string(nil), p.Args...)
	}
	if len(p.LaunchEnv) > 0 {
		out.LaunchEnv = make(map[string]string, len(p.LaunchEnv))
		maps.Copy(out.LaunchEnv, p.LaunchEnv)
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
	IsExecutable bool
	LastModified time.Time
}

// WriteFileResult summarizes a completed node-side file write.
type WriteFileResult struct {
	BytesWritten int64
	SHA256       string
}

// DownloadIntegrity carries provider-advertised integrity metadata for a
// node-side HTTP download. Zero values mean the corresponding check is skipped.
type DownloadIntegrity struct {
	ExpectedSize   int64
	ExpectedSHA256 string
	ExpectedSHA1   string
}

// DownloadFileResult summarizes a completed node-side HTTP download.
type DownloadFileResult struct {
	RelativePath  string
	BytesWritten  int64
	SHA256        string
	SHA1          string
	ExpectedMatch bool
}

// HasExpectedMetadata reports whether at least one integrity check is configured.
func (i DownloadIntegrity) HasExpectedMetadata() bool {
	return i.ExpectedSize > 0 || strings.TrimSpace(i.ExpectedSHA256) != "" || strings.TrimSpace(i.ExpectedSHA1) != ""
}

// CopyFileOperation describes one source -> destination copy inside a node
// directory.
type CopyFileOperation struct {
	SourceRelativePath      string
	DestinationRelativePath string
}

// ArchiveCompression names the file-archive format a node should create.
type ArchiveCompression int

// Archive compression formats supported by node file archives.
const (
	ArchiveCompressionZIP ArchiveCompression = iota
	ArchiveCompressionBZIP2
	ArchiveCompressionGZIP
	ArchiveCompressionZST
	ArchiveCompressionXZ
)

// ArchiveProgress summarizes a completed file archive operation.
type ArchiveProgress struct {
	TotalFiles      int64
	FilesCompressed int64
	TotalBytes      int64
	BytesCompressed int64
	CurrentFile     string
}

// ExtractProgress summarizes a completed file extraction operation.
type ExtractProgress struct {
	TotalFiles     int64
	FilesExtracted int64
	TotalBytes     int64
	BytesExtracted int64
	CurrentFile    string
}

// BindableIP describes a host IP address that game servers may bind to.
type BindableIP struct {
	Address  string
	Usable   bool
	External bool
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

// InstalledVersionProbeKind identifies the narrow node-side installed-version
// probe to run.
type InstalledVersionProbeKind int

const (
	// InstalledVersionProbeKindUnspecified is invalid and returns ErrInvalidPath.
	InstalledVersionProbeKindUnspecified InstalledVersionProbeKind = iota
	// InstalledVersionProbeKindMinecraftJar reads version.json from a Minecraft server jar.
	InstalledVersionProbeKindMinecraftJar
	// InstalledVersionProbeKindSteamManifest reads buildid from a Steam appmanifest ACF file.
	InstalledVersionProbeKindSteamManifest
)

// InstalledVersionProbeRequest asks the node to inspect local files for an
// installed game version. It intentionally carries only filesystem paths and
// probe hints, not controller models.
type InstalledVersionProbeRequest struct {
	Directory           string
	Kind                InstalledVersionProbeKind
	RelativePaths       []string
	PreferredSteamAppID string
}

// InstalledVersionProbeResult is the outcome of a node-side installed-version
// probe.
type InstalledVersionProbeResult struct {
	Found      bool
	Version    string
	SourcePath string
}

// GameServerQueryKind identifies the narrow game-server network probe a node
// may execute. Scheduling and persistence stay in the controller.
type GameServerQueryKind int

const (
	// GameServerQueryKindUnknown is invalid and performs no probe.
	GameServerQueryKindUnknown GameServerQueryKind = iota
	// GameServerQueryKindMinecraft probes Minecraft query/ping protocols.
	GameServerQueryKindMinecraft
	// GameServerQueryKindSource probes Source-engine A2S info.
	GameServerQueryKindSource
	// GameServerQueryKindPalworld probes the authenticated Palworld REST API.
	GameServerQueryKindPalworld
	// GameServerQueryKindSevenDaysToDie identifies 7 Days to Die console
	// player-management commands.
	GameServerQueryKindSevenDaysToDie
	// GameServerQueryKindFactorio identifies Factorio console commands.
	GameServerQueryKindFactorio
	// GameServerQueryKindHytale identifies Hytale console commands.
	GameServerQueryKindHytale
	// GameServerQueryKindProjectZomboid identifies Project Zomboid console
	// commands.
	GameServerQueryKindProjectZomboid
	// GameServerQueryKindTerraria identifies Terraria console commands.
	GameServerQueryKindTerraria
	// GameServerQueryKindSourceRCON identifies Source-engine RCON commands.
	GameServerQueryKindSourceRCON
	// GameServerQueryKindRust identifies Rust console commands.
	GameServerQueryKindRust
)

// GameServerPlayerAction identifies a typed administrative action. The node
// translates these values into game-specific commands or API calls; callers
// never provide raw command text or endpoint paths.
type GameServerPlayerAction int

const (
	// GameServerPlayerActionUnknown is invalid and performs no action.
	GameServerPlayerActionUnknown GameServerPlayerAction = iota
	// GameServerPlayerActionKick disconnects an online player.
	GameServerPlayerActionKick
	// GameServerPlayerActionBan blocks a player from joining.
	GameServerPlayerActionBan
	// GameServerPlayerActionUnban removes a player ban.
	GameServerPlayerActionUnban
	// GameServerPlayerActionAllowlistAdd adds a player to the allowlist.
	GameServerPlayerActionAllowlistAdd
	// GameServerPlayerActionAllowlistRemove removes a player from the allowlist.
	GameServerPlayerActionAllowlistRemove
)

// GameServerPlayer is a player identity returned by a game-server query. ID is
// populated only when the target protocol provides a stable, action-safe
// identifier.
type GameServerPlayer struct {
	Name string
	ID   string
}

// GameServerPlayerActionRequest asks a node to execute one typed action using
// the target game's native management protocol.
type GameServerPlayerActionRequest struct {
	Kind      GameServerQueryKind
	Action    GameServerPlayerAction
	ProcessID string
	IP        string
	QueryPort int64
	Username  string
	Password  string
	PlayerID  string
	Reason    string
}

// GameServerQueryRequest asks a node to probe a game server from the node host.
type GameServerQueryRequest struct {
	Kind       GameServerQueryKind
	IP         string
	QueryPort  int64
	MaxPlayers int64
	Username   string
	Password   string
}

// MinecraftQueryInfo is the transport-agnostic result of a Minecraft query.
type MinecraftQueryInfo struct {
	MOTD            string
	GameType        string
	Map             string
	NumberOfPlayers uint32
	MaxPlayers      uint32
	PlayerList      []string
	ProtocolVersion uint32
	ServerVersion   string
	PlayerDetails   []GameServerPlayer
}

// SourceQueryInfo is the transport-agnostic result of a Source query.
type SourceQueryInfo struct {
	Name                string
	Map                 string
	Game                string
	AppID               uint32
	SteamID             uint64
	GameID              uint64
	Players             uint32
	MaxPlayers          uint32
	Bots                uint32
	ServerOS            string
	Visibility          bool
	VAC                 bool
	Version             string
	Protocol            uint32
	PlayerList          []string
	PlayerListSupported bool
}

// PalworldQueryInfo is the transport-agnostic result of a Palworld REST query.
type PalworldQueryInfo struct {
	Name              string
	Description       string
	Version           string
	WorldGUID         string
	Players           uint32
	MaxPlayers        uint32
	PlayerList        []string
	UptimeSeconds     uint64
	ServerFPS         float64
	ServerFrameTimeMS float64
	Days              uint32
	Responded         bool
	PlayerDetails     []GameServerPlayer
}

// PalworldMapActorKind identifies one sanitized live-map actor category.
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

// PalworldMapActor contains the display-safe actor fields transported from a
// node to the controller. Administrative identifiers never enter this type.
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

// PalworldMapSnapshot is a sanitized point-in-time world snapshot.
type PalworldMapSnapshot struct {
	SourceTime  string
	CollectedAt time.Time
	Source      string
	Partial     bool
	Truncated   bool
	Actors      []PalworldMapActor
}

// PalworldMapQueryRequest contains the node-local Palworld REST connection.
type PalworldMapQueryRequest struct {
	IP        string
	QueryPort int64
	Username  string
	Password  string
}

// GameServerQueryResult is the transport-agnostic result of a node-side
// network probe.
type GameServerQueryResult struct {
	Kind      GameServerQueryKind
	Minecraft *MinecraftQueryInfo
	Source    *SourceQueryInfo
	Palworld  *PalworldQueryInfo
}

// IsConfigured reports whether the policy has any fields worth checking. If
// both fields are empty the node will skip the protected-path check.
func (p ProtectionPolicy) IsConfigured() bool {
	return p.ServerExecutable != "" || p.BaseCommand != ""
}

// NewFileEntry builds a FileEntry from raw fields. Provided for callers that
// need to build entries directly (tests, in-process bridges).
func NewFileEntry(name string, size int64, isDirectory bool, modTime time.Time, isExecutable ...bool) FileEntry {
	executable := false
	if len(isExecutable) > 0 {
		executable = isExecutable[0]
	}
	return FileEntry{
		Name:         name,
		Size:         size,
		IsDirectory:  isDirectory,
		IsExecutable: executable,
		LastModified: modTime,
	}
}

// ProcessSnapshot is a point-in-time view of a single supervised process.
type ProcessSnapshot struct {
	ID                   string
	ExecutionID          string
	Name                 string
	Status               string
	PreviousStatus       string
	TransitionSequence   uint64
	IntentionalStop      bool
	ExitCode             int
	ExitCodeKnown        bool
	UnixStartedAt        int64
	CPUPercent           float64
	CPUValid             bool
	MetricsValid         bool
	CPUCores             int32
	MemoryRSS            uint64
	MemoryVMS            uint64
	MemoryPercent        float32
	NumThreads           int32
	DiskUsageBytes       uint64
	DiskTotalBytes       uint64
	DiskFreeBytes        uint64
	DiskPercent          float64
	DiskMeasuredAt       time.Time
	DiskValid            bool
	IOValid              bool
	IOReadRate           float64
	IOWriteRate          float64
	ConnectionCount      int32
	ConnectionCountValid bool
	WorkingDir           string
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
	ProcessID   string
	Data        string
	Sequence    uint64
	ResetBuffer bool
}

// EventType identifies the kind of node event.
type EventType string

// Known node event types.
const (
	EventTypeProcessStatus EventType = "process_status"
	EventTypeConsoleOutput EventType = "console_output"
	EventTypeMetrics       EventType = "metrics"
)

// Event is a typed event emitted by the node and consumed by the controller.
type Event struct {
	Type               EventType
	ProcessID          string
	Status             string
	OldStatus          string
	ExecutionID        string
	TransitionSequence uint64
	IntentionalStop    bool
	ExitCode           int
	ExitCodeKnown      bool
	Replayed           bool
	Payload            any
	Timestamp          time.Time
}
