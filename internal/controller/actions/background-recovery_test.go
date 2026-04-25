package actions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestRunBackgroundTaskRecoversAndLogsPanic(t *testing.T) {
	var logOutput bytes.Buffer
	logger := zerolog.New(&logOutput).Level(zerolog.DebugLevel)
	previousLogger := backgroundTaskLogger.Swap(&logger)
	t.Cleanup(func() {
		backgroundTaskLogger.Store(previousLogger)
	})

	runBackgroundTask("test-job", "tick", map[string]string{
		"server_id": "server-123",
	}, func() {
		panic("boom")
	})

	logText := logOutput.String()
	if !strings.Contains(logText, `"background_job":"test-job"`) {
		t.Fatalf("log output missing background_job field: %s", logText)
	}
	if !strings.Contains(logText, `"phase":"tick"`) {
		t.Fatalf("log output missing phase field: %s", logText)
	}
	if !strings.Contains(logText, `"server_id":"server-123"`) {
		t.Fatalf("log output missing server_id field: %s", logText)
	}
	if !strings.Contains(logText, `"panic_value":"boom"`) {
		t.Fatalf("log output missing panic_value field: %s", logText)
	}
	if !strings.Contains(logText, "Recovered from panic in background job") {
		t.Fatalf("log output missing recovery message: %s", logText)
	}
}
