package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node"
)

type fakeSelfUpdateCompleter struct {
	calls int
	err   error
}

func TestProcessSnapshotToProtoPreservesMetricValidity(t *testing.T) {
	t.Parallel()

	result := processSnapshotToProto(&node.ProcessSnapshot{
		IOValid:              true,
		ConnectionCountValid: true,
	})
	if !result.GetIoValid() || !result.GetConnectionCountValid() {
		t.Fatalf("metric validity = (IO %t, connections %t), want both true", result.GetIoValid(), result.GetConnectionCountValid())
	}
}

func (c *fakeSelfUpdateCompleter) CompleteSelfUpdate() error {
	c.calls++
	return c.err
}

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
		if cfg.processMetricsInterval != 3*time.Second || cfg.diskMetricsInterval != 30*time.Second || cfg.diskMetricsScans != 2 {
			t.Fatalf("unexpected metrics defaults: process=%v disk=%v scans=%d", cfg.processMetricsInterval, cfg.diskMetricsInterval, cfg.diskMetricsScans)
		}
	})

	t.Run("all flags are captured", func(t *testing.T) {
		t.Parallel()
		args := []string{
			"--controller-url", "https://controller.test",
			"--join-token", "tok-123",
			"--listen", ":9800",
			"--data-dir", "/var/lib/xylona-node",
			"--process-metrics-interval", "4s",
			"--disk-metrics-interval", "15s",
			"--disk-metrics-scans-per-tick", "3",
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
		if cfg.processMetricsInterval != 4*time.Second || cfg.diskMetricsInterval != 15*time.Second || cfg.diskMetricsScans != 3 {
			t.Fatalf("metrics flags: process=%v disk=%v scans=%d", cfg.processMetricsInterval, cfg.diskMetricsInterval, cfg.diskMetricsScans)
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

func TestRunCLIReportsErrorsOnce(t *testing.T) {
	originalStdout := nodeCLIStdout
	originalStderr := nodeCLIStderr
	originalLogger := log.Logger
	originalTimeFieldFormat := zerolog.TimeFieldFormat
	t.Cleanup(func() {
		nodeCLIStdout = originalStdout
		nodeCLIStderr = originalStderr
		log.Logger = originalLogger
		zerolog.TimeFieldFormat = originalTimeFieldFormat
	})

	tests := []struct {
		name      string
		arguments []string
		message   string
	}{
		{
			name:      "usage error",
			arguments: []string{"xylona-node", "--definitely-invalid"},
			message:   "flag provided but not defined: -definitely-invalid",
		},
		{
			name:      "action error",
			arguments: []string{"xylona-node", "--listen", " "},
			message:   "--listen is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			nodeCLIStdout = io.Discard
			nodeCLIStderr = &stderr

			exitCode := runCLI(test.arguments)
			if exitCode != 1 {
				t.Fatalf("runCLI() exit code = %d, want 1", exitCode)
			}
			if occurrences := strings.Count(stderr.String(), test.message); occurrences != 1 {
				t.Fatalf(
					"runCLI() output contains %q %d times, want once:\n%s",
					test.message,
					occurrences,
					stderr.String(),
				)
			}
		})
	}
}

func TestCLIConfigRestartArgs(t *testing.T) {
	t.Parallel()

	cfg := &cliConfig{
		controllerURL:          "https://controller.test",
		joinToken:              "secret-bootstrap-token",
		listen:                 ":9800",
		advertiseURL:           "https://node.test",
		nodeName:               "node-one",
		dataDir:                "relative-node-data",
		processMetricsInterval: 4 * time.Second,
		diskMetricsInterval:    15 * time.Second,
		diskMetricsScans:       3,
		skipInsecureTLS:        true,
	}
	want := []string{
		"--listen", ":9800",
		"--data-dir", "C:\\node-data",
		"--process-metrics-interval", "4s",
		"--disk-metrics-interval", "15s",
		"--disk-metrics-scans-per-tick", "3",
	}
	got := cfg.restartArgs("C:\\node-data")
	if !slices.Equal(got, want) {
		t.Fatalf("restartArgs() = %q, want %q", got, want)
	}
}

// TestRunWithoutIdentity verifies that a node without persisted identity
// requires either a join token or complete bootstrap configuration.
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

func TestCompleteNodeSelfUpdate(t *testing.T) {
	t.Parallel()

	errServe := errors.New("serve failed")
	errComplete := errors.New("completion failed")
	tests := []struct {
		name        string
		serveErr    error
		completeErr error
	}{
		{name: "successful shutdown and completion"},
		{name: "serve failure still completes update", serveErr: errServe},
		{name: "completion failure is returned", completeErr: errComplete},
		{name: "serve and completion failures are joined", serveErr: errServe, completeErr: errComplete},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			completer := &fakeSelfUpdateCompleter{err: test.completeErr}
			errResult := completeNodeSelfUpdate(completer, test.serveErr)
			if completer.calls != 1 {
				t.Fatalf("CompleteSelfUpdate() calls = %d, want 1", completer.calls)
			}
			if test.serveErr != nil && !errors.Is(errResult, test.serveErr) {
				t.Fatalf("completeNodeSelfUpdate() error = %v, want serve error", errResult)
			}
			if test.completeErr != nil && !errors.Is(errResult, test.completeErr) {
				t.Fatalf("completeNodeSelfUpdate() error = %v, want completion error", errResult)
			}
			if test.serveErr == nil && test.completeErr == nil && errResult != nil {
				t.Fatalf("completeNodeSelfUpdate() error = %v, want nil", errResult)
			}
		})
	}
}

func TestNodeHTTPServerCancelsActiveRequests(t *testing.T) {
	t.Parallel()

	serverCtx, cancelServer := context.WithCancel(t.Context())
	requestStarted := make(chan struct{})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
	})
	listenConfig := net.ListenConfig{}
	listener, errListen := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if errListen != nil {
		t.Fatalf("listen: %v", errListen)
	}
	server := newNodeHTTPServer(serverCtx, listener.Addr().String(), handler)
	t.Cleanup(func() {
		errClose := server.Close()
		if errClose != nil && !errors.Is(errClose, http.ErrServerClosed) && !errors.Is(errClose, net.ErrClosed) {
			t.Errorf("close server: %v", errClose)
		}
	})

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()
	request, errRequest := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String(), nil)
	if errRequest != nil {
		t.Fatalf("create request: %v", errRequest)
	}
	requestResult := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		response, errGet := client.Do(request)
		if errGet != nil {
			requestResult <- errGet
			return
		}
		requestResult <- response.Body.Close()
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach node HTTP server")
	}
	cancelServer()
	shutdownCtx, cancelShutdown := context.WithTimeout(t.Context(), time.Second)
	errShutdown := server.Shutdown(shutdownCtx)
	cancelShutdown()
	if errShutdown != nil {
		t.Fatalf("Shutdown() error = %v", errShutdown)
	}

	select {
	case errServe := <-serveResult:
		if !errors.Is(errServe, http.ErrServerClosed) {
			t.Fatalf("Serve() error = %v, want http.ErrServerClosed", errServe)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after lifecycle cancellation")
	}
	select {
	case <-requestResult:
	case <-time.After(time.Second):
		t.Fatal("active request did not exit after lifecycle cancellation")
	}
}
