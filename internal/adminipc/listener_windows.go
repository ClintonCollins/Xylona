//go:build windows

package adminipc

import (
	"context"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

func listen(endpoint string) (net.Listener, func() error, error) {
	listener, errListen := winio.ListenPipe(endpoint, &winio.PipeConfig{
		SecurityDescriptor: `D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;OW)`,
	})
	if errListen != nil {
		return nil, nil, fmt.Errorf(`adminipc: listen on named pipe: %w`, errListen)
	}

	return listener, listener.Close, nil
}

func dialContext(ctx context.Context, endpoint string) (net.Conn, error) {
	conn, errDial := winio.DialPipeContext(ctx, endpoint)
	if errDial != nil {
		return nil, fmt.Errorf(`adminipc: dial named pipe: %w`, errDial)
	}
	return conn, nil
}
