package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

func shutdownServers(ctxCancel context.CancelFunc, servers ...*http.Server) {
	ctxCancel()
	log.Debug().Msg("Graceful shutdown context cancelled")
	shutdownCtx, shutdownCtxCancel := context.WithTimeout(context.Background(), time.Second*5)
	defer shutdownCtxCancel()

	for _, server := range servers {
		if server == nil {
			continue
		}
		errShutdown := server.Shutdown(shutdownCtx)
		if errShutdown != nil {
			log.Error().Err(errShutdown).Str("addr", server.Addr).Msg("Failed to shutdown server")
		}
	}
	log.Info().Msg("Xylona control panel backend fully stopped.")
}

func startServer(cancel context.CancelFunc, serverName string, serve func() error, startupErrCh chan<- error) {
	errServe := serve()
	if errServe == nil || errors.Is(errServe, http.ErrServerClosed) {
		return
	}

	cancel()

	errStartup := fmt.Errorf("%s: %w", serverName, errServe)
	select {
	case startupErrCh <- errStartup:
	default:
		log.Error().Err(errStartup).Msg("Dropping startup error because the startup error channel is full")
	}
}
