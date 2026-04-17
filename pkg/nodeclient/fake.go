package nodeclient

import (
	"context"
	"sync"

	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/supervisor"
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

	StartProcessCmd   *supervisor.Command
	StartProcessErr   error
	StartProcessCalls []StartProcessCall

	StopProcessErr   error
	StopProcessCalls []StopProcessCall

	SendConsoleInputErr   error
	SendConsoleInputCalls []SendConsoleInputCall

	ReadConsoleBufferResult node.ConsoleChunk
	ReadConsoleBufferErr    error
	ReadConsoleBufferCalls  []string

	ListFilesResult []node.FileEntry
	ListFilesErr    error
	ListFilesCalls  []ListFilesCall

	ReadFileResult []byte
	ReadFileErr    error
	ReadFileCalls  []ListFilesCall

	WriteFileErr   error
	WriteFileCalls []WriteFileCall

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

	DownloadFileFromURLResult string
	DownloadFileFromURLErr    error
	DownloadFileFromURLCalls  []DownloadFileFromURLCall

	CreateBackupArchiveBytes  int64
	CreateBackupArchiveSHA256 string
	CreateBackupArchiveErr    error
	CreateBackupArchiveCalls  []CreateBackupArchiveCall

	ExtractBackupArchiveErr   error
	ExtractBackupArchiveCalls []ExtractBackupArchiveCall

	SendConsoleOutputErr   error
	SendConsoleOutputCalls []SendConsoleOutputCall

	GetProcessSnapshotResult *node.ProcessSnapshot
	GetProcessSnapshotFound  bool
	GetProcessSnapshotErr    error
	GetProcessSnapshotCalls  []string

	SnapshotResult *node.NodeSnapshot
	SnapshotErr    error
	SnapshotCalls  int

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

// DownloadFileFromURLCall records a single DownloadFileFromURL invocation.
type DownloadFileFromURLCall struct {
	Directory                string
	RawURL                   string
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

// ID returns the configured NodeID (zero-value-safe: empty string if unset).
func (f *FakeNodeClient) ID() string {
	return f.NodeID
}

// StartProcess records the call and returns the configured result.
func (f *FakeNodeClient) StartProcess(_ context.Context, cfg node.ProcessConfig, status xylona.Status) (*supervisor.Command, error) {
	f.mu.Lock()
	f.StartProcessCalls = append(f.StartProcessCalls, StartProcessCall{Config: cfg, Status: status})
	f.mu.Unlock()
	return f.StartProcessCmd, f.StartProcessErr
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

// WriteFile records the call and returns the configured error.
func (f *FakeNodeClient) WriteFile(_ context.Context, directory, relativePath string, content []byte, _ node.ProtectionPolicy) error {
	f.mu.Lock()
	// Copy content to avoid callers mutating recorded bytes afterwards.
	copied := append([]byte(nil), content...)
	f.WriteFileCalls = append(f.WriteFileCalls, WriteFileCall{Directory: directory, RelativePath: relativePath, Content: copied})
	f.mu.Unlock()
	return f.WriteFileErr
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

// DownloadFileFromURL records the call and returns the configured result.
func (f *FakeNodeClient) DownloadFileFromURL(_ context.Context, directory, rawURL, destinationDirectoryPath string, _ node.ProtectionPolicy) (string, error) {
	f.mu.Lock()
	f.DownloadFileFromURLCalls = append(f.DownloadFileFromURLCalls, DownloadFileFromURLCall{
		Directory:                directory,
		RawURL:                   rawURL,
		DestinationDirectoryPath: destinationDirectoryPath,
	})
	f.mu.Unlock()
	return f.DownloadFileFromURLResult, f.DownloadFileFromURLErr
}

// CreateBackupArchive records the call and returns the configured result.
func (f *FakeNodeClient) CreateBackupArchive(_ context.Context, directory string, includePaths []string, destinationArchivePath string) (int64, string, error) {
	f.mu.Lock()
	copied := append([]string(nil), includePaths...)
	f.CreateBackupArchiveCalls = append(f.CreateBackupArchiveCalls, CreateBackupArchiveCall{
		Directory:              directory,
		IncludePaths:           copied,
		DestinationArchivePath: destinationArchivePath,
	})
	f.mu.Unlock()
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
