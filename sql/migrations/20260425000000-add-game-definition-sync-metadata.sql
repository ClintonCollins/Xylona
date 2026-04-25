-- +migrate Up
alter table game add column official_definition_hash text not null default '';
alter table game add column official_definition_source text not null default '';
alter table game add column official_definition_schema_version bigint not null default 0;
alter table game add column official_definition_diverged boolean not null default false;

-- +migrate Down
alter table game drop column official_definition_diverged;
alter table game drop column official_definition_schema_version;
alter table game drop column official_definition_source;
alter table game drop column official_definition_hash;
