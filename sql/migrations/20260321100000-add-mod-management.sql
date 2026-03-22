-- +migrate Up

-- Node-level API keys for external services
CREATE TABLE IF NOT EXISTS node_api_key (
    id TEXT PRIMARY KEY,
    service_name TEXT NOT NULL UNIQUE,
    api_key TEXT NOT NULL,
    created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Per-server installed mod tracking
CREATE TABLE IF NOT EXISTS installed_mod (
    id TEXT PRIMARY KEY,
    game_server_id TEXT NOT NULL REFERENCES game_server(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    source_id TEXT NOT NULL,
    mod_name TEXT NOT NULL,
    mod_author TEXT NOT NULL DEFAULT '',
    installed_version TEXT NOT NULL,
    installed_version_id TEXT NOT NULL DEFAULT '',
    file_hash TEXT NOT NULL DEFAULT '',
    auto_update INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    pinned_version TEXT DEFAULT NULL,
    created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_server_id, source, source_id)
);

-- Files belonging to an installed mod
CREATE TABLE IF NOT EXISTS installed_mod_file (
    id TEXT PRIMARY KEY,
    installed_mod_id TEXT NOT NULL REFERENCES installed_mod(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    file_hash TEXT NOT NULL DEFAULT '',
    file_size INTEGER NOT NULL DEFAULT 0,
    is_primary INTEGER NOT NULL DEFAULT 0
);

-- Server software variant (JSON array of options)
ALTER TABLE game ADD COLUMN server_software TEXT DEFAULT NULL;

-- Active server software on each game server
ALTER TABLE game_server ADD COLUMN server_software TEXT DEFAULT NULL;

-- RBAC permission for mod management
INSERT INTO permission (id, name, description)
VALUES ('game_server.mods', 'game_server.mods', 'Manage mods and plugins on a game server')
ON CONFLICT DO NOTHING;

-- +migrate Down

DELETE FROM permission WHERE id = 'game_server.mods';

-- SQLite doesn't support DROP COLUMN before 3.35.0, but modernc.org/sqlite should support it
-- If not, these can be no-ops since down migrations are rarely used in practice
ALTER TABLE game_server DROP COLUMN server_software;
ALTER TABLE game DROP COLUMN server_software;

DROP TABLE IF EXISTS installed_mod_file;
DROP TABLE IF EXISTS installed_mod;
DROP TABLE IF EXISTS node_api_key;
