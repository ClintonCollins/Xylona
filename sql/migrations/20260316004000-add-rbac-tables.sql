-- +migrate Up
create table if not exists permission (
    id          text primary key not null,
    name        text not null,
    description text not null default ''
);

create table if not exists role (
    id          text primary key not null,
    name        text not null unique,
    description text not null default '',
    is_system   boolean not null default false,
    created_at  datetime not null default current_timestamp
);

create table if not exists role_permission (
    role_id       text not null references role(id) on delete cascade,
    permission_id text not null references permission(id) on delete cascade,
    primary key (role_id, permission_id)
);

create table if not exists user_role_assignment (
    id             text primary key not null,
    user_id        text not null references user(id) on delete cascade,
    role_id        text not null references role(id) on delete cascade,
    game_server_id text references game_server(id) on delete cascade,
    granted_by     text not null references user(id),
    created_at     datetime not null default current_timestamp
);

create unique index if not exists user_role_server_unique
    on user_role_assignment(user_id, role_id, game_server_id);

create unique index if not exists user_role_global_unique
    on user_role_assignment(user_id, role_id) where game_server_id is null;

insert into permission (id, name, description) values
    ('game_server.view', 'View Game Server', 'View game server details and status'),
    ('game_server.start', 'Start Game Server', 'Start a game server'),
    ('game_server.stop', 'Stop Game Server', 'Stop a game server'),
    ('game_server.restart', 'Restart Game Server', 'Restart a game server'),
    ('game_server.console', 'Access Console', 'View and send console commands'),
    ('game_server.files.view', 'View Files', 'Browse game server files'),
    ('game_server.files.edit', 'Edit Files', 'Edit game server files'),
    ('game_server.settings', 'Manage Settings', 'Change game server settings'),
    ('game_server.backup', 'Manage Backups', 'Create and restore backups'),
    ('game_server.delete', 'Delete Game Server', 'Delete a game server')
on conflict do nothing;

insert into role (id, name, description, is_system) values
    ('viewer', 'Viewer', 'Read-only access to game server status and details', true),
    ('operator', 'Operator', 'Can start, stop, restart, and use console', true),
    ('admin', 'Admin', 'Full control over the game server', true)
on conflict do nothing;

insert into role_permission (role_id, permission_id) values
    ('viewer', 'game_server.view')
on conflict do nothing;

insert into role_permission (role_id, permission_id) values
    ('operator', 'game_server.view'),
    ('operator', 'game_server.start'),
    ('operator', 'game_server.stop'),
    ('operator', 'game_server.restart'),
    ('operator', 'game_server.console')
on conflict do nothing;

insert into role_permission (role_id, permission_id) values
    ('admin', 'game_server.view'),
    ('admin', 'game_server.start'),
    ('admin', 'game_server.stop'),
    ('admin', 'game_server.restart'),
    ('admin', 'game_server.console'),
    ('admin', 'game_server.files.view'),
    ('admin', 'game_server.files.edit'),
    ('admin', 'game_server.settings'),
    ('admin', 'game_server.backup'),
    ('admin', 'game_server.delete')
on conflict do nothing;

-- +migrate Down
drop index if exists user_role_global_unique;
drop index if exists user_role_server_unique;
drop table if exists user_role_assignment;
drop table if exists role_permission;
drop table if exists role;
drop table if exists permission;
