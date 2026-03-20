package supervisor

import (
	"sync"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)


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
		ID:                  serverID,
		RWMutex:             &sync.RWMutex{},
		instanceCtx:         inst.ctx,
		cpuPercent:          cpuPercent,
		memoryRSS:           memoryRSS,
		memoryVMS:           memoryVMS,
		memoryPercent:       memoryPercent,
		cpuCores:            cpuCores,
		numThreads:          numThreads,
		diskUsageBytes:      diskUsageBytes,
		ioReadRate:          ioReadRate,
		ioWriteRate:         ioWriteRate,
		connectionCount:     connectionCount,
		unixStartedAt:       unixStartedAt,
		outputListeners:     make(map[string]chan *xylona.Message),
		outputListenersLock: &sync.RWMutex{},
		statusListeners:     make(map[string]chan *xylona.GameServerStatusUpdate),
		statusListenersLock: &sync.RWMutex{},
	}
	inst.Lock()
	inst.runningCommands[serverID] = cmd
	inst.Unlock()
}

// UpdateCommandMetrics updates the metrics on an existing command for testing.
// This allows RPC-layer tests to simulate metric changes without restarting or
// replacing the command, preserving any registered listeners.
func UpdateCommandMetrics(
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
) {
	cmd, errGet := inst.GetCommandByID(serverID)
	if errGet != nil {
		return
	}
	cmd.Lock()
	defer cmd.Unlock()
	cmd.cpuPercent = cpuPercent
	cmd.memoryRSS = memoryRSS
	cmd.memoryVMS = memoryVMS
	cmd.memoryPercent = memoryPercent
	cmd.cpuCores = cpuCores
	cmd.numThreads = numThreads
	cmd.diskUsageBytes = diskUsageBytes
	cmd.ioReadRate = ioReadRate
	cmd.ioWriteRate = ioWriteRate
	cmd.connectionCount = connectionCount
}

// TriggerStatusNotification fires a status change notification on the command
// identified by serverID, causing all registered status listeners to receive
// the update. This is a test-only helper that allows RPC-layer tests to
// simulate status changes without starting a real OS process.
func TriggerStatusNotification(inst *Instance, serverID string, status xylona.Status) {
	cmd, errGet := inst.GetCommandByID(serverID)
	if errGet != nil {
		return
	}
	cmd.sendJobStatusNotification(status)
}
