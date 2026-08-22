//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/urfave/cli/v3"

	"github.com/ClintonCollins/Xylona/internal/appservice"
)

func platformCommands() []*cli.Command {
	return []*cli.Command{newSystemdNodeServiceCommand()}
}

func newSystemdNodeServiceCommand() *cli.Command {
	baseDefinition := nodeServiceDefinition()
	return &cli.Command{
		Name:      "service",
		Usage:     "Install and manage the Xylona node as a systemd service",
		UsageText: "xylona-node service <install|start|stop|status|uninstall>",
		Before:    rejectRootNodeFlagsForService,
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
		Commands: []*cli.Command{
			{
				Name:  "install",
				Usage: "Pair if needed, then install and enable a Xylona node systemd service",
				Flags: nodeServiceInstallFlags(true),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					errPrivileges := appservice.RequireLinuxRoot("install the Xylona node systemd service")
					if errPrivileges != nil {
						return fmt.Errorf("validate node service installation privileges: %w", errPrivileges)
					}
					account, warning, errAccount := appservice.ResolveLinuxAccount(cmd.String("user"))
					if errAccount != nil {
						return fmt.Errorf("resolve node systemd service account: %w", errAccount)
					}
					preparation, errPreparation := prepareNodeServiceInstall(cmd)
					if errPreparation != nil {
						return errPreparation
					}
					createdDataDirectories, errDataPath := prepareNodeServiceDataPath(preparation, account)
					if errDataPath != nil {
						return errDataPath
					}
					errPair := pairNodeForService(ctx, preparation)
					if errPair != nil {
						errCleanup := cleanupCreatedNodeServiceDirectories(createdDataDirectories)
						return errors.Join(errPair, errCleanup)
					}
					errOwnership := assignNewNodeServicePaths(preparation, account, createdDataDirectories)
					if errOwnership != nil {
						return errOwnership
					}

					definition := nodeServiceDefinition(
						preparation.config.restartArgs(preparation.absoluteDataDir)...,
					)
					result, errInstall := appservice.Install(ctx, definition, appservice.InstallOptions{
						Start:   cmd.Bool("start"),
						Account: &account,
					})
					if errInstall != nil {
						return fmt.Errorf("install node systemd service: %w", errInstall)
					}
					errOutput := writeNodeServiceOutput(
						"Installed and enabled %s using %s as %s with data directory %s.",
						nodeServiceUnitName,
						result.ExecutablePath,
						account.Username,
						preparation.absoluteDataDir,
					)
					if errOutput != nil {
						return errOutput
					}
					for _, warningMessage := range []string{warning, result.Warning} {
						if warningMessage == "" {
							continue
						}
						errOutput = writeNodeServiceOutput("Warning: %s", warningMessage)
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
				Usage: "Start the installed Xylona node systemd service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStart := appservice.Start(ctx, baseDefinition)
					if errStart != nil {
						return fmt.Errorf("start node systemd service: %w", errStart)
					}
					return writeNodeServiceOutput("%s is running.", nodeServiceUnitName)
				},
			},
			{
				Name:  "stop",
				Usage: "Gracefully stop the installed Xylona node systemd service",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errStop := appservice.Stop(ctx, baseDefinition)
					if errStop != nil {
						return fmt.Errorf("stop node systemd service: %w", errStop)
					}
					return writeNodeServiceOutput("%s is stopped.", nodeServiceUnitName)
				},
			},
			{
				Name:  "status",
				Usage: "Show the installed Xylona node systemd service status",
				Action: func(ctx context.Context, _ *cli.Command) error {
					state, errStatus := appservice.Status(ctx, baseDefinition)
					if errStatus != nil {
						return fmt.Errorf("query node systemd service: %w", errStatus)
					}
					return writeNodeServiceOutput("%s is %s.", nodeServiceUnitName, state)
				},
			},
			{
				Name:  "uninstall",
				Usage: "Stop and uninstall the node systemd service without deleting its identity or data",
				Action: func(ctx context.Context, _ *cli.Command) error {
					errUninstall := appservice.Uninstall(ctx, baseDefinition)
					if errUninstall != nil {
						return fmt.Errorf("uninstall node systemd service: %w", errUninstall)
					}
					return writeNodeServiceOutput(
						"Uninstalled %s. Node identity and data were left unchanged.",
						nodeServiceUnitName,
					)
				},
			},
		},
	}
}

func validateExistingNodeServiceAccess(preparation *nodeServicePreparation, account appservice.Account) error {
	info, errInfo := os.Lstat(preparation.absoluteDataDir)
	if errInfo != nil {
		return fmt.Errorf("inspect existing node data directory: %w", errInfo)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("node service data directory cannot be a symlink: %s", preparation.absoluteDataDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("node service data path is not a directory: %s", preparation.absoluteDataDir)
	}
	if !appservice.LinuxAccountCanAccess(
		preparation.absoluteDataDir,
		account,
		appservice.LinuxRead|appservice.LinuxWrite|appservice.LinuxExecute,
	) {
		return fmt.Errorf(
			"service user %s cannot read, write, and traverse existing data directory %s",
			account.Username,
			preparation.absoluteDataDir,
		)
	}
	if !appservice.LinuxAccountCanTraverse(preparation.absoluteDataDir, account) {
		return fmt.Errorf(
			"service user %s cannot traverse an ancestor of existing data directory %s",
			account.Username,
			preparation.absoluteDataDir,
		)
	}
	if !preparation.identityExists {
		return nil
	}
	identityPath := filepath.Join(preparation.absoluteDataDir, identityFileName)
	if !appservice.LinuxAccountCanAccess(identityPath, account, appservice.LinuxRead|appservice.LinuxWrite) {
		return fmt.Errorf(
			"service user %s cannot read and write existing identity %s",
			account.Username,
			identityPath,
		)
	}
	return nil
}

func assignNewNodeServicePaths(
	preparation *nodeServicePreparation,
	account appservice.Account,
	createdDataDirectories []string,
) error {
	if preparation.identityCreated {
		identityPath := filepath.Join(preparation.absoluteDataDir, identityFileName)
		errIdentity := appservice.ChownLinuxPath(identityPath, account)
		if errIdentity != nil {
			return nodeServiceOwnershipRecoveryError(
				preparation,
				account,
				fmt.Errorf("assign new node identity to service account: %w", errIdentity),
			)
		}
	}
	for _, directoryPath := range slices.Backward(createdDataDirectories) {
		errDirectory := appservice.ChownLinuxPath(directoryPath, account)
		if errDirectory != nil {
			return nodeServiceOwnershipRecoveryError(
				preparation,
				account,
				fmt.Errorf("assign new node data directory %s to service account: %w", directoryPath, errDirectory),
			)
		}
	}
	return nil
}

func nodeServiceOwnershipRecoveryError(
	preparation *nodeServicePreparation,
	account appservice.Account,
	ownershipError error,
) error {
	identityPath := filepath.Join(preparation.absoluteDataDir, identityFileName)
	return fmt.Errorf(
		"%w; node pairing succeeded, so preserve identity %s and data directory %s, "+
			"repair ownership of the identity, data directory, and any installer-created "+
			"ancestors for %s:%s, then rerun service install",
		ownershipError,
		identityPath,
		preparation.absoluteDataDir,
		account.Username,
		account.PrimaryGroup,
	)
}

func prepareNodeServiceDataPath(
	preparation *nodeServicePreparation,
	account appservice.Account,
) ([]string, error) {
	missingDirectories, existingParent, errMissing := missingNodeServiceDirectories(preparation.absoluteDataDir)
	if errMissing != nil {
		return nil, errMissing
	}
	if len(missingDirectories) == 0 {
		return nil, validateExistingNodeServiceAccess(preparation, account)
	}
	if !appservice.LinuxAccountCanTraverse(existingParent, account) {
		return nil, fmt.Errorf(
			"service user %s cannot traverse existing parent directory %s for node data path %s",
			account.Username,
			existingParent,
			preparation.absoluteDataDir,
		)
	}

	createdDataDirectories := make([]string, 0, len(missingDirectories))
	for _, directoryPath := range missingDirectories {
		errMkdir := os.Mkdir(directoryPath, 0o700)
		if errMkdir != nil {
			errCleanup := cleanupCreatedNodeServiceDirectories(createdDataDirectories)
			return nil, errors.Join(
				fmt.Errorf("create node service data directory %s: %w", directoryPath, errMkdir),
				errCleanup,
			)
		}
		createdDataDirectories = append(createdDataDirectories, directoryPath)
	}
	return createdDataDirectories, nil
}

func missingNodeServiceDirectories(dataDirectory string) ([]string, string, error) {
	missingLeafFirst := make([]string, 0)
	currentPath := filepath.Clean(dataDirectory)
	for {
		info, errInfo := os.Stat(currentPath)
		if errInfo == nil {
			if !info.IsDir() {
				return nil, "", fmt.Errorf("node service data ancestor is not a directory: %s", currentPath)
			}
			missingDirectories := make([]string, len(missingLeafFirst))
			for index := range missingLeafFirst {
				missingDirectories[len(missingLeafFirst)-1-index] = missingLeafFirst[index]
			}
			return missingDirectories, currentPath, nil
		}
		if !errors.Is(errInfo, os.ErrNotExist) {
			return nil, "", fmt.Errorf("inspect node service data ancestor %s: %w", currentPath, errInfo)
		}
		missingLeafFirst = append(missingLeafFirst, currentPath)
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return nil, "", fmt.Errorf("find existing parent for node service data directory %s", dataDirectory)
		}
		currentPath = parentPath
	}
}

func cleanupCreatedNodeServiceDirectories(createdDataDirectories []string) error {
	var cleanupErrors []error
	for _, directoryPath := range slices.Backward(createdDataDirectories) {
		errRemove := os.Remove(directoryPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("remove installer-created node data directory %s: %w", directoryPath, errRemove),
			)
		}
	}
	return errors.Join(cleanupErrors...)
}
