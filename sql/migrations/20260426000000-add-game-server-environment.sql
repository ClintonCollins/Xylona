-- +migrate Up
alter table game_server add column env_vars text not null default '[]';

create table game_server_secret
(
    game_server_id     text     not null references game_server (id) on delete cascade,
    kind               text     not null,
    name               text     not null,
    value_encrypted    text     not null,
    updated_by_user_id text references user (id) on delete set null,
    created_at         datetime not null default current_timestamp,
    updated_at         datetime not null default current_timestamp,
    primary key (game_server_id, kind, name)
);

create index idx_game_server_secret_server_kind
    on game_server_secret (game_server_id, kind);

-- +migrate Down
drop index idx_game_server_secret_server_kind;
drop table game_server_secret;
alter table game_server drop column env_vars;
