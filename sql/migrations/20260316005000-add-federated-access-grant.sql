-- +migrate Up
create table if not exists federated_access_grant (
    id               text primary key not null,
    game_server_id   text not null references game_server(id) on delete cascade,
    remote_node_id   text not null references node(id) on delete cascade,
    remote_user_id   text not null,
    remote_user_name text not null default '',
    role_id          text not null references role(id) on delete cascade,
    granted_by       text not null references user(id),
    created_at       datetime not null default current_timestamp
);

create unique index if not exists fed_grant_unique
    on federated_access_grant(game_server_id, remote_node_id, remote_user_id, role_id);

-- +migrate Down
drop index if exists fed_grant_unique;
drop table if exists federated_access_grant;
