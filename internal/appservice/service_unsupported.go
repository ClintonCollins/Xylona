//go:build !windows && !linux

package appservice

import (
	"context"
	"errors"
)

func platformInstall(context.Context, Definition, InstallOptions) (InstallResult, error) {
	return InstallResult{}, ErrUnsupported
}

func platformStart(context.Context, Definition) error {
	return ErrUnsupported
}

func platformStop(context.Context, Definition) error {
	return ErrUnsupported
}

func platformStatus(context.Context, Definition) (string, error) {
	return "", ErrUnsupported
}

func platformUninstall(context.Context, Definition) error {
	return ErrUnsupported
}

func platformRun(Definition, RunFunc, LogConfigurator) error {
	return errors.New("service hosting is unavailable: " + ErrUnsupported.Error())
}
