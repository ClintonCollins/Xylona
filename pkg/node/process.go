package node

import (
	"errors"
	"fmt"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// StartProcess launches the process described by config and returns the
// supervisor command for further interaction (status, console listeners).
//
// The xylona.Status passed in determines the supervisor lifecycle slot
// (ONLINE for game server start, INSTALLING/UPDATING for internal commands,
// etc.). This method is a thin wrapper around supervisor.StartCommand and
// does no policy of its own.
func (n *Node) StartProcess(config ProcessConfig, status xylona.Status) (*supervisor.Command, error) {
	if n.supervisor == nil {
		return nil, errors.New("node: supervisor not configured")
	}

	normalized := config.normalize()
	prepared := supervisor.PreparedCommand{
		ID:               normalized.ID,
		GameServerName:   normalized.Name,
		BaseCommand:      normalized.BaseCommand,
		Args:             normalized.Args,
		WorkingDirectory: normalized.WorkingDirectory,
		User:             normalized.User,
		NodeID:           normalized.NodeID,
		ServiceID:        normalized.ServiceID,
		Status:           status,
		StopTimeout:      normalized.StopTimeout,
	}

	if normalized.InputTelnet != nil {
		prepared.InputMethod = supervisor.InputMethod{
			Type: supervisor.InputTypeTelnet,
			TelnetCredentials: &supervisor.TelnetCredentials{
				Port:     normalized.InputTelnet.Port,
				Password: normalized.InputTelnet.Password,
			},
		}
	}

	if normalized.InternalCommand {
		prepared.InternalCommand = true
		// Prefer the caller-supplied model; fall back to a DB lookup by
		// InternalGameServerID.
		if suppliedGS, ok := normalized.InternalGameServer.(*models.GameServer); ok && suppliedGS != nil {
			prepared.InternalGameServer = suppliedGS
		} else if normalized.InternalGameServerID != "" && n.db != nil {
			lookedUpGS, errGet := n.db.GetGameServerByID(normalized.InternalGameServerID)
			if errGet == nil {
				prepared.InternalGameServer = lookedUpGS
			}
		}
		if normalized.InternalGameID != "" {
			gameID := normalized.InternalGameID
			prepared.GameID = &gameID
		}
	}

	cmd, errStart := n.supervisor.StartCommand(prepared)
	if errStart != nil {
		return nil, fmt.Errorf("node: start process: %w", errStart)
	}
	return cmd, nil
}

// StopProcess requests a graceful stop of the supervised command identified by
// processID. The optional stopInputCommand is sent on stdin before the
// supervisor falls back to signal-based termination.
func (n *Node) StopProcess(processID, stopInputCommand string) error {
	if n.supervisor == nil {
		return errors.New("node: supervisor not configured")
	}

	cmd, errGet := n.supervisor.GetCommandByID(processID)
	if errGet != nil {
		return fmt.Errorf("node: stop process: %w", errGet)
	}
	cmd.Stop(stopInputCommand)
	return nil
}

// SendConsoleInput writes a single line of input to the running process's
// configured input writer (stdin or telnet, depending on InputMethod).
func (n *Node) SendConsoleInput(processID, input string) error {
	if n.supervisor == nil {
		return errors.New("node: supervisor not configured")
	}

	cmd, errGet := n.supervisor.GetCommandByID(processID)
	if errGet != nil {
		return fmt.Errorf("node: send console input: %w", errGet)
	}
	errSend := cmd.SendInput(input)
	if errSend != nil {
		return fmt.Errorf("node: send console input: %w", errSend)
	}
	return nil
}

// ReadConsoleBuffer returns the supervisor's buffered console output for the
// given process. Returns an empty ConsoleChunk when the process is unknown so
// callers do not need to special-case missing servers.
func (n *Node) ReadConsoleBuffer(processID string) ConsoleChunk {
	if n.supervisor == nil {
		return ConsoleChunk{ProcessID: processID}
	}

	cmd, errGet := n.supervisor.GetCommandByID(processID)
	if errGet != nil {
		return ConsoleChunk{ProcessID: processID}
	}
	return ConsoleChunk{
		ProcessID: processID,
		Data:      cmd.GetOutputBuffer(),
	}
}

// SendConsoleOutput writes a controller-generated line into the process's
// console buffer. Creates a shell slot if the process doesn't exist yet so
// pre-start messages can be shown in the UI before the process launches.
// Acts as a no-op when the node's supervisor is unconfigured.
func (n *Node) SendConsoleOutput(processID, line string) error {
	if n.supervisor == nil {
		return errors.New("node: supervisor not configured")
	}
	n.supervisor.SendConsoleOutput(processID, line)
	return nil
}

// GetProcessSnapshot returns the metrics + status for one process. Returns
// (nil, false, nil) when the process is not currently tracked, so callers
// can distinguish "not found" from transport errors.
func (n *Node) GetProcessSnapshot(processID string) (*ProcessSnapshot, bool, error) {
	if n.supervisor == nil {
		return nil, false, nil
	}
	cmd, errGet := n.supervisor.GetCommandByID(processID)
	if errGet != nil {
		// Supervisor returns ErrCommandDoesNotExist for untracked processes.
		// Callers treat found=false as "not tracked" rather than an error, so
		// we intentionally squash the lookup error.
		return nil, false, nil //nolint:nilerr // intentional not-found signal
	}
	cpuPercent, memoryRSS, memoryVMS, memoryPercent, cpuCores, numThreads, diskUsageBytes, ioReadRate, ioWriteRate, connectionCount := cmd.Metrics()
	return &ProcessSnapshot{
		ID:              cmd.ID,
		Name:            cmd.GameServerName(),
		Status:          cmd.Status().String(),
		UnixStartedAt:   cmd.UnixStartedAt(),
		CPUPercent:      cpuPercent,
		CPUCores:        cpuCores,
		MemoryRSS:       memoryRSS,
		MemoryVMS:       memoryVMS,
		MemoryPercent:   memoryPercent,
		NumThreads:      numThreads,
		DiskUsageBytes:  diskUsageBytes,
		IOReadRate:      ioReadRate,
		IOWriteRate:     ioWriteRate,
		ConnectionCount: connectionCount,
		WorkingDir:      cmd.WorkingDir(),
	}, true, nil
}
