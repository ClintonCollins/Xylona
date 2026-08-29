package nodeclient

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// ErrNodeNil is returned when an inProcessNodeClient is constructed without
// an underlying *node.Node.
var ErrNodeNil = errors.New("nodeclient: underlying node is nil")

// ErrUpdateUnsupported is returned by clients that cannot self-update through
// the node update RPC surface.
var ErrUpdateUnsupported = errors.New("nodeclient: update unsupported")

// inProcessNodeClient implements NodeClient by delegating to an in-process
// *internal/node.Node. Used for the controller's embedded node.
type inProcessNodeClient struct {
	id   string
	node *node.Node
}

// NewInProcessClient returns a NodeClient backed by an in-process Node. The
// returned client is safe for concurrent use by multiple goroutines.
func NewInProcessClient(nodeID string, n *node.Node) NodeClient {
	return &inProcessNodeClient{
		id:   nodeID,
		node: n,
	}
}

func (c *inProcessNodeClient) ID() string {
	return c.id
}

func (c *inProcessNodeClient) StartProcess(_ context.Context, cfg node.ProcessConfig, status xylona.Status) error {
	if c.node == nil {
		return ErrNodeNil
	}
	_, errStart := c.node.StartProcess(cfg, status)
	if errStart != nil {
		return fmt.Errorf("nodeclient: start process: %w", errStart)
	}
	return nil
}

func (c *inProcessNodeClient) StopProcess(_ context.Context, processID, stopInputCommand string) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errStop := c.node.StopProcess(processID, stopInputCommand)
	if errStop != nil {
		return fmt.Errorf("nodeclient: stop process: %w", errStop)
	}
	return nil
}

func (c *inProcessNodeClient) SendConsoleInput(ctx context.Context, processID, input string) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errSend := c.node.SendConsoleInputContext(ctx, processID, input)
	if errSend != nil {
		return fmt.Errorf("nodeclient: send console input: %w", errSend)
	}
	return nil
}

func (c *inProcessNodeClient) ReadConsoleBuffer(ctx context.Context, processID string) (node.ConsoleChunk, error) {
	errContext := ctx.Err()
	if errContext != nil {
		return node.ConsoleChunk{ProcessID: processID}, fmt.Errorf("nodeclient: read console buffer: %w", errContext)
	}
	if c.node == nil {
		return node.ConsoleChunk{ProcessID: processID}, ErrNodeNil
	}
	return c.node.ReadConsoleBuffer(processID), nil
}

func (c *inProcessNodeClient) StreamConsoleOutput(ctx context.Context, processID string) (<-chan node.ConsoleChunk, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}

	stream, errStream := c.node.StreamConsoleOutput(ctx, processID, true)
	if errStream != nil {
		return nil, fmt.Errorf("nodeclient: stream console output: %w", errStream)
	}
	return stream, nil
}

func (c *inProcessNodeClient) ListFiles(_ context.Context, directory, relativePath string) ([]node.FileEntry, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	entries, errList := c.node.ListFiles(directory, relativePath)
	if errList != nil {
		return nil, fmt.Errorf("nodeclient: list files: %w", errList)
	}
	return entries, nil
}

func (c *inProcessNodeClient) ReadFile(_ context.Context, directory, relativePath string) ([]byte, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	data, errRead := c.node.ReadFile(directory, relativePath)
	if errRead != nil {
		return nil, fmt.Errorf("nodeclient: read file: %w", errRead)
	}
	return data, nil
}

func (c *inProcessNodeClient) StatFile(_ context.Context, directory, relativePath string) (node.FileEntry, error) {
	if c.node == nil {
		return node.FileEntry{}, ErrNodeNil
	}
	entry, errStat := c.node.StatFile(directory, relativePath)
	if errStat != nil {
		return node.FileEntry{}, fmt.Errorf("nodeclient: stat file: %w", errStat)
	}
	return entry, nil
}

func (c *inProcessNodeClient) StreamFile(_ context.Context, directory, relativePath string) (io.ReadCloser, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	reader, errOpen := c.node.OpenFile(directory, relativePath)
	if errOpen != nil {
		return nil, fmt.Errorf("nodeclient: stream file: %w", errOpen)
	}
	return reader, nil
}

func (c *inProcessNodeClient) WriteFile(_ context.Context, directory, relativePath string, content []byte, policy node.ProtectionPolicy) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errWrite := c.node.WriteFile(directory, relativePath, content, policy)
	if errWrite != nil {
		return fmt.Errorf("nodeclient: write file: %w", errWrite)
	}
	return nil
}

func (c *inProcessNodeClient) StreamWriteFile(_ context.Context, directory, relativePath string, reader io.Reader, policy node.ProtectionPolicy) (node.WriteFileResult, error) {
	if c.node == nil {
		return node.WriteFileResult{}, ErrNodeNil
	}
	result, errWrite := c.node.WriteFileFromReader(directory, relativePath, reader, policy)
	if errWrite != nil {
		return node.WriteFileResult{}, fmt.Errorf("nodeclient: stream write file: %w", errWrite)
	}
	return result, nil
}

func (c *inProcessNodeClient) CreateFileOrDirectory(_ context.Context, directory, relativePath, content string, isDirectory bool, policy node.ProtectionPolicy) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errCreate := c.node.CreateFileOrDirectory(directory, relativePath, content, isDirectory, policy)
	if errCreate != nil {
		return fmt.Errorf("nodeclient: create file or directory: %w", errCreate)
	}
	return nil
}

func (c *inProcessNodeClient) DeleteFiles(ctx context.Context, directory string, files []string, policy node.ProtectionPolicy) ([]string, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	deleted, errDelete := c.node.DeleteFiles(ctx, directory, files, policy)
	if errDelete != nil {
		return nil, fmt.Errorf("nodeclient: delete files: %w", errDelete)
	}
	return deleted, nil
}

func (c *inProcessNodeClient) RenameFile(_ context.Context, directory, oldRelativePath, newRelativePath string, policy node.ProtectionPolicy) (string, error) {
	if c.node == nil {
		return "", ErrNodeNil
	}
	renamed, errRename := c.node.RenameFile(directory, oldRelativePath, newRelativePath, policy)
	if errRename != nil {
		return "", fmt.Errorf("nodeclient: rename file: %w", errRename)
	}
	return renamed, nil
}

func (c *inProcessNodeClient) MoveFiles(ctx context.Context, directory string, files []string, destination string, policy node.ProtectionPolicy) ([]string, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	moved, errMove := c.node.MoveFiles(ctx, directory, files, destination, policy)
	if errMove != nil {
		return nil, fmt.Errorf("nodeclient: move files: %w", errMove)
	}
	return moved, nil
}

func (c *inProcessNodeClient) CopyFiles(ctx context.Context, directory string, operations []node.CopyFileOperation, policy node.ProtectionPolicy) ([]string, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	copied, errCopy := c.node.CopyFiles(ctx, directory, operations, policy)
	if errCopy != nil {
		return nil, fmt.Errorf("nodeclient: copy files: %w", errCopy)
	}
	return copied, nil
}

func (c *inProcessNodeClient) DownloadFileFromURL(ctx context.Context, directory, rawURL, destinationDirectoryPath string, integrity node.DownloadIntegrity, policy node.ProtectionPolicy) (node.DownloadFileResult, error) {
	if c.node == nil {
		return node.DownloadFileResult{}, ErrNodeNil
	}
	downloaded, errDownload := c.node.DownloadFileFromURL(ctx, directory, rawURL, destinationDirectoryPath, integrity, policy)
	if errDownload != nil {
		return node.DownloadFileResult{}, fmt.Errorf("nodeclient: download file from URL: %w", errDownload)
	}
	return downloaded, nil
}

func (c *inProcessNodeClient) CreateFileArchive(ctx context.Context, directory string, destinationArchivePath string, includePaths []string, compression node.ArchiveCompression, policy node.ProtectionPolicy) (string, node.ArchiveProgress, error) {
	return c.CreateFileArchiveWithProgress(ctx, directory, destinationArchivePath, includePaths, compression, policy, nil)
}

func (c *inProcessNodeClient) CreateFileArchiveWithProgress(ctx context.Context, directory string, destinationArchivePath string, includePaths []string, compression node.ArchiveCompression, policy node.ProtectionPolicy, onProgress func(node.ArchiveProgress) error) (string, node.ArchiveProgress, error) {
	if c.node == nil {
		return "", node.ArchiveProgress{}, ErrNodeNil
	}
	archivePath, progress, errArchive := c.node.CreateFileArchiveWithProgress(ctx, directory, destinationArchivePath, includePaths, compression, policy, onProgress)
	if errArchive != nil {
		return "", node.ArchiveProgress{}, fmt.Errorf("nodeclient: create file archive: %w", errArchive)
	}
	return archivePath, progress, nil
}

func (c *inProcessNodeClient) ExtractFileArchive(ctx context.Context, directory string, archivePath string, destinationDirectoryPath string, policy node.ProtectionPolicy) ([]string, node.ExtractProgress, error) {
	return c.ExtractFileArchiveWithProgress(ctx, directory, archivePath, destinationDirectoryPath, policy, nil)
}

func (c *inProcessNodeClient) ExtractFileArchiveWithProgress(ctx context.Context, directory string, archivePath string, destinationDirectoryPath string, policy node.ProtectionPolicy, onProgress func(node.ExtractProgress) error) ([]string, node.ExtractProgress, error) {
	if c.node == nil {
		return nil, node.ExtractProgress{}, ErrNodeNil
	}
	extracted, progress, errExtract := c.node.ExtractFileArchiveWithProgress(ctx, directory, archivePath, destinationDirectoryPath, policy, onProgress)
	if errExtract != nil {
		return nil, node.ExtractProgress{}, fmt.Errorf("nodeclient: extract file archive: %w", errExtract)
	}
	return extracted, progress, nil
}

func (c *inProcessNodeClient) CreateBackupArchive(ctx context.Context, directory string, includePaths []string, destinationArchivePath string) (int64, string, error) {
	if c.node == nil {
		return 0, "", ErrNodeNil
	}
	bytesWritten, sum, errArchive := c.node.CreateBackupArchive(ctx, directory, includePaths, destinationArchivePath)
	if errArchive != nil {
		return 0, "", fmt.Errorf("nodeclient: create backup archive: %w", errArchive)
	}
	return bytesWritten, sum, nil
}

func (c *inProcessNodeClient) ExtractBackupArchive(ctx context.Context, directory string, archivePath string, mode node.ExtractMode) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errExtract := c.node.ExtractBackupArchive(ctx, directory, archivePath, mode)
	if errExtract != nil {
		return fmt.Errorf("nodeclient: extract backup archive: %w", errExtract)
	}
	return nil
}

func (c *inProcessNodeClient) ProbeInstalledVersion(_ context.Context, req node.InstalledVersionProbeRequest) (node.InstalledVersionProbeResult, error) {
	if c.node == nil {
		return node.InstalledVersionProbeResult{}, ErrNodeNil
	}
	result, errProbe := c.node.ProbeInstalledVersion(req)
	if errProbe != nil {
		return node.InstalledVersionProbeResult{}, fmt.Errorf("nodeclient: probe installed version: %w", errProbe)
	}
	return result, nil
}

func (c *inProcessNodeClient) QueryGameServer(ctx context.Context, req node.GameServerQueryRequest) (node.GameServerQueryResult, error) {
	if c.node == nil {
		return node.GameServerQueryResult{}, ErrNodeNil
	}
	result, errQuery := c.node.QueryGameServer(ctx, req)
	if errQuery != nil {
		return node.GameServerQueryResult{}, fmt.Errorf("nodeclient: query game server: %w", errQuery)
	}
	return result, nil
}

func (c *inProcessNodeClient) QueryPalworldMap(ctx context.Context, req node.PalworldMapQueryRequest) (*node.PalworldMapSnapshot, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	snapshot, errQuery := c.node.QueryPalworldMap(ctx, req)
	if errQuery != nil {
		return nil, fmt.Errorf("nodeclient: query palworld map: %w", errQuery)
	}
	return snapshot, nil
}

func (c *inProcessNodeClient) QuerySevenDaysToDieMap(ctx context.Context, req node.SevenDaysToDieMapQueryRequest) (*node.SevenDaysToDieMapSnapshot, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	snapshot, errQuery := c.node.QuerySevenDaysToDieMap(ctx, req)
	if errQuery != nil {
		return nil, fmt.Errorf("nodeclient: query 7 Days to Die map: %w", errQuery)
	}
	return snapshot, nil
}

func (c *inProcessNodeClient) QuerySevenDaysToDieWebAPIStatus(ctx context.Context, req node.SevenDaysToDieWebAPIStatusQueryRequest) (*node.SevenDaysToDieWebAPIStatus, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	status, errQuery := c.node.QuerySevenDaysToDieWebAPIStatus(ctx, req)
	if errQuery != nil {
		return nil, fmt.Errorf("nodeclient: query 7 Days to Die WebAPI status: %w", errQuery)
	}
	return status, nil
}

func (c *inProcessNodeClient) QuerySevenDaysToDiePlayers(ctx context.Context, req node.SevenDaysToDiePlayersQueryRequest) (*node.SevenDaysToDiePlayers, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	result, errQuery := c.node.QuerySevenDaysToDiePlayers(ctx, req)
	if errQuery != nil {
		return nil, fmt.Errorf("nodeclient: query 7 Days to Die players: %w", errQuery)
	}
	return result, nil
}

func (c *inProcessNodeClient) QuerySevenDaysToDieReportedMods(ctx context.Context, req node.SevenDaysToDieReportedModsQueryRequest) (*node.SevenDaysToDieReportedMods, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	result, errQuery := c.node.QuerySevenDaysToDieReportedMods(ctx, req)
	if errQuery != nil {
		return nil, fmt.Errorf("nodeclient: query 7 Days to Die reported mods: %w", errQuery)
	}
	return result, nil
}

// QuerySevenDaysToDieSandboxSettings queries the embedded node directly.
func (c *inProcessNodeClient) QuerySevenDaysToDieSandboxSettings(ctx context.Context, req node.SevenDaysToDieSandboxSettingsQueryRequest) (*node.SevenDaysToDieSandboxSettings, error) {
	if c == nil || c.node == nil {
		return nil, ErrNodeNil
	}
	result, errQuery := c.node.QuerySevenDaysToDieSandboxSettings(ctx, req)
	if errQuery != nil {
		return nil, fmt.Errorf("nodeclient: query 7 Days to Die sandbox settings: %w", errQuery)
	}
	return result, nil
}

func (c *inProcessNodeClient) GetSevenDaysToDieMapTile(ctx context.Context, req node.SevenDaysToDieMapTileRequest) ([]byte, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	content, errTile := c.node.GetSevenDaysToDieMapTile(ctx, req)
	if errTile != nil {
		return nil, fmt.Errorf("nodeclient: get 7 Days to Die map tile: %w", errTile)
	}
	return content, nil
}

func (c *inProcessNodeClient) EnsureMinecraftMap(ctx context.Context, req node.MinecraftMapEnsureRequest) (node.MinecraftMapStatus, error) {
	if c.node == nil {
		return node.MinecraftMapStatus{}, ErrNodeNil
	}
	status, errEnsure := c.node.EnsureMinecraftMap(ctx, req)
	if errEnsure != nil {
		return node.MinecraftMapStatus{}, fmt.Errorf("nodeclient: ensure Minecraft map: %w", errEnsure)
	}
	return status, nil
}

func (c *inProcessNodeClient) StopMinecraftMap(ctx context.Context, processID string) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errStop := c.node.StopMinecraftMap(ctx, processID)
	if errStop != nil {
		return fmt.Errorf("nodeclient: stop Minecraft map: %w", errStop)
	}
	return nil
}

func (c *inProcessNodeClient) GetMinecraftMapAsset(ctx context.Context, req node.MinecraftMapAssetRequest) (node.MinecraftMapAsset, error) {
	if c.node == nil {
		return node.MinecraftMapAsset{}, ErrNodeNil
	}
	asset, errAsset := c.node.GetMinecraftMapAsset(ctx, req)
	if errAsset != nil {
		return node.MinecraftMapAsset{}, fmt.Errorf("nodeclient: get Minecraft map asset: %w", errAsset)
	}
	return asset, nil
}

func (c *inProcessNodeClient) PerformGameServerPlayerAction(ctx context.Context, req node.GameServerPlayerActionRequest) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errAction := c.node.PerformGameServerPlayerAction(ctx, req)
	if errAction != nil {
		return fmt.Errorf("nodeclient: perform game server player action: %w", errAction)
	}
	return nil
}

func (c *inProcessNodeClient) ExecuteGameOperation(ctx context.Context, req node.GameOperationRequest) (node.GameOperationResult, error) {
	if c.node == nil {
		return node.GameOperationResult{}, ErrNodeNil
	}
	return c.node.ExecuteGameOperation(ctx, req), nil
}

func (c *inProcessNodeClient) SendConsoleOutput(_ context.Context, processID, line string) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errSend := c.node.SendConsoleOutput(processID, line)
	if errSend != nil {
		return fmt.Errorf("nodeclient: send console output: %w", errSend)
	}
	return nil
}

func (c *inProcessNodeClient) GetProcessSnapshot(_ context.Context, processID string) (*node.ProcessSnapshot, bool, error) {
	if c.node == nil {
		return nil, false, ErrNodeNil
	}
	snap, found, errSnap := c.node.GetProcessSnapshot(processID)
	if errSnap != nil {
		return nil, false, fmt.Errorf("nodeclient: get process snapshot: %w", errSnap)
	}
	return snap, found, nil
}

func (c *inProcessNodeClient) GetNodeSnapshot(ctx context.Context) (*node.NodeSnapshot, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	snapshot, errSnapshot := c.node.GetNodeSnapshot(ctx)
	if errSnapshot != nil {
		return nil, fmt.Errorf("nodeclient: get node snapshot: %w", errSnapshot)
	}
	return snapshot, nil
}

func (c *inProcessNodeClient) ListBindableIPs(_ context.Context) ([]node.BindableIP, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}

	rawIPs, errIPs := helpers.GetBindableIPs()
	if errIPs != nil {
		return nil, fmt.Errorf("nodeclient: list bindable IPs: %w", errIPs)
	}

	ips := make([]node.BindableIP, 0, len(rawIPs))
	for _, rawIP := range rawIPs {
		if rawIP == nil {
			continue
		}
		ips = append(ips, node.BindableIP{
			Address:  rawIP.String(),
			Usable:   true,
			External: !rawIP.IsPrivate(),
		})
	}
	return ips, nil
}

func (c *inProcessNodeClient) GetUpdateCapabilities(_ context.Context) (node.UpdateCapabilities, error) {
	if c.node == nil {
		return node.UpdateCapabilities{}, ErrNodeNil
	}
	return node.UpdateCapabilities{
		Supported: false,
		Reason:    "embedded node updates are handled by controller self-update",
		Component: "node",
	}, nil
}

func (c *inProcessNodeClient) GetRuntimeCapabilities(_ context.Context) (node.RuntimeCapabilities, error) {
	if c.node == nil {
		return node.RuntimeCapabilities{}, ErrNodeNil
	}
	return c.node.RuntimeCapabilities(), nil
}

func (c *inProcessNodeClient) StageSelfUpdate(_ context.Context, _ node.StageSelfUpdateRequest) (node.StageSelfUpdateResult, error) {
	if c.node == nil {
		return node.StageSelfUpdateResult{}, ErrNodeNil
	}
	return node.StageSelfUpdateResult{}, ErrUpdateUnsupported
}

func (c *inProcessNodeClient) ApplySelfUpdate(_ context.Context, _ node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error) {
	if c.node == nil {
		return node.ApplySelfUpdateResult{}, ErrNodeNil
	}
	return node.ApplySelfUpdateResult{}, ErrUpdateUnsupported
}

// StreamEvents bridges the underlying node's EventEmitter to a per-call
// channel, unsubscribing when ctx is canceled so callers do not leak
// subscriber buffers. The returned channel is closed after unsubscribe.
func (c *inProcessNodeClient) StreamEvents(ctx context.Context) (<-chan node.Event, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}

	emitter := c.node.Events()
	if emitter == nil {
		return nil, errors.New("nodeclient: node has no event emitter")
	}

	subscription := emitter.SubscribeWithReplay(true)
	out := make(chan node.Event, cap(subscription))

	go func() {
		defer close(out)
		defer emitter.Unsubscribe(subscription)

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-subscription:
				if !ok {
					return
				}
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// Ping for the in-process client is a trivial success: if the caller has a
// handle, the node is reachable. Returns ctx.Err() if ctx is already done.
func (c *inProcessNodeClient) Ping(ctx context.Context) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errCtx := ctx.Err()
	if errCtx != nil {
		return fmt.Errorf("nodeclient: ping: %w", errCtx)
	}
	return nil
}
