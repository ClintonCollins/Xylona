package helpers

import (
	"math"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	cpuPercentThreshold       = 0.5
	memoryBytesThreshold      = 1024 * 1024 // 1 MB
	memoryWorkingSetThreshold = 1024 * 1024 // 1 MB
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
	if curr.GetUptimeSeconds() != prev.GetUptimeSeconds() {
		return true
	}

	// Integer fields -- any change is meaningful.
	if curr.GetNumberOfThreads() != prev.GetNumberOfThreads() {
		return true
	}
	if curr.GetConnectionCount() != prev.GetConnectionCount() {
		return true
	}
	if curr.GetCpuCores() != prev.GetCpuCores() {
		return true
	}

	// Float/int fields with thresholds.
	if math.Abs(curr.GetCpuPercent()-prev.GetCpuPercent()) >= cpuPercentThreshold {
		return true
	}
	if absInt64(curr.GetMemoryBytes()-prev.GetMemoryBytes()) >= memoryBytesThreshold {
		return true
	}
	if absInt64(curr.GetMemoryWorkingSetBytes()-prev.GetMemoryWorkingSetBytes()) >= memoryWorkingSetThreshold {
		return true
	}
	if math.Abs(curr.GetMemoryPercent()-prev.GetMemoryPercent()) >= memoryPercentThreshold {
		return true
	}
	if absInt64(curr.GetDiskUsageBytes()-prev.GetDiskUsageBytes()) >= diskUsageBytesThreshold {
		return true
	}
	if math.Abs(curr.GetIoReadRate()-prev.GetIoReadRate()) >= ioRateThreshold {
		return true
	}
	if math.Abs(curr.GetIoWriteRate()-prev.GetIoWriteRate()) >= ioRateThreshold {
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
