-- +migrate Up
alter table game add column console_commands text not null default '[]';

-- +migrate Down
alter table game drop column console_commands;
