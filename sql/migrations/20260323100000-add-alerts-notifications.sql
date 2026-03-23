-- +migrate Up

-- System-wide admin settings (encrypted values for sensitive data)
CREATE TABLE IF NOT EXISTS system_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Reusable notification delivery destinations owned by a user
CREATE TABLE IF NOT EXISTS notification_channel (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    channel_type TEXT NOT NULL,
    config TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_notification_channel_user_id ON notification_channel(user_id);

-- Per-user, per-server alert rules
CREATE TABLE IF NOT EXISTS alert_rule (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    server_id TEXT,
    server_node_id TEXT,
    node_id TEXT REFERENCES node(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    condition TEXT,
    notification_channel_id TEXT NOT NULL REFERENCES notification_channel(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        (server_id IS NULL AND server_node_id IS NULL) OR
        (server_id IS NOT NULL AND server_node_id IS NOT NULL)
    ),
    CHECK (
        NOT (server_id IS NOT NULL AND node_id IS NOT NULL)
    )
);

CREATE INDEX idx_alert_rule_user_id ON alert_rule(user_id);
CREATE INDEX idx_alert_rule_server ON alert_rule(server_id, server_node_id);
CREATE INDEX idx_alert_rule_event_type ON alert_rule(event_type);

-- Threshold deduplication state — one row per rule per entity
CREATE TABLE IF NOT EXISTS alert_state (
    id TEXT PRIMARY KEY,
    alert_rule_id TEXT NOT NULL REFERENCES alert_rule(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    entity_node_id TEXT NOT NULL,
    triggered INTEGER NOT NULL DEFAULT 0,
    triggered_at datetime,
    resolved_at datetime
);

CREATE UNIQUE INDEX idx_alert_state_unique ON alert_state(alert_rule_id, entity_type, entity_id, entity_node_id);

-- Log of fired alerts
CREATE TABLE IF NOT EXISTS alert_history (
    id TEXT PRIMARY KEY,
    alert_rule_id TEXT REFERENCES alert_rule(id) ON DELETE SET NULL,
    user_id TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    server_id TEXT,
    server_node_id TEXT,
    node_id TEXT,
    event_type TEXT NOT NULL,
    event_data TEXT,
    channel_type TEXT NOT NULL,
    delivery_status TEXT NOT NULL DEFAULT 'pending',
    delivery_error TEXT,
    created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_alert_history_user_id ON alert_history(user_id);
CREATE INDEX idx_alert_history_server ON alert_history(server_id, server_node_id);
CREATE INDEX idx_alert_history_created_at ON alert_history(created_at);

-- RBAC permissions for alert management
INSERT INTO permission (id, name, description)
VALUES ('alerts.manage', 'alerts.manage', 'Manage alert rules and notification channels')
ON CONFLICT DO NOTHING;

INSERT INTO permission (id, name, description)
VALUES ('alerts.view_history', 'alerts.view_history', 'View alert notification history')
ON CONFLICT DO NOTHING;

-- Grant to admin role
INSERT INTO role_permission (role_id, permission_id)
SELECT 'admin', 'alerts.manage'
WHERE NOT EXISTS (
    SELECT 1 FROM role_permission WHERE role_id = 'admin' AND permission_id = 'alerts.manage'
);

INSERT INTO role_permission (role_id, permission_id)
SELECT 'admin', 'alerts.view_history'
WHERE NOT EXISTS (
    SELECT 1 FROM role_permission WHERE role_id = 'admin' AND permission_id = 'alerts.view_history'
);

-- +migrate Down

DELETE FROM role_permission WHERE permission_id IN ('alerts.manage', 'alerts.view_history');
DELETE FROM permission WHERE id IN ('alerts.manage', 'alerts.view_history');
DROP TABLE IF EXISTS alert_history;
DROP TABLE IF EXISTS alert_state;
DROP TABLE IF EXISTS alert_rule;
DROP TABLE IF EXISTS notification_channel;
DROP TABLE IF EXISTS system_config;
