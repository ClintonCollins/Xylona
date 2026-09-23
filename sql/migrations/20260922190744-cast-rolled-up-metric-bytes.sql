-- +migrate Up
-- Hourly rollups used to store AVG() byte values as REAL, which cannot be scanned into int64.
update node_metrics_history
set memory_used_bytes = cast(round(memory_used_bytes) as integer),
    disk_used_bytes = cast(round(disk_used_bytes) as integer)
where typeof(memory_used_bytes) = 'real'
   or typeof(disk_used_bytes) = 'real';

update game_server_metrics_history
set memory_bytes = cast(round(memory_bytes) as integer),
    disk_usage_bytes = cast(round(disk_usage_bytes) as integer)
where typeof(memory_bytes) = 'real'
   or typeof(disk_usage_bytes) = 'real';

-- +migrate Down
-- Data normalization only; nothing to undo.
