package supervisor

import (
	"bufio"
	"bytes"
	"context"
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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/ziutek/telnet"
	"golang.org/x/sync/errgroup"

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

func formatXylonaMessage(message string) string {
	return fmt.Sprintf("[%s] [Xylona]: %s", time.Now().Format("2006-01-02 15:04:05"), message)
}

// StartCommand prepares, launches, and tracks a command execution.
func (inst *Instance) StartCommand(preparedCommand PreparedCommand) (*Command, error) {
	cmd, err := inst.prepareCommandProcess(preparedCommand)
	if err != nil {
		return nil, err
	}
	if cmd.currentCMD == nil && !preparedCommand.InternalCommand {
		return nil, fmt.Errorf("%s", cmd.GetOutputBuffer())
	}
	return cmd, nil
}

// Stop requests that the command shut down gracefully, then forces cancelation on timeout.
func (c *Command) Stop(stopInputCommand string) {
	c.intentionalStop.Store(true)
	if c.currentCMD == nil {
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
		errInterrupt := c.currentCMD.Process.Signal(os.Interrupt)
		if errInterrupt != nil {
			log.Error().Err(errInterrupt).Msg("Error interrupting process")
			errTerm := c.currentCMD.Process.Signal(syscall.SIGTERM)
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

func (c *Command) jobOutputReaders() (io.Reader, io.Reader, bool) {
	c.RLock()
	defer c.RUnlock()

	if c.currentCMD == nil && !c.InternalCommand {
		return nil, nil, false
	}
	if c.stdout == nil || c.stderr == nil {
		return nil, nil, false
	}
	return c.stdout, c.stderr, true
}

// readJobOut reads the output of the current command execution.
// It scans the combined output, splits it by lines, and processes each line.
// It pushes each line to the output buffer and handles the output listeners.
// If an error occurs while scanning the output, it logs the error.
// If the context is done, it stops reading the output.
// It closes the job notification after reading all the output.
func (c *Command) readJobOut() {
	log.Debug().Str("Game Server ID", c.ID).Msg("Reading job output")
	stdoutReader, stderrReader, ok := c.jobOutputReaders()
	if !ok {
		return
	}
	var disableOutput atomic.Bool
	scannerDone := make(chan struct{}, 2)

	wg := &sync.WaitGroup{}
	wg.Add(3)

	go func() {
		defer wg.Done()
		scannersFinished := 0
		for {
			if scannersFinished == 2 {
				return
			}
			select {
			case <-c.instanceCtx.Done():
				return
			case <-c.processCtx.Done():
				return
			case <-scannerDone:
				scannersFinished++
			case <-c.toggleOutputType:
				disableOutput.Store(!disableOutput.Load())
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer func() {
			scannerDone <- struct{}{}
		}()
		scannerStdOut := bufio.NewScanner(stdoutReader)
		scannerStdOut.Split(bufio.ScanLines)
		for scannerStdOut.Scan() {
			if scannerStdOut.Err() != nil {
				log.Error().Err(scannerStdOut.Err()).Msg("Error scanning output")
				return
			}
			select {
			case <-c.instanceCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal. Closing job output reader.")
				return
			case <-c.processCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing job output reader.")
				return
			default:
				if disableOutput.Load() {
					continue
				}
				stdOut := scannerStdOut.Text()
				log.Debug().Str("ID", c.ID).Str("stdout", stdOut).Msg("Output")
				c.sendJobNotification(stdOut)
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer func() {
			scannerDone <- struct{}{}
		}()
		scannerStdErr := bufio.NewScanner(stderrReader)
		scannerStdErr.Split(bufio.ScanLines)
		for scannerStdErr.Scan() {
			if scannerStdErr.Err() != nil {
				log.Error().Err(scannerStdErr.Err()).Msg("Error scanning output")
				return
			}
			select {
			case <-c.instanceCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal. Closing job output reader.")
				return
			case <-c.processCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing job output reader.")
				return
			default:
				if disableOutput.Load() {
					continue
				}
				stdErr := scannerStdErr.Text()
				log.Debug().Str("ID", c.ID).Str("stderr", stdErr).Msg("Output")
				c.sendJobNotification(stdErr)
			}
		}
	}()

	wg.Wait()
	log.Debug().Str("Game Server ID", c.ID).Msg("Job output listener stopped")
	c.processCtxCancel()
	c.closeJobNotification()
}

// readTelnetOutput reads the output of the telnet connection.
func (c *Command) readTelnetOutput() {
	retries := 60
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	// Wait for telnet to start.
	for {
		select {
		case <-c.instanceCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal. Closing job telnet reader.")
			return
		case <-c.processCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing job telnet reader.")
			return
		case <-ticker.C:
			log.Debug().Str("Game Server ID", c.ID).Msg("Checking if telnet is running")
		}
		if c.telnetConn != nil {
			log.Debug().Str("Game Server ID", c.ID).Msg("Telnet is running")
			c.toggleOutputType <- struct{}{}
			break
		}
		retries--
		if retries <= 0 {
			log.Debug().Str("Game Server ID", c.ID).Msg("Telnet did not start")
			return
		}
	}

	scanner := bufio.NewScanner(c.telnetConn)
	// scanner.Buffer(make([]byte, 16), 65536)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		if scanner.Err() != nil {
			log.Error().Err(scanner.Err()).Msg("Error scanning telnet")
			return
		}
		select {
		case <-c.instanceCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal.  Closing telnet reader.")
		case <-c.processCtx.Done():
			log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing telnet reader.")
			return
		default:
		}
		telnetOut := scanner.Text()
		// log.Debug().Str("Game Server ID", c.ID).Str("telnet", "out").Msg(telnetOut)
		c.sendJobNotification(telnetOut)
	}
	log.Debug().Str("Game Server ID", c.ID).Msg("Telnet listener stopped")
}

func (c *Command) handleOutputListeners(payload *xylona.Message) {
	var listenerIDsToRemove []string
	var removeLock sync.Mutex

	c.outputListenersLock.RLock()
	errGroup, ctx := errgroup.WithContext(c.instanceCtx)
	for id, listener := range c.outputListeners {
		errGroup.Go(func() error {
			select {
			case <-c.instanceCtx.Done():
				log.Debug().Str("Game Server ID", id).Msg("Received Xylona shutdown signal. Closing output listener.")
				return nil
			case <-ctx.Done():
				log.Debug().Str("Game Server ID", id).Msg("Received error group context shutdown signal. Closing output listener.")
				return nil
			case listener <- payload:
				// log.Debug().Str("ID", id).Str("out", payload.Data).Str("type", payload.OutputType.String()).Str("status", payload.Status.String()).Msg("Sending output to listener")
			// Give the channel receiver 500 milliseconds to handle the output, otherwise we discard the message.
			case <-time.After(time.Second * 1):
				// log.Debug().Msg("Had to wait for listener.")
				removeLock.Lock()
				listenerIDsToRemove = append(listenerIDsToRemove, id)
				removeLock.Unlock()
				return nil
			}
			return nil
		})
	}
	c.outputListenersLock.RUnlock()
	_ = errGroup.Wait()

	for _, id := range listenerIDsToRemove {
		log.Debug().Str("ID", id).Msg("Removing output listener")
		c.RemoveOutputListener(id)
	}
}

func (c *Command) closeJobNotification() {
	c.sendJobNotification(MessageStoppedServer)
	oldStatus := c.Status()
	c.sendJobStatusNotification(oldStatus, xylona.Status_OFFLINE)
}

func (c *Command) sendJobStatusNotification(oldStatus, newStatus xylona.Status) {
	c.handleOutputListeners(&xylona.Message{
		Type: xylona.Message_GameServerStatus,
		GameServerStatusUpdate: &xylona.GameServerStatusUpdate{
			GameServerId: c.ID,
			Status:       newStatus,
		},
	})
	c.handleStatusListeners(newStatus)

	// Publish status change to the event bus for alert evaluation.
	// Skip no-op transitions (e.g., OFFLINE→OFFLINE from concurrent shutdown paths).
	if oldStatus != newStatus {
		eb := eventbus.Get()
		eb.Publish(eventbus.TopicGameServerStatusChanged, eventbus.StatusChangedEvent{
			ServerID:     c.ID,
			ServerNodeID: c.nodeID,
			OldStatus:    oldStatus.String(),
			NewStatus:    newStatus.String(),
		})
	}
}

func (c *Command) handleStatusListeners(status xylona.Status) {
	update := &xylona.GameServerStatusUpdate{
		GameServerId: c.ID,
		Status:       status,
	}
	listenerIDsToRemove := make([]string, 0)
	c.statusListenersLock.RLock()
	for id, listener := range c.statusListeners {
		select {
		case listener <- update:
		default:
			listenerIDsToRemove = append(listenerIDsToRemove, id)
		}
	}
	c.statusListenersLock.RUnlock()
	for _, id := range listenerIDsToRemove {
		log.Debug().Str("ID", id).Msg("Removing slow status listener")
		c.RemoveStatusListener(id)
	}
}

func (c *Command) sendJobNotification(message string) {
	c.pushToOutputBuffer(message)
	c.handleOutputListeners(&xylona.Message{
		Type: xylona.Message_GameServerConsole,
		GameServerConsoleOutput: &xylona.GameServerConsoleOutput{
			GameServerId: c.ID,
			Output:       message + "\n",
		},
	})
}

// SendOutput injects a message into the command's console output stream,
// writing it to the output buffer and broadcasting it to all output listeners.
// This allows external callers (e.g., the actions layer) to surface status
// messages in the game server console without routing through stdin.
func (c *Command) SendOutput(message string) {
	c.Lock()
	if c.currentCMD == nil && c.status == xylona.Status_OFFLINE {
		c.preserveBufferedOutputOnReuse = true
	}
	c.Unlock()
	c.sendJobNotification(message)
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
	if command.currentCMD == nil {
		return
	}
	fullCommandStr := fmt.Sprintf("%s %s", command.currentCMD.Path, strings.Join(command.currentCMD.Args, " "))
	err := command.currentCMD.Start()
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

	errWait := command.currentCMD.Wait()
	exitCode := extractExitCode(command.currentCMD, errWait)
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
		if persistentCommand.currentCMD != nil {
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

func (inst *Instance) initNewCommand(preparedCommand PreparedCommand, persistentCommand *Command) *Command {
	var newCommand *Command
	processCtx, processCtxCancel := context.WithCancel(inst.ctx)
	if persistentCommand != nil {
		log.Debug().Str("Command ID", persistentCommand.ID).Msg("Reusing persistent command")
		newCommand = persistentCommand
		newCommand.Lock()
		preserveBufferedOutputOnReuse := newCommand.preserveBufferedOutputOnReuse
		newCommand.User = preparedCommand.User
		newCommand.nodeID = preparedCommand.NodeID
		newCommand.stopTimeout = preparedCommand.StopTimeout
		newCommand.outputListeners = persistentCommand.outputListeners
		newCommand.BaseCommand = preparedCommand.BaseCommand
		newCommand.Args = append([]string(nil), preparedCommand.Args...)
		newCommand.unixStartedAt = time.Now().Unix()
		newCommand.status = preparedCommand.Status
		newCommand.serviceID = preparedCommand.ServiceID
		if !preserveBufferedOutputOnReuse {
			newCommand.outBuffer = ""
		}
		newCommand.preserveBufferedOutputOnReuse = false
		newCommand.intentionalStop.Store(false)
		newCommand.instanceCtx = inst.ctx
		newCommand.processCtx = processCtx
		newCommand.processCtxCancel = processCtxCancel
		newCommand.inputMethod = preparedCommand.InputMethod
		newCommand.workingDir = preparedCommand.WorkingDirectory
		defer newCommand.Unlock()
	} else {
		log.Debug().Str("Command ID", preparedCommand.ID).Msg("Creating new command")
		newCommand = &Command{
			ID:                  preparedCommand.ID,
			User:                preparedCommand.User,
			BaseCommand:         preparedCommand.BaseCommand,
			Args:                append([]string(nil), preparedCommand.Args...),
			nodeID:              preparedCommand.NodeID,
			stopTimeout:         preparedCommand.StopTimeout,
			unixStartedAt:       time.Now().Unix(),
			status:              preparedCommand.Status,
			serviceID:           preparedCommand.ServiceID,
			RWMutex:             &sync.RWMutex{},
			stdInWriter:         &bytes.Buffer{},
			combinedOutput:      &bytes.Buffer{},
			instanceCtx:         inst.ctx,
			processCtx:          processCtx,
			processCtxCancel:    processCtxCancel,
			outputListeners:     make(map[string]chan *xylona.Message),
			outputListenersLock: &sync.RWMutex{},
			statusListeners:     make(map[string]chan *xylona.GameServerStatusUpdate),
			statusListenersLock: &sync.RWMutex{},
			inputMethod:         preparedCommand.InputMethod,
			toggleOutputType:    make(chan struct{}),
			workingDir:          preparedCommand.WorkingDirectory,
		}
	}
	return newCommand
}

func (inst *Instance) setupCmd(newCommand *Command, preparedCommand PreparedCommand) (*exec.Cmd, error) {
	log.Debug().Str("Command ID", preparedCommand.ID).Msg("Setting up command")
	baseCommand := strings.TrimSpace(preparedCommand.BaseCommand)
	if baseCommand == "" {
		log.Error().Interface("Game server ID", preparedCommand.GameServerID).Msg("No command specified")
		return nil, ErrNoCommandProvided
	}

	cmd := exec.CommandContext(newCommand.processCtx, baseCommand, preparedCommand.Args...)
	cmd.Dir = preparedCommand.WorkingDirectory

	stdOutPipe, stdErrPipe, err := inst.setupCmdPipes(newCommand, cmd)
	if err != nil {
		return nil, err
	}

	newCommand.stdout = stdOutPipe
	newCommand.stderr = stdErrPipe

	switch newCommand.inputMethod.Type {
	case InputTypeTelnet:
		log.Debug().Str("Command ID", newCommand.ID).Msg("Setting up telnet to run after startup")
		newCommand.runAfterStartup = connectTelnetAndSetAsStdinWriter
	default:
		log.Debug().Str("Command ID", newCommand.ID).Msg("Setting up StdInPipe")
		stdInPipe, errStdInPipe := cmd.StdinPipe()
		if errStdInPipe != nil {
			log.Error().Err(errStdInPipe).Msg("Unable to get StdInPipe")
			return nil, fmt.Errorf("create stdin pipe: %w", errStdInPipe)
		}
		newCommand.stdInWriter = stdInPipe
	}
	return cmd, nil
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

func (inst *Instance) setupCmdPipes(newCommand *Command, cmd *exec.Cmd) (io.Reader, io.Reader, error) {
	log.Debug().Str("Command ID", newCommand.ID).Msg("Setting up command pipes")
	stdOutPipe, errStdOutPipe := cmd.StdoutPipe()
	if errStdOutPipe != nil {
		log.Error().Err(errStdOutPipe).Msg("Unable to get StdOutPipe")
		return nil, nil, fmt.Errorf("create stdout pipe: %w", errStdOutPipe)
	}

	stdErrPipe, errStdErrPipe := cmd.StderrPipe()
	if errStdErrPipe != nil {
		log.Error().Err(errStdErrPipe).Msg("Unable to get StdErrPipe")
		return nil, nil, fmt.Errorf("create stderr pipe: %w", errStdErrPipe)
	}
	return stdOutPipe, stdErrPipe, nil
}

// GetCommandByID returns a tracked command by ID.
func (inst *Instance) GetCommandByID(commandID string) (*Command, error) {
	inst.RLock()
	defer inst.RUnlock()
	proc, exists := inst.runningCommands[commandID]
	if !exists || proc == nil {
		// log.Debug().Str("Command ID", commandID).Msg("Command does not exist.")
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

func (c *Command) pushToOutputBuffer(output string) {
	c.Lock()
	defer c.Unlock()
	if len(c.outBuffer) > maxOutputBufferBytes {
		c.outBuffer = c.outBuffer[len(c.outBuffer)-maxOutputBufferBytes:]
	}
	c.outBuffer += output + "\n"
}

// GetOutputBuffer returns the buffered command output.
func (c *Command) GetOutputBuffer() string {
	c.RLock()
	defer c.RUnlock()
	return c.outBuffer
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
