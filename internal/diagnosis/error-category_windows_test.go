package diagnosis

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestCaptureWindowsErrors(t *testing.T) {
	for _, tt := range []struct {
		err      error
		category string
	}{
		{windows.ERROR_DISK_FULL, CategoryDiskFull},
		{windows.ERROR_HANDLE_DISK_FULL, CategoryDiskFull},
		{windows.WSAEADDRINUSE, CategoryPortInUse},
	} {
		report := Capture(tt.err, "")
		if report.Category != tt.category || report.MatchedEvidence != "" {
			t.Fatalf("native error %v classified as %+v", tt.err, report)
		}
	}
}
