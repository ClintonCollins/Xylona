package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestStartServerCancelsContextAndReportsFatalServeError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startupErrCh := make(chan error, 1)
	serveErr := errors.New(`bind failed`)

	startServer(cancel, `web`, func() error {
		return serveErr
	}, startupErrCh)

	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf(`context error = %v, want %v`, ctx.Err(), context.Canceled)
	}

	select {
	case errStart := <-startupErrCh:
		if !errors.Is(errStart, serveErr) {
			t.Fatalf(`startup error = %v, want wrapped %v`, errStart, serveErr)
		}
	default:
		t.Fatal(`expected startup error to be reported`)
	}
}

func TestStartServerIgnoresGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startupErrCh := make(chan error, 1)

	startServer(cancel, `web`, func() error {
		return http.ErrServerClosed
	}, startupErrCh)

	if ctx.Err() != nil {
		t.Fatalf(`context error = %v, want nil`, ctx.Err())
	}

	select {
	case errStart := <-startupErrCh:
		t.Fatalf(`unexpected startup error = %v`, errStart)
	default:
	}
}
