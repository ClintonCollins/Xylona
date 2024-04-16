package supervisor

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	maxOutputBufferBytes = 1024 << 10
)

type PreparedCommand struct {
	ID                 string
	FullCommandAndArgs string
	WorkingDirectory   string
	User               string
	GameServerID       *string
	CallbackFunction   func(*Command)
}

func (inst *Instance) StartCommand(preparedCommand PreparedCommand) (*Command, error) {
	cmd, err := inst.prepareCommandProcess(preparedCommand)
	if err != nil {
		return nil, err
	}
	if cmd.currentCMD == nil {
		return nil, fmt.Errorf(cmd.GetOutputBuffer())
	}
	return cmd, nil
}

func (c *Command) Stop() {
	c.ctxCancel()
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
	if c.currentCMD == nil {
		return
	}
	scanner := bufio.NewScanner(c.combinedOutput)
	scanner.Split(bufio.ScanLines)
	skippedOutputSessions := make(map[string]int)
	maxSkipsAllowedBeforeClose := 5
	for scanner.Scan() {
		if scanner.Err() != nil {
			log.Error().Err(scanner.Err()).Msg("Error scanning output")
			return
		}
		select {
		case <-c.ctx.Done():
			return
		default:
			stdOut := scanner.Text()
			c.pushToOutputBuffer(stdOut)
			c.handleOutputListeners(stdOut, skippedOutputSessions, maxSkipsAllowedBeforeClose)
		}
	}
	c.closeJobNotification()
}

func (c *Command) handleOutputListeners(stdOut string, skippedOutputSessions map[string]int, maxSkipsAllowedBeforeClose int) {
	listenerIDsToRemove := make([]string, 0)
	c.outputListenersLock.RLock()
	for id, listener := range c.outputListeners {
		select {
		case listener <- stdOut + "\n":
		// Give the channel receiver 100 milliseconds to handle the output, otherwise we discard the message.
		case <-time.After(time.Millisecond * 500):
			if skippedOutputSessions[id] >= maxSkipsAllowedBeforeClose {
				listenerIDsToRemove = append(listenerIDsToRemove, id)
				continue
			}
			skippedOutputSessions[id]++
			continue
		}
	}
	c.outputListenersLock.RUnlock()
	for _, id := range listenerIDsToRemove {
		log.Debug().Str("ID", id).Msg("Removing output listener")
		c.RemoveOutputListener(id)
	}
}

func (c *Command) closeJobNotification() {
	c.sendJobNotification("Server stopped...")
}

func (c *Command) sendJobNotification(message string) {
	c.pushToOutputBuffer(message)
	c.handleOutputListeners(message, make(map[string]int), 0)
}

func (inst *Instance) startAndWaitForJob(command *Command, commandEndFunc func(command *Command)) {
	if command.currentCMD == nil {
		return
	}
	err := command.currentCMD.Start()
	if err != nil {
		log.Error().Err(err).Msg("Unable to start command.")
		command.sendJobNotification(err.Error())
		command.Lock()
		command.currentCMD = nil
		command.Unlock()
		commandEndFunc(command)
		return
	}
	err = command.currentCMD.Wait()
	if err != nil {
		log.Error().Err(err).Msg("Error waiting for command.")
		command.sendJobNotification(err.Error())
	}
	command.Lock()
	command.currentCMD = nil
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

	ctx, cancelFunc := context.WithCancel(inst.ctx)

	newCommand := inst.initNewCommand(ctx, cancelFunc, preparedCommand, persistentCommand)

	// Extracted Command setup logic to a private function.
	cmd, err := inst.setupCmd(newCommand, preparedCommand)
	if err != nil {
		return nil, err
	}

	newCommand.currentCMD = cmd
	if persistentCommand == nil {
		inst.runningCommands[preparedCommand.ID] = newCommand
	}

	go newCommand.readJobOut()
	go inst.startAndWaitForJob(newCommand, preparedCommand.CallbackFunction)
	return newCommand, nil
}

func (inst *Instance) initNewCommand(ctx context.Context, cancelFunc context.CancelFunc, preparedCommand PreparedCommand, persistentCommand *Command) *Command {
	var newCommand *Command
	if persistentCommand != nil {
		log.Debug().Str("Command ID", persistentCommand.ID).Msg("Reusing persistent command")
		newCommand = persistentCommand
		newCommand.Lock()
		newCommand.User = preparedCommand.User
		newCommand.outputListeners = persistentCommand.outputListeners
		newCommand.FullCommandAndArgs = preparedCommand.FullCommandAndArgs
		newCommand.UnixStartedAt = time.Now().Unix()
		newCommand.outBuffer = ""
		newCommand.ctx, newCommand.ctxCancel = ctx, cancelFunc
		defer newCommand.Unlock()
	} else {
		log.Debug().Str("Command ID", preparedCommand.ID).Msg("Creating new command")
		newCommand = &Command{
			User:                preparedCommand.User,
			FullCommandAndArgs:  preparedCommand.FullCommandAndArgs,
			UnixStartedAt:       time.Now().Unix(),
			RWMutex:             &sync.RWMutex{},
			stdInWriter:         &bytes.Buffer{},
			combinedOutput:      &bytes.Buffer{},
			ctx:                 ctx,
			ctxCancel:           cancelFunc,
			outputListeners:     make(map[string]chan string),
			outputListenersLock: &sync.RWMutex{},
		}
	}
	return newCommand
}

func (inst *Instance) setupCmd(newCommand *Command, preparedCommand PreparedCommand) (*exec.Cmd, error) {
	commandSplit := strings.Fields(preparedCommand.FullCommandAndArgs)
	if len(commandSplit) <= 0 {
		log.Error().Interface("Game server ID", preparedCommand.GameServerID).Str("Command", preparedCommand.FullCommandAndArgs).Msg("No command specified")
		return nil, fmt.Errorf("no command provided")
	}
	command := commandSplit[0]
	args := commandSplit[1:]

	cmd := exec.CommandContext(newCommand.ctx, command, args...)
	cmd.Dir = preparedCommand.WorkingDirectory

	stdOutPipe, stdErrPipe, err := inst.setupCmdPipes(newCommand, cmd)
	if err != nil {
		return nil, err
	}

	combinedOutput := io.MultiReader(stdOutPipe, stdErrPipe)
	newCommand.combinedOutput = combinedOutput

	stdInPipe, errStdInPipe := cmd.StdinPipe()
	if errStdInPipe != nil {
		log.Error().Err(errStdInPipe).Msg("Unable to get StdInPipe")
		return nil, errStdInPipe
	}
	newCommand.stdInWriter = stdInPipe
	return cmd, nil
}

func (inst *Instance) setupCmdPipes(newCommand *Command, cmd *exec.Cmd) (io.Reader, io.Reader, error) {
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
		return nil, errors.New(fmt.Sprintf("Command with ID: %s does not exist.", commandID))
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
			outputListeners:     make(map[string]chan string),
			outputListenersLock: &sync.RWMutex{},
			RWMutex:             &sync.RWMutex{},
		}
		return inst.runningCommands[commandID]
	}
	return persistentCommand
}

func (c *Command) SendInput(input string) error {
	_, wErr := c.stdInWriter.Write([]byte(fmt.Sprintf("%s\n", input)))
	if wErr != nil {
		return wErr
	}
	return nil
}

func (c *Command) pushToOutputBuffer(output string) {
	c.Lock()
	defer c.Unlock()
	if len(c.outBuffer) > maxOutputBufferBytes {
		log.Debug().Msg("Over buffer size")
		c.outBuffer = c.outBuffer[len(c.outBuffer)-maxOutputBufferBytes:]
	}
	c.outBuffer += output + "\n"
}

func (c *Command) GetOutputBuffer() string {
	c.RLock()
	defer c.RUnlock()
	return c.outBuffer
}
