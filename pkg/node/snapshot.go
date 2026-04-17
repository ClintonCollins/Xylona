package node

import (
	"context"
	"fmt"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/sysinfo"
)

// GetNodeSnapshot returns a point-in-time view of the host (CPU/memory/disk)
// plus per-process metrics for everything the supervisor is currently
// tracking. Errors from sysinfo are surfaced; per-process metrics use the
// supervisor's last-collected values and never fail.
func (n *Node) GetNodeSnapshot(_ context.Context) (*NodeSnapshot, error) {
	systemInfo, errSystem := sysinfo.CollectSystemInfo()
	if errSystem != nil {
		return nil, fmt.Errorf("node: collect system info: %w", errSystem)
	}

	resource, errResource := sysinfo.CollectResourceSnapshot()
	if errResource != nil {
		return nil, fmt.Errorf("node: collect resource snapshot: %w", errResource)
	}

	snapshot := &NodeSnapshot{
		CPUModel:      systemInfo.CPUModel,
		CPUCores:      systemInfo.CPUCores,
		CPUThreads:    systemInfo.CPUThreads,
		TotalMemory:   systemInfo.TotalMemory,
		OS:            systemInfo.OS,
		OSVersion:     systemInfo.OSVersion,
		Architecture:  systemInfo.Architecture,
		XylonaVersion: systemInfo.XylonaVersion,

		CPUPercent:    resource.CPUPercent,
		MemoryUsed:    resource.MemoryUsed,
		MemoryPercent: resource.MemoryPercent,
		DiskUsed:      resource.DiskUsed,
		DiskTotal:     resource.DiskTotal,
		DiskPercent:   resource.DiskPercent,

		Collected: time.Now(),
	}

	if n.supervisor != nil {
		commands := n.supervisor.ListCommands()
		snapshot.Processes = make([]ProcessSnapshot, 0, len(commands))
		for _, cmd := range commands {
			cpuPercent, memoryRSS, memoryVMS, memoryPercent, cpuCores, numThreads, diskUsageBytes, ioReadRate, ioWriteRate, connectionCount := cmd.Metrics()
			snapshot.Processes = append(snapshot.Processes, ProcessSnapshot{
				ID:              cmd.ID,
				Name:            cmd.GameServerName(),
				Status:          cmd.Status().String(),
				UnixStartedAt:   cmd.UnixStartedAt(),
				CPUPercent:      cpuPercent,
				CPUCores:        cpuCores,
				MemoryRSS:       memoryRSS,
				MemoryVMS:       memoryVMS,
				MemoryPercent:   memoryPercent,
				NumThreads:      numThreads,
				DiskUsageBytes:  diskUsageBytes,
				IOReadRate:      ioReadRate,
				IOWriteRate:     ioWriteRate,
				ConnectionCount: connectionCount,
				WorkingDir:      cmd.WorkingDir(),
			})
		}
	}

	return snapshot, nil
}
