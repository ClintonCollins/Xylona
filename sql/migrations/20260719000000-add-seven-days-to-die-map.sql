-- +migrate Up
create table game_server_seven_days_to_die_map (
    game_server_id text primary key references game_server(id) on delete cascade,
    share_token_hash text unique,
    notes_json text not null default '[]',
    snapshot_json text not null default '',
    snapshot_at datetime,
    updated_by_user_id text references user(id) on delete set null,
    created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp
);

-- +migrate Down
drop table game_server_seven_days_to_die_map;
