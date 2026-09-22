package rpc

import (
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
)

func TestDownsampleNodeMetricsRows(t *testing.T) {
	since := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	until := since.Add(10 * time.Minute)
	rows := make([]*db.NodeMetricsRow, 0, 10)
	for i := range 10 {
		rows = append(rows, &db.NodeMetricsRow{
			RecordedAt:             since.Add(time.Duration(i) * time.Minute),
			CPUPercent:             float64(i * 10),
			MemoryUsedBytes:        int64(i),
			MemoryTotalBytes:       100,
			RunningGameServerCount: i % 2,
		})
	}

	unchanged, interval := downsampleNodeMetricsRows(rows, since, until, 10)
	if len(unchanged) != 10 || interval != 0 {
		t.Fatalf("no reduction expected, got %d rows interval %d", len(unchanged), interval)
	}

	result, interval := downsampleNodeMetricsRows(rows, since, until, 2)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if interval != 300 {
		t.Errorf("interval = %d, want 300", interval)
	}
	if result[0].CPUPercent != 20 || result[1].CPUPercent != 70 {
		t.Errorf("cpu averages = %v, %v; want 20, 70", result[0].CPUPercent, result[1].CPUPercent)
	}
	if !result[1].RecordedAt.Equal(since.Add(5 * time.Minute)) {
		t.Errorf("second bucket start = %v", result[1].RecordedAt)
	}
	if result[0].MemoryTotalBytes != 100 {
		t.Errorf("total bytes not carried: %d", result[0].MemoryTotalBytes)
	}
}
