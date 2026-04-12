//go:build !windows

package adminipc

import (
	"context"
	"fmt"
	"net"
	"os"
)

func listen(endpoint string) (net.Listener, func() error, error) {
	errRemove := os.Remove(endpoint)
	if errRemove != nil && !os.IsNotExist(errRemove) {
		return nil, nil, fmt.Errorf(`adminipc: remove stale socket: %w`, errRemove)
	}

	listener, errListen := net.Listen(`unix`, endpoint)
	if errListen != nil {
		return nil, nil, fmt.Errorf(`adminipc: listen on unix socket: %w`, errListen)
	}

	errChmod := os.Chmod(endpoint, 0o600)
	if errChmod != nil {
		_ = listener.Close()
		_ = os.Remove(endpoint)
		return nil, nil, fmt.Errorf(`adminipc: chmod unix socket: %w`, errChmod)
	}

	cleanup := func() error {
		errClose := listener.Close()
		errRemoveSocket := os.Remove(endpoint)
		if errRemoveSocket != nil && !os.IsNotExist(errRemoveSocket) && errClose == nil {
			return errRemoveSocket
		}
		return errClose
	}

	return listener, cleanup, nil
}

func dialContext(_ context.Context, endpoint string) (net.Conn, error) {
	return net.Dial(`unix`, endpoint)
}
