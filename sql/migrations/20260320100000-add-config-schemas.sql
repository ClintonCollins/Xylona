-- +migrate Up
ALTER TABLE game ADD COLUMN config_schemas TEXT DEFAULT NULL;

INSERT INTO permission (id, name, description)
VALUES ('game_server.config', 'game_server.config', 'Edit game server configuration files')
ON CONFLICT DO NOTHING;

INSERT INTO role_permission (role_id, permission_id)
SELECT 'admin', 'game_server.config'
WHERE NOT EXISTS (
    SELECT 1 FROM role_permission WHERE role_id = 'admin' AND permission_id = 'game_server.config'
);

-- +migrate Down
DELETE FROM role_permission WHERE permission_id = 'game_server.config';
DELETE FROM permission WHERE id = 'game_server.config';
