package helpers

import (
	"math"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	cpuPercentThreshold       = 0.5
	memoryBytesThreshold      = 1024 * 1024     // 1 MB
	memoryWorkingSetThreshold = 1024 * 1024     // 1 MB
	memoryPercentThreshold    = 0.5
	diskUsageBytesThreshold   = 10 * 1024 * 1024 // 10 MB
	ioRateThreshold           = 1.0
)

// MetricsChanged returns true if the metrics have changed enough to warrant sending an update.
// Returns true if prev is nil (first metrics for this server).
// Returns false if curr is nil (server stopped -- no metrics to send).
func MetricsChanged(prev, curr *xylona.GameServerMetrics) bool {
	if curr == nil {
		return false
	}
	if prev == nil {
		return true
	}

	// Uptime always triggers (monotonically increasing, used for display).
	if curr.UptimeSeconds != prev.UptimeSeconds {
		return true
	}

	// Integer fields -- any change is meaningful.
	if curr.NumberOfThreads != prev.NumberOfThreads {
		return true
	}
	if curr.ConnectionCount != prev.ConnectionCount {
		return true
	}
	if curr.CpuCores != prev.CpuCores {
		return true
	}

	// Float/int fields with thresholds.
	if math.Abs(curr.CpuPercent-prev.CpuPercent) >= cpuPercentThreshold {
		return true
	}
	if absInt64(curr.MemoryBytes-prev.MemoryBytes) >= memoryBytesThreshold {
		return true
	}
	if absInt64(curr.MemoryWorkingSetBytes-prev.MemoryWorkingSetBytes) >= memoryWorkingSetThreshold {
		return true
	}
	if math.Abs(curr.MemoryPercent-prev.MemoryPercent) >= memoryPercentThreshold {
		return true
	}
	if absInt64(curr.DiskUsageBytes-prev.DiskUsageBytes) >= diskUsageBytesThreshold {
		return true
	}
	if math.Abs(curr.IoReadRate-prev.IoReadRate) >= ioRateThreshold {
		return true
	}
	if math.Abs(curr.IoWriteRate-prev.IoWriteRate) >= ioRateThreshold {
		return true
	}

	return false
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
