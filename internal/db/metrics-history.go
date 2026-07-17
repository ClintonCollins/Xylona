package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// sqliteDatetimeFormat is the format used to store and query datetime values
// in SQLite. Using a clean format without timezone or nanoseconds ensures
// compatibility with SQLite's strftime() and datetime() functions.
const sqliteDatetimeFormat = "2006-01-02 15:04:05"

const sqliteDatetimePrefixLength = len(sqliteDatetimeFormat)

const recordedAtComparableExpr = `replace(substr(recorded_at, 1, 19), 'T', ' ')`
const recordedAtHourBucketExpr = `replace(substr(recorded_at, 1, 13), 'T', ' ')`
const recordedAtMinuteSecondExpr = `substr(replace(recorded_at, 'T', ' '), 15, 5)`

// fmtTime formats a time.Time as a SQLite-compatible datetime string.
func fmtTime(t time.Time) string {
	return t.UTC().Format(sqliteDatetimeFormat)
}

func fmtOptionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return fmtTime(*t)
}

func parseMetricsRecordedAt(value string) (time.Time, error) {
	if len(value) < sqliteDatetimePrefixLength {
		return time.Time{}, fmt.Errorf("timestamp %q shorter than expected", value)
	}

	normalizedValue := strings.ReplaceAll(value[:sqliteDatetimePrefixLength], "T", " ")
	parsedTime, errParse := time.Parse(sqliteDatetimeFormat, normalizedValue)
	if errParse != nil {
		return time.Time{}, fmt.Errorf("parse normalized timestamp %q: %w", normalizedValue, errParse)
	}

	return parsedTime.UTC(), nil
}

func parseOptionalMetricsTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil //nolint:nilnil // nil explicitly represents an unavailable measurement time.
	}
	parsed, errParse := parseMetricsRecordedAt(value.String)
	if errParse != nil {
		return nil, errParse
	}
	return &parsed, nil
}

// NodeMetricsRow represents a row from the node_metrics_history table.
type NodeMetricsRow struct {
	ID                     string
	NodeID                 string
	CPUPercent             float64
	MemoryPercent          float64
	MemoryUsedBytes        int64
	MemoryTotalBytes       int64
	DiskPercent            float64
	DiskUsedBytes          int64
	DiskTotalBytes         int64
	GameServerCount        int
	RunningGameServerCount int
	UserCount              int
	RecordedAt             time.Time
}

// GameServerMetricsRow represents a row from the game_server_metrics_history table.
type GameServerMetricsRow struct {
	ID                              string
	GameServerID                    string
	NodeID                          sql.NullString
	CPUPercent                      float64
	MemoryBytes                     int64
	MemoryPercent                   float64
	DiskUsageBytes                  int64
	IOReadRate                      float64
	IOWriteRate                     float64
	ConnectionCount                 int64
	PlayerCount                     int64
	RecordedAt                      time.Time
	GranularitySeconds              int64
	SampleCount                     int64
	AvailableSampleCount            int64
	CPUValidSampleCount             int64
	QuerySuccessfulSampleCount      int64
	QueryDurationValidSampleCount   int64
	ServerFPSValidSampleCount       int64
	ServerFrameTimeValidSampleCount int64
	VolumeValidSampleCount          int64
	IOValidSampleCount              int64
	IOValidSampleCountSet           bool
	ConnectionValidSampleCount      int64
	ConnectionValidSampleCountSet   bool
	AvailabilityRatio               float64
	CollectionStatus                string
	ProcessCollectedAt              *time.Time
	CPUValid                        bool
	CPUValidSet                     bool
	CPUPercentMin                   sql.NullFloat64
	CPUPercentMax                   sql.NullFloat64
	NodeCPUCores                    sql.NullInt64
	MemoryBytesMin                  sql.NullInt64
	MemoryBytesMax                  sql.NullInt64
	MemoryPercentMin                sql.NullFloat64
	MemoryPercentMax                sql.NullFloat64
	NodeMemoryUsedBytes             sql.NullInt64
	NodeMemoryTotalBytes            sql.NullInt64
	ConfiguredMemoryBytes           sql.NullInt64
	DiskUsageBytesMin               sql.NullInt64
	DiskUsageBytesMax               sql.NullInt64
	VolumeTotalBytes                sql.NullInt64
	VolumeFreeBytes                 sql.NullInt64
	VolumePercent                   sql.NullFloat64
	VolumeValid                     bool
	DiskMeasuredAt                  *time.Time
	IOReadRateMin                   sql.NullFloat64
	IOReadRateMax                   sql.NullFloat64
	IOWriteRateMin                  sql.NullFloat64
	IOWriteRateMax                  sql.NullFloat64
	ConnectionCountMin              sql.NullInt64
	ConnectionCountMax              sql.NullInt64
	PlayerCountMin                  sql.NullInt64
	PlayerCountMax                  sql.NullInt64
	PlayerCapacity                  sql.NullInt64
	QuerySupported                  sql.NullBool
	QuerySuccess                    sql.NullBool
	QueryDurationMS                 sql.NullFloat64
	QueryDurationMSMin              sql.NullFloat64
	QueryDurationMSMax              sql.NullFloat64
	QueryCheckedAt                  *time.Time
	ServerFPS                       sql.NullFloat64
	ServerFPSMin                    sql.NullFloat64
	ServerFPSMax                    sql.NullFloat64
	ServerFrameTimeMS               sql.NullFloat64
	ServerFrameTimeMSMin            sql.NullFloat64
	ServerFrameTimeMSMax            sql.NullFloat64
	ServerUptimeSeconds             sql.NullInt64
	ProcessStatus                   sql.NullString
	ExecutionID                     sql.NullString
}

// InsertNodeMetricsHistory inserts a node metrics snapshot.
func (c *Connection) InsertNodeMetricsHistory(row *NodeMetricsRow) error {
	_, errExec := c.SQLDb.ExecContext(c.ctx,
		`INSERT INTO node_metrics_history (id, node_id, cpu_percent, memory_percent, memory_used_bytes, memory_total_bytes, disk_percent, disk_used_bytes, disk_total_bytes, game_server_count, running_game_server_count, user_count, recorded_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID, row.NodeID, row.CPUPercent, row.MemoryPercent, row.MemoryUsedBytes, row.MemoryTotalBytes,
		row.DiskPercent, row.DiskUsedBytes, row.DiskTotalBytes, row.GameServerCount, row.RunningGameServerCount,
		row.UserCount, fmtTime(row.RecordedAt),
	)
	if errExec != nil {
		return fmt.Errorf("insert node metrics history: %w", errExec)
	}
	return nil
}

// InsertGameServerMetricsHistory inserts a game server metrics snapshot.
func (c *Connection) InsertGameServerMetricsHistory(row *GameServerMetricsRow) error {
	granularitySeconds := row.GranularitySeconds
	if granularitySeconds <= 0 {
		granularitySeconds = 60
	}
	sampleCount := row.SampleCount
	if sampleCount <= 0 {
		sampleCount = 1
	}
	availableSampleCount := max(row.AvailableSampleCount, 0)
	collectionStatus := row.CollectionStatus
	if collectionStatus == "" {
		collectionStatus = "available"
	}
	cpuValid := row.CPUValid
	if !row.CPUValidSet && collectionStatus == "available" {
		cpuValid = true
	}
	cpuValidSampleCount := min(max(row.CPUValidSampleCount, 0), sampleCount)
	if cpuValidSampleCount == 0 && cpuValid {
		cpuValidSampleCount = sampleCount
	}
	querySuccessfulSampleCount := min(max(row.QuerySuccessfulSampleCount, 0), sampleCount)
	if querySuccessfulSampleCount == 0 && row.QuerySuccess.Valid && row.QuerySuccess.Bool {
		querySuccessfulSampleCount = sampleCount
	}
	queryDurationValidSampleCount := min(max(row.QueryDurationValidSampleCount, 0), querySuccessfulSampleCount)
	if queryDurationValidSampleCount == 0 && querySuccessfulSampleCount > 0 && row.QueryDurationMS.Valid {
		queryDurationValidSampleCount = querySuccessfulSampleCount
	}
	serverFPSValidSampleCount := min(max(row.ServerFPSValidSampleCount, 0), querySuccessfulSampleCount)
	if serverFPSValidSampleCount == 0 && querySuccessfulSampleCount > 0 && row.ServerFPS.Valid {
		serverFPSValidSampleCount = querySuccessfulSampleCount
	}
	serverFrameTimeValidSampleCount := min(max(row.ServerFrameTimeValidSampleCount, 0), querySuccessfulSampleCount)
	if serverFrameTimeValidSampleCount == 0 && querySuccessfulSampleCount > 0 && row.ServerFrameTimeMS.Valid {
		serverFrameTimeValidSampleCount = querySuccessfulSampleCount
	}
	volumeValidSampleCount := min(max(row.VolumeValidSampleCount, 0), sampleCount)
	if volumeValidSampleCount == 0 && row.VolumeValid {
		volumeValidSampleCount = sampleCount
	}
	ioValidSampleCount := min(max(row.IOValidSampleCount, 0), sampleCount)
	if !row.IOValidSampleCountSet && ioValidSampleCount == 0 && availableSampleCount > 0 {
		ioValidSampleCount = availableSampleCount
	}
	connectionValidSampleCount := min(max(row.ConnectionValidSampleCount, 0), sampleCount)
	if !row.ConnectionValidSampleCountSet && connectionValidSampleCount == 0 && availableSampleCount > 0 {
		connectionValidSampleCount = availableSampleCount
	}
	availabilityRatio := row.AvailabilityRatio
	if row.AvailableSampleCount == 0 && collectionStatus == "available" {
		availableSampleCount = 1
		availabilityRatio = 1
	}

	columns := `id, game_server_id, cpu_percent, memory_bytes, memory_percent, disk_usage_bytes,
		io_read_rate, io_write_rate, connection_count, player_count, recorded_at, node_id,
		granularity_seconds, sample_count, available_sample_count, cpu_valid_sample_count,
		query_successful_sample_count, query_duration_valid_sample_count,
		server_fps_valid_sample_count, server_frame_time_valid_sample_count,
		volume_valid_sample_count, io_valid_sample_count,
		connection_valid_sample_count, availability_ratio, collection_status,
		process_collected_at, cpu_valid, cpu_percent_min, cpu_percent_max, node_cpu_cores,
		memory_bytes_min, memory_bytes_max, memory_percent_min, memory_percent_max,
		node_memory_used_bytes, node_memory_total_bytes, configured_memory_bytes,
		disk_usage_bytes_min, disk_usage_bytes_max, volume_total_bytes, volume_free_bytes,
		volume_percent, volume_valid, disk_measured_at, io_read_rate_min, io_read_rate_max,
		io_write_rate_min, io_write_rate_max, connection_count_min, connection_count_max,
		player_count_min, player_count_max, player_capacity, query_supported, query_success,
		query_duration_ms, query_duration_ms_min, query_duration_ms_max, query_checked_at,
		server_fps, server_fps_min, server_fps_max, server_frame_time_ms,
		server_frame_time_ms_min, server_frame_time_ms_max, server_uptime_seconds,
		process_status, execution_id`
	values := []any{
		row.ID, row.GameServerID, row.CPUPercent, row.MemoryBytes, row.MemoryPercent,
		row.DiskUsageBytes, row.IOReadRate, row.IOWriteRate, row.ConnectionCount, row.PlayerCount,
		fmtTime(row.RecordedAt), row.NodeID, granularitySeconds, sampleCount, availableSampleCount,
		cpuValidSampleCount, querySuccessfulSampleCount, queryDurationValidSampleCount,
		serverFPSValidSampleCount, serverFrameTimeValidSampleCount, volumeValidSampleCount, ioValidSampleCount,
		connectionValidSampleCount, availabilityRatio, collectionStatus,
		fmtOptionalTime(row.ProcessCollectedAt), cpuValid,
		row.CPUPercentMin, row.CPUPercentMax, row.NodeCPUCores, row.MemoryBytesMin, row.MemoryBytesMax,
		row.MemoryPercentMin, row.MemoryPercentMax, row.NodeMemoryUsedBytes, row.NodeMemoryTotalBytes,
		row.ConfiguredMemoryBytes, row.DiskUsageBytesMin, row.DiskUsageBytesMax, row.VolumeTotalBytes,
		row.VolumeFreeBytes, row.VolumePercent, row.VolumeValid, fmtOptionalTime(row.DiskMeasuredAt),
		row.IOReadRateMin, row.IOReadRateMax, row.IOWriteRateMin, row.IOWriteRateMax,
		row.ConnectionCountMin, row.ConnectionCountMax, row.PlayerCountMin, row.PlayerCountMax,
		row.PlayerCapacity, row.QuerySupported, row.QuerySuccess, row.QueryDurationMS,
		row.QueryDurationMSMin, row.QueryDurationMSMax, fmtOptionalTime(row.QueryCheckedAt), row.ServerFPS,
		row.ServerFPSMin, row.ServerFPSMax, row.ServerFrameTimeMS, row.ServerFrameTimeMSMin,
		row.ServerFrameTimeMSMax, row.ServerUptimeSeconds, row.ProcessStatus, row.ExecutionID,
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
	// #nosec G201 -- columns and placeholder count are fixed by this function; values remain parameterized.
	query := fmt.Sprintf("INSERT INTO game_server_metrics_history (%s) VALUES (%s)", columns, placeholders)
	_, errExec := c.SQLDb.ExecContext(c.ctx, query, values...)
	if errExec != nil {
		return fmt.Errorf("insert game server metrics history: %w", errExec)
	}
	return nil
}

// GetNodeMetricsHistory returns node metrics within the given time range.
func (c *Connection) GetNodeMetricsHistory(nodeID string, since, until time.Time) ([]*NodeMetricsRow, error) {
	query := fmt.Sprintf(
		`SELECT id, node_id, cpu_percent, memory_percent, memory_used_bytes, memory_total_bytes, disk_percent, disk_used_bytes, disk_total_bytes, game_server_count, running_game_server_count, user_count, recorded_at
		FROM node_metrics_history
		WHERE node_id = ? AND %s >= ? AND %s <= ?
		ORDER BY %s ASC`,
		recordedAtComparableExpr,
		recordedAtComparableExpr,
		recordedAtComparableExpr,
	)
	rows, errQuery := c.SQLDb.QueryContext(c.ctx, query, nodeID, fmtTime(since), fmtTime(until))
	if errQuery != nil {
		return nil, fmt.Errorf("query node metrics history: %w", errQuery)
	}
	defer func() { _ = rows.Close() }()

	var results []*NodeMetricsRow
	for rows.Next() {
		row := &NodeMetricsRow{}
		var recordedAtStr string
		errScan := rows.Scan(
			&row.ID, &row.NodeID, &row.CPUPercent, &row.MemoryPercent, &row.MemoryUsedBytes,
			&row.MemoryTotalBytes, &row.DiskPercent, &row.DiskUsedBytes, &row.DiskTotalBytes,
			&row.GameServerCount, &row.RunningGameServerCount, &row.UserCount, &recordedAtStr,
		)
		if errScan != nil {
			return nil, fmt.Errorf("scan node metrics history row: %w", errScan)
		}
		parsedTime, errParse := parseMetricsRecordedAt(recordedAtStr)
		if errParse != nil {
			return nil, fmt.Errorf("parse node metrics history timestamp: %w", errParse)
		}
		row.RecordedAt = parsedTime
		results = append(results, row)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate node metrics history: %w", errRows)
	}

	return results, nil
}

// GetGameServerMetricsHistory returns game server metrics within the given time range.
func (c *Connection) GetGameServerMetricsHistory(gameServerID string, since, until time.Time) ([]*GameServerMetricsRow, error) {
	query := fmt.Sprintf(
		`SELECT id, game_server_id, cpu_percent, memory_bytes, memory_percent, disk_usage_bytes,
			io_read_rate, io_write_rate, connection_count, player_count, recorded_at, node_id,
			granularity_seconds, sample_count, available_sample_count, cpu_valid_sample_count,
			query_successful_sample_count, query_duration_valid_sample_count,
			server_fps_valid_sample_count, server_frame_time_valid_sample_count,
			volume_valid_sample_count, io_valid_sample_count,
			connection_valid_sample_count, availability_ratio, collection_status,
			process_collected_at, cpu_valid, cpu_percent_min, cpu_percent_max, node_cpu_cores,
			memory_bytes_min, memory_bytes_max, memory_percent_min, memory_percent_max,
			node_memory_used_bytes, node_memory_total_bytes, configured_memory_bytes,
			disk_usage_bytes_min, disk_usage_bytes_max, volume_total_bytes, volume_free_bytes,
			volume_percent, volume_valid, disk_measured_at, io_read_rate_min, io_read_rate_max,
			io_write_rate_min, io_write_rate_max, connection_count_min, connection_count_max,
			player_count_min, player_count_max, player_capacity, query_supported, query_success,
			query_duration_ms, query_duration_ms_min, query_duration_ms_max, query_checked_at,
			server_fps, server_fps_min, server_fps_max, server_frame_time_ms,
			server_frame_time_ms_min, server_frame_time_ms_max, server_uptime_seconds,
			process_status, execution_id
		FROM game_server_metrics_history
		WHERE game_server_id = ? AND %s >= ? AND %s <= ?
		ORDER BY %s ASC`,
		recordedAtComparableExpr,
		recordedAtComparableExpr,
		recordedAtComparableExpr,
	)
	rows, errQuery := c.SQLDb.QueryContext(c.ctx, query, gameServerID, fmtTime(since), fmtTime(until))
	if errQuery != nil {
		return nil, fmt.Errorf("query game server metrics history: %w", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			log.Error().Err(errClose).Str("game_server_id", gameServerID).Msg("Failed to close game server metrics rows")
		}
	}()

	var results []*GameServerMetricsRow
	for rows.Next() {
		row := &GameServerMetricsRow{}
		var recordedAtStr string
		var processCollectedAtStr sql.NullString
		var diskMeasuredAtStr sql.NullString
		var queryCheckedAtStr sql.NullString
		errScan := rows.Scan(
			&row.ID, &row.GameServerID, &row.CPUPercent, &row.MemoryBytes, &row.MemoryPercent,
			&row.DiskUsageBytes, &row.IOReadRate, &row.IOWriteRate, &row.ConnectionCount, &row.PlayerCount,
			&recordedAtStr, &row.NodeID, &row.GranularitySeconds, &row.SampleCount,
			&row.AvailableSampleCount, &row.CPUValidSampleCount, &row.QuerySuccessfulSampleCount,
			&row.QueryDurationValidSampleCount, &row.ServerFPSValidSampleCount,
			&row.ServerFrameTimeValidSampleCount, &row.VolumeValidSampleCount,
			&row.IOValidSampleCount, &row.ConnectionValidSampleCount,
			&row.AvailabilityRatio, &row.CollectionStatus,
			&processCollectedAtStr, &row.CPUValid, &row.CPUPercentMin, &row.CPUPercentMax,
			&row.NodeCPUCores, &row.MemoryBytesMin, &row.MemoryBytesMax, &row.MemoryPercentMin,
			&row.MemoryPercentMax, &row.NodeMemoryUsedBytes, &row.NodeMemoryTotalBytes,
			&row.ConfiguredMemoryBytes, &row.DiskUsageBytesMin, &row.DiskUsageBytesMax,
			&row.VolumeTotalBytes, &row.VolumeFreeBytes, &row.VolumePercent, &row.VolumeValid,
			&diskMeasuredAtStr, &row.IOReadRateMin, &row.IOReadRateMax, &row.IOWriteRateMin,
			&row.IOWriteRateMax, &row.ConnectionCountMin, &row.ConnectionCountMax,
			&row.PlayerCountMin, &row.PlayerCountMax, &row.PlayerCapacity, &row.QuerySupported,
			&row.QuerySuccess, &row.QueryDurationMS, &row.QueryDurationMSMin, &row.QueryDurationMSMax,
			&queryCheckedAtStr, &row.ServerFPS, &row.ServerFPSMin, &row.ServerFPSMax,
			&row.ServerFrameTimeMS, &row.ServerFrameTimeMSMin, &row.ServerFrameTimeMSMax,
			&row.ServerUptimeSeconds, &row.ProcessStatus, &row.ExecutionID,
		)
		if errScan != nil {
			return nil, fmt.Errorf("scan game server metrics history row: %w", errScan)
		}
		row.CPUValidSet = true
		parsedTime, errParse := parseMetricsRecordedAt(recordedAtStr)
		if errParse != nil {
			return nil, fmt.Errorf("parse game server metrics history timestamp: %w", errParse)
		}
		row.RecordedAt = parsedTime
		processCollectedAt, errProcessTime := parseOptionalMetricsTime(processCollectedAtStr)
		if errProcessTime != nil {
			return nil, fmt.Errorf("parse process metrics timestamp: %w", errProcessTime)
		}
		row.ProcessCollectedAt = processCollectedAt
		diskMeasuredAt, errDiskTime := parseOptionalMetricsTime(diskMeasuredAtStr)
		if errDiskTime != nil {
			return nil, fmt.Errorf("parse disk metrics timestamp: %w", errDiskTime)
		}
		row.DiskMeasuredAt = diskMeasuredAt
		queryCheckedAt, errQueryTime := parseOptionalMetricsTime(queryCheckedAtStr)
		if errQueryTime != nil {
			return nil, fmt.Errorf("parse query metrics timestamp: %w", errQueryTime)
		}
		row.QueryCheckedAt = queryCheckedAt
		results = append(results, row)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate game server metrics history: %w", errRows)
	}

	return results, nil
}

// DeleteNodeMinuteMetricsHistoryOlderThan deletes minute-granularity node metrics older than the given time.
func (c *Connection) DeleteNodeMinuteMetricsHistoryOlderThan(olderThan time.Time) (int64, error) {
	query := fmt.Sprintf(
		`DELETE FROM node_metrics_history WHERE %s < ? AND %s != '00:00'`,
		recordedAtComparableExpr,
		recordedAtMinuteSecondExpr,
	)
	result, errExec := c.SQLDb.ExecContext(c.ctx, query, fmtTime(olderThan))
	if errExec != nil {
		return 0, fmt.Errorf("delete old node metrics history: %w", errExec)
	}
	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("delete old node metrics history rows affected: %w", errRowsAffected)
	}
	return rowsAffected, nil
}

// DeleteNodeHourlyMetricsHistoryOlderThan deletes hourly node metrics older than the given time.
func (c *Connection) DeleteNodeHourlyMetricsHistoryOlderThan(olderThan time.Time) (int64, error) {
	query := fmt.Sprintf(
		`DELETE FROM node_metrics_history WHERE %s < ? AND %s = '00:00'`,
		recordedAtComparableExpr,
		recordedAtMinuteSecondExpr,
	)
	result, errExec := c.SQLDb.ExecContext(c.ctx, query, fmtTime(olderThan))
	if errExec != nil {
		return 0, fmt.Errorf("delete old hourly node metrics history: %w", errExec)
	}
	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("delete old hourly node metrics history rows affected: %w", errRowsAffected)
	}
	return rowsAffected, nil
}

// DeleteGameServerMetricsHistoryOlderThan deletes game server metrics older than the given time.
func (c *Connection) DeleteGameServerMetricsHistoryOlderThan(olderThan time.Time) (int64, error) {
	query := fmt.Sprintf(`DELETE FROM game_server_metrics_history WHERE %s < ?`, recordedAtComparableExpr)
	result, errExec := c.SQLDb.ExecContext(c.ctx, query, fmtTime(olderThan))
	if errExec != nil {
		return 0, fmt.Errorf("delete old game server metrics history: %w", errExec)
	}
	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("delete old game server metrics history rows affected: %w", errRowsAffected)
	}
	return rowsAffected, nil
}

// RollupNodeMetricsToHourly aggregates minute-granularity data older than cutoff into hourly averages,
// then deletes the original minute-level rows.
func (c *Connection) RollupNodeMetricsToHourly(cutoff time.Time) error {
	cutoffStr := fmtTime(cutoff)
	insertQuery := fmt.Sprintf(
		`INSERT INTO node_metrics_history (id, node_id, cpu_percent, memory_percent, memory_used_bytes, memory_total_bytes, disk_percent, disk_used_bytes, disk_total_bytes, game_server_count, running_game_server_count, user_count, recorded_at)
		SELECT
			lower(hex(randomblob(16))),
			node_id,
			AVG(cpu_percent),
			AVG(memory_percent),
			AVG(memory_used_bytes),
			MAX(memory_total_bytes),
			AVG(disk_percent),
			AVG(disk_used_bytes),
			MAX(disk_total_bytes),
			MAX(game_server_count),
			MAX(running_game_server_count),
			MAX(user_count),
			%s || ':00:00'
		FROM node_metrics_history
		WHERE %s < ? AND %s != '00:00'
		GROUP BY node_id, %s
		`,
		recordedAtHourBucketExpr,
		recordedAtComparableExpr,
		recordedAtMinuteSecondExpr,
		recordedAtHourBucketExpr,
	)
	_, errExec := c.SQLDb.ExecContext(c.ctx, insertQuery, cutoffStr)
	if errExec != nil {
		return fmt.Errorf("roll up node metrics to hourly: %w", errExec)
	}

	// Delete the original fine-grained rows that were rolled up.
	// Keep only the hourly aggregates (those with :00:00 seconds).
	deleteQuery := fmt.Sprintf(
		`DELETE FROM node_metrics_history WHERE %s < ? AND %s != '00:00'`,
		recordedAtComparableExpr,
		recordedAtMinuteSecondExpr,
	)
	_, errDelete := c.SQLDb.ExecContext(c.ctx, deleteQuery, cutoffStr)
	if errDelete != nil {
		return fmt.Errorf("delete rolled-up node metrics history: %w", errDelete)
	}
	return nil
}

// RollupGameServerMetricsToHourly aggregates minute-granularity data older than cutoff into hourly averages.
func (c *Connection) RollupGameServerMetricsToHourly(cutoff time.Time) error {
	cutoffStr := fmtTime(cutoff)
	insertQuery := fmt.Sprintf(
		`WITH eligible_metrics AS (
			SELECT
				game_server_metrics_history.*,
				%s || ':00:00' AS hour_bucket,
				ROW_NUMBER() OVER (
					PARTITION BY game_server_id, %s
					ORDER BY %s DESC, recorded_at DESC, id DESC
				) AS latest_rank
			FROM game_server_metrics_history
			WHERE %s < ? AND granularity_seconds < 3600
		)
		INSERT OR REPLACE INTO game_server_metrics_history (
			id, game_server_id, cpu_percent, memory_bytes, memory_percent, disk_usage_bytes,
			io_read_rate, io_write_rate, connection_count, player_count, recorded_at, node_id,
			granularity_seconds, sample_count, available_sample_count, cpu_valid_sample_count,
			query_successful_sample_count, query_duration_valid_sample_count,
			server_fps_valid_sample_count, server_frame_time_valid_sample_count,
			volume_valid_sample_count, io_valid_sample_count,
			connection_valid_sample_count, availability_ratio, collection_status, rollup_hour,
			process_collected_at, cpu_valid, cpu_percent_min, cpu_percent_max, node_cpu_cores,
			memory_bytes_min, memory_bytes_max, memory_percent_min, memory_percent_max,
			node_memory_used_bytes, node_memory_total_bytes, configured_memory_bytes,
			disk_usage_bytes_min, disk_usage_bytes_max, volume_total_bytes, volume_free_bytes,
			volume_percent, volume_valid, disk_measured_at, io_read_rate_min, io_read_rate_max,
			io_write_rate_min, io_write_rate_max, connection_count_min, connection_count_max,
			player_count_min, player_count_max, player_capacity, query_supported, query_success,
			query_duration_ms, query_duration_ms_min, query_duration_ms_max, query_checked_at,
			server_fps, server_fps_min, server_fps_max, server_frame_time_ms,
			server_frame_time_ms_min, server_frame_time_ms_max, server_uptime_seconds,
			process_status, execution_id
		)
		SELECT
			lower(hex(randomblob(16))),
			game_server_id,
			COALESCE(SUM(cpu_percent * cpu_valid_sample_count) /
				NULLIF(SUM(cpu_valid_sample_count), 0), AVG(cpu_percent), 0),
			COALESCE(SUM(memory_bytes * available_sample_count) /
				NULLIF(SUM(available_sample_count), 0), AVG(memory_bytes), 0),
			COALESCE(SUM(memory_percent * available_sample_count) /
				NULLIF(SUM(available_sample_count), 0), AVG(memory_percent), 0),
			COALESCE(SUM(disk_usage_bytes * volume_valid_sample_count) /
				NULLIF(SUM(volume_valid_sample_count), 0), AVG(disk_usage_bytes), 0),
			COALESCE(SUM(io_read_rate * io_valid_sample_count) /
				NULLIF(SUM(io_valid_sample_count), 0), AVG(io_read_rate), 0),
			COALESCE(SUM(io_write_rate * io_valid_sample_count) /
				NULLIF(SUM(io_valid_sample_count), 0), AVG(io_write_rate), 0),
			CAST(ROUND(COALESCE(SUM(connection_count * connection_valid_sample_count) /
				NULLIF(SUM(connection_valid_sample_count), 0), AVG(connection_count), 0)) AS INTEGER),
			CAST(ROUND(COALESCE(SUM(player_count * query_successful_sample_count) /
				NULLIF(SUM(query_successful_sample_count), 0), AVG(player_count), 0)) AS INTEGER),
			hour_bucket,
			MAX(CASE WHEN latest_rank = 1 THEN node_id END),
			3600,
			SUM(sample_count),
			SUM(available_sample_count),
			SUM(cpu_valid_sample_count),
			SUM(query_successful_sample_count),
			SUM(query_duration_valid_sample_count),
			SUM(server_fps_valid_sample_count),
			SUM(server_frame_time_valid_sample_count),
			SUM(volume_valid_sample_count),
			SUM(io_valid_sample_count),
			SUM(connection_valid_sample_count),
			CAST(SUM(available_sample_count) AS REAL) / NULLIF(SUM(sample_count), 0),
			CASE WHEN SUM(available_sample_count) > 0 THEN 'available'
				ELSE MAX(CASE WHEN latest_rank = 1 THEN collection_status END) END,
			hour_bucket,
			MAX(process_collected_at),
			CASE WHEN SUM(cpu_valid_sample_count) > 0 THEN true ELSE false END,
			MIN(CASE WHEN cpu_valid THEN cpu_percent_min END),
			MAX(CASE WHEN cpu_valid THEN cpu_percent_max END),
			MAX(CASE WHEN latest_rank = 1 THEN node_cpu_cores END),
			MIN(CASE WHEN available_sample_count > 0 THEN memory_bytes_min END),
			MAX(CASE WHEN available_sample_count > 0 THEN memory_bytes_max END),
			MIN(CASE WHEN available_sample_count > 0 THEN memory_percent_min END),
			MAX(CASE WHEN available_sample_count > 0 THEN memory_percent_max END),
			CAST(AVG(node_memory_used_bytes) AS INTEGER),
			MAX(CASE WHEN latest_rank = 1 THEN node_memory_total_bytes END),
			MAX(CASE WHEN latest_rank = 1 THEN configured_memory_bytes END),
			MIN(CASE WHEN volume_valid_sample_count > 0 THEN disk_usage_bytes_min END),
			MAX(CASE WHEN volume_valid_sample_count > 0 THEN disk_usage_bytes_max END),
			MAX(CASE WHEN latest_rank = 1 THEN volume_total_bytes END),
			CAST(SUM(volume_free_bytes * volume_valid_sample_count) /
				NULLIF(SUM(CASE WHEN volume_free_bytes IS NOT NULL THEN volume_valid_sample_count ELSE 0 END), 0) AS INTEGER),
			SUM(volume_percent * volume_valid_sample_count) /
				NULLIF(SUM(CASE WHEN volume_percent IS NOT NULL THEN volume_valid_sample_count ELSE 0 END), 0),
			CASE WHEN SUM(volume_valid_sample_count) > 0 THEN true ELSE false END,
			MAX(disk_measured_at),
			MIN(CASE WHEN io_valid_sample_count > 0 THEN io_read_rate_min END),
			MAX(CASE WHEN io_valid_sample_count > 0 THEN io_read_rate_max END),
			MIN(CASE WHEN io_valid_sample_count > 0 THEN io_write_rate_min END),
			MAX(CASE WHEN io_valid_sample_count > 0 THEN io_write_rate_max END),
			MIN(CASE WHEN connection_valid_sample_count > 0 THEN connection_count_min END),
			MAX(CASE WHEN connection_valid_sample_count > 0 THEN connection_count_max END),
			MIN(CASE WHEN query_success THEN player_count_min END),
			MAX(CASE WHEN query_success THEN player_count_max END),
			MAX(CASE WHEN latest_rank = 1 THEN player_capacity END),
			MAX(CASE WHEN latest_rank = 1 THEN query_supported END),
			MAX(CASE WHEN latest_rank = 1 THEN query_success END),
			SUM(query_duration_ms * query_duration_valid_sample_count) /
				NULLIF(SUM(query_duration_valid_sample_count), 0),
			MIN(CASE WHEN query_duration_valid_sample_count > 0 THEN query_duration_ms_min END),
			MAX(CASE WHEN query_duration_valid_sample_count > 0 THEN query_duration_ms_max END),
			MAX(query_checked_at),
			SUM(server_fps * server_fps_valid_sample_count) /
				NULLIF(SUM(server_fps_valid_sample_count), 0),
			MIN(CASE WHEN server_fps_valid_sample_count > 0 THEN server_fps_min END),
			MAX(CASE WHEN server_fps_valid_sample_count > 0 THEN server_fps_max END),
			SUM(server_frame_time_ms * server_frame_time_valid_sample_count) /
				NULLIF(SUM(server_frame_time_valid_sample_count), 0),
			MIN(CASE WHEN server_frame_time_valid_sample_count > 0 THEN server_frame_time_ms_min END),
			MAX(CASE WHEN server_frame_time_valid_sample_count > 0 THEN server_frame_time_ms_max END),
			MAX(CASE WHEN latest_rank = 1 THEN server_uptime_seconds END),
			MAX(CASE WHEN latest_rank = 1 THEN process_status END),
			MAX(CASE WHEN latest_rank = 1 THEN execution_id END)
		FROM eligible_metrics
		GROUP BY game_server_id, hour_bucket
		`,
		recordedAtHourBucketExpr,
		recordedAtHourBucketExpr,
		recordedAtComparableExpr,
		recordedAtComparableExpr,
	)
	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return fmt.Errorf("begin game server metrics rollup transaction: %w", errBegin)
	}
	committed := false
	defer rollbackTxIfNeeded(tx, &committed, "game server metrics rollup")

	_, errExec := tx.ExecContext(c.ctx, insertQuery, cutoffStr)
	if errExec != nil {
		return fmt.Errorf("roll up game server metrics to hourly: %w", errExec)
	}

	deleteQuery := fmt.Sprintf(
		`DELETE FROM game_server_metrics_history WHERE %s < ? AND granularity_seconds < 3600`,
		recordedAtComparableExpr,
	)
	_, errDelete := tx.ExecContext(c.ctx, deleteQuery, cutoffStr)
	if errDelete != nil {
		return fmt.Errorf("delete rolled-up game server metrics history: %w", errDelete)
	}
	errCommit := tx.Commit()
	if errCommit != nil {
		return fmt.Errorf("commit game server metrics rollup transaction: %w", errCommit)
	}
	committed = true
	return nil
}
