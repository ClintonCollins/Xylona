-- +migrate Up

-- Per-server scheduled tasks with cron expressions
CREATE TABLE IF NOT EXISTS scheduled_task (
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

CREATE INDEX IF NOT EXISTS idx_scheduled_task_game_server_id ON scheduled_task(game_server_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_task_enabled ON scheduled_task(enabled);
CREATE UNIQUE INDEX IF NOT EXISTS idx_scheduled_task_server_name ON scheduled_task(game_server_id, name);

-- Execution log for scheduled tasks
CREATE TABLE IF NOT EXISTS scheduled_task_log (
    id TEXT PRIMARY KEY NOT NULL,
    scheduled_task_id TEXT NOT NULL REFERENCES scheduled_task(id) ON DELETE CASCADE,
    game_server_id TEXT NOT NULL,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('success', 'failed', 'skipped', 'timed_out')),
    message TEXT,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scheduled_task_log_task_id ON scheduled_task_log(scheduled_task_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_task_log_game_server ON scheduled_task_log(game_server_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_task_log_created_at ON scheduled_task_log(created_at);
CREATE INDEX IF NOT EXISTS idx_scheduled_task_log_task_created ON scheduled_task_log(scheduled_task_id, created_at DESC);

-- Permission for managing scheduled tasks
INSERT INTO permission (id, name, description)
VALUES ('game_server.scheduled_tasks', 'game_server.scheduled_tasks', 'Manage scheduled tasks for a game server')
ON CONFLICT DO NOTHING;

-- Grant to admin role
INSERT INTO role_permission (role_id, permission_id)
SELECT 'admin', 'game_server.scheduled_tasks'
WHERE NOT EXISTS (
    SELECT 1 FROM role_permission WHERE role_id = 'admin' AND permission_id = 'game_server.scheduled_tasks'
);

-- Grant to operator role
INSERT INTO role_permission (role_id, permission_id)
SELECT 'operator', 'game_server.scheduled_tasks'
WHERE NOT EXISTS (
    SELECT 1 FROM role_permission WHERE role_id = 'operator' AND permission_id = 'game_server.scheduled_tasks'
);

-- +migrate Down

DELETE FROM role_permission WHERE permission_id = 'game_server.scheduled_tasks';
DELETE FROM permission WHERE id = 'game_server.scheduled_tasks';
DROP TABLE IF EXISTS scheduled_task_log;
DROP TABLE IF EXISTS scheduled_task;
