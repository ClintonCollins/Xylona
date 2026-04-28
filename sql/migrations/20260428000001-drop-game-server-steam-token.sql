-- +migrate Up
alter table game_server drop column steam_game_server_login_token;

-- +migrate Down
alter table game_server add column steam_game_server_login_token text not null default '';
