//go:build !windows

package adminipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
)

func listen(endpoint string) (net.Listener, func() error, error) {
	errRemove := os.Remove(endpoint)
	if errRemove != nil && !os.IsNotExist(errRemove) {
		return nil, nil, fmt.Errorf(`adminipc: remove stale socket: %w`, errRemove)
	}

	ctx := context.Background()
	listenConfig := net.ListenConfig{}
	listener, errListen := listenConfig.Listen(ctx, `unix`, endpoint)
	if errListen != nil {
		return nil, nil, fmt.Errorf(`adminipc: listen on unix socket: %w`, errListen)
	}

	errChmod := os.Chmod(endpoint, 0o600)
	if errChmod != nil {
		errCleanup := errors.Join(
			closeListener(listener),
			removeSocket(endpoint),
		)
		if errCleanup != nil {
			return nil, nil, errors.Join(
				fmt.Errorf(`adminipc: chmod unix socket: %w`, errChmod),
				errCleanup,
			)
		}

		return nil, nil, fmt.Errorf(`adminipc: chmod unix socket: %w`, errChmod)
	}

	cleanup := func() error {
		return errors.Join(
			closeListener(listener),
			removeSocket(endpoint),
		)
	}

	return listener, cleanup, nil
}

func dialContext(ctx context.Context, endpoint string) (net.Conn, error) {
	dialer := net.Dialer{}
	conn, errDial := dialer.DialContext(ctx, `unix`, endpoint)
	if errDial != nil {
		return nil, fmt.Errorf(`adminipc: dial unix socket: %w`, errDial)
	}

	return conn, nil
}

func closeListener(listener net.Listener) error {
	errClose := listener.Close()
	if errClose != nil {
		return fmt.Errorf(`adminipc: close unix socket listener: %w`, errClose)
	}

	return nil
}

func removeSocket(endpoint string) error {
	errRemove := os.Remove(endpoint)
	if errRemove != nil && !os.IsNotExist(errRemove) {
		return fmt.Errorf(`adminipc: remove unix socket: %w`, errRemove)
	}

	return nil
}
