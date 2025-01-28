-- +migrate Up
insert into node (id, name, secret_key, is_local, host, port)
values (1, 'localhost', null, true, 'localhost', 8080);

alter table game_server
    add column node_id references node not null default 1;
-- +migrate Down
alter table game_server
    drop column node_id;
delete
from node
where id = 1;