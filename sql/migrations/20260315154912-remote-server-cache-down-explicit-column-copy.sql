-- +migrate Up

create table remote_server_cache_new
(
    id                 text primary key not null,
    source_node_id     text             not null,
    node_id            text             not null references node (id) on delete cascade,
    remote_server_id   text             not null,
    display_name       text             not null default '',
    status             text             not null default 'UNKNOWN',
    game_name          text             not null default '',
    game_id            text             not null default '',
    ip_address         text             not null default '',
    port               integer          not null default 0,
    query_port         integer          not null default 0,
    max_players        integer          not null default 0,
    current_players    integer          not null default 0,
    map_name           text             not null default '',
    version            text             not null default '',
    node_name          text             not null default '',
    node_host          text             not null default '',
    last_remote_update datetime,
    last_synced_at     datetime,
    is_stale           boolean          not null default false,
    raw_metadata       text             not null default '',
    created_at         datetime         not null default current_timestamp,
    updated_at         datetime         not null default current_timestamp
);

insert into remote_server_cache_new (
    id,
    source_node_id,
    node_id,
    remote_server_id,
    display_name,
    status,
    game_name,
    game_id,
    ip_address,
    port,
    query_port,
    max_players,
    current_players,
    map_name,
    version,
    node_name,
    node_host,
    last_remote_update,
    last_synced_at,
    is_stale,
    raw_metadata,
    created_at,
    updated_at
)
select
    id,
    source_node_id,
    node_id,
    remote_server_id,
    display_name,
    status,
    game_name,
    game_id,
    ip_address,
    port,
    query_port,
    max_players,
    current_players,
    map_name,
    version,
    node_name,
    node_host,
    last_remote_update,
    last_synced_at,
    is_stale,
    raw_metadata,
    created_at,
    updated_at
from remote_server_cache;

drop table remote_server_cache;
alter table remote_server_cache_new rename to remote_server_cache;

create unique index if not exists remote_server_cache_source_server on remote_server_cache (source_node_id, remote_server_id);
create index if not exists remote_server_cache_node_id on remote_server_cache (node_id);
create index if not exists remote_server_cache_status on remote_server_cache (status);

-- +migrate Down

create table remote_server_cache_old
(
    id                 text primary key not null,
    source_node_id     text             not null,
    node_id            text             not null references node (id) on delete cascade,
    remote_server_id   text             not null,
    display_name       text             not null default '',
    status             text             not null default 'UNKNOWN',
    game_name          text             not null default '',
    game_id            text             not null default '',
    ip_address         text             not null default '',
    port               integer          not null default 0,
    query_port         integer          not null default 0,
    max_players        integer          not null default 0,
    current_players    integer          not null default 0,
    map_name           text             not null default '',
    version            text             not null default '',
    node_name          text             not null default '',
    node_host          text             not null default '',
    last_remote_update datetime,
    last_synced_at     datetime,
    is_stale           boolean          not null default false,
    raw_metadata       text             not null default '',
    created_at         datetime         not null default current_timestamp,
    updated_at         datetime         not null default current_timestamp
);

insert into remote_server_cache_old (
    id,
    source_node_id,
    node_id,
    remote_server_id,
    display_name,
    status,
    game_name,
    game_id,
    ip_address,
    port,
    query_port,
    max_players,
    current_players,
    map_name,
    version,
    node_name,
    node_host,
    last_remote_update,
    last_synced_at,
    is_stale,
    raw_metadata,
    created_at,
    updated_at
)
select
    id,
    source_node_id,
    node_id,
    remote_server_id,
    display_name,
    status,
    game_name,
    game_id,
    ip_address,
    port,
    query_port,
    max_players,
    current_players,
    map_name,
    version,
    node_name,
    node_host,
    last_remote_update,
    last_synced_at,
    is_stale,
    raw_metadata,
    created_at,
    updated_at
from remote_server_cache;

drop table remote_server_cache;
alter table remote_server_cache_old rename to remote_server_cache;

create unique index if not exists remote_server_cache_source_server on remote_server_cache (source_node_id, remote_server_id);
create index if not exists remote_server_cache_node_id on remote_server_cache (node_id);
create index if not exists remote_server_cache_status on remote_server_cache (status);
