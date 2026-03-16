-- +migrate Up

alter table federation_local_identity add column cert_pem text not null default '';
alter table federation_local_identity add column key_pem text not null default '';

-- +migrate Down

create table federation_local_identity_old
(
    id               integer primary key not null check (id = 1),
    node_id          text                not null,
    cert_path        text                not null,
    key_path         text                not null,
    cert_fingerprint text                not null,
    created_at       datetime            not null default current_timestamp,
    updated_at       datetime            not null default current_timestamp
);

insert into federation_local_identity_old (id, node_id, cert_path, key_path, cert_fingerprint, created_at, updated_at)
select id, node_id, cert_path, key_path, cert_fingerprint, created_at, updated_at
from federation_local_identity;

drop table federation_local_identity;
alter table federation_local_identity_old rename to federation_local_identity;
