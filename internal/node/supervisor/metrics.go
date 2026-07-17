package supervisor

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
)

// collectProcessMetrics reads interval CPU%, working set (RSS), private memory (VMS),
// memory %, thread count, I/O rates, and connection count for the command's process tree.
// Results are stored under write lock.
func (c *Command) collectProcessMetrics() {
	c.RLock()
	var managedProcess *os.Process
	if c.currentPTYCMD != nil {
		managedProcess = c.currentPTYCMD.Process
	} else if c.currentCMD != nil {
		managedProcess = c.currentCMD.Process
	}
	c.RUnlock()

	if managedProcess == nil {
		c.clearProcessMetrics()
		return
	}

	pid := helpers.ClampInt32FromInt(managedProcess.Pid)
	proc, errNewProcess := process.NewProcess(pid)
	if errNewProcess != nil {
		log.Debug().Err(errNewProcess).Int32("pid", pid).Msg("Failed to create process handle for metrics")
		c.clearProcessMetrics()
		return
	}

	numCPU := runtime.NumCPU()
	processes, treeComplete := collectProcessTree(proc)
	samples := make([]processMetricSample, 0, len(processes))
	for _, processInTree := range processes {
		sample := processMetricSample{pid: processInTree.Pid}
		times, errTimes := processInTree.Times()
		if errTimes == nil && times != nil {
			sample.cpuSeconds = times.User + times.System
			sample.cpuReadOK = true
		}
		memInfo, errMem := processInTree.MemoryInfo()
		if errMem == nil && memInfo != nil {
			sample.memoryRSS = memInfo.RSS
			sample.memoryVMS = memInfo.VMS
			sample.memoryInfoReadOK = true
		}
		memPct, errMemPct := processInTree.MemoryPercent()
		if errMemPct == nil {
			sample.memoryPercent = memPct
			sample.memoryPercentReadOK = true
		}
		nThreads, errThreads := processInTree.NumThreads()
		if errThreads == nil {
			sample.numThreads = nThreads
		}
		ioCounters, errIO := processInTree.IOCounters()
		if errIO == nil && ioCounters != nil {
			sample.ioRead = ioCounters.ReadBytes
			sample.ioWrite = ioCounters.WriteBytes
			sample.ioReadOK = true
		}
		conns, errConns := processInTree.Connections()
		if errConns == nil {
			sample.connections = helpers.ClampInt32FromInt(len(conns))
			sample.connectionsReadOK = true
		}
		samples = append(samples, sample)
	}

	// Compute I/O rates using delta from last poll.
	now := time.Now()
	c.RLock()
	lastIOByPID := maps.Clone(c.lastIOByPID)
	lastPoll := c.lastIOPollTime
	lastCPUSecondsByPID := maps.Clone(c.lastCPUSecondsByPID)
	lastCPUPoll := c.lastCPUPollTime
	c.RUnlock()
	aggregate := aggregateProcessMetricSamples(samples, lastCPUSecondsByPID, lastIOByPID, treeComplete)

	ioReadRate, ioWriteRate, ioValid := intervalIORates(aggregate.ioReadDelta, aggregate.ioWriteDelta, aggregate.ioDeltaValid, now, lastPoll)
	cpuPercent, cpuValid := intervalCPUPercent(aggregate.cpuDeltaSeconds, aggregate.cpuDeltaValid, now, lastCPUPoll, numCPU)

	c.Lock()
	c.cpuPercent = cpuPercent
	c.cpuValid = cpuValid
	c.metricsValid = aggregate.memoryValid
	c.cpuCores = helpers.ClampInt32FromInt(numCPU)
	c.memoryRSS = aggregate.memoryRSS
	c.memoryVMS = aggregate.memoryVMS
	c.memoryPercent = aggregate.memoryPercent
	c.numThreads = aggregate.numThreads
	c.ioReadRate = ioReadRate
	c.ioWriteRate = ioWriteRate
	c.ioValid = ioValid
	c.lastIOByPID = aggregate.ioByPID
	c.lastIOPollTime = now
	c.lastCPUSecondsByPID = aggregate.cpuSecondsByPID
	c.lastCPUPollTime = now
	c.connectionCount = aggregate.connections
	c.connectionCountValid = aggregate.connectionsValid
	c.Unlock()
}

func (c *Command) clearProcessMetrics() {
	c.Lock()
	c.cpuPercent = 0
	c.cpuValid = false
	c.metricsValid = false
	c.memoryRSS = 0
	c.memoryVMS = 0
	c.memoryPercent = 0
	c.numThreads = 0
	c.ioReadRate = 0
	c.ioWriteRate = 0
	c.ioValid = false
	c.lastIOByPID = nil
	c.lastIOPollTime = time.Time{}
	c.lastCPUSecondsByPID = nil
	c.lastCPUPollTime = time.Time{}
	c.connectionCount = 0
	c.connectionCountValid = false
	c.Unlock()
}

type processMetricSample struct {
	pid                 int32
	cpuSeconds          float64
	cpuReadOK           bool
	memoryRSS           uint64
	memoryVMS           uint64
	memoryPercent       float32
	memoryInfoReadOK    bool
	memoryPercentReadOK bool
	numThreads          int32
	ioRead              uint64
	ioWrite             uint64
	ioReadOK            bool
	connections         int32
	connectionsReadOK   bool
}

type processIOCounters struct {
	read  uint64
	write uint64
}

type processMetricAggregate struct {
	cpuSecondsByPID  map[int32]float64
	cpuDeltaSeconds  float64
	cpuDeltaValid    bool
	memoryRSS        uint64
	memoryVMS        uint64
	memoryPercent    float32
	memoryValid      bool
	numThreads       int32
	ioByPID          map[int32]processIOCounters
	ioReadDelta      uint64
	ioWriteDelta     uint64
	ioDeltaValid     bool
	connections      int32
	connectionsValid bool
}

func aggregateProcessMetricSamples(samples []processMetricSample, previousCPUSecondsByPID map[int32]float64, previousIOByPID map[int32]processIOCounters, treeComplete bool) processMetricAggregate {
	aggregate := processMetricAggregate{
		cpuSecondsByPID:  make(map[int32]float64, len(samples)),
		cpuDeltaValid:    treeComplete && len(samples) > 0,
		memoryValid:      treeComplete && len(samples) > 0,
		ioByPID:          make(map[int32]processIOCounters, len(samples)),
		ioDeltaValid:     treeComplete && len(samples) > 0,
		connectionsValid: treeComplete && len(samples) > 0,
	}
	usableCPUDeltas := 0
	usableIODeltas := 0
	for _, sample := range samples {
		if sample.cpuReadOK {
			aggregate.cpuSecondsByPID[sample.pid] = sample.cpuSeconds
			previousCPUSeconds, exists := previousCPUSecondsByPID[sample.pid]
			if exists && sample.cpuSeconds >= previousCPUSeconds {
				aggregate.cpuDeltaSeconds += sample.cpuSeconds - previousCPUSeconds
				usableCPUDeltas++
			} else {
				aggregate.cpuDeltaValid = false
			}
		} else {
			aggregate.cpuDeltaValid = false
		}

		if sample.memoryInfoReadOK && sample.memoryPercentReadOK {
			aggregate.memoryRSS += sample.memoryRSS
			aggregate.memoryVMS += sample.memoryVMS
			aggregate.memoryPercent += sample.memoryPercent
		} else {
			aggregate.memoryValid = false
		}
		aggregate.numThreads += sample.numThreads
		if sample.ioReadOK {
			currentIO := processIOCounters{read: sample.ioRead, write: sample.ioWrite}
			aggregate.ioByPID[sample.pid] = currentIO
			previousIO, exists := previousIOByPID[sample.pid]
			if exists && currentIO.read >= previousIO.read && currentIO.write >= previousIO.write {
				aggregate.ioReadDelta += currentIO.read - previousIO.read
				aggregate.ioWriteDelta += currentIO.write - previousIO.write
				usableIODeltas++
			} else {
				aggregate.ioDeltaValid = false
			}
		} else {
			aggregate.ioDeltaValid = false
		}
		if sample.connectionsReadOK {
			aggregate.connections += sample.connections
		} else {
			aggregate.connectionsValid = false
		}
	}
	for previousPID := range previousCPUSecondsByPID {
		_, stillPresent := aggregate.cpuSecondsByPID[previousPID]
		if !stillPresent {
			aggregate.cpuDeltaValid = false
		}
	}
	for previousPID := range previousIOByPID {
		_, stillPresent := aggregate.ioByPID[previousPID]
		if !stillPresent {
			aggregate.ioDeltaValid = false
		}
	}
	if usableCPUDeltas == 0 {
		aggregate.cpuDeltaValid = false
	}
	if usableIODeltas == 0 {
		aggregate.ioDeltaValid = false
	}
	if !aggregate.memoryValid {
		aggregate.memoryRSS = 0
		aggregate.memoryVMS = 0
		aggregate.memoryPercent = 0
	}
	return aggregate
}

func intervalIORates(readDelta, writeDelta uint64, deltaValid bool, now, previousPoll time.Time) (float64, float64, bool) {
	if !deltaValid || previousPoll.IsZero() {
		return 0, 0, false
	}
	elapsedSeconds := now.Sub(previousPoll).Seconds()
	if elapsedSeconds <= 0 {
		return 0, 0, false
	}
	return float64(readDelta) / elapsedSeconds, float64(writeDelta) / elapsedSeconds, true
}

// collectProcessTree returns the root process and each reachable descendant
// exactly once. Process trees can briefly contain repeated observations while
// children exit, so PIDs are deduplicated before aggregation.
func collectProcessTree(root *process.Process) ([]*process.Process, bool) {
	if root == nil {
		return nil, false
	}
	seen := map[int32]struct{}{root.Pid: {}}
	queue := []*process.Process{root}
	processes := make([]*process.Process, 0, 1)
	complete := true
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		processes = append(processes, current)
		children, errChildren := current.Children()
		if errChildren != nil {
			complete = false
			continue
		}
		for _, child := range children {
			if child == nil {
				continue
			}
			_, exists := seen[child.Pid]
			if exists {
				continue
			}
			seen[child.Pid] = struct{}{}
			queue = append(queue, child)
		}
	}
	return processes, complete
}

func intervalCPUPercent(cpuDeltaSeconds float64, deltaValid bool, now, previousPoll time.Time, numCPU int) (float64, bool) {
	if !deltaValid || previousPoll.IsZero() || numCPU <= 0 {
		return 0, false
	}
	elapsedSeconds := now.Sub(previousPoll).Seconds()
	if elapsedSeconds <= 0 {
		return 0, false
	}
	return (cpuDeltaSeconds / elapsedSeconds) * 100 / float64(numCPU), true
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
