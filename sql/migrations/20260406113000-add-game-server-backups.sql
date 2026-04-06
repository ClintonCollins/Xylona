-- +migrate Up
create table if not exists game_server_backup (
    id text primary key,
    game_server_id text not null references game_server(id) on delete cascade,
    node_id text not null,
    created_by text,
    trigger_source text not null check (trigger_source in ('manual', 'scheduled')),
    archive_path text not null,
    archive_format text not null check (archive_format = 'zip'),
    status text not null check (status in ('pending', 'completed', 'failed')),
    size_bytes integer not null default 0,
    retention_exempt integer not null default 0 check (retention_exempt in (0, 1)),
    error_message text,
    created_at datetime not null,
    completed_at datetime
);

create index if not exists idx_game_server_backup_server_created_at
    on game_server_backup (game_server_id, created_at desc);

create index if not exists idx_game_server_backup_server_retention
    on game_server_backup (game_server_id, trigger_source, retention_exempt, status, created_at desc);

update game_server
set max_backups = 10
where max_backups <= 0;

-- +migrate Down
drop table if exists game_server_backup;
