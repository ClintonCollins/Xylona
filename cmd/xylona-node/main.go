// Command xylona-node is the lightweight agent half of the Xylona hub-spoke
// architecture. It owns process supervision, file operations, and host metrics
// for a single host and exposes them to the Xylona controller over Connect-RPC
// (xylona.node.v1.NodeService). On first run it can exchange a controller
// join token for a persistent node identity, then serve the pinned TLS
// NodeService endpoint on subsequent starts.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/api/xylona-internal/games"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/internal/nodetls"
	"github.com/ClintonCollins/Xylona/pkg/selfupdate"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
)

// cliConfig holds parsed command-line flags.
type cliConfig struct {
	controllerURL   string
	joinToken       string
	listen          string
	advertiseURL    string
	nodeName        string
	dataDir         string
	skipInsecureTLS bool
}

// parseFlags parses CLI flags from args (excluding program name).
func parseFlags(args []string) (*cliConfig, error) {
	fs := flag.NewFlagSet("xylona-node", flag.ContinueOnError)
	cfg := &cliConfig{}
	fs.StringVar(&cfg.controllerURL, "controller-url", "", "base URL of the Xylona controller, e.g. https://xylona.example.com")
	fs.StringVar(&cfg.joinToken, "join-token", "", "one-time bootstrap pairing token issued by the controller")
	fs.StringVar(&cfg.listen, "listen", ":9500", "HTTPS listen address for the node service")
	fs.StringVar(&cfg.advertiseURL, "advertise-url", "", "URL the controller should use to reach this node (defaults to the local address routed to the controller plus the --listen port, then hostname)")
	fs.StringVar(&cfg.nodeName, "node-name", "", "display name to register with the controller (defaults to OS hostname)")
	fs.StringVar(&cfg.dataDir, "data-dir", "./xylona-node-data", "directory to store persistent node identity")
	fs.BoolVar(&cfg.skipInsecureTLS, "skip-insecure-tls", false, "skip TLS certificate verification when sending the bootstrap request (one-shot; only affects --join-token pairing)")

	errParse := fs.Parse(args)
	if errParse != nil {
		return nil, fmt.Errorf("parse flags: %w", errParse)
	}

	if strings.TrimSpace(cfg.listen) == "" {
		return nil, errors.New("--listen is required")
	}
	if strings.TrimSpace(cfg.dataDir) == "" {
		return nil, errors.New("--data-dir is required")
	}
	return cfg, nil
}

func main() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	handledHelper, errHelper := selfupdate.RunHelperFromArgs(os.Args)
	if handledHelper {
		if errHelper != nil {
			log.Fatal().Err(errHelper).Msg("xylona-node update helper failed")
		}
		return
	}

	cfg, errFlags := parseFlags(os.Args[1:])
	if errFlags != nil {
		log.Fatal().Err(errFlags).Msg("invalid flags")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	errRun := run(ctx, cfg)
	cancel()
	if errRun != nil {
		log.Error().Err(errRun).Msg("xylona-node exited with error")
		os.Exit(1)
	}
}

// run executes the node binary logic. Split out so tests can exercise flag
// handling and identity loading without spawning a real process.
func run(ctx context.Context, cfg *cliConfig) error {
	absDataDir, errAbs := filepath.Abs(cfg.dataDir)
	if errAbs != nil {
		return fmt.Errorf("resolve data dir: %w", errAbs)
	}
	log.Info().Str("data_dir", absDataDir).Str("listen", cfg.listen).Msg("xylona-node starting")

	identity, errLoad := loadIdentity(absDataDir)
	if errLoad != nil {
		if !errors.Is(errLoad, errIdentityMissing) {
			return fmt.Errorf("load identity: %w", errLoad)
		}

		if strings.TrimSpace(cfg.joinToken) == "" {
			return fmt.Errorf("no persisted identity at %s and no --join-token provided: %w", absDataDir, errIdentityMissing)
		}

		bootstrapped, errBootstrap := performBootstrap(ctx, cfg, absDataDir)
		if errBootstrap != nil {
			return fmt.Errorf("bootstrap node: %w", errBootstrap)
		}
		identity = bootstrapped
	}

	log.Info().Str("node_id", identity.NodeID).Str("fingerprint", identity.Fingerprint).Msg("loaded node identity")

	supInst, errSup := supervisor.New(ctx)
	if errSup != nil {
		return fmt.Errorf("create supervisor: %w", errSup)
	}
	supInst.StartMetricsPoller(ctx)
	// Register built-in internal game installers (e.g. Minecraft) so remote
	// StartProcess requests with internal_command=true resolve locally. The
	// controller also registers these for the embedded node path.
	games.RegisterInternalGames()
	// pkg/node.New tolerates a nil *db.Connection for the node binary — the
	// node does not persist any game-server data. Controller is the single
	// source of truth.
	n := node.New(ctx, supInst, nil)

	updateManager, errUpdateManager := selfupdate.NewDefaultManager("node", filepath.Join(absDataDir, "updates"))
	if errUpdateManager != nil {
		return fmt.Errorf("create self-update manager: %w", errUpdateManager)
	}

	return serveNodeService(ctx, cfg.listen, identity, n, updateManager)
}

// serveNodeService mounts the NodeService handler on an HTTPS listener and
// blocks until ctx is canceled. Split out so serve can be exercised against a
// test listener.
func serveNodeService(ctx context.Context, listen string, identity *nodeIdentity, n *node.Node, updateManager selfUpdater) error {
	tlsConfig, errTLS := nodetls.NewServerTLSConfig([]byte(identity.CertPEM), []byte(identity.KeyPEM))
	if errTLS != nil {
		return fmt.Errorf("build server TLS: %w", errTLS)
	}

	mux := http.NewServeMux()
	svc := newNodeServiceServer(n, identity.SharedSecret, updateManager)
	path, handler := nodeprotoconnect.NewNodeServiceHandler(svc)
	mux.Handle(path, handler)

	server := &http.Server{
		Addr:              listen,
		Handler:           mux,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info().Str("listen", listen).Str("node_id", identity.NodeID).Msg("xylona-node HTTPS listener up")
		errListen := server.ListenAndServeTLS("", "")
		if errListen != nil && !errors.Is(errListen, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen and serve TLS: %w", errListen)
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("xylona-node shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		errShutdown := server.Shutdown(shutdownCtx)
		if errShutdown != nil {
			return fmt.Errorf("shutdown server: %w", errShutdown)
		}
		return nil
	case errServe := <-errCh:
		return errServe
	}
}
