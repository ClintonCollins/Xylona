-- +migrate Up
create table dns_record_binding (
    game_server_id text primary key not null references game_server(id) on delete cascade,
    relative_name text not null,
    owned_provider_record_id text,
    owned_fqdn text,
    owned_record_type text,
    owned_value text,
    owned_ttl integer,
    constraint dns_record_binding_ownership_check check (
        (
            owned_provider_record_id is null
            and owned_fqdn is null
            and owned_record_type is null
            and owned_value is null
            and owned_ttl is null
        )
        or (
            owned_fqdn is not null
            and owned_record_type is not null
            and owned_record_type in ('A', 'AAAA')
            and owned_value is not null
            and owned_ttl is not null
            and owned_ttl >= 0
        )
    )
);

create unique index dns_record_binding_owned_target_unique_index
    on dns_record_binding (owned_fqdn, owned_record_type)
    where owned_fqdn is not null and owned_record_type is not null;

-- +migrate Down
drop table dns_record_binding;
