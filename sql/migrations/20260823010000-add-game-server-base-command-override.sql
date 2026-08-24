-- +migrate Up
alter table game_server add column base_command_override text not null default '';

-- +migrate Down
alter table game_server drop column base_command_override;
