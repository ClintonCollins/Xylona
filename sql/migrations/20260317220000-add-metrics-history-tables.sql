-- +migrate Up
CREATE TABLE node_metrics_history (
  id TEXT PRIMARY KEY NOT NULL,
  node_id TEXT NOT NULL REFERENCES node(id) ON DELETE CASCADE,
  cpu_percent REAL NOT NULL DEFAULT 0,
  memory_percent REAL NOT NULL DEFAULT 0,
  memory_used_bytes INTEGER NOT NULL DEFAULT 0,
  memory_total_bytes INTEGER NOT NULL DEFAULT 0,
  disk_percent REAL NOT NULL DEFAULT 0,
  disk_used_bytes INTEGER NOT NULL DEFAULT 0,
  disk_total_bytes INTEGER NOT NULL DEFAULT 0,
  game_server_count INTEGER NOT NULL DEFAULT 0,
  running_game_server_count INTEGER NOT NULL DEFAULT 0,
  user_count INTEGER NOT NULL DEFAULT 0,
  recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_node_metrics_node_time ON node_metrics_history(node_id, recorded_at);

CREATE TABLE game_server_metrics_history (
  id TEXT PRIMARY KEY NOT NULL,
  game_server_id TEXT NOT NULL REFERENCES game_server(id) ON DELETE CASCADE,
  cpu_percent REAL NOT NULL DEFAULT 0,
  memory_bytes INTEGER NOT NULL DEFAULT 0,
  memory_percent REAL NOT NULL DEFAULT 0,
  disk_usage_bytes INTEGER NOT NULL DEFAULT 0,
  io_read_rate REAL NOT NULL DEFAULT 0,
  io_write_rate REAL NOT NULL DEFAULT 0,
  connection_count INTEGER NOT NULL DEFAULT 0,
  player_count INTEGER NOT NULL DEFAULT 0,
  recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_gs_metrics_server_time ON game_server_metrics_history(game_server_id, recorded_at);

-- +migrate Down
DROP TABLE IF EXISTS game_server_metrics_history;
DROP TABLE IF EXISTS node_metrics_history;
