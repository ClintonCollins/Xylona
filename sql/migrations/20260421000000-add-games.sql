-- +migrate Up
insert into game (
    id, name, default_port, default_query_port, default_max_players, require_dedicated_ip,
    uses_source_query, uses_steamcmd, steam_app_id, requires_steam_game_server_login_token,
    linux_support, linux_stop_command, linux_install_command, linux_install_command_type,
    linux_update_command, linux_update_command_type, linux_working_directory,
    windows_support, windows_stop_command, windows_install_command, windows_install_command_type,
    windows_update_command, windows_update_command_type, windows_working_directory,
    binds_to_all_ips, xylona_official, config_schemas, linux_start_args_template,
    windows_start_args_template, linux_base_command, windows_base_command, start_arg_blocklist,
    allow_start_arg_editing
) values (
    'windrose', 'Windrose', 7777, 7778, 8, 0,
    0, 1, '4129620', 0,
    0, '', '', 'direct',
    '', 'direct', '',
    1, '', 'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 4129620 validate +quit', 'direct',
    'steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 4129620 validate +quit', 'direct', '',
    0, 1,
    '[{"path":"ServerDescription.json","format":"json","category":"Server","managed_fields":{"ServerDescription_Persistent.DirectConnectionServerPort":"game_server.port"},"schema":{"type":"object","x-groups":["Identity","Access","Networking"],"properties":{"ServerDescription_Persistent.ServerName":{"type":"string","title":"Server name","description":"Name shown when identifying the server.","default":"My Windrose Server","x-group":"Identity","x-order":0},"ServerDescription_Persistent.IsPasswordProtected":{"type":"boolean","title":"Password protected","description":"Require a password before players can join.","default":false,"x-group":"Access","x-order":0},"ServerDescription_Persistent.Password":{"type":"string","title":"Password","description":"Password required when password protection is enabled.","default":"","x-group":"Access","x-order":1},"ServerDescription_Persistent.MaxPlayerCount":{"type":"integer","title":"Max players","description":"Maximum simultaneous players.","default":8,"minimum":1,"maximum":10,"x-group":"Access","x-order":2},"ServerDescription_Persistent.P2pProxyAddress":{"type":"string","title":"P2P proxy address","description":"IP address for listening sockets used by relay mode.","default":"127.0.0.1","x-group":"Networking","x-order":0},"ServerDescription_Persistent.UseDirectConnection":{"type":"boolean","title":"Use direct connection","description":"Enable direct IP and port connections instead of invite-code relay networking.","default":false,"x-group":"Networking","x-order":1},"ServerDescription_Persistent.DirectConnectionServerAddress":{"type":"string","title":"Direct connection address","description":"Public IP or hostname players use for direct connection. Leave empty for automatic detection.","default":"","x-group":"Networking","x-order":2},"ServerDescription_Persistent.DirectConnectionServerPort":{"type":"integer","title":"Direct connection port","description":"Managed from the game server port.","default":7777,"minimum":1,"maximum":65535,"x-group":"Networking","x-order":3,"x-managed":{"source":"game_server.port"}},"ServerDescription_Persistent.DirectConnectionProxyAddress":{"type":"string","title":"Direct connection proxy bind","description":"Local bind address for direct connection sockets.","default":"0.0.0.0","x-group":"Networking","x-order":4}}}}]',
    null,
    '[{"id":"01K8WR00000000000000000001","order":1,"ownership":"editable","tokens":["-log"],"label":"Log output"}]',
    '',
    'WindroseServer.exe',
    '[]',
    1
);

-- +migrate Down
delete from game where id = 'windrose';
