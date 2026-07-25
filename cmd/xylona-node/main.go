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
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations/games"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/internal/nodetls"
	"github.com/ClintonCollins/Xylona/internal/selfupdate"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
)

const (
	defaultNodeListen              = ":9500"
	defaultNodeDataDir             = "./xylona-node-data"
	defaultProcessMetricsInterval  = 3 * time.Second
	defaultDiskMetricsInterval     = 30 * time.Second
	defaultDiskMetricsScansPerTick = 2
)

var (
	nodeCLIStdout = io.Writer(os.Stdout)
	nodeCLIStderr = io.Writer(os.Stderr)
)

type cliErrorWriter struct {
	writer io.Writer
	wrote  bool
}

func (w *cliErrorWriter) Write(message []byte) (int, error) {
	written, errWrite := w.writer.Write(message)
	if written > 0 {
		w.wrote = true
	}
	if errWrite != nil {
		return written, fmt.Errorf("write CLI error output: %w", errWrite)
	}
	return written, nil
}

// cliConfig holds parsed command-line flags.
type cliConfig struct {
	controllerURL          string
	joinToken              string
	listen                 string
	advertiseURL           string
	nodeName               string
	dataDir                string
	processMetricsInterval time.Duration
	diskMetricsInterval    time.Duration
	diskMetricsScans       int
	skipInsecureTLS        bool
}

func (cfg *cliConfig) restartArgs(absDataDir string) []string {
	return []string{
		"--listen", cfg.listen,
		"--data-dir", absDataDir,
		"--process-metrics-interval", cfg.processMetricsInterval.String(),
		"--disk-metrics-interval", cfg.diskMetricsInterval.String(),
		"--disk-metrics-scans-per-tick", fmt.Sprintf("%d", cfg.diskMetricsScans),
	}
}

func nodeFlags(includeBootstrap bool, local bool) []cli.Flag {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Value: defaultNodeListen,
			Usage: "HTTPS listen address for the node service",
			Local: local,
		},
		&cli.StringFlag{
			Name:  "data-dir",
			Value: defaultNodeDataDir,
			Usage: "Directory used to store the persistent node identity and update state",
			Local: local,
		},
		&cli.DurationFlag{
			Name:  "process-metrics-interval",
			Value: defaultProcessMetricsInterval,
			Usage: "Interval between process resource samples",
			Local: local,
		},
		&cli.DurationFlag{
			Name:  "disk-metrics-interval",
			Value: defaultDiskMetricsInterval,
			Usage: "Interval between bounded server-directory scan batches",
			Local: local,
		},
		&cli.IntFlag{
			Name:  "disk-metrics-scans-per-tick",
			Value: defaultDiskMetricsScansPerTick,
			Usage: "Maximum server directories scanned per disk metrics interval",
			Local: local,
		},
	}
	if !includeBootstrap {
		return flags
	}
	return append(flags,
		&cli.StringFlag{
			Name:  "controller-url",
			Usage: "Base URL of the Xylona controller, e.g. https://xylona.example.com",
			Local: local,
		},
		&cli.StringFlag{
			Name:  "join-token",
			Usage: "One-time bootstrap pairing token issued by the controller",
			Local: local,
		},
		&cli.StringFlag{
			Name:  "advertise-url",
			Usage: "URL the controller should use to reach this node",
			Local: local,
		},
		&cli.StringFlag{
			Name:  "node-name",
			Usage: "Display name to register with the controller; defaults to the OS hostname",
			Local: local,
		},
		&cli.BoolFlag{
			Name:  "skip-insecure-tls",
			Usage: "Skip controller TLS verification during one-time pairing only",
			Local: local,
		},
	)
}

func nodeConfigFromCommand(cmd *cli.Command) (*cliConfig, error) {
	cfg := &cliConfig{
		controllerURL:          cmd.String("controller-url"),
		joinToken:              cmd.String("join-token"),
		listen:                 cmd.String("listen"),
		advertiseURL:           cmd.String("advertise-url"),
		nodeName:               cmd.String("node-name"),
		dataDir:                cmd.String("data-dir"),
		processMetricsInterval: cmd.Duration("process-metrics-interval"),
		diskMetricsInterval:    cmd.Duration("disk-metrics-interval"),
		diskMetricsScans:       cmd.Int("disk-metrics-scans-per-tick"),
		skipInsecureTLS:        cmd.Bool("skip-insecure-tls"),
	}
	if strings.TrimSpace(cfg.listen) == "" {
		return nil, errors.New("--listen is required")
	}
	if strings.TrimSpace(cfg.dataDir) == "" {
		return nil, errors.New("--data-dir is required")
	}
	return cfg, nil
}

// parseFlags parses the foreground CLI flags from args (excluding program
// name). Production and tests use the same urfave/cli flag definitions.
func parseFlags(args []string) (*cliConfig, error) {
	var parsedConfig *cliConfig
	command := &cli.Command{
		Name:      "xylona-node",
		Writer:    io.Discard,
		ErrWriter: io.Discard,
		Flags:     nodeFlags(true, true),
		Action: func(_ context.Context, cmd *cli.Command) error {
			cfg, errConfig := nodeConfigFromCommand(cmd)
			if errConfig != nil {
				return errConfig
			}
			parsedConfig = cfg
			return nil
		},
	}
	runArgs := append([]string{"xylona-node"}, args...)
	errRun := command.Run(context.Background(), runArgs)
	if errRun != nil {
		return nil, fmt.Errorf("parse flags: %w", errRun)
	}
	return parsedConfig, nil
}

func main() {
	os.Exit(runCLI(os.Args))
}

func runCLI(args []string) int {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: nodeCLIStderr, TimeFormat: time.RFC3339})

	handledHelper, errHelper := selfupdate.RunHelperFromArgs(args)
	if handledHelper {
		if errHelper != nil {
			_, _ = fmt.Fprintln(nodeCLIStderr, errHelper)
			return 1
		}
		return 0
	}

	rootCommand := newNodeRootCommand()
	cliErrors := &cliErrorWriter{writer: nodeCLIStderr}
	rootCommand.ErrWriter = cliErrors
	errRun := rootCommand.Run(context.Background(), args)
	if errRun != nil {
		if !cliErrors.wrote {
			_, _ = fmt.Fprintln(nodeCLIStderr, errRun)
		}
		return 1
	}
	return 0
}

func newNodeRootCommand() *cli.Command {
	return &cli.Command{
		Name:      "xylona-node",
		Usage:     "Run the Xylona node or a service-management subcommand",
		UsageText: "xylona-node [foreground options] | xylona-node service <command> [service options]",
		Writer:    nodeCLIStdout,
		ErrWriter: nodeCLIStderr,
		Flags:     nodeFlags(true, true),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, errConfig := nodeConfigFromCommand(cmd)
			if errConfig != nil {
				return errConfig
			}
			return runNodeForeground(ctx, cfg)
		},
		Commands: platformCommands(),
	}
}

func runNodeForeground(parentContext context.Context, cfg *cliConfig) error {
	ctx, cancel := signal.NotifyContext(parentContext, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	errRun := run(ctx, cfg)
	if errRun != nil {
		log.Error().Err(errRun).Msg("xylona-node exited with error")
		return errRun
	}
	return nil
}

// run executes the node binary logic. Split out so tests can exercise flag
// handling and identity loading without spawning a real process.
func run(ctx context.Context, cfg *cliConfig) error {
	nodeCtx, shutdownNode := context.WithCancel(ctx)
	defer shutdownNode()

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

		bootstrapped, errBootstrap := performBootstrap(nodeCtx, cfg, absDataDir)
		if errBootstrap != nil {
			return fmt.Errorf("bootstrap node: %w", errBootstrap)
		}
		identity = bootstrapped
	}

	log.Info().Str("node_id", identity.NodeID).Str("fingerprint", identity.Fingerprint).Msg("loaded node identity")

	supInst, errSup := supervisor.New(nodeCtx)
	if errSup != nil {
		return fmt.Errorf("create supervisor: %w", errSup)
	}
	supInst.StartMetricsPollerWithOptions(nodeCtx, supervisor.MetricsPollerOptions{
		ProcessInterval:  cfg.processMetricsInterval,
		DiskInterval:     cfg.diskMetricsInterval,
		DiskScansPerTick: cfg.diskMetricsScans,
	})
	// Register built-in internal game installers (e.g. Minecraft) so remote
	// StartProcess requests with internal_command=true resolve locally. The
	// controller also registers these for the embedded node path.
	games.RegisterInternalGames()
	// node.New tolerates a nil *db.Connection for the node binary — the
	// node does not persist any game-server data. Controller is the single
	// source of truth.
	n := node.New(nodeCtx, supInst, nil)

	updateManager, errUpdateManager := selfupdate.NewManager(selfupdate.Config{
		Component:    "node",
		StageDir:     filepath.Join(absDataDir, "updates"),
		RestartArgs:  cfg.restartArgs(absDataDir),
		RestartMode:  selfupdate.RestartMode(os.Getenv(selfupdate.RestartModeEnvironment)),
		ShutdownFunc: shutdownNode,
	})
	if errUpdateManager != nil {
		return fmt.Errorf("create self-update manager: %w", errUpdateManager)
	}

	errServe := serveNodeService(nodeCtx, cfg.listen, identity, n, updateManager)
	shutdownNode()
	return completeNodeSelfUpdate(updateManager, errServe)
}

type selfUpdateCompleter interface {
	CompleteSelfUpdate() error
}

func completeNodeSelfUpdate(completer selfUpdateCompleter, errServe error) error {
	errComplete := completer.CompleteSelfUpdate()
	if errComplete != nil {
		errComplete = fmt.Errorf("complete self-update: %w", errComplete)
	}
	return errors.Join(errServe, errComplete)
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

	server := newNodeHTTPServer(ctx, listen, mux)
	server.TLSConfig = tlsConfig

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

func newNodeHTTPServer(ctx context.Context, listen string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}
}
