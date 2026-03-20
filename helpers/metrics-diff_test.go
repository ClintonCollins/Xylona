package helpers

import (
	"testing"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestMetricsChanged(t *testing.T) {
	tests := []struct {
		name     string
		prev     *xylona.GameServerMetrics
		curr     *xylona.GameServerMetrics
		expected bool
	}{
		{
			name:     "nil previous returns true",
			prev:     nil,
			curr:     &xylona.GameServerMetrics{CpuPercent: 10.0},
			expected: true,
		},
		{
			name:     "nil current returns false",
			prev:     &xylona.GameServerMetrics{CpuPercent: 10.0},
			curr:     nil,
			expected: false,
		},
		{
			name:     "small CPU fluctuation suppressed",
			prev:     &xylona.GameServerMetrics{CpuPercent: 10.0, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{CpuPercent: 10.3, UptimeSeconds: 100},
			expected: false,
		},
		{
			name:     "meaningful CPU change passes",
			prev:     &xylona.GameServerMetrics{CpuPercent: 10.0, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{CpuPercent: 10.6, UptimeSeconds: 100},
			expected: true,
		},
		{
			name:     "thread count change always passes",
			prev:     &xylona.GameServerMetrics{NumberOfThreads: 5, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{NumberOfThreads: 6, UptimeSeconds: 100},
			expected: true,
		},
		{
			name:     "connection count change always passes",
			prev:     &xylona.GameServerMetrics{ConnectionCount: 10, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{ConnectionCount: 11, UptimeSeconds: 100},
			expected: true,
		},
		{
			name:     "uptime change always triggers",
			prev:     &xylona.GameServerMetrics{UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{UptimeSeconds: 103},
			expected: true,
		},
		{
			name:     "memory below threshold suppressed",
			prev:     &xylona.GameServerMetrics{MemoryBytes: 100 * 1024 * 1024, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{MemoryBytes: 100*1024*1024 + 500*1024, UptimeSeconds: 100},
			expected: false,
		},
		{
			name:     "memory above threshold passes",
			prev:     &xylona.GameServerMetrics{MemoryBytes: 100 * 1024 * 1024, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{MemoryBytes: 102 * 1024 * 1024, UptimeSeconds: 100},
			expected: true,
		},
		{
			name:     "disk below threshold suppressed",
			prev:     &xylona.GameServerMetrics{DiskUsageBytes: 1024 * 1024 * 1024, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{DiskUsageBytes: 1024*1024*1024 + 5*1024*1024, UptimeSeconds: 100},
			expected: false,
		},
		{
			name:     "disk above threshold passes",
			prev:     &xylona.GameServerMetrics{DiskUsageBytes: 1024 * 1024 * 1024, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{DiskUsageBytes: 1024*1024*1024 + 11*1024*1024, UptimeSeconds: 100},
			expected: true,
		},
		{
			name:     "cpu cores change always passes",
			prev:     &xylona.GameServerMetrics{CpuCores: 4, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{CpuCores: 8, UptimeSeconds: 100},
			expected: true,
		},
		{
			name:     "io read rate above threshold passes",
			prev:     &xylona.GameServerMetrics{IoReadRate: 5.0, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{IoReadRate: 6.1, UptimeSeconds: 100},
			expected: true,
		},
		{
			name:     "io read rate below threshold suppressed",
			prev:     &xylona.GameServerMetrics{IoReadRate: 5.0, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{IoReadRate: 5.8, UptimeSeconds: 100},
			expected: false,
		},
		{
			name:     "memory percent above threshold passes",
			prev:     &xylona.GameServerMetrics{MemoryPercent: 50.0, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{MemoryPercent: 50.6, UptimeSeconds: 100},
			expected: true,
		},
		{
			name:     "working set above threshold passes",
			prev:     &xylona.GameServerMetrics{MemoryWorkingSetBytes: 100 * 1024 * 1024, UptimeSeconds: 100},
			curr:     &xylona.GameServerMetrics{MemoryWorkingSetBytes: 102 * 1024 * 1024, UptimeSeconds: 100},
			expected: true,
		},
		{
			name: "identical metrics returns false",
			prev: &xylona.GameServerMetrics{
				CpuPercent: 10.0, MemoryBytes: 100, NumberOfThreads: 5,
				ConnectionCount: 3, UptimeSeconds: 100, CpuCores: 4,
			},
			curr: &xylona.GameServerMetrics{
				CpuPercent: 10.0, MemoryBytes: 100, NumberOfThreads: 5,
				ConnectionCount: 3, UptimeSeconds: 100, CpuCores: 4,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MetricsChanged(tt.prev, tt.curr)
			if got != tt.expected {
				t.Errorf("MetricsChanged() = %v, want %v", got, tt.expected)
			}
		})
	}
}
