-- +migrate Up

-- SQLite ALTER TABLE cannot change column types, so we rebuild the table to
-- add the new auto-restart columns with the correct BOOLEAN / INTEGER types
-- that bobgen maps to Go bool / int64.

PRAGMA foreign_keys = OFF;

CREATE TABLE game_server_new (
    id                            TEXT PRIMARY KEY             NOT NULL,
    user_id                       TEXT REFERENCES user(id)     NOT NULL,
    name                          TEXT                         NOT NULL,
    game_id                       TEXT REFERENCES game(id)     NOT NULL,
    status                        TEXT                         NOT NULL,
    set_players                   BIGINT                       NOT NULL,
    max_players                   BIGINT                       NOT NULL,
    map                           TEXT                         NOT NULL DEFAULT '',
    ip                            TEXT REFERENCES ip(address)  NOT NULL,
    port                          BIGINT                       NOT NULL,
    query_port                    BIGINT                       NOT NULL DEFAULT 0,
    directory                     TEXT                         NOT NULL,
    max_memory_mb                 BIGINT                       NOT NULL DEFAULT 0,
    backups_enabled               BOOLEAN                      NOT NULL DEFAULT true,
    steam_game_server_login_token TEXT                         NOT NULL DEFAULT '',
    backup_directory              TEXT                         NOT NULL DEFAULT '' CHECK (backups_enabled == 0 OR backup_directory NOT NULL),
    max_backups                   BIGINT                       NOT NULL DEFAULT 0,
    version                       TEXT                         NOT NULL DEFAULT '',
    branch                        TEXT                         NOT NULL DEFAULT 'public',
    created_at                    DATETIME                     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                    DATETIME                     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    node_id                       REFERENCES node              NOT NULL DEFAULT 1,
    server_software               TEXT                         DEFAULT NULL,
    server_executable             TEXT                         DEFAULT NULL,
    target_pinned                 BOOLEAN                      NOT NULL DEFAULT 0,
    start_args_patches            TEXT                         NOT NULL DEFAULT '[]',
    auto_restart_enabled          BOOLEAN                      NOT NULL DEFAULT true,
    auto_restart_max_retries      INTEGER                      NOT NULL DEFAULT 3,
    auto_restart_cooldown_seconds INTEGER                      NOT NULL DEFAULT 30
);

INSERT INTO game_server_new (
    id, user_id, name, game_id, status, set_players, max_players, map, ip,
    port, query_port, directory, max_memory_mb, backups_enabled,
    steam_game_server_login_token, backup_directory, max_backups, version,
    branch, created_at, updated_at, node_id, server_software,
    server_executable, target_pinned, start_args_patches
)
SELECT
    id, user_id, name, game_id, status, set_players, max_players, map, ip,
    port, query_port, directory, max_memory_mb, backups_enabled,
    steam_game_server_login_token, backup_directory, max_backups, version,
    branch, created_at, updated_at, node_id, server_software,
    server_executable, target_pinned, start_args_patches
FROM game_server;

DROP TABLE game_server;
ALTER TABLE game_server_new RENAME TO game_server;

CREATE UNIQUE INDEX IF NOT EXISTS game_server_user_id_name_unique_index ON game_server(user_id, name);

PRAGMA foreign_keys = ON;

-- +migrate Down

-- Table rebuild is not reversible; the added columns are additive and safe to
-- leave in place.
