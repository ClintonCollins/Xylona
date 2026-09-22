package rpc

import (
	"sort"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
)

const (
	defaultNodeMetricsMaxPoints = 720
	maximumNodeMetricsMaxPoints = 1440
)

// downsampleNodeMetricsRows buckets rows into at most maxPoints time-aligned
// averages so a 7-day range does not ship every minute-level sample. Returns
// the rows unchanged, and interval 0, when no reduction was needed.
func downsampleNodeMetricsRows(rows []*db.NodeMetricsRow, since, until time.Time, maxPoints int) ([]*db.NodeMetricsRow, int32) {
	if maxPoints <= 0 || len(rows) <= maxPoints || !until.After(since) {
		return rows, 0
	}
	bucketWidth := (until.Sub(since) + time.Duration(maxPoints) - 1) / time.Duration(maxPoints)
	bucketWidth = max(bucketWidth, time.Second)

	buckets := make(map[int][]*db.NodeMetricsRow, maxPoints)
	indices := make([]int, 0, maxPoints)
	for _, row := range rows {
		index := int(row.RecordedAt.Sub(since) / bucketWidth)
		index = min(max(index, 0), maxPoints-1)
		if len(buckets[index]) == 0 {
			indices = append(indices, index)
		}
		buckets[index] = append(buckets[index], row)
	}
	sort.Ints(indices)

	result := make([]*db.NodeMetricsRow, 0, len(indices))
	for _, index := range indices {
		result = append(result, aggregateNodeMetricsBucket(buckets[index], since.Add(time.Duration(index)*bucketWidth)))
	}
	return result, helpers.ClampInt32FromInt64(int64((bucketWidth + time.Second - 1) / time.Second))
}

func aggregateNodeMetricsBucket(rows []*db.NodeMetricsRow, recordedAt time.Time) *db.NodeMetricsRow {
	result := &db.NodeMetricsRow{RecordedAt: recordedAt}
	count := float64(len(rows))
	if count == 0 {
		return result
	}
	var memoryUsed, diskUsed, servers, running int64
	for _, row := range rows {
		result.NodeID = row.NodeID
		result.CPUPercent += row.CPUPercent
		result.MemoryPercent += row.MemoryPercent
		result.DiskPercent += row.DiskPercent
		memoryUsed += row.MemoryUsedBytes
		diskUsed += row.DiskUsedBytes
		servers += int64(row.GameServerCount)
		running += int64(row.RunningGameServerCount)
		result.MemoryTotalBytes = max(result.MemoryTotalBytes, row.MemoryTotalBytes)
		result.DiskTotalBytes = max(result.DiskTotalBytes, row.DiskTotalBytes)
	}
	result.CPUPercent /= count
	result.MemoryPercent /= count
	result.DiskPercent /= count
	result.MemoryUsedBytes = memoryUsed / int64(count)
	result.DiskUsedBytes = diskUsed / int64(count)
	// Counts are rounded rather than averaged so a bucket reads as whole servers.
	result.GameServerCount = int(servers+int64(count)/2) / int(count)
	result.RunningGameServerCount = int(running+int64(count)/2) / int(count)
	return result
}
