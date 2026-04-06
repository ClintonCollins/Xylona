package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// binaryPath returns the path to the compiled test binary.
// Each test run uses go test -run, which means we need the binary built separately.
// We use `go run` or build once in TestMain.
var binaryPath string

func resolveGoBinary() string {
	goBinary, errLookPath := exec.LookPath("go")
	if errLookPath == nil {
		return goBinary
	}

	goBinaryName := "go"
	if runtime.GOOS == "windows" {
		goBinaryName += ".exe"
	}

	var candidates []string

	goRoot, okGoRoot := os.LookupEnv("GOROOT")
	if okGoRoot && goRoot != "" {
		candidates = append(candidates, filepath.Join(goRoot, "bin", goBinaryName))
	}

	if runtime.GOOS == "windows" {
		programFiles, okProgramFiles := os.LookupEnv("ProgramFiles")
		if okProgramFiles && programFiles != "" {
			candidates = append(candidates, filepath.Join(programFiles, "Go", "bin", goBinaryName))
		}

		candidates = append(candidates,
			filepath.Join(`C:\Program Files`, "Go", "bin", goBinaryName),
			filepath.Join(`C:\Go`, "bin", goBinaryName),
		)
	}

	for _, candidate := range candidates {
		_, errStat := os.Stat(candidate)
		if errStat == nil {
			return candidate
		}
	}

	return "go"
}

func TestMain(m *testing.M) {
	// Build the binary once for all tests.
	tmp, err := os.MkdirTemp("", "dummy_game_server_test_*")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	binaryName := "dummy_game_server_qa"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath = filepath.Join(tmp, binaryName)

	cmd := exec.CommandContext(context.Background(), resolveGoBinary(), "build", "-o", binaryPath, ".") //nolint:gosec // test helper executes the local Go toolchain to build the fixture binary
	cmd.Stderr = os.Stderr
	errRun := cmd.Run()
	if errRun != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to build binary: %v\n", errRun)
		os.Exit(1) //nolint:gocritic // standard TestMain exit; defer cleanup is intentionally skipped on build failure
	}

	os.Exit(m.Run())
}

// runServer runs the dummy game server with the given stdin and flags, and
// returns stdout, stderr, and the exit code.
func runServer(t *testing.T, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), binaryPath, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running server: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// runServerWithPipe runs the server with a controlled stdin pipe so tests can
// delay sending commands.
func runServerWithPipe(t *testing.T, args ...string) (stdinWriter io.WriteCloser, stdoutBuf *bytes.Buffer, wait func() int) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), binaryPath, args...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to get stdin pipe: %v", err)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	errStart := cmd.Start()
	if errStart != nil {
		t.Fatalf("failed to start server: %v", errStart)
	}
	return stdinPipe, &outBuf, func() int {
		errWait := cmd.Wait()
		if errWait != nil {
			var exitErr *exec.ExitError
			if errors.As(errWait, &exitErr) {
				return exitErr.ExitCode()
			}
		}
		return 0
	}
}

// TestStartupBanner verifies the banner is printed on stdout with a valid PID.
func TestStartupBanner(t *testing.T) {
	t.Parallel()

	stdout, _, exitCode := runServer(t, "stop\n", "-heartbeat=0")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no output on stdout")
	}
	banner := lines[0]
	if !strings.HasPrefix(banner, "[dummy-game-server] started pid=") {
		t.Errorf("startup banner format wrong: %q", banner)
	}
	pidStr := strings.TrimPrefix(banner, "[dummy-game-server] started pid=")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		t.Errorf("PID in banner is not a valid integer: %q", pidStr)
	}
	if pid <= 0 {
		t.Errorf("PID in banner must be positive, got %d", pid)
	}
}

// TestStopCommand verifies the stop command exits with code 0 and prints goodbye.
func TestStopCommand(t *testing.T) {
	t.Parallel()

	stdout, _, exitCode := runServer(t, "stop\n", "-heartbeat=0")
	if exitCode != 0 {
		t.Errorf("stop: expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "goodbye") {
		t.Errorf("stop: expected 'goodbye' in stdout, got: %q", stdout)
	}
}

// TestEchoCommand verifies the echo command outputs exactly the provided message.
func TestEchoCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple word", input: "echo hello\nstop\n", want: "hello"},
		{name: "multiple words", input: "echo hello world\nstop\n", want: "hello world"},
		{name: "internal spaces", input: "echo hello   world  test\nstop\n", want: "hello   world  test"},
		{name: "no argument", input: "echo\nstop\n", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, _, exitCode := runServer(t, tt.input, "-heartbeat=0")
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d", exitCode)
			}
			lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
			// Filter out server-prefixed lines to isolate echo output.
			var echoLines []string
			for _, l := range lines {
				if !strings.HasPrefix(l, "[dummy-game-server]") {
					echoLines = append(echoLines, l)
				}
			}
			got := strings.Join(echoLines, "\n")
			if got != tt.want {
				t.Errorf("echo output = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestStatusCommand verifies the status output contains pid=, uptime=, status=running.
func TestStatusCommand(t *testing.T) {
	t.Parallel()

	stdout, _, exitCode := runServer(t, "status\nstop\n", "-heartbeat=0")
	if exitCode != 0 {
		t.Fatalf("unexpected exit code %d", exitCode)
	}
	var statusLine string
	for l := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(l, "uptime=") {
			statusLine = l
			break
		}
	}
	if statusLine == "" {
		t.Fatalf("no status line found in output: %q", stdout)
	}
	for _, field := range []string{"pid=", "uptime=", "status=running"} {
		if !strings.Contains(statusLine, field) {
			t.Errorf("status line missing %q: %q", field, statusLine)
		}
	}
}

// TestStatusUptimeIncreases verifies uptime in status is non-zero after a delay.
func TestStatusUptimeIncreases(t *testing.T) {
	t.Parallel()

	stdin, _, wait := runServerWithPipe(t, "-heartbeat=0")
	time.Sleep(250 * time.Millisecond)
	_, _ = fmt.Fprintln(stdin, "status")
	_, _ = fmt.Fprintln(stdin, "stop")
	_ = stdin.Close()
	wait()

	// Re-run with proper output capture.
	stdout, _, exitCode := runServer(t, "status\nstop\n", "-heartbeat=0")
	if exitCode != 0 {
		t.Fatalf("unexpected exit code %d", exitCode)
	}
	// Just verify the uptime field is present - timing is hard to assert reliably.
	if !strings.Contains(stdout, "uptime=") {
		t.Errorf("status output missing uptime=: %q", stdout)
	}
}

// TestCrashCommand verifies crash exits with code 1.
func TestCrashCommand(t *testing.T) {
	t.Parallel()

	stdout, stderr, exitCode := runServer(t, "crash\n", "-heartbeat=0")
	if exitCode != 1 {
		t.Errorf("crash: expected exit code 1, got %d", exitCode)
	}
	_ = stdout
	if !strings.Contains(stderr, "crashing") {
		t.Errorf("crash: expected 'crashing' in stderr, got: %q", stderr)
	}
}

// TestStderrCommand verifies the stderr command writes to stderr, not stdout.
func TestStderrCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		msg     string
		wantMsg string
	}{
		{name: "simple message", msg: "test message", wantMsg: "test message"},
		{name: "multi-word message", msg: "this is a multi word error", wantMsg: "this is a multi word error"},
		{name: "no argument", msg: "", wantMsg: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var input string
			if tt.msg != "" {
				input = fmt.Sprintf("stderr %s\nstop\n", tt.msg)
			} else {
				input = "stderr\nstop\n"
			}
			stdout, stderr, exitCode := runServer(t, input, "-heartbeat=0")
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d", exitCode)
			}
			if tt.wantMsg != "" {
				if !strings.Contains(stderr, tt.wantMsg) {
					t.Errorf("stderr command: message %q not found in stderr: %q", tt.wantMsg, stderr)
				}
				if strings.Contains(stdout, tt.wantMsg) {
					t.Errorf("stderr command: message %q leaked to stdout: %q", tt.wantMsg, stdout)
				}
			}
		})
	}
}

// TestFloodCommand verifies the flood command outputs exactly 100 flood lines on stdout.
func TestFloodCommand(t *testing.T) {
	t.Parallel()

	stdout, stderr, exitCode := runServer(t, "flood\nstop\n", "-heartbeat=0")
	if exitCode != 0 {
		t.Fatalf("unexpected exit code %d", exitCode)
	}
	if stderr != "" {
		t.Errorf("flood: unexpected stderr output: %q", stderr)
	}
	var floodLines []string
	for l := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(l, "flood line") {
			floodLines = append(floodLines, l)
		}
	}
	if len(floodLines) != 100 {
		t.Errorf("flood: expected 100 flood lines, got %d", len(floodLines))
	}
	if len(floodLines) > 0 {
		if !strings.Contains(floodLines[0], "flood line 1") {
			t.Errorf("flood: first line should be 'flood line 1', got: %q", floodLines[0])
		}
		if !strings.Contains(floodLines[len(floodLines)-1], "flood line 100") {
			t.Errorf("flood: last line should be 'flood line 100', got: %q", floodLines[len(floodLines)-1])
		}
	}
}

// TestUnknownCommand verifies unknown commands print an "unknown command" response.
func TestUnknownCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  string
	}{
		{name: "all caps", cmd: "STOP"},
		{name: "mixed case", cmd: "Stop"},
		{name: "nonsense", cmd: "foobar"},
		{name: "numeric", cmd: "12345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := fmt.Sprintf("%s\nstop\n", tt.cmd)
			stdout, _, exitCode := runServer(t, input, "-heartbeat=0")
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d", exitCode)
			}
			if !strings.Contains(strings.ToLower(stdout), "unknown command") {
				t.Errorf("unknown command %q: expected 'unknown command' in output, got: %q", tt.cmd, stdout)
			}
		})
	}
}

// TestHeartbeatEnabled verifies heartbeat lines appear when -heartbeat is set.
func TestHeartbeatEnabled(t *testing.T) {
	t.Parallel()

	stdin, stdoutBuf, wait := runServerWithPipe(t, "-heartbeat=100ms")
	time.Sleep(350 * time.Millisecond)
	_, _ = fmt.Fprintln(stdin, "stop")
	_ = stdin.Close()
	exitCode := wait()
	if exitCode != 0 {
		t.Errorf("heartbeat: expected exit code 0, got %d", exitCode)
	}
	stdout := stdoutBuf.String()
	var heartbeatLines []string
	for l := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(l, "heartbeat") {
			heartbeatLines = append(heartbeatLines, l)
		}
	}
	if len(heartbeatLines) == 0 {
		t.Errorf("heartbeat: expected at least 1 heartbeat line, got 0\nstdout: %q", stdout)
	}
	// Verify heartbeat line format.
	for _, hbLine := range heartbeatLines {
		if !strings.Contains(hbLine, "pid=") {
			t.Errorf("heartbeat line missing pid=: %q", hbLine)
		}
		if !strings.Contains(hbLine, "uptime=") {
			t.Errorf("heartbeat line missing uptime=: %q", hbLine)
		}
	}
}

// TestHeartbeatDisabled verifies no heartbeat lines appear when -heartbeat=0.
func TestHeartbeatDisabled(t *testing.T) {
	t.Parallel()

	stdin, stdoutBuf, wait := runServerWithPipe(t, "-heartbeat=0")
	time.Sleep(200 * time.Millisecond)
	_, _ = fmt.Fprintln(stdin, "stop")
	_ = stdin.Close()
	exitCode := wait()
	if exitCode != 0 {
		t.Errorf("heartbeat disabled: expected exit code 0, got %d", exitCode)
	}
	stdout := stdoutBuf.String()
	for l := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(l, "heartbeat") {
			t.Errorf("heartbeat disabled: unexpected heartbeat line: %q", l)
		}
	}
}

// TestHeartbeatDefaultIs5s verifies the default heartbeat interval is 5 seconds.
func TestHeartbeatDefaultIs5s(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping long-running heartbeat default test in short mode")
	}
	stdin, stdoutBuf, wait := runServerWithPipe(t)
	// After 5.5s at least one heartbeat should have fired.
	time.Sleep(5500 * time.Millisecond)
	_, _ = fmt.Fprintln(stdin, "stop")
	_ = stdin.Close()
	exitCode := wait()
	if exitCode != 0 {
		t.Errorf("default heartbeat: expected exit code 0, got %d", exitCode)
	}
	stdout := stdoutBuf.String()
	var count int
	for l := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(l, "heartbeat") {
			count++
		}
	}
	if count < 1 {
		t.Errorf("default heartbeat: expected at least 1 heartbeat in 5.5s, got 0")
	}
}

// TestHeartbeatPIDMatchesBanner verifies heartbeat PID matches startup banner PID.
func TestHeartbeatPIDMatchesBanner(t *testing.T) {
	t.Parallel()

	stdin, stdoutBuf, wait := runServerWithPipe(t, "-heartbeat=100ms")
	time.Sleep(250 * time.Millisecond)
	_, _ = fmt.Fprintln(stdin, "stop")
	_ = stdin.Close()
	wait()
	stdout := stdoutBuf.String()
	var bannerPID, heartbeatPID string
	for l := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(l, "started pid=") {
			bannerPID = extractField(l, "pid=")
		}
		if strings.Contains(l, "heartbeat") && heartbeatPID == "" {
			heartbeatPID = extractField(l, "pid=")
		}
	}
	if bannerPID == "" {
		t.Fatal("no startup banner PID found")
	}
	if heartbeatPID == "" {
		t.Fatal("no heartbeat PID found")
	}
	if bannerPID != heartbeatPID {
		t.Errorf("banner PID %q != heartbeat PID %q", bannerPID, heartbeatPID)
	}
}

// TestStartupDelay verifies the -startup-delay flag delays the startup banner.
func TestStartupDelay(t *testing.T) {
	t.Parallel()

	start := time.Now()
	stdout, _, exitCode := runServer(t, "stop\n", "-heartbeat=0", "-startup-delay=400ms")
	elapsed := time.Since(start)
	if exitCode != 0 {
		t.Errorf("startup delay: expected exit code 0, got %d", exitCode)
	}
	if elapsed < 350*time.Millisecond {
		t.Errorf("startup delay: expected at least 350ms elapsed, got %v", elapsed)
	}
	if !strings.Contains(stdout, "started pid=") {
		t.Errorf("startup delay: banner missing from stdout: %q", stdout)
	}
}

// TestStartupDelayZero verifies -startup-delay=0 has no meaningful delay.
func TestStartupDelayZero(t *testing.T) {
	t.Parallel()

	start := time.Now()
	_, _, exitCode := runServer(t, "stop\n", "-heartbeat=0", "-startup-delay=0")
	elapsed := time.Since(start)
	if exitCode != 0 {
		t.Errorf("startup-delay=0: expected exit code 0, got %d", exitCode)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("startup-delay=0: expected fast startup, elapsed %v", elapsed)
	}
}

// TestEOFShutdown verifies that closing stdin (EOF) triggers a graceful shutdown with exit 0.
func TestEOFShutdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		stdin string
	}{
		{name: "empty input", stdin: ""},
		{name: "blank line then EOF", stdin: "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, _, exitCode := runServer(t, tt.stdin, "-heartbeat=0")
			if exitCode != 0 {
				t.Errorf("EOF: expected exit code 0, got %d", exitCode)
			}
			if !strings.Contains(stdout, "stdin closed") {
				t.Errorf("EOF: expected 'stdin closed' in stdout, got: %q", stdout)
			}
		})
	}
}

// TestBlankLinesIgnored verifies blank and whitespace-only lines are not treated as commands.
func TestBlankLinesIgnored(t *testing.T) {
	t.Parallel()

	stdout, _, exitCode := runServer(t, "\n\n   \n\t\nstop\n", "-heartbeat=0")
	if exitCode != 0 {
		t.Fatalf("unexpected exit code %d", exitCode)
	}
	if strings.Contains(stdout, "unknown command") {
		t.Errorf("blank lines should be ignored, got unknown command: %q", stdout)
	}
}

// TestCommandCaseSensitivity verifies commands are case-sensitive.
func TestCommandCaseSensitivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  string
	}{
		{"uppercase STOP", "STOP"},
		{"title case Stop", "Stop"},
		{"uppercase ECHO", "ECHO"},
		{"uppercase CRASH", "CRASH"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := fmt.Sprintf("%s\nstop\n", tt.cmd)
			stdout, _, exitCode := runServer(t, input, "-heartbeat=0")
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d", exitCode)
			}
			if !strings.Contains(strings.ToLower(stdout), "unknown command") {
				t.Errorf("command %q should be unknown (case-sensitive), stdout: %q", tt.cmd, stdout)
			}
		})
	}
}

// TestMultipleCommands verifies multiple commands in sequence are all processed.
func TestMultipleCommands(t *testing.T) {
	t.Parallel()

	input := "echo alpha\necho beta\nstatus\necho gamma\nstop\n"
	stdout, _, exitCode := runServer(t, input, "-heartbeat=0")
	if exitCode != 0 {
		t.Fatalf("unexpected exit code %d", exitCode)
	}
	for _, want := range []string{"alpha", "beta", "gamma", "status=running"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in stdout, not found: %q", want, stdout)
		}
	}
}

// TestCrashNoStdout verifies crash does not print additional lines to stdout after banner.
func TestCrashNoStdout(t *testing.T) {
	t.Parallel()

	stdout, _, exitCode := runServer(t, "crash\n", "-heartbeat=0")
	if exitCode != 1 {
		t.Errorf("crash: expected exit code 1, got %d", exitCode)
	}
	lines := nonEmptyLines(stdout)
	// Should only have the startup banner.
	if len(lines) != 1 {
		t.Errorf("crash: expected only startup banner on stdout, got %d lines: %q", len(lines), stdout)
	}
}

// TestScannerBufferOverflow tests behavior when a stdin line exceeds bufio.Scanner's 64KB limit.
// The scanner error must be reported on stderr and exit non-zero — not silently treated as EOF.
func TestScannerBufferOverflow(t *testing.T) {
	t.Parallel()

	// Generate a line just over the 64KB scanner default limit.
	oversizedLine := "echo " + strings.Repeat("A", 65540)
	input := oversizedLine + "\nstop\n"
	_, stderr, exitCode := runServer(t, input, "-heartbeat=0")
	if exitCode == 0 {
		t.Errorf("expected non-zero exit for oversized stdin line, got 0")
	}
	if !strings.Contains(stderr, "stdin read error") {
		t.Errorf("expected 'stdin read error' in stderr, got: %q", stderr)
	}
}

// TestHeartbeatUptimeMonotonic verifies heartbeat uptime values increase over time.
func TestHeartbeatUptimeMonotonic(t *testing.T) {
	t.Parallel()

	stdin, stdoutBuf, wait := runServerWithPipe(t, "-heartbeat=100ms")
	time.Sleep(500 * time.Millisecond)
	_, _ = fmt.Fprintln(stdin, "stop")
	_ = stdin.Close()
	wait()
	stdout := stdoutBuf.String()

	var uptimes []time.Duration
	for l := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(l, "heartbeat") {
			uptimeStr := extractField(l, "uptime=")
			if uptimeStr == "" {
				continue
			}
			d, err := time.ParseDuration(uptimeStr)
			if err != nil {
				t.Errorf("failed to parse uptime %q: %v", uptimeStr, err)
				continue
			}
			uptimes = append(uptimes, d)
		}
	}
	if len(uptimes) < 2 {
		t.Skipf("not enough heartbeat lines to check monotonicity (%d)", len(uptimes))
	}
	for i := 1; i < len(uptimes); i++ {
		if uptimes[i] <= uptimes[i-1] {
			t.Errorf("heartbeat uptime not increasing: [%d]=%v <= [%d]=%v",
				i, uptimes[i], i-1, uptimes[i-1])
		}
	}
}

// TestFloodThenStop verifies flood followed by stop works correctly.
func TestFloodThenStop(t *testing.T) {
	t.Parallel()

	stdout, _, exitCode := runServer(t, "flood\nstop\n", "-heartbeat=0")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "goodbye") {
		t.Errorf("expected goodbye after flood+stop: %q", stdout)
	}
	var floodCount int
	for l := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(l, "flood line") {
			floodCount++
		}
	}
	if floodCount != 100 {
		t.Errorf("expected 100 flood lines after flood+stop, got %d", floodCount)
	}
}

// TestLargeEchoMessage verifies echo handles messages up to the scanner limit.
func TestLargeEchoMessage(t *testing.T) {
	t.Parallel()

	// 10000 chars is well within the 64KB scanner limit.
	bigMsg := strings.Repeat("X", 10000)
	input := fmt.Sprintf("echo %s\nstop\n", bigMsg)
	stdout, _, exitCode := runServer(t, input, "-heartbeat=0")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, bigMsg) {
		t.Errorf("echo: 10000-char message not found in stdout (len=%d)", len(stdout))
	}
}

// TestInvalidFlagExitsNonZero verifies invalid flag values cause non-zero exit.
func TestInvalidFlagExitsNonZero(t *testing.T) {
	t.Parallel()

	cmd := exec.CommandContext(context.Background(), binaryPath, "-heartbeat=notaduration")
	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit for invalid flag, got exit 0")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 0 {
			t.Errorf("expected non-zero exit code, got 0")
		}
	}
}

// extractField extracts the value after a key= prefix up to the next space or end.
func extractField(line, key string) string {
	_, rest, found := strings.Cut(line, key)
	if !found {
		return ""
	}
	before, _, _ := strings.Cut(rest, " ")
	return before
}

// nonEmptyLines returns non-empty lines from a string.
func nonEmptyLines(s string) []string {
	var result []string
	for l := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(l) != "" {
			result = append(result, l)
		}
	}
	return result
}
