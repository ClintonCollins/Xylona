// Package sysinfo collects host and runtime resource information.
package sysinfo

import (
	"runtime"

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

	cpuPercents, errCPU := cpu.Percent(0, false)
	if errCPU == nil && len(cpuPercents) > 0 {
		snapshot.CPUPercent = cpuPercents[0]
	}

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
