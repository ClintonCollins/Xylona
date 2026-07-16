//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/ClintonCollins/Xylona/internal/selfupdate"
)

const (
	windowsServiceName             = "Xylona"
	windowsServiceDisplayName      = "Xylona"
	windowsServiceDescription      = "Xylona game server control panel"
	windowsServiceOperationTimeout = 30 * time.Second
	windowsServiceStopWaitHint     = 10 * time.Second
	windowsEventLogRegistryPath    = `SYSTEM\CurrentControlSet\Services\EventLog\Application\Xylona`
)

var runWindowsServiceApplication = runServiceUntil

func platformCommands() []*cli.Command {
	return []*cli.Command{newWindowsServiceCommand()}
}

func newWindowsServiceCommand() *cli.Command {
	return &cli.Command{
		Name:      "service",
		Usage:     "Install and manage Xylona as a Windows service",
		UsageText: "xylona service <install|start|stop|status|uninstall>",
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
		Commands: []*cli.Command{
			{
				Name:  "install",
				Usage: "Install Xylona as an automatic Windows service",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "start",
						Usage: "Start the service after installing it",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					executablePath, errInstall := installWindowsService(ctx, cmd.Bool("start"))
					if errInstall != nil {
						return errInstall
					}

					errOutput := writeWindowsServiceOutput("Installed Windows service %s using %s.", windowsServiceName, executablePath)
					if errOutput != nil {
						return errOutput
					}
					errOutput = writeWindowsServiceOutput("The service runs as LocalSystem; restrict access to the Xylona executable, .env, database, and managed server directories.")
					if errOutput != nil {
						return errOutput
					}
					if !cmd.Bool("start") {
						return writeWindowsServiceOutput("Start it with: xylona service start")
					}
					return nil
				},
			},
			{
				Name:  "start",
				Usage: "Start the installed Xylona service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStart := startWindowsService(ctx)
					if errStart != nil {
						return errStart
					}
					return writeWindowsServiceOutput("Windows service %s is running.", windowsServiceName)
				},
			},
			{
				Name:  "stop",
				Usage: "Gracefully stop the installed Xylona service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStop := stopWindowsService(ctx)
					if errStop != nil {
						return errStop
					}
					return writeWindowsServiceOutput("Windows service %s is stopped.", windowsServiceName)
				},
			},
			{
				Name:  "status",
				Usage: "Show the installed Xylona service status",
				Action: func(_ context.Context, _ *cli.Command) error {
					state, errStatus := queryWindowsServiceState()
					if errStatus != nil {
						return errStatus
					}
					return writeWindowsServiceOutput("Windows service %s is %s.", windowsServiceName, windowsServiceStateName(state))
				},
			},
			{
				Name:  "uninstall",
				Usage: "Stop and uninstall the Xylona service without deleting Xylona data",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errUninstall := uninstallWindowsService(ctx)
					if errUninstall != nil {
						return errUninstall
					}
					return writeWindowsServiceOutput("Uninstalled Windows service %s. Xylona files and data were left unchanged.", windowsServiceName)
				},
			},
			{
				Name:   "run",
				Usage:  "Run Xylona under the Windows Service Control Manager",
				Hidden: true,
				Action: func(_ context.Context, _ *cli.Command) error {
					return runAsWindowsService()
				},
			},
		},
	}
}

func writeWindowsServiceOutput(format string, values ...any) error {
	_, errWrite := fmt.Fprintf(rootCLIStdout, format+"\n", values...)
	if errWrite != nil {
		return fmt.Errorf("write Windows service command output: %w", errWrite)
	}
	return nil
}

func installWindowsService(ctx context.Context, start bool) (executablePath string, errReturn error) {
	executablePath, errExecutablePath := resolveWindowsServiceExecutablePath()
	if errExecutablePath != nil {
		return "", errExecutablePath
	}

	manager, errConnect := mgr.Connect()
	if errConnect != nil {
		return "", fmt.Errorf("connect to Windows Service Control Manager (run this command as Administrator): %w", errConnect)
	}
	defer func() {
		errDisconnect := manager.Disconnect()
		if errDisconnect != nil {
			errReturn = errors.Join(errReturn, fmt.Errorf("disconnect from Windows Service Control Manager: %w", errDisconnect))
		}
	}()

	service, errCreate := manager.CreateService(
		windowsServiceName,
		executablePath,
		mgr.Config{
			StartType:    mgr.StartAutomatic,
			ErrorControl: mgr.ErrorNormal,
			DisplayName:  windowsServiceDisplayName,
			Description:  windowsServiceDescription,
		},
		"service",
		"run",
	)
	if errCreate != nil {
		return "", fmt.Errorf("install Windows service %s (run this command as Administrator): %w", windowsServiceName, errCreate)
	}
	defer func() {
		errClose := service.Close()
		if errClose != nil {
			errReturn = errors.Join(errReturn, fmt.Errorf("close Windows service %s: %w", windowsServiceName, errClose))
		}
	}()

	eventLogSourceInstalled, errEventLogSource := installWindowsEventLogSource()
	if errEventLogSource != nil {
		errRollback := rollbackWindowsServiceInstallation(service, false)
		return "", errors.Join(
			fmt.Errorf("install Windows event log source: %w", errEventLogSource),
			errRollback,
		)
	}

	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: time.Minute},
	}
	errRecoveryActions := service.SetRecoveryActions(recoveryActions, uint32((24*time.Hour)/time.Second))
	if errRecoveryActions != nil {
		errRollback := rollbackWindowsServiceInstallation(service, eventLogSourceInstalled)
		return "", errors.Join(
			fmt.Errorf("configure Windows service recovery actions: %w", errRecoveryActions),
			errRollback,
		)
	}

	errRecoveryFlag := service.SetRecoveryActionsOnNonCrashFailures(true)
	if errRecoveryFlag != nil {
		errRollback := rollbackWindowsServiceInstallation(service, eventLogSourceInstalled)
		return "", errors.Join(
			fmt.Errorf("configure Windows service non-crash recovery: %w", errRecoveryFlag),
			errRollback,
		)
	}

	if start {
		errStart := startOpenedWindowsService(ctx, service)
		if errStart != nil {
			return "", fmt.Errorf("start newly installed Windows service: %w", errStart)
		}
	}

	return executablePath, nil
}

func rollbackWindowsServiceInstallation(service *mgr.Service, removeEventLogSource bool) error {
	errDelete := service.Delete()
	var errRemoveEventLogSource error
	if removeEventLogSource {
		errRemoveEventLogSource = eventlog.Remove(windowsServiceName)
	}
	return errors.Join(
		wrapWindowsServiceRollbackError("delete service", errDelete),
		wrapWindowsServiceRollbackError("remove event log source", errRemoveEventLogSource),
	)
}

func wrapWindowsServiceRollbackError(operation string, errRollback error) error {
	if errRollback == nil {
		return nil
	}
	return fmt.Errorf("roll back incomplete Windows service installation (%s): %w", operation, errRollback)
}

func installWindowsEventLogSource() (bool, error) {
	sourceExists, errSourceExists := windowsEventLogSourceExists()
	if errSourceExists != nil {
		return false, errSourceExists
	}
	if sourceExists {
		return false, nil
	}

	errInstall := eventlog.InstallAsEventCreate(windowsServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if errInstall == nil {
		return true, nil
	}

	errRemove := eventlog.Remove(windowsServiceName)
	if errors.Is(errRemove, registry.ErrNotExist) {
		errRemove = nil
	}
	return false, errors.Join(
		fmt.Errorf("register Windows event log source %s: %w", windowsServiceName, errInstall),
		wrapWindowsServiceRollbackError("remove partial event log source", errRemove),
	)
}

func windowsEventLogSourceExists() (exists bool, errReturn error) {
	key, errOpen := registry.OpenKey(registry.LOCAL_MACHINE, windowsEventLogRegistryPath, registry.QUERY_VALUE)
	if errors.Is(errOpen, registry.ErrNotExist) {
		return false, nil
	}
	if errOpen != nil {
		return false, fmt.Errorf("check Windows event log source %s: %w", windowsServiceName, errOpen)
	}
	defer func() {
		errClose := key.Close()
		if errClose != nil {
			errReturn = errors.Join(errReturn, fmt.Errorf("close Windows event log registry key: %w", errClose))
		}
	}()
	return true, nil
}

func startWindowsService(ctx context.Context) (errReturn error) {
	manager, service, errOpen := openWindowsService()
	if errOpen != nil {
		return errOpen
	}
	defer func() {
		errReturn = closeWindowsServiceHandles(manager, service, errReturn)
	}()

	return startOpenedWindowsService(ctx, service)
}

func startOpenedWindowsService(ctx context.Context, service *mgr.Service) error {
	status, errQuery := service.Query()
	if errQuery != nil {
		return fmt.Errorf("query Windows service %s before start: %w", windowsServiceName, errQuery)
	}
	if status.State == svc.Running {
		return nil
	}
	if status.State == svc.StartPending {
		return waitForWindowsServiceState(ctx, service, svc.Running)
	}
	if status.State == svc.StopPending {
		errWaitStopped := waitForWindowsServiceState(ctx, service, svc.Stopped)
		if errWaitStopped != nil {
			return errWaitStopped
		}
	}

	errStart := service.Start()
	if errStart != nil {
		return fmt.Errorf("start Windows service %s: %w", windowsServiceName, errStart)
	}
	return waitForWindowsServiceState(ctx, service, svc.Running)
}

func stopWindowsService(ctx context.Context) (errReturn error) {
	manager, service, errOpen := openWindowsService()
	if errOpen != nil {
		return errOpen
	}
	defer func() {
		errReturn = closeWindowsServiceHandles(manager, service, errReturn)
	}()

	return stopOpenedWindowsService(ctx, service)
}

func stopOpenedWindowsService(ctx context.Context, service *mgr.Service) error {
	status, errQuery := service.Query()
	if errQuery != nil {
		return fmt.Errorf("query Windows service %s before stop: %w", windowsServiceName, errQuery)
	}
	if status.State == svc.Stopped {
		return nil
	}
	if status.State == svc.StopPending {
		return waitForWindowsServiceState(ctx, service, svc.Stopped)
	}
	if status.State == svc.StartPending {
		errWaitRunning := waitForWindowsServiceState(ctx, service, svc.Running)
		if errWaitRunning != nil {
			return errWaitRunning
		}
	}

	_, errControl := service.Control(svc.Stop)
	if errControl != nil {
		return fmt.Errorf("request graceful stop for Windows service %s: %w", windowsServiceName, errControl)
	}
	return waitForWindowsServiceState(ctx, service, svc.Stopped)
}

func queryWindowsServiceState() (state svc.State, errReturn error) {
	manager, service, errOpen := openWindowsService()
	if errOpen != nil {
		return svc.Stopped, errOpen
	}
	defer func() {
		errReturn = closeWindowsServiceHandles(manager, service, errReturn)
	}()

	status, errQuery := service.Query()
	if errQuery != nil {
		return svc.Stopped, fmt.Errorf("query Windows service %s: %w", windowsServiceName, errQuery)
	}
	return status.State, nil
}

func uninstallWindowsService(ctx context.Context) (errReturn error) {
	manager, service, errOpen := openWindowsService()
	if errOpen != nil {
		return errOpen
	}
	defer func() {
		errReturn = closeWindowsServiceHandles(manager, service, errReturn)
	}()

	errStop := stopOpenedWindowsService(ctx, service)
	if errStop != nil {
		return fmt.Errorf("stop Windows service before uninstall: %w", errStop)
	}
	errDelete := service.Delete()
	if errDelete != nil {
		return fmt.Errorf("uninstall Windows service %s (run this command as Administrator): %w", windowsServiceName, errDelete)
	}
	errRemoveEventLogSource := eventlog.Remove(windowsServiceName)
	if errRemoveEventLogSource != nil && !errors.Is(errRemoveEventLogSource, registry.ErrNotExist) {
		return fmt.Errorf("windows service was removed, but its event log source could not be removed: %w", errRemoveEventLogSource)
	}
	return nil
}

func openWindowsService() (*mgr.Mgr, *mgr.Service, error) {
	manager, errConnect := mgr.Connect()
	if errConnect != nil {
		return nil, nil, fmt.Errorf("connect to Windows Service Control Manager (run this command as Administrator): %w", errConnect)
	}

	service, errOpen := manager.OpenService(windowsServiceName)
	if errOpen != nil {
		errDisconnect := manager.Disconnect()
		return nil, nil, errors.Join(
			fmt.Errorf("open Windows service %s: %w", windowsServiceName, errOpen),
			windowsServiceManagerDisconnectError(errDisconnect),
		)
	}
	return manager, service, nil
}

func closeWindowsServiceHandles(manager *mgr.Mgr, service *mgr.Service, operationError error) error {
	errClose := service.Close()
	if errClose != nil {
		operationError = errors.Join(operationError, fmt.Errorf("close Windows service %s: %w", windowsServiceName, errClose))
	}
	errDisconnect := manager.Disconnect()
	if errDisconnect != nil {
		operationError = errors.Join(operationError, windowsServiceManagerDisconnectError(errDisconnect))
	}
	return operationError
}

func windowsServiceManagerDisconnectError(errDisconnect error) error {
	if errDisconnect == nil {
		return nil
	}
	return fmt.Errorf("disconnect from Windows Service Control Manager: %w", errDisconnect)
}

func waitForWindowsServiceState(ctx context.Context, service *mgr.Service, targetState svc.State) error {
	waitContext, waitCancel := context.WithTimeout(ctx, windowsServiceOperationTimeout)
	defer waitCancel()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		status, errQuery := service.Query()
		if errQuery != nil {
			return fmt.Errorf("query Windows service %s while waiting for %s: %w", windowsServiceName, windowsServiceStateName(targetState), errQuery)
		}
		if status.State == targetState {
			return nil
		}
		if targetState == svc.Running && status.State == svc.Stopped {
			return fmt.Errorf("windows service %s stopped before reaching running state", windowsServiceName)
		}

		select {
		case <-waitContext.Done():
			return fmt.Errorf("wait for Windows service %s to become %s: %w", windowsServiceName, windowsServiceStateName(targetState), waitContext.Err())
		case <-ticker.C:
		}
	}
}

func resolveWindowsServiceExecutablePath() (string, error) {
	executablePath, errExecutable := os.Executable()
	if errExecutable != nil {
		return "", fmt.Errorf("resolve Xylona executable for Windows service installation: %w", errExecutable)
	}
	absolutePath, errAbsolute := filepath.Abs(executablePath)
	if errAbsolute != nil {
		return "", fmt.Errorf("resolve absolute Xylona executable path for Windows service installation: %w", errAbsolute)
	}
	return filepath.Clean(absolutePath), nil
}

func runAsWindowsService() (errReturn error) {
	executablePath, errExecutable := resolveWindowsServiceExecutablePath()
	if errExecutable != nil {
		return errExecutable
	}
	errWorkingDirectory := os.Chdir(filepath.Dir(executablePath))
	if errWorkingDirectory != nil {
		return fmt.Errorf("set Windows service working directory: %w", errWorkingDirectory)
	}
	errRestartMode := os.Setenv(selfupdate.RestartModeEnvironment, string(selfupdate.RestartModeServiceManager))
	if errRestartMode != nil {
		return fmt.Errorf("configure Windows service update restart mode: %w", errRestartMode)
	}
	eventLogger, errEventLogger := eventlog.Open(windowsServiceName)
	if errEventLogger != nil {
		return fmt.Errorf("open Windows event log source %s: %w", windowsServiceName, errEventLogger)
	}
	eventLogWriter := &windowsEventLogWriter{logger: eventLogger}
	runtimeLogWriterOverride = eventLogWriter
	log.Logger = zerolog.New(eventLogWriter).With().Timestamp().Logger()
	defer func() {
		runtimeLogWriterOverride = nil
		errClose := eventLogger.Close()
		if errClose != nil {
			errReturn = errors.Join(errReturn, fmt.Errorf("close Windows event log source %s: %w", windowsServiceName, errClose))
		}
	}()

	errRun := svc.Run(windowsServiceName, &windowsServiceHandler{run: runWindowsServiceApplication})
	if errRun != nil {
		errService := fmt.Errorf("run Windows service %s: %w", windowsServiceName, errRun)
		log.Error().Err(errService).Msg("Windows service host failed")
		return errService
	}
	return nil
}

type windowsEventLogWriter struct {
	logger *eventlog.Log
}

func (w *windowsEventLogWriter) Write(message []byte) (int, error) {
	return w.WriteLevel(zerolog.InfoLevel, message)
}

func (w *windowsEventLogWriter) WriteLevel(level zerolog.Level, message []byte) (int, error) {
	trimmedMessage := strings.TrimSpace(string(message))
	var errWrite error
	switch level {
	case zerolog.WarnLevel:
		errWrite = w.logger.Warning(1, trimmedMessage)
	case zerolog.ErrorLevel, zerolog.FatalLevel, zerolog.PanicLevel:
		errWrite = w.logger.Error(1, trimmedMessage)
	default:
		errWrite = w.logger.Info(1, trimmedMessage)
	}
	if errWrite != nil {
		return 0, fmt.Errorf("write Windows event log entry: %w", errWrite)
	}
	return len(message), nil
}

type windowsServiceHandler struct {
	run func(<-chan os.Signal) int
}

func (h *windowsServiceHandler) Execute(
	_ []string,
	requests <-chan svc.ChangeRequest,
	changes chan<- svc.Status,
) (serviceSpecificExitCode bool, exitCode uint32) {
	changes <- svc.Status{
		State:    svc.StartPending,
		WaitHint: uint32((10 * time.Second) / time.Millisecond),
	}

	shutdownSignalChannel := make(chan os.Signal, 1)
	serviceDone := make(chan int, 1)
	go func() {
		serviceDone <- h.run(shutdownSignalChannel)
	}()

	currentStatus := svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}
	changes <- currentStatus

	var stopProgressTicker *time.Ticker
	var stopProgress <-chan time.Time
	stopRequested := false
	checkpoint := uint32(1)

	for {
		select {
		case serviceExitCode := <-serviceDone:
			if stopProgressTicker != nil {
				stopProgressTicker.Stop()
			}
			if stopRequested {
				return false, 0
			}
			return true, normalizeWindowsServiceExitCode(serviceExitCode)
		case request, requestsOpen := <-requests:
			if !requestsOpen {
				requests = nil
				continue
			}
			switch request.Cmd {
			case svc.Interrogate:
				changes <- currentStatus
			case svc.Stop, svc.Shutdown:
				if stopRequested {
					continue
				}
				stopRequested = true
				currentStatus = svc.Status{
					State:      svc.StopPending,
					CheckPoint: checkpoint,
					WaitHint:   uint32(windowsServiceStopWaitHint / time.Millisecond),
				}
				changes <- currentStatus
				shutdownSignalChannel <- os.Interrupt
				stopProgressTicker = time.NewTicker(2 * time.Second)
				stopProgress = stopProgressTicker.C
			}
		case <-stopProgress:
			checkpoint++
			currentStatus.CheckPoint = checkpoint
			changes <- currentStatus
		}
	}
}

func normalizeWindowsServiceExitCode(exitCode int) uint32 {
	if exitCode <= 0 || uint64(exitCode) > math.MaxUint32 {
		return 1
	}
	return uint32(exitCode)
}

func windowsServiceStateName(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "starting"
	case svc.StopPending:
		return "stopping"
	case svc.Running:
		return "running"
	case svc.ContinuePending:
		return "resuming"
	case svc.PausePending:
		return "pausing"
	case svc.Paused:
		return "paused"
	default:
		return fmt.Sprintf("unknown (%d)", state)
	}
}
