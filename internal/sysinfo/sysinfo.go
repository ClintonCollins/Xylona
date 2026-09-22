// Package sysinfo collects host and runtime resource information.
package sysinfo

import (
	"runtime"
	"sync"
	"time"

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

// ResourceSnapshot contains a point-in-time resource usage snapshot.
type ResourceSnapshot struct {
	CPUPercent    float64
	MemoryPercent float64
	MemoryUsed    uint64
	MemoryTotal   uint64
	DiskPercent   float64
	DiskUsed      uint64
	DiskTotal     uint64
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

	snapshot.CPUPercent = sampleCPUPercent()

	memInfo, errMem := mem.VirtualMemory()
	if errMem == nil {
		snapshot.MemoryPercent = memInfo.UsedPercent
		snapshot.MemoryUsed = memInfo.Used
		snapshot.MemoryTotal = memInfo.Total
	}

	diskInfo, errDisk := disk.Usage("/")
	if errDisk == nil {
		snapshot.DiskPercent = diskInfo.UsedPercent
		snapshot.DiskUsed = diskInfo.Used
		snapshot.DiskTotal = diskInfo.Total
	}

	return snapshot, nil
}

// cpuSampleWindow is the shortest interval a CPU reading may span. Callers
// that arrive faster than this (the websocket loop and the metrics recorder
// share one process) receive the previous reading instead of a delta so short
// it rounds to 0% or 100%.
const cpuSampleWindow = 2 * time.Second

var cpuSampler struct {
	mu      sync.Mutex
	primed  bool
	last    cpu.TimesStat
	lastAt  time.Time
	percent float64
}

// sampleCPUPercent returns host CPU utilisation over the window since the
// previous reading, independent of how many callers share the process.
func sampleCPUPercent() float64 {
	times, errTimes := cpu.Times(false)
	if errTimes != nil || len(times) == 0 {
		return 0
	}
	now := time.Now()

	cpuSampler.mu.Lock()
	defer cpuSampler.mu.Unlock()
	if !cpuSampler.primed {
		cpuSampler.primed = true
		cpuSampler.last = times[0]
		cpuSampler.lastAt = now
		return 0
	}
	if now.Sub(cpuSampler.lastAt) < cpuSampleWindow {
		return cpuSampler.percent
	}
	if percent, ok := cpuBusyPercent(cpuSampler.last, times[0]); ok {
		cpuSampler.percent = percent
	}
	cpuSampler.last = times[0]
	cpuSampler.lastAt = now
	return cpuSampler.percent
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
