package supervisor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	pty "github.com/aymanbagabas/go-pty"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (inst *Instance) initNewCommand(preparedCommand PreparedCommand, persistentCommand *Command) *Command {
	if preparedCommand.AttemptStartedAt.IsZero() {
		preparedCommand.AttemptStartedAt = time.Now().UTC()
	}
	redactValues := slices.Clone(preparedCommand.RedactValues)
	for _, value := range preparedCommand.LaunchEnv {
		redactValues = append(redactValues, value)
	}
	if preparedCommand.InputMethod.TelnetCredentials != nil {
		redactValues = append(redactValues, preparedCommand.InputMethod.TelnetCredentials.Password)
	}
	if preparedCommand.InputMethod.RCONCredentials != nil {
		redactValues = append(redactValues, preparedCommand.InputMethod.RCONCredentials.Password)
	}
	if preparedCommand.InputMethod.RESTCredentials != nil {
		redactValues = append(redactValues, preparedCommand.InputMethod.RESTCredentials.Password)
		redactValues = append(redactValues, preparedCommand.InputMethod.RESTCredentials.PreviousPasswords...)
	}
	var newCommand *Command
	var internalLaunchEnv map[string]string
	if preparedCommand.InternalCommand {
		internalLaunchEnv = maps.Clone(preparedCommand.LaunchEnv)
	}
	processCtx, processCtxCancel := context.WithCancel(inst.ctx)
	gameServerName := preparedCommand.GameServerName
	if gameServerName == "" && preparedCommand.InternalGameServer != nil {
		gameServerName = preparedCommand.InternalGameServer.Name
	}
	if persistentCommand != nil {
		log.Debug().Str("Command ID", persistentCommand.ID).Msg("Reusing persistent command")
		newCommand = persistentCommand
		newCommand.Lock()
		newCommand.processGeneration++
		preserveBufferedOutputOnReuse := newCommand.preserveBufferedOutputOnReuse
		newCommand.User = preparedCommand.User
		newCommand.executionID = preparedCommand.ExecutionID
		newCommand.attemptStartedAt = preparedCommand.AttemptStartedAt
		clear(newCommand.redactValues)
		newCommand.redactValues = redactValues
		newCommand.failure = nil
		newCommand.failureOutput.Reset()
		newCommand.failureOutputTruncated = false
		newCommand.nodeID = preparedCommand.NodeID
		newCommand.stopTimeout = preparedCommand.StopTimeout
		newCommand.BaseCommand = preparedCommand.BaseCommand
		newCommand.Args = append([]string(nil), preparedCommand.Args...)
		newCommand.gameServerName = gameServerName
		newCommand.unixStartedAt = time.Now().Unix()
		newCommand.status = preparedCommand.Status
		newCommand.serviceID = preparedCommand.ServiceID
		if !preserveBufferedOutputOnReuse {
			newCommand.outputListenersLock.Lock()
			newCommand.outBuffer = ""
			newCommand.outputListenersLock.Unlock()
		}
		newCommand.preserveBufferedOutputOnReuse = false
		newCommand.intentionalStop.Store(false)
		newCommand.previousStatus = xylona.Status_UNKNOWN
		newCommand.transitionSequence = 0
		newCommand.lastExitCode = 0
		newCommand.exitCodeKnown = false
		newCommand.instanceCtx = inst.ctx
		newCommand.processCtx = processCtx
		newCommand.processCtxCancel = processCtxCancel
		newCommand.finalizationDone = make(chan struct{})
		newCommand.inputMethod = preparedCommand.InputMethod
		newCommand.workingDir = preparedCommand.WorkingDirectory
		newCommand.launchEnv = internalLaunchEnv
		newCommand.statusEventHook = inst.statusEventHook
		newCommand.suppressStatusEvents = preparedCommand.SuppressStatusEvents
		defer newCommand.Unlock()
	} else {
		log.Debug().Str("Command ID", preparedCommand.ID).Msg("Creating new command")
		newCommand = &Command{
			attemptStartedAt:     preparedCommand.AttemptStartedAt,
			redactValues:         redactValues,
			ID:                   preparedCommand.ID,
			executionID:          preparedCommand.ExecutionID,
			User:                 preparedCommand.User,
			BaseCommand:          preparedCommand.BaseCommand,
			Args:                 append([]string(nil), preparedCommand.Args...),
			gameServerName:       gameServerName,
			nodeID:               preparedCommand.NodeID,
			stopTimeout:          preparedCommand.StopTimeout,
			unixStartedAt:        time.Now().Unix(),
			status:               preparedCommand.Status,
			serviceID:            preparedCommand.ServiceID,
			RWMutex:              &sync.RWMutex{},
			stdInWriter:          &bytes.Buffer{},
			combinedOutput:       &bytes.Buffer{},
			instanceCtx:          inst.ctx,
			processCtx:           processCtx,
			processCtxCancel:     processCtxCancel,
			processGeneration:    1,
			finalizationDone:     make(chan struct{}),
			executionMutex:       &sync.Mutex{},
			outputListeners:      make(map[string]chan *xylona.Message),
			outputListenersLock:  &sync.RWMutex{},
			statusListeners:      make(map[string]chan *xylona.GameServerStatusUpdate),
			statusListenersLock:  &sync.RWMutex{},
			inputMethod:          preparedCommand.InputMethod,
			toggleOutputType:     make(chan struct{}),
			workingDir:           preparedCommand.WorkingDirectory,
			launchEnv:            internalLaunchEnv,
			statusEventHook:      inst.statusEventHook,
			suppressStatusEvents: preparedCommand.SuppressStatusEvents,
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

	errValidateInput := validateInputMethod(newCommand.inputMethod)
	if errValidateInput != nil {
		log.Error().Err(errValidateInput).Str("Command ID", preparedCommand.ID).Msg("Invalid console input method")
		return nil, errValidateInput
	}

	resolvedBaseCommand := resolveServerLocalBaseCommand(baseCommand, preparedCommand.WorkingDirectory)
	processBaseCommand, processArgs, errInvocation := prepareProcessInvocation(resolvedBaseCommand, preparedCommand.Args)
	if errInvocation != nil {
		return nil, errInvocation
	}
	cmd := exec.CommandContext(
		newCommand.processCtx,
		processBaseCommand,
		processArgs...,
	)
	configureProcessTree(cmd)
	cmd.Cancel = func() error {
		return terminateProcessTree(cmd.Process)
	}
	cmd.Dir = preparedCommand.WorkingDirectory
	cmd.Env = appendLaunchEnvironment(buildChildEnvironment(CurrentRuntime, os.Environ()), preparedCommand.LaunchEnv)

	stdOutPipe, stdErrPipe, err := inst.setupCmdPipes(newCommand, cmd)
	if err != nil {
		return nil, err
	}

	newCommand.stdout = stdOutPipe
	newCommand.stderr = stdErrPipe
	newCommand.runAfterStartup = nil

	switch newCommand.inputMethod.Type {
	case InputTypeTelnet:
		log.Debug().Str("Command ID", newCommand.ID).Msg("Setting up telnet to run after startup")
		newCommand.telnetConn = nil
		newCommand.stdInWriter = nil
		newCommand.telnetOutputActive.Store(false)
		telnetExecution := captureTelnetExecution(newCommand)
		newCommand.runAfterStartup = func(command *Command) {
			connectTelnetForExecution(command, telnetExecution)
		}
	case InputTypeRCON:
		newCommand.stdInWriter = nil
		newCommand.runAfterStartup = nil
	case InputTypeREST:
		newCommand.stdInWriter = nil
		newCommand.runAfterStartup = nil
		if newCommand.inputMethod.RESTCredentials.Kind == RESTInputKindSatisfactory {
			restCredentials := *newCommand.inputMethod.RESTCredentials
			commandID := newCommand.ID
			gameServerName := newCommand.gameServerName
			processCtx := newCommand.processCtx
			newCommand.runAfterStartup = func(_ *Command) {
				configureSatisfactoryAdminPasswordAfterStartup(
					processCtx,
					commandID,
					gameServerName,
					restCredentials,
				)
			}
		}
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

func (inst *Instance) setupPseudoTerminal(
	newCommand *Command,
	preparedCommand PreparedCommand,
) (*pty.Cmd, pty.Pty, error) {
	baseCommand := strings.TrimSpace(preparedCommand.BaseCommand)
	if baseCommand == "" {
		return nil, nil, ErrNoCommandProvided
	}
	terminal, errNewPTY := pty.New()
	if errNewPTY != nil {
		return nil, nil, fmt.Errorf("create pseudo-terminal: %w", errNewPTY)
	}
	errResize := terminal.Resize(160, 50)
	if errResize != nil {
		errClose := terminal.Close()
		return nil, nil, errors.Join(
			fmt.Errorf("resize pseudo-terminal: %w", errResize),
			wrapSupervisorError("close pseudo-terminal", errClose),
		)
	}
	resolvedBaseCommand := resolveServerLocalBaseCommand(baseCommand, preparedCommand.WorkingDirectory)
	processBaseCommand, processArgs, errInvocation := prepareProcessInvocation(resolvedBaseCommand, preparedCommand.Args)
	if errInvocation != nil {
		errClose := terminal.Close()
		return nil, nil, errors.Join(errInvocation, wrapSupervisorError("close pseudo-terminal", errClose))
	}
	command := preparePseudoTerminalCommand(
		newCommand.processCtx,
		terminal,
		processBaseCommand,
		processArgs,
	)
	command.Dir = preparedCommand.WorkingDirectory
	command.Env = appendLaunchEnvironment(buildChildEnvironment(CurrentRuntime, os.Environ()), preparedCommand.LaunchEnv)
	command.Cancel = func() error {
		return terminateProcessTree(command.Process)
	}
	newCommand.stdout = terminal
	newCommand.stderr = strings.NewReader("")
	newCommand.stdInWriter = terminal
	newCommand.runAfterStartup = nil
	return command, terminal, nil
}

func wrapSupervisorError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func validateTelnetInputMethod(inputMethod InputMethod) error {
	if inputMethod.Type != InputTypeTelnet {
		return nil
	}
	if inputMethod.TelnetCredentials == nil {
		return ErrTelnetCredentialsRequired
	}
	if inputMethod.TelnetCredentials.Port <= 0 {
		return ErrTelnetPortRequired
	}
	return nil
}

func validateInputMethod(inputMethod InputMethod) error {
	errTelnet := validateTelnetInputMethod(inputMethod)
	if errTelnet != nil {
		return errTelnet
	}
	switch inputMethod.Type {
	case InputTypeRCON:
		credentials := inputMethod.RCONCredentials
		if credentials == nil || strings.TrimSpace(credentials.Host) == "" ||
			credentials.Port <= 0 || credentials.Port > 65535 || strings.TrimSpace(credentials.Password) == "" {
			return ErrRemoteInputConfiguration
		}
		if credentials.Protocol != RCONProtocolSource &&
			credentials.Protocol != RCONProtocolMinecraft &&
			credentials.Protocol != RCONProtocolRustWeb {
			return ErrRemoteInputConfiguration
		}
	case InputTypeREST:
		credentials := inputMethod.RESTCredentials
		if credentials == nil || strings.TrimSpace(credentials.Host) == "" ||
			credentials.Port <= 0 || credentials.Port > 65535 ||
			strings.TrimSpace(credentials.Password) == "" {
			return ErrRemoteInputConfiguration
		}
		if credentials.Kind != RESTInputKindSatisfactory && credentials.Kind != RESTInputKindPalworld {
			return ErrRemoteInputConfiguration
		}
	}
	return nil
}

func resolveServerLocalBaseCommand(baseCommand string, workingDir string) string {
	baseCommand = strings.TrimSpace(baseCommand)
	if baseCommand == "" || workingDir == "" {
		return baseCommand
	}
	if filepath.IsAbs(baseCommand) {
		return baseCommand
	}
	if !looksLikeServerLocalBaseCommand(baseCommand) {
		return baseCommand
	}

	cleanBaseCommand := filepath.Clean(baseCommand)
	if !filepath.IsLocal(cleanBaseCommand) {
		return baseCommand
	}

	candidate := filepath.Join(workingDir, cleanBaseCommand)
	info, errStat := os.Stat(candidate)
	if errStat != nil {
		return baseCommand
	}
	if info.IsDir() {
		return baseCommand
	}

	return candidate
}

func looksLikeServerLocalBaseCommand(baseCommand string) bool {
	if strings.ContainsAny(baseCommand, `/\`) {
		return true
	}
	if strings.HasPrefix(baseCommand, ".") {
		return true
	}
	if filepath.Ext(baseCommand) != "" {
		return true
	}
	if strings.ContainsAny(baseCommand, "-_") {
		return true
	}

	return false
}

func (inst *Instance) setupCmdPipes(newCommand *Command, cmd *exec.Cmd) (io.Reader, io.Reader, error) {
	log.Debug().Str("Command ID", newCommand.ID).Msg("Setting up command pipes")
	stdOutPipeReader, stdOutPipeWriter, errStdOutPipe := os.Pipe()
	if errStdOutPipe != nil {
		log.Error().Err(errStdOutPipe).Msg("Unable to get StdOutPipe")
		return nil, nil, fmt.Errorf("create stdout pipe: %w", errStdOutPipe)
	}

	stdErrPipeReader, stdErrPipeWriter, errStdErrPipe := os.Pipe()
	if errStdErrPipe != nil {
		log.Error().Err(errStdErrPipe).Msg("Unable to get StdErrPipe")
		return nil, nil, errors.Join(
			fmt.Errorf("create stderr pipe: %w", errStdErrPipe),
			wrapSupervisorError("close stdout pipe reader", stdOutPipeReader.Close()),
			wrapSupervisorError("close stdout pipe writer", stdOutPipeWriter.Close()),
		)
	}
	cmd.Stdout = stdOutPipeWriter
	cmd.Stderr = stdErrPipeWriter
	return stdOutPipeReader, stdErrPipeReader, nil
}
