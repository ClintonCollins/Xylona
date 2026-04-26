-- +migrate Up
-- Official game definitions are synced from bundled JSON after migrations.
select 1;

-- +migrate Down
-- No-op: this migration no longer owns game seed rows.
select 1;
