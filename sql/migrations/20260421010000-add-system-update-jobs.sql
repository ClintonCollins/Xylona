-- +migrate Up
CREATE TABLE system_update_job (
    id TEXT PRIMARY KEY,
    component TEXT NOT NULL CHECK (component IN ('controller', 'node')),
    node_id TEXT REFERENCES node(id) ON DELETE SET NULL,
    current_version TEXT NOT NULL DEFAULT '',
    target_version TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT '',
    progress_percent INTEGER NOT NULL DEFAULT 0,
    error TEXT,
    artifact_name TEXT,
    artifact_sha256 TEXT,
    requested_by_user_id TEXT REFERENCES user(id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX idx_system_update_job_node_id ON system_update_job(node_id);
CREATE INDEX idx_system_update_job_status ON system_update_job(status);
CREATE INDEX idx_system_update_job_created_at ON system_update_job(created_at);
CREATE UNIQUE INDEX idx_system_update_job_active_target
ON system_update_job(component, COALESCE(node_id, ''))
WHERE status NOT IN ('succeeded', 'failed');

CREATE TABLE system_update_job_event (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES system_update_job(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT '',
    progress_percent INTEGER NOT NULL DEFAULT 0,
    message TEXT,
    error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_system_update_job_event_job_id ON system_update_job_event(job_id);
CREATE INDEX idx_system_update_job_event_created_at ON system_update_job_event(created_at);

-- +migrate Down
DROP TABLE system_update_job_event;
DROP TABLE system_update_job;
