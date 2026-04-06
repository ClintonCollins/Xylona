-- +migrate Up
alter table game_server_backup add column archive_root text not null default '';

-- +migrate Down
PRAGMA foreign_keys=OFF;

CREATE TABLE game_server_backup_old (
    id text primary key,
    game_server_id text not null references game_server(id) on delete cascade,
    node_id text not null references node(id) on delete cascade,
    created_by text,
    trigger_source text not null check (trigger_source in ('manual', 'scheduled')),
    archive_path text not null,
    archive_format text not null check (archive_format = 'zip'),
    status text not null check (status in ('pending', 'completed', 'failed')),
    size_bytes integer not null default 0,
    retention_exempt boolean not null default false,
    error_message text,
    created_at datetime not null,
    completed_at datetime
);

INSERT INTO game_server_backup_old (
    id,
    game_server_id,
    node_id,
    created_by,
    trigger_source,
    archive_path,
    archive_format,
    status,
    size_bytes,
    retention_exempt,
    error_message,
    created_at,
    completed_at
)
SELECT
    id,
    game_server_id,
    node_id,
    created_by,
    trigger_source,
    archive_path,
    archive_format,
    status,
    size_bytes,
    retention_exempt,
    error_message,
    created_at,
    completed_at
FROM game_server_backup;

DROP TABLE game_server_backup;
ALTER TABLE game_server_backup_old RENAME TO game_server_backup;

CREATE INDEX idx_game_server_backup_server_created_at
    on game_server_backup (game_server_id, created_at desc);

CREATE INDEX idx_game_server_backup_server_retention
    on game_server_backup (game_server_id, trigger_source, retention_exempt, status, created_at desc);

CREATE INDEX idx_game_server_backup_node_id
    on game_server_backup (node_id);

PRAGMA foreign_keys=ON;
