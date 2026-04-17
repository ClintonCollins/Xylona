package supervisor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/ziutek/telnet"

	internal "github.com/ClintonCollins/Xylona/api/xylona-internal"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
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

const defaultStopTimeout = 15 * time.Second

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
	if !preparedCommand.InternalCommand {
		cmd.RLock()
		currentCMDNil := cmd.currentCMD == nil
		cmd.RUnlock()
		if currentCMDNil {
			return nil, fmt.Errorf("%s", cmd.GetOutputBuffer())
		}
	}
	return cmd, nil
}

// Stop requests that the command shut down gracefully, then forces cancelation on timeout.
func (c *Command) Stop(stopInputCommand string) {
	c.intentionalStop.Store(true)
	c.RLock()
	currentCMD := c.currentCMD
	c.RUnlock()
	if currentCMD == nil {
		return
	}
	c.sendJobNotification(MessageStoppingServer)
	if stopInputCommand != "" {
		log.Debug().Str("Game Server ID", c.ID).Str("Stop Input Command", stopInputCommand).Msg("Sending stop command")
		errSend := c.SendInput(stopInputCommand)
		if errSend != nil {
			log.Error().Err(errSend).Msg("Error sending stop command")
		}
	} else if runtime.GOOS != "windows" {
		errInterrupt := currentCMD.Process.Signal(os.Interrupt)
		if errInterrupt != nil {
			log.Error().Err(errInterrupt).Msg("Error interrupting process")
			errTerm := currentCMD.Process.Signal(syscall.SIGTERM)
			if errTerm != nil {
				log.Error().Err(errTerm).Msg("Error terminating process")
			}
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

func (inst *Instance) startAndWaitForJob(command *Command, commandEndFunc func(command *Command)) {
	defer func(command *Command) {
		if command.inputMethod.Type == InputTypeTelnet {
			log.Debug().Str("Game Server ID", command.ID).Msg("Closing telnet connection")
			if command.telnetConn != nil {
				errCloseTelnetConn := command.telnetConn.Close()
				if errCloseTelnetConn != nil {
					log.Error().Err(errCloseTelnetConn).Msg("Error closing telnet connection")
				}
			}
		}
	}(command)
	log.Debug().Str("Game Server ID", command.ID).Msg("Starting job")
	// If it's an internal command, we need to run the internal command.
	if command.InternalCommand && (command.status == xylona.Status_INSTALLING || command.status == xylona.Status_UPDATING) {
		if command.internalGameServer == nil {
			log.Error().Str("Game Server ID", command.ID).Msg("Internal game server is nil")
			return
		}
		if command.gameID == nil {
			log.Error().Str("Game Server ID", command.ID).Msg("Game ID is nil")
			return
		}
		internalGame, exists := internal.GetGame(*command.gameID)
		if !exists {
			log.Error().Str("Game ID", *command.gameID).Str("Game Server ID", command.ID).Msg("Internal game does not exist")
			return
		}
		command.sendJobStatusNotification(xylona.Status_OFFLINE, command.status)
		defer func() {
			if pipeWriter, ok := command.internalCommandStdOut.(*io.PipeWriter); ok {
				_ = pipeWriter.Close()
			}
			if pipeWriter, ok := command.internalCommandStdErr.(*io.PipeWriter); ok {
				_ = pipeWriter.Close()
			}
			oldStatus := command.Status()
			command.sendJobStatusNotification(oldStatus, xylona.Status_OFFLINE)
			command.Lock()
			command.currentCMD = nil
			command.status = xylona.Status_OFFLINE
			command.Unlock()
			if commandEndFunc != nil {
				commandEndFunc(command)
			}
		}()
		switch command.status {
		case xylona.Status_INSTALLING:
			err := internalGame.Install(command.internalGameServer, command.internalCommandStdOut, command.internalCommandStdErr)
			if err != nil {
				log.Error().Err(err).Msg("Error installing internal game")
				return
			}
			return
		case xylona.Status_UPDATING:
			err := internalGame.Update(command.internalGameServer, command.internalCommandStdOut, command.internalCommandStdErr)
			if err != nil {
				log.Error().Err(err).Msg("Error updating internal game")
				return
			}
			return
		}
		log.Error().Str("Game Server ID", command.ID).Msg("Unable to find internal command to run.")
		command.sendJobNotification("Unable to find internal command to run.")
		return
	}

	// If it's not an internal command, we need to run the command.
	command.RLock()
	currentCMD := command.currentCMD
	command.RUnlock()
	if currentCMD == nil {
		return
	}
	fullCommandStr := fmt.Sprintf("%s %s", currentCMD.Path, strings.Join(currentCMD.Args, " "))
	err := currentCMD.Start()
	if err != nil {
		log.Error().Err(err).Msg("Unable to start command.")
		command.sendJobNotification(err.Error())
		oldStatus := command.Status()
		command.sendJobStatusNotification(oldStatus, xylona.Status_OFFLINE)
		command.Lock()
		command.currentCMD = nil
		command.status = xylona.Status_OFFLINE
		command.Unlock()
		if commandEndFunc != nil {
			commandEndFunc(command)
		}
		return
	}

	log.Debug().Str("Command ID", command.ID).Str("Exec", fullCommandStr).Msg("Command started")
	command.sendJobStatusNotification(xylona.Status_OFFLINE, command.status)

	// Run after startup function if it exists.
	if command.runAfterStartup != nil {
		command.runAfterStartup(command)
	}

	errWait := currentCMD.Wait()
	exitCode := extractExitCode(currentCMD, errWait)
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
	command.sendJobStatusNotificationWithExit(oldStatus, xylona.Status_OFFLINE, exitCode)
	command.Lock()
	command.currentCMD = nil
	command.status = xylona.Status_OFFLINE
	command.Unlock()
	if commandEndFunc != nil {
		commandEndFunc(command)
	}
}

func (inst *Instance) prepareCommandProcess(preparedCommand PreparedCommand) (*Command, error) {
	persistentCommand, exists := inst.runningCommands[preparedCommand.ID]
	if exists {
		persistentCommand.RLock()
		commandAlreadyRunning := persistentCommand.currentCMD != nil
		persistentCommand.RUnlock()
		if commandAlreadyRunning {
			return nil, ErrCommandAlreadyRunning
		}
	}

	newCommand := inst.initNewCommand(preparedCommand, persistentCommand)

	if !preparedCommand.InternalCommand {
		// Extracted Command setup logic to a private function.
		cmd, err := inst.setupCmd(newCommand, preparedCommand)
		if err != nil {
			return nil, err
		}

		log.Debug().Str("Command ID", preparedCommand.ID).Msg("Starting command")
		newCommand.currentCMD = cmd
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
	}

	if persistentCommand == nil {
		inst.runningCommands[preparedCommand.ID] = newCommand
	}

	go newCommand.readJobOut()
	if newCommand.status == xylona.Status_ONLINE {
		newCommand.sendJobNotification(MessageStartingServer)
	}
	go inst.startAndWaitForJob(newCommand, preparedCommand.CallbackFunction)
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

	telnetConnect := func() (*telnet.Conn, error) {
		log.Debug().Str("Command ID", command.ID).Msg("Dialing telnet")
		telnetConn, errDial := telnet.DialTimeout("tcp", net.JoinHostPort("localhost", strconv.Itoa(command.inputMethod.TelnetCredentials.Port)), time.Second*5)
		if errDial != nil {
			log.Error().Err(errDial).Msg("Error dialing telnet")
			command.stdInWriter = io.Discard
			return nil, fmt.Errorf("dial telnet: %w", errDial)
		}
		log.Debug().Msg("Telnet connection successful")
		log.Debug().Msg("Writing password to telnet")
		if command.inputMethod.TelnetCredentials.Password != "" {
			b, errAuth := telnetConn.Write([]byte(command.inputMethod.TelnetCredentials.Password))
			if errAuth != nil {
				log.Error().Err(errAuth).Msg("Error authenticating telnet")
				command.stdInWriter = io.Discard
				return nil, fmt.Errorf("authenticate telnet: %w", errAuth)
			}
			log.Debug().Int("bytes written", b).Msg("Wrote password to telnet")
		}
		return telnetConn, nil
	}

	telnetConnection, errConnect := telnetConnect()
	if errConnect == nil {
		command.telnetConn = telnetConnection
		command.stdInWriter = telnetConnection
		go command.readTelnetOutput()
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
				command.telnetConn = telnetConnection
				command.stdInWriter = telnetConnection
				go command.readTelnetOutput()
				return
			}
		}
	}
	command.stdInWriter = io.Discard
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
	persistentCommand, exists := inst.runningCommands[commandID]
	if !exists {
		inst.Lock()
		defer inst.Unlock()
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
	// TODO Implement sending to telnet if the game uses it for input.... (7 Days to Die)
	c.RLock()
	if c.currentCMD == nil {
		return errors.New("command is not running")
	}
	c.RUnlock()
	log.Debug().Str("Command ID", c.ID).Str("Input", input).Msg("Sending input")
	b, wErr := fmt.Fprintf(c.stdInWriter, "%s\n", input)
	if wErr != nil {
		return fmt.Errorf("write command input: %w", wErr)
	}
	log.Debug().Str("Command ID", c.ID).Int("bytes written", b).Msg("Wrote input")
	return nil
}

// extractExitCode returns the exit code from a completed command. It checks
// the ProcessState first (which is set even when Wait returns a context
// cancellation error on Windows), then falls back to unwrapping exec.ExitError.
// Returns 0 if err is nil or the process exited with code 0, the actual exit
// code if it can be determined, or -1 if the error is not an exit error.
func extractExitCode(cmd *exec.Cmd, err error) int {
	if err == nil {
		return 0
	}
	// ProcessState is populated after Wait completes, even when the error
	// is a context cancellation wrapping a TerminateProcess failure (Windows).
	if cmd != nil && cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	// If we can't determine the exit code but there was an error, assume non-zero.
	return -1
}
