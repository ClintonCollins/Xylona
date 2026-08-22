package rpc

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	defaultGameServerMetricsMaxPoints = 720
	maximumGameServerMetricsMaxPoints = 1440
	maximumGameServerMetricsRange     = 365 * 24 * time.Hour
)

type gameServerMetricsHistorySeries struct {
	points                []*xylona.GameServerMetricsHistoryPoint
	resolution            xylona.GameServerMetricsResolution
	sampleIntervalSeconds int32
	hasMixedResolution    bool
}

type weightedMetric struct {
	total  float64
	weight int64
}

func (metric *weightedMetric) add(value float64, weight int64) {
	if weight <= 0 {
		return
	}
	metric.total += value * float64(weight)
	metric.weight += weight
}

func (metric *weightedMetric) value() (float64, bool) {
	if metric.weight <= 0 {
		return 0, false
	}
	return metric.total / float64(metric.weight), true
}

func validateGameServerMetricsHistoryRequest(request *xylona.GetGameServerMetricsHistoryRequest) (time.Time, time.Time, int, error) {
	if request == nil {
		return time.Time{}, time.Time{}, 0, errors.New("request is required")
	}
	if strings.TrimSpace(request.GetGameServerId()) == "" {
		return time.Time{}, time.Time{}, 0, errors.New("game server ID is required")
	}
	if request.GetSince() == nil || request.GetUntil() == nil {
		return time.Time{}, time.Time{}, 0, errors.New("since and until timestamps are required")
	}
	errSince := request.GetSince().CheckValid()
	if errSince != nil {
		return time.Time{}, time.Time{}, 0, errors.New("since timestamp is invalid")
	}
	errUntil := request.GetUntil().CheckValid()
	if errUntil != nil {
		return time.Time{}, time.Time{}, 0, errors.New("until timestamp is invalid")
	}

	since := request.GetSince().AsTime().UTC()
	until := request.GetUntil().AsTime().UTC()
	if !since.Before(until) {
		return time.Time{}, time.Time{}, 0, errors.New("since timestamp must be before until timestamp")
	}
	if until.Sub(since) > maximumGameServerMetricsRange {
		return time.Time{}, time.Time{}, 0, errors.New("metrics history range must not exceed 365 days")
	}

	maxPoints := int(request.GetMaxPoints())
	if maxPoints == 0 {
		maxPoints = defaultGameServerMetricsMaxPoints
	}
	if maxPoints < 1 || maxPoints > maximumGameServerMetricsMaxPoints {
		return time.Time{}, time.Time{}, 0, errors.New("max points must be between 1 and 1440")
	}
	return since, until, maxPoints, nil
}

func buildGameServerMetricsHistory(rows []*db.GameServerMetricsRow, since, until time.Time, maxPoints int) gameServerMetricsHistorySeries {
	ordered := compactAndSortGameServerMetricsRows(rows)
	baseResolution, baseInterval, mixed := classifyGameServerMetricsResolution(ordered)
	if len(ordered) == 0 {
		return gameServerMetricsHistorySeries{
			resolution:         baseResolution,
			hasMixedResolution: mixed,
		}
	}

	resolution := baseResolution
	sampleInterval := baseInterval
	if maxPoints > 0 && len(ordered) > maxPoints {
		ordered, sampleInterval = downsampleGameServerMetricsRows(ordered, since, until, maxPoints)
		resolution = xylona.GameServerMetricsResolution_GAME_SERVER_METRICS_RESOLUTION_DOWNSAMPLED
	}

	points := make([]*xylona.GameServerMetricsHistoryPoint, 0, len(ordered))
	for _, row := range ordered {
		points = append(points, mapGameServerMetricsHistoryPoint(row))
	}
	return gameServerMetricsHistorySeries{
		points:                points,
		resolution:            resolution,
		sampleIntervalSeconds: sampleInterval,
		hasMixedResolution:    mixed,
	}
}

func compactAndSortGameServerMetricsRows(rows []*db.GameServerMetricsRow) []*db.GameServerMetricsRow {
	ordered := make([]*db.GameServerMetricsRow, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			ordered = append(ordered, row)
		}
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		return ordered[left].RecordedAt.Before(ordered[right].RecordedAt)
	})
	return ordered
}

func classifyGameServerMetricsResolution(rows []*db.GameServerMetricsRow) (xylona.GameServerMetricsResolution, int32, bool) {
	if len(rows) == 0 {
		return xylona.GameServerMetricsResolution_GAME_SERVER_METRICS_RESOLUTION_UNSPECIFIED, 0, false
	}

	hasRaw := false
	hasHourly := false
	interval := int64(0)
	uniformInterval := true
	for _, row := range rows {
		granularity := normalizedGranularity(row.GranularitySeconds)
		if interval == 0 {
			interval = granularity
		} else if interval != granularity {
			uniformInterval = false
		}
		if granularity >= int64(time.Hour/time.Second) {
			hasHourly = true
		} else {
			hasRaw = true
		}
	}
	if hasRaw && hasHourly {
		return xylona.GameServerMetricsResolution_GAME_SERVER_METRICS_RESOLUTION_MIXED, 0, true
	}
	if !uniformInterval {
		interval = 0
	}
	if hasHourly {
		return xylona.GameServerMetricsResolution_GAME_SERVER_METRICS_RESOLUTION_HOURLY, helpers.ClampInt32FromInt64(interval), false
	}
	return xylona.GameServerMetricsResolution_GAME_SERVER_METRICS_RESOLUTION_RAW, helpers.ClampInt32FromInt64(interval), false
}

func normalizedGranularity(granularity int64) int64 {
	if granularity <= 0 {
		return 60
	}
	return granularity
}

func downsampleGameServerMetricsRows(rows []*db.GameServerMetricsRow, since, until time.Time, maxPoints int) ([]*db.GameServerMetricsRow, int32) {
	rangeDuration := until.Sub(since)
	bucketWidth := (rangeDuration + time.Duration(maxPoints) - 1) / time.Duration(maxPoints)
	bucketWidth = max(bucketWidth, time.Second)

	buckets := make(map[int][]*db.GameServerMetricsRow, maxPoints)
	indices := make([]int, 0, maxPoints)
	for _, row := range rows {
		index := int(row.RecordedAt.Sub(since) / bucketWidth)
		index = max(index, 0)
		if index >= maxPoints {
			index = maxPoints - 1
		}
		if len(buckets[index]) == 0 {
			indices = append(indices, index)
		}
		buckets[index] = append(buckets[index], row)
	}
	sort.Ints(indices)

	result := make([]*db.GameServerMetricsRow, 0, len(indices))
	granularitySeconds := int64((bucketWidth + time.Second - 1) / time.Second)
	for _, index := range indices {
		bucketStart := since.Add(time.Duration(index) * bucketWidth)
		result = append(result, aggregateGameServerMetricsBucket(buckets[index], bucketStart, granularitySeconds))
	}
	return result, helpers.ClampInt32FromInt64(granularitySeconds)
}

func aggregateGameServerMetricsBucket(rows []*db.GameServerMetricsRow, recordedAt time.Time, granularitySeconds int64) *db.GameServerMetricsRow {
	result := &db.GameServerMetricsRow{
		RecordedAt:         recordedAt,
		GranularitySeconds: granularitySeconds,
	}
	if len(rows) == 0 {
		return result
	}

	var cpu weightedMetric
	var memory weightedMetric
	var memoryPercent weightedMetric
	var diskUsage weightedMetric
	var ioRead weightedMetric
	var ioWrite weightedMetric
	var connections weightedMetric
	var nodeMemoryUsed weightedMetric
	var volumeFree weightedMetric
	var volumePercent weightedMetric
	var players weightedMetric
	var queryDuration weightedMetric
	var serverFPS weightedMetric
	var serverFrameTime weightedMetric
	var latestStatus string

	for _, row := range rows {
		sampleCount := normalizedSampleCount(row.SampleCount)
		availableCount := normalizedAvailableSampleCount(row.AvailableSampleCount, sampleCount)
		cpuValidCount := normalizedMetricSampleCount(row.CPUValidSampleCount, row.CPUValid, sampleCount)
		querySuccessfulCount := normalizedMetricSampleCount(
			row.QuerySuccessfulSampleCount,
			row.QuerySuccess.Valid && row.QuerySuccess.Bool,
			sampleCount,
		)
		queryDurationValidCount := normalizedMetricSampleCount(
			row.QueryDurationValidSampleCount,
			row.QueryDurationMS.Valid && querySuccessfulCount > 0,
			querySuccessfulCount,
		)
		serverFPSValidCount := normalizedMetricSampleCount(
			row.ServerFPSValidSampleCount,
			row.ServerFPS.Valid && querySuccessfulCount > 0,
			querySuccessfulCount,
		)
		serverFrameTimeValidCount := normalizedMetricSampleCount(
			row.ServerFrameTimeValidSampleCount,
			row.ServerFrameTimeMS.Valid && querySuccessfulCount > 0,
			querySuccessfulCount,
		)
		volumeValidCount := normalizedMetricSampleCount(row.VolumeValidSampleCount, row.VolumeValid, sampleCount)
		ioValidCount := normalizedExplicitMetricSampleCount(row.IOValidSampleCount, sampleCount)
		connectionValidCount := normalizedExplicitMetricSampleCount(row.ConnectionValidSampleCount, sampleCount)
		result.SampleCount += sampleCount
		result.AvailableSampleCount += availableCount
		result.CPUValidSampleCount += cpuValidCount
		result.QuerySuccessfulSampleCount += querySuccessfulCount
		result.QueryDurationValidSampleCount += queryDurationValidCount
		result.ServerFPSValidSampleCount += serverFPSValidCount
		result.ServerFrameTimeValidSampleCount += serverFrameTimeValidCount
		result.VolumeValidSampleCount += volumeValidCount
		result.IOValidSampleCount += ioValidCount
		result.ConnectionValidSampleCount += connectionValidCount
		latestStatus = row.CollectionStatus

		if cpuValidCount > 0 {
			cpu.add(row.CPUPercent, cpuValidCount)
			result.CPUValid = true
			result.CPUPercentMin = minNullFloat64(result.CPUPercentMin, metricFloatMinimum(row.CPUPercentMin, row.CPUPercent))
			result.CPUPercentMax = maxNullFloat64(result.CPUPercentMax, metricFloatMaximum(row.CPUPercentMax, row.CPUPercent))
		}
		if availableCount > 0 {
			memory.add(float64(row.MemoryBytes), availableCount)
			memoryPercent.add(row.MemoryPercent, availableCount)
			result.MemoryBytesMin = minNullInt64(result.MemoryBytesMin, metricIntMinimum(row.MemoryBytesMin, row.MemoryBytes))
			result.MemoryBytesMax = maxNullInt64(result.MemoryBytesMax, metricIntMaximum(row.MemoryBytesMax, row.MemoryBytes))
			result.MemoryPercentMin = minNullFloat64(result.MemoryPercentMin, metricFloatMinimum(row.MemoryPercentMin, row.MemoryPercent))
			result.MemoryPercentMax = maxNullFloat64(result.MemoryPercentMax, metricFloatMaximum(row.MemoryPercentMax, row.MemoryPercent))
		}
		if ioValidCount > 0 {
			ioRead.add(row.IOReadRate, ioValidCount)
			ioWrite.add(row.IOWriteRate, ioValidCount)
			result.IOReadRateMin = minNullFloat64(result.IOReadRateMin, metricFloatMinimum(row.IOReadRateMin, row.IOReadRate))
			result.IOReadRateMax = maxNullFloat64(result.IOReadRateMax, metricFloatMaximum(row.IOReadRateMax, row.IOReadRate))
			result.IOWriteRateMin = minNullFloat64(result.IOWriteRateMin, metricFloatMinimum(row.IOWriteRateMin, row.IOWriteRate))
			result.IOWriteRateMax = maxNullFloat64(result.IOWriteRateMax, metricFloatMaximum(row.IOWriteRateMax, row.IOWriteRate))
		}
		if connectionValidCount > 0 {
			connections.add(float64(row.ConnectionCount), connectionValidCount)
			result.ConnectionCountMin = minNullInt64(result.ConnectionCountMin, metricIntMinimum(row.ConnectionCountMin, row.ConnectionCount))
			result.ConnectionCountMax = maxNullInt64(result.ConnectionCountMax, metricIntMaximum(row.ConnectionCountMax, row.ConnectionCount))
		}
		if row.NodeMemoryUsedBytes.Valid {
			nodeMemoryUsed.add(float64(row.NodeMemoryUsedBytes.Int64), sampleCount)
		}
		if volumeValidCount > 0 {
			result.VolumeValid = true
			diskUsage.add(float64(row.DiskUsageBytes), volumeValidCount)
			result.DiskUsageBytesMin = minNullInt64(result.DiskUsageBytesMin, metricIntMinimum(row.DiskUsageBytesMin, row.DiskUsageBytes))
			result.DiskUsageBytesMax = maxNullInt64(result.DiskUsageBytesMax, metricIntMaximum(row.DiskUsageBytesMax, row.DiskUsageBytes))
			if row.VolumeFreeBytes.Valid {
				volumeFree.add(float64(row.VolumeFreeBytes.Int64), volumeValidCount)
			}
			if row.VolumePercent.Valid {
				volumePercent.add(row.VolumePercent.Float64, volumeValidCount)
			}
		}
		if querySuccessfulCount > 0 {
			players.add(float64(row.PlayerCount), querySuccessfulCount)
			result.PlayerCountMin = minNullInt64(result.PlayerCountMin, metricIntMinimum(row.PlayerCountMin, row.PlayerCount))
			result.PlayerCountMax = maxNullInt64(result.PlayerCountMax, metricIntMaximum(row.PlayerCountMax, row.PlayerCount))
		}
		if queryDurationValidCount > 0 {
			queryDuration.add(row.QueryDurationMS.Float64, queryDurationValidCount)
			result.QueryDurationMSMin = minNullFloat64(result.QueryDurationMSMin, metricFloatMinimum(row.QueryDurationMSMin, row.QueryDurationMS.Float64))
			result.QueryDurationMSMax = maxNullFloat64(result.QueryDurationMSMax, metricFloatMaximum(row.QueryDurationMSMax, row.QueryDurationMS.Float64))
		}
		if serverFPSValidCount > 0 {
			serverFPS.add(row.ServerFPS.Float64, serverFPSValidCount)
			result.ServerFPSMin = minNullFloat64(result.ServerFPSMin, metricFloatMinimum(row.ServerFPSMin, row.ServerFPS.Float64))
			result.ServerFPSMax = maxNullFloat64(result.ServerFPSMax, metricFloatMaximum(row.ServerFPSMax, row.ServerFPS.Float64))
		}
		if serverFrameTimeValidCount > 0 {
			serverFrameTime.add(row.ServerFrameTimeMS.Float64, serverFrameTimeValidCount)
			result.ServerFrameTimeMSMin = minNullFloat64(result.ServerFrameTimeMSMin, metricFloatMinimum(row.ServerFrameTimeMSMin, row.ServerFrameTimeMS.Float64))
			result.ServerFrameTimeMSMax = maxNullFloat64(result.ServerFrameTimeMSMax, metricFloatMaximum(row.ServerFrameTimeMSMax, row.ServerFrameTimeMS.Float64))
		}

		applyLatestGameServerMetricsFields(result, row)
	}

	result.AvailabilityRatio = ratio(result.AvailableSampleCount, result.SampleCount)
	if result.AvailableSampleCount > 0 {
		result.CollectionStatus = "available"
	} else {
		result.CollectionStatus = latestStatus
	}
	result.CPUPercent, _ = cpu.value()
	result.MemoryBytes = roundedInt64(memory)
	result.MemoryPercent, _ = memoryPercent.value()
	result.DiskUsageBytes = roundedInt64(diskUsage)
	result.IOReadRate, _ = ioRead.value()
	result.IOWriteRate, _ = ioWrite.value()
	result.ConnectionCount = roundedInt64(connections)
	result.PlayerCount = roundedInt64(players)
	result.QueryDurationMS = nullFloat64(queryDuration)
	result.ServerFPS = nullFloat64(serverFPS)
	result.ServerFrameTimeMS = nullFloat64(serverFrameTime)
	result.NodeMemoryUsedBytes = nullInt64(nodeMemoryUsed)
	result.VolumeFreeBytes = nullInt64(volumeFree)
	result.VolumePercent = nullFloat64(volumePercent)
	return result
}

func applyLatestGameServerMetricsFields(result, row *db.GameServerMetricsRow) {
	result.GameServerID = row.GameServerID
	result.NodeID = row.NodeID
	result.NodeCPUCores = latestNullInt64(result.NodeCPUCores, row.NodeCPUCores)
	result.NodeMemoryTotalBytes = latestNullInt64(result.NodeMemoryTotalBytes, row.NodeMemoryTotalBytes)
	result.ConfiguredMemoryBytes = latestNullInt64(result.ConfiguredMemoryBytes, row.ConfiguredMemoryBytes)
	result.VolumeTotalBytes = latestNullInt64(result.VolumeTotalBytes, row.VolumeTotalBytes)
	result.PlayerCapacity = latestNullInt64(result.PlayerCapacity, row.PlayerCapacity)
	result.QuerySupported = latestNullBool(result.QuerySupported, row.QuerySupported)
	result.QuerySuccess = latestNullBool(result.QuerySuccess, row.QuerySuccess)
	result.ServerUptimeSeconds = latestNullInt64(result.ServerUptimeSeconds, row.ServerUptimeSeconds)
	result.ProcessStatus = latestNullString(result.ProcessStatus, row.ProcessStatus)
	result.ExecutionID = latestNullString(result.ExecutionID, row.ExecutionID)
	result.ProcessCollectedAt = laterTime(result.ProcessCollectedAt, row.ProcessCollectedAt)
	result.DiskMeasuredAt = laterTime(result.DiskMeasuredAt, row.DiskMeasuredAt)
	result.QueryCheckedAt = laterTime(result.QueryCheckedAt, row.QueryCheckedAt)
}

func mapGameServerMetricsHistoryPoint(row *db.GameServerMetricsRow) *xylona.GameServerMetricsHistoryPoint {
	sampleCount := normalizedSampleCount(row.SampleCount)
	availableCount := normalizedAvailableSampleCount(row.AvailableSampleCount, sampleCount)
	querySuccessfulCount := normalizedMetricSampleCount(
		row.QuerySuccessfulSampleCount,
		row.QuerySuccess.Valid && row.QuerySuccess.Bool,
		sampleCount,
	)
	point := &xylona.GameServerMetricsHistoryPoint{
		Timestamp:       timestamppb.New(row.RecordedAt),
		CpuPercent:      row.CPUPercent,
		MemoryBytes:     row.MemoryBytes, //nolint:staticcheck // Deprecated field remains populated for API compatibility.
		MemoryPercent:   row.MemoryPercent,
		DiskUsageBytes:  row.DiskUsageBytes,
		PlayerCount:     helpers.ClampInt32FromInt64(row.PlayerCount),
		IoReadRate:      row.IOReadRate,
		IoWriteRate:     row.IOWriteRate,
		ConnectionCount: helpers.ClampInt32FromInt64(row.ConnectionCount),
		MemoryRssBytes:  row.MemoryBytes,
		CpuValid:        row.CPUValid,
		VolumeValid:     row.VolumeValid,
		IoValid:         normalizedExplicitMetricSampleCount(row.IOValidSampleCount, sampleCount) > 0,
		ConnectionCountValid: normalizedExplicitMetricSampleCount(
			row.ConnectionValidSampleCount,
			sampleCount,
		) > 0,
		PlayerCountValid:           querySuccessfulCount > 0,
		CollectionStatus:           mapGameServerMetricsCollectionStatus(row.CollectionStatus),
		GranularitySeconds:         helpers.ClampInt32FromInt64(normalizedGranularity(row.GranularitySeconds)),
		SampleCount:                helpers.ClampInt32FromInt64(sampleCount),
		AvailableSampleCount:       helpers.ClampInt32FromInt64(availableCount),
		CpuValidSampleCount:        helpers.ClampInt32FromInt64(normalizedMetricSampleCount(row.CPUValidSampleCount, row.CPUValid, sampleCount)),
		QuerySuccessfulSampleCount: helpers.ClampInt32FromInt64(querySuccessfulCount),
		QueryDurationValidSampleCount: helpers.ClampInt32FromInt64(normalizedMetricSampleCount(
			row.QueryDurationValidSampleCount,
			row.QueryDurationMS.Valid && querySuccessfulCount > 0,
			querySuccessfulCount,
		)),
		ServerFpsValidSampleCount: helpers.ClampInt32FromInt64(normalizedMetricSampleCount(
			row.ServerFPSValidSampleCount,
			row.ServerFPS.Valid && querySuccessfulCount > 0,
			querySuccessfulCount,
		)),
		ServerFrameTimeValidSampleCount: helpers.ClampInt32FromInt64(normalizedMetricSampleCount(
			row.ServerFrameTimeValidSampleCount,
			row.ServerFrameTimeMS.Valid && querySuccessfulCount > 0,
			querySuccessfulCount,
		)),
		VolumeValidSampleCount:     helpers.ClampInt32FromInt64(normalizedMetricSampleCount(row.VolumeValidSampleCount, row.VolumeValid, sampleCount)),
		IoValidSampleCount:         helpers.ClampInt32FromInt64(normalizedExplicitMetricSampleCount(row.IOValidSampleCount, sampleCount)),
		ConnectionValidSampleCount: helpers.ClampInt32FromInt64(normalizedExplicitMetricSampleCount(row.ConnectionValidSampleCount, sampleCount)),
		AvailabilityRatio:          row.AvailabilityRatio,
	}
	if row.NodeID.Valid {
		point.NodeId = row.NodeID.String
	}
	if row.ProcessStatus.Valid {
		point.ProcessStatus = row.ProcessStatus.String
	}
	if row.ExecutionID.Valid {
		point.ExecutionId = row.ExecutionID.String
	}
	if row.ProcessCollectedAt != nil {
		point.ProcessCollectedAt = timestamppb.New(*row.ProcessCollectedAt)
	}
	if row.DiskMeasuredAt != nil {
		point.DiskMeasuredAt = timestamppb.New(*row.DiskMeasuredAt)
	}
	if row.QueryCheckedAt != nil {
		point.QueryCheckedAt = timestamppb.New(*row.QueryCheckedAt)
	}

	point.CpuPercentMin = optionalFloat64(row.CPUPercentMin)
	point.CpuPercentMax = optionalFloat64(row.CPUPercentMax)
	point.NodeCpuCores = optionalInt32(row.NodeCPUCores)
	point.MemoryRssBytesMin = optionalInt64(row.MemoryBytesMin)
	point.MemoryRssBytesMax = optionalInt64(row.MemoryBytesMax)
	point.MemoryPercentMin = optionalFloat64(row.MemoryPercentMin)
	point.MemoryPercentMax = optionalFloat64(row.MemoryPercentMax)
	point.NodeMemoryUsedBytes = optionalInt64(row.NodeMemoryUsedBytes)
	point.NodeMemoryTotalBytes = optionalInt64(row.NodeMemoryTotalBytes)
	point.ConfiguredMemoryBytes = optionalInt64(row.ConfiguredMemoryBytes)
	point.DiskUsageBytesMin = optionalInt64(row.DiskUsageBytesMin)
	point.DiskUsageBytesMax = optionalInt64(row.DiskUsageBytesMax)
	point.VolumeTotalBytes = optionalInt64(row.VolumeTotalBytes)
	point.VolumeFreeBytes = optionalInt64(row.VolumeFreeBytes)
	point.VolumePercent = optionalFloat64(row.VolumePercent)
	point.IoReadRateMin = optionalFloat64(row.IOReadRateMin)
	point.IoReadRateMax = optionalFloat64(row.IOReadRateMax)
	point.IoWriteRateMin = optionalFloat64(row.IOWriteRateMin)
	point.IoWriteRateMax = optionalFloat64(row.IOWriteRateMax)
	point.ConnectionCountMin = optionalInt32(row.ConnectionCountMin)
	point.ConnectionCountMax = optionalInt32(row.ConnectionCountMax)
	point.PlayerCountMin = optionalInt32(row.PlayerCountMin)
	point.PlayerCountMax = optionalInt32(row.PlayerCountMax)
	point.PlayerCapacity = optionalInt32(row.PlayerCapacity)
	point.QuerySupported = optionalBool(row.QuerySupported)
	point.QuerySuccess = optionalBool(row.QuerySuccess)
	point.QueryDurationMs = optionalFloat64(row.QueryDurationMS)
	point.QueryDurationMsMin = optionalFloat64(row.QueryDurationMSMin)
	point.QueryDurationMsMax = optionalFloat64(row.QueryDurationMSMax)
	point.ServerFps = optionalFloat64(row.ServerFPS)
	point.ServerFpsMin = optionalFloat64(row.ServerFPSMin)
	point.ServerFpsMax = optionalFloat64(row.ServerFPSMax)
	point.ServerFrameTimeMs = optionalFloat64(row.ServerFrameTimeMS)
	point.ServerFrameTimeMsMin = optionalFloat64(row.ServerFrameTimeMSMin)
	point.ServerFrameTimeMsMax = optionalFloat64(row.ServerFrameTimeMSMax)
	point.ServerUptimeSeconds = optionalInt64(row.ServerUptimeSeconds)
	return point
}

func mapGameServerLifecycleEvents(rows []db.GameServerLifecycleEvent) []*xylona.GameServerLifecycleHistoryEvent {
	events := make([]*xylona.GameServerLifecycleHistoryEvent, 0, len(rows))
	for _, row := range rows {
		event := &xylona.GameServerLifecycleHistoryEvent{
			Id:                 row.ID,
			GameServerId:       row.GameServerID,
			NodeId:             row.NodeID,
			ExecutionId:        row.ExecutionID,
			TransitionSequence: row.TransitionSequence,
			PreviousStatus:     row.PreviousStatus,
			Status:             row.Status,
			IntentionalStop:    row.IntentionalStop,
			ObservedAt:         timestamppb.New(row.ObservedAt),
		}
		if row.ExitCode != nil {
			exitCode := helpers.ClampInt32FromInt(*row.ExitCode)
			event.ExitCode = &exitCode
		}
		events = append(events, event)
	}
	return events
}

func mapGameServerOperationEvents(rows []db.GameServerOperationEvent) []*xylona.GameServerOperationHistoryEvent {
	events := make([]*xylona.GameServerOperationHistoryEvent, 0, len(rows))
	for _, row := range rows {
		event := &xylona.GameServerOperationHistoryEvent{
			Id:             row.ID,
			GameServerId:   row.GameServerID,
			Operation:      string(row.Operation),
			Outcome:        string(row.Outcome),
			StartedAt:      timestamppb.New(row.StartedAt),
			DurationMs:     row.DurationMS,
			BytesProcessed: row.BytesProcessed,
			Source:         string(row.Source),
		}
		if row.Phase != "" {
			phase := string(row.Phase)
			event.Phase = &phase
		}
		if row.CompletedAt != nil {
			event.CompletedAt = timestamppb.New(*row.CompletedAt)
		}
		events = append(events, event)
	}
	return events
}

func mapGameServerMetricsCollectionStatus(status string) xylona.GameServerMetricsCollectionStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "available":
		return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_AVAILABLE
	case "warming_up":
		return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_WARMING_UP
	case "server_offline":
		return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_SERVER_OFFLINE
	case "node_unavailable":
		return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_NODE_UNAVAILABLE
	case "collector_error":
		return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_COLLECTOR_ERROR
	default:
		return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_UNSPECIFIED
	}
}

func normalizedSampleCount(value int64) int64 {
	if value <= 0 {
		return 1
	}
	return value
}

func normalizedAvailableSampleCount(value, sampleCount int64) int64 {
	if value <= 0 {
		return 0
	}
	if value > sampleCount {
		return sampleCount
	}
	return value
}

func normalizedMetricSampleCount(value int64, valid bool, sampleCount int64) int64 {
	if value <= 0 {
		if valid {
			return sampleCount
		}
		return 0
	}
	return min(value, sampleCount)
}

func normalizedExplicitMetricSampleCount(value, sampleCount int64) int64 {
	return min(max(value, 0), sampleCount)
}

func ratio(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func roundedInt64(metric weightedMetric) int64 {
	value, valid := metric.value()
	if !valid {
		return 0
	}
	if value >= 0 {
		return int64(value + 0.5)
	}
	return int64(value - 0.5)
}

func nullFloat64(metric weightedMetric) sql.NullFloat64 {
	value, valid := metric.value()
	return sql.NullFloat64{Float64: value, Valid: valid}
}

func nullInt64(metric weightedMetric) sql.NullInt64 {
	if metric.weight <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: roundedInt64(metric), Valid: true}
}

func metricFloatMinimum(stored sql.NullFloat64, fallback float64) sql.NullFloat64 {
	if stored.Valid {
		return stored
	}
	return sql.NullFloat64{Float64: fallback, Valid: true}
}

func metricFloatMaximum(stored sql.NullFloat64, fallback float64) sql.NullFloat64 {
	if stored.Valid {
		return stored
	}
	return sql.NullFloat64{Float64: fallback, Valid: true}
}

func metricIntMinimum(stored sql.NullInt64, fallback int64) sql.NullInt64 {
	if stored.Valid {
		return stored
	}
	return sql.NullInt64{Int64: fallback, Valid: true}
}

func metricIntMaximum(stored sql.NullInt64, fallback int64) sql.NullInt64 {
	if stored.Valid {
		return stored
	}
	return sql.NullInt64{Int64: fallback, Valid: true}
}

func minNullFloat64(current, candidate sql.NullFloat64) sql.NullFloat64 {
	if !candidate.Valid {
		return current
	}
	if !current.Valid || candidate.Float64 < current.Float64 {
		return candidate
	}
	return current
}

func maxNullFloat64(current, candidate sql.NullFloat64) sql.NullFloat64 {
	if !candidate.Valid {
		return current
	}
	if !current.Valid || candidate.Float64 > current.Float64 {
		return candidate
	}
	return current
}

func minNullInt64(current, candidate sql.NullInt64) sql.NullInt64 {
	if !candidate.Valid {
		return current
	}
	if !current.Valid || candidate.Int64 < current.Int64 {
		return candidate
	}
	return current
}

func maxNullInt64(current, candidate sql.NullInt64) sql.NullInt64 {
	if !candidate.Valid {
		return current
	}
	if !current.Valid || candidate.Int64 > current.Int64 {
		return candidate
	}
	return current
}

func latestNullInt64(current, candidate sql.NullInt64) sql.NullInt64 {
	if candidate.Valid {
		return candidate
	}
	return current
}

func latestNullString(current, candidate sql.NullString) sql.NullString {
	if candidate.Valid {
		return candidate
	}
	return current
}

func latestNullBool(current, candidate sql.NullBool) sql.NullBool {
	if candidate.Valid {
		return candidate
	}
	return current
}

func laterTime(current, candidate *time.Time) *time.Time {
	if candidate == nil {
		return current
	}
	if current == nil || candidate.After(*current) {
		value := *candidate
		return &value
	}
	return current
}

func optionalFloat64(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}

func optionalInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func optionalInt32(value sql.NullInt64) *int32 {
	if !value.Valid {
		return nil
	}
	result := helpers.ClampInt32FromInt64(value.Int64)
	return &result
}

func optionalBool(value sql.NullBool) *bool {
	if !value.Valid {
		return nil
	}
	result := value.Bool
	return &result
}
