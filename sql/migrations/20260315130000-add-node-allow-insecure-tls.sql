-- +migrate Up

alter table node add column allow_insecure_tls boolean not null default false;

-- +migrate Down

-- SQLite does not support dropping a column without table recreation.
select 1;
