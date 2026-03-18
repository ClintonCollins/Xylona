package supervisor

import "sync"

// NewCommandWithMetrics registers a synthetic Command with preset metric
// values under serverID in inst. It exists solely to support integration
// tests that need to verify metric propagation through higher-level layers
// (e.g. RPC handlers) without having to start a real OS process.
//
// The command is registered with the given unixStartedAt timestamp so that
// callers can exercise uptime calculations deterministically.
func NewCommandWithMetrics(
	inst *Instance,
	serverID string,
	cpuPercent float64,
	memoryRSS uint64,
	memoryVMS uint64,
	memoryPercent float32,
	cpuCores int32,
	numThreads int32,
	diskUsageBytes uint64,
	ioReadRate float64,
	ioWriteRate float64,
	connectionCount int32,
	unixStartedAt int64,
) {
	cmd := &Command{
		ID:              serverID,
		RWMutex:         &sync.RWMutex{},
		cpuPercent:      cpuPercent,
		memoryRSS:       memoryRSS,
		memoryVMS:       memoryVMS,
		memoryPercent:   memoryPercent,
		cpuCores:        cpuCores,
		numThreads:      numThreads,
		diskUsageBytes:  diskUsageBytes,
		ioReadRate:      ioReadRate,
		ioWriteRate:     ioWriteRate,
		connectionCount: connectionCount,
		unixStartedAt:   unixStartedAt,
	}
	inst.Lock()
	inst.runningCommands[serverID] = cmd
	inst.Unlock()
}
