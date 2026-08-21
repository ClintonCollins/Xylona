-- +migrate Up
create table game_server_status_page_identifier (
    identifier text primary key collate binary,
    created_at datetime not null default current_timestamp
);

create table game_server_status_page (
    user_id text primary key references user(id) on delete cascade,
    public_identifier text not null unique references game_server_status_page_identifier(identifier) on delete restrict,
    title text not null,
    enabled boolean not null default false,
    created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp
);

alter table game_server add column public_connection_address text;

-- +migrate Down
alter table game_server drop column public_connection_address;
drop table game_server_status_page;
drop table game_server_status_page_identifier;
