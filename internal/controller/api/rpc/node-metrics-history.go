package rpc

import (
	"database/sql"
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
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

// aggregateNodeMetricsBucket averages each host reading over the rows that
// have it, so failed readings (NULL) neither drag the average down nor, when
// every row failed, turn into a 0.
func aggregateNodeMetricsBucket(rows []*db.NodeMetricsRow, recordedAt time.Time) *db.NodeMetricsRow {
	result := &db.NodeMetricsRow{RecordedAt: recordedAt}
	count := int64(len(rows))
	if count == 0 {
		return result
	}
	var cpu, memoryPercent, diskPercent, memoryUsed, diskUsed weightedMetric
	var servers, running int64
	for _, row := range rows {
		result.NodeID = row.NodeID
		addNullFloat64(&cpu, row.CPUPercent)
		addNullFloat64(&memoryPercent, row.MemoryPercent)
		addNullFloat64(&diskPercent, row.DiskPercent)
		addNullInt64(&memoryUsed, row.MemoryUsedBytes)
		addNullInt64(&diskUsed, row.DiskUsedBytes)
		servers += int64(row.GameServerCount)
		running += int64(row.RunningGameServerCount)
		result.MemoryTotalBytes = maxNullInt64(result.MemoryTotalBytes, row.MemoryTotalBytes)
		result.DiskTotalBytes = maxNullInt64(result.DiskTotalBytes, row.DiskTotalBytes)
	}
	result.CPUPercent = nullFloat64(cpu)
	result.MemoryPercent = nullFloat64(memoryPercent)
	result.DiskPercent = nullFloat64(diskPercent)
	result.MemoryUsedBytes = nullInt64(memoryUsed)
	result.DiskUsedBytes = nullInt64(diskUsed)
	// Counts are rounded rather than averaged so a bucket reads as whole servers.
	result.GameServerCount = int((servers + count/2) / count)
	result.RunningGameServerCount = int((running + count/2) / count)
	return result
}

func addNullFloat64(metric *weightedMetric, value sql.NullFloat64) {
	if value.Valid {
		metric.add(value.Float64, 1)
	}
}

func addNullInt64(metric *weightedMetric, value sql.NullInt64) {
	if value.Valid {
		metric.add(float64(value.Int64), 1)
	}
}

// nodeMetricsHistoryPointProto maps a stored or aggregated row to the wire
// shape, flagging each metric the row has no valid reading for.
func nodeMetricsHistoryPointProto(row *db.NodeMetricsRow) *xylona.MetricsHistoryPoint {
	return &xylona.MetricsHistoryPoint{
		Timestamp:              timestamppb.New(row.RecordedAt),
		CpuPercent:             row.CPUPercent.Float64,
		MemoryPercent:          row.MemoryPercent.Float64,
		DiskPercent:            row.DiskPercent.Float64,
		MemoryUsedBytes:        row.MemoryUsedBytes.Int64,
		DiskUsedBytes:          row.DiskUsedBytes.Int64,
		GameServerCount:        helpers.ClampInt32FromInt(row.GameServerCount),
		RunningGameServerCount: helpers.ClampInt32FromInt(row.RunningGameServerCount),
		CpuUnavailable:         !row.CPUPercent.Valid,
		MemoryUnavailable:      !row.MemoryPercent.Valid || !row.MemoryUsedBytes.Valid,
		DiskUnavailable:        !row.DiskPercent.Valid || !row.DiskUsedBytes.Valid,
	}
}
