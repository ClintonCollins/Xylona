//go:build windows

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/ClintonCollins/Xylona/internal/appservice"
)

var runWindowsServiceApplication = runServiceUntil

func platformCommands() []*cli.Command {
	return []*cli.Command{newWindowsServiceCommand()}
}

func newWindowsServiceCommand() *cli.Command {
	definition := controllerServiceDefinition("service", "run")
	return &cli.Command{
		Name:      "service",
		Usage:     "Install and manage Xylona as a Windows service",
		UsageText: "xylona service <install|start|stop|status|uninstall>",
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
		Commands: []*cli.Command{
			{
				Name:  "install",
				Usage: "Install Xylona as an automatic Windows service",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "start", Usage: "Start the service after installing it"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					result, errInstall := appservice.Install(ctx, definition, appservice.InstallOptions{Start: cmd.Bool("start")})
					if errInstall != nil {
						return fmt.Errorf("install controller Windows service: %w", errInstall)
					}
					errOutput := writeServiceOutput(
						"Installed Windows service %s using %s.",
						controllerServiceName,
						result.ExecutablePath,
					)
					if errOutput != nil {
						return errOutput
					}
					if result.Warning != "" {
						errOutput = writeServiceOutput("%s", result.Warning)
						if errOutput != nil {
							return errOutput
						}
					}
					if !cmd.Bool("start") {
						return writeServiceOutput("Start it with: xylona service start")
					}
					return nil
				},
			},
			{
				Name:  "start",
				Usage: "Start the installed Xylona service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStart := appservice.Start(ctx, definition)
					if errStart != nil {
						return fmt.Errorf("start controller Windows service: %w", errStart)
					}
					return writeServiceOutput("Windows service %s is running.", controllerServiceName)
				},
			},
			{
				Name:  "stop",
				Usage: "Gracefully stop the installed Xylona service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStop := appservice.Stop(ctx, definition)
					if errStop != nil {
						return fmt.Errorf("stop controller Windows service: %w", errStop)
					}
					return writeServiceOutput("Windows service %s is stopped.", controllerServiceName)
				},
			},
			{
				Name:  "status",
				Usage: "Show the installed Xylona service status",
				Action: func(ctx context.Context, _ *cli.Command) error {
					state, errStatus := appservice.Status(ctx, definition)
					if errStatus != nil {
						return fmt.Errorf("query controller Windows service: %w", errStatus)
					}
					return writeServiceOutput("Windows service %s is %s.", controllerServiceName, state)
				},
			},
			{
				Name:  "uninstall",
				Usage: "Stop and uninstall the Xylona service without deleting Xylona data",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errUninstall := appservice.Uninstall(ctx, definition)
					if errUninstall != nil {
						return fmt.Errorf("uninstall controller Windows service: %w", errUninstall)
					}
					return writeServiceOutput(
						"Uninstalled Windows service %s. Xylona files and data were left unchanged.",
						controllerServiceName,
					)
				},
			},
			{
				Name:   "run",
				Usage:  "Run Xylona under the Windows Service Control Manager",
				Hidden: true,
				Action: func(_ context.Context, _ *cli.Command) error {
					errRun := appservice.Run(definition, runWindowsServiceApplication, configureControllerServiceLogs)
					if errRun != nil {
						return fmt.Errorf("host controller Windows service: %w", errRun)
					}
					return nil
				},
			},
		},
	}
}

func configureControllerServiceLogs(writer io.Writer) (func(), error) {
	runtimeLogWriterOverride = writer
	log.Logger = zerolog.New(writer).With().Timestamp().Logger()
	return func() {
		runtimeLogWriterOverride = nil
	}, nil
}
