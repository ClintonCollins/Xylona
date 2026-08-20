-- +migrate Up
create table game_server_seven_days_to_die_map_share (
    id text primary key not null,
    game_server_id text not null references game_server(id) on delete cascade,
    token_hash text unique not null,
    token_encrypted text,
    created_by_user_id text references user(id) on delete set null,
    created_at datetime not null default current_timestamp
);

create index idx_game_server_seven_days_to_die_map_share_server_created
    on game_server_seven_days_to_die_map_share(game_server_id, created_at desc);

insert into game_server_seven_days_to_die_map_share (
    id,
    game_server_id,
    token_hash,
    created_by_user_id,
    created_at
)
select
    lower(hex(randomblob(16))),
    game_server_id,
    share_token_hash,
    updated_by_user_id,
    updated_at
from game_server_seven_days_to_die_map
where share_token_hash is not null;

update game_server_seven_days_to_die_map
set share_token_hash = null
where share_token_hash is not null;

-- +migrate Down
update game_server_seven_days_to_die_map
set share_token_hash = (
    select token_hash
    from game_server_seven_days_to_die_map_share
    where game_server_id = game_server_seven_days_to_die_map.game_server_id
    order by created_at desc, id desc
    limit 1
);

drop table game_server_seven_days_to_die_map_share;
