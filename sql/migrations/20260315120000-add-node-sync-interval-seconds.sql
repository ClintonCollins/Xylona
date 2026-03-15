-- +migrate Up

alter table node add column sync_interval_seconds integer not null default 60;

-- +migrate Down

-- SQLite does not support dropping a column without table recreation.
select 1;
