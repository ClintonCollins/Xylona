package nodeclient

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// FakeNodeClient is a test double implementing NodeClient. All error fields
// default to nil and all return-value fields default to their zero value,
// which is sufficient for most happy-path tests. Callers can override any
// field before exercising the client and inspect the *Calls slices after.
//
// The zero value is ready to use; no constructor is required. The mutex
// protects *Calls slices only, so tests may safely invoke a FakeNodeClient
// from multiple goroutines while asserting on recorded calls afterwards.
//
// This is deliberately simple: scenarios that need per-call behavior should
// wrap the fake with a wrapper struct in the test itself.
type FakeNodeClient struct {
	mu sync.Mutex

	NodeID string

	StartProcessErr   error
	StartProcessCalls []StartProcessCall

	StopProcessErr   error
	StopProcessCalls []StopProcessCall

	SendConsoleInputErr   error
	SendConsoleInputCalls []SendConsoleInputCall

	ReadConsoleBufferResult node.ConsoleChunk
	ReadConsoleBufferErr    error
	ReadConsoleBufferCalls  []string

	StreamConsoleOutputChannel chan node.ConsoleChunk
	StreamConsoleOutputErr     error
	StreamConsoleOutputCalls   []string

	ListFilesResult []node.FileEntry
	ListFilesErr    error
	ListFilesCalls  []ListFilesCall

	ReadFileResult []byte
	ReadFileErr    error
	ReadFileCalls  []ListFilesCall

	StatFileResult node.FileEntry
	StatFileErr    error
	StatFileCalls  []ListFilesCall

	StreamFileReader io.ReadCloser
	StreamFileErr    error
	StreamFileCalls  []ListFilesCall

	WriteFileErr   error
	WriteFileCalls []WriteFileCall

	StreamWriteFileResult node.WriteFileResult
	StreamWriteFileErr    error
	StreamWriteFileCalls  []StreamWriteFileCall

	CreateFileOrDirectoryErr   error
	CreateFileOrDirectoryCalls []CreateFileOrDirectoryCall

	DeleteFilesResult []string
	DeleteFilesErr    error
	DeleteFilesCalls  []DeleteFilesCall

	RenameFileResult string
	RenameFileErr    error
	RenameFileCalls  []RenameFileCall

	MoveFilesResult []string
	MoveFilesErr    error
	MoveFilesCalls  []MoveFilesCall

	CopyFilesResult []string
	CopyFilesErr    error
	CopyFilesCalls  []CopyFilesCall

	DownloadFileFromURLResult node.DownloadFileResult
	DownloadFileFromURLErr    error
	DownloadFileFromURLCalls  []DownloadFileFromURLCall

	CreateFileArchiveResult   string
	CreateFileArchiveProgress node.ArchiveProgress
	CreateFileArchiveErr      error
	CreateFileArchiveCalls    []CreateFileArchiveCall

	ExtractFileArchiveResult   []string
	ExtractFileArchiveProgress node.ExtractProgress
	ExtractFileArchiveErr      error
	ExtractFileArchiveCalls    []ExtractFileArchiveCall

	CreateBackupArchiveBytes  int64
	CreateBackupArchiveSHA256 string
	CreateBackupArchiveErr    error
	CreateBackupArchiveFunc   func(ctx context.Context, directory string, includePaths []string, destinationArchivePath string) (int64, string, error)
	CreateBackupArchiveCalls  []CreateBackupArchiveCall

	ExtractBackupArchiveErr   error
	ExtractBackupArchiveCalls []ExtractBackupArchiveCall

	ProbeInstalledVersionResult node.InstalledVersionProbeResult
	ProbeInstalledVersionErr    error
	ProbeInstalledVersionCalls  []node.InstalledVersionProbeRequest

	QueryGameServerResult node.GameServerQueryResult
	QueryGameServerErr    error
	QueryGameServerCalls  []node.GameServerQueryRequest

	QueryPalworldMapResult *node.PalworldMapSnapshot
	QueryPalworldMapErr    error
	QueryPalworldMapCalls  []node.PalworldMapQueryRequest
	QueryPalworldMapFunc   func(context.Context, node.PalworldMapQueryRequest) (*node.PalworldMapSnapshot, error)

	QuerySevenDaysToDieMapResult *node.SevenDaysToDieMapSnapshot
	QuerySevenDaysToDieMapErr    error
	QuerySevenDaysToDieMapCalls  []node.SevenDaysToDieMapQueryRequest
	QuerySevenDaysToDieMapFunc   func(context.Context, node.SevenDaysToDieMapQueryRequest) (*node.SevenDaysToDieMapSnapshot, error)

	QuerySevenDaysToDieWebAPIStatusResult *node.SevenDaysToDieWebAPIStatus
	QuerySevenDaysToDieWebAPIStatusErr    error
	QuerySevenDaysToDieWebAPIStatusCalls  []node.SevenDaysToDieWebAPIStatusQueryRequest
	QuerySevenDaysToDieWebAPIStatusFunc   func(context.Context, node.SevenDaysToDieWebAPIStatusQueryRequest) (*node.SevenDaysToDieWebAPIStatus, error)

	QuerySevenDaysToDiePlayersResult *node.SevenDaysToDiePlayers
	QuerySevenDaysToDiePlayersErr    error
	QuerySevenDaysToDiePlayersCalls  []node.SevenDaysToDiePlayersQueryRequest
	QuerySevenDaysToDiePlayersFunc   func(context.Context, node.SevenDaysToDiePlayersQueryRequest) (*node.SevenDaysToDiePlayers, error)

	QuerySevenDaysToDieReportedModsResult *node.SevenDaysToDieReportedMods
	QuerySevenDaysToDieReportedModsErr    error
	QuerySevenDaysToDieReportedModsCalls  []node.SevenDaysToDieReportedModsQueryRequest
	QuerySevenDaysToDieReportedModsFunc   func(context.Context, node.SevenDaysToDieReportedModsQueryRequest) (*node.SevenDaysToDieReportedMods, error)

	SevenDaysToDieMapTileResult []byte
	SevenDaysToDieMapTileErr    error
	SevenDaysToDieMapTileCalls  []node.SevenDaysToDieMapTileRequest

	EnsureMinecraftMapResult node.MinecraftMapStatus
	EnsureMinecraftMapErr    error
	EnsureMinecraftMapCalls  []node.MinecraftMapEnsureRequest
	EnsureMinecraftMapFunc   func(context.Context, node.MinecraftMapEnsureRequest) (node.MinecraftMapStatus, error)

	StopMinecraftMapErr   error
	StopMinecraftMapCalls []string

	MinecraftMapAssetResult node.MinecraftMapAsset
	MinecraftMapAssetErr    error
	MinecraftMapAssetCalls  []node.MinecraftMapAssetRequest

	PerformGameServerPlayerActionErr   error
	PerformGameServerPlayerActionCalls []node.GameServerPlayerActionRequest

	SendConsoleOutputErr   error
	SendConsoleOutputCalls []SendConsoleOutputCall

	GetProcessSnapshotResult *node.ProcessSnapshot
	GetProcessSnapshotFound  bool
	GetProcessSnapshotErr    error
	GetProcessSnapshotCalls  []string

	SnapshotResult *node.NodeSnapshot
	SnapshotErr    error
	SnapshotCalls  int

	BindableIPsResult []node.BindableIP
	BindableIPsErr    error
	BindableIPsCalls  int

	UpdateCapabilitiesResult node.UpdateCapabilities
	UpdateCapabilitiesErr    error
	UpdateCapabilitiesCalls  int
	UpdateCapabilitiesFunc   func(context.Context) (node.UpdateCapabilities, error)

	RuntimeCapabilitiesResult node.RuntimeCapabilities
	RuntimeCapabilitiesErr    error
	RuntimeCapabilitiesCalls  int
	RuntimeCapabilitiesFunc   func(context.Context) (node.RuntimeCapabilities, error)

	StageSelfUpdateResult node.StageSelfUpdateResult
	StageSelfUpdateErr    error
	StageSelfUpdateCalls  []StageSelfUpdateCall
	StageSelfUpdateFunc   func(context.Context, node.StageSelfUpdateRequest) (node.StageSelfUpdateResult, error)

	ApplySelfUpdateResult node.ApplySelfUpdateResult
	ApplySelfUpdateErr    error
	ApplySelfUpdateCalls  []node.ApplySelfUpdateRequest
	ApplySelfUpdateFunc   func(context.Context, node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error)

	StreamEventsChannel chan node.Event
	StreamEventsErr     error
	StreamEventsCalls   int

	PingErr   error
	PingCalls int
}

// StartProcessCall records a single StartProcess invocation.
type StartProcessCall struct {
	Config node.ProcessConfig
	Status xylona.Status
}

// StopProcessCall records a single StopProcess invocation.
type StopProcessCall struct {
	ProcessID        string
	StopInputCommand string
}

// SendConsoleInputCall records a single SendConsoleInput invocation.
type SendConsoleInputCall struct {
	ProcessID string
	Input     string
}

// ListFilesCall records a single ListFiles (or ReadFile) invocation.
type ListFilesCall struct {
	Directory    string
	RelativePath string
}

// WriteFileCall records a single WriteFile invocation.
type WriteFileCall struct {
	Directory    string
	RelativePath string
	Content      []byte
}

// StreamWriteFileCall records a single StreamWriteFile invocation.
type StreamWriteFileCall struct {
	Directory    string
	RelativePath string
	Content      []byte
	Policy       node.ProtectionPolicy
}

// CreateFileOrDirectoryCall records a single CreateFileOrDirectory invocation.
type CreateFileOrDirectoryCall struct {
	Directory    string
	RelativePath string
	Content      string
	IsDirectory  bool
}

// DeleteFilesCall records a single DeleteFiles invocation.
type DeleteFilesCall struct {
	Directory string
	Files     []string
}

// RenameFileCall records a single RenameFile invocation.
type RenameFileCall struct {
	Directory       string
	OldRelativePath string
	NewRelativePath string
}

// MoveFilesCall records a single MoveFiles invocation.
type MoveFilesCall struct {
	Directory   string
	Files       []string
	Destination string
}

// CopyFilesCall records a single CopyFiles invocation.
type CopyFilesCall struct {
	Directory  string
	Operations []node.CopyFileOperation
	Policy     node.ProtectionPolicy
}

// DownloadFileFromURLCall records a single DownloadFileFromURL invocation.
type DownloadFileFromURLCall struct {
	Directory                string
	RawURL                   string
	DestinationDirectoryPath string
	Integrity                node.DownloadIntegrity
	Policy                   node.ProtectionPolicy
}

// CreateFileArchiveCall records a single CreateFileArchive invocation.
type CreateFileArchiveCall struct {
	Directory              string
	DestinationArchivePath string
	IncludePaths           []string
	Compression            node.ArchiveCompression
}

// ExtractFileArchiveCall records a single ExtractFileArchive invocation.
type ExtractFileArchiveCall struct {
	Directory                string
	ArchivePath              string
	DestinationDirectoryPath string
}

// CreateBackupArchiveCall records a single CreateBackupArchive invocation.
type CreateBackupArchiveCall struct {
	Directory              string
	IncludePaths           []string
	DestinationArchivePath string
}

// ExtractBackupArchiveCall records a single ExtractBackupArchive invocation.
type ExtractBackupArchiveCall struct {
	Directory   string
	ArchivePath string
	Mode        node.ExtractMode
}

// SendConsoleOutputCall records a single SendConsoleOutput invocation.
type SendConsoleOutputCall struct {
	ProcessID string
	Line      string
}

// StageSelfUpdateCall records a single StageSelfUpdate invocation.
type StageSelfUpdateCall struct {
	Request node.StageSelfUpdateRequest
	Content []byte
}

// ID returns the configured NodeID (zero-value-safe: empty string if unset).
func (f *FakeNodeClient) ID() string {
	return f.NodeID
}

// StartProcess records the call and returns the configured error.
func (f *FakeNodeClient) StartProcess(_ context.Context, cfg node.ProcessConfig, status xylona.Status) error {
	recorded := cfg
	recorded.Args = append([]string(nil), cfg.Args...)
	recorded.LaunchEnv = cloneStringMap(cfg.LaunchEnv)
	f.mu.Lock()
	f.StartProcessCalls = append(f.StartProcessCalls, StartProcessCall{Config: recorded, Status: status})
	f.mu.Unlock()
	return f.StartProcessErr
}

// StopProcess records the call and returns the configured error.
func (f *FakeNodeClient) StopProcess(_ context.Context, processID, stopInputCommand string) error {
	f.mu.Lock()
	f.StopProcessCalls = append(f.StopProcessCalls, StopProcessCall{ProcessID: processID, StopInputCommand: stopInputCommand})
	f.mu.Unlock()
	return f.StopProcessErr
}

// SendConsoleInput records the call and returns the configured error.
func (f *FakeNodeClient) SendConsoleInput(_ context.Context, processID, input string) error {
	f.mu.Lock()
	f.SendConsoleInputCalls = append(f.SendConsoleInputCalls, SendConsoleInputCall{ProcessID: processID, Input: input})
	f.mu.Unlock()
	return f.SendConsoleInputErr
}

// ReadConsoleBuffer records the call and returns the configured result.
func (f *FakeNodeClient) ReadConsoleBuffer(_ context.Context, processID string) (node.ConsoleChunk, error) {
	f.mu.Lock()
	f.ReadConsoleBufferCalls = append(f.ReadConsoleBufferCalls, processID)
	f.mu.Unlock()
	if f.ReadConsoleBufferErr != nil {
		return node.ConsoleChunk{ProcessID: processID}, f.ReadConsoleBufferErr
	}
	return f.ReadConsoleBufferResult, nil
}

// StreamConsoleOutput records the call and returns the configured channel.
func (f *FakeNodeClient) StreamConsoleOutput(_ context.Context, processID string) (<-chan node.ConsoleChunk, error) {
	f.mu.Lock()
	f.StreamConsoleOutputCalls = append(f.StreamConsoleOutputCalls, processID)
	f.mu.Unlock()
	return f.StreamConsoleOutputChannel, f.StreamConsoleOutputErr
}

// ListFiles records the call and returns the configured result.
func (f *FakeNodeClient) ListFiles(_ context.Context, directory, relativePath string) ([]node.FileEntry, error) {
	f.mu.Lock()
	f.ListFilesCalls = append(f.ListFilesCalls, ListFilesCall{Directory: directory, RelativePath: relativePath})
	f.mu.Unlock()
	return f.ListFilesResult, f.ListFilesErr
}

// ReadFile records the call and returns the configured result.
func (f *FakeNodeClient) ReadFile(_ context.Context, directory, relativePath string) ([]byte, error) {
	f.mu.Lock()
	f.ReadFileCalls = append(f.ReadFileCalls, ListFilesCall{Directory: directory, RelativePath: relativePath})
	f.mu.Unlock()
	return f.ReadFileResult, f.ReadFileErr
}

// StatFile records the call and returns the configured result.
func (f *FakeNodeClient) StatFile(_ context.Context, directory, relativePath string) (node.FileEntry, error) {
	f.mu.Lock()
	f.StatFileCalls = append(f.StatFileCalls, ListFilesCall{Directory: directory, RelativePath: relativePath})
	f.mu.Unlock()
	return f.StatFileResult, f.StatFileErr
}

// StreamFile records the call and returns the configured reader.
func (f *FakeNodeClient) StreamFile(_ context.Context, directory, relativePath string) (io.ReadCloser, error) {
	f.mu.Lock()
	f.StreamFileCalls = append(f.StreamFileCalls, ListFilesCall{Directory: directory, RelativePath: relativePath})
	f.mu.Unlock()
	return f.StreamFileReader, f.StreamFileErr
}

// WriteFile records the call and returns the configured error.
func (f *FakeNodeClient) WriteFile(_ context.Context, directory, relativePath string, content []byte, _ node.ProtectionPolicy) error {
	f.mu.Lock()
	// Copy content to avoid callers mutating recorded bytes afterwards.
	copied := append([]byte(nil), content...)
	f.WriteFileCalls = append(f.WriteFileCalls, WriteFileCall{Directory: directory, RelativePath: relativePath, Content: copied})
	f.mu.Unlock()
	return f.WriteFileErr
}

// StreamWriteFile records the call and returns the configured result.
func (f *FakeNodeClient) StreamWriteFile(_ context.Context, directory, relativePath string, reader io.Reader, policy node.ProtectionPolicy) (node.WriteFileResult, error) {
	var content []byte
	if reader != nil {
		data, errRead := io.ReadAll(reader)
		if errRead != nil {
			return node.WriteFileResult{}, fmt.Errorf("nodeclient fake: read stream write file: %w", errRead)
		}
		content = data
	}
	f.mu.Lock()
	copied := append([]byte(nil), content...)
	f.StreamWriteFileCalls = append(f.StreamWriteFileCalls, StreamWriteFileCall{Directory: directory, RelativePath: relativePath, Content: copied, Policy: policy})
	f.mu.Unlock()
	return f.StreamWriteFileResult, f.StreamWriteFileErr
}

// CreateFileOrDirectory records the call and returns the configured error.
func (f *FakeNodeClient) CreateFileOrDirectory(_ context.Context, directory, relativePath, content string, isDirectory bool, _ node.ProtectionPolicy) error {
	f.mu.Lock()
	f.CreateFileOrDirectoryCalls = append(f.CreateFileOrDirectoryCalls, CreateFileOrDirectoryCall{
		Directory:    directory,
		RelativePath: relativePath,
		Content:      content,
		IsDirectory:  isDirectory,
	})
	f.mu.Unlock()
	return f.CreateFileOrDirectoryErr
}

// DeleteFiles records the call and returns the configured result.
func (f *FakeNodeClient) DeleteFiles(_ context.Context, directory string, files []string, _ node.ProtectionPolicy) ([]string, error) {
	f.mu.Lock()
	copied := append([]string(nil), files...)
	f.DeleteFilesCalls = append(f.DeleteFilesCalls, DeleteFilesCall{Directory: directory, Files: copied})
	f.mu.Unlock()
	return f.DeleteFilesResult, f.DeleteFilesErr
}

// RenameFile records the call and returns the configured result.
func (f *FakeNodeClient) RenameFile(_ context.Context, directory, oldRelativePath, newRelativePath string, _ node.ProtectionPolicy) (string, error) {
	f.mu.Lock()
	f.RenameFileCalls = append(f.RenameFileCalls, RenameFileCall{
		Directory:       directory,
		OldRelativePath: oldRelativePath,
		NewRelativePath: newRelativePath,
	})
	f.mu.Unlock()
	return f.RenameFileResult, f.RenameFileErr
}

// MoveFiles records the call and returns the configured result.
func (f *FakeNodeClient) MoveFiles(_ context.Context, directory string, files []string, destination string, _ node.ProtectionPolicy) ([]string, error) {
	f.mu.Lock()
	copied := append([]string(nil), files...)
	f.MoveFilesCalls = append(f.MoveFilesCalls, MoveFilesCall{Directory: directory, Files: copied, Destination: destination})
	f.mu.Unlock()
	return f.MoveFilesResult, f.MoveFilesErr
}

// CopyFiles records the call and returns the configured result.
func (f *FakeNodeClient) CopyFiles(_ context.Context, directory string, operations []node.CopyFileOperation, policy node.ProtectionPolicy) ([]string, error) {
	f.mu.Lock()
	copiedOperations := append([]node.CopyFileOperation(nil), operations...)
	f.CopyFilesCalls = append(f.CopyFilesCalls, CopyFilesCall{Directory: directory, Operations: copiedOperations, Policy: policy})
	f.mu.Unlock()
	return append([]string(nil), f.CopyFilesResult...), f.CopyFilesErr
}

// DownloadFileFromURL records the call and returns the configured result.
func (f *FakeNodeClient) DownloadFileFromURL(_ context.Context, directory, rawURL, destinationDirectoryPath string, integrity node.DownloadIntegrity, policy node.ProtectionPolicy) (node.DownloadFileResult, error) {
	f.mu.Lock()
	f.DownloadFileFromURLCalls = append(f.DownloadFileFromURLCalls, DownloadFileFromURLCall{
		Directory:                directory,
		RawURL:                   rawURL,
		DestinationDirectoryPath: destinationDirectoryPath,
		Integrity:                integrity,
		Policy:                   policy,
	})
	f.mu.Unlock()
	return f.DownloadFileFromURLResult, f.DownloadFileFromURLErr
}

// CreateFileArchive records the call and returns the configured result.
func (f *FakeNodeClient) CreateFileArchive(_ context.Context, directory string, destinationArchivePath string, includePaths []string, compression node.ArchiveCompression, _ node.ProtectionPolicy) (string, node.ArchiveProgress, error) {
	return f.CreateFileArchiveWithProgress(context.Background(), directory, destinationArchivePath, includePaths, compression, node.ProtectionPolicy{}, nil)
}

// CreateFileArchiveWithProgress records the call and returns the configured result.
func (f *FakeNodeClient) CreateFileArchiveWithProgress(_ context.Context, directory string, destinationArchivePath string, includePaths []string, compression node.ArchiveCompression, _ node.ProtectionPolicy, onProgress func(node.ArchiveProgress) error) (string, node.ArchiveProgress, error) {
	f.mu.Lock()
	copied := append([]string(nil), includePaths...)
	f.CreateFileArchiveCalls = append(f.CreateFileArchiveCalls, CreateFileArchiveCall{
		Directory:              directory,
		DestinationArchivePath: destinationArchivePath,
		IncludePaths:           copied,
		Compression:            compression,
	})
	f.mu.Unlock()
	if onProgress != nil {
		errProgress := onProgress(f.CreateFileArchiveProgress)
		if errProgress != nil {
			return "", node.ArchiveProgress{}, errProgress
		}
	}
	return f.CreateFileArchiveResult, f.CreateFileArchiveProgress, f.CreateFileArchiveErr
}

// ExtractFileArchive records the call and returns the configured result.
func (f *FakeNodeClient) ExtractFileArchive(_ context.Context, directory string, archivePath string, destinationDirectoryPath string, _ node.ProtectionPolicy) ([]string, node.ExtractProgress, error) {
	return f.ExtractFileArchiveWithProgress(context.Background(), directory, archivePath, destinationDirectoryPath, node.ProtectionPolicy{}, nil)
}

// ExtractFileArchiveWithProgress records the call and returns the configured result.
func (f *FakeNodeClient) ExtractFileArchiveWithProgress(_ context.Context, directory string, archivePath string, destinationDirectoryPath string, _ node.ProtectionPolicy, onProgress func(node.ExtractProgress) error) ([]string, node.ExtractProgress, error) {
	f.mu.Lock()
	f.ExtractFileArchiveCalls = append(f.ExtractFileArchiveCalls, ExtractFileArchiveCall{
		Directory:                directory,
		ArchivePath:              archivePath,
		DestinationDirectoryPath: destinationDirectoryPath,
	})
	f.mu.Unlock()
	if onProgress != nil {
		errProgress := onProgress(f.ExtractFileArchiveProgress)
		if errProgress != nil {
			return nil, node.ExtractProgress{}, errProgress
		}
	}
	return append([]string(nil), f.ExtractFileArchiveResult...), f.ExtractFileArchiveProgress, f.ExtractFileArchiveErr
}

// CreateBackupArchive records the call and returns the configured result.
func (f *FakeNodeClient) CreateBackupArchive(ctx context.Context, directory string, includePaths []string, destinationArchivePath string) (int64, string, error) {
	f.mu.Lock()
	copied := append([]string(nil), includePaths...)
	f.CreateBackupArchiveCalls = append(f.CreateBackupArchiveCalls, CreateBackupArchiveCall{
		Directory:              directory,
		IncludePaths:           copied,
		DestinationArchivePath: destinationArchivePath,
	})
	f.mu.Unlock()
	if f.CreateBackupArchiveFunc != nil {
		return f.CreateBackupArchiveFunc(ctx, directory, copied, destinationArchivePath)
	}
	return f.CreateBackupArchiveBytes, f.CreateBackupArchiveSHA256, f.CreateBackupArchiveErr
}

// ExtractBackupArchive records the call and returns the configured error.
func (f *FakeNodeClient) ExtractBackupArchive(_ context.Context, directory string, archivePath string, mode node.ExtractMode) error {
	f.mu.Lock()
	f.ExtractBackupArchiveCalls = append(f.ExtractBackupArchiveCalls, ExtractBackupArchiveCall{
		Directory:   directory,
		ArchivePath: archivePath,
		Mode:        mode,
	})
	f.mu.Unlock()
	return f.ExtractBackupArchiveErr
}

// ProbeInstalledVersion records the call and returns the configured result.
func (f *FakeNodeClient) ProbeInstalledVersion(_ context.Context, req node.InstalledVersionProbeRequest) (node.InstalledVersionProbeResult, error) {
	f.mu.Lock()
	copied := req
	copied.RelativePaths = append([]string(nil), req.RelativePaths...)
	f.ProbeInstalledVersionCalls = append(f.ProbeInstalledVersionCalls, copied)
	f.mu.Unlock()
	return f.ProbeInstalledVersionResult, f.ProbeInstalledVersionErr
}

// QueryGameServer records the call and returns the configured result.
func (f *FakeNodeClient) QueryGameServer(_ context.Context, req node.GameServerQueryRequest) (node.GameServerQueryResult, error) {
	f.mu.Lock()
	f.QueryGameServerCalls = append(f.QueryGameServerCalls, req)
	f.mu.Unlock()
	return f.QueryGameServerResult, f.QueryGameServerErr
}

// QueryPalworldMap records the call and returns the configured snapshot.
func (f *FakeNodeClient) QueryPalworldMap(ctx context.Context, req node.PalworldMapQueryRequest) (*node.PalworldMapSnapshot, error) {
	f.mu.Lock()
	f.QueryPalworldMapCalls = append(f.QueryPalworldMapCalls, req)
	f.mu.Unlock()
	if f.QueryPalworldMapFunc != nil {
		return f.QueryPalworldMapFunc(ctx, req)
	}
	return f.QueryPalworldMapResult, f.QueryPalworldMapErr
}

// QuerySevenDaysToDieMap records the call and returns the configured snapshot.
func (f *FakeNodeClient) QuerySevenDaysToDieMap(ctx context.Context, req node.SevenDaysToDieMapQueryRequest) (*node.SevenDaysToDieMapSnapshot, error) {
	f.mu.Lock()
	f.QuerySevenDaysToDieMapCalls = append(f.QuerySevenDaysToDieMapCalls, req)
	f.mu.Unlock()
	if f.QuerySevenDaysToDieMapFunc != nil {
		return f.QuerySevenDaysToDieMapFunc(ctx, req)
	}
	return f.QuerySevenDaysToDieMapResult, f.QuerySevenDaysToDieMapErr
}

// QuerySevenDaysToDieWebAPIStatus records the call and returns the configured status.
func (f *FakeNodeClient) QuerySevenDaysToDieWebAPIStatus(ctx context.Context, req node.SevenDaysToDieWebAPIStatusQueryRequest) (*node.SevenDaysToDieWebAPIStatus, error) {
	f.mu.Lock()
	f.QuerySevenDaysToDieWebAPIStatusCalls = append(f.QuerySevenDaysToDieWebAPIStatusCalls, req)
	f.mu.Unlock()
	if f.QuerySevenDaysToDieWebAPIStatusFunc != nil {
		return f.QuerySevenDaysToDieWebAPIStatusFunc(ctx, req)
	}
	return f.QuerySevenDaysToDieWebAPIStatusResult, f.QuerySevenDaysToDieWebAPIStatusErr
}

// QuerySevenDaysToDiePlayers records the call and returns the configured roster.
func (f *FakeNodeClient) QuerySevenDaysToDiePlayers(ctx context.Context, req node.SevenDaysToDiePlayersQueryRequest) (*node.SevenDaysToDiePlayers, error) {
	f.mu.Lock()
	f.QuerySevenDaysToDiePlayersCalls = append(f.QuerySevenDaysToDiePlayersCalls, req)
	f.mu.Unlock()
	if f.QuerySevenDaysToDiePlayersFunc != nil {
		return f.QuerySevenDaysToDiePlayersFunc(ctx, req)
	}
	return f.QuerySevenDaysToDiePlayersResult, f.QuerySevenDaysToDiePlayersErr
}

// QuerySevenDaysToDieReportedMods records the call and returns the configured reported mods.
func (f *FakeNodeClient) QuerySevenDaysToDieReportedMods(ctx context.Context, req node.SevenDaysToDieReportedModsQueryRequest) (*node.SevenDaysToDieReportedMods, error) {
	f.mu.Lock()
	f.QuerySevenDaysToDieReportedModsCalls = append(f.QuerySevenDaysToDieReportedModsCalls, req)
	f.mu.Unlock()
	if f.QuerySevenDaysToDieReportedModsFunc != nil {
		return f.QuerySevenDaysToDieReportedModsFunc(ctx, req)
	}
	return f.QuerySevenDaysToDieReportedModsResult, f.QuerySevenDaysToDieReportedModsErr
}

// GetSevenDaysToDieMapTile records the call and returns configured PNG bytes.
func (f *FakeNodeClient) GetSevenDaysToDieMapTile(_ context.Context, req node.SevenDaysToDieMapTileRequest) ([]byte, error) {
	f.mu.Lock()
	f.SevenDaysToDieMapTileCalls = append(f.SevenDaysToDieMapTileCalls, req)
	f.mu.Unlock()
	return append([]byte(nil), f.SevenDaysToDieMapTileResult...), f.SevenDaysToDieMapTileErr
}

// EnsureMinecraftMap records the call and returns the configured map status.
func (f *FakeNodeClient) EnsureMinecraftMap(ctx context.Context, req node.MinecraftMapEnsureRequest) (node.MinecraftMapStatus, error) {
	f.mu.Lock()
	f.EnsureMinecraftMapCalls = append(f.EnsureMinecraftMapCalls, req)
	f.mu.Unlock()
	if f.EnsureMinecraftMapFunc != nil {
		return f.EnsureMinecraftMapFunc(ctx, req)
	}
	return f.EnsureMinecraftMapResult, f.EnsureMinecraftMapErr
}

// StopMinecraftMap records the process whose managed companion was stopped.
func (f *FakeNodeClient) StopMinecraftMap(_ context.Context, processID string) error {
	f.mu.Lock()
	f.StopMinecraftMapCalls = append(f.StopMinecraftMapCalls, processID)
	f.mu.Unlock()
	return f.StopMinecraftMapErr
}

// GetMinecraftMapAsset records the request and returns the configured asset.
func (f *FakeNodeClient) GetMinecraftMapAsset(_ context.Context, req node.MinecraftMapAssetRequest) (node.MinecraftMapAsset, error) {
	f.mu.Lock()
	f.MinecraftMapAssetCalls = append(f.MinecraftMapAssetCalls, req)
	f.mu.Unlock()
	asset := f.MinecraftMapAssetResult
	asset.Content = append([]byte(nil), asset.Content...)
	return asset, f.MinecraftMapAssetErr
}

// PerformGameServerPlayerAction records the call and returns the configured error.
func (f *FakeNodeClient) PerformGameServerPlayerAction(_ context.Context, req node.GameServerPlayerActionRequest) error {
	f.mu.Lock()
	f.PerformGameServerPlayerActionCalls = append(f.PerformGameServerPlayerActionCalls, req)
	f.mu.Unlock()
	return f.PerformGameServerPlayerActionErr
}

// SendConsoleOutput records the call and returns the configured error.
func (f *FakeNodeClient) SendConsoleOutput(_ context.Context, processID, line string) error {
	f.mu.Lock()
	f.SendConsoleOutputCalls = append(f.SendConsoleOutputCalls, SendConsoleOutputCall{
		ProcessID: processID,
		Line:      line,
	})
	f.mu.Unlock()
	return f.SendConsoleOutputErr
}

// GetProcessSnapshot records the call and returns the configured result.
func (f *FakeNodeClient) GetProcessSnapshot(_ context.Context, processID string) (*node.ProcessSnapshot, bool, error) {
	f.mu.Lock()
	f.GetProcessSnapshotCalls = append(f.GetProcessSnapshotCalls, processID)
	f.mu.Unlock()
	return f.GetProcessSnapshotResult, f.GetProcessSnapshotFound, f.GetProcessSnapshotErr
}

// GetNodeSnapshot records the call and returns the configured result.
func (f *FakeNodeClient) GetNodeSnapshot(_ context.Context) (*node.NodeSnapshot, error) {
	f.mu.Lock()
	f.SnapshotCalls++
	f.mu.Unlock()
	return f.SnapshotResult, f.SnapshotErr
}

// ListBindableIPs records the call and returns the configured result.
func (f *FakeNodeClient) ListBindableIPs(_ context.Context) ([]node.BindableIP, error) {
	f.mu.Lock()
	f.BindableIPsCalls++
	f.mu.Unlock()
	return append([]node.BindableIP(nil), f.BindableIPsResult...), f.BindableIPsErr
}

// GetUpdateCapabilities records the call and returns the configured result.
func (f *FakeNodeClient) GetUpdateCapabilities(ctx context.Context) (node.UpdateCapabilities, error) {
	f.mu.Lock()
	f.UpdateCapabilitiesCalls++
	f.mu.Unlock()
	if f.UpdateCapabilitiesFunc != nil {
		return f.UpdateCapabilitiesFunc(ctx)
	}
	return f.UpdateCapabilitiesResult, f.UpdateCapabilitiesErr
}

// GetRuntimeCapabilities records the call and returns the configured result.
func (f *FakeNodeClient) GetRuntimeCapabilities(ctx context.Context) (node.RuntimeCapabilities, error) {
	f.mu.Lock()
	f.RuntimeCapabilitiesCalls++
	f.mu.Unlock()
	if f.RuntimeCapabilitiesFunc != nil {
		return f.RuntimeCapabilitiesFunc(ctx)
	}
	return f.RuntimeCapabilitiesResult, f.RuntimeCapabilitiesErr
}

// StageSelfUpdate records the call and returns the configured result.
func (f *FakeNodeClient) StageSelfUpdate(ctx context.Context, req node.StageSelfUpdateRequest) (node.StageSelfUpdateResult, error) {
	if f.StageSelfUpdateFunc != nil {
		return f.StageSelfUpdateFunc(ctx, req)
	}
	var content []byte
	if req.Reader != nil {
		data, errRead := io.ReadAll(req.Reader)
		if errRead != nil {
			return node.StageSelfUpdateResult{}, fmt.Errorf("nodeclient fake: read stage self-update content: %w", errRead)
		}
		content = data
	}
	recorded := req
	recorded.Reader = nil
	f.mu.Lock()
	f.StageSelfUpdateCalls = append(f.StageSelfUpdateCalls, StageSelfUpdateCall{
		Request: recorded,
		Content: append([]byte(nil), content...),
	})
	f.mu.Unlock()
	return f.StageSelfUpdateResult, f.StageSelfUpdateErr
}

// ApplySelfUpdate records the call and returns the configured result.
func (f *FakeNodeClient) ApplySelfUpdate(ctx context.Context, req node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error) {
	if f.ApplySelfUpdateFunc != nil {
		return f.ApplySelfUpdateFunc(ctx, req)
	}
	f.mu.Lock()
	f.ApplySelfUpdateCalls = append(f.ApplySelfUpdateCalls, req)
	f.mu.Unlock()
	return f.ApplySelfUpdateResult, f.ApplySelfUpdateErr
}

// StreamEvents records the call and returns the configured channel.
func (f *FakeNodeClient) StreamEvents(_ context.Context) (<-chan node.Event, error) {
	f.mu.Lock()
	f.StreamEventsCalls++
	f.mu.Unlock()
	return f.StreamEventsChannel, f.StreamEventsErr
}

// Ping records the call and returns the configured error.
func (f *FakeNodeClient) Ping(_ context.Context) error {
	f.mu.Lock()
	f.PingCalls++
	f.mu.Unlock()
	return f.PingErr
}
