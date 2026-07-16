-- +migrate Up
insert into permission (id, name, description)
values ('game_server.players.manage', 'Manage Players', 'Kick, ban, unban, and manage game server allowlists');

insert into role_permission (role_id, permission_id) values
    ('operator', 'game_server.players.manage'),
    ('admin', 'game_server.players.manage');

-- +migrate Down
delete from role_permission where permission_id = 'game_server.players.manage';
delete from permission where id = 'game_server.players.manage';
