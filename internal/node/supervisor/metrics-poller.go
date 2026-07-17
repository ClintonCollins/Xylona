package supervisor

import (
	"context"
	"sort"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v4/disk"
)

const (
	processMetricsPollInterval = 3 * time.Second
	diskMetricsPollInterval    = 30 * time.Second
	diskMetricsScansPerTick    = 2
	minProcessMetricsInterval  = time.Second
	maxProcessMetricsInterval  = time.Minute
	minDiskMetricsInterval     = time.Second
	maxDiskMetricsInterval     = time.Hour
	maxDiskMetricsScansPerTick = 64
)

// MetricsPollerOptions controls node-local metric collection cost. Callers can
// source these values from node configuration or environment without changing
// the controller or wire protocol.
type MetricsPollerOptions struct {
	ProcessInterval  time.Duration
	DiskInterval     time.Duration
	DiskScansPerTick int
}

// DefaultMetricsPollerOptions returns the bounded collection defaults.
func DefaultMetricsPollerOptions() MetricsPollerOptions {
	return MetricsPollerOptions{
		ProcessInterval:  processMetricsPollInterval,
		DiskInterval:     diskMetricsPollInterval,
		DiskScansPerTick: diskMetricsScansPerTick,
	}
}

// StartMetricsPoller starts two background goroutines:
//   - Every 3 seconds: collect CPU/memory/thread metrics for all running commands.
//   - Every 30 seconds: scan a bounded round-robin subset of working directories.
func (inst *Instance) StartMetricsPoller(ctx context.Context) {
	inst.StartMetricsPollerWithOptions(ctx, DefaultMetricsPollerOptions())
}

// StartMetricsPollerWithOptions starts bounded metrics collection with
// node-local overrides. Invalid values fall back to the default values.
func (inst *Instance) StartMetricsPollerWithOptions(ctx context.Context, options MetricsPollerOptions) {
	options = normalizeMetricsPollerOptions(options)
	go inst.pollProcessMetricsWithInterval(ctx, options.ProcessInterval)
	go inst.pollDiskMetricsWithOptions(ctx, options.DiskInterval, options.DiskScansPerTick)
}

func (inst *Instance) pollProcessMetrics(ctx context.Context) {
	inst.pollProcessMetricsWithInterval(ctx, processMetricsPollInterval)
}

func (inst *Instance) pollProcessMetricsWithInterval(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
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
	inst.pollDiskMetricsWithOptions(ctx, diskMetricsPollInterval, diskMetricsScansPerTick)
}

func (inst *Instance) pollDiskMetricsWithOptions(ctx context.Context, interval time.Duration, scansPerTick int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var nextCommandIndex int
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
			sort.Slice(commands, func(i, j int) bool {
				return commands[i].ID < commands[j].ID
			})
			selected, updatedIndex := selectDiskMetricScanTargets(commands, nextCommandIndex, scansPerTick)
			nextCommandIndex = updatedIndex
			for _, cmd := range selected {
				cmd.collectDiskMetrics()
			}
		}
	}
}

func normalizeMetricsPollerOptions(options MetricsPollerOptions) MetricsPollerOptions {
	defaults := DefaultMetricsPollerOptions()
	if options.ProcessInterval <= 0 {
		options.ProcessInterval = defaults.ProcessInterval
	}
	options.ProcessInterval = min(max(options.ProcessInterval, minProcessMetricsInterval), maxProcessMetricsInterval)
	if options.DiskInterval <= 0 {
		options.DiskInterval = defaults.DiskInterval
	}
	options.DiskInterval = min(max(options.DiskInterval, minDiskMetricsInterval), maxDiskMetricsInterval)
	if options.DiskScansPerTick <= 0 {
		options.DiskScansPerTick = defaults.DiskScansPerTick
	}
	options.DiskScansPerTick = min(options.DiskScansPerTick, maxDiskMetricsScansPerTick)
	return options
}

func selectDiskMetricScanTargets(commands []*Command, nextIndex, limit int) ([]*Command, int) {
	if len(commands) == 0 || limit <= 0 {
		return nil, 0
	}
	if nextIndex < 0 || nextIndex >= len(commands) {
		nextIndex = 0
	}
	count := min(limit, len(commands))
	selected := make([]*Command, 0, count)
	for offset := range count {
		index := (nextIndex + offset) % len(commands)
		selected = append(selected, commands[index])
	}
	return selected, (nextIndex + count) % len(commands)
}

func (c *Command) collectDiskMetrics() {
	directory := c.WorkingDir()
	if directory == "" {
		c.Lock()
		c.diskValid = false
		c.Unlock()
		return
	}

	usageBytes, errSize := calculateDirSize(directory)
	if errSize != nil {
		log.Debug().Err(errSize).Str("dir", directory).Msg("Failed to calculate disk usage")
		c.Lock()
		c.diskValid = false
		c.Unlock()
		return
	}
	volume, errVolume := disk.Usage(directory)
	if errVolume != nil {
		log.Debug().Err(errVolume).Str("dir", directory).Msg("Failed to collect working-directory volume metrics")
		c.Lock()
		c.diskValid = false
		c.Unlock()
		return
	}

	c.Lock()
	c.diskUsageBytes = usageBytes
	c.diskTotalBytes = volume.Total
	c.diskFreeBytes = volume.Free
	c.diskPercent = volume.UsedPercent
	c.diskMeasuredAt = time.Now().UTC()
	c.diskValid = true
	c.Unlock()
}
