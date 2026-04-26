-- +migrate Up
-- No-op: official game metadata is assigned by SyncOfficialDefinitions.
select 1;

-- +migrate Down
-- No-op: this migration no longer owns official game metadata.
select 1;
