-- +migrate Up

-- Hub-spoke migration step 5: rip-and-replace of all federation tables and
-- a complete redesign of the "node" table to hold only what the controller
-- needs to dial a node (id, name, listen_url, cert_fingerprint,
-- shared_secret_encrypted, enabled, last_seen_at, timestamps).
--
-- TODO(hub-spoke step 6): on controller startup, insert the self-node row
-- using a fixed node id and rebind any pre-existing game_server.node_id
-- values to that id. This migration intentionally drops every row in "node",
-- which can leave game_server.node_id values referencing ids that no longer
-- exist until step 6 lands. That is acceptable because this is a rip-and-
-- replace migration on a development schema; no production deployments
-- depend on the old mesh data.

-- Drop dependent federation tables first so foreign keys pointing at "node"
-- don't block the node rebuild.
drop table if exists federated_access_grant;
drop table if exists federation_advisory;
drop table if exists federation_pairing_token;
drop table if exists federation_trusted_peer;
drop table if exists federation_local_identity;
drop table if exists peer_sync_state;
drop table if exists remote_server_cache;
drop table if exists node_sync_queue;
drop table if exists node_api_key;
drop table if exists local_secret_keys;

-- Rebuild the node table with the hub-spoke shape. SQLite can't drop a
-- dozen columns at once cleanly, so rename-swap is the safe path.
drop table if exists node_hub_spoke_new;
create table node_hub_spoke_new
(
    id                      text primary key not null,
    name                    text             not null default '',
    listen_url              text             not null default '',
    cert_fingerprint        text             not null default '',
    shared_secret_encrypted text             not null default '',
    enabled                 boolean          not null default true,
    last_seen_at            datetime,
    created_at              datetime         not null default current_timestamp,
    updated_at              datetime         not null default current_timestamp
);

drop table if exists node;
alter table node_hub_spoke_new rename to node;

-- Join-token bootstrap: one-shot tokens a node consumes to register itself
-- and receive its long-lived shared secret.
create table if not exists node_join_token
(
    id                  text primary key not null,
    token_hash          text             not null,
    node_name           text             not null default '',
    created_at          datetime         not null default current_timestamp,
    expires_at          datetime,
    consumed_at         datetime,
    consumed_by_node_id text
);

create index if not exists node_join_token_token_hash_idx on node_join_token (token_hash);

-- +migrate Down

-- Down migration restores only enough shape for compile/migrate round-trips.
-- None of the historical federation data is reconstructed.

drop index if exists node_join_token_token_hash_idx;
drop table if exists node_join_token;

-- Rebuild the pre-hub-spoke "node" table shape (columns accumulated over
-- earlier migrations: initial + consolidate + allow_insecure_tls +
-- sync_interval_seconds + federation_advisory + os).
drop table if exists node_hub_spoke_old;
create table node_hub_spoke_old
(
    id                    text primary key not null,
    name                  text             not null default '',
    secret_key            text,
    is_local              boolean          not null default false,
    host                  text             not null default '',
    port                  integer          not null default 0,
    base_url              text             not null default '',
    enabled               boolean          not null default true,
    last_seen_at          datetime,
    last_sync_at          datetime,
    last_sync_status      text             not null default '',
    health_status         text             not null default '',
    version               text             not null default '',
    protocol_version      integer          not null default 0,
    capabilities          text             not null default '',
    sync_interval_seconds integer          not null default 60,
    allow_insecure_tls    boolean          not null default false,
    departed              boolean          not null default false,
    auto_paired           boolean          not null default false,
    os                    text             not null default '',
    created_at            datetime         not null default current_timestamp,
    updated_at            datetime         not null default current_timestamp
);

drop table if exists node;
alter table node_hub_spoke_old rename to node;

-- Recreate the federation tables with enough shape for sql-migrate down/up
-- cycles. Column sets mirror the latest pre-rewipe migrations.
create table if not exists local_secret_keys
(
    id                 integer primary key not null,
    secret_key_hash    text                not null,
    last_accessed_from text                not null,
    last_used_at       datetime            not null,
    created_at         datetime            not null,
    name               text                not null default ''
);

create table if not exists node_api_key
(
    id           text primary key not null,
    service_name text             not null unique,
    api_key      text             not null,
    created_at   datetime         not null default current_timestamp,
    updated_at   datetime         not null default current_timestamp
);

create table if not exists node_sync_queue
(
    id           integer primary key           not null,
    node_id      text references node          not null,
    action_json  text                          not null,
    created_at   datetime                      not null,
    succeeded_at datetime
);

create table if not exists remote_server_cache
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

create unique index if not exists remote_server_cache_source_server on remote_server_cache (source_node_id, remote_server_id);
create index if not exists remote_server_cache_node_id on remote_server_cache (node_id);
create index if not exists remote_server_cache_status on remote_server_cache (status);

create table if not exists peer_sync_state
(
    id                 text primary key not null,
    node_id            text             not null references node (id) on delete cascade,
    last_cursor        text             not null default '',
    last_full_sync_at  datetime,
    last_delta_sync_at datetime,
    last_error         text             not null default '',
    retry_count        integer          not null default 0,
    next_retry_at      datetime,
    created_at         datetime         not null default current_timestamp,
    updated_at         datetime         not null default current_timestamp
);

create unique index if not exists peer_sync_state_node_id_unique on peer_sync_state (node_id);

create table if not exists federation_local_identity
(
    id               integer primary key not null check (id = 1),
    node_id          text                not null,
    cert_path        text                not null,
    key_path         text                not null,
    cert_fingerprint text                not null,
    cert_pem         text                not null default '',
    key_pem          text                not null default '',
    key_pem_format   text                not null default 'plaintext',
    created_at       datetime            not null default current_timestamp,
    updated_at       datetime            not null default current_timestamp
);

create table if not exists federation_trusted_peer
(
    node_id          text primary key not null references node (id) on delete cascade,
    peer_node_id     text             not null default '',
    peer_fingerprint text             not null,
    enabled          boolean          not null default true,
    revoked          boolean          not null default false,
    created_at       datetime         not null default current_timestamp,
    updated_at       datetime         not null default current_timestamp
);

create unique index if not exists federation_trusted_peer_peer_fingerprint_unique on federation_trusted_peer (peer_fingerprint);
create unique index if not exists federation_trusted_peer_peer_node_id_unique on federation_trusted_peer (peer_node_id) where peer_node_id != '';

create table if not exists federation_pairing_token
(
    id         text primary key not null,
    token_hash text             not null,
    target_url text             not null default '',
    created_at datetime         not null default current_timestamp,
    expires_at datetime         not null,
    used       boolean          not null default false
);

create index if not exists federation_pairing_token_token_hash_idx on federation_pairing_token (token_hash);

create table if not exists federation_advisory
(
    id                    text primary key,
    type                  text     not null,
    title                 text     not null,
    message               text     not null,
    source_node_id        text     not null default '',
    source_node_name      text     not null default '',
    subject_node_id       text     not null default '',
    subject_node_name     text     not null default '',
    subject_node_base_url text     not null default '',
    read                  boolean  not null default false,
    created_at            datetime not null default current_timestamp
);

create table if not exists federated_access_grant
(
    id               text primary key not null,
    game_server_id   text             not null references game_server (id) on delete cascade,
    remote_node_id   text             not null references node (id) on delete cascade,
    remote_user_id   text             not null,
    remote_user_name text             not null default '',
    role_id          text             not null references role (id) on delete cascade,
    granted_by       text             not null references user (id),
    created_at       datetime         not null default current_timestamp
);

create unique index if not exists fed_grant_unique
    on federated_access_grant (game_server_id, remote_node_id, remote_user_id, role_id);
