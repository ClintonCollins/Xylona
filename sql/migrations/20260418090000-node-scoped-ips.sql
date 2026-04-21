-- +migrate Up

pragma foreign_keys = off;
pragma legacy_alter_table = on;

alter table ip
    rename to ip_node_scoped_old;

drop index if exists idx_ip_node_id;

create table ip
(
    address             text    not null,
    usable              boolean not null default true,
    external            boolean not null default false,
    automatically_added boolean not null default false,
    node_id             text    not null references node (id),
    primary key (address, node_id)
);

insert into ip (address, usable, external, automatically_added, node_id)
select address,
       usable,
       external,
       automatically_added,
       (select node_id from local_settings where id = 1)
from ip_node_scoped_old;

create index if not exists idx_ip_node_id
    on ip (node_id);

alter table game_server
    rename to game_server_node_scoped_old;

drop index if exists game_server_user_id_name_unique_index;

create table game_server
(
    id                            text primary key not null,
    user_id                       text             not null references user (id),
    name                          text             not null,
    game_id                       text             not null references game (id),
    status                        text             not null,
    set_players                   bigint           not null,
    max_players                   bigint           not null,
    map                           text             not null default '',
    ip                            text             not null,
    port                          bigint           not null,
    query_port                    bigint           not null default 0,
    directory                     text             not null,
    max_memory_mb                 bigint           not null default 0,
    backups_enabled               boolean          not null default true,
    steam_game_server_login_token text             not null default '',
    backup_directory              text             not null default '',
    max_backups                   bigint           not null default 0,
    version                       text             not null default '',
    branch                        text             not null default 'public',
    created_at                    datetime         not null default current_timestamp,
    updated_at                    datetime         not null default current_timestamp,
    node_id                       text             not null references node (id),
    server_software               text,
    server_executable             text,
    target_pinned                 boolean          not null default 0,
    start_args_patches            text             not null default '[]',
    auto_restart_enabled          boolean          not null default true,
    auto_restart_max_retries      integer          not null default 3,
    auto_restart_cooldown_seconds integer          not null default 30,
    foreign key (ip, node_id) references ip (address, node_id)
);

insert into game_server (
    id,
    user_id,
    name,
    game_id,
    status,
    set_players,
    max_players,
    map,
    ip,
    port,
    query_port,
    directory,
    max_memory_mb,
    backups_enabled,
    steam_game_server_login_token,
    backup_directory,
    max_backups,
    version,
    branch,
    created_at,
    updated_at,
    node_id,
    server_software,
    server_executable,
    target_pinned,
    start_args_patches,
    auto_restart_enabled,
    auto_restart_max_retries,
    auto_restart_cooldown_seconds
)
select id,
       user_id,
       name,
       game_id,
       status,
       set_players,
       max_players,
       map,
       ip,
       port,
       query_port,
       directory,
       max_memory_mb,
       backups_enabled,
       steam_game_server_login_token,
       backup_directory,
       max_backups,
       version,
       branch,
       created_at,
       updated_at,
       node_id,
       server_software,
       server_executable,
       target_pinned,
       start_args_patches,
       auto_restart_enabled,
       auto_restart_max_retries,
       auto_restart_cooldown_seconds
from game_server_node_scoped_old;

create unique index if not exists game_server_user_id_name_unique_index
    on game_server (user_id, name);

pragma writable_schema = on;

update sqlite_schema
set sql = replace(sql, 'game_server_node_scoped_old', 'game_server')
where type = 'table'
  and name in (
      'game_server_backup',
      'game_server_metrics_history',
      'installed_mod',
      'scheduled_task',
      'user_role_assignment',
      'federated_access_grant'
  );

pragma writable_schema = off;

drop table game_server_node_scoped_old;
drop table ip_node_scoped_old;

pragma legacy_alter_table = off;
pragma foreign_keys = on;

-- +migrate Down

pragma foreign_keys = off;
pragma legacy_alter_table = on;

alter table game_server
    rename to game_server_node_scoped_new;

drop index if exists game_server_user_id_name_unique_index;

alter table ip
    rename to ip_node_scoped_new;

drop index if exists idx_ip_node_id;

create table ip
(
    address             text primary key not null,
    usable              boolean          not null default true,
    external            boolean          not null default false,
    automatically_added boolean          not null default false
);

insert into ip (address, usable, external, automatically_added)
select address,
       max(cast(usable as integer)),
       max(cast(external as integer)),
       max(cast(automatically_added as integer))
from ip_node_scoped_new
group by address;

create table game_server
(
    id                            text primary key not null,
    user_id                       text             not null references user (id),
    name                          text             not null,
    game_id                       text             not null references game (id),
    status                        text             not null,
    set_players                   bigint           not null,
    max_players                   bigint           not null,
    map                           text             not null default '',
    ip                            text             not null references ip (address),
    port                          bigint           not null,
    query_port                    bigint           not null default 0,
    directory                     text             not null,
    max_memory_mb                 bigint           not null default 0,
    backups_enabled               boolean          not null default true,
    steam_game_server_login_token text             not null default '',
    backup_directory              text             not null default '',
    max_backups                   bigint           not null default 0,
    version                       text             not null default '',
    branch                        text             not null default 'public',
    created_at                    datetime         not null default current_timestamp,
    updated_at                    datetime         not null default current_timestamp,
    node_id                       text             not null references node (id),
    server_software               text,
    server_executable             text,
    target_pinned                 boolean          not null default 0,
    start_args_patches            text             not null default '[]',
    auto_restart_enabled          boolean          not null default true,
    auto_restart_max_retries      integer          not null default 3,
    auto_restart_cooldown_seconds integer          not null default 30
);

insert into game_server (
    id,
    user_id,
    name,
    game_id,
    status,
    set_players,
    max_players,
    map,
    ip,
    port,
    query_port,
    directory,
    max_memory_mb,
    backups_enabled,
    steam_game_server_login_token,
    backup_directory,
    max_backups,
    version,
    branch,
    created_at,
    updated_at,
    node_id,
    server_software,
    server_executable,
    target_pinned,
    start_args_patches,
    auto_restart_enabled,
    auto_restart_max_retries,
    auto_restart_cooldown_seconds
)
select id,
       user_id,
       name,
       game_id,
       status,
       set_players,
       max_players,
       map,
       ip,
       port,
       query_port,
       directory,
       max_memory_mb,
       backups_enabled,
       steam_game_server_login_token,
       backup_directory,
       max_backups,
       version,
       branch,
       created_at,
       updated_at,
       node_id,
       server_software,
       server_executable,
       target_pinned,
       start_args_patches,
       auto_restart_enabled,
       auto_restart_max_retries,
       auto_restart_cooldown_seconds
from game_server_node_scoped_new;

create unique index if not exists game_server_user_id_name_unique_index
    on game_server (user_id, name);

pragma writable_schema = on;

update sqlite_schema
set sql = replace(sql, 'game_server_node_scoped_new', 'game_server')
where type = 'table'
  and name in (
      'game_server_backup',
      'game_server_metrics_history',
      'installed_mod',
      'scheduled_task',
      'user_role_assignment',
      'federated_access_grant'
  );

pragma writable_schema = off;

drop table game_server_node_scoped_new;
drop table ip_node_scoped_new;

pragma legacy_alter_table = off;
pragma foreign_keys = on;
