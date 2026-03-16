-- +migrate Up

create table if not exists federation_local_identity
(
    id               integer primary key not null check (id = 1),
    node_id          text                not null,
    cert_path        text                not null,
    key_path         text                not null,
    cert_fingerprint text                not null,
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

-- +migrate Down

drop table if exists federation_trusted_peer;
drop table if exists federation_local_identity;
