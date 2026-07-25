// Package appservice installs and runs Xylona binaries as operating-system
// services.
package appservice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsupported is returned when the current operating system does not have a
// supported service backend.
var ErrUnsupported = errors.New("service management is not supported on this operating system")

// UpdateHandoffExitCode tells the Windows service host that the application
// exited for an update helper that will explicitly restart the service. It is
// an internal service-host result, not an operating-system process exit code.
const UpdateHandoffExitCode = -1

// Definition describes one Xylona operating-system service.
type Definition struct {
	Name             string
	UnitName         string
	DisplayName      string
	Description      string
	ExecutablePath   string
	WorkingDirectory string
	Arguments        []string
}

// Account identifies the Linux account used to run a systemd service.
type Account struct {
	Username       string
	UID            string
	PrimaryGroup   string
	PrimaryGroupID string
	GroupIDs       []string
}

// InstallOptions controls service registration.
type InstallOptions struct {
	Start   bool
	User    string
	Account *Account
}

// InstallResult reports the resolved service installation details.
type InstallResult struct {
	ExecutablePath string
	User           string
	Warning        string
}

// RunFunc runs the application until a service stop signal is received and
// returns the process exit code.
type RunFunc func(shutdownSignals <-chan os.Signal) int

// LogConfigurator directs application logs to the service-native log writer.
// The returned cleanup function is called before the writer is closed.
type LogConfigurator func(writer io.Writer) (cleanup func(), err error)

func resolveDefinition(definition Definition) (Definition, error) {
	definition.Name = strings.TrimSpace(definition.Name)
	definition.UnitName = strings.TrimSpace(definition.UnitName)
	definition.DisplayName = strings.TrimSpace(definition.DisplayName)
	definition.Description = strings.TrimSpace(definition.Description)
	if definition.Name == "" {
		return Definition{}, errors.New("service name is required")
	}
	if definition.DisplayName == "" {
		definition.DisplayName = definition.Name
	}
	if definition.Description == "" {
		definition.Description = definition.DisplayName
	}
	if strings.ContainsAny(definition.Description, "\r\n\x00") {
		return Definition{}, errors.New("service description cannot contain a newline or NUL")
	}

	executablePath := strings.TrimSpace(definition.ExecutablePath)
	if executablePath == "" {
		resolvedExecutable, errExecutable := os.Executable()
		if errExecutable != nil {
			return Definition{}, fmt.Errorf("resolve service executable: %w", errExecutable)
		}
		executablePath = resolvedExecutable
	}
	absoluteExecutable, errAbsoluteExecutable := filepath.Abs(executablePath)
	if errAbsoluteExecutable != nil {
		return Definition{}, fmt.Errorf("resolve absolute service executable path: %w", errAbsoluteExecutable)
	}
	definition.ExecutablePath = filepath.Clean(absoluteExecutable)

	workingDirectory := strings.TrimSpace(definition.WorkingDirectory)
	if workingDirectory == "" {
		workingDirectory = filepath.Dir(definition.ExecutablePath)
	}
	absoluteWorkingDirectory, errAbsoluteWorkingDirectory := filepath.Abs(workingDirectory)
	if errAbsoluteWorkingDirectory != nil {
		return Definition{}, fmt.Errorf("resolve absolute service working directory: %w", errAbsoluteWorkingDirectory)
	}
	definition.WorkingDirectory = filepath.Clean(absoluteWorkingDirectory)
	definition.Arguments = append([]string(nil), definition.Arguments...)
	return definition, nil
}

// Install registers and optionally starts a service.
func Install(ctx context.Context, definition Definition, options InstallOptions) (InstallResult, error) {
	return platformInstall(ctx, definition, options)
}

// Start starts an installed service and waits for it to become active.
func Start(ctx context.Context, definition Definition) error {
	return platformStart(ctx, definition)
}

// Stop gracefully stops an installed service and waits for it to stop.
func Stop(ctx context.Context, definition Definition) error {
	return platformStop(ctx, definition)
}

// Status returns the service's current operating-system state.
func Status(ctx context.Context, definition Definition) (string, error) {
	return platformStatus(ctx, definition)
}

// Uninstall stops and unregisters a service without deleting application data.
func Uninstall(ctx context.Context, definition Definition) error {
	return platformUninstall(ctx, definition)
}

// Run hosts the application under the current operating system's native
// service manager.
func Run(definition Definition, runApplication RunFunc, configureLogs LogConfigurator) error {
	return platformRun(definition, runApplication, configureLogs)
}
