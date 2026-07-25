//go:build windows

package appservice

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/ClintonCollins/Xylona/internal/selfupdate"
)

const (
	windowsServiceOperationTimeout = 30 * time.Second
	windowsServiceStopWaitHint     = 10 * time.Second
)

// RequireWindowsServiceManagerAccess verifies that the current process can
// open the Service Control Manager with the access needed to install a service.
func RequireWindowsServiceManagerAccess() error {
	manager, errConnect := mgr.Connect()
	if errConnect != nil {
		return fmt.Errorf(
			"connect to Windows Service Control Manager (run this command as Administrator): %w",
			errConnect,
		)
	}
	errDisconnect := manager.Disconnect()
	if errDisconnect != nil {
		return fmt.Errorf("disconnect from Windows Service Control Manager: %w", errDisconnect)
	}
	return nil
}

func platformInstall(ctx context.Context, definition Definition, options InstallOptions) (result InstallResult, resultErr error) {
	resolvedDefinition, errDefinition := resolveWindowsDefinition(definition)
	if errDefinition != nil {
		return InstallResult{}, errDefinition
	}

	manager, errConnect := mgr.Connect()
	if errConnect != nil {
		return InstallResult{}, fmt.Errorf("connect to Windows Service Control Manager (run this command as Administrator): %w", errConnect)
	}
	defer func() {
		errDisconnect := manager.Disconnect()
		if errDisconnect != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("disconnect from Windows Service Control Manager: %w", errDisconnect))
		}
	}()

	service, errCreate := manager.CreateService(
		resolvedDefinition.Name,
		resolvedDefinition.ExecutablePath,
		mgr.Config{
			StartType:    mgr.StartAutomatic,
			ErrorControl: mgr.ErrorNormal,
			DisplayName:  resolvedDefinition.DisplayName,
			Description:  resolvedDefinition.Description,
		},
		resolvedDefinition.Arguments...,
	)
	if errCreate != nil {
		return InstallResult{}, fmt.Errorf(
			"install Windows service %s (run this command as Administrator): %w",
			resolvedDefinition.Name,
			errCreate,
		)
	}
	defer func() {
		errClose := service.Close()
		if errClose != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close Windows service %s: %w", resolvedDefinition.Name, errClose))
		}
	}()

	eventLogSourceInstalled, errEventLogSource := installWindowsEventLogSource(resolvedDefinition.Name)
	if errEventLogSource != nil {
		errRollback := rollbackWindowsServiceInstallation(service, resolvedDefinition.Name, false)
		return InstallResult{}, errors.Join(
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
		errRollback := rollbackWindowsServiceInstallation(service, resolvedDefinition.Name, eventLogSourceInstalled)
		return InstallResult{}, errors.Join(
			fmt.Errorf("configure Windows service recovery actions: %w", errRecoveryActions),
			errRollback,
		)
	}

	errRecoveryFlag := service.SetRecoveryActionsOnNonCrashFailures(true)
	if errRecoveryFlag != nil {
		errRollback := rollbackWindowsServiceInstallation(service, resolvedDefinition.Name, eventLogSourceInstalled)
		return InstallResult{}, errors.Join(
			fmt.Errorf("configure Windows service non-crash recovery: %w", errRecoveryFlag),
			errRollback,
		)
	}

	result = InstallResult{
		ExecutablePath: resolvedDefinition.ExecutablePath,
		User:           "LocalSystem",
		Warning:        "The service runs as LocalSystem; restrict access to the executable, configuration, identity, database, and managed server directories.",
	}
	if options.Start {
		errStart := startOpenedWindowsService(ctx, service, resolvedDefinition.Name)
		if errStart != nil {
			return result, fmt.Errorf("windows service was installed but failed to start: %w", errStart)
		}
	}
	return result, nil
}

func platformStart(ctx context.Context, definition Definition) (resultErr error) {
	resolvedDefinition, errDefinition := resolveWindowsDefinition(definition)
	if errDefinition != nil {
		return errDefinition
	}
	manager, service, errOpen := openWindowsService(resolvedDefinition.Name)
	if errOpen != nil {
		return errOpen
	}
	defer func() {
		resultErr = closeWindowsServiceHandles(manager, service, resolvedDefinition.Name, resultErr)
	}()
	return startOpenedWindowsService(ctx, service, resolvedDefinition.Name)
}

func platformStop(ctx context.Context, definition Definition) (resultErr error) {
	resolvedDefinition, errDefinition := resolveWindowsDefinition(definition)
	if errDefinition != nil {
		return errDefinition
	}
	manager, service, errOpen := openWindowsService(resolvedDefinition.Name)
	if errOpen != nil {
		return errOpen
	}
	defer func() {
		resultErr = closeWindowsServiceHandles(manager, service, resolvedDefinition.Name, resultErr)
	}()
	return stopOpenedWindowsService(ctx, service, resolvedDefinition.Name)
}

func platformStatus(_ context.Context, definition Definition) (state string, resultErr error) {
	resolvedDefinition, errDefinition := resolveWindowsDefinition(definition)
	if errDefinition != nil {
		return "", errDefinition
	}
	manager, service, errOpen := openWindowsService(resolvedDefinition.Name)
	if errOpen != nil {
		return "", errOpen
	}
	defer func() {
		resultErr = closeWindowsServiceHandles(manager, service, resolvedDefinition.Name, resultErr)
	}()

	serviceStatus, errQuery := service.Query()
	if errQuery != nil {
		return "", fmt.Errorf("query Windows service %s: %w", resolvedDefinition.Name, errQuery)
	}
	return windowsServiceStateName(serviceStatus.State), nil
}

func platformUninstall(ctx context.Context, definition Definition) (resultErr error) {
	resolvedDefinition, errDefinition := resolveWindowsDefinition(definition)
	if errDefinition != nil {
		return errDefinition
	}
	manager, service, errOpen := openWindowsService(resolvedDefinition.Name)
	if errOpen != nil {
		return errOpen
	}
	defer func() {
		resultErr = closeWindowsServiceHandles(manager, service, resolvedDefinition.Name, resultErr)
	}()

	errStop := stopOpenedWindowsService(ctx, service, resolvedDefinition.Name)
	if errStop != nil {
		return fmt.Errorf("stop Windows service before uninstall: %w", errStop)
	}
	errDelete := service.Delete()
	if errDelete != nil {
		return fmt.Errorf(
			"uninstall Windows service %s (run this command as Administrator): %w",
			resolvedDefinition.Name,
			errDelete,
		)
	}
	errRemoveEventLogSource := eventlog.Remove(resolvedDefinition.Name)
	if errRemoveEventLogSource != nil && !errors.Is(errRemoveEventLogSource, registry.ErrNotExist) {
		return fmt.Errorf("windows service was removed, but its event log source could not be removed: %w", errRemoveEventLogSource)
	}
	return nil
}

func platformRun(definition Definition, runApplication RunFunc, configureLogs LogConfigurator) (resultErr error) {
	resolvedDefinition, errDefinition := resolveWindowsDefinition(definition)
	if errDefinition != nil {
		return errDefinition
	}
	if runApplication == nil {
		return errors.New("windows service application runner is required")
	}
	errWorkingDirectory := os.Chdir(resolvedDefinition.WorkingDirectory)
	if errWorkingDirectory != nil {
		return fmt.Errorf("set Windows service working directory: %w", errWorkingDirectory)
	}
	errRestartMode := os.Setenv(selfupdate.RestartModeEnvironment, string(selfupdate.RestartModeWindowsService))
	if errRestartMode != nil {
		return fmt.Errorf("configure Windows service update restart mode: %w", errRestartMode)
	}
	errServiceName := os.Setenv(selfupdate.ServiceNameEnvironment, resolvedDefinition.Name)
	if errServiceName != nil {
		return fmt.Errorf("configure Windows service update service name: %w", errServiceName)
	}

	eventLogger, errEventLogger := eventlog.Open(resolvedDefinition.Name)
	if errEventLogger != nil {
		return fmt.Errorf("open Windows event log source %s: %w", resolvedDefinition.Name, errEventLogger)
	}
	eventLogWriter := &windowsEventLogWriter{logger: eventLogger}
	cleanupLogs := func() {}
	if configureLogs == nil {
		log.Logger = zerolog.New(eventLogWriter).With().Timestamp().Logger()
	} else {
		configuredCleanup, errConfigure := configureLogs(eventLogWriter)
		if errConfigure != nil {
			errClose := eventLogger.Close()
			return errors.Join(errConfigure, errClose)
		}
		if configuredCleanup != nil {
			cleanupLogs = configuredCleanup
		}
	}
	defer func() {
		cleanupLogs()
		errClose := eventLogger.Close()
		if errClose != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close Windows event log source %s: %w", resolvedDefinition.Name, errClose))
		}
	}()

	errRun := svc.Run(resolvedDefinition.Name, &windowsServiceHandler{run: runApplication})
	if errRun != nil {
		errService := fmt.Errorf("run Windows service %s: %w", resolvedDefinition.Name, errRun)
		log.Error().Err(errService).Msg("Windows service host failed")
		return errService
	}
	return nil
}

func resolveWindowsDefinition(definition Definition) (Definition, error) {
	resolvedDefinition, errDefinition := resolveDefinition(definition)
	if errDefinition != nil {
		return Definition{}, errDefinition
	}
	if strings.ContainsAny(resolvedDefinition.Name, `/\`+"\r\n\x00") {
		return Definition{}, fmt.Errorf("invalid Windows service name %q", resolvedDefinition.Name)
	}
	return resolvedDefinition, nil
}

func rollbackWindowsServiceInstallation(service *mgr.Service, serviceName string, removeEventLogSource bool) error {
	errDelete := service.Delete()
	var errRemoveEventLogSource error
	if removeEventLogSource {
		errRemoveEventLogSource = eventlog.Remove(serviceName)
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

func installWindowsEventLogSource(serviceName string) (bool, error) {
	sourceExists, errSourceExists := windowsEventLogSourceExists(serviceName)
	if errSourceExists != nil {
		return false, errSourceExists
	}
	if sourceExists {
		return false, nil
	}

	errInstall := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if errInstall == nil {
		return true, nil
	}

	errRemove := eventlog.Remove(serviceName)
	if errors.Is(errRemove, registry.ErrNotExist) {
		errRemove = nil
	}
	return false, errors.Join(
		fmt.Errorf("register Windows event log source %s: %w", serviceName, errInstall),
		wrapWindowsServiceRollbackError("remove partial event log source", errRemove),
	)
}

func windowsEventLogSourceExists(serviceName string) (exists bool, resultErr error) {
	registryPath := `SYSTEM\CurrentControlSet\Services\EventLog\Application\` + serviceName
	key, errOpen := registry.OpenKey(registry.LOCAL_MACHINE, registryPath, registry.QUERY_VALUE)
	if errors.Is(errOpen, registry.ErrNotExist) {
		return false, nil
	}
	if errOpen != nil {
		return false, fmt.Errorf("check Windows event log source %s: %w", serviceName, errOpen)
	}
	defer func() {
		errClose := key.Close()
		if errClose != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close Windows event log registry key: %w", errClose))
		}
	}()
	return true, nil
}

func startOpenedWindowsService(ctx context.Context, service *mgr.Service, serviceName string) error {
	serviceStatus, errQuery := service.Query()
	if errQuery != nil {
		return fmt.Errorf("query Windows service %s before start: %w", serviceName, errQuery)
	}
	if serviceStatus.State == svc.Running {
		return nil
	}
	if serviceStatus.State == svc.StartPending {
		return waitForWindowsServiceState(ctx, service, serviceName, svc.Running)
	}
	if serviceStatus.State == svc.StopPending {
		errWaitStopped := waitForWindowsServiceState(ctx, service, serviceName, svc.Stopped)
		if errWaitStopped != nil {
			return errWaitStopped
		}
	}

	errStart := service.Start()
	if errStart != nil {
		return fmt.Errorf("start Windows service %s: %w", serviceName, errStart)
	}
	return waitForWindowsServiceState(ctx, service, serviceName, svc.Running)
}

func stopOpenedWindowsService(ctx context.Context, service *mgr.Service, serviceName string) error {
	serviceStatus, errQuery := service.Query()
	if errQuery != nil {
		return fmt.Errorf("query Windows service %s before stop: %w", serviceName, errQuery)
	}
	if serviceStatus.State == svc.Stopped {
		return nil
	}
	if serviceStatus.State == svc.StopPending {
		return waitForWindowsServiceState(ctx, service, serviceName, svc.Stopped)
	}
	if serviceStatus.State == svc.StartPending {
		errWaitRunning := waitForWindowsServiceState(ctx, service, serviceName, svc.Running)
		if errWaitRunning != nil {
			return errWaitRunning
		}
	}

	_, errControl := service.Control(svc.Stop)
	if errControl != nil {
		return fmt.Errorf("request graceful stop for Windows service %s: %w", serviceName, errControl)
	}
	return waitForWindowsServiceState(ctx, service, serviceName, svc.Stopped)
}

func openWindowsService(serviceName string) (*mgr.Mgr, *mgr.Service, error) {
	manager, errConnect := mgr.Connect()
	if errConnect != nil {
		return nil, nil, fmt.Errorf("connect to Windows Service Control Manager (run this command as Administrator): %w", errConnect)
	}

	service, errOpen := manager.OpenService(serviceName)
	if errOpen != nil {
		errDisconnect := manager.Disconnect()
		return nil, nil, errors.Join(
			fmt.Errorf("open Windows service %s: %w", serviceName, errOpen),
			windowsServiceManagerDisconnectError(errDisconnect),
		)
	}
	return manager, service, nil
}

func closeWindowsServiceHandles(
	manager *mgr.Mgr,
	service *mgr.Service,
	serviceName string,
	operationError error,
) error {
	errClose := service.Close()
	if errClose != nil {
		operationError = errors.Join(operationError, fmt.Errorf("close Windows service %s: %w", serviceName, errClose))
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

func waitForWindowsServiceState(
	ctx context.Context,
	service *mgr.Service,
	serviceName string,
	targetState svc.State,
) error {
	waitContext, waitCancel := context.WithTimeout(ctx, windowsServiceOperationTimeout)
	defer waitCancel()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		serviceStatus, errQuery := service.Query()
		if errQuery != nil {
			return fmt.Errorf(
				"query Windows service %s while waiting for %s: %w",
				serviceName,
				windowsServiceStateName(targetState),
				errQuery,
			)
		}
		if serviceStatus.State == targetState {
			return nil
		}
		if targetState == svc.Running && serviceStatus.State == svc.Stopped {
			return fmt.Errorf("windows service %s stopped before reaching running state", serviceName)
		}

		select {
		case <-waitContext.Done():
			return fmt.Errorf(
				"wait for Windows service %s to become %s: %w",
				serviceName,
				windowsServiceStateName(targetState),
				waitContext.Err(),
			)
		case <-ticker.C:
		}
	}
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
	run RunFunc
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
			if stopRequested || serviceExitCode == UpdateHandoffExitCode {
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
