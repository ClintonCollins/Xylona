package node

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/sysinfo"
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

	// Resolve the node's own default install root. Logged-and-zeroed on error
	// so a misconfigured node (missing $HOME on Linux, missing %USERPROFILE%
	// on Windows) still reports a snapshot — the controller falls back to its
	// own default and surfaces a user-visible error only when the empty
	// install path actually matters.
	installPath, errInstallPath := DefaultInstallPath()
	if errInstallPath != nil {
		log.Warn().Err(errInstallPath).Msg("node: could not resolve default install path for snapshot")
	} else {
		snapshot.DefaultInstallPath = installPath
	}

	if n.supervisor != nil {
		commands := n.supervisor.ListCommands()
		snapshot.Processes = make([]ProcessSnapshot, 0, len(commands))
		for _, cmd := range commands {
			cpuPercent, cpuValid, metricsValid, memoryRSS, memoryVMS, memoryPercent, cpuCores, numThreads, diskUsageBytes, diskTotalBytes, diskFreeBytes, diskPercent, diskMeasuredAt, diskValid, ioValid, ioReadRate, ioWriteRate, connectionCount, connectionCountValid := cmd.Metrics()
			lifecycle := cmd.Lifecycle()
			snapshot.Processes = append(snapshot.Processes, ProcessSnapshot{
				ID:                   cmd.ID,
				ExecutionID:          lifecycle.ExecutionID,
				Name:                 cmd.GameServerName(),
				Status:               cmd.Status().String(),
				PreviousStatus:       lifecycle.PreviousStatus.String(),
				TransitionSequence:   lifecycle.TransitionSequence,
				IntentionalStop:      lifecycle.IntentionalStop,
				ExitCode:             lifecycle.ExitCode,
				ExitCodeKnown:        lifecycle.ExitCodeKnown,
				UnixStartedAt:        cmd.UnixStartedAt(),
				CPUPercent:           cpuPercent,
				CPUValid:             cpuValid,
				MetricsValid:         metricsValid,
				CPUCores:             cpuCores,
				MemoryRSS:            memoryRSS,
				MemoryVMS:            memoryVMS,
				MemoryPercent:        memoryPercent,
				NumThreads:           numThreads,
				DiskUsageBytes:       diskUsageBytes,
				DiskTotalBytes:       diskTotalBytes,
				DiskFreeBytes:        diskFreeBytes,
				DiskPercent:          diskPercent,
				DiskMeasuredAt:       diskMeasuredAt,
				DiskValid:            diskValid,
				IOValid:              ioValid,
				IOReadRate:           ioReadRate,
				IOWriteRate:          ioWriteRate,
				ConnectionCount:      connectionCount,
				ConnectionCountValid: connectionCountValid,
				WorkingDir:           cmd.WorkingDir(),
			})
		}
	}

	return snapshot, nil
}
