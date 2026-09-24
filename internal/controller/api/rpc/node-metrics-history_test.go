package rpc

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
)

func TestDownsampleNodeMetricsRows(t *testing.T) {
	since := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	until := since.Add(10 * time.Minute)
	rows := make([]*db.NodeMetricsRow, 0, 10)
	for i := range 10 {
		rows = append(rows, &db.NodeMetricsRow{
			RecordedAt:             since.Add(time.Duration(i) * time.Minute),
			CPUPercent:             sql.NullFloat64{Float64: float64(i * 10), Valid: true},
			MemoryUsedBytes:        sql.NullInt64{Int64: int64(i), Valid: true},
			MemoryTotalBytes:       sql.NullInt64{Int64: 100, Valid: true},
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
	if result[0].CPUPercent.Float64 != 20 || result[1].CPUPercent.Float64 != 70 {
		t.Errorf("cpu averages = %v, %v; want 20, 70", result[0].CPUPercent, result[1].CPUPercent)
	}
	if !result[1].RecordedAt.Equal(since.Add(5 * time.Minute)) {
		t.Errorf("second bucket start = %v", result[1].RecordedAt)
	}
	if result[0].MemoryTotalBytes.Int64 != 100 {
		t.Errorf("total bytes not carried: %v", result[0].MemoryTotalBytes)
	}
}

// A failed reading must neither drag a bucket's average down nor, when every
// sample failed, surface as a 0 on the chart.
func TestAggregateNodeMetricsBucketIgnoresUnavailableReadings(t *testing.T) {
	valid := func(value float64) sql.NullFloat64 { return sql.NullFloat64{Float64: value, Valid: true} }
	tests := []struct {
		name            string
		cpu             []sql.NullFloat64
		wantCPU         sql.NullFloat64
		wantUnavailable bool
	}{
		{name: "all valid", cpu: []sql.NullFloat64{valid(20), valid(40)}, wantCPU: valid(30)},
		{name: "unavailable samples are skipped", cpu: []sql.NullFloat64{valid(20), {}, {}}, wantCPU: valid(20)},
		{name: "no valid sample is unavailable", cpu: []sql.NullFloat64{{}, {}}, wantUnavailable: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := make([]*db.NodeMetricsRow, 0, len(test.cpu))
			for _, cpu := range test.cpu {
				rows = append(rows, &db.NodeMetricsRow{
					CPUPercent:      cpu,
					MemoryPercent:   valid(50),
					MemoryUsedBytes: sql.NullInt64{Int64: 512, Valid: true},
					DiskPercent:     cpu,
					DiskUsedBytes:   sql.NullInt64{Int64: int64(cpu.Float64), Valid: cpu.Valid},
				})
			}

			bucket := aggregateNodeMetricsBucket(rows, time.Time{})
			if bucket.CPUPercent != test.wantCPU {
				t.Errorf("bucket cpu = %v, want %v", bucket.CPUPercent, test.wantCPU)
			}
			point := nodeMetricsHistoryPointProto(bucket)
			if point.GetCpuUnavailable() != test.wantUnavailable || point.GetDiskUnavailable() != test.wantUnavailable {
				t.Errorf("point (cpu, disk) unavailable = (%t, %t), want %t",
					point.GetCpuUnavailable(), point.GetDiskUnavailable(), test.wantUnavailable)
			}
			if point.GetMemoryUnavailable() || point.GetMemoryUsedBytes() != 512 {
				t.Errorf("memory point = (unavailable %t, %d bytes), want available 512",
					point.GetMemoryUnavailable(), point.GetMemoryUsedBytes())
			}
		})
	}
}

// The snapshot endpoint and the dashboard overview share this mapping.
func TestNodeResourceSnapshotProtoFlagsUnavailableReadings(t *testing.T) {
	tests := []struct {
		name                          string
		snapshot                      node.NodeSnapshot
		wantCPU, wantMemory, wantDisk bool
	}{
		{
			name:     "every reading taken",
			snapshot: node.NodeSnapshot{CPUPercent: 12, CPUValid: true, MemoryValid: true, DiskValid: true},
		},
		{
			name:     "failed readings are flagged",
			snapshot: node.NodeSnapshot{MemoryValid: true},
			wantCPU:  true, wantDisk: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := nodeResourceSnapshotProto(&test.snapshot, nil, 0)
			if got.GetCpuUnavailable() != test.wantCPU || got.GetMemoryUnavailable() != test.wantMemory || got.GetDiskUnavailable() != test.wantDisk {
				t.Fatalf("unavailable = (cpu %t, memory %t, disk %t), want (%t, %t, %t)",
					got.GetCpuUnavailable(), got.GetMemoryUnavailable(), got.GetDiskUnavailable(),
					test.wantCPU, test.wantMemory, test.wantDisk)
			}
		})
	}
}
