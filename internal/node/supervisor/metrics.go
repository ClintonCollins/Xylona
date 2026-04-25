package supervisor

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
)

// collectProcessMetrics reads normalized CPU%, working set (RSS), private memory (VMS),
// memory %, thread count, I/O rates, and connection count for the command's process tree.
// Results are stored under write lock.
func (c *Command) collectProcessMetrics() {
	c.RLock()
	cmd := c.currentCMD
	c.RUnlock()

	if cmd == nil || cmd.Process == nil {
		c.Lock()
		c.cpuPercent = 0
		c.memoryRSS = 0
		c.memoryVMS = 0
		c.memoryPercent = 0
		c.numThreads = 0
		c.ioReadRate = 0
		c.ioWriteRate = 0
		c.connectionCount = 0
		c.Unlock()
		return
	}

	pid := helpers.ClampInt32FromInt(cmd.Process.Pid)
	proc, errNewProcess := process.NewProcess(pid)
	if errNewProcess != nil {
		log.Debug().Err(errNewProcess).Int32("pid", pid).Msg("Failed to create process handle for metrics")
		c.Lock()
		c.cpuPercent = 0
		c.memoryRSS = 0
		c.memoryVMS = 0
		c.memoryPercent = 0
		c.numThreads = 0
		c.ioReadRate = 0
		c.ioWriteRate = 0
		c.connectionCount = 0
		c.Unlock()
		return
	}

	var totalCPU float64
	var totalRSS uint64
	var totalVMS uint64
	var totalMemPct float32
	var totalThreads int32
	var totalIORead uint64
	var totalIOWrite uint64
	var totalConns int32

	numCPU := runtime.NumCPU()

	// Collect metrics for root process.
	if cpuPct, errCPU := proc.CPUPercent(); errCPU == nil {
		totalCPU += cpuPct
	}
	if memInfo, errMem := proc.MemoryInfo(); errMem == nil && memInfo != nil {
		totalRSS += memInfo.RSS
		totalVMS += memInfo.VMS
	}
	if memPct, errMemPct := proc.MemoryPercent(); errMemPct == nil {
		totalMemPct += memPct
	}
	if nThreads, errThreads := proc.NumThreads(); errThreads == nil {
		totalThreads += nThreads
	}
	if ioCounters, errIO := proc.IOCounters(); errIO == nil && ioCounters != nil {
		totalIORead += ioCounters.ReadBytes
		totalIOWrite += ioCounters.WriteBytes
	}
	if conns, errConns := proc.Connections(); errConns == nil {
		totalConns += helpers.ClampInt32FromInt(len(conns))
	}

	// Sum child process metrics for process-tree coverage.
	children, errChildren := proc.Children()
	if errChildren == nil {
		for _, child := range children {
			if childCPU, errChildCPU := child.CPUPercent(); errChildCPU == nil {
				totalCPU += childCPU
			}
			if childMem, errChildMem := child.MemoryInfo(); errChildMem == nil && childMem != nil {
				totalRSS += childMem.RSS
				totalVMS += childMem.VMS
			}
			if childMemPct, errChildMemPct := child.MemoryPercent(); errChildMemPct == nil {
				totalMemPct += childMemPct
			}
			if childThreads, errChildThreads := child.NumThreads(); errChildThreads == nil {
				totalThreads += childThreads
			}
			if childIO, errChildIO := child.IOCounters(); errChildIO == nil && childIO != nil {
				totalIORead += childIO.ReadBytes
				totalIOWrite += childIO.WriteBytes
			}
			if childConns, errChildConns := child.Connections(); errChildConns == nil {
				totalConns += helpers.ClampInt32FromInt(len(childConns))
			}
		}
	}

	// Normalize CPU to 0-100% across all cores.
	if numCPU > 0 {
		totalCPU /= float64(numCPU)
	}

	// Compute I/O rates using delta from last poll.
	now := time.Now()
	c.RLock()
	lastRead := c.lastIORead
	lastWrite := c.lastIOWrite
	lastPoll := c.lastIOPollTime
	c.RUnlock()

	var ioReadRate, ioWriteRate float64
	if !lastPoll.IsZero() {
		elapsed := now.Sub(lastPoll).Seconds()
		if elapsed > 0 {
			if totalIORead >= lastRead {
				ioReadRate = float64(totalIORead-lastRead) / elapsed
			}
			if totalIOWrite >= lastWrite {
				ioWriteRate = float64(totalIOWrite-lastWrite) / elapsed
			}
		}
	}

	c.Lock()
	c.cpuPercent = totalCPU
	c.cpuCores = helpers.ClampInt32FromInt(numCPU)
	c.memoryRSS = totalRSS
	c.memoryVMS = totalVMS
	c.memoryPercent = totalMemPct
	c.numThreads = totalThreads
	c.ioReadRate = ioReadRate
	c.ioWriteRate = ioWriteRate
	c.lastIORead = totalIORead
	c.lastIOWrite = totalIOWrite
	c.lastIOPollTime = now
	c.connectionCount = totalConns
	c.Unlock()
}

// calculateDirSize sums the sizes of all files under directory using WalkDir.
func calculateDirSize(directory string) (uint64, error) {
	if directory == "" {
		return 0, nil
	}
	var total uint64
	errWalk := filepath.WalkDir(directory, func(_ string, d fs.DirEntry, errEntry error) error {
		if errEntry != nil {
			return nil //nolint:nilerr // intentionally skipping unreadable entries to continue the walk
		}
		if !d.IsDir() {
			info, errInfo := d.Info()
			if errInfo == nil {
				total += helpers.ClampUint64FromInt64(info.Size())
			}
		}
		return nil
	})
	if errWalk != nil {
		return 0, fmt.Errorf("supervisor: walk working directory: %w", errWalk)
	}
	return total, nil
}
