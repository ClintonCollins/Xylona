package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (inst *Instance) initNewCommand(preparedCommand PreparedCommand, persistentCommand *Command) *Command {
	var newCommand *Command
	processCtx, processCtxCancel := context.WithCancel(inst.ctx)
	gameServerName := preparedCommand.GameServerName
	if gameServerName == "" && preparedCommand.InternalGameServer != nil {
		gameServerName = preparedCommand.InternalGameServer.Name
	}
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
		newCommand.gameServerName = gameServerName
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
			gameServerName:      gameServerName,
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

	errValidateTelnet := validateTelnetInputMethod(newCommand.inputMethod)
	if errValidateTelnet != nil {
		log.Error().Err(errValidateTelnet).Str("Command ID", preparedCommand.ID).Msg("Invalid telnet input method")
		return nil, errValidateTelnet
	}

	cmd := exec.CommandContext( //nolint:gosec // Supervisor intentionally launches configured game server commands.
		newCommand.processCtx,
		resolveServerLocalBaseCommand(baseCommand, preparedCommand.WorkingDirectory),
		preparedCommand.Args...,
	)
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
