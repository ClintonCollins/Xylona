package supervisor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/ziutek/telnet"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type inputType int

const maxOutputBufferBytes = 1024 << 10

// Input types supported for command input wiring.
const (
	InputTypeStdIn inputType = iota
	InputTypeTelnet
)

var (
	// MessageStartingServer is emitted when Xylona begins launching a server.
	MessageStartingServer = formatXylonaMessage("Starting server...")
	// MessageStoppingServer is emitted when Xylona begins stopping a server.
	MessageStoppingServer = formatXylonaMessage("Stopping server...")
	// MessageStoppedServer is emitted after a server has stopped.
	MessageStoppedServer = formatXylonaMessage("Server stopped.")
)

// TelnetCredentials contains telnet connection settings for server input.
type TelnetCredentials struct {
	Port     int
	Password string
}

// InputMethod describes how input is sent to a managed command.
type InputMethod struct {
	Type              inputType
	TelnetCredentials *TelnetCredentials
}

// PreparedCommand contains all inputs needed to launch or reuse a command.
type PreparedCommand struct {
	ID                 string
	ExecutionID        string
	GameServerName     string
	InternalCommand    bool
	InternalGameServer *models.GameServer
	BaseCommand        string
	Args               []string
	WorkingDirectory   string
	User               string
	NodeID             string
	ServiceID          string      // ServiceID is usually the ID of the game this command is associated with.
	InputMethod        InputMethod // InputMethod is used to determine how to send input to the command.
	GameID             *string
	GameServerID       *string
	CallbackFunction   func(*Command)
	Status             xylona.Status
	// StopTimeout overrides the default 15-second graceful-stop timeout.
	// When zero the default is used.
	StopTimeout time.Duration
	LaunchEnv   map[string]string
}

func (imt inputType) String() string {
	switch imt {
	case InputTypeStdIn:
		return "StdIn"
	case InputTypeTelnet:
		return "Telnet"
	default:
		return "Unknown"
	}
}

const (
	defaultStopTimeout    = 15 * time.Second
	outputDrainTimeout    = 5 * time.Second
	pseudoTerminalNewline = "\r"
)

func (c *Command) effectiveStopTimeout() time.Duration {
	if c.stopTimeout > 0 {
		return c.stopTimeout
	}
	return defaultStopTimeout
}

// StartCommand prepares, launches, and tracks a command execution.
func (inst *Instance) StartCommand(preparedCommand PreparedCommand) (*Command, error) {
	cmd, err := inst.prepareCommandProcess(preparedCommand)
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

// Stop requests that the command shut down gracefully, then forces cancelation on timeout.
func (c *Command) Stop(stopInputCommand string) {
	c.intentionalStop.Store(true)
	c.RLock()
	currentCMD := c.currentCMD
	currentPTYCMD := c.currentPTYCMD
	currentPTY := c.currentPTY
	c.RUnlock()
	if currentCMD == nil && currentPTYCMD == nil {
		return
	}
	c.sendJobNotification(MessageStoppingServer)
	if stopInputCommand != "" {
		log.Debug().Str("Game Server ID", c.ID).Str("Stop Input Command", stopInputCommand).Msg("Sending stop command")
		errSend := c.SendInput(stopInputCommand)
		if errSend != nil {
			log.Error().Err(errSend).Msg("Error sending stop command")
		}
	} else {
		var errInterrupt error
		if currentPTY != nil {
			_, errInterrupt = currentPTY.Write([]byte{3})
		} else {
			errInterrupt = interruptProcessTree(currentCMD.Process)
		}
		if errInterrupt != nil {
			log.Error().Err(errInterrupt).Msg("Error sending graceful interrupt to process tree")
		}
	}
	select {
	case <-c.processCtx.Done():
		log.Debug().Str("Game Server ID", c.ID).Msg("Job process context done.")
		return
	case <-c.instanceCtx.Done():
		log.Debug().Str("Game Server ID", c.ID).Msg("Xylona shutdown signal received. Closing job.")
		return
	case <-time.After(c.effectiveStopTimeout()):
		c.RLock()
		log.Warn().Str("ID", c.ID).Str("User", c.User).Msg("Timeout waiting for command to stop")
		c.RUnlock()
		c.processCtxCancel()
	}
}

// ListCommands returns the currently tracked commands.
func (inst *Instance) ListCommands() []*Command {
	inst.RLock()
	defer inst.RUnlock()
	commands := make([]*Command, 0, len(inst.runningCommands))
	for _, p := range inst.runningCommands {
		commands = append(commands, p)
	}
	return commands
}

func (inst *Instance) startAndWaitForJob(
	command *Command,
	commandEndFunc func(command *Command),
	startupResult chan<- error,
	outputDone <-chan struct{},
) {
	processCtxCancel := command.processCtxCancel
	defer func(command *Command) {
		if command.inputMethod.Type == InputTypeTelnet {
			log.Debug().Str("Game Server ID", command.ID).Msg("Closing telnet connection")
			command.Lock()
			telnetConn := command.telnetConn
			command.telnetConn = nil
			command.Unlock()
			if telnetConn != nil {
				errCloseTelnetConn := telnetConn.Close()
				if errCloseTelnetConn != nil {
					log.Error().Err(errCloseTelnetConn).Msg("Error closing telnet connection")
				}
			}
		}
	}(command)
	defer processCtxCancel()
	log.Debug().Str("Game Server ID", command.ID).Msg("Starting job")
	// If it's an internal command, we need to run the internal command.
	if command.InternalCommand && (command.status == xylona.Status_INSTALLING || command.status == xylona.Status_UPDATING) {
		startupResult <- nil
		exitCode := 0
		command.sendJobStatusNotification(xylona.Status_OFFLINE, command.status)
		defer func() {
			stdoutPipeWriter, stdoutIsPipe := command.internalCommandStdOut.(*io.PipeWriter)
			if stdoutIsPipe {
				errCloseStdout := stdoutPipeWriter.Close()
				if errCloseStdout != nil {
					log.Error().Err(errCloseStdout).Str("Game Server ID", command.ID).
						Msg("Error closing internal command stdout")
				}
			}
			stderrPipeWriter, stderrIsPipe := command.internalCommandStdErr.(*io.PipeWriter)
			if stderrIsPipe {
				errCloseStderr := stderrPipeWriter.Close()
				if errCloseStderr != nil {
					log.Error().Err(errCloseStderr).Str("Game Server ID", command.ID).
						Msg("Error closing internal command stderr")
				}
			}
			waitForJobOutput(command.ID, outputDone)
			oldStatus := command.Status()
			command.Lock()
			command.currentCMD = nil
			command.currentPTYCMD = nil
			command.currentPTY = nil
			command.status = xylona.Status_OFFLINE
			for name := range command.launchEnv {
				command.launchEnv[name] = ""
				delete(command.launchEnv, name)
			}
			command.Unlock()
			command.sendJobStatusNotificationWithExit(oldStatus, exitCode)
			if commandEndFunc != nil {
				commandEndFunc(command)
			}
		}()

		reportInternalFailure := func(message string, err error) {
			exitCode = 1
			log.Error().Err(err).Str("Game Server ID", command.ID).Msg(message)
			_, errWrite := fmt.Fprintf(command.internalCommandStdErr, "%s: %v\n", message, err)
			if errWrite != nil {
				log.Error().Err(errWrite).Str("Game Server ID", command.ID).
					Msg("Error writing internal command failure to stderr")
			}
		}

		if command.internalGameServer == nil {
			reportInternalFailure("Internal game server is unavailable", errors.New("internal game server is nil"))
			return
		}
		if command.gameID == nil {
			reportInternalFailure("Internal game ID is unavailable", errors.New("game ID is nil"))
			return
		}
		internalGame, exists := gameintegrations.GetGame(*command.gameID)
		if !exists {
			reportInternalFailure("Internal game integration is unavailable", fmt.Errorf("game %q is not registered", *command.gameID))
			return
		}
		switch command.status {
		case xylona.Status_INSTALLING:
			err := internalGame.Install(command.internalGameServer, command.internalCommandStdOut, command.internalCommandStdErr)
			if err != nil {
				reportInternalFailure("Error installing internal game", err)
				return
			}
			return
		case xylona.Status_UPDATING:
			environmentUpdater, supportsEnvironment := internalGame.(gameintegrations.EnvironmentUpdater)
			var err error
			if supportsEnvironment {
				err = environmentUpdater.UpdateWithEnvironment(
					command.internalGameServer,
					command.internalCommandStdOut,
					command.internalCommandStdErr,
					command.launchEnv,
				)
			} else {
				err = internalGame.Update(command.internalGameServer, command.internalCommandStdOut, command.internalCommandStdErr)
			}
			if err != nil {
				reportInternalFailure("Error updating internal game", err)
				return
			}
			return
		}
		reportInternalFailure("Unable to find internal command to run", fmt.Errorf("unsupported status %s", command.status.String()))
		return
	}

	// If it's not an internal command, we need to run the command.
	command.RLock()
	currentCMD := command.currentCMD
	currentPTYCMD := command.currentPTYCMD
	currentPTY := command.currentPTY
	command.RUnlock()
	if currentCMD == nil && currentPTYCMD == nil {
		errUnavailable := errors.New("start command: prepared process is unavailable")
		startupResult <- errUnavailable
		oldStatus := command.Status()
		command.Lock()
		command.status = xylona.Status_OFFLINE
		command.Unlock()
		waitForJobOutput(command.ID, outputDone)
		command.sendJobStatusNotificationWithExit(oldStatus, 1)
		return
	}
	command.Lock()
	var errStartProcess error
	if currentPTYCMD != nil {
		errStartProcess = currentPTYCMD.Start()
	} else {
		errStartProcess = currentCMD.Start()
	}
	command.Unlock()
	if errStartProcess != nil {
		errStart := fmt.Errorf("start command: %w", errStartProcess)
		startupResult <- errStart
		log.Error().Err(errStartProcess).Msg("Unable to start command.")
		command.sendJobNotification(errStartProcess.Error())
		oldStatus := command.Status()
		command.Lock()
		command.currentCMD = nil
		command.currentPTYCMD = nil
		command.currentPTY = nil
		command.status = xylona.Status_OFFLINE
		command.Unlock()
		if currentPTY != nil {
			errClosePTY := currentPTY.Close()
			if errClosePTY != nil {
				log.Error().Err(errClosePTY).Str("Game Server ID", command.ID).
					Msg("Error closing pseudo-terminal after failed start")
			}
		} else {
			errClosePipes := closePreparedCommandPipes(command)
			if errClosePipes != nil {
				log.Debug().Err(errClosePipes).Str("Game Server ID", command.ID).
					Msg("Error closing command pipes after failed start")
			}
		}
		waitForJobOutput(command.ID, outputDone)
		command.sendJobStatusNotificationWithExit(oldStatus, 1)
		if commandEndFunc != nil {
			commandEndFunc(command)
		}
		return
	}
	startupResult <- nil
	command.Lock()
	if currentCMD != nil && command.currentCMD == currentCMD {
		command.currentCMD.Env = nil
	}
	if currentPTYCMD != nil && command.currentPTYCMD == currentPTYCMD {
		command.currentPTYCMD.Env = nil
	}
	command.Unlock()

	var executable string
	var argumentCount int
	if currentPTYCMD != nil {
		executable = currentPTYCMD.Path
		argumentCount = len(currentPTYCMD.Args)
	} else {
		executable = currentCMD.Path
		argumentCount = len(currentCMD.Args)
	}
	log.Debug().Str("Command ID", command.ID).Str("executable", executable).
		Int("argument_count", argumentCount).Msg("Command started")
	command.sendJobStatusNotification(xylona.Status_OFFLINE, command.status)

	// Run after startup function if it exists.
	if command.runAfterStartup != nil {
		go command.runAfterStartup(command)
	}

	var errWait error
	var exitCode int
	if currentPTYCMD != nil {
		stopCancellationWatch := watchPseudoTerminalCancellation(command.processCtx, currentPTYCMD)
		errWait = currentPTYCMD.Wait()
		stopCancellationWatch()
		exitCode = extractProcessExitCode(currentPTYCMD.ProcessState, errWait)
	} else {
		errWait = currentCMD.Wait()
		exitCode = extractExitCode(currentCMD, errWait)
	}
	if currentPTY != nil {
		if drainPseudoTerminalBeforeClose() {
			waitForJobOutput(command.ID, outputDone)
		}
		errClosePTY := currentPTY.Close()
		if errClosePTY != nil {
			log.Debug().Err(errClosePTY).Str("Game Server ID", command.ID).
				Msg("Error closing completed pseudo-terminal")
		}
	}
	waitForJobOutput(command.ID, outputDone)
	if errWait != nil {
		checkErrorAccessDenied(errWait, command)
		log.Debug().Err(errWait).Msg("Error waiting for command.")
	}

	// Publish crash event if the process exited with a non-zero exit code.
	if exitCode != 0 {
		log.Warn().Str("Game Server ID", command.ID).Int("exit_code", exitCode).Msg("Game server process crashed")
		eb := eventbus.Get()
		eb.Publish(eventbus.TopicGameServerCrashed, eventbus.ServerCrashedEvent{
			ServerID:     command.ID,
			ServerNodeID: command.nodeID,
			ExitCode:     exitCode,
			Timestamp:    time.Now(),
		})
	}

	log.Debug().Str("Game Server ID", command.ID).Msg("Game server stopped.")
	oldStatus := command.Status()
	command.Lock()
	command.currentCMD = nil
	command.currentPTYCMD = nil
	command.currentPTY = nil
	command.status = xylona.Status_OFFLINE
	command.Unlock()
	command.sendJobStatusNotificationWithExit(oldStatus, exitCode)
	if commandEndFunc != nil {
		commandEndFunc(command)
	}
}

func (inst *Instance) prepareCommandProcess(preparedCommand PreparedCommand) (*Command, error) {
	inst.Lock()
	defer inst.Unlock()

	persistentCommand, exists := inst.runningCommands[preparedCommand.ID]
	if exists {
		persistentCommand.RLock()
		commandAlreadyRunning := persistentCommand.currentCMD != nil ||
			persistentCommand.currentPTYCMD != nil ||
			persistentCommand.status != xylona.Status_OFFLINE
		persistentCommand.RUnlock()
		if commandAlreadyRunning {
			return nil, ErrCommandAlreadyRunning
		}
	}

	newCommand := inst.initNewCommand(preparedCommand, persistentCommand)

	if !preparedCommand.InternalCommand {
		newCommand.InternalCommand = false
		newCommand.internalGameServer = nil
		newCommand.gameID = nil
		newCommand.internalCommandStdOut = nil
		newCommand.internalCommandStdErr = nil
		var err error
		if requiresPseudoTerminal(preparedCommand.ServiceID) {
			newCommand.currentPTYCMD, newCommand.currentPTY, err = inst.setupPseudoTerminal(newCommand, preparedCommand)
			newCommand.currentCMD = nil
		} else {
			newCommand.currentCMD, err = inst.setupCmd(newCommand, preparedCommand)
			newCommand.currentPTYCMD = nil
			newCommand.currentPTY = nil
		}
		if err != nil {
			newCommand.Lock()
			newCommand.currentCMD = nil
			newCommand.currentPTYCMD = nil
			newCommand.currentPTY = nil
			newCommand.status = xylona.Status_OFFLINE
			newCommand.Unlock()
			newCommand.processCtxCancel()
			return nil, err
		}

		log.Debug().Str("Command ID", preparedCommand.ID).Msg("Starting command")
	} else {
		// Internal command
		newCommand.InternalCommand = preparedCommand.InternalCommand
		newCommand.internalGameServer = preparedCommand.InternalGameServer
		newCommand.gameID = preparedCommand.GameID
		stdOutPipeReader, stdOutPipeWriter := io.Pipe()
		stdErrPipeReader, stdErrPipeWriter := io.Pipe()
		newCommand.internalCommandStdOut = stdOutPipeWriter
		newCommand.internalCommandStdErr = stdErrPipeWriter
		newCommand.stdout = stdOutPipeReader
		newCommand.stderr = stdErrPipeReader
		newCommand.currentCMD = nil
		newCommand.currentPTYCMD = nil
		newCommand.currentPTY = nil
	}

	if persistentCommand == nil {
		inst.runningCommands[preparedCommand.ID] = newCommand
	}

	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		newCommand.readJobOut()
	}()
	if newCommand.status == xylona.Status_ONLINE {
		newCommand.sendJobNotification(MessageStartingServer)
	}
	startupResult := make(chan error, 1)
	go inst.startAndWaitForJob(newCommand, preparedCommand.CallbackFunction, startupResult, outputDone)
	errStart := <-startupResult
	if errStart != nil {
		return nil, errStart
	}
	return newCommand, nil
}

// connectTelnetAndSetAsStdinWriter is a function that sets up a telnet connection and assigns it as the standard input writer for a command.
// It takes a Command pointer as an argument.
// The function performs the following steps:
// Dials a telnet connection to the localhost at the port specified in the command's telnet credentials.
// If there is an error in dialing the telnet connection, it logs the error and assigns the standard input writer of the command to io.Discard, effectively ignoring any input.
// If there is an error in writing the password (If one is provided), it logs the error and assigns the standard input writer of the command to io.Discard.
// Finally, it assigns the dialed telnet connection to the command's telnetConn field and uses it as the standard input writer for the command.
func connectTelnetAndSetAsStdinWriter(command *Command) {
	log.Debug().Str("Command ID", command.ID).Msg("Setting up telnet")
	inputMethod := command.inputMethod
	if inputMethod.Type != InputTypeTelnet {
		log.Debug().Str("Command ID", command.ID).Msg("Skipping telnet setup for non-telnet input method")
		return
	}

	errValidateTelnet := validateTelnetInputMethod(inputMethod)
	if errValidateTelnet != nil {
		log.Error().Err(errValidateTelnet).Str("Command ID", command.ID).Msg("Invalid telnet input method")
		command.Lock()
		command.stdInWriter = io.Discard
		command.Unlock()
		return
	}
	telnetCredentials := inputMethod.TelnetCredentials

	telnetConnect := func() (*telnet.Conn, error) {
		log.Debug().Str("Command ID", command.ID).Msg("Dialing telnet")
		telnetConn, errDial := telnet.DialTimeout("tcp", net.JoinHostPort("localhost", strconv.Itoa(telnetCredentials.Port)), time.Second*5)
		if errDial != nil {
			log.Error().Err(errDial).Msg("Error dialing telnet")
			return nil, fmt.Errorf("dial telnet: %w", errDial)
		}
		log.Debug().Msg("Telnet connection successful")
		log.Debug().Msg("Writing password to telnet")
		if telnetCredentials.Password != "" {
			b, errAuth := telnetConn.Write([]byte(telnetCredentials.Password))
			if errAuth != nil {
				log.Error().Err(errAuth).Msg("Error authenticating telnet")
				errClose := telnetConn.Close()
				if errClose != nil {
					log.Error().Err(errClose).Msg("Error closing unauthenticated telnet connection")
				}
				return nil, fmt.Errorf("authenticate telnet: %w", errAuth)
			}
			log.Debug().Int("bytes written", b).Msg("Wrote password to telnet")
		}
		return telnetConn, nil
	}

	telnetConnection, errConnect := telnetConnect()
	if errConnect == nil {
		attachTelnetConnection(command, telnetConnection)
		return
	}

	retries := 5
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	for range retries {
		select {
		case <-command.instanceCtx.Done():
			return
		case <-command.processCtx.Done():
			return
		case <-ticker.C:
			log.Debug().Str("Command ID", command.ID).Msg("Retrying telnet connection")
			telnetConnection, errConnect = telnetConnect()
			if errConnect == nil {
				attachTelnetConnection(command, telnetConnection)
				return
			}
		}
	}
	command.Lock()
	command.stdInWriter = io.Discard
	command.Unlock()
}

func attachTelnetConnection(command *Command, telnetConnection *telnet.Conn) {
	command.Lock()
	if command.processCtx.Err() != nil {
		command.Unlock()
		errClose := telnetConnection.Close()
		if errClose != nil {
			log.Debug().Err(errClose).Str("Command ID", command.ID).
				Msg("Error closing late telnet connection")
		}
		return
	}
	command.telnetConn = telnetConnection
	command.stdInWriter = telnetConnection
	processDone := command.processCtx.Done()
	command.Unlock()
	go command.readTelnetOutput(telnetConnection, processDone)
}

// GetCommandByID returns a tracked command by ID.
func (inst *Instance) GetCommandByID(commandID string) (*Command, error) {
	inst.RLock()
	defer inst.RUnlock()
	proc, exists := inst.runningCommands[commandID]
	if !exists || proc == nil {
		return nil, ErrCommandDoesNotExist
	}
	return proc, nil
}

// GetCommandByIDOrCreateShell returns a tracked command or creates an offline shell placeholder.
func (inst *Instance) GetCommandByIDOrCreateShell(commandID string) *Command {
	inst.Lock()
	defer inst.Unlock()

	persistentCommand, exists := inst.runningCommands[commandID]
	if !exists {
		inst.runningCommands[commandID] = &Command{
			ID:                  commandID,
			instanceCtx:         inst.ctx,
			stdInWriter:         &bytes.Buffer{},
			combinedOutput:      &bytes.Buffer{},
			outputListeners:     make(map[string]chan *xylona.Message),
			outputListenersLock: &sync.RWMutex{},
			statusListeners:     make(map[string]chan *xylona.GameServerStatusUpdate),
			statusListenersLock: &sync.RWMutex{},
			RWMutex:             &sync.RWMutex{},
			status:              xylona.Status_OFFLINE,
			toggleOutputType:    make(chan struct{}),
		}
		return inst.runningCommands[commandID]
	}
	return persistentCommand
}

// SendConsoleOutput injects a Xylona status line into the server console stream,
// creating a synthetic shell command buffer when the server is offline.
func (inst *Instance) SendConsoleOutput(commandID string, message string) {
	inst.GetCommandByIDOrCreateShell(commandID).SendOutput(formatXylonaMessage(message))
}

// SendInput sends input to the command's StdIn.
func (c *Command) SendInput(input string) error {
	c.RLock()
	currentCMD := c.currentCMD
	currentPTYCMD := c.currentPTYCMD
	inputWriter := c.stdInWriter
	inputMethod := c.inputMethod.Type
	if currentCMD == nil && currentPTYCMD == nil {
		c.RUnlock()
		return errors.New("command is not running")
	}
	c.RUnlock()
	log.Debug().Str("Command ID", c.ID).Int("input_bytes", len(input)).Msg("Sending console input")
	inputTerminator := "\n"
	if currentPTYCMD != nil {
		inputTerminator = pseudoTerminalNewline
	}
	b, wErr := fmt.Fprintf(inputWriter, "%s%s", input, inputTerminator)
	var errConsoleInput error
	if currentPTYCMD == nil && inputMethod == InputTypeStdIn {
		errConsoleInput = mirrorProcessConsoleInput(currentCMD, input)
	}
	if wErr != nil && errConsoleInput != nil {
		return errors.Join(
			fmt.Errorf("write command input: %w", wErr),
			fmt.Errorf("mirror command input to console: %w", errConsoleInput),
		)
	}
	if wErr != nil {
		log.Debug().Err(wErr).Str("Command ID", c.ID).
			Msg("Standard input pipe rejected command; console input succeeded")
	}
	if errConsoleInput != nil {
		log.Debug().Err(errConsoleInput).Str("Command ID", c.ID).
			Msg("Console input unavailable; standard input pipe accepted command")
	}
	log.Debug().Str("Command ID", c.ID).Int("bytes written", b).Msg("Wrote input")
	return nil
}

func closePreparedCommandPipes(command *Command) error {
	command.RLock()
	readersAndWriters := []any{command.stdout, command.stderr, command.stdInWriter}
	command.RUnlock()

	errorsToJoin := make([]error, 0, len(readersAndWriters))
	for _, stream := range readersAndWriters {
		closer, canClose := stream.(io.Closer)
		if !canClose || closer == nil {
			continue
		}
		errClose := closer.Close()
		if errClose != nil && !errors.Is(errClose, os.ErrClosed) {
			errorsToJoin = append(errorsToJoin, errClose)
		}
	}
	return errors.Join(errorsToJoin...)
}

func waitForJobOutput(commandID string, outputDone <-chan struct{}) {
	if outputDone == nil {
		return
	}
	timer := time.NewTimer(outputDrainTimeout)
	defer timer.Stop()
	select {
	case <-outputDone:
	case <-timer.C:
		log.Warn().Str("Game Server ID", commandID).Dur("timeout", outputDrainTimeout).
			Msg("Timed out draining final game server console output")
	}
}

// extractExitCode returns the exit code from a completed command. It checks
// the ProcessState first (which is set even when Wait returns a context
// cancellation error on Windows), then falls back to unwrapping exec.ExitError.
// Returns 0 if err is nil or the process exited with code 0, the actual exit
// code if it can be determined, or -1 if the error is not an exit error.
func extractExitCode(cmd *exec.Cmd, err error) int {
	var processState *os.ProcessState
	if cmd != nil {
		processState = cmd.ProcessState
	}
	return extractProcessExitCode(processState, err)
}

func extractProcessExitCode(processState *os.ProcessState, err error) int {
	if err == nil {
		return 0
	}
	if processState != nil {
		return processState.ExitCode()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	// If we can't determine the exit code but there was an error, assume non-zero.
	return -1
}
