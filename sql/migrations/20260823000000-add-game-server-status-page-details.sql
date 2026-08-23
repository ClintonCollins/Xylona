-- +migrate Up
alter table game_server add column public_status_note text;
alter table game_server add column public_status_password text;

-- +migrate Down
alter table game_server drop column public_status_password;
alter table game_server drop column public_status_note;
