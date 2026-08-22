-- +migrate Up
create table game_server_map_share (
    game_server_id text primary key not null references game_server(id) on delete cascade,
    public_identifier text collate binary not null unique,
    enabled boolean not null default false
);

update game_server_palworld_map
set share_token_hash = null
where share_token_hash is not null;

update game_server_minecraft_map
set share_token_hash = null
where share_token_hash is not null;

delete from game_server_seven_days_to_die_map_share;

-- +migrate Down
drop table game_server_map_share;
