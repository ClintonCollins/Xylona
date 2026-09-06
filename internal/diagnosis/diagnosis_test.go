package diagnosis

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"unicode/utf8"
)

func TestCaptureClassification(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		output   string
		category string
		matched  bool
	}{
		{"missing executable", &exec.Error{Name: "server", Err: exec.ErrNotFound}, "", CategoryMissingExecutable, false},
		{"missing configuration file is not executable", &os.PathError{Op: "open", Path: "server.cfg", Err: os.ErrNotExist}, "", CategoryUnknown, false},
		{"permission", fmt.Errorf("launch: %w", os.ErrPermission), "", CategoryPermissionDenied, false},
		{"socket", syscall.EADDRINUSE, "", CategoryPortInUse, false},
		{"storage", syscall.ENOSPC, "", CategoryDiskFull, false},
		{"unix bind log", nil, "fatal: bind: address already in use", CategoryPortInUse, true},
		{"windows bind log", nil, "Failed to bind: Only one usage of each socket address is normally permitted", CategoryPortInUse, true},
		{"windows executable log", errors.New("CreateProcess server.exe: The system cannot find the file specified"), "", CategoryMissingExecutable, true},
		{"windows permission log", nil, "open C:/server/config: Access is denied", CategoryPermissionDenied, true},
		{"disk log", nil, "write world.dat: no space left on device", CategoryDiskFull, true},
		{"conflicting logs", nil, "bind: address already in use\nwrite world.dat: no space left on device", CategoryUnknown, false},
		{"structured error wins", os.ErrPermission, "bind: address already in use", CategoryPermissionDenied, false},
		{"generic reference", nil, "permission denied is an error message", CategoryUnknown, false},
		{"chat reference", nil, "[CHAT] player: bind: address already in use", CategoryUnknown, false},
		{"minecraft player chat", nil, "[Server thread/INFO]: <Player> java.net.BindException: Address already in use", CategoryUnknown, false},
		{"documentation reference", nil, "Documentation example: bind: address already in use", CategoryUnknown, false},
		{"resolved error", nil, "Resolved bind: address already in use", CategoryUnknown, false},
		{"unexplained kill", errors.New("signal: killed"), "memory usage 99%", CategoryUnknown, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			report := Capture(tt.err, tt.output)
			if report.Category != tt.category || (report.MatchedEvidence != "") != tt.matched {
				t.Fatalf("Capture() = %+v, want category %q, matched %t", report, tt.category, tt.matched)
			}
		})
	}
}

func TestBoundNormalizesPublicMetadata(t *testing.T) {
	report := Bound(Report{Stage: "private stage", Category: "private category"})
	if report.Stage != StageUnknown || report.Category != CategoryUnknown {
		t.Fatal("unknown metadata was exposed")
	}
}

func TestCaptureBoundsAndRedaction(t *testing.T) {
	secret := "password-with-a-longer-suffix"
	output := strings.Repeat("old line\n", 250) + "\x1b[31mopen config: permission denied\x1b[0m\n" + secret + "\n" + strings.Repeat("界", MaxEvidenceBytes) + "\xff"
	report := Capture(fmt.Errorf("failed: %s", secret), output, "password", secret)
	if strings.Contains(report.Error, secret) || strings.Contains(report.Error, "longer-suffix") || !strings.Contains(report.Error, "[redacted]") {
		t.Fatalf("credential was not fully redacted: %q", report.Error)
	}
	if !report.Truncated || len(report.Evidence) > MaxEvidenceBytes || !utf8.ValidString(report.Evidence) || strings.Count(report.Evidence, "\n") >= MaxEvidenceLines {
		t.Fatalf("invalid bounded evidence: bytes=%d, truncated=%t", len(report.Evidence), report.Truncated)
	}
	if report.Category != CategoryUnknown {
		t.Fatal("classification used evidence that was discarded")
	}
	bounded := Capture(nil, strings.Repeat("old\n", 250)+"\x1b[31mfinal\x1b[0m\n")
	if !strings.HasSuffix(bounded.Evidence, "final") || strings.Contains(bounded.Evidence, "\x1b") || strings.Count(bounded.Evidence, "\n") != MaxEvidenceLines-1 {
		t.Fatal("tail did not retain final lines or remove terminal formatting")
	}
	formatted := Capture(errors.New("abc\x1b[31m123"), "abc\x00123", "abc123")
	if formatted.Error != "[redacted]" || formatted.Evidence != "[redacted]" {
		t.Fatal("normalizing terminal text exposed a credential")
	}
}
