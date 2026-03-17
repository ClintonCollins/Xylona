package supervisor

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// StartMetricsPoller starts two background goroutines:
//   - Every 3 seconds: collect CPU/memory/thread metrics for all running commands.
//   - Every 30 seconds: calculate disk usage for each command's working directory.
func (inst *Instance) StartMetricsPoller(ctx context.Context) {
	go inst.pollProcessMetrics(ctx)
	go inst.pollDiskMetrics(ctx)
}

func (inst *Instance) pollProcessMetrics(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			inst.RLock()
			commands := make([]*Command, 0, len(inst.runningCommands))
			for _, cmd := range inst.runningCommands {
				commands = append(commands, cmd)
			}
			inst.RUnlock()
			for _, cmd := range commands {
				cmd.collectProcessMetrics()
			}
		}
	}
}

func (inst *Instance) pollDiskMetrics(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			inst.RLock()
			commands := make([]*Command, 0, len(inst.runningCommands))
			for _, cmd := range inst.runningCommands {
				commands = append(commands, cmd)
			}
			inst.RUnlock()
			for _, cmd := range commands {
				dir := cmd.WorkingDir()
				if dir == "" {
					continue
				}
				size, errSize := calculateDirSize(dir)
				if errSize != nil {
					log.Debug().Err(errSize).Str("dir", dir).Msg("Failed to calculate disk usage")
					continue
				}
				cmd.Lock()
				cmd.diskUsageBytes = size
				cmd.Unlock()
			}
		}
	}
}
