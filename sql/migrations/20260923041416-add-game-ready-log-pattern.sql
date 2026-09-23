-- +migrate Up
alter table game add column ready_log_pattern text not null default '';

-- +migrate Down
alter table game drop column ready_log_pattern;
