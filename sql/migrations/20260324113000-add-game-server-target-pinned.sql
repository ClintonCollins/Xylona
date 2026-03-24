-- +migrate Up
ALTER TABLE game_server
ADD COLUMN target_pinned BOOLEAN NOT NULL DEFAULT 0;

-- +migrate Down
ALTER TABLE game_server
DROP COLUMN target_pinned;
