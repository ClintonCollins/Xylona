-- +migrate Up

ALTER TABLE game ADD COLUMN linux_start_args_template TEXT DEFAULT NULL;
ALTER TABLE game ADD COLUMN windows_start_args_template TEXT DEFAULT NULL;
ALTER TABLE game ADD COLUMN linux_base_command TEXT NOT NULL DEFAULT '';
ALTER TABLE game ADD COLUMN windows_base_command TEXT NOT NULL DEFAULT '';
ALTER TABLE game ADD COLUMN start_arg_blocklist TEXT NOT NULL DEFAULT '[]';
ALTER TABLE game ADD COLUMN allow_start_arg_editing BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE game_server ADD COLUMN start_args_patches TEXT NOT NULL DEFAULT '[]';
ALTER TABLE node ADD COLUMN os TEXT NOT NULL DEFAULT '';

WITH schema_tokens AS (
    SELECT
        CASE
            WHEN EXISTS (
                SELECT 1
                FROM sqlite_master
                WHERE type = 'table'
                  AND name = 'game'
                  AND sql LIKE '%xylona_internal%'
            ) THEN 'xylona_internal'
            ELSE 'internal'
        END AS internal_type,
        CASE
            WHEN EXISTS (
                SELECT 1
                FROM sqlite_master
                WHERE type = 'table'
                  AND name = 'game'
                  AND sql LIKE '%''pwsh''%'
            ) THEN 'pwsh'
            ELSE 'powershell'
        END AS powershell_type
)
UPDATE game
SET linux_install_command_type = CASE
        WHEN lower(trim(linux_install_command_type)) IN ('direct', 'bash')
            THEN lower(trim(linux_install_command_type))
        WHEN lower(trim(linux_install_command_type)) IN ('internal', 'xylona_internal', 'papermc', 'mojang')
            THEN (SELECT internal_type FROM schema_tokens)
        WHEN lower(trim(linux_install_command_type)) IN ('none', 'steamcmd')
            THEN 'direct'
        ELSE 'direct'
    END,
    linux_update_command_type = CASE
        WHEN lower(trim(linux_update_command_type)) IN ('direct', 'bash')
            THEN lower(trim(linux_update_command_type))
        WHEN lower(trim(linux_update_command_type)) IN ('internal', 'xylona_internal', 'papermc', 'mojang')
            THEN (SELECT internal_type FROM schema_tokens)
        WHEN lower(trim(linux_update_command_type)) IN ('none', 'steamcmd')
            THEN 'direct'
        ELSE 'direct'
    END,
    windows_install_command_type = CASE
        WHEN lower(trim(windows_install_command_type)) IN ('direct', 'cmd', 'powershell')
            THEN lower(trim(windows_install_command_type))
        WHEN lower(trim(windows_install_command_type)) = 'pwsh'
            THEN (SELECT powershell_type FROM schema_tokens)
        WHEN lower(trim(windows_install_command_type)) IN ('internal', 'xylona_internal', 'papermc', 'mojang')
            THEN (SELECT internal_type FROM schema_tokens)
        WHEN lower(trim(windows_install_command_type)) IN ('none', 'steamcmd')
            THEN 'direct'
        ELSE 'direct'
    END,
    windows_update_command_type = CASE
        WHEN lower(trim(windows_update_command_type)) IN ('direct', 'cmd', 'powershell')
            THEN lower(trim(windows_update_command_type))
        WHEN lower(trim(windows_update_command_type)) = 'pwsh'
            THEN (SELECT powershell_type FROM schema_tokens)
        WHEN lower(trim(windows_update_command_type)) IN ('internal', 'xylona_internal', 'papermc', 'mojang')
            THEN (SELECT internal_type FROM schema_tokens)
        WHEN lower(trim(windows_update_command_type)) IN ('none', 'steamcmd')
            THEN 'direct'
        ELSE 'direct'
    END;

UPDATE game
SET linux_base_command = 'java',
    windows_base_command = 'java',
    linux_start_args_template = '[{"id":"01JQSA00000000000000000001","order":1,"ownership":"locked","tokens":["-Dlog4j2.formatMsgNoLookups=true"],"label":"Log4j security fix"},{"id":"01JQSA00000000000000000002","order":2,"ownership":"editable","tokens":["-Xms512M"],"label":"Min heap size"},{"id":"01JQSA00000000000000000003","order":3,"ownership":"editable","tokens":["-Xmx2G"],"label":"Max heap size"},{"id":"01JQSA00000000000000000004","order":4,"ownership":"editable","tokens":["-XX:+UnlockExperimentalVMOptions"],"label":"Unlock experimental VM options"},{"id":"01JQSA00000000000000000005","order":5,"ownership":"editable","tokens":["-XX:+UseZGC"],"label":"Use ZGC"},{"id":"01JQSA00000000000000000006","order":6,"ownership":"editable","tokens":["-XX:+ZProactive"],"label":"Enable proactive ZGC"},{"id":"01JQSA00000000000000000007","order":7,"ownership":"editable","tokens":["-XX:ZCollectionInterval=600"],"label":"ZGC collection interval"},{"id":"01JQSA00000000000000000008","order":8,"ownership":"editable","tokens":["-XX:+DisableExplicitGC"],"label":"Disable explicit GC"},{"id":"01JQSA00000000000000000009","order":9,"ownership":"editable","tokens":["-XX:+AlwaysPreTouch"],"label":"Always pre-touch memory"},{"id":"01JQSA0000000000000000000A","order":10,"ownership":"editable","tokens":["-XX:+ParallelRefProcEnabled"],"label":"Parallel reference processing"},{"id":"01JQSA0000000000000000000B","order":11,"ownership":"editable","tokens":["-XX:+PerfDisableSharedMem"],"label":"Disable perf shared memory"},{"id":"01JQSA0000000000000000000C","order":12,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"],"label":"Server executable","managed_source":"server_executable"},{"id":"01JQSA0000000000000000000D","order":13,"ownership":"editable","tokens":["nogui"],"label":"No graphical UI"}]',
    windows_start_args_template = '[{"id":"01JQSA00000000000000000001","order":1,"ownership":"locked","tokens":["-Dlog4j2.formatMsgNoLookups=true"],"label":"Log4j security fix"},{"id":"01JQSA00000000000000000002","order":2,"ownership":"editable","tokens":["-Xms512M"],"label":"Min heap size"},{"id":"01JQSA00000000000000000003","order":3,"ownership":"editable","tokens":["-Xmx2G"],"label":"Max heap size"},{"id":"01JQSA00000000000000000004","order":4,"ownership":"editable","tokens":["-XX:+UnlockExperimentalVMOptions"],"label":"Unlock experimental VM options"},{"id":"01JQSA00000000000000000005","order":5,"ownership":"editable","tokens":["-XX:+UseZGC"],"label":"Use ZGC"},{"id":"01JQSA00000000000000000006","order":6,"ownership":"editable","tokens":["-XX:+ZProactive"],"label":"Enable proactive ZGC"},{"id":"01JQSA00000000000000000007","order":7,"ownership":"editable","tokens":["-XX:ZCollectionInterval=600"],"label":"ZGC collection interval"},{"id":"01JQSA00000000000000000008","order":8,"ownership":"editable","tokens":["-XX:+DisableExplicitGC"],"label":"Disable explicit GC"},{"id":"01JQSA00000000000000000009","order":9,"ownership":"editable","tokens":["-XX:+AlwaysPreTouch"],"label":"Always pre-touch memory"},{"id":"01JQSA0000000000000000000A","order":10,"ownership":"editable","tokens":["-XX:+ParallelRefProcEnabled"],"label":"Parallel reference processing"},{"id":"01JQSA0000000000000000000B","order":11,"ownership":"editable","tokens":["-XX:+PerfDisableSharedMem"],"label":"Disable perf shared memory"},{"id":"01JQSA0000000000000000000C","order":12,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"],"label":"Server executable","managed_source":"server_executable"},{"id":"01JQSA0000000000000000000D","order":13,"ownership":"editable","tokens":["nogui"],"label":"No graphical UI"}]',
    start_arg_blocklist = '[{"pattern":"-agentlib:","reason":"Debug agents can expose remote code execution"},{"pattern":"-javaagent:","reason":"Java agents can modify server behavior unpredictably"},{"pattern":"-Dlog4j2\\\\.","reason":"Log4j settings are security-managed by the game definition"}]'
WHERE id = 'minecraft';

UPDATE game
SET linux_base_command = './7DaysToDieServer',
    windows_base_command = '7DaysToDieServer.exe',
    linux_start_args_template = '[{"id":"01JQSB00000000000000000001","order":1,"ownership":"editable","tokens":["-logfile","-"],"label":"Log to stdout"},{"id":"01JQSB00000000000000000002","order":2,"ownership":"editable","tokens":["-quit"],"label":"Quit on shutdown"},{"id":"01JQSB00000000000000000003","order":3,"ownership":"editable","tokens":["-batchmode"],"label":"Batch mode"},{"id":"01JQSB00000000000000000004","order":4,"ownership":"editable","tokens":["-nographics"],"label":"No graphics"},{"id":"01JQSB00000000000000000005","order":5,"ownership":"editable","tokens":["-configfile","settings.xml"],"label":"Config file"},{"id":"01JQSB00000000000000000006","order":6,"ownership":"editable","tokens":["-dedicated"],"label":"Dedicated mode"}]',
    windows_start_args_template = '[{"id":"01JQSB00000000000000000001","order":1,"ownership":"editable","tokens":["-logfile","-"],"label":"Log to stdout"},{"id":"01JQSB00000000000000000002","order":2,"ownership":"editable","tokens":["-quit"],"label":"Quit on shutdown"},{"id":"01JQSB00000000000000000003","order":3,"ownership":"editable","tokens":["-batchmode"],"label":"Batch mode"},{"id":"01JQSB00000000000000000004","order":4,"ownership":"editable","tokens":["-nographics"],"label":"No graphics"},{"id":"01JQSB00000000000000000005","order":5,"ownership":"editable","tokens":["-configfile","settings.xml"],"label":"Config file"},{"id":"01JQSB00000000000000000006","order":6,"ownership":"editable","tokens":["-dedicated"],"label":"Dedicated mode"}]',
    start_arg_blocklist = '[]'
WHERE id = '7_days_to_die';

INSERT INTO game (
    id,
    name,
    default_port,
    default_query_port,
    default_max_players,
    require_dedicated_ip,
    uses_source_query,
    uses_steamcmd,
    steam_app_id,
    linux_support,
    windows_support,
    linux_install_command,
    linux_update_command,
    windows_install_command,
    windows_update_command,
    linux_base_command,
    windows_base_command,
    linux_start_args_template,
    windows_start_args_template,
    start_arg_blocklist,
    xylona_official
) VALUES (
    'v_rising',
    'V Rising',
    9876,
    9877,
    40,
    true,
    false,
    true,
    '1829350',
    false,
    true,
    '',
    '',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1829350 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1829350 validate +quit',
    '',
    'VRisingServer.exe',
    NULL,
    '[{"id":"01JQSC00000000000000000001","order":1,"ownership":"system","tokens":["-persistentDataPath",".\\save-data"],"label":"Save data path"},{"id":"01JQSC00000000000000000002","order":2,"ownership":"system","tokens":["-serverPort","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSC00000000000000000003","order":3,"ownership":"system","tokens":["-queryPort","{{QUERY_PORT}}"],"label":"Query port","managed_source":"game_server.query_port"},{"id":"01JQSC00000000000000000004","order":4,"ownership":"editable","tokens":["-lan"],"label":"LAN mode"}]',
    '[]',
    true
) ON CONFLICT(id) DO NOTHING;

INSERT INTO game (
    id,
    name,
    default_port,
    default_query_port,
    default_max_players,
    require_dedicated_ip,
    uses_source_query,
    uses_steamcmd,
    steam_app_id,
    linux_support,
    windows_support,
    linux_install_command,
    linux_update_command,
    windows_install_command,
    windows_update_command,
    linux_base_command,
    windows_base_command,
    linux_start_args_template,
    windows_start_args_template,
    start_arg_blocklist,
    xylona_official
) VALUES (
    'valheim',
    'Valheim',
    2456,
    2457,
    10,
    true,
    false,
    true,
    '896660',
    true,
    true,
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 896660 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 896660 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 896660 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 896660 validate +quit',
    'valheim_server.x86_64',
    'valheim_server.exe',
    '[{"id":"01JQSD00000000000000000001","order":1,"ownership":"editable","tokens":["-name","My Valheim Server"],"label":"Server name"},{"id":"01JQSD00000000000000000002","order":2,"ownership":"editable","tokens":["-world","Dedicated"],"label":"World name"},{"id":"01JQSD00000000000000000003","order":3,"ownership":"editable","tokens":["-password","{{SERVER_ID}}"],"label":"Server password"},{"id":"01JQSD00000000000000000004","order":4,"ownership":"system","tokens":["-port","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"}]',
    '[{"id":"01JQSD00000000000000000001","order":1,"ownership":"editable","tokens":["-name","My Valheim Server"],"label":"Server name"},{"id":"01JQSD00000000000000000002","order":2,"ownership":"editable","tokens":["-world","Dedicated"],"label":"World name"},{"id":"01JQSD00000000000000000003","order":3,"ownership":"editable","tokens":["-password","{{SERVER_ID}}"],"label":"Server password"},{"id":"01JQSD00000000000000000004","order":4,"ownership":"system","tokens":["-port","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"}]',
    '[]',
    true
) ON CONFLICT(id) DO NOTHING;

INSERT INTO game (
    id,
    name,
    default_port,
    default_query_port,
    default_max_players,
    require_dedicated_ip,
    uses_source_query,
    uses_steamcmd,
    steam_app_id,
    linux_support,
    windows_support,
    linux_install_command,
    linux_update_command,
    windows_install_command,
    windows_update_command,
    linux_base_command,
    windows_base_command,
    linux_start_args_template,
    windows_start_args_template,
    start_arg_blocklist,
    xylona_official
) VALUES (
    'core_keeper',
    'Core Keeper',
    27015,
    27016,
    8,
    true,
    false,
    true,
    '1963720',
    true,
    true,
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1963720 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1963720 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1963720 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 1963720 validate +quit',
    'CoreKeeperServer.x86_64',
    'CoreKeeperServer.exe',
    '[{"id":"01JQSE00000000000000000001","order":1,"ownership":"system","tokens":["-worldport","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSE00000000000000000002","order":2,"ownership":"editable","tokens":["-maxplayers","8"],"label":"Max players"}]',
    '[{"id":"01JQSE00000000000000000001","order":1,"ownership":"system","tokens":["-worldport","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSE00000000000000000002","order":2,"ownership":"editable","tokens":["-maxplayers","8"],"label":"Max players"}]',
    '[]',
    true
) ON CONFLICT(id) DO NOTHING;

INSERT INTO game (
    id,
    name,
    default_port,
    default_query_port,
    default_max_players,
    require_dedicated_ip,
    uses_source_query,
    uses_steamcmd,
    steam_app_id,
    linux_support,
    windows_support,
    linux_install_command,
    linux_update_command,
    windows_install_command,
    windows_update_command,
    linux_base_command,
    windows_base_command,
    linux_start_args_template,
    windows_start_args_template,
    start_arg_blocklist,
    xylona_official
) VALUES (
    'palworld',
    'Palworld',
    8211,
    27015,
    32,
    true,
    false,
    true,
    '2394010',
    true,
    true,
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2394010 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2394010 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2394010 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2394010 validate +quit',
    'PalServer-Linux-Shipping',
    'PalServer.exe',
    '[{"id":"01JQSF00000000000000000001","order":1,"ownership":"system","tokens":["-port={{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSF00000000000000000002","order":2,"ownership":"system","tokens":["-publicip={{IP}}"],"label":"Bind IP","managed_source":"game_server.ip"},{"id":"01JQSF00000000000000000003","order":3,"ownership":"editable","tokens":["-useperfthreads"],"label":"Use perf threads"}]',
    '[{"id":"01JQSF00000000000000000001","order":1,"ownership":"system","tokens":["-port={{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSF00000000000000000002","order":2,"ownership":"system","tokens":["-publicip={{IP}}"],"label":"Bind IP","managed_source":"game_server.ip"},{"id":"01JQSF00000000000000000003","order":3,"ownership":"editable","tokens":["-useperfthreads"],"label":"Use perf threads"}]',
    '[]',
    true
) ON CONFLICT(id) DO NOTHING;

WITH schema_tokens AS (
    SELECT CASE
        WHEN EXISTS (
            SELECT 1
            FROM sqlite_master
            WHERE type = 'table'
              AND name = 'game'
              AND sql LIKE '%xylona_internal%'
        ) THEN 'xylona_internal'
        ELSE 'internal'
    END AS internal_type
)
INSERT INTO game (
    id,
    name,
    default_port,
    default_query_port,
    default_max_players,
    require_dedicated_ip,
    uses_source_query,
    uses_steamcmd,
    steam_app_id,
    linux_support,
    windows_support,
    linux_install_command_type,
    linux_update_command_type,
    windows_install_command_type,
    windows_update_command_type,
    linux_base_command,
    windows_base_command,
    linux_start_args_template,
    windows_start_args_template,
    start_arg_blocklist,
    xylona_official
)
SELECT
    'hytale',
    'Hytale',
    5520,
    5521,
    20,
    true,
    false,
    false,
    '',
    true,
    true,
    internal_type,
    internal_type,
    internal_type,
    internal_type,
    'java',
    'java',
    '[{"id":"01JQSG00000000000000000001","order":1,"ownership":"editable","tokens":["-Xms1G"],"label":"Min heap size"},{"id":"01JQSG00000000000000000002","order":2,"ownership":"editable","tokens":["-Xmx4G"],"label":"Max heap size"},{"id":"01JQSG00000000000000000003","order":3,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"],"label":"Server executable","managed_source":"server_executable"}]',
    '[{"id":"01JQSG00000000000000000001","order":1,"ownership":"editable","tokens":["-Xms1G"],"label":"Min heap size"},{"id":"01JQSG00000000000000000002","order":2,"ownership":"editable","tokens":["-Xmx4G"],"label":"Max heap size"},{"id":"01JQSG00000000000000000003","order":3,"ownership":"system","tokens":["-jar","{{SERVER_EXECUTABLE}}"],"label":"Server executable","managed_source":"server_executable"}]',
    '[]',
    true
FROM schema_tokens
WHERE NOT EXISTS (
    SELECT 1
    FROM game
    WHERE id = 'hytale'
);

INSERT INTO game (
    id,
    name,
    default_port,
    default_query_port,
    default_max_players,
    require_dedicated_ip,
    uses_source_query,
    uses_steamcmd,
    steam_app_id,
    linux_support,
    windows_support,
    linux_install_command,
    linux_update_command,
    windows_install_command,
    windows_update_command,
    linux_base_command,
    windows_base_command,
    linux_start_args_template,
    windows_start_args_template,
    start_arg_blocklist,
    xylona_official
) VALUES (
    'enshrouded',
    'Enshrouded',
    15636,
    15637,
    16,
    true,
    false,
    true,
    '2278520',
    false,
    true,
    '',
    '',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2278520 validate +quit',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 2278520 validate +quit',
    '',
    'enshrouded_server.exe',
    NULL,
    '[{"id":"01JQSH00000000000000000001","order":1,"ownership":"system","tokens":["-port","{{PORT}}"],"label":"Game port","managed_source":"game_server.port"},{"id":"01JQSH00000000000000000002","order":2,"ownership":"editable","tokens":["-maxplayers","16"],"label":"Max players"}]',
    '[]',
    true
) ON CONFLICT(id) DO NOTHING;

ALTER TABLE game DROP COLUMN linux_start_command;
ALTER TABLE game DROP COLUMN windows_start_command;
ALTER TABLE game_server DROP COLUMN start_command;

-- +migrate Down

CREATE TEMP TABLE structured_start_args_down_guard
(
    id INTEGER PRIMARY KEY NOT NULL
);

-- +migrate StatementBegin
CREATE TEMP TRIGGER structured_start_args_down_guard_fail
BEFORE INSERT ON structured_start_args_down_guard
BEGIN
    SELECT raise(abort, 'down migration unsupported: structured start args replaced legacy start commands and rollback would lose launch data; restore a pre-migration backup instead');
END;
-- +migrate StatementEnd

INSERT INTO structured_start_args_down_guard (id) VALUES (1);
