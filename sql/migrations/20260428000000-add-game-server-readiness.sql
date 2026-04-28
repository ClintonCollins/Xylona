-- +migrate Up
create table game_server_readiness (
    game_server_id text not null references game_server (id) on delete cascade,
    kind text not null,
    public_data text not null default '{}',
    updated_by_user_id text references user (id) on delete set null,
    created_at timestamp not null default current_timestamp,
    updated_at timestamp not null default current_timestamp,
    primary key (game_server_id, kind)
);

-- +migrate Down
drop table game_server_readiness;
