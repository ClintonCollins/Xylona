-- +migrate Up

alter table federation_local_identity add column key_pem_format text not null default 'plaintext';

-- +migrate Down

create temp table federation_local_identity_key_pem_format_down_guard
(
    id integer primary key not null
);

-- +migrate StatementBegin
create temp trigger federation_local_identity_key_pem_format_down_guard_fail
before insert on federation_local_identity_key_pem_format_down_guard
begin
    select raise(abort, 'down migration unsupported: federation_local_identity.key_pem remains encrypted without the runtime ENCRYPTION_KEY_BASE64; restore a pre-migration backup or rotate the federation identity instead');
end;
-- +migrate StatementEnd

insert into federation_local_identity_key_pem_format_down_guard (id) values (1);
