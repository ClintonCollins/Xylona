-- +migrate Up

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

-- +migrate Down

drop table if exists federation_pairing_token;
