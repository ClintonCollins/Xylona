-- +migrate Up

-- Add federation fields to the node table.
alter table node add column base_url text not null default '';
alter table node add column enabled boolean not null default true;
alter table node add column last_seen_at datetime;
alter table node add column last_sync_at datetime;
alter table node add column last_sync_status text not null default '';
alter table node add column health_status text not null default '';
alter table node add column version text not null default '';
alter table node add column protocol_version integer not null default 0;
alter table node add column capabilities text not null default '';
alter table node add column created_at datetime;
alter table node add column updated_at datetime;

-- Set timestamps for existing local nodes.
update node set created_at = datetime('now'), updated_at = datetime('now') where created_at is null;

-- Migrate peer_node data into node table.
-- peer_node records become remote nodes (is_local = false).
insert into node (id, name, secret_key, is_local, host, port, base_url, enabled, last_seen_at, last_sync_at, last_sync_status, health_status, version, protocol_version, capabilities, created_at, updated_at)
select id, name, secret_key, false, '', 0, base_url, enabled, last_seen_at, last_sync_at, last_sync_status, health_status, version, protocol_version, capabilities, created_at, updated_at
from peer_node;

-- Update remote_server_cache to reference node instead of peer_node.
-- The peer_node_id values already match node.id since we inserted with the same id.
-- SQLite does not support ALTER COLUMN or ADD CONSTRAINT, so we recreate the table.
create table remote_server_cache_new
(
    id                   text primary key not null,
    source_node_id       text             not null,
    node_id              text             not null references node (id) on delete cascade,
    remote_server_id     text             not null,
    display_name         text             not null default '',
    status               text             not null default 'UNKNOWN',
    game_name            text             not null default '',
    game_id              text             not null default '',
    ip_address           text             not null default '',
    port                 integer          not null default 0,
    query_port           integer          not null default 0,
    max_players          integer          not null default 0,
    current_players      integer          not null default 0,
    map_name             text             not null default '',
    version              text             not null default '',
    node_name            text             not null default '',
    node_host            text             not null default '',
    last_remote_update   datetime,
    last_synced_at       datetime,
    is_stale             boolean          not null default false,
    raw_metadata         text             not null default '',
    created_at           datetime         not null default current_timestamp,
    updated_at           datetime         not null default current_timestamp
);

insert into remote_server_cache_new (id, source_node_id, node_id, remote_server_id, display_name, status, game_name, game_id, ip_address, port, query_port, max_players, current_players, map_name, version, node_name, node_host, last_remote_update, last_synced_at, is_stale, raw_metadata, created_at, updated_at)
select id, source_node_id, peer_node_id, remote_server_id, display_name, status, game_name, game_id, ip_address, port, query_port, max_players, current_players, map_name, version, node_name, node_host, last_remote_update, last_synced_at, is_stale, raw_metadata, created_at, updated_at
from remote_server_cache;

drop table remote_server_cache;
alter table remote_server_cache_new rename to remote_server_cache;

create unique index if not exists remote_server_cache_source_server on remote_server_cache (source_node_id, remote_server_id);
create index if not exists remote_server_cache_node_id on remote_server_cache (node_id);
create index if not exists remote_server_cache_status on remote_server_cache (status);

-- Update peer_sync_state to reference node instead of peer_node.
create table peer_sync_state_new
(
    id                text primary key not null,
    node_id           text             not null references node (id) on delete cascade,
    last_cursor       text             not null default '',
    last_full_sync_at datetime,
    last_delta_sync_at datetime,
    last_error        text             not null default '',
    retry_count       integer          not null default 0,
    next_retry_at     datetime,
    created_at        datetime         not null default current_timestamp,
    updated_at        datetime         not null default current_timestamp
);

insert into peer_sync_state_new (id, node_id, last_cursor, last_full_sync_at, last_delta_sync_at, last_error, retry_count, next_retry_at, created_at, updated_at)
select id, peer_node_id, last_cursor, last_full_sync_at, last_delta_sync_at, last_error, retry_count, next_retry_at, created_at, updated_at
from peer_sync_state;

drop table peer_sync_state;
alter table peer_sync_state_new rename to peer_sync_state;

create unique index if not exists peer_sync_state_node_id_unique on peer_sync_state (node_id);

-- Drop the old peer_node table.
drop table if exists peer_node;

-- Add useful indexes on node for federation queries.
create index if not exists node_is_local on node (is_local);
create unique index if not exists node_base_url_unique on node (base_url) where base_url != '';

-- +migrate Down

-- Recreate peer_node table.
create table if not exists peer_node
(
    id                text primary key not null,
    node_id           text             not null default '',
    name              text             not null default '',
    base_url          text             not null,
    enabled           boolean          not null default true,
    secret_key        text             not null default '',
    last_seen_at      datetime,
    last_sync_at      datetime,
    last_sync_status  text             not null default '',
    health_status     text             not null default 'unknown',
    version           text             not null default '',
    protocol_version  integer          not null default 0,
    capabilities      text             not null default '',
    created_at        datetime         not null default current_timestamp,
    updated_at        datetime         not null default current_timestamp
);

-- Migrate remote nodes back to peer_node.
insert into peer_node (id, node_id, name, base_url, secret_key, enabled, last_seen_at, last_sync_at, last_sync_status, health_status, version, protocol_version, capabilities, created_at, updated_at)
select id, '', name, base_url, secret_key, enabled, last_seen_at, last_sync_at, last_sync_status, health_status, version, protocol_version, capabilities, created_at, updated_at
from node where is_local = false;

-- Recreate remote_server_cache with peer_node_id FK.
create table remote_server_cache_old
(
    id                   text primary key not null,
    source_node_id       text             not null,
    peer_node_id         text             not null references peer_node (id) on delete cascade,
    remote_server_id     text             not null,
    display_name         text             not null default '',
    status               text             not null default 'UNKNOWN',
    game_name            text             not null default '',
    game_id              text             not null default '',
    ip_address           text             not null default '',
    port                 integer          not null default 0,
    query_port           integer          not null default 0,
    max_players          integer          not null default 0,
    current_players      integer          not null default 0,
    map_name             text             not null default '',
    version              text             not null default '',
    node_name            text             not null default '',
    node_host            text             not null default '',
    last_remote_update   datetime,
    last_synced_at       datetime,
    is_stale             boolean          not null default false,
    raw_metadata         text             not null default '',
    created_at           datetime         not null default current_timestamp,
    updated_at           datetime         not null default current_timestamp
);

insert into remote_server_cache_old select * from remote_server_cache;
drop table remote_server_cache;
alter table remote_server_cache_old rename to remote_server_cache;

create unique index if not exists remote_server_cache_source_server on remote_server_cache (source_node_id, remote_server_id);
create index if not exists remote_server_cache_peer_node_id on remote_server_cache (peer_node_id);
create index if not exists remote_server_cache_status on remote_server_cache (status);

-- Recreate peer_sync_state with peer_node_id FK.
create table peer_sync_state_old
(
    id                text primary key not null,
    peer_node_id      text             not null references peer_node (id) on delete cascade,
    last_cursor       text             not null default '',
    last_full_sync_at datetime,
    last_delta_sync_at datetime,
    last_error        text             not null default '',
    retry_count       integer          not null default 0,
    next_retry_at     datetime,
    created_at        datetime         not null default current_timestamp,
    updated_at        datetime         not null default current_timestamp
);

insert into peer_sync_state_old (id, peer_node_id, last_cursor, last_full_sync_at, last_delta_sync_at, last_error, retry_count, next_retry_at, created_at, updated_at)
select id, node_id, last_cursor, last_full_sync_at, last_delta_sync_at, last_error, retry_count, next_retry_at, created_at, updated_at
from peer_sync_state;

drop table peer_sync_state;
alter table peer_sync_state_old rename to peer_sync_state;

create unique index if not exists peer_sync_state_peer_node_id_unique on peer_sync_state (peer_node_id);

create unique index if not exists peer_node_base_url_unique on peer_node (base_url);
create unique index if not exists peer_node_node_id_unique on peer_node (node_id) where node_id != '';

-- Remove remote nodes from node table.
delete from node where is_local = false;

-- Remove federation columns from node table.
-- SQLite doesn't support DROP COLUMN before 3.35.0, so we recreate.
create table node_old
(
    id         text primary key not null,
    name       text             not null default '',
    secret_key text,
    is_local   boolean          not null default false,
    host       text             not null default '',
    port       integer          not null default 0
);

insert into node_old (id, name, secret_key, is_local, host, port)
select id, name, secret_key, is_local, host, port from node where is_local = true;

drop table node;
alter table node_old rename to node;

-- Re-insert the default local node if needed.
drop index if exists node_is_local;
drop index if exists node_base_url_unique;
