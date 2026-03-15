-- +migrate Up

-- Peer nodes that this node federates with.
-- Separate from the existing "node" table which tracks local node identity.
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

create unique index if not exists peer_node_base_url_unique on peer_node (base_url);
create unique index if not exists peer_node_node_id_unique on peer_node (node_id) where node_id != '';

-- Cached summaries of game servers from remote peer nodes.
create table if not exists remote_server_cache
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

create unique index if not exists remote_server_cache_source_server on remote_server_cache (source_node_id, remote_server_id);
create index if not exists remote_server_cache_peer_node_id on remote_server_cache (peer_node_id);
create index if not exists remote_server_cache_status on remote_server_cache (status);

-- Sync state tracking per peer node.
create table if not exists peer_sync_state
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

create unique index if not exists peer_sync_state_peer_node_id_unique on peer_sync_state (peer_node_id);

-- +migrate Down
drop table if exists peer_sync_state;
drop table if exists remote_server_cache;
drop table if exists peer_node;
