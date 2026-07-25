//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	windowsServiceRestartTimeout = 30 * time.Second
	windowsServiceRestartPoll    = 250 * time.Millisecond
)

type windowsServiceRestarter interface {
	Query() (svc.Status, error)
	Control(svc.Cmd) (svc.Status, error)
	Start(args ...string) error
}

func restartWindowsService(serviceName string) (resultErr error) {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return errors.New("windows service name is required")
	}
	manager, errConnect := mgr.Connect()
	if errConnect != nil {
		return unsafeServiceRestartError(
			fmt.Errorf("connect to Windows Service Control Manager: %w", errConnect),
		)
	}
	defer func() {
		errDisconnect := manager.Disconnect()
		if errDisconnect == nil {
			return
		}
		errDisconnect = fmt.Errorf("disconnect from Windows Service Control Manager: %w", errDisconnect)
		if resultErr == nil {
			log.Warn().Err(errDisconnect).Msg("selfupdate: updated Windows service restarted with incomplete handle cleanup")
			return
		}
		resultErr = errors.Join(resultErr, errDisconnect)
	}()

	service, errOpen := manager.OpenService(serviceName)
	if errOpen != nil {
		return unsafeServiceRestartError(
			fmt.Errorf("open Windows service %s: %w", serviceName, errOpen),
		)
	}
	defer func() {
		errClose := service.Close()
		if errClose == nil {
			return
		}
		errClose = fmt.Errorf("close Windows service %s: %w", serviceName, errClose)
		if resultErr == nil {
			log.Warn().Err(errClose).Msg("selfupdate: updated Windows service restarted with incomplete handle cleanup")
			return
		}
		resultErr = errors.Join(resultErr, errClose)
	}()

	deadline := time.Now().Add(windowsServiceRestartTimeout)
	return restartOpenedWindowsService(service, serviceName, deadline)
}

func restartOpenedWindowsService(
	service windowsServiceRestarter,
	serviceName string,
	deadline time.Time,
) error {
	errStop := stopWindowsServiceForRestart(service, serviceName, deadline)
	if errStop != nil {
		return unsafeServiceRestartError(errStop)
	}

	errStart := service.Start()
	if errStart != nil && !errors.Is(errStart, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		errStart = fmt.Errorf("start Windows service %s: %w", serviceName, errStart)
		return classifyWindowsServiceRestartError(service, errStart)
	}
	errWait := waitForRestartedWindowsService(service, serviceName, deadline)
	if errWait != nil {
		return classifyWindowsServiceRestartError(service, errWait)
	}
	return nil
}

func classifyWindowsServiceRestartError(service windowsServiceRestarter, restartError error) error {
	status, errQuery := service.Query()
	if errQuery != nil || status.State != svc.Stopped {
		return unsafeServiceRestartError(restartError)
	}
	return restartError
}

func unsafeServiceRestartError(restartError error) error {
	return errors.Join(errServiceRestartRollbackUnsafe, restartError)
}

func stopWindowsServiceForRestart(
	service windowsServiceRestarter,
	serviceName string,
	deadline time.Time,
) error {
	for {
		status, errQuery := service.Query()
		if errQuery != nil {
			return fmt.Errorf("query Windows service %s before restart: %w", serviceName, errQuery)
		}
		switch status.State {
		case svc.Stopped:
			return nil
		case svc.Running, svc.Paused:
			_, errControl := service.Control(svc.Stop)
			if errControl != nil &&
				!errors.Is(errControl, windows.ERROR_SERVICE_NOT_ACTIVE) &&
				!errors.Is(errControl, windows.ERROR_SERVICE_CANNOT_ACCEPT_CTRL) {
				return fmt.Errorf("stop Windows service %s before restart: %w", serviceName, errControl)
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait for Windows service %s to stop before restart: deadline exceeded", serviceName)
		}
		time.Sleep(windowsServiceRestartPoll)
	}
}

func waitForRestartedWindowsService(
	service windowsServiceRestarter,
	serviceName string,
	deadline time.Time,
) error {
	for {
		status, errQuery := service.Query()
		if errQuery != nil {
			return fmt.Errorf("query Windows service %s while starting: %w", serviceName, errQuery)
		}
		if status.State == svc.Running {
			return nil
		}
		if status.State == svc.Stopped {
			return fmt.Errorf("windows service %s stopped before reaching running state", serviceName)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait for Windows service %s to start: deadline exceeded", serviceName)
		}
		time.Sleep(windowsServiceRestartPoll)
	}
}
