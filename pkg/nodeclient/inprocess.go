package nodeclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// ErrNodeNil is returned when an inProcessNodeClient is constructed without
// an underlying *node.Node.
var ErrNodeNil = errors.New("nodeclient: underlying node is nil")

// inProcessNodeClient implements NodeClient by delegating to an in-process
// *pkg/node.Node. Used for the controller's embedded node.
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

func (c *inProcessNodeClient) StartProcess(_ context.Context, cfg node.ProcessConfig, status xylona.Status) (*supervisor.Command, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}
	cmd, errStart := c.node.StartProcess(cfg, status)
	if errStart != nil {
		return nil, fmt.Errorf("nodeclient: start process: %w", errStart)
	}
	return cmd, nil
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

func (c *inProcessNodeClient) SendConsoleInput(_ context.Context, processID, input string) error {
	if c.node == nil {
		return ErrNodeNil
	}
	errSend := c.node.SendConsoleInput(processID, input)
	if errSend != nil {
		return fmt.Errorf("nodeclient: send console input: %w", errSend)
	}
	return nil
}

func (c *inProcessNodeClient) ReadConsoleBuffer(_ context.Context, processID string) (node.ConsoleChunk, error) {
	if c.node == nil {
		return node.ConsoleChunk{ProcessID: processID}, ErrNodeNil
	}
	return c.node.ReadConsoleBuffer(processID), nil
}

func (c *inProcessNodeClient) StreamConsoleOutput(ctx context.Context, processID string) (<-chan node.ConsoleChunk, error) {
	if c.node == nil {
		return nil, ErrNodeNil
	}

	supervisorInst := c.node.Supervisor()
	if supervisorInst == nil {
		return nil, errors.New("nodeclient: node has no supervisor")
	}

	command := supervisorInst.GetCommandByIDOrCreateShell(processID)
	listenerID := fmt.Sprintf("nodeclient-console-%s-%d", processID, time.Now().UnixNano())
	listener := make(chan *xylona.Message, 256)
	command.AddOutputListener(listenerID, listener)

	out := make(chan node.ConsoleChunk, 64)
	go func() {
		defer close(out)
		defer command.RemoveOutputListener(listenerID)

		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-listener:
				if msg == nil || msg.GetType() != xylona.Message_GameServerConsole {
					continue
				}

				consoleOutput := msg.GetGameServerConsoleOutput()
				if consoleOutput == nil {
					continue
				}

				chunk := node.ConsoleChunk{
					ProcessID: consoleOutput.GetGameServerId(),
					Data:      consoleOutput.GetOutput(),
				}

				select {
				case <-ctx.Done():
					return
				case out <- chunk:
				}
			}
		}
	}()

	return out, nil
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

func (c *inProcessNodeClient) DownloadFileFromURL(ctx context.Context, directory, rawURL, destinationDirectoryPath string, policy node.ProtectionPolicy) (string, error) {
	if c.node == nil {
		return "", ErrNodeNil
	}
	downloaded, errDownload := c.node.DownloadFileFromURL(ctx, directory, rawURL, destinationDirectoryPath, policy)
	if errDownload != nil {
		return "", fmt.Errorf("nodeclient: download file from URL: %w", errDownload)
	}
	return downloaded, nil
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

	subscription := emitter.Subscribe()
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
