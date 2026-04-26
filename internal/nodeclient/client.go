// Package nodeclient defines the abstraction the Xylona controller uses to
// talk to a node, regardless of whether the node is running in-process
// (embedded) or across the network (remote gRPC node).
//
// The controller never holds a *supervisor.Instance or a *internal/node.Node
// directly; it holds a NodeClient. For the embedded path this is a thin
// in-process wrapper; for remote nodes it is a gRPC client.
//
// Method signatures use plain Go types from internal/node rather than proto types.
// This keeps the interface stable even when the on-the-wire proto evolves.
package nodeclient

import (
	"context"
	"io"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// NodeClient is the controller-side handle to a single node (embedded or
// remote). Each implementation is responsible for translating method calls
// into the appropriate transport: direct method calls for in-process nodes,
// gRPC for remote nodes.
//
// Consumers that only need a small subset of node behavior should declare a
// local consumer-side interface instead of accepting this full aggregate.
//
// Implementations MUST be safe for concurrent use by multiple goroutines.
type NodeClient interface {
	// ID returns the node's identifier. Stable for the lifetime of the client.
	ID() string

	// StartProcess launches the process described by cfg. Callers observe
	// lifecycle, status, metrics, and console output through the node client
	// stream/snapshot methods instead of a supervisor handle.
	StartProcess(ctx context.Context, cfg node.ProcessConfig, status xylona.Status) error

	// StopProcess requests a graceful stop of the process identified by
	// processID. The optional stopInputCommand is written to the process's
	// console before the node falls back to signal-based termination.
	StopProcess(ctx context.Context, processID, stopInputCommand string) error

	// SendConsoleInput writes a single line of input to the running process's
	// configured input writer (stdin or telnet, depending on InputMethod).
	SendConsoleInput(ctx context.Context, processID, input string) error

	// ReadConsoleBuffer returns the node's buffered console output for the
	// given process. Callers receive an empty ConsoleChunk when the process
	// is unknown rather than an error so they do not need to special-case
	// missing servers.
	ReadConsoleBuffer(ctx context.Context, processID string) (node.ConsoleChunk, error)

	// StreamConsoleOutput streams live console output chunks for one process.
	// The returned channel closes when ctx is canceled or the stream ends.
	StreamConsoleOutput(ctx context.Context, processID string) (<-chan node.ConsoleChunk, error)

	// ListFiles enumerates the entries directly under directory/relativePath.
	ListFiles(ctx context.Context, directory, relativePath string) ([]node.FileEntry, error)

	// ReadFile returns the bytes of directory/relativePath.
	ReadFile(ctx context.Context, directory, relativePath string) ([]byte, error)

	// StatFile returns metadata for directory/relativePath without reading
	// the file content.
	StatFile(ctx context.Context, directory, relativePath string) (node.FileEntry, error)

	// StreamFile returns a reader for directory/relativePath so callers can
	// proxy large node-resident files without controller-side temp staging.
	StreamFile(ctx context.Context, directory, relativePath string) (io.ReadCloser, error)

	// WriteFile writes content to directory/relativePath. policy carries the
	// controller's protected-path context so the node can reject writes to
	// the game server's executable or launch script; pass a zero-value
	// ProtectionPolicy for non-game-server requests.
	WriteFile(ctx context.Context, directory, relativePath string, content []byte, policy node.ProtectionPolicy) error

	// StreamWriteFile writes reader content to directory/relativePath without
	// loading the complete payload into memory.
	StreamWriteFile(ctx context.Context, directory, relativePath string, reader io.Reader, policy node.ProtectionPolicy) (node.WriteFileResult, error)

	// CreateFileOrDirectory creates a file (with optional content) or
	// directory inside directory.
	CreateFileOrDirectory(ctx context.Context, directory, relativePath, content string, isDirectory bool, policy node.ProtectionPolicy) error

	// DeleteFiles removes each provided relative path from directory and
	// returns the validated paths that were successfully removed.
	DeleteFiles(ctx context.Context, directory string, files []string, policy node.ProtectionPolicy) ([]string, error)

	// RenameFile renames oldRelativePath to newRelativePath inside directory.
	// The validated new path is returned on success.
	RenameFile(ctx context.Context, directory, oldRelativePath, newRelativePath string, policy node.ProtectionPolicy) (string, error)

	// MoveFiles moves files into destination, which is a relative path inside
	// directory. The destination directory is created if needed. The returned
	// slice contains the validated source paths that were successfully moved.
	MoveFiles(ctx context.Context, directory string, files []string, destination string, policy node.ProtectionPolicy) ([]string, error)

	// CopyFiles copies source paths to paired destination paths inside
	// directory. Destination paths are protected by policy.
	CopyFiles(ctx context.Context, directory string, operations []node.CopyFileOperation, policy node.ProtectionPolicy) ([]string, error)

	// DownloadFileFromURL fetches rawURL over HTTP/HTTPS and stores the
	// result inside directory under destinationDirectoryPath.
	DownloadFileFromURL(ctx context.Context, directory, rawURL, destinationDirectoryPath string, integrity node.DownloadIntegrity, policy node.ProtectionPolicy) (node.DownloadFileResult, error)

	// CreateFileArchive asks the node to build a user-requested archive inside
	// directory without staging remote content on the controller.
	CreateFileArchive(ctx context.Context, directory string, destinationArchivePath string, includePaths []string, compression node.ArchiveCompression, policy node.ProtectionPolicy) (string, node.ArchiveProgress, error)

	// CreateFileArchiveWithProgress is CreateFileArchive plus progress events
	// forwarded from the node. Implementations should avoid short HTTP client
	// timeouts because archive operations can be long-running.
	CreateFileArchiveWithProgress(ctx context.Context, directory string, destinationArchivePath string, includePaths []string, compression node.ArchiveCompression, policy node.ProtectionPolicy, onProgress func(node.ArchiveProgress) error) (string, node.ArchiveProgress, error)

	// ExtractFileArchive asks the node to extract a user-requested archive
	// inside directory without staging remote content on the controller.
	ExtractFileArchive(ctx context.Context, directory string, archivePath string, destinationDirectoryPath string, policy node.ProtectionPolicy) ([]string, node.ExtractProgress, error)

	// ExtractFileArchiveWithProgress is ExtractFileArchive plus progress
	// events forwarded from the node. Implementations should avoid short HTTP
	// client timeouts because extraction can be long-running.
	ExtractFileArchiveWithProgress(ctx context.Context, directory string, archivePath string, destinationDirectoryPath string, policy node.ProtectionPolicy, onProgress func(node.ExtractProgress) error) ([]string, node.ExtractProgress, error)

	// CreateBackupArchive asks the node to build a zip archive at
	// destinationArchivePath containing includePaths relative to directory.
	// Returns the archive size in bytes and its SHA-256 digest for integrity
	// verification by the controller.
	CreateBackupArchive(ctx context.Context, directory string, includePaths []string, destinationArchivePath string) (archiveBytes int64, archiveSHA256 string, err error)

	// ExtractBackupArchive unpacks archivePath into directory using the given
	// mode. Used by backup-restore flows.
	ExtractBackupArchive(ctx context.Context, directory string, archivePath string, mode node.ExtractMode) error

	// ProbeInstalledVersion asks the node to inspect local files for a narrow
	// installed-version marker.
	ProbeInstalledVersion(ctx context.Context, req node.InstalledVersionProbeRequest) (node.InstalledVersionProbeResult, error)

	// QueryGameServer asks the node to execute a game-server network query
	// probe from the node host. The controller remains responsible for
	// scheduling and storing the returned result.
	QueryGameServer(ctx context.Context, req node.GameServerQueryRequest) (node.GameServerQueryResult, error)

	// SendConsoleOutput writes a controller-generated line into the process's
	// console buffer on the node side. Used for pre-start messages, mod
	// auto-update progress, and other controller chatter that should appear
	// in the console UI for the given game server.
	SendConsoleOutput(ctx context.Context, processID string, line string) error

	// GetProcessSnapshot returns the current status + metrics snapshot for a
	// single process. The second return value is false when the process is
	// not currently tracked by the node (e.g. never started).
	GetProcessSnapshot(ctx context.Context, processID string) (snapshot *node.ProcessSnapshot, found bool, err error)

	// GetNodeSnapshot returns a point-in-time view of the host plus
	// per-process metrics for everything the node is currently tracking.
	GetNodeSnapshot(ctx context.Context) (*node.NodeSnapshot, error)

	// ListBindableIPs returns the node-local IP addresses that can be bound
	// by game servers on this node.
	ListBindableIPs(ctx context.Context) ([]node.BindableIP, error)

	// GetUpdateCapabilities reports whether the node can stage and apply a
	// service-manager-backed self-update.
	GetUpdateCapabilities(ctx context.Context) (node.UpdateCapabilities, error)

	// GetRuntimeCapabilities reports process-runtime features such as
	// secret-safe launch environment support.
	GetRuntimeCapabilities(ctx context.Context) (node.RuntimeCapabilities, error)

	// StageSelfUpdate streams a verified xylona-node artifact to the node.
	StageSelfUpdate(ctx context.Context, req node.StageSelfUpdateRequest) (node.StageSelfUpdateResult, error)

	// ApplySelfUpdate asks the node to hand a staged update to its helper.
	ApplySelfUpdate(ctx context.Context, req node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error)

	// StreamEvents returns a channel receiving events published by the node.
	// The channel closes when ctx is canceled or the client is closed.
	//
	StreamEvents(ctx context.Context) (<-chan node.Event, error)

	// Ping verifies the client can reach the node. For the in-process
	// implementation this always succeeds.
	Ping(ctx context.Context) error
}
