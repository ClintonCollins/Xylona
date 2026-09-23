package supervisor

import (
	"context"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// printThenWaitArgs prints line and then stays running for a few seconds.
func printThenWaitArgs(line string) (string, []string) {
	if runtime.GOOS == "windows" {
		return shellCommandArgs("echo " + line + " & ping -n 4 127.0.0.1 >NUL")
	}
	return shellCommandArgs("echo '" + line + "'; sleep 3")
}

func TestReadinessMovesStartFromPreStartToOnline(t *testing.T) {
	previousInterval := readyProbeInterval
	readyProbeInterval = 20 * time.Millisecond
	t.Cleanup(func() { readyProbeInterval = previousInterval })

	tests := []struct {
		name       string
		line       string
		readiness  *Readiness
		wantOnline bool
		wantNotice string
	}{
		{
			name:       "ready log line",
			line:       "Done (1.5s)! For help",
			readiness:  &Readiness{LogPattern: regexp.MustCompile(`Done \([0-9.,]+s\)!`)},
			wantOnline: true,
			wantNotice: "Server is ready for players.",
		},
		{
			name:       "answered query",
			line:       "loading world",
			readiness:  &Readiness{Probe: func(context.Context) bool { return true }},
			wantOnline: true,
			wantNotice: "Server is ready for players.",
		},
		{
			name:       "timeout fallback",
			line:       "loading world",
			readiness:  &Readiness{Timeout: 50 * time.Millisecond},
			wantOnline: true,
			wantNotice: "No readiness signal after 50ms",
		},
		{
			name:      "no signal before exit",
			line:      "loading world",
			readiness: &Readiness{LogPattern: regexp.MustCompile(`never printed`)},
		},
		{
			name:       "no readiness keeps online at spawn",
			line:       "loading world",
			wantOnline: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inst, errNew := New(t.Context())
			if errNew != nil {
				t.Fatalf("New() error = %v", errNew)
			}
			var eventsMutex sync.Mutex
			var transitions []string
			inst.SetStatusEventHook(func(event eventbus.StatusChangedEvent) {
				eventsMutex.Lock()
				transitions = append(transitions, event.OldStatus+">"+event.NewStatus)
				eventsMutex.Unlock()
			})
			done := make(chan struct{})
			baseCommand, args := printThenWaitArgs(test.line)
			command, errStart := inst.StartCommand(PreparedCommand{
				ID:               "readiness-" + strings.ReplaceAll(test.name, " ", "-"),
				BaseCommand:      baseCommand,
				Args:             args,
				Status:           xylona.Status_ONLINE,
				Readiness:        test.readiness,
				CallbackFunction: func(*Command) { close(done) },
			})
			if errStart != nil {
				t.Fatalf("StartCommand() error = %v", errStart)
			}
			select {
			case <-done:
			case <-time.After(15 * time.Second):
				command.Stop("")
				t.Fatal("timed out waiting for the process to exit")
			}

			eventsMutex.Lock()
			got := strings.Join(transitions, ",")
			eventsMutex.Unlock()
			want := "OFFLINE>PRE_START,PRE_START>OFFLINE"
			switch {
			case test.readiness == nil:
				want = "OFFLINE>ONLINE,ONLINE>OFFLINE"
			case test.wantOnline:
				want = "OFFLINE>PRE_START,PRE_START>ONLINE,ONLINE>OFFLINE"
			}
			if got != want {
				t.Fatalf("transitions = %q, want %q", got, want)
			}
			if test.wantNotice != "" && !strings.Contains(command.GetOutputBuffer(), test.wantNotice) {
				t.Fatalf("console output = %q, want notice %q", command.GetOutputBuffer(), test.wantNotice)
			}
		})
	}
}

func TestMarkReadyIgnoresStaleExecution(t *testing.T) {
	command := newTestCommand(t)
	command.executionMutex = &sync.Mutex{}
	command.processGeneration = 2
	command.status = xylona.Status_PRE_START

	command.markReady(1, "stale")
	if command.Status() != xylona.Status_PRE_START {
		t.Fatalf("status after stale markReady = %v, want PRE_START", command.Status())
	}
	command.markReady(2, "ready")
	if command.Status() != xylona.Status_ONLINE {
		t.Fatalf("status after markReady = %v, want ONLINE", command.Status())
	}
}
