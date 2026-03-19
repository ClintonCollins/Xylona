-- +migrate Up
insert into permission (id, name, description) values
    ('game_server.metrics', 'View Metrics History', 'View historical game server metrics and charts')
on conflict do nothing;

insert into role_permission (role_id, permission_id) values
    ('admin', 'game_server.metrics')
on conflict do nothing;

-- +migrate Down
delete from role_permission where role_id = 'admin' and permission_id = 'game_server.metrics';
delete from permission where id = 'game_server.metrics';
