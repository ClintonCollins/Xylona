package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestPrepareNodeServiceInstall(t *testing.T) {
	t.Run("reuses existing identity with absolute data path", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "node-data")
		writeServiceTestIdentity(t, dataDir)

		preparation, errPrepare := parseNodeServiceInstallForTest(t, []string{
			"--data-dir", dataDir,
			"--listen", ":9800",
		})
		if errPrepare != nil {
			t.Fatalf("prepareNodeServiceInstall() error = %v", errPrepare)
		}
		if !preparation.identityExists || preparation.identityCreated {
			t.Fatalf(
				"identity state = exists:%t created:%t, want existing identity",
				preparation.identityExists,
				preparation.identityCreated,
			)
		}
		if !filepath.IsAbs(preparation.absoluteDataDir) {
			t.Fatalf("data directory = %q, want absolute path", preparation.absoluteDataDir)
		}
		if preparation.config.listen != ":9800" {
			t.Fatalf("listen = %q, want :9800", preparation.config.listen)
		}
	})

	t.Run("rejects bootstrap flags for existing identity", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "node-data")
		writeServiceTestIdentity(t, dataDir)

		_, errPrepare := parseNodeServiceInstallForTest(t, []string{
			"--data-dir", dataDir,
			"--join-token", "one-time-secret",
		})
		if errPrepare == nil || !strings.Contains(errPrepare.Error(), "already contains a node identity") {
			t.Fatalf("prepareNodeServiceInstall() error = %v, want existing-identity rejection", errPrepare)
		}
	})

	t.Run("requires controller and token for new identity", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "new-node-data")

		_, errPrepare := parseNodeServiceInstallForTest(t, []string{"--data-dir", dataDir})
		if errPrepare == nil || !strings.Contains(errPrepare.Error(), "--controller-url is required") {
			t.Fatalf("prepareNodeServiceInstall() error = %v, want controller requirement", errPrepare)
		}

		_, errPrepare = parseNodeServiceInstallForTest(t, []string{
			"--data-dir", dataDir,
			"--controller-url", "https://controller.test",
		})
		if errPrepare == nil || !strings.Contains(errPrepare.Error(), "--join-token is required") {
			t.Fatalf("prepareNodeServiceInstall() error = %v, want token requirement", errPrepare)
		}
	})

	t.Run("rejects invalid existing identity", func(t *testing.T) {
		dataDir := t.TempDir()
		errSave := saveIdentity(dataDir, &nodeIdentity{NodeID: "incomplete"})
		if errSave == nil {
			t.Fatal("saveIdentity() unexpectedly accepted incomplete identity")
		}
		identityPath := filepath.Join(dataDir, identityFileName)
		errWrite := os.WriteFile(identityPath, []byte("{invalid"), 0o600)
		if errWrite != nil {
			t.Fatalf("write invalid identity: %v", errWrite)
		}

		_, errPrepare := parseNodeServiceInstallForTest(t, []string{"--data-dir", dataDir})
		if errPrepare == nil || !strings.Contains(errPrepare.Error(), "load existing node identity") {
			t.Fatalf("prepareNodeServiceInstall() error = %v, want invalid identity error", errPrepare)
		}
	})
}

func TestServiceCommandFlagPlacement(t *testing.T) {
	t.Parallel()

	newCommand := func() *cli.Command {
		return &cli.Command{
			Name:  "xylona-node",
			Flags: nodeFlags(true),
			Commands: []*cli.Command{
				{
					Name:   "service",
					Before: rejectRootNodeFlagsForService,
					Commands: []*cli.Command{
						{
							Name:   "install",
							Flags:  nodeServiceInstallFlags(false),
							Action: func(context.Context, *cli.Command) error { return nil },
						},
					},
				},
			},
		}
	}

	errBeforeSubcommand := newCommand().Run(t.Context(), []string{
		"xylona-node",
		"--data-dir", "root-value",
		"service",
		"install",
	})
	if errBeforeSubcommand == nil ||
		!strings.Contains(errBeforeSubcommand.Error(), "after \"service install\"") {
		t.Fatalf(
			"pre-subcommand flag error = %v, want explicit placement guidance",
			errBeforeSubcommand,
		)
	}

	errAfterSubcommand := newCommand().Run(t.Context(), []string{
		"xylona-node",
		"service",
		"install",
		"--data-dir", "service-value",
	})
	if errAfterSubcommand != nil {
		t.Fatalf("post-subcommand flag placement error = %v", errAfterSubcommand)
	}
}

func parseNodeServiceInstallForTest(t *testing.T, arguments []string) (*nodeServicePreparation, error) {
	t.Helper()

	var preparation *nodeServicePreparation
	command := &cli.Command{
		Name:  "install",
		Flags: nodeServiceInstallFlags(false),
		Action: func(_ context.Context, cmd *cli.Command) error {
			var errPrepare error
			preparation, errPrepare = prepareNodeServiceInstall(cmd)
			return errPrepare
		},
	}
	runArguments := append([]string{"install"}, arguments...)
	errRun := command.Run(t.Context(), runArguments)
	if errRun != nil {
		return preparation, fmt.Errorf("run test node service install command: %w", errRun)
	}
	return preparation, nil
}

func writeServiceTestIdentity(t *testing.T, dataDir string) {
	t.Helper()

	errSave := saveIdentity(dataDir, &nodeIdentity{
		NodeID:        "node-service-test",
		CertPEM:       "test-cert",
		KeyPEM:        "test-key",
		Fingerprint:   "test-fingerprint",
		ControllerURL: "https://controller.test",
		SharedSecret:  "test-shared-secret",
	})
	if errSave != nil {
		t.Fatalf("save service test identity: %v", errSave)
	}
}
