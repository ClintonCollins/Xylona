package main

import (
	"errors"
	"testing"
)

// TestParseFlags covers the CLI surface. Individual field defaults and
// required-field rejection are important because --listen / --data-dir are
// the only inputs that must be present.
func TestParseFlags(t *testing.T) {
	t.Parallel()

	t.Run("defaults are applied when no flags given", func(t *testing.T) {
		t.Parallel()
		cfg, errParse := parseFlags(nil)
		if errParse != nil {
			t.Fatalf("parseFlags: %v", errParse)
		}
		if cfg.listen == "" {
			t.Fatalf("expected default listen, got empty")
		}
		if cfg.dataDir == "" {
			t.Fatalf("expected default data dir, got empty")
		}
	})

	t.Run("all flags are captured", func(t *testing.T) {
		t.Parallel()
		args := []string{
			"--controller-url", "https://controller.test",
			"--join-token", "tok-123",
			"--listen", ":9800",
			"--data-dir", "/var/lib/xylona-node",
		}
		cfg, errParse := parseFlags(args)
		if errParse != nil {
			t.Fatalf("parseFlags: %v", errParse)
		}
		if cfg.controllerURL != "https://controller.test" {
			t.Fatalf("controllerURL: %q", cfg.controllerURL)
		}
		if cfg.joinToken != "tok-123" {
			t.Fatalf("joinToken: %q", cfg.joinToken)
		}
		if cfg.listen != ":9800" {
			t.Fatalf("listen: %q", cfg.listen)
		}
		if cfg.dataDir != "/var/lib/xylona-node" {
			t.Fatalf("dataDir: %q", cfg.dataDir)
		}
	})

	t.Run("empty listen is rejected", func(t *testing.T) {
		t.Parallel()
		_, errParse := parseFlags([]string{"--listen", "  "})
		if errParse == nil {
			t.Fatal("expected error for empty listen")
		}
	})

	t.Run("empty data-dir is rejected", func(t *testing.T) {
		t.Parallel()
		_, errParse := parseFlags([]string{"--data-dir", "  "})
		if errParse == nil {
			t.Fatal("expected error for empty data-dir")
		}
	})
}

// TestRunWithoutIdentity exercises the Step 4 behavior: calling run() against
// a data dir with no identity fails cleanly.
//   - No join token: returns errIdentityMissing wrapper (no pairing was requested)
//   - Join token: returns errBootstrapNotImplemented (pairing is Step 6)
func TestRunWithoutIdentity(t *testing.T) {
	t.Parallel()

	t.Run("no identity, no token -> errIdentityMissing", func(t *testing.T) {
		t.Parallel()
		cfg := &cliConfig{
			listen:  "127.0.0.1:0",
			dataDir: t.TempDir(),
		}
		errRun := run(t.Context(), cfg)
		if !errors.Is(errRun, errIdentityMissing) {
			t.Fatalf("expected errIdentityMissing, got %v", errRun)
		}
	})

	t.Run("no identity, join token without controller URL -> error", func(t *testing.T) {
		t.Parallel()
		cfg := &cliConfig{
			listen:    "127.0.0.1:0",
			dataDir:   t.TempDir(),
			joinToken: "tok-abc",
		}
		errRun := run(t.Context(), cfg)
		if errRun == nil {
			t.Fatalf("expected bootstrap to fail without --controller-url, got nil")
		}
	})
}
