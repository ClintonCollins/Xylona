//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/ClintonCollins/Xylona/internal/appservice"
)

func platformCommands() []*cli.Command {
	return []*cli.Command{newSystemdServiceCommand()}
}

func newSystemdServiceCommand() *cli.Command {
	definition := controllerServiceDefinition()
	return &cli.Command{
		Name:      "service",
		Usage:     "Install and manage Xylona as a systemd service",
		UsageText: "xylona service <install|start|stop|status|uninstall>",
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
		Commands: []*cli.Command{
			{
				Name:  "install",
				Usage: "Install and enable Xylona as a systemd service",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "user", Usage: "Existing Linux user for the service; defaults to the invoking user"},
					&cli.BoolFlag{Name: "start", Usage: "Start the service after installing it"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					errPrivileges := appservice.RequireLinuxRoot("install the Xylona systemd service")
					if errPrivileges != nil {
						return fmt.Errorf("validate controller service installation privileges: %w", errPrivileges)
					}
					account, warning, errAccount := appservice.ResolveLinuxAccount(cmd.String("user"))
					if errAccount != nil {
						return fmt.Errorf("resolve controller systemd service account: %w", errAccount)
					}
					result, errInstall := appservice.Install(ctx, definition, appservice.InstallOptions{
						Start:   cmd.Bool("start"),
						Account: &account,
					})
					if errInstall != nil {
						return fmt.Errorf("install controller systemd service: %w", errInstall)
					}
					errOutput := writeServiceOutput(
						"Installed and enabled %s using %s as %s.",
						controllerServiceUnitName,
						result.ExecutablePath,
						account.Username,
					)
					if errOutput != nil {
						return errOutput
					}
					for _, warningMessage := range []string{warning, result.Warning} {
						if warningMessage == "" {
							continue
						}
						errOutput = writeServiceOutput("Warning: %s", warningMessage)
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
				Usage: "Start the installed Xylona systemd service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStart := appservice.Start(ctx, definition)
					if errStart != nil {
						return fmt.Errorf("start controller systemd service: %w", errStart)
					}
					return writeServiceOutput("%s is running.", controllerServiceUnitName)
				},
			},
			{
				Name:  "stop",
				Usage: "Gracefully stop the installed Xylona systemd service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStop := appservice.Stop(ctx, definition)
					if errStop != nil {
						return fmt.Errorf("stop controller systemd service: %w", errStop)
					}
					return writeServiceOutput("%s is stopped.", controllerServiceUnitName)
				},
			},
			{
				Name:  "status",
				Usage: "Show the installed Xylona systemd service status",
				Action: func(ctx context.Context, _ *cli.Command) error {
					state, errStatus := appservice.Status(ctx, definition)
					if errStatus != nil {
						return fmt.Errorf("query controller systemd service: %w", errStatus)
					}
					return writeServiceOutput("%s is %s.", controllerServiceUnitName, state)
				},
			},
			{
				Name:  "uninstall",
				Usage: "Stop and uninstall the Xylona systemd service without deleting Xylona data",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errUninstall := appservice.Uninstall(ctx, definition)
					if errUninstall != nil {
						return fmt.Errorf("uninstall controller systemd service: %w", errUninstall)
					}
					return writeServiceOutput(
						"Uninstalled %s. Xylona files and data were left unchanged.",
						controllerServiceUnitName,
					)
				},
			},
		},
	}
}
