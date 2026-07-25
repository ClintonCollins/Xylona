package appservice

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/selfupdate"
)

const (
	systemdManagedMarker = "# Managed by the Xylona service CLI. Do not edit."
	systemdUnitDirectory = "/etc/systemd/system"
)

func buildSystemdUnit(definition Definition, account Account) (string, error) {
	definition, errDefinition := resolveDefinition(definition)
	if errDefinition != nil {
		return "", errDefinition
	}
	if strings.TrimSpace(definition.UnitName) == "" {
		return "", errors.New("systemd unit name is required")
	}
	if !strings.HasSuffix(definition.UnitName, ".service") || strings.ContainsAny(definition.UnitName, `/\`) {
		return "", fmt.Errorf("invalid systemd unit name %q", definition.UnitName)
	}
	if strings.TrimSpace(account.Username) == "" || strings.TrimSpace(account.PrimaryGroup) == "" {
		return "", errors.New("systemd service user and group are required")
	}

	executable, errExecutable := quoteSystemdExecArgument(definition.ExecutablePath)
	if errExecutable != nil {
		return "", fmt.Errorf("encode systemd executable path: %w", errExecutable)
	}
	workingDirectory, errWorkingDirectory := quoteSystemdValue(definition.WorkingDirectory)
	if errWorkingDirectory != nil {
		return "", fmt.Errorf("encode systemd working directory: %w", errWorkingDirectory)
	}

	var execStart strings.Builder
	execStart.WriteString(executable)
	for _, argument := range definition.Arguments {
		quotedArgument, errArgument := quoteSystemdExecArgument(argument)
		if errArgument != nil {
			return "", fmt.Errorf("encode systemd argument: %w", errArgument)
		}
		execStart.WriteString(" ")
		execStart.WriteString(quotedArgument)
	}

	var unit strings.Builder
	unit.WriteString(systemdManagedMarker)
	unit.WriteString("\n[Unit]\n")
	unit.WriteString("Description=")
	unit.WriteString(definition.Description)
	unit.WriteString("\nWants=network-online.target\n")
	unit.WriteString("After=network-online.target\n")
	unit.WriteString("StartLimitIntervalSec=5min\n")
	unit.WriteString("StartLimitBurst=5\n\n")
	unit.WriteString("[Service]\n")
	unit.WriteString("Type=simple\n")
	unit.WriteString("User=")
	unit.WriteString(account.Username)
	unit.WriteString("\nGroup=")
	unit.WriteString(account.PrimaryGroup)
	unit.WriteString("\nWorkingDirectory=")
	unit.WriteString(workingDirectory)
	unit.WriteString("\nEnvironment=")
	restartEnvironment := selfupdate.RestartModeEnvironment + "=" + string(selfupdate.RestartModeSelf)
	unit.WriteString(strconv.Quote(restartEnvironment))
	unit.WriteString("\nExecStart=")
	unit.WriteString(execStart.String())
	unit.WriteString("\nRestart=on-failure\n")
	unit.WriteString("RestartSec=10s\n")
	unit.WriteString("TimeoutStopSec=2min\n\n")
	unit.WriteString("[Install]\n")
	unit.WriteString("WantedBy=multi-user.target\n")
	return unit.String(), nil
}

func quoteSystemdValue(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("systemd values cannot contain a newline or NUL")
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "%", "%%")
	return `"` + escaped + `"`, nil
}

func quoteSystemdExecArgument(value string) (string, error) {
	quoted, errQuote := quoteSystemdValue(value)
	if errQuote != nil {
		return "", errQuote
	}
	return strings.ReplaceAll(quoted, "$", "$$"), nil
}
