// Package sysinfo collects host and runtime resource information.
package sysinfo

import (
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/ClintonCollins/Xylona/pkg/version"
)

// SystemInfo contains static hardware and OS information.
type SystemInfo struct {
	CPUModel      string
	CPUCores      int
	CPUThreads    int
	TotalMemory   uint64
	OS            string
	OSVersion     string
	Architecture  string
	XylonaVersion string
}

// ResourceSnapshot contains a point-in-time resource usage snapshot. Each
// *Valid flag is true only when that reading succeeded; an invalid metric's
// values are zero, not a real 0%.
type ResourceSnapshot struct {
	CPUPercent    float64
	CPUValid      bool
	MemoryPercent float64
	MemoryUsed    uint64
	MemoryTotal   uint64
	MemoryValid   bool
	DiskPercent   float64
	DiskUsed      uint64
	DiskTotal     uint64
	DiskValid     bool
}

// CollectSystemInfo gathers static hardware and OS information.
func CollectSystemInfo() (*SystemInfo, error) {
	info := &SystemInfo{
		OS:            runtime.GOOS,
		Architecture:  runtime.GOARCH,
		XylonaVersion: version.SoftwareVersion,
	}

	hostInfo, errHost := host.Info()
	if errHost == nil {
		info.OS = hostInfo.OS
		info.OSVersion = hostInfo.PlatformVersion
	}

	cpuInfos, errCPU := cpu.Info()
	if errCPU == nil && len(cpuInfos) > 0 {
		info.CPUModel = cpuInfos[0].ModelName
		info.CPUCores = int(cpuInfos[0].Cores)
	}

	info.CPUThreads = runtime.NumCPU()

	memInfo, errMem := mem.VirtualMemory()
	if errMem == nil {
		info.TotalMemory = memInfo.Total
	}

	return info, nil
}

// CollectResourceSnapshot gathers current resource usage.
func CollectResourceSnapshot() (*ResourceSnapshot, error) {
	snapshot := &ResourceSnapshot{}

	snapshot.CPUPercent, snapshot.CPUValid = sampleCPUPercent()

	// Read failures log at debug: several loops sample every few seconds, and
	// the validity flags already keep consumers from trusting the zeros.
	memInfo, errMem := mem.VirtualMemory()
	if errMem != nil {
		log.Debug().Err(errMem).Msg("sysinfo: read host memory usage failed; reporting memory unavailable")
	} else {
		snapshot.MemoryPercent = memInfo.UsedPercent
		snapshot.MemoryUsed = memInfo.Used
		snapshot.MemoryTotal = memInfo.Total
		snapshot.MemoryValid = true
	}

	diskInfo, errDisk := disk.Usage("/")
	if errDisk != nil {
		log.Debug().Err(errDisk).Str("path", "/").Msg("sysinfo: read host disk usage failed; reporting disk unavailable")
	} else {
		snapshot.DiskPercent = diskInfo.UsedPercent
		snapshot.DiskUsed = diskInfo.Used
		snapshot.DiskTotal = diskInfo.Total
		snapshot.DiskValid = true
	}

	return snapshot, nil
}

// cpuSampleWindow is the shortest interval a CPU reading may span. Callers
// that arrive faster than this (the websocket loop and the metrics recorder
// share one process) receive the previous reading instead of a delta so short
// it rounds to 0% or 100%.
const cpuSampleWindow = 2 * time.Second

// cpuSampler turns cumulative CPU times into a utilisation percentage.
type cpuSampler struct {
	mu      sync.Mutex
	primed  bool
	valid   bool
	last    cpu.TimesStat
	lastAt  time.Time
	percent float64
}

var hostCPUSampler cpuSampler

// sampleCPUPercent returns host CPU utilisation over the window since the
// previous reading, independent of how many callers share the process. The
// bool is false when the read fails or no full window has completed yet.
func sampleCPUPercent() (float64, bool) {
	times, errTimes := cpu.Times(false)
	if errTimes != nil {
		log.Debug().Err(errTimes).Msg("sysinfo: read host CPU times failed; reporting CPU unavailable")
		return 0, false
	}
	if len(times) == 0 {
		log.Debug().Msg("sysinfo: host CPU times reading was empty; reporting CPU unavailable")
		return 0, false
	}
	return hostCPUSampler.sample(times[0], time.Now())
}

// sample folds a cumulative CPU reading taken at now into the sampler.
func (s *cpuSampler) sample(current cpu.TimesStat, now time.Time) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.primed {
		s.primed = true
		s.last = current
		s.lastAt = now
		return 0, false
	}
	if now.Sub(s.lastAt) < cpuSampleWindow {
		return s.percent, s.valid
	}
	if percent, ok := cpuBusyPercent(s.last, current); ok {
		s.percent = percent
		s.valid = true
	}
	s.last = current
	s.lastAt = now
	return s.percent, s.valid
}

// cpuBusyPercent computes the busy share of CPU time between two readings.
func cpuBusyPercent(previous, current cpu.TimesStat) (float64, bool) {
	totalDelta := cpuTotal(current) - cpuTotal(previous)
	if totalDelta <= 0 {
		return 0, false
	}
	idleDelta := (current.Idle + current.Iowait) - (previous.Idle + previous.Iowait)
	busy := (totalDelta - idleDelta) / totalDelta * 100
	return min(max(busy, 0), 100), true
}

func cpuTotal(t cpu.TimesStat) float64 {
	return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice
}
