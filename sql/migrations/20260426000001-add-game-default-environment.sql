-- +migrate Up
alter table game add column default_env_vars text not null default '[]';

-- +migrate Down
alter table game drop column default_env_vars;
