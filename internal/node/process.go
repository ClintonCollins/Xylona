package node

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/internal/valheim"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// StartProcess launches the process described by config and returns the
// supervisor command for further interaction (status, console listeners).
//
// The xylona.Status passed in determines the supervisor lifecycle slot
// (ONLINE for game server start, INSTALLING/UPDATING for internal commands,
// etc.). This method is a thin wrapper around supervisor.StartCommand and
// does no policy of its own.
func (n *Node) StartProcess(config ProcessConfig, status xylona.Status) (*supervisor.Command, error) {
	if config.RuntimeMode == valheim.NativeRuntime {
		for index, arg := range config.Args {
			if strings.EqualFold(arg, "-password") && index+1 < len(config.Args) {
				config.RedactValues = append(config.RedactValues, config.Args[index+1])
			}
		}
	}
	if n.supervisor == nil {
		return nil, errors.New("node: supervisor not configured")
	}

	unlockLifecycle, errLifecycle := n.lockServerLifecycle(config.WorkingDirectory)
	if errLifecycle != nil {
		return nil, errLifecycle
	}
	defer unlockLifecycle()
	normalized := config.normalize()
	launchEnvironmentIssues := launchenv.ValidateMap(normalized.LaunchEnv)
	errLaunchEnvironment := launchenv.NewValidationError(launchEnvironmentIssues)
	if errLaunchEnvironment != nil {
		return nil, fmt.Errorf("node: validate launch environment: %w", errLaunchEnvironment)
	}
	if normalized.ExecutionID == "" {
		normalized.ExecutionID = uuid.NewString()
	}
	prepared := supervisor.PreparedCommand{
		RedactValues:         slices.Clone(normalized.RedactValues),
		ID:                   normalized.ID,
		ExecutionID:          normalized.ExecutionID,
		GameServerName:       normalized.Name,
		BaseCommand:          normalized.BaseCommand,
		Args:                 normalized.Args,
		WorkingDirectory:     normalized.WorkingDirectory,
		User:                 normalized.User,
		NodeID:               normalized.NodeID,
		ServiceID:            normalized.ServiceID,
		Status:               status,
		StopTimeout:          normalized.StopTimeout,
		LaunchEnv:            normalized.LaunchEnv,
		SuppressStatusEvents: normalized.SuppressStatusEvents,
	}

	if normalized.Readiness != nil && status == xylona.Status_ONLINE {
		readiness, errReadiness := n.supervisorReadiness(*normalized.Readiness)
		if errReadiness != nil {
			return nil, errReadiness
		}
		prepared.Readiness = readiness
	}

	if normalized.RuntimeMode != "" {
		if normalized.RuntimeMode != valheim.NativeRuntime || normalized.GameID != "valheim" || status != xylona.Status_ONLINE || normalized.InternalCommand {
			return nil, errors.New("unsupported managed runtime mode")
		}
		root, executable, trusted, errPrepare := valheim.PrepareNative(runtime.GOOS, normalized.WorkingDirectory, normalized.BaseCommand, normalized.ID, normalized.Args, normalized.LaunchEnv)
		if errPrepare != nil {
			return nil, fmt.Errorf("prepare native Valheim runtime: %w", errPrepare)
		}
		errLoader := prepareInstalledValheimLoader(root, runtime.GOOS, trusted)
		if errLoader != nil {
			return nil, errLoader
		}
		prepared.WorkingDirectory, prepared.BaseCommand, prepared.TrustedRuntimeEnv = root, executable, trusted
		prepared.StopTimeout = 120 * time.Second
		prepared.ServiceID = "valheim"

	}
	configuredInputs := 0
	if normalized.InputTelnet != nil {
		configuredInputs++
		prepared.InputMethod = supervisor.InputMethod{
			Type: supervisor.InputTypeTelnet,
			TelnetCredentials: &supervisor.TelnetCredentials{
				Port:     normalized.InputTelnet.Port,
				Password: normalized.InputTelnet.Password,
			},
		}
	}
	if normalized.InputRCON != nil {
		configuredInputs++
		protocol, errProtocol := supervisorRCONProtocol(normalized.InputRCON.Protocol)
		if errProtocol != nil {
			return nil, errProtocol
		}
		prepared.InputMethod = supervisor.InputMethod{
			Type: supervisor.InputTypeRCON,
			RCONCredentials: &supervisor.RCONCredentials{
				Host:     normalized.InputRCON.Host,
				Port:     normalized.InputRCON.Port,
				Password: normalized.InputRCON.Password,
				Protocol: protocol,
			},
		}
	}
	if normalized.InputREST != nil {
		configuredInputs++
		kind, errKind := supervisorRESTInputKind(normalized.InputREST.Kind)
		if errKind != nil {
			return nil, errKind
		}
		prepared.InputMethod = supervisor.InputMethod{
			Type: supervisor.InputTypeREST,
			RESTCredentials: &supervisor.RESTCredentials{
				Host:              normalized.InputREST.Host,
				Port:              normalized.InputREST.Port,
				Kind:              kind,
				Password:          normalized.InputREST.Password,
				PreviousPasswords: slices.Clone(normalized.InputREST.PreviousPasswords),
			},
		}
	}
	if configuredInputs > 1 {
		return nil, errors.New("node: only one console input transport may be configured")
	}

	if normalized.InternalCommand {
		prepared.InternalCommand = true
		// Prefer the caller-supplied model; fall back to a DB lookup by
		// InternalGameServerID.
		suppliedGameServer, supplied := normalized.InternalGameServer.(*models.GameServer)
		if supplied && suppliedGameServer != nil {
			prepared.InternalGameServer = suppliedGameServer
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

func (n *Node) supervisorReadiness(readiness ProcessReadiness) (*supervisor.Readiness, error) {
	out := &supervisor.Readiness{Timeout: readiness.Timeout}
	if readiness.LogPattern != "" {
		pattern, errCompile := regexp.Compile(readiness.LogPattern)
		if errCompile != nil {
			return nil, fmt.Errorf("node: compile readiness log pattern: %w", errCompile)
		}
		out.LogPattern = pattern
	}
	if readiness.Query != nil {
		query := *readiness.Query
		out.Probe = func(ctx context.Context) bool {
			result, errQuery := n.QueryGameServer(ctx, query)
			return errQuery == nil && result.Responded()
		}
	}
	return out, nil
}

func supervisorRCONProtocol(protocol RCONProtocol) (supervisor.RCONProtocol, error) {
	switch protocol {
	case RCONProtocolSource:
		return supervisor.RCONProtocolSource, nil
	case RCONProtocolMinecraft:
		return supervisor.RCONProtocolMinecraft, nil
	case RCONProtocolRustWeb:
		return supervisor.RCONProtocolRustWeb, nil
	default:
		return supervisor.RCONProtocolUnknown, errors.New("node: unsupported RCON protocol")
	}
}

func supervisorRESTInputKind(kind RESTInputKind) (supervisor.RESTInputKind, error) {
	switch kind {
	case RESTInputKindSatisfactory:
		return supervisor.RESTInputKindSatisfactory, nil
	case RESTInputKindPalworld:
		return supervisor.RESTInputKindPalworld, nil
	default:
		return supervisor.RESTInputKindUnknown, errors.New("node: unsupported REST input kind")
	}
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
		if errors.Is(errGet, supervisor.ErrCommandDoesNotExist) {
			return ErrProcessNotFound
		}
		return fmt.Errorf("node: stop process: %w", errGet)
	}
	unlockLifecycle, errLifecycle := n.lockServerLifecycle(cmd.WorkingDir())
	if errLifecycle != nil {
		return errLifecycle
	}
	defer unlockLifecycle()
	if cmd.ServiceID() == "valheim" {
		stopInputCommand = ""
	}
	cmd.Stop(stopInputCommand)
	return nil
}

// SendConsoleInput sends one command through the running process's configured
// console transport.
func (n *Node) SendConsoleInput(processID, input string) error {
	return n.SendConsoleInputContext(context.Background(), processID, input)
}

// SendConsoleInputContext writes a single line through the running process's
// configured input transport and honors cancellation for network transports.
func (n *Node) SendConsoleInputContext(ctx context.Context, processID, input string) error {
	if n.supervisor == nil {
		return errors.New("node: supervisor not configured")
	}

	cmd, errGet := n.supervisor.GetCommandByID(processID)
	if errGet != nil {
		if errors.Is(errGet, supervisor.ErrCommandDoesNotExist) {
			return ErrProcessNotFound
		}
		return fmt.Errorf("node: send console input: %w", errGet)
	}
	if cmd.ServiceID() == "valheim" {
		return ErrConsoleInputUnavailable
	}
	_, errSend := cmd.ExecuteInput(ctx, input)
	if errSend != nil {
		return translateSupervisorConsoleInputError(errSend)
	}
	return nil
}

func translateSupervisorConsoleInputError(errSend error) error {
	rejectedError, isRejected := errors.AsType[*supervisor.ConsoleInputRejectedError](errSend)
	if isRejected {
		return NewConsoleInputRejectedError(rejectedError.Detail)
	}
	if errors.Is(errSend, supervisor.ErrConsoleInputUnavailable) {
		return fmt.Errorf("%w: %w", ErrConsoleInputUnavailable, errSend)
	}
	return fmt.Errorf("node: send console input: %w", errSend)
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
	buffer, sequence := cmd.GetOutputSnapshot()
	return ConsoleChunk{
		ProcessID: processID,
		Data:      buffer,
		Sequence:  sequence,
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
	cpuPercent, cpuValid, metricsValid, memoryRSS, memoryVMS, memoryPercent, cpuCores, numThreads, diskUsageBytes, diskTotalBytes, diskFreeBytes, diskPercent, diskMeasuredAt, diskValid, ioValid, ioReadRate, ioWriteRate, connectionCount, connectionCountValid := cmd.Metrics()
	lifecycle := cmd.Lifecycle()
	return &ProcessSnapshot{
		ID:                   cmd.ID,
		ExecutionID:          lifecycle.ExecutionID,
		Name:                 cmd.GameServerName(),
		Status:               cmd.Status().String(),
		PreviousStatus:       lifecycle.PreviousStatus.String(),
		TransitionSequence:   lifecycle.TransitionSequence,
		IntentionalStop:      lifecycle.IntentionalStop,
		ExitCode:             lifecycle.ExitCode,
		ExitCodeKnown:        lifecycle.ExitCodeKnown,
		UnixStartedAt:        cmd.UnixStartedAt(),
		CPUPercent:           cpuPercent,
		CPUValid:             cpuValid,
		MetricsValid:         metricsValid,
		CPUCores:             cpuCores,
		MemoryRSS:            memoryRSS,
		MemoryVMS:            memoryVMS,
		MemoryPercent:        memoryPercent,
		NumThreads:           numThreads,
		DiskUsageBytes:       diskUsageBytes,
		DiskTotalBytes:       diskTotalBytes,
		DiskFreeBytes:        diskFreeBytes,
		DiskPercent:          diskPercent,
		DiskMeasuredAt:       diskMeasuredAt,
		DiskValid:            diskValid,
		IOValid:              ioValid,
		IOReadRate:           ioReadRate,
		IOWriteRate:          ioWriteRate,
		ConnectionCount:      connectionCount,
		ConnectionCountValid: connectionCountValid,
		WorkingDir:           cmd.WorkingDir(),
	}, true, nil
}

// StreamConsoleOutput streams live console output chunks for one process.
func (n *Node) StreamConsoleOutput(ctx context.Context, processID string, replayBuffer bool) (<-chan ConsoleChunk, error) {
	if n.supervisor == nil {
		return nil, errors.New("node: supervisor not configured")
	}

	command := n.supervisor.GetCommandByIDOrCreateShell(processID)
	listenerID := fmt.Sprintf("node-console-%s-%d", processID, time.Now().UnixNano())
	listener := make(chan *xylona.Message, 256)
	var replay *xylona.Message
	if replayBuffer {
		replay = command.AddOutputListenerWithReplay(listenerID, listener)
	} else {
		command.AddOutputListener(listenerID, listener)
	}

	out := make(chan ConsoleChunk, 64)
	go func() {
		defer close(out)
		defer command.RemoveOutputListener(listenerID)

		if replay != nil {
			replayChunk, ok := consoleChunkFromMessage(replay)
			if ok {
				select {
				case <-ctx.Done():
					return
				case out <- replayChunk:
				}
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-listener:
				if !ok {
					return
				}
				if msg == nil || msg.GetType() != xylona.Message_GameServerConsole {
					continue
				}

				chunk, valid := consoleChunkFromMessage(msg)
				if !valid {
					continue
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

func consoleChunkFromMessage(msg *xylona.Message) (ConsoleChunk, bool) {
	if msg == nil || msg.GetType() != xylona.Message_GameServerConsole {
		return ConsoleChunk{}, false
	}
	consoleOutput := msg.GetGameServerConsoleOutput()
	if consoleOutput == nil {
		return ConsoleChunk{}, false
	}
	return ConsoleChunk{
		ProcessID:   consoleOutput.GetGameServerId(),
		Data:        consoleOutput.GetOutput(),
		Sequence:    consoleOutput.GetSequence(),
		ResetBuffer: consoleOutput.GetResetBuffer(),
	}, true
}
