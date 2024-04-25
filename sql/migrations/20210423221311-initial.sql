-- +migrate Up
create table if not exists user
(
    id            text primary key not null,
    user_name     text             not null unique,
    email         text             not null default '',
    first_name    text             not null default '',
    last_name     text             not null default '',
    password_hash text             not null default '',
    super_user    boolean          not null default false,
    last_login_at datetime         not null default current_timestamp,
    created_at    datetime         not null default current_timestamp,
    updated_at    datetime         not null default current_timestamp
);

create table if not exists user_session
(
    id         text primary key          not null,
    user_id    text references user (id) not null,
    token      text                      not null,
    created_at datetime                  not null default current_timestamp,
    updated_at datetime                  not null default current_timestamp,
    expires_at datetime                  not null
);

create table if not exists game
(
    id                                     text primary key not null,
    name                                   text             not null,
    default_port                           bigint           not null,
    default_query_port                     bigint           not null,
    default_max_players                    bigint           not null,
    require_dedicated_ip                   boolean          not null default false,
    binds_to_all_ips                       boolean          not null default false,
    uses_source_query                      boolean          not null default false,
    uses_steamcmd                          boolean          not null default false,
    steam_app_id                           text             not null default '' check ( uses_steamcmd == 0 or steam_app_id not null ),
    requires_steam_game_server_login_token boolean          not null default false,
    linux_support                          boolean          not null default false,
    linux_start_command                    text             not null default '' check ( linux_support == 0 or linux_start_command not null ),
    linux_stop_command                     text             not null default '',
    linux_install_command                  text             not null default '' check ( linux_support == 0 or linux_install_command not null ),
    linux_install_command_type             text             not null default 'direct',
    linux_update_command                   text             not null default '',
    linux_update_command_type              text             not null default 'direct',
    linux_backup_command                   text             not null default '',
    linux_backup_command_type              text             not null default 'direct',
    linux_restore_command                  text             not null default '',
    linux_restore_command_type             text             not null default 'direct',
    linux_allow_backups                    boolean          not null default false,
    linux_working_directory                text             not null default '',
    linux_configuration_file_paths         text             not null default '',
    windows_support                        boolean          not null default false,
    windows_start_command                  text             not null default '' check ( windows_support == 0 or windows_start_command not null ),
    windows_stop_command                   text             not null default '',
    windows_install_command                text             not null default '' check ( windows_support == 0 or windows_install_command not null ),
    windows_install_command_type           text             not null default 'direct',
    windows_update_command                 text             not null default '',
    windows_update_command_type            text             not null default 'direct',
    windows_backup_command                 text             not null default '',
    windows_backup_command_type            text             not null default 'direct',
    windows_restore_command                text             not null default '',
    windows_restore_command_type           text             not null default 'direct',
    windows_allow_backups                  boolean          not null default false,
    windows_working_directory              text             not null default '',
    windows_configuration_file_paths       text             not null default '',
    created_at                             datetime         not null default current_timestamp,
    updated_at                             datetime         not null default current_timestamp,

    constraint linux_install_command_type_check check ( linux_install_command_type in ('direct', 'bash', 'xylona_internal') ),
    constraint linux_update_command_type_check check ( linux_update_command_type in ('direct', 'bash', 'xylona_internal') ),
    constraint linux_backup_command_type_check check ( linux_backup_command_type in ('direct', 'bash', 'xylona_internal') ),
    constraint linux_restore_command_type_check check ( linux_restore_command_type in ('direct', 'bash', 'xylona_internal') ),
    constraint windows_install_command_type_check check ( windows_install_command_type in
                                                          ('direct', 'cmd', 'powershell', 'pwsh', 'xylona_internal') ),
    constraint windows_update_command_type_check check ( windows_update_command_type in
                                                         ('direct', 'cmd', 'powershell', 'pwsh', 'xylona_internal') ),
    constraint windows_backup_command_type_check check ( windows_backup_command_type in
                                                         ('direct', 'cmd', 'powershell', 'pwsh', 'xylona_internal') ),
    constraint windows_restore_command_type_check check ( windows_restore_command_type in
                                                          ('direct', 'cmd', 'powershell', 'pwsh', 'xylona_internal') )
);

create table if not exists game_server
(
    id                            text primary key             not null,
    user_id                       text references user (id)    not null,
    name                          text                         not null,
    game_id                       text references game (id)    not null,
    start_command                 text                         not null,
    status                        text                         not null,
    set_players                   bigint                       not null,
    max_players                   bigint                       not null,
    map                           text                         not null default '',
    ip                            text references ip (address) not null,
    port                          bigint                       not null,
    query_port                    bigint                       not null default 0,
    directory                     text                         not null,
    max_memory_mb                 bigint                       not null default 0,
    backups_enabled               boolean                      not null default false,
    steam_game_server_login_token text                         not null default '',
    backup_directory              text                         not null default '' check ( backups_enabled == 0 or backup_directory not null ),
    max_backups                   bigint                       not null default 0,
    version                       text                         not null default '',
    branch                        text                         not null default 'public',
    created_at                    datetime                     not null default current_timestamp,
    updated_at                    datetime                     not null default current_timestamp
);

create unique index if not exists game_server_user_id_name_unique_index on game_server (user_id, name);

create table if not exists ip
(
    address  text primary key not null,
    usable   boolean          not null default true,
    external boolean          not null default false
);

-- Insert Minecraft
INSERT INTO game (id, name, default_port, default_query_port, default_max_players, require_dedicated_ip,
                  binds_to_all_ips, linux_support, linux_start_command, linux_stop_command, linux_install_command,
                  linux_update_command, linux_backup_command, linux_restore_command, linux_allow_backups,
                  linux_working_directory, linux_configuration_file_paths, windows_support, windows_start_command,
                  windows_stop_command, windows_install_command, windows_update_command, windows_backup_command,
                  windows_restore_command, windows_allow_backups, windows_working_directory,
                  windows_configuration_file_paths, created_at, updated_at)
VALUES ('minecraft', 'Minecraft', 25565, 25565, 32, 0, 0, 1,
        'java -Dlog4j2.formatMsgNoLookups=true -XX:+UnlockExperimentalVMOptions -XX:+UseZGC -XX:+ZProactive -XX:ZCollectionInterval=600 -XX:+UseLargePages -XX:+DisableExplicitGC -XX:+AlwaysPreTouch -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem  -jar minecraft_server.jar',
        '/stop',
        'curl -o minecraft_server.jar https://piston-data.mojang.com/v1/objects/5b868151bd02b41319f54c8d4061b8cae84e665c/server.jar',
        '', '', '', 0, '', 'server.properties', 1,
        'java -Dlog4j2.formatMsgNoLookups=true -XX:+UnlockExperimentalVMOptions -XX:+UseZGC -XX:+ZProactive -XX:ZCollectionInterval=600 -XX:+DisableExplicitGC -XX:+AlwaysPreTouch -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem  -jar minecraft_server.jar',
        '/stop',
        'curl -o minecraft_server.jar https://piston-data.mojang.com/v1/objects/5b868151bd02b41319f54c8d4061b8cae84e665c/server.jar',
        '', '', '', 0, '', 'server.properties', current_timestamp, current_timestamp)
on conflict do nothing;

-- Insert 7 Days to Die
INSERT INTO game (id, name, default_port, default_query_port, default_max_players, require_dedicated_ip,
                  binds_to_all_ips, linux_support, linux_start_command, linux_stop_command, linux_install_command,
                  linux_update_command, linux_backup_command, linux_restore_command, linux_allow_backups,
                  linux_working_directory, linux_configuration_file_paths, windows_support, windows_start_command,
                  windows_stop_command, windows_install_command, windows_update_command, windows_backup_command,
                  windows_restore_command, windows_allow_backups, windows_working_directory,
                  windows_configuration_file_paths, created_at, updated_at)
VALUES ('7_days_to_die', '7 Days to Die', 26900, 26900, 32, 0, 0, 1,
        './7DaysToDieServer -logfile - -quit -batchmode -nographics -configfile=settings.xml -dedicated', '',
        'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 294420 validate +quit',
        '', '', '', 0, '', 'settings.xml', 1,
        './7DaysToDieServer -logfile - -quit -batchmode -nographics -configfile=settings.xml -dedicated', '',
        'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 294420 validate +quit',
        '', '', '', 0, '', 'settings.xml', current_timestamp, current_timestamp)
on conflict do nothing;


-- +migrate Down
drop table if exists game_server;
drop table if exists game;
drop table if exists user_session;
drop table if exists user;
drop table if exists ip;