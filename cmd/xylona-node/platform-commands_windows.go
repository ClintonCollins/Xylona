//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/ClintonCollins/Xylona/internal/appservice"
	"github.com/ClintonCollins/Xylona/internal/selfupdate"
)

var (
	requireWindowsServiceManagerAccess = appservice.RequireWindowsServiceManagerAccess
	pairWindowsNodeForService          = pairNodeForService
)

func platformCommands() []*cli.Command {
	return []*cli.Command{newWindowsNodeServiceCommand()}
}

func newWindowsNodeServiceCommand() *cli.Command {
	baseDefinition := nodeServiceDefinition()
	return &cli.Command{
		Name:      "service",
		Usage:     "Install and manage the Xylona node as a Windows service",
		UsageText: "xylona-node service <install|start|stop|status|uninstall>",
		Before:    rejectRootNodeFlagsForService,
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
		Commands: []*cli.Command{
			{
				Name:  "install",
				Usage: "Pair if needed, then install an automatic Xylona node service",
				Flags: nodeServiceInstallFlags(false),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					errPrivileges := requireWindowsServiceManagerAccess()
					if errPrivileges != nil {
						return fmt.Errorf("validate node service installation privileges: %w", errPrivileges)
					}
					preparation, errPreparation := prepareNodeServiceInstall(cmd)
					if errPreparation != nil {
						return errPreparation
					}
					errPair := pairWindowsNodeForService(ctx, preparation)
					if errPair != nil {
						return errPair
					}
					arguments := append(
						[]string{"service", "run"},
						preparation.config.restartArgs(preparation.absoluteDataDir)...,
					)
					definition := nodeServiceDefinition(arguments...)
					result, errInstall := appservice.Install(ctx, definition, appservice.InstallOptions{Start: cmd.Bool("start")})
					if errInstall != nil {
						return fmt.Errorf("install node Windows service: %w", errInstall)
					}
					errOutput := writeNodeServiceOutput(
						"Installed Windows service %s using %s and data directory %s.",
						nodeServiceDisplayName,
						result.ExecutablePath,
						preparation.absoluteDataDir,
					)
					if errOutput != nil {
						return errOutput
					}
					if result.Warning != "" {
						errOutput = writeNodeServiceOutput("%s", result.Warning)
						if errOutput != nil {
							return errOutput
						}
					}
					if !cmd.Bool("start") {
						return writeNodeServiceOutput("Start it with: xylona-node service start")
					}
					return nil
				},
			},
			{
				Name:  "start",
				Usage: "Start the installed Xylona node service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStart := appservice.Start(ctx, baseDefinition)
					if errStart != nil {
						return fmt.Errorf("start node Windows service: %w", errStart)
					}
					return writeNodeServiceOutput("Windows service %s is running.", nodeServiceDisplayName)
				},
			},
			{
				Name:  "stop",
				Usage: "Gracefully stop the installed Xylona node service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStop := appservice.Stop(ctx, baseDefinition)
					if errStop != nil {
						return fmt.Errorf("stop node Windows service: %w", errStop)
					}
					return writeNodeServiceOutput("Windows service %s is stopped.", nodeServiceDisplayName)
				},
			},
			{
				Name:  "status",
				Usage: "Show the installed Xylona node service status",
				Action: func(ctx context.Context, _ *cli.Command) error {
					state, errStatus := appservice.Status(ctx, baseDefinition)
					if errStatus != nil {
						return fmt.Errorf("query node Windows service: %w", errStatus)
					}
					return writeNodeServiceOutput("Windows service %s is %s.", nodeServiceDisplayName, state)
				},
			},
			{
				Name:  "uninstall",
				Usage: "Stop and uninstall the node service without deleting its identity or data",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errUninstall := appservice.Uninstall(ctx, baseDefinition)
					if errUninstall != nil {
						return fmt.Errorf("uninstall node Windows service: %w", errUninstall)
					}
					return writeNodeServiceOutput(
						"Uninstalled Windows service %s. Node identity and data were left unchanged.",
						nodeServiceDisplayName,
					)
				},
			},
			{
				Name:   "run",
				Usage:  "Run the Xylona node under the Windows Service Control Manager",
				Hidden: true,
				Flags:  nodeFlags(false, false),
				Action: func(_ context.Context, cmd *cli.Command) error {
					cfg, errConfig := nodeConfigFromCommand(cmd)
					if errConfig != nil {
						return errConfig
					}
					definition := nodeServiceDefinition()
					errRun := appservice.Run(
						definition,
						func(shutdownSignals <-chan os.Signal) int {
							return runNodeUntilSignal(cfg, shutdownSignals)
						},
						configureNodeServiceLogs,
					)
					if errRun != nil {
						return fmt.Errorf("host node Windows service: %w", errRun)
					}
					return nil
				},
			},
		},
	}
}

func runNodeUntilSignal(cfg *cliConfig, shutdownSignals <-chan os.Signal) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stopRequested atomic.Bool
	go func() {
		select {
		case <-shutdownSignals:
			stopRequested.Store(true)
			cancel()
		case <-ctx.Done():
		}
	}()
	errRun := run(ctx, cfg)
	if errRun != nil {
		log.Error().Err(errRun).Msg("xylona-node service exited with error")
	}
	restartMode := selfupdate.RestartMode(os.Getenv(selfupdate.RestartModeEnvironment))
	return windowsNodeServiceExitCode(errRun, stopRequested.Load(), restartMode)
}

func windowsNodeServiceExitCode(
	errRun error,
	stopRequested bool,
	restartMode selfupdate.RestartMode,
) int {
	if errRun != nil {
		return 1
	}
	if !stopRequested && restartMode == selfupdate.RestartModeWindowsService {
		return appservice.UpdateHandoffExitCode
	}
	return 0
}

func configureNodeServiceLogs(writer io.Writer) (func(), error) {
	log.Logger = zerolog.New(writer).With().Timestamp().Logger()
	return func() {}, nil
}
