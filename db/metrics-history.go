package db

import (
	"time"
)

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
	ID              string
	GameServerID    string
	CPUPercent      float64
	MemoryBytes     int64
	MemoryPercent   float64
	DiskUsageBytes  int64
	IOReadRate      float64
	IOWriteRate     float64
	ConnectionCount int
	PlayerCount     int
	RecordedAt      time.Time
}

// InsertNodeMetricsHistory inserts a node metrics snapshot.
func (c *Connection) InsertNodeMetricsHistory(row *NodeMetricsRow) error {
	_, errExec := c.SQLDb.ExecContext(c.ctx,
		`INSERT INTO node_metrics_history (id, node_id, cpu_percent, memory_percent, memory_used_bytes, memory_total_bytes, disk_percent, disk_used_bytes, disk_total_bytes, game_server_count, running_game_server_count, user_count, recorded_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID, row.NodeID, row.CPUPercent, row.MemoryPercent, row.MemoryUsedBytes, row.MemoryTotalBytes,
		row.DiskPercent, row.DiskUsedBytes, row.DiskTotalBytes, row.GameServerCount, row.RunningGameServerCount,
		row.UserCount, row.RecordedAt,
	)
	return errExec
}

// InsertGameServerMetricsHistory inserts a game server metrics snapshot.
func (c *Connection) InsertGameServerMetricsHistory(row *GameServerMetricsRow) error {
	_, errExec := c.SQLDb.ExecContext(c.ctx,
		`INSERT INTO game_server_metrics_history (id, game_server_id, cpu_percent, memory_bytes, memory_percent, disk_usage_bytes, io_read_rate, io_write_rate, connection_count, player_count, recorded_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID, row.GameServerID, row.CPUPercent, row.MemoryBytes, row.MemoryPercent,
		row.DiskUsageBytes, row.IOReadRate, row.IOWriteRate, row.ConnectionCount, row.PlayerCount,
		row.RecordedAt,
	)
	return errExec
}

// GetNodeMetricsHistory returns node metrics within the given time range.
func (c *Connection) GetNodeMetricsHistory(nodeID string, since, until time.Time) ([]*NodeMetricsRow, error) {
	rows, errQuery := c.SQLDb.QueryContext(c.ctx,
		`SELECT id, node_id, cpu_percent, memory_percent, memory_used_bytes, memory_total_bytes, disk_percent, disk_used_bytes, disk_total_bytes, game_server_count, running_game_server_count, user_count, recorded_at FROM node_metrics_history WHERE node_id = ? AND recorded_at >= ? AND recorded_at <= ? ORDER BY recorded_at ASC`,
		nodeID, since, until,
	)
	if errQuery != nil {
		return nil, errQuery
	}
	defer rows.Close()

	var results []*NodeMetricsRow
	for rows.Next() {
		row := &NodeMetricsRow{}
		errScan := rows.Scan(
			&row.ID, &row.NodeID, &row.CPUPercent, &row.MemoryPercent, &row.MemoryUsedBytes,
			&row.MemoryTotalBytes, &row.DiskPercent, &row.DiskUsedBytes, &row.DiskTotalBytes,
			&row.GameServerCount, &row.RunningGameServerCount, &row.UserCount, &row.RecordedAt,
		)
		if errScan != nil {
			return nil, errScan
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// GetGameServerMetricsHistory returns game server metrics within the given time range.
func (c *Connection) GetGameServerMetricsHistory(gameServerID string, since, until time.Time) ([]*GameServerMetricsRow, error) {
	rows, errQuery := c.SQLDb.QueryContext(c.ctx,
		`SELECT id, game_server_id, cpu_percent, memory_bytes, memory_percent, disk_usage_bytes, io_read_rate, io_write_rate, connection_count, player_count, recorded_at FROM game_server_metrics_history WHERE game_server_id = ? AND recorded_at >= ? AND recorded_at <= ? ORDER BY recorded_at ASC`,
		gameServerID, since, until,
	)
	if errQuery != nil {
		return nil, errQuery
	}
	defer rows.Close()

	var results []*GameServerMetricsRow
	for rows.Next() {
		row := &GameServerMetricsRow{}
		errScan := rows.Scan(
			&row.ID, &row.GameServerID, &row.CPUPercent, &row.MemoryBytes, &row.MemoryPercent,
			&row.DiskUsageBytes, &row.IOReadRate, &row.IOWriteRate, &row.ConnectionCount,
			&row.PlayerCount, &row.RecordedAt,
		)
		if errScan != nil {
			return nil, errScan
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// DeleteNodeMetricsHistoryOlderThan deletes node metrics older than the given time.
func (c *Connection) DeleteNodeMetricsHistoryOlderThan(olderThan time.Time) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(c.ctx,
		`DELETE FROM node_metrics_history WHERE recorded_at < ?`, olderThan,
	)
	if errExec != nil {
		return 0, errExec
	}
	return result.RowsAffected()
}

// DeleteGameServerMetricsHistoryOlderThan deletes game server metrics older than the given time.
func (c *Connection) DeleteGameServerMetricsHistoryOlderThan(olderThan time.Time) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(c.ctx,
		`DELETE FROM game_server_metrics_history WHERE recorded_at < ?`, olderThan,
	)
	if errExec != nil {
		return 0, errExec
	}
	return result.RowsAffected()
}

// RollupNodeMetricsToHourly aggregates minute-granularity data older than cutoff into hourly averages,
// then deletes the original minute-level rows.
func (c *Connection) RollupNodeMetricsToHourly(cutoff time.Time) error {
	_, errExec := c.SQLDb.ExecContext(c.ctx,
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
			strftime('%Y-%m-%d %H:00:00', recorded_at)
		FROM node_metrics_history
		WHERE recorded_at < ?
		GROUP BY node_id, strftime('%Y-%m-%d %H', recorded_at)
		HAVING COUNT(*) > 1`,
		cutoff,
	)
	if errExec != nil {
		return errExec
	}

	// Delete the original fine-grained rows that were rolled up.
	// Keep only the hourly aggregates (those with :00:00 seconds).
	_, errDelete := c.SQLDb.ExecContext(c.ctx,
		`DELETE FROM node_metrics_history WHERE recorded_at < ? AND strftime('%M:%S', recorded_at) != '00:00'`,
		cutoff,
	)
	return errDelete
}

// RollupGameServerMetricsToHourly aggregates minute-granularity data older than cutoff into hourly averages.
func (c *Connection) RollupGameServerMetricsToHourly(cutoff time.Time) error {
	_, errExec := c.SQLDb.ExecContext(c.ctx,
		`INSERT INTO game_server_metrics_history (id, game_server_id, cpu_percent, memory_bytes, memory_percent, disk_usage_bytes, io_read_rate, io_write_rate, connection_count, player_count, recorded_at)
		SELECT
			lower(hex(randomblob(16))),
			game_server_id,
			AVG(cpu_percent),
			AVG(memory_bytes),
			AVG(memory_percent),
			AVG(disk_usage_bytes),
			AVG(io_read_rate),
			AVG(io_write_rate),
			MAX(connection_count),
			MAX(player_count),
			strftime('%Y-%m-%d %H:00:00', recorded_at)
		FROM game_server_metrics_history
		WHERE recorded_at < ?
		GROUP BY game_server_id, strftime('%Y-%m-%d %H', recorded_at)
		HAVING COUNT(*) > 1`,
		cutoff,
	)
	if errExec != nil {
		return errExec
	}

	_, errDelete := c.SQLDb.ExecContext(c.ctx,
		`DELETE FROM game_server_metrics_history WHERE recorded_at < ? AND strftime('%M:%S', recorded_at) != '00:00'`,
		cutoff,
	)
	return errDelete
}
