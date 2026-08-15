-- +migrate Up
drop table if exists revoked_jwt;

-- +migrate Down
create table revoked_jwt
(
    id                       text primary key not null,
    jwt_id                   text,
    username                 text,
    delete_all_tokens_before datetime,
    created_at               datetime         not null default current_timestamp,

    constraint jwt_id_or_username_check check (jwt_id is not null or username is not null)
);
