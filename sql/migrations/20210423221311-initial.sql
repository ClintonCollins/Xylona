-- +migrate Up
create table user
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

create table game
(
    id                                     text primary key not null,
    name                                   text             not null,
    default_port                           bigint           not null,
    default_query_port                     bigint           not null,
    default_max_players                    bigint           not null,
    require_dedicated_ip                   boolean          not null default false,
    uses_source_query                      boolean          not null default false,
    uses_steamcmd                          boolean          not null default false,
    steam_app_id                           text             not null default '' check (uses_steamcmd == 0 or steam_app_id not null),
    requires_steam_game_server_login_token boolean          not null default false,
    linux_support                          boolean          not null default false,
    linux_stop_command                     text             not null default '',
    linux_install_command                  text             not null default '' check (linux_support == 0 or linux_install_command not null),
    linux_install_command_type             text             not null default 'direct',
    linux_update_command                   text             not null default '',
    linux_update_command_type              text             not null default 'direct',
    linux_working_directory                text             not null default '',
    windows_support                        boolean          not null default false,
    windows_stop_command                   text             not null default '',
    windows_install_command                text             not null default '' check (windows_support == 0 or windows_install_command not null),
    windows_install_command_type           text             not null default 'direct',
    windows_update_command                 text             not null default '',
    windows_update_command_type            text             not null default 'direct',
    windows_working_directory              text             not null default '',
    binds_to_all_ips                       boolean          not null default false,
    created_at                             datetime         not null default current_timestamp,
    updated_at                             datetime         not null default current_timestamp,
    xylona_official                        boolean          not null default false,
    config_schemas                         text             default null,
    server_software                        text             default null,
    linux_start_args_template              text             default null,
    windows_start_args_template            text             default null,
    linux_base_command                     text             not null default '',
    windows_base_command                   text             not null default '',
    start_arg_blocklist                    text             not null default '[]',
    allow_start_arg_editing                boolean          not null default true,

    constraint linux_install_command_type_check check (linux_install_command_type in ('direct', 'bash', 'internal')),
    constraint linux_update_command_type_check check (linux_update_command_type in ('direct', 'bash', 'internal')),
    constraint windows_install_command_type_check check (windows_install_command_type in ('direct', 'cmd', 'powershell', 'internal')),
    constraint windows_update_command_type_check check (windows_update_command_type in ('direct', 'cmd', 'powershell', 'internal'))
);

create table node
(
    id                      text primary key not null,
    name                    text             not null default '',
    listen_url              text             not null default '',
    cert_fingerprint        text             not null default '',
    shared_secret_encrypted text             not null default '',
    enabled                 boolean          not null default true,
    last_seen_at            datetime,
    created_at              datetime         not null default current_timestamp,
    updated_at              datetime         not null default current_timestamp
);

create table ip
(
    address             text    not null,
    usable              boolean not null default true,
    external            boolean not null default false,
    automatically_added boolean not null default false,
    node_id             text    not null references node (id),
    primary key (address, node_id)
);

create index idx_ip_node_id
    on ip (node_id);

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

create unique index game_server_user_id_name_unique_index
    on game_server (user_id, name);

create table revoked_jwt
(
    id                       text primary key not null,
    jwt_id                   text,
    username                 text,
    delete_all_tokens_before datetime,
    created_at               datetime         not null default current_timestamp,

    constraint jwt_id_or_username_check check (jwt_id is not null or username is not null)
);

create table local_settings
(
    id      integer primary key not null,
    node_id text                not null
);

create table permission
(
    id          text primary key not null,
    name        text             not null,
    description text             not null default ''
);

create table role
(
    id          text primary key not null,
    name        text             not null unique,
    description text             not null default '',
    is_system   boolean          not null default false,
    created_at  datetime         not null default current_timestamp
);

create table role_permission
(
    role_id       text not null references role (id) on delete cascade,
    permission_id text not null references permission (id) on delete cascade,
    primary key (role_id, permission_id)
);

create table user_role_assignment
(
    id             text primary key not null,
    user_id        text             not null references user (id) on delete cascade,
    role_id        text             not null references role (id) on delete cascade,
    game_server_id text references game_server (id) on delete cascade,
    granted_by     text             not null references user (id),
    created_at     datetime         not null default current_timestamp
);

create unique index user_role_server_unique
    on user_role_assignment (user_id, role_id, game_server_id);

create unique index user_role_global_unique
    on user_role_assignment (user_id, role_id) where game_server_id is null;

create table user_session
(
    id         text primary key not null,
    user_id    text             not null references user (id) on delete cascade,
    token      text             not null,
    created_at datetime         not null default current_timestamp,
    updated_at datetime         not null default current_timestamp,
    expires_at datetime         not null
);

create table node_join_token
(
    id                  text primary key not null,
    token_hash          text             not null,
    node_name           text             not null default '',
    created_at          datetime         not null default current_timestamp,
    expires_at          datetime,
    consumed_at         datetime,
    consumed_by_node_id text
);

create index node_join_token_token_hash_idx
    on node_join_token (token_hash);

create table node_metrics_history
(
    id                        text primary key not null,
    node_id                   text             not null references node (id) on delete cascade,
    cpu_percent               real             not null default 0,
    memory_percent            real             not null default 0,
    memory_used_bytes         integer          not null default 0,
    memory_total_bytes        integer          not null default 0,
    disk_percent              real             not null default 0,
    disk_used_bytes           integer          not null default 0,
    disk_total_bytes          integer          not null default 0,
    game_server_count         integer          not null default 0,
    running_game_server_count integer          not null default 0,
    user_count                integer          not null default 0,
    recorded_at               datetime         not null default current_timestamp
);

create index idx_node_metrics_node_time
    on node_metrics_history (node_id, recorded_at);

create table game_server_metrics_history
(
    id               text primary key not null,
    game_server_id   text             not null references game_server (id) on delete cascade,
    cpu_percent      real             not null default 0,
    memory_bytes     integer          not null default 0,
    memory_percent   real             not null default 0,
    disk_usage_bytes integer          not null default 0,
    io_read_rate     real             not null default 0,
    io_write_rate    real             not null default 0,
    connection_count integer          not null default 0,
    player_count     integer          not null default 0,
    recorded_at      datetime         not null default current_timestamp
);

create index idx_gs_metrics_server_time
    on game_server_metrics_history (game_server_id, recorded_at);

create table installed_mod
(
    id                   text primary key,
    game_server_id       text     not null references game_server (id) on delete cascade,
    source               text     not null,
    source_id            text     not null,
    mod_name             text     not null,
    mod_author           text     not null default '',
    installed_version    text     not null,
    installed_version_id text     not null default '',
    file_hash            text     not null default '',
    auto_update          integer  not null default 0,
    enabled              integer  not null default 1,
    pinned_version       text     default null,
    created_at           datetime not null default current_timestamp,
    updated_at           datetime not null default current_timestamp,
    unique (game_server_id, source, source_id)
);

create table installed_mod_file
(
    id               text primary key,
    installed_mod_id text    not null references installed_mod (id) on delete cascade,
    file_path        text    not null,
    file_hash        text    not null default '',
    file_size        integer not null default 0,
    is_primary       integer not null default 0
);

create table system_config
(
    key        text primary key,
    value      text     not null,
    updated_at datetime not null default current_timestamp
);

create table notification_channel
(
    id           text primary key,
    user_id      text     not null references user (id) on delete cascade,
    name         text     not null,
    channel_type text     not null,
    config       text     not null,
    enabled      integer  not null default 1,
    created_at   datetime not null default current_timestamp,
    updated_at   datetime not null default current_timestamp
);

create index idx_notification_channel_user_id
    on notification_channel (user_id);

create table alert_rule
(
    id                      text primary key,
    user_id                 text    not null references user (id) on delete cascade,
    server_id               text,
    server_node_id          text,
    node_id                 text references node (id) on delete cascade,
    event_type              text    not null,
    condition               text,
    notification_channel_id text    not null references notification_channel (id) on delete cascade,
    enabled                 integer not null default 1,
    created_at              datetime not null default current_timestamp,
    updated_at              datetime not null default current_timestamp,
    check (
        (server_id is null and server_node_id is null) or
        (server_id is not null and server_node_id is not null)
    ),
    check (
        not (server_id is not null and node_id is not null)
    )
);

create index idx_alert_rule_user_id
    on alert_rule (user_id);

create index idx_alert_rule_server
    on alert_rule (server_id, server_node_id);

create index idx_alert_rule_event_type
    on alert_rule (event_type);

create table alert_state
(
    id             text primary key,
    alert_rule_id  text    not null references alert_rule (id) on delete cascade,
    entity_type    text    not null,
    entity_id      text    not null,
    entity_node_id text    not null,
    triggered      integer not null default 0,
    triggered_at   datetime,
    resolved_at    datetime
);

create unique index idx_alert_state_unique
    on alert_state (alert_rule_id, entity_type, entity_id, entity_node_id);

create table alert_history
(
    id              text primary key,
    alert_rule_id   text references alert_rule (id) on delete set null,
    user_id         text     not null references user (id) on delete cascade,
    server_id       text,
    server_node_id  text,
    node_id         text,
    event_type      text     not null,
    event_data      text,
    channel_type    text     not null,
    delivery_status text     not null default 'pending',
    delivery_error  text,
    created_at      datetime not null default current_timestamp
);

create index idx_alert_history_user_id
    on alert_history (user_id);

create index idx_alert_history_server
    on alert_history (server_id, server_node_id);

create index idx_alert_history_created_at
    on alert_history (created_at);

create table scheduled_task
(
    id              text primary key not null,
    game_server_id  text             not null references game_server (id) on delete cascade,
    created_by      text             not null references user (id) on delete cascade,
    name            text             not null,
    task_type       text             not null check (task_type in ('restart', 'console_command', 'backup')),
    cron_expression text             not null,
    timezone        text             not null default 'UTC',
    console_command text,
    enabled         integer          not null default 1,
    last_run_at     datetime,
    next_run_at     datetime,
    created_at      datetime         not null default current_timestamp,
    updated_at      datetime         not null default current_timestamp
);

create index idx_scheduled_task_game_server_id
    on scheduled_task (game_server_id);

create index idx_scheduled_task_enabled
    on scheduled_task (enabled);

create unique index idx_scheduled_task_server_name
    on scheduled_task (game_server_id, name);

create table scheduled_task_log
(
    id                text primary key not null,
    scheduled_task_id text             not null references scheduled_task (id) on delete cascade,
    game_server_id    text             not null,
    task_type         text             not null,
    status            text             not null check (status in ('success', 'failed', 'skipped', 'timed_out')),
    message           text,
    started_at        datetime         not null,
    finished_at       datetime,
    created_at        datetime         not null default current_timestamp
);

create index idx_scheduled_task_log_task_id
    on scheduled_task_log (scheduled_task_id);

create index idx_scheduled_task_log_game_server
    on scheduled_task_log (game_server_id);

create index idx_scheduled_task_log_created_at
    on scheduled_task_log (created_at);

create index idx_scheduled_task_log_task_created
    on scheduled_task_log (scheduled_task_id, created_at desc);

create table game_server_backup
(
    id               text primary key,
    game_server_id   text     not null references game_server (id) on delete cascade,
    node_id          text     not null references node (id) on delete cascade,
    created_by       text,
    trigger_source   text     not null check (trigger_source in ('manual', 'scheduled')),
    archive_path     text     not null,
    archive_format   text     not null check (archive_format = 'zip'),
    status           text     not null check (status in ('pending', 'completed', 'failed')),
    size_bytes       integer  not null default 0,
    retention_exempt boolean  not null default false,
    error_message    text,
    created_at       datetime not null,
    completed_at     datetime,
    archive_root     text     not null default ''
);

create index idx_game_server_backup_server_created_at
    on game_server_backup (game_server_id, created_at desc);

create index idx_game_server_backup_server_retention
    on game_server_backup (game_server_id, trigger_source, retention_exempt, status, created_at desc);

create index idx_game_server_backup_node_id
    on game_server_backup (node_id);

insert into game (
    id, name, default_port, default_query_port, default_max_players, require_dedicated_ip,
    uses_source_query, uses_steamcmd, steam_app_id, requires_steam_game_server_login_token,
    linux_support, linux_stop_command, linux_install_command, linux_install_command_type,
    linux_update_command, linux_update_command_type, linux_working_directory,
    windows_support, windows_stop_command, windows_install_command, windows_install_command_type,
    windows_update_command, windows_update_command_type, windows_working_directory,
    binds_to_all_ips, xylona_official, linux_start_args_template, windows_start_args_template,
    linux_base_command, windows_base_command, start_arg_blocklist, allow_start_arg_editing
) values
    ('minecraft', 'Minecraft', 25565, 25565, 32, 1, 0, 0, '', 0, 1, '/stop', '', 'internal', '', 'direct', '', 1, '/stop', '', 'internal', '', 'direct', '', 0, 0, '[{"id":"01JQSA00000000000000000001","order":1,"ownership":"locked","tokens":["-Dlog4j2.formatMsgNoLookups=true"],"label":"Log4j security fix"},{"id":"01JQSA00000000000000000002","order":2,"ownership":"editable","tokens":["-Xms512M"],"label":"Min heap size"},{"id":"01JQSA00000000000000000003","order":3,"ownership":"editable","tokens":["-Xmx2G"],"label":"Max heap size"},{"id":"01JQSA00000000000000000004","order":4,"ownership":"editable","tokens":["-XX:+UnlockExperimentalVMOptions"],"label":"Unlock experimental VM options"},{"id":"01JQSA00000000000000000005","order":5,"ownership":"editable","tokens":["-XX:+UseZGC"],"label":"Use ZGC"},{"id":"01JQSA00000000000000000006","order":6,"ownership":"editable","tokens":["-XX:+ZProactive"],"label":"Enable proactive ZGC"},{"id":"01JQSA00000000000000000007","order":7,"ownership":"editable","tokens":["-XX:ZCollectionInterval=600"],"label":"ZGC collection interval"},{"id":"01JQSA00000000000000000008","order":8,"ownership":"editable","tokens":["-XX:+DisableExplicitGC"],"label":"Disable explicit GC"},{"id":"01JQSA00000000000000000009","order":9,"ownership":"editable","tokens":["-XX:+AlwaysPreTouch"],"label":"Always pre-touch memory"},{"id":"01JQSA0000000000000000000A","order":10,"ownership":"editable","tokens":["-XX:+ParallelRefProcEnabled"],"label":"Parallel reference processing"},{"id":"01JQSA0000000000000000000B","order":11,"ownership":"editable","tokens":["-XX:+PerfDisableSharedMem"],"label":"Disable perf shared memory"},{"id":"01JQSA0000000000000000000C","order":12,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"],"label":"Server executable","managed_source":"server_executable"},{"id":"01JQSA0000000000000000000D","order":13,"ownership":"editable","tokens":["nogui"],"label":"No graphical UI"}]', '[{"id":"01JQSA00000000000000000001","order":1,"ownership":"locked","tokens":["-Dlog4j2.formatMsgNoLookups=true"],"label":"Log4j security fix"},{"id":"01JQSA00000000000000000002","order":2,"ownership":"editable","tokens":["-Xms512M"],"label":"Min heap size"},{"id":"01JQSA00000000000000000003","order":3,"ownership":"editable","tokens":["-Xmx2G"],"label":"Max heap size"},{"id":"01JQSA00000000000000000004","order":4,"ownership":"editable","tokens":["-XX:+UnlockExperimentalVMOptions"],"label":"Unlock experimental VM options"},{"id":"01JQSA00000000000000000005","order":5,"ownership":"editable","tokens":["-XX:+UseZGC"],"label":"Use ZGC"},{"id":"01JQSA00000000000000000006","order":6,"ownership":"editable","tokens":["-XX:+ZProactive"],"label":"Enable proactive ZGC"},{"id":"01JQSA00000000000000000007","order":7,"ownership":"editable","tokens":["-XX:ZCollectionInterval=600"],"label":"ZGC collection interval"},{"id":"01JQSA00000000000000000008","order":8,"ownership":"editable","tokens":["-XX:+DisableExplicitGC"],"label":"Disable explicit GC"},{"id":"01JQSA00000000000000000009","order":9,"ownership":"editable","tokens":["-XX:+AlwaysPreTouch"],"label":"Always pre-touch memory"},{"id":"01JQSA0000000000000000000A","order":10,"ownership":"editable","tokens":["-XX:+ParallelRefProcEnabled"],"label":"Parallel reference processing"},{"id":"01JQSA0000000000000000000B","order":11,"ownership":"editable","tokens":["-XX:+PerfDisableSharedMem"],"label":"Disable perf shared memory"},{"id":"01JQSA0000000000000000000C","order":12,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"],"label":"Server executable","managed_source":"server_executable"},{"id":"01JQSA0000000000000000000D","order":13,"ownership":"editable","tokens":["nogui"],"label":"No graphical UI"}]', 'java', 'java', '[{"pattern":"-agentlib:","reason":"Debug agents can expose remote code execution"},{"pattern":"-javaagent:","reason":"Java agents can modify server behavior unpredictably"},{"pattern":"-Dlog4j2\\\\.","reason":"Log4j settings are security-managed by the game definition"}]', 1),
    ('7_days_to_die', '7 Days to Die', 26900, 26900, 32, 1, 0, 0, '', 0, 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 294420 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 294420 validate +quit', 'direct', '', 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 294420 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 294420 validate +quit', 'direct', '', 0, 0, '[{"id":"01JQSB00000000000000000001","order":1,"ownership":"editable","tokens":["-logfile","-"],"label":"Log to stdout"},{"id":"01JQSB00000000000000000002","order":2,"ownership":"editable","tokens":["-quit"],"label":"Quit on shutdown"},{"id":"01JQSB00000000000000000003","order":3,"ownership":"editable","tokens":["-batchmode"],"label":"Batch mode"},{"id":"01JQSB00000000000000000004","order":4,"ownership":"editable","tokens":["-nographics"],"label":"No graphics"},{"id":"01JQSB00000000000000000005","order":5,"ownership":"editable","tokens":["-configfile","settings.xml"],"label":"Config file"},{"id":"01JQSB00000000000000000006","order":6,"ownership":"editable","tokens":["-dedicated"],"label":"Dedicated mode"}]', '[{"id":"01JQSB00000000000000000001","order":1,"ownership":"editable","tokens":["-logfile","-"],"label":"Log to stdout"},{"id":"01JQSB00000000000000000002","order":2,"ownership":"editable","tokens":["-quit"],"label":"Quit on shutdown"},{"id":"01JQSB00000000000000000003","order":3,"ownership":"editable","tokens":["-batchmode"],"label":"Batch mode"},{"id":"01JQSB00000000000000000004","order":4,"ownership":"editable","tokens":["-nographics"],"label":"No graphics"},{"id":"01JQSB00000000000000000005","order":5,"ownership":"editable","tokens":["-configfile","settings.xml"],"label":"Config file"},{"id":"01JQSB00000000000000000006","order":6,"ownership":"editable","tokens":["-dedicated"],"label":"Dedicated mode"}]', './7DaysToDieServer', '7DaysToDieServer.exe', '[]', 1),
    ('v_rising', 'V Rising', 9876, 9877, 40, 1, 0, 1, '1829350', 0, 0, '', '', 'direct', '', 'direct', '', 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1829350 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1829350 validate +quit', 'direct', '', 0, 1, null, '[{"id":"01JQSC00000000000000000001","order":1,"ownership":"system","tokens":["-persistentDataPath",".\\save-data"],"label":"Save data path"},{"id":"01JQSC00000000000000000002","order":2,"ownership":"system","tokens":["-serverPort","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSC00000000000000000003","order":3,"ownership":"system","tokens":["-queryPort","{{QUERY_PORT}}"],"label":"Query port","managed_source":"game_server.query_port"},{"id":"01JQSC00000000000000000004","order":4,"ownership":"editable","tokens":["-lan"],"label":"LAN mode"}]', '', 'VRisingServer.exe', '[]', 1),
    ('valheim', 'Valheim', 2456, 2457, 10, 1, 0, 1, '896660', 0, 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 896660 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 896660 validate +quit', 'direct', '', 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 896660 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 896660 validate +quit', 'direct', '', 0, 1, '[{"id":"01JQSD00000000000000000001","order":1,"ownership":"editable","tokens":["-name","My Valheim Server"],"label":"Server name"},{"id":"01JQSD00000000000000000002","order":2,"ownership":"editable","tokens":["-world","Dedicated"],"label":"World name"},{"id":"01JQSD00000000000000000003","order":3,"ownership":"editable","tokens":["-password","{{SERVER_ID}}"],"label":"Server password"},{"id":"01JQSD00000000000000000004","order":4,"ownership":"system","tokens":["-port","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"}]', '[{"id":"01JQSD00000000000000000001","order":1,"ownership":"editable","tokens":["-name","My Valheim Server"],"label":"Server name"},{"id":"01JQSD00000000000000000002","order":2,"ownership":"editable","tokens":["-world","Dedicated"],"label":"World name"},{"id":"01JQSD00000000000000000003","order":3,"ownership":"editable","tokens":["-password","{{SERVER_ID}}"],"label":"Server password"},{"id":"01JQSD00000000000000000004","order":4,"ownership":"system","tokens":["-port","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"}]', 'valheim_server.x86_64', 'valheim_server.exe', '[]', 1),
    ('core_keeper', 'Core Keeper', 27015, 27016, 8, 1, 0, 1, '1963720', 0, 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1963720 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1963720 validate +quit', 'direct', '', 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1963720 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1963720 validate +quit', 'direct', '', 0, 1, '[{"id":"01JQSE00000000000000000001","order":1,"ownership":"system","tokens":["-worldport","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSE00000000000000000002","order":2,"ownership":"editable","tokens":["-maxplayers","8"],"label":"Max players"}]', '[{"id":"01JQSE00000000000000000001","order":1,"ownership":"system","tokens":["-worldport","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSE00000000000000000002","order":2,"ownership":"editable","tokens":["-maxplayers","8"],"label":"Max players"}]', 'CoreKeeperServer.x86_64', 'CoreKeeperServer.exe', '[]', 1),
    ('palworld', 'Palworld', 8211, 27015, 32, 1, 0, 1, '2394010', 0, 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2394010 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2394010 validate +quit', 'direct', '', 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2394010 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2394010 validate +quit', 'direct', '', 0, 1, '[{"id":"01JQSF00000000000000000001","order":1,"ownership":"system","tokens":["-port={{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSF00000000000000000002","order":2,"ownership":"system","tokens":["-publicip={{IP}}"],"label":"Bind IP","managed_source":"game_server.ip"},{"id":"01JQSF00000000000000000003","order":3,"ownership":"editable","tokens":["-useperfthreads"],"label":"Use perf threads"}]', '[{"id":"01JQSF00000000000000000001","order":1,"ownership":"system","tokens":["-port={{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSF00000000000000000002","order":2,"ownership":"system","tokens":["-publicip={{IP}}"],"label":"Bind IP","managed_source":"game_server.ip"},{"id":"01JQSF00000000000000000003","order":3,"ownership":"editable","tokens":["-useperfthreads"],"label":"Use perf threads"}]', 'PalServer-Linux-Shipping', 'PalServer.exe', '[]', 1),
    ('hytale', 'Hytale', 5520, 5521, 20, 1, 0, 0, '', 0, 1, '', '', 'internal', '', 'internal', '', 1, '', '', 'internal', '', 'internal', '', 0, 1, '[{"id":"01JQSG00000000000000000001","order":1,"ownership":"editable","tokens":["-Xms1G"],"label":"Min heap size"},{"id":"01JQSG00000000000000000002","order":2,"ownership":"editable","tokens":["-Xmx4G"],"label":"Max heap size"},{"id":"01JQSG00000000000000000003","order":3,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"],"label":"Server executable","managed_source":"server_executable"}]', '[{"id":"01JQSG00000000000000000001","order":1,"ownership":"editable","tokens":["-Xms1G"],"label":"Min heap size"},{"id":"01JQSG00000000000000000002","order":2,"ownership":"editable","tokens":["-Xmx4G"],"label":"Max heap size"},{"id":"01JQSG00000000000000000003","order":3,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"],"label":"Server executable","managed_source":"server_executable"}]', 'java', 'java', '[]', 1),
    ('enshrouded', 'Enshrouded', 15636, 15637, 16, 1, 0, 1, '2278520', 0, 0, '', '', 'direct', '', 'direct', '', 1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2278520 validate +quit', 'direct', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2278520 validate +quit', 'direct', '', 0, 1, null, '[{"id":"01JQSH00000000000000000001","order":1,"ownership":"system","tokens":["-port","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSH00000000000000000002","order":2,"ownership":"editable","tokens":["-maxplayers","16"],"label":"Max players"}]', '', 'enshrouded_server.exe', '[]', 1);

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
    ('game_server.delete', 'Delete Game Server', 'Delete a game server'),
    ('game_server.metrics', 'View Metrics History', 'View historical game server metrics and charts'),
    ('game_server.config', 'game_server.config', 'Edit game server configuration files'),
    ('game_server.mods', 'game_server.mods', 'Manage mods and plugins on a game server'),
    ('alerts.manage', 'alerts.manage', 'Manage alert rules and notification channels'),
    ('alerts.view_history', 'alerts.view_history', 'View alert notification history'),
    ('game_server.scheduled_tasks', 'game_server.scheduled_tasks', 'Manage scheduled tasks for a game server');

insert into role (id, name, description, is_system) values
    ('viewer', 'Viewer', 'Read-only access to game server status and details', true),
    ('operator', 'Operator', 'Can start, stop, restart, and use console', true),
    ('admin', 'Admin', 'Full control over the game server', true);

insert into role_permission (role_id, permission_id) values
    ('viewer', 'game_server.view'),
    ('operator', 'game_server.view'),
    ('operator', 'game_server.start'),
    ('operator', 'game_server.stop'),
    ('operator', 'game_server.restart'),
    ('operator', 'game_server.console'),
    ('operator', 'game_server.scheduled_tasks'),
    ('admin', 'game_server.view'),
    ('admin', 'game_server.start'),
    ('admin', 'game_server.stop'),
    ('admin', 'game_server.restart'),
    ('admin', 'game_server.console'),
    ('admin', 'game_server.files.view'),
    ('admin', 'game_server.files.edit'),
    ('admin', 'game_server.settings'),
    ('admin', 'game_server.backup'),
    ('admin', 'game_server.delete'),
    ('admin', 'game_server.metrics'),
    ('admin', 'game_server.config'),
    ('admin', 'game_server.mods'),
    ('admin', 'alerts.manage'),
    ('admin', 'alerts.view_history'),
    ('admin', 'game_server.scheduled_tasks');

-- +migrate Down
pragma foreign_keys = off;

drop index if exists idx_game_server_backup_node_id;
drop index if exists idx_game_server_backup_server_retention;
drop index if exists idx_game_server_backup_server_created_at;
drop index if exists idx_scheduled_task_log_task_created;
drop index if exists idx_scheduled_task_log_created_at;
drop index if exists idx_scheduled_task_log_game_server;
drop index if exists idx_scheduled_task_log_task_id;
drop index if exists idx_scheduled_task_server_name;
drop index if exists idx_scheduled_task_enabled;
drop index if exists idx_scheduled_task_game_server_id;
drop index if exists idx_alert_history_created_at;
drop index if exists idx_alert_history_server;
drop index if exists idx_alert_history_user_id;
drop index if exists idx_alert_state_unique;
drop index if exists idx_alert_rule_event_type;
drop index if exists idx_alert_rule_server;
drop index if exists idx_alert_rule_user_id;
drop index if exists idx_notification_channel_user_id;
drop index if exists idx_gs_metrics_server_time;
drop index if exists idx_node_metrics_node_time;
drop index if exists node_join_token_token_hash_idx;
drop index if exists user_role_global_unique;
drop index if exists user_role_server_unique;
drop index if exists game_server_user_id_name_unique_index;
drop index if exists idx_ip_node_id;

drop table if exists game_server_backup;
drop table if exists scheduled_task_log;
drop table if exists scheduled_task;
drop table if exists alert_history;
drop table if exists alert_state;
drop table if exists alert_rule;
drop table if exists notification_channel;
drop table if exists system_config;
drop table if exists installed_mod_file;
drop table if exists installed_mod;
drop table if exists game_server_metrics_history;
drop table if exists node_metrics_history;
drop table if exists node_join_token;
drop table if exists user_session;
drop table if exists user_role_assignment;
drop table if exists role_permission;
drop table if exists role;
drop table if exists permission;
drop table if exists local_settings;
drop table if exists revoked_jwt;
drop table if exists game_server;
drop table if exists ip;
drop table if exists node;
drop table if exists game;
drop table if exists user;

pragma foreign_keys = on;
