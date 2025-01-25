-- +migrate Up
alter table game
    add column xylona_official boolean not null default false;
alter table ip
    add column automatically_added boolean not null default false;

create table revoked_jwt
(
    id                       text primary key not null,
    jwt_id                   text,
    username                 text,
    delete_all_tokens_before datetime,
    created_at               datetime         not null default current_timestamp,

    constraint jwt_id_or_username_check
        check ( jwt_id is not null or username is not null )
);

create table local_settings
(
    id      integer primary key not null,
    node_id text                not null
);

create table local_secret_keys
(
    id                 integer primary key not null,
    secret_key_hash    text                not null,
    last_accessed_from text                not null,
    last_used_at       datetime            not null,
    created_at         datetime            not null
);

create table node
(
    id         text primary key not null,
    name       text             not null,
    secret_key text,
    is_local   boolean          not null default false,
    host       text             not null,
    rpc_port   integer          not null,
    web_port   integer          not null
);

create table node_sync_queue
(
    id           integer primary key  not null,
    node_id      text references node not null,
    action_json  text                 not null,
    created_at   datetime             not null,
    succeeded_at datetime
);

-- +migrate Down
alter table game
    drop column xylona_official;
alter table ip
    drop column automatically_added;
drop table revoked_jwt;
drop table local_settings;
drop table local_secret_keys;
drop table node;
drop table node_sync_queue;