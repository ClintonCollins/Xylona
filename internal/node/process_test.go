package node

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestStartProcessRejectsInvalidLaunchEnvironmentAtNodeBoundary(t *testing.T) {
	supervisorInst, errNew := supervisor.New(context.Background())
	if errNew != nil {
		t.Fatalf("supervisor.New() error = %v", errNew)
	}
	nodeInst := New(context.Background(), supervisorInst, nil)

	secretValue := "must-not-appear"
	_, errStart := nodeInst.StartProcess(ProcessConfig{
		ID:          "invalid-env",
		BaseCommand: "unused",
		LaunchEnv: map[string]string{
			"JDK_JAVA_OPTIONS": secretValue,
		},
	}, xylona.Status_ONLINE)
	if errStart == nil {
		t.Fatal("StartProcess() error = nil, want launch environment validation error")
	}
	validationError := &launchenv.ValidationError{}
	if !errors.As(errStart, &validationError) {
		t.Fatalf("StartProcess() error = %T %v, want *launchenv.ValidationError", errStart, errStart)
	}
	if strings.Contains(errStart.Error(), secretValue) {
		t.Fatalf("StartProcess() error leaked launch environment value: %v", errStart)
	}
	failure, isFailure := errors.AsType[*StartFailureError](errStart)
	if !isFailure || failure.Report.Stage != diagnosis.StagePreStart || failure.Report.EvidenceAvailable || failure.Report.ExecutionID == "" || failure.Report.AttemptStartedAt.IsZero() {
		t.Fatalf("pre-start report = %+v", failure)
	}
}

func TestTranslateSupervisorConsoleInputError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      error
		want       error
		wantDetail string
		notWant    error
	}{
		{
			name:       "preserves sanitized command rejection",
			input:      &supervisor.ConsoleInputRejectedError{Detail: "Palworld API returned 401 Unauthorized"},
			want:       ErrConsoleInputRejected,
			wantDetail: "Palworld API returned 401 Unauthorized",
			notWant:    ErrConsoleInputUnavailable,
		},
		{
			name:    "maps reconnectable transport failure",
			input:   errors.Join(supervisor.ErrConsoleInputUnavailable, errors.New("connection refused")),
			want:    ErrConsoleInputUnavailable,
			notWant: ErrConsoleInputRejected,
		},
		{
			name:    "maps internal client timeout as unavailable",
			input:   errors.Join(supervisor.ErrConsoleInputUnavailable, context.DeadlineExceeded),
			want:    ErrConsoleInputUnavailable,
			notWant: ErrConsoleInputRejected,
		},
		{
			name:    "preserves caller cancellation",
			input:   context.Canceled,
			want:    context.Canceled,
			notWant: ErrConsoleInputUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			errTranslated := translateSupervisorConsoleInputError(tc.input)
			if !errors.Is(errTranslated, tc.want) {
				t.Fatalf("translateSupervisorConsoleInputError() error = %v, want %v", errTranslated, tc.want)
			}
			if tc.notWant != nil && errors.Is(errTranslated, tc.notWant) {
				t.Fatalf("translateSupervisorConsoleInputError() error = %v, must not match %v", errTranslated, tc.notWant)
			}
			if tc.wantDetail != "" {
				var rejectedError *ConsoleInputRejectedError
				if !errors.As(errTranslated, &rejectedError) || rejectedError.Detail() != tc.wantDetail {
					t.Fatalf("translateSupervisorConsoleInputError() error = %v, want detail %q", errTranslated, tc.wantDetail)
				}
			}
		})
	}
}

func TestConsoleInputRejectedErrorBoundsRemoteDetail(t *testing.T) {
	t.Parallel()

	rejectedError := NewConsoleInputRejectedError("first\nsecond " + strings.Repeat("x", 1024))
	if strings.Contains(rejectedError.Detail(), "\n") {
		t.Fatalf("Detail() = %q, want a single line", rejectedError.Detail())
	}
	if len([]rune(rejectedError.Detail())) > maxConsoleInputRejectedDetailRunes {
		t.Fatalf(
			"Detail() runes = %d, want at most %d",
			len([]rune(rejectedError.Detail())),
			maxConsoleInputRejectedDetailRunes,
		)
	}
	if !errors.Is(rejectedError, ErrConsoleInputRejected) {
		t.Fatalf("NewConsoleInputRejectedError() = %v, want ErrConsoleInputRejected", rejectedError)
	}
}
