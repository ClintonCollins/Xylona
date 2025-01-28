-- +migrate Up
-- Remove null constraints.
create table local_secret_keys_alter_column
(
    id                 integer primary key not null,
    name               text                not null default '',
    secret_key_hash    text                not null,
    last_accessed_from text,
    last_used_at       datetime,
    created_at         datetime            not null default current_timestamp
);

insert into local_secret_keys_alter_column
(id, name, secret_key_hash, last_accessed_from, last_used_at, created_at)
select id, '', secret_key_hash, last_accessed_from, last_used_at, created_at
from local_secret_keys;

drop table local_secret_keys;
alter table local_secret_keys_alter_column
    rename to local_secret_keys;

-- Update node for game servers without a node id to the current local node.
update game_server set node_id = (select id from node where is_local = true limit 1) where node_id = 1;

-- +migrate Down
create table local_secret_keys_alter_column
(
    id                 integer primary key not null,
    secret_key_hash    text                not null,
    last_accessed_from text                not null default '',
    last_used_at       datetime            not null default current_timestamp,
    created_at         datetime            not null default current_timestamp
);
insert into local_secret_keys_alter_column
    (id, secret_key_hash, last_accessed_from, last_used_at, created_at)
select id, secret_key_hash, last_accessed_from, last_used_at, created_at
from local_secret_keys;
drop table local_secret_keys;
alter table local_secret_keys_alter_column
    rename to local_secret_keys;
