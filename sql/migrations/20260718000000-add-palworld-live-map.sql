-- +migrate Up
create table game_server_palworld_map (
    game_server_id text primary key not null references game_server (id) on delete cascade,
    share_token_hash text unique,
    layers_json text not null default '[]',
    updated_by_user_id text references user (id) on delete set null,
    created_at timestamp not null default current_timestamp,
    updated_at timestamp not null default current_timestamp
);

-- +migrate Down
drop table game_server_palworld_map;
