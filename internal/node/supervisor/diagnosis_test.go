package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestProcessFailureEvidence(t *testing.T) {
	if testing.Short() {
		t.Skip("process integration test")
	}
	executable, errExecutable := os.Executable()
	if errExecutable != nil {
		t.Fatal(errExecutable)
	}
	for _, test := range []struct {
		name      string
		status    xylona.Status
		missing   bool
		clean     bool
		wantStage string
	}{
		{name: "launch", status: xylona.Status_ONLINE, missing: true, wantStage: diagnosis.StageLaunch},
		{name: "runtime", status: xylona.Status_ONLINE, wantStage: diagnosis.StageRuntime},
		{name: "clean exit", status: xylona.Status_ONLINE, clean: true},
		{name: "update excluded", status: xylona.Status_UPDATING},
	} {
		t.Run(test.name, func(t *testing.T) {
			inst, errNew := New(t.Context())
			if errNew != nil {
				t.Fatal(errNew)
			}
			events := make(chan eventbus.StatusChangedEvent, 4)
			inst.SetStatusEventHook(func(event eventbus.StatusChangedEvent) { events <- event })
			attempt := time.Now().Add(-time.Minute).UTC()
			prepared := PreparedCommand{
				ID: "failure-evidence", ExecutionID: "execution-first", AttemptStartedAt: attempt,
				BaseCommand: executable, Args: []string{"-test.run=^TestFailureEvidenceChild$"},
				Status: test.status, RedactValues: []string{"private-argument-token"},
				LaunchEnv: map[string]string{"XYLONA_FAILURE_TEST_CHILD": "active"},
			}
			if test.missing {
				prepared.BaseCommand = filepath.Join(t.TempDir(), "missing")
			}
			if test.clean {
				prepared.LaunchEnv["XYLONA_FAILURE_TEST_CLEAN"] = "yes"
			}
			_, errStart := inst.StartCommand(prepared)
			if (errStart != nil) != test.missing {
				t.Fatalf("start error = %v", errStart)
			}
			var terminal eventbus.StatusChangedEvent
			deadline := time.After(10 * time.Second)
		waitForTerminal:
			for {
				select {
				case event := <-events:
					if event.NewStatus == xylona.Status_OFFLINE.String() {
						terminal = event
						break waitForTerminal
					}
				case <-deadline:
					t.Fatal("terminal status not delivered")
				}
			}
			if test.wantStage == "" {
				if terminal.Failure != nil {
					t.Fatalf("unexpected failure report: %+v", terminal.Failure)
				}
				return
			}
			report := terminal.Failure
			if report == nil || report.Stage != test.wantStage || report.ExecutionID != prepared.ExecutionID || !report.AttemptStartedAt.Equal(attempt) {
				t.Fatalf("failure report = %+v", report)
			}
			if test.missing {
				if report.ExitCode != nil || report.Category != diagnosis.CategoryMissingExecutable {
					t.Fatalf("launch report must preserve structured cause without synthetic exit: %+v", report)
				}
				return
			}
			if report.ExitCode == nil || *report.ExitCode != 23 || !report.EvidenceAvailable || !report.Truncated {
				t.Fatalf("runtime metadata = %+v", report)
			}
			if !strings.Contains(report.Evidence, "final stderr") || strings.Contains(report.Evidence, "private-argument-token") ||
				!strings.Contains(report.Evidence, "[redacted]") || strings.Contains(report.Evidence, "first line") ||
				len(report.Evidence) > diagnosis.MaxEvidenceBytes || len(strings.Split(report.Evidence, "\n")) > diagnosis.MaxEvidenceLines || !utf8.ValidString(report.Evidence) {
				t.Fatalf("invalid terminal evidence: %q", report.Evidence)
			}
			// A newer execution must not alter evidence already published for the old one.
			original := report.Evidence
			prepared.ExecutionID = "execution-second"
			prepared.LaunchEnv["XYLONA_FAILURE_TEST_CLEAN"] = "yes"
			_, errRestart := inst.StartCommand(prepared)
			if errRestart != nil {
				t.Fatal(errRestart)
			}
			if report.Evidence != original || report.ExecutionID != "execution-first" {
				t.Fatal("reusing the command changed published evidence")
			}
		})
	}
}

func TestFailureEvidenceChild(t *testing.T) {
	if os.Getenv("XYLONA_FAILURE_TEST_CHILD") != "active" {
		return
	}
	_, errWrite := fmt.Fprintln(os.Stdout, "first line")
	if errWrite != nil {
		t.Fatal(errWrite)
	}
	for range 250 {
		_, errWrite = fmt.Fprintln(os.Stdout, strings.Repeat("界", 100))
		if errWrite != nil {
			t.Fatal(errWrite)
		}
	}
	_, errWrite = fmt.Fprintln(os.Stderr, "final stderr private-argument-token")
	if errWrite != nil {
		t.Fatal(errWrite)
	}
	if os.Getenv("XYLONA_FAILURE_TEST_CLEAN") == "yes" {
		return
	}
	os.Exit(23)
}

func TestFailureCaptureRejectsPreviousExecutionAndIntentionalStop(t *testing.T) {
	inst, errNew := New(t.Context())
	if errNew != nil {
		t.Fatal(errNew)
	}
	command := inst.initNewCommand(PreparedCommand{ID: "server", Status: xylona.Status_ONLINE}, nil)
	oldContext, cancelOld := context.WithCancel(t.Context())
	defer cancelOld()
	command.captureProcessOutput(oldContext.Done(), "stale output")
	command.captureProcessOutput(command.processCtx.Done(), "current output\n")
	command.captureFailure(command.processGeneration+1, diagnosis.StageRuntime, nil, nil)
	if command.failure != nil || command.failureOutput.String() != "current output\n" {
		t.Fatal("previous execution contaminated failure evidence")
	}
	command.captureFailure(command.processGeneration, diagnosis.StageRuntime, errors.New("process state unavailable"), nil)
	if command.failure == nil || command.failure.ExitCode != nil || command.failure.Category != diagnosis.CategoryUnknown {
		t.Fatalf("unknown exit invented an exit code or cause: %+v", command.failure)
	}
	command.failure = nil
	command.intentionalStop.Store(true)
	command.captureFailure(command.processGeneration, diagnosis.StageRuntime, nil, nil)
	if command.failure != nil {
		t.Fatal("intentional stop produced failure evidence")
	}
	command.processCtxCancel()
}

func TestFailureCaptureRedactsTruncatedRawTail(t *testing.T) {
	inst, errNew := New(t.Context())
	if errNew != nil {
		t.Fatal(errNew)
	}
	secret := strings.Repeat("secret-fragment-", 100)
	command := inst.initNewCommand(PreparedCommand{
		ID: "server", Status: xylona.Status_ONLINE, RedactValues: []string{secret},
	}, nil)
	defer command.processCtxCancel()
	command.captureProcessOutput(command.processCtx.Done(), strings.Repeat("x", maxOutputBufferBytes)+secret)
	command.captureFailure(command.processGeneration, diagnosis.StageRuntime, nil, nil)
	if command.failure == nil || !command.failure.Truncated || strings.Contains(command.failure.Evidence, "secret-fragment") ||
		!strings.Contains(command.failure.Evidence, "[redacted]") {
		t.Fatal("raw tail truncation leaked credential text")
	}
}

func TestFailureOutputStreamRedaction(t *testing.T) {
	inst, errNew := New(t.Context())
	if errNew != nil {
		t.Fatal(errNew)
	}
	const secret = "private-argument-token"
	command := inst.initNewCommand(PreparedCommand{
		ID: "server", Status: xylona.Status_ONLINE, RedactValues: []string{secret},
	}, nil)
	defer command.processCtxCancel()
	writer := command.newFailureOutputWriter(command.processCtx.Done())
	line := strings.Repeat("x", maxConsoleRecordBytes-len(secret)/2) + secret + " final output\n"
	errRead := readConsoleRecords(io.TeeReader(strings.NewReader(line), writer), func(string) bool { return true })
	if errRead != nil {
		t.Fatal(errRead)
	}
	writer.flush(true)
	command.captureFailure(command.processGeneration, diagnosis.StageRuntime, nil, nil)
	if command.failure == nil || strings.Contains(command.failure.Evidence, "private") || strings.Contains(command.failure.Evidence, "argument-token") ||
		!strings.Contains(command.failure.Evidence, "[redacted] final output") {
		t.Fatal("oversized console record split leaked a credential")
	}
	stdout := command.newFailureOutputWriter(command.processCtx.Done())
	stderr := command.newFailureOutputWriter(command.processCtx.Done())
	for _, write := range []struct {
		writer *failureOutputWriter
		text   string
	}{
		{stdout, "private-arg"}, {stderr, "interleaved stderr\n"}, {stdout, "ument-token\n"},
	} {
		_, errWrite := write.writer.Write([]byte(write.text))
		if errWrite != nil {
			t.Fatal(errWrite)
		}
	}
	stdout.flush(true)
	stderr.flush(true)
	command.captureFailure(command.processGeneration, diagnosis.StageRuntime, nil, nil)
	if strings.Contains(command.failure.Evidence, "private-arg") || strings.Contains(command.failure.Evidence, "ument-token") {
		t.Fatal("interleaved byte streams leaked credential fragments")
	}
}
