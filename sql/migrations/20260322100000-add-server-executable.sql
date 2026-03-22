-- +migrate Up

ALTER TABLE game_server ADD COLUMN server_executable TEXT DEFAULT NULL;

-- +migrate Down

ALTER TABLE game_server DROP COLUMN server_executable;
