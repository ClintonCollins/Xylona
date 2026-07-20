-- +migrate Up
create table game_server_minecraft_map (
    game_server_id text primary key not null references game_server(id) on delete cascade,
    enabled integer not null default 0,
    world_name text not null default 'world',
    share_token_hash text unique,
    accepted_at datetime,
    updated_by_user_id text references user(id) on delete set null,
    created_at datetime not null,
    updated_at datetime not null
);

-- +migrate Down
drop table game_server_minecraft_map;
