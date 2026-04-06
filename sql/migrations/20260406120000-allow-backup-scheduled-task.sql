-- +migrate Up

PRAGMA foreign_keys=OFF;

CREATE TABLE scheduled_task_new (
    id TEXT PRIMARY KEY NOT NULL,
    game_server_id TEXT NOT NULL REFERENCES game_server(id) ON DELETE CASCADE,
    created_by TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    task_type TEXT NOT NULL CHECK (task_type IN ('restart', 'console_command', 'backup')),
    cron_expression TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    console_command TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO scheduled_task_new (
    id,
    game_server_id,
    created_by,
    name,
    task_type,
    cron_expression,
    timezone,
    console_command,
    enabled,
    last_run_at,
    next_run_at,
    created_at,
    updated_at
)
SELECT
    id,
    game_server_id,
    created_by,
    name,
    task_type,
    cron_expression,
    timezone,
    console_command,
    enabled,
    last_run_at,
    next_run_at,
    created_at,
    updated_at
FROM scheduled_task;

DROP TABLE scheduled_task;
ALTER TABLE scheduled_task_new RENAME TO scheduled_task;

CREATE INDEX idx_scheduled_task_game_server_id ON scheduled_task(game_server_id);
CREATE INDEX idx_scheduled_task_enabled ON scheduled_task(enabled);
CREATE UNIQUE INDEX idx_scheduled_task_server_name ON scheduled_task(game_server_id, name);

PRAGMA foreign_keys=ON;

-- +migrate Down

PRAGMA foreign_keys=OFF;

DELETE FROM scheduled_task_log
WHERE scheduled_task_id IN (
    SELECT id FROM scheduled_task WHERE task_type = 'backup'
);

CREATE TABLE scheduled_task_old (
    id TEXT PRIMARY KEY NOT NULL,
    game_server_id TEXT NOT NULL REFERENCES game_server(id) ON DELETE CASCADE,
    created_by TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    task_type TEXT NOT NULL CHECK (task_type IN ('restart', 'console_command')),
    cron_expression TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    console_command TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO scheduled_task_old (
    id,
    game_server_id,
    created_by,
    name,
    task_type,
    cron_expression,
    timezone,
    console_command,
    enabled,
    last_run_at,
    next_run_at,
    created_at,
    updated_at
)
SELECT
    id,
    game_server_id,
    created_by,
    name,
    task_type,
    cron_expression,
    timezone,
    console_command,
    enabled,
    last_run_at,
    next_run_at,
    created_at,
    updated_at
FROM scheduled_task
WHERE task_type IN ('restart', 'console_command');

DROP TABLE scheduled_task;
ALTER TABLE scheduled_task_old RENAME TO scheduled_task;

CREATE INDEX idx_scheduled_task_game_server_id ON scheduled_task(game_server_id);
CREATE INDEX idx_scheduled_task_enabled ON scheduled_task(enabled);
CREATE UNIQUE INDEX idx_scheduled_task_server_name ON scheduled_task(game_server_id, name);

PRAGMA foreign_keys=ON;
