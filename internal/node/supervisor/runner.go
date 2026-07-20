package supervisor

import (
	"bytes"
	"context"
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
	InputTypeRCON
	InputTypeREST
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

// RCONProtocol identifies the response framing spoken by an RCON server.
type RCONProtocol int

const (
	// RCONProtocolUnknown is invalid.
	RCONProtocolUnknown RCONProtocol = iota
	// RCONProtocolSource uses the Source-compatible packet protocol.
	RCONProtocolSource
	// RCONProtocolMinecraft uses the Minecraft-compatible packet protocol.
	RCONProtocolMinecraft
	// RCONProtocolRustWeb uses Rust's WebSocket RCON protocol.
	RCONProtocolRustWeb
)

// RCONCredentials contains authenticated RCON connection settings.
type RCONCredentials struct {
	Host     string
	Port     int
	Password string
	Protocol RCONProtocol
}

// RESTInputKind identifies an explicitly supported REST command API.
type RESTInputKind int

const (
	// RESTInputKindUnknown is invalid.
	RESTInputKindUnknown RESTInputKind = iota
	// RESTInputKindSatisfactory uses the Satisfactory dedicated-server API.
	RESTInputKindSatisfactory
)

// RESTCredentials contains game-specific REST command settings.
type RESTCredentials struct {
	Host              string
	Port              int
	Kind              RESTInputKind
	Password          string
	PreviousPasswords []string
}

// InputMethod describes how input is sent to a managed command.
type InputMethod struct {
	Type              inputType
	TelnetCredentials *TelnetCredentials
	RCONCredentials   *RCONCredentials
	RESTCredentials   *RESTCredentials
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
	// SuppressStatusEvents keeps companion tasks out of game-server status
	// listeners, broadcasts, and alert evaluation.
	SuppressStatusEvents bool
}

func (imt inputType) String() string {
	switch imt {
	case InputTypeStdIn:
		return "StdIn"
	case InputTypeTelnet:
		return "Telnet"
	case InputTypeRCON:
		return "RCON"
	case InputTypeREST:
		return "REST"
	default:
		return "Unknown"
	}
}

const (
	defaultStopTimeout    = 15 * time.Second
	outputDrainTimeout    = 5 * time.Second
	pseudoTerminalNewline = "\r"
	telnetInitialBackoff  = time.Second
	telnetMaximumBackoff  = 10 * time.Second
	telnetAuthTimeout     = 5 * time.Second
	telnetLoginPrompt     = "Please enter password:"
	telnetLogonSuccessful = "Logon successful."
	telnetLogonFailed     = "Logon failed"
)

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
	finalizationDone := c.finalizationDone
	instanceDone := c.instanceCtx.Done()
	processCtxCancel := c.processCtxCancel
	processGeneration := c.processGeneration
	stopTimeout := c.stopTimeout
	commandID := c.ID
	user := c.User
	c.RUnlock()
	if currentCMD == nil && currentPTYCMD == nil {
		return
	}
	if stopTimeout <= 0 {
		stopTimeout = defaultStopTimeout
	}
	c.sendJobNotification(MessageStoppingServer)
	if stopInputCommand != "" {
		log.Debug().Str("Game Server ID", c.ID).Str("Stop Input Command", stopInputCommand).Msg("Sending stop command")
		errSend := c.sendInputForExecution(stopInputCommand, &processGeneration)
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
	case <-finalizationDone:
		log.Debug().Str("Game Server ID", c.ID).Msg("Job execution finalized.")
		return
	case <-instanceDone:
		log.Debug().Str("Game Server ID", c.ID).Msg("Xylona shutdown signal received. Closing job.")
		return
	case <-time.After(stopTimeout):
		log.Warn().Str("ID", commandID).Str("User", user).Msg("Timeout waiting for command to stop")
		processCtxCancel()
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
	command.RLock()
	processCtxCancel := command.processCtxCancel
	processGeneration := command.processGeneration
	inputMethodType := command.inputMethod.Type
	command.RUnlock()
	defer closeTelnetConnectionForGeneration(command, processGeneration, inputMethodType)
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
			command.finalizeExecution(processGeneration, exitCode, true, true, commandEndFunc)
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
		waitForJobOutput(command.ID, outputDone)
		command.finalizeExecution(processGeneration, 1, true, false, commandEndFunc)
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
	drainBeforeClose := false
	if errStartProcess == nil && currentPTY != nil {
		var errPrepareDrain error
		drainBeforeClose, errPrepareDrain = preparePseudoTerminalDrain(currentPTY)
		if errPrepareDrain != nil {
			log.Debug().Err(errPrepareDrain).Str("Game Server ID", command.ID).
				Msg("Error preparing pseudo-terminal output drain")
		}
	}
	if errStartProcess != nil {
		errStart := fmt.Errorf("start command: %w", errStartProcess)
		startupResult <- errStart
		log.Error().Err(errStartProcess).Msg("Unable to start command.")
		command.sendJobNotification(errStartProcess.Error())
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
		command.finalizeExecution(processGeneration, 1, true, false, commandEndFunc)
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
	var exitCodeKnown bool
	if currentPTYCMD != nil {
		stopCancellationWatch := watchPseudoTerminalCancellation(command.processCtx, currentPTYCMD)
		errWait = currentPTYCMD.Wait()
		stopCancellationWatch()
		exitCode = extractProcessExitCode(currentPTYCMD.ProcessState, errWait)
	} else {
		errWait = currentCMD.Wait()
		exitCode = extractExitCode(currentCMD, errWait)
	}
	exitCodeKnown = errWait == nil || exitCode >= 0
	lifecycleExitCode, lifecycleExitCodeKnown := lifecycleExitDetails(command, errWait, exitCode, exitCodeKnown)
	if currentPTY != nil {
		if drainBeforeClose {
			waitForJobOutput(command.ID, outputDone)
		}
		errClosePTY := closePseudoTerminal(currentPTY, drainBeforeClose)
		if errClosePTY != nil {
			log.Debug().Err(errClosePTY).Str("Game Server ID", command.ID).
				Msg("Error closing completed pseudo-terminal")
		}
	}
	waitForJobOutput(command.ID, outputDone)
	processCtxCancel()
	closeTelnetConnectionForGeneration(command, processGeneration, inputMethodType)
	if errWait != nil {
		checkErrorAccessDenied(errWait, command)
		log.Debug().Err(errWait).Msg("Error waiting for command.")
	}

	reportUnexpectedProcessExit(command, errWait, lifecycleExitCode, exitCodeKnown)

	log.Debug().Str("Game Server ID", command.ID).Msg("Game server stopped.")
	command.finalizeExecution(processGeneration, lifecycleExitCode, lifecycleExitCodeKnown, false, commandEndFunc)
}

func lifecycleExitDetails(command *Command, errWait error, exitCode int, exitCodeKnown bool) (int, bool) {
	// ProcessState reports -1 for signal termination on Unix. Preserve that
	// sentinel on unintentional failures so remote controllers can distinguish
	// a real crash from absent terminal metadata.
	if errWait != nil && !exitCodeKnown && !command.IntentionalStop() {
		return -1, true
	}
	return exitCode, exitCodeKnown
}

func (c *Command) finalizeExecution(
	processGeneration uint64,
	exitCode int,
	exitCodeKnown bool,
	clearLaunchEnvironment bool,
	commandEndFunc func(*Command),
) {
	c.executionMutex.Lock()

	c.Lock()
	if c.processGeneration != processGeneration {
		c.Unlock()
		c.executionMutex.Unlock()
		return
	}
	oldStatus := c.status
	c.currentCMD = nil
	c.currentPTYCMD = nil
	c.currentPTY = nil
	c.status = xylona.Status_OFFLINE
	finalizationDone := c.finalizationDone
	c.finalizationDone = nil
	if clearLaunchEnvironment {
		for name := range c.launchEnv {
			c.launchEnv[name] = ""
			delete(c.launchEnv, name)
		}
	}
	c.Unlock()

	c.sendJobStatusNotificationWithExitDetails(oldStatus, exitCode, exitCodeKnown)
	c.executionMutex.Unlock()
	if finalizationDone != nil {
		close(finalizationDone)
	}
	if commandEndFunc != nil {
		commandEndFunc(c)
	}
}

func reportUnexpectedProcessExit(command *Command, errWait error, exitCode int, exitCodeKnown bool) {
	if command.IntentionalStop() {
		return
	}
	if exitCodeKnown && exitCode != 0 {
		log.Warn().Str("Game Server ID", command.ID).Int("exit_code", exitCode).Msg("Game server process crashed")
		command.sendJobNotification(formatXylonaMessage(
			fmt.Sprintf("Game server process exited unexpectedly with code %d.", exitCode),
		))
		publishProcessCrash(command, exitCode)
		return
	}
	if errWait != nil && !exitCodeKnown {
		log.Warn().Err(errWait).Str("Game Server ID", command.ID).
			Msg("Game server process ended unexpectedly without an exit code")
		command.sendJobNotification(formatXylonaMessage(
			"Game server process ended unexpectedly; exit code unavailable.",
		))
		publishProcessCrash(command, -1)
	}
}

func publishProcessCrash(command *Command, exitCode int) {
	if command.Status() != xylona.Status_ONLINE {
		return
	}
	eb := eventbus.Get()
	eb.Publish(eventbus.TopicGameServerCrashed, eventbus.ServerCrashedEvent{
		ServerID:     command.ID,
		ServerNodeID: command.nodeID,
		ExitCode:     exitCode,
		Timestamp:    time.Now(),
	})
}

func (inst *Instance) prepareCommandProcess(preparedCommand PreparedCommand) (*Command, error) {
	inst.Lock()
	persistentCommand, exists := inst.runningCommands[preparedCommand.ID]
	if exists {
		executionMutex := persistentCommand.executionMutex
		inst.Unlock()
		executionMutex.Lock()
		defer executionMutex.Unlock()
		inst.Lock()
	}
	defer inst.Unlock()

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

type telnetExecution struct {
	inputMethod       InputMethod
	processGeneration uint64
	instanceDone      <-chan struct{}
	processDone       <-chan struct{}
}

func captureTelnetExecution(command *Command) telnetExecution {
	command.RLock()
	defer command.RUnlock()
	return telnetExecution{
		inputMethod:       command.inputMethod,
		processGeneration: command.processGeneration,
		instanceDone:      command.instanceCtx.Done(),
		processDone:       command.processCtx.Done(),
	}
}

// connectTelnetAndSetAsStdinWriter owns the telnet connection for the current
// process execution. Production setup wraps connectTelnetForExecution in a
// closure so these values are captured before the goroutine is scheduled.
func connectTelnetAndSetAsStdinWriter(command *Command) {
	connectTelnetForExecution(command, captureTelnetExecution(command))
}

// connectTelnetForExecution keeps retrying with bounded backoff while one
// process execution lives and reads exactly one connection at a time.
func connectTelnetForExecution(command *Command, execution telnetExecution) {
	log.Debug().Str("Command ID", command.ID).Msg("Setting up telnet")
	inputMethod := execution.inputMethod
	processDone := execution.processDone
	instanceDone := execution.instanceDone
	processGeneration := execution.processGeneration
	if inputMethod.Type != InputTypeTelnet {
		log.Debug().Str("Command ID", command.ID).Msg("Skipping telnet setup for non-telnet input method")
		return
	}

	errValidateTelnet := validateTelnetInputMethod(inputMethod)
	if errValidateTelnet != nil {
		log.Error().Err(errValidateTelnet).Str("Command ID", command.ID).Msg("Invalid telnet input method")
		command.Lock()
		command.stdInWriter = nil
		command.telnetConn = nil
		command.telnetOutputActive.Store(false)
		command.Unlock()
		command.sendJobNotification(formatXylonaMessage("Telnet console configuration is invalid; console input is unavailable."))
		return
	}
	telnetCredentials := inputMethod.TelnetCredentials
	backoff := telnetInitialBackoff
	connectionUnavailable := false

	for {
		if commandExecutionDone(instanceDone, processDone) {
			return
		}

		telnetConnection, errConnect := connectTelnet(telnetCredentials)
		if errConnect != nil {
			log.Warn().Err(errConnect).Str("Command ID", command.ID).Dur("retry_in", backoff).
				Msg("Telnet console is unavailable; will retry")
			if !connectionUnavailable {
				command.sendJobNotification(formatXylonaMessage("Telnet console is not ready; console input is unavailable while Xylona retries."))
				connectionUnavailable = true
			}
			if waitForTelnetRetry(instanceDone, processDone, backoff) {
				return
			}
			backoff *= 2
			if backoff > telnetMaximumBackoff {
				backoff = telnetMaximumBackoff
			}
			continue
		}

		if !attachTelnetConnection(command, telnetConnection, processGeneration, instanceDone, processDone) {
			return
		}
		if connectionUnavailable {
			command.sendJobNotification(formatXylonaMessage("Telnet console connected; console input is available."))
		}
		backoff = telnetInitialBackoff

		errRead := command.readTelnetOutput(telnetConnection, processDone)
		detachTelnetConnection(command, telnetConnection, processGeneration)
		if commandExecutionDone(instanceDone, processDone) {
			return
		}
		log.Warn().Err(errRead).Str("Command ID", command.ID).Dur("retry_in", backoff).
			Msg("Telnet console disconnected; will reconnect")
		command.sendJobNotification(formatXylonaMessage("Telnet console disconnected; console input is unavailable while Xylona reconnects."))
		connectionUnavailable = true
		if waitForTelnetRetry(instanceDone, processDone, backoff) {
			return
		}
	}
}

func connectTelnet(credentials *TelnetCredentials) (*telnet.Conn, error) {
	log.Debug().Msg("Dialing telnet")
	telnetConnection, errDial := telnet.DialTimeout(
		"tcp",
		net.JoinHostPort("localhost", strconv.Itoa(credentials.Port)),
		5*time.Second,
	)
	if errDial != nil {
		return nil, fmt.Errorf("dial telnet: %w", errDial)
	}
	if credentials.Password == "" {
		return telnetConnection, nil
	}

	errDeadline := telnetConnection.SetDeadline(time.Now().Add(telnetAuthTimeout))
	if errDeadline != nil {
		return nil, closeUnauthenticatedTelnetConnection(telnetConnection, fmt.Errorf("set telnet authentication deadline: %w", errDeadline))
	}
	_, errPrompt := telnetConnection.ReadUntil(telnetLoginPrompt)
	if errPrompt != nil {
		return nil, closeUnauthenticatedTelnetConnection(telnetConnection, fmt.Errorf("read telnet password prompt: %w", errPrompt))
	}
	_, errPassword := fmt.Fprintf(telnetConnection, "%s\n", credentials.Password)
	if errPassword != nil {
		return nil, closeUnauthenticatedTelnetConnection(telnetConnection, fmt.Errorf("write telnet password: %w", errPassword))
	}
	_, resultIndex, errResult := telnetConnection.ReadUntilIndex(telnetLogonSuccessful, telnetLogonFailed)
	if errResult != nil {
		return nil, closeUnauthenticatedTelnetConnection(telnetConnection, fmt.Errorf("read telnet authentication result: %w", errResult))
	}
	if resultIndex != 0 {
		return nil, closeUnauthenticatedTelnetConnection(telnetConnection, errors.New("telnet authentication rejected"))
	}
	errClearDeadline := telnetConnection.SetDeadline(time.Time{})
	if errClearDeadline != nil {
		return nil, closeUnauthenticatedTelnetConnection(telnetConnection, fmt.Errorf("clear telnet authentication deadline: %w", errClearDeadline))
	}
	return telnetConnection, nil
}

func closeUnauthenticatedTelnetConnection(telnetConnection *telnet.Conn, authError error) error {
	errClose := telnetConnection.Close()
	if errClose != nil {
		log.Error().Err(errClose).Msg("Error closing unauthenticated telnet connection")
	}
	return authError
}

func attachTelnetConnection(
	command *Command,
	telnetConnection *telnet.Conn,
	processGeneration uint64,
	instanceDone <-chan struct{},
	processDone <-chan struct{},
) bool {
	command.Lock()
	defer command.Unlock()
	if command.processGeneration != processGeneration || commandExecutionDone(instanceDone, processDone) {
		errClose := telnetConnection.Close()
		if errClose != nil {
			log.Debug().Err(errClose).Str("Command ID", command.ID).
				Msg("Error closing late telnet connection")
		}
		return false
	}
	command.telnetConn = telnetConnection
	command.stdInWriter = telnetConnection
	command.telnetOutputActive.Store(true)
	return true
}

func detachTelnetConnection(command *Command, telnetConnection *telnet.Conn, processGeneration uint64) {
	command.Lock()
	if command.processGeneration == processGeneration && command.telnetConn == telnetConnection {
		command.telnetConn = nil
		command.stdInWriter = nil
		command.telnetOutputActive.Store(false)
	}
	command.Unlock()
	errClose := telnetConnection.Close()
	if errClose != nil {
		log.Debug().Err(errClose).Str("Command ID", command.ID).Msg("Error closing detached telnet connection")
	}
}

func commandExecutionDone(instanceDone <-chan struct{}, processDone <-chan struct{}) bool {
	select {
	case <-instanceDone:
		return true
	case <-processDone:
		return true
	default:
		return false
	}
}

func waitForTelnetRetry(instanceDone <-chan struct{}, processDone <-chan struct{}, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-instanceDone:
		return true
	case <-processDone:
		return true
	case <-timer.C:
		return false
	}
}

func closeTelnetConnectionForGeneration(command *Command, processGeneration uint64, inputMethodType inputType) {
	if inputMethodType != InputTypeTelnet {
		return
	}

	command.Lock()
	if command.processGeneration != processGeneration {
		command.Unlock()
		return
	}
	telnetConnection := command.telnetConn
	command.telnetConn = nil
	command.stdInWriter = nil
	command.telnetOutputActive.Store(false)
	command.Unlock()
	if telnetConnection == nil {
		return
	}

	log.Debug().Str("Game Server ID", command.ID).Msg("Closing telnet connection")
	errClose := telnetConnection.Close()
	if errClose != nil && !errors.Is(errClose, net.ErrClosed) {
		log.Error().Err(errClose).Str("Game Server ID", command.ID).Msg("Error closing telnet connection")
	}
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
			executionMutex:      &sync.Mutex{},
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

// SendInput sends input through the command's configured console transport.
func (c *Command) SendInput(input string) error {
	_, errExecute := c.executeInputForExecution(context.Background(), input, nil)
	return errExecute
}

func (c *Command) sendInputForExecution(input string, expectedProcessGeneration *uint64) error {
	_, errExecute := c.executeInputForExecution(context.Background(), input, expectedProcessGeneration)
	return errExecute
}

// ExecuteInput sends input through the configured transport and returns a
// synchronous response when the transport provides one.
func (c *Command) ExecuteInput(ctx context.Context, input string) (string, error) {
	return c.executeInputForExecution(ctx, input, nil)
}

func (c *Command) executeInputForExecution(
	ctx context.Context,
	input string,
	expectedProcessGeneration *uint64,
) (string, error) {
	c.RLock()
	currentCMD := c.currentCMD
	currentPTYCMD := c.currentPTYCMD
	inputWriter := c.stdInWriter
	inputMethod := c.inputMethod
	processGeneration := c.processGeneration
	c.RUnlock()
	if expectedProcessGeneration != nil && processGeneration != *expectedProcessGeneration {
		return "", fmt.Errorf("%w: command execution has changed", ErrConsoleInputUnavailable)
	}
	if currentCMD == nil && currentPTYCMD == nil {
		return "", fmt.Errorf("%w: command is not running", ErrConsoleInputUnavailable)
	}
	if inputMethod.Type == InputTypeRCON || inputMethod.Type == InputTypeREST {
		response, errExecute := executeRemoteInput(ctx, inputMethod, input)
		if errExecute != nil {
			return "", errors.Join(ErrConsoleInputUnavailable, errExecute)
		}
		if response != "" {
			c.SendOutput(response)
		}
		return response, nil
	}
	if inputWriter == nil {
		return "", fmt.Errorf("%w: %s input is not attached", ErrConsoleInputUnavailable, inputMethod.Type.String())
	}
	log.Debug().Str("Command ID", c.ID).Int("input_bytes", len(input)).Msg("Sending console input")
	inputTerminator := "\n"
	if currentPTYCMD != nil {
		inputTerminator = pseudoTerminalNewline
	}
	b, wErr := fmt.Fprintf(inputWriter, "%s%s", input, inputTerminator)
	var errConsoleInput error
	consoleInputMirrored := false
	if currentPTYCMD == nil && inputMethod.Type == InputTypeStdIn {
		consoleInputMirrored, errConsoleInput = mirrorProcessConsoleInput(currentCMD, input)
	}
	if inputMethod.Type == InputTypeTelnet && wErr != nil {
		detachFailedTelnetWriter(c, inputWriter)
		return "", errors.Join(
			ErrConsoleInputUnavailable,
			fmt.Errorf("write telnet command input: %w", wErr),
		)
	}
	if wErr != nil && errConsoleInput != nil {
		return "", errors.Join(
			ErrConsoleInputUnavailable,
			fmt.Errorf("write command input: %w", wErr),
			fmt.Errorf("mirror command input to console: %w", errConsoleInput),
		)
	}
	if wErr != nil {
		if consoleInputMirrored {
			log.Debug().Err(wErr).Str("Command ID", c.ID).
				Msg("Standard input pipe rejected command; console input succeeded")
			return "", nil
		}
		return "", errors.Join(
			ErrConsoleInputUnavailable,
			fmt.Errorf("write command input: %w", wErr),
		)
	}
	if errConsoleInput != nil {
		log.Debug().Err(errConsoleInput).Str("Command ID", c.ID).
			Msg("Console input unavailable; standard input pipe accepted command")
	}
	log.Debug().Str("Command ID", c.ID).Int("bytes written", b).Msg("Wrote input")
	return "", nil
}

func detachFailedTelnetWriter(command *Command, inputWriter io.Writer) {
	telnetConnection, ok := inputWriter.(*telnet.Conn)
	if !ok {
		return
	}
	command.Lock()
	if command.telnetConn == telnetConnection {
		command.telnetConn = nil
		command.stdInWriter = nil
		command.telnetOutputActive.Store(false)
	}
	command.Unlock()
	errClose := telnetConnection.Close()
	if errClose != nil {
		log.Debug().Err(errClose).Str("Command ID", command.ID).
			Msg("Error closing failed telnet input connection")
	}
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
