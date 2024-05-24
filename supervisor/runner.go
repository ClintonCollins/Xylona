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
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/ziutek/telnet"
	"golang.org/x/sync/errgroup"

	internal "github.com/ClintonCollins/Xylona/api/xylona-internal"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type inputType int

const (
	maxOutputBufferBytes = 1024 << 10

	InputTypeStdIn inputType = iota
	InputTypeTelnet
)

var (
	MessageStartingServer = formatXylonaMessage("Starting server...")
	MessageStoppingServer = formatXylonaMessage("Stopping server...")
	MessageStoppedServer  = formatXylonaMessage("Server stopped.")
)

type TelnetCredentials struct {
	Port     int
	Password string
}

type InputMethod struct {
	Type              inputType
	TelnetCredentials *TelnetCredentials
}

type PreparedCommand struct {
	ID                 string
	InternalCommand    bool
	InternalGameServer *models.GameServer
	FullCommandAndArgs string
	WorkingDirectory   string
	User               string
	ServiceID          string      // ServiceID is usually the ID of the game this command is associated with.
	InputMethod        InputMethod // InputMethod is used to determine how to send input to the command.
	GameID             *string
	GameServerID       *string
	CallbackFunction   func(*Command)
	Status             xylona.Status
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

func formatXylonaMessage(message string) string {
	return fmt.Sprintf("[%s] [Xylona]: %s", time.Now().Format("2006-01-02 15:04:05"), message)
}

func (inst *Instance) StartCommand(preparedCommand PreparedCommand) (*Command, error) {
	cmd, err := inst.prepareCommandProcess(preparedCommand)
	if err != nil {
		return nil, err
	}
	if cmd.currentCMD == nil && !preparedCommand.InternalCommand {
		return nil, fmt.Errorf(cmd.GetOutputBuffer())
	}
	return cmd, nil
}

func (c *Command) Stop(stopInputCommand string) {
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
	} else {
		if runtime.GOOS != "windows" {
			errInterrupt := c.currentCMD.Process.Signal(os.Interrupt)
			if errInterrupt != nil {
				log.Error().Err(errInterrupt).Msg("Error interrupting process")
				errTerm := c.currentCMD.Process.Signal(syscall.SIGTERM)
				if errTerm != nil {
					log.Error().Err(errTerm).Msg("Error terminating process")
				}
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
	case <-time.After(time.Second * 15):
		c.RLock()
		log.Warn().Str("ID", c.ID).Str("User", c.User).Msg("Timeout waiting for command to stop")
		c.RUnlock()
		c.processCtxCancel()
	}
}

func (inst *Instance) ListCommands() []Command {
	inst.RLock()
	defer inst.RUnlock()
	var commands []Command
	for _, p := range inst.runningCommands {
		commands = append(commands, *p)
	}
	return commands
}

// readJobOut reads the output of the current command execution.
// It scans the combined output, splits it by lines, and processes each line.
// It pushes each line to the output buffer and handles the output listeners.
// If an error occurs while scanning the output, it logs the error.
// If the context is done, it stops reading the output.
// It closes the job notification after reading all the output.
func (c *Command) readJobOut() {
	log.Debug().Str("Game Server ID", c.ID).Msg("Reading job output")
	if c.currentCMD == nil && !c.InternalCommand {
		return
	}
	disableOutput := false

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		scannerStdOut := bufio.NewScanner(c.stdout)
		scannerStdOut.Split(bufio.ScanLines)
		for scannerStdOut.Scan() {
			if scannerStdOut.Err() != nil {
				log.Error().Err(scannerStdOut.Err()).Msg("Error scanning output")
				return
			}
			select {
			case <-c.instanceCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal. Closing job output reader.")
			case <-c.processCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing job output reader.")
				return
			case <-c.toggleOutputType:
				disableOutput = !disableOutput
			default:
				if disableOutput {
					continue
				}
				stdOut := scannerStdOut.Text()
				log.Debug().Str("ID", c.ID).Str("stdout", stdOut).Msg("Output")
				c.sendJobNotification(stdOut)
			}
		}
		wg.Done()
	}()

	go func() {
		scannerStdErr := bufio.NewScanner(c.stderr)
		scannerStdErr.Split(bufio.ScanLines)
		for scannerStdErr.Scan() {
			if scannerStdErr.Err() != nil {
				log.Error().Err(scannerStdErr.Err()).Msg("Error scanning output")
				return
			}
			select {
			case <-c.instanceCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received Xylona shutdown signal. Closing job output reader.")
			case <-c.processCtx.Done():
				log.Debug().Str("Game Server ID", c.ID).Msg("Received job process context shutdown signal. Closing job output reader.")
				return
			case <-c.toggleOutputType:
				disableOutput = !disableOutput
			default:
				if disableOutput {
					continue
				}
				stdErr := scannerStdErr.Text()
				log.Debug().Str("ID", c.ID).Str("stderr", stdErr).Msg("Output")
				c.sendJobNotification(stdErr)
			}
		}
		wg.Done()
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

func (c *Command) handleOutputListeners(payload xylona.Message) {
	listenerIDsToRemove := make([]string, 0)
	c.outputListenersLock.RLock()
	errGroup, ctx := errgroup.WithContext(c.instanceCtx)
	for id, listener := range c.outputListeners {
		id, listener := id, listener
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
				listenerIDsToRemove = append(listenerIDsToRemove, id)
				return nil
			}
			return nil
		})
	}
	c.outputListenersLock.RUnlock()
	for _, id := range listenerIDsToRemove {
		log.Debug().Str("ID", id).Msg("Removing output listener")
		c.RemoveOutputListener(id)
	}
}

func (c *Command) closeJobNotification() {
	c.sendJobNotification(MessageStoppedServer)
	c.sendJobStatusNotification(xylona.Status_OFFLINE)
}

func (c *Command) sendJobStatusNotification(status xylona.Status) {
	payload := xylona.Message{
		Type: xylona.Message_GameServerStatus,
		GameServerStatusUpdate: &xylona.GameServerStatusUpdate{
			GameServerId: c.ID,
			Status:       status,
		},
	}
	c.handleOutputListeners(payload)
}

func (c *Command) sendJobNotification(message string) {
	c.pushToOutputBuffer(message)
	payload := xylona.Message{
		Type: xylona.Message_GameServerConsole,
		GameServerConsoleOutput: &xylona.GameServerConsoleOutput{
			GameServerId: c.ID,
			Output:       message + "\n",
		},
	}
	c.handleOutputListeners(payload)
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
		command.sendJobStatusNotification(command.status)
		defer func() {
			command.sendJobStatusNotification(xylona.Status_OFFLINE)
			command.Lock()
			command.currentCMD = nil
			command.status = xylona.Status_OFFLINE
			command.Unlock()
			commandEndFunc(command)
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
		command.sendJobStatusNotification(xylona.Status_OFFLINE)
		command.Lock()
		command.currentCMD = nil
		command.status = xylona.Status_OFFLINE
		command.Unlock()
		commandEndFunc(command)
		return
	}

	fullCommandStr = fmt.Sprintf("%s %s", command.currentCMD.Path, strings.Join(command.currentCMD.Args, " "))
	log.Debug().Str("Command ID", command.ID).Str("Exec", fullCommandStr).Msg("Command started")
	command.sendJobStatusNotification(command.status)

	// Run after startup function if it exists.
	if command.runAfterStartup != nil {
		command.runAfterStartup(command)
	}

	err = command.currentCMD.Wait()
	if err != nil {
		checkErrorAccessDenied(err, command)
		log.Debug().Err(err).Msg("Error waiting for command.")
	}
	log.Debug().Str("Game Server ID", command.ID).Msg("Game server stopped.")
	command.Lock()
	command.currentCMD = nil
	command.status = xylona.Status_OFFLINE
	command.Unlock()
	commandEndFunc(command)
}

func (inst *Instance) prepareCommandProcess(preparedCommand PreparedCommand) (*Command, error) {
	persistentCommand, exists := inst.runningCommands[preparedCommand.ID]
	if exists {
		if persistentCommand.currentCMD != nil {
			return nil, fmt.Errorf("command is already running")
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
	switch newCommand.status {
	case xylona.Status_ONLINE:
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
		newCommand.User = preparedCommand.User
		newCommand.outputListeners = persistentCommand.outputListeners
		newCommand.FullCommandAndArgs = preparedCommand.FullCommandAndArgs
		newCommand.unixStartedAt = time.Now().Unix()
		newCommand.status = preparedCommand.Status
		newCommand.serviceID = preparedCommand.ServiceID
		newCommand.outBuffer = ""
		newCommand.instanceCtx = inst.ctx
		newCommand.processCtx = processCtx
		newCommand.processCtxCancel = processCtxCancel
		newCommand.inputMethod = preparedCommand.InputMethod
		defer newCommand.Unlock()
	} else {
		log.Debug().Str("Command ID", preparedCommand.ID).Msg("Creating new command")
		newCommand = &Command{
			ID:                  preparedCommand.ID,
			User:                preparedCommand.User,
			FullCommandAndArgs:  preparedCommand.FullCommandAndArgs,
			unixStartedAt:       time.Now().Unix(),
			status:              preparedCommand.Status,
			serviceID:           preparedCommand.ServiceID,
			RWMutex:             &sync.RWMutex{},
			stdInWriter:         &bytes.Buffer{},
			combinedOutput:      &bytes.Buffer{},
			instanceCtx:         inst.ctx,
			processCtx:          processCtx,
			processCtxCancel:    processCtxCancel,
			outputListeners:     make(map[string]chan xylona.Message),
			outputListenersLock: &sync.RWMutex{},
			inputMethod:         preparedCommand.InputMethod,
			toggleOutputType:    make(chan struct{}),
		}
	}
	return newCommand
}

func (inst *Instance) setupCmd(newCommand *Command, preparedCommand PreparedCommand) (*exec.Cmd, error) {
	log.Debug().Str("Command ID", preparedCommand.ID).Msg("Setting up command")
	commandSplit := strings.Fields(preparedCommand.FullCommandAndArgs)
	if len(commandSplit) <= 0 {
		log.Error().Interface("Game server ID", preparedCommand.GameServerID).Str("Command", preparedCommand.FullCommandAndArgs).Msg("No command specified")
		return nil, fmt.Errorf("no command provided")
	}
	command := commandSplit[0]
	args := commandSplit[1:]

	cmd := exec.CommandContext(newCommand.processCtx, command, args...)
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
			return nil, errStdInPipe
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
			return nil, errDial
		}
		log.Debug().Msg("Telnet connection successful")
		log.Debug().Str("telnet password", command.inputMethod.TelnetCredentials.Password).Msg("Writing password to telnet")
		if command.inputMethod.TelnetCredentials.Password != "" {
			b, errAuth := telnetConn.Write([]byte(command.inputMethod.TelnetCredentials.Password))
			if errAuth != nil {
				log.Error().Err(errAuth).Msg("Error authenticating telnet")
				command.stdInWriter = io.Discard
				return nil, errDial
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
		return nil, nil, errStdOutPipe
	}

	stdErrPipe, errStdErrPipe := cmd.StderrPipe()
	if errStdErrPipe != nil {
		log.Error().Err(errStdErrPipe).Msg("Unable to get StdErrPipe")
		return nil, nil, errStdErrPipe
	}
	return stdOutPipe, stdErrPipe, nil
}

func (inst *Instance) GetCommandByID(commandID string) (*Command, error) {
	proc, exists := inst.runningCommands[commandID]
	if !exists || proc == nil {
		// log.Debug().Str("Command ID", commandID).Msg("Command does not exist.")
		return nil, ErrCommandDoesNotExist
	}
	return proc, nil
}

func (inst *Instance) GetCommandByIDOrCreateShell(commandID string) *Command {
	persistentCommand, exists := inst.runningCommands[commandID]
	if !exists {
		inst.Lock()
		defer inst.Unlock()
		inst.runningCommands[commandID] = &Command{
			ID:                  commandID,
			stdInWriter:         &bytes.Buffer{},
			combinedOutput:      &bytes.Buffer{},
			outputListeners:     make(map[string]chan xylona.Message),
			outputListenersLock: &sync.RWMutex{},
			RWMutex:             &sync.RWMutex{},
			status:              xylona.Status_OFFLINE,
			toggleOutputType:    make(chan struct{}),
		}
		return inst.runningCommands[commandID]
	}
	return persistentCommand
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
	b, wErr := c.stdInWriter.Write([]byte(fmt.Sprintf("%s\n", input)))
	if wErr != nil {
		return wErr
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

func (c *Command) GetOutputBuffer() string {
	c.RLock()
	defer c.RUnlock()
	return c.outBuffer
}
