package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/ClintonCollins/Xylona/internal/appservice"
)

const (
	nodeServiceName        = "XylonaNode"
	nodeServiceUnitName    = "xylona-node.service"
	nodeServiceDisplayName = "Xylona Node"
	nodeServiceDescription = "Xylona remote game server node"
)

var nodeBootstrapFlagNames = []string{
	"controller-url",
	"join-token",
	"advertise-url",
	"node-name",
	"skip-insecure-tls",
}

type nodeServicePreparation struct {
	config          *cliConfig
	absoluteDataDir string
	identityCreated bool
	identityExists  bool
}

func nodeServiceDefinition(arguments ...string) appservice.Definition {
	return appservice.Definition{
		Name:        nodeServiceName,
		UnitName:    nodeServiceUnitName,
		DisplayName: nodeServiceDisplayName,
		Description: nodeServiceDescription,
		Arguments:   append([]string(nil), arguments...),
	}
}

func nodeServiceInstallFlags(includeUser bool) []cli.Flag {
	flags := nodeFlags(false)
	flags = append(flags, &cli.BoolFlag{
		Name:  "start",
		Usage: "Start the service after installing it",
	})
	if includeUser {
		flags = append(flags, &cli.StringFlag{
			Name:  "user",
			Usage: "Existing Linux user for the service; defaults to the invoking user",
		})
	}
	return flags
}

func rejectRootNodeFlagsForService(
	ctx context.Context,
	cmd *cli.Command,
) (context.Context, error) {
	for _, flag := range cmd.Root().Flags {
		if !flag.IsSet() {
			continue
		}
		flagNames := flag.Names()
		if len(flagNames) == 0 {
			continue
		}
		flagName := flagNames[0]
		if flagName == "help" || flagName == "version" {
			continue
		}
		return ctx, fmt.Errorf(
			"--%s was provided before the service subcommand and would not affect it; "+
				"place installation options after \"service install\"",
			flagName,
		)
	}
	return ctx, nil
}

func prepareNodeServiceInstall(cmd *cli.Command) (*nodeServicePreparation, error) {
	cfg, errConfig := nodeConfigFromCommand(cmd)
	if errConfig != nil {
		return nil, errConfig
	}
	absoluteDataDir, errAbsolute := filepath.Abs(cfg.dataDir)
	if errAbsolute != nil {
		return nil, fmt.Errorf("resolve node service data directory: %w", errAbsolute)
	}
	absoluteDataDir = filepath.Clean(absoluteDataDir)

	info, errDataDir := os.Lstat(absoluteDataDir)
	switch {
	case errors.Is(errDataDir, os.ErrNotExist):
	case errDataDir != nil:
		return nil, fmt.Errorf("inspect node service data directory: %w", errDataDir)
	default:
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("node service data directory cannot be a symlink: %s", absoluteDataDir)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("node service data path is not a directory: %s", absoluteDataDir)
		}
	}

	_, errIdentity := loadIdentity(absoluteDataDir)
	identityExists := errIdentity == nil
	if errIdentity != nil && !errors.Is(errIdentity, errIdentityMissing) {
		return nil, fmt.Errorf("load existing node identity before service installation: %w", errIdentity)
	}
	if identityExists {
		for _, flagName := range nodeBootstrapFlagNames {
			if cmd.IsSet(flagName) {
				return nil, fmt.Errorf(
					"--%s cannot be supplied when %s already contains a node identity",
					flagName,
					absoluteDataDir,
				)
			}
		}
	} else {
		if strings.TrimSpace(cfg.controllerURL) == "" {
			return nil, errors.New("--controller-url is required when installing a node without an existing identity")
		}
		if strings.TrimSpace(cfg.joinToken) == "" {
			return nil, errors.New("--join-token is required when installing a node without an existing identity")
		}
	}

	cfg.dataDir = absoluteDataDir
	return &nodeServicePreparation{
		config:          cfg,
		absoluteDataDir: absoluteDataDir,
		identityExists:  identityExists,
	}, nil
}

func pairNodeForService(ctx context.Context, preparation *nodeServicePreparation) error {
	if preparation == nil || preparation.config == nil {
		return errors.New("node service preparation is required")
	}
	if preparation.identityExists {
		return nil
	}
	_, errBootstrap := performBootstrap(ctx, preparation.config, preparation.absoluteDataDir)
	if errBootstrap != nil {
		return fmt.Errorf("pair node before service installation: %w", errBootstrap)
	}
	_, errIdentity := loadIdentity(preparation.absoluteDataDir)
	if errIdentity != nil {
		return fmt.Errorf("validate paired node identity before service installation: %w", errIdentity)
	}
	preparation.identityCreated = true
	preparation.identityExists = true
	return nil
}

func writeNodeServiceOutput(format string, values ...any) error {
	_, errWrite := fmt.Fprintf(nodeCLIStdout, format+"\n", values...)
	if errWrite != nil {
		return fmt.Errorf("write node service command output: %w", errWrite)
	}
	return nil
}
