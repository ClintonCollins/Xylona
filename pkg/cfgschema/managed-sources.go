// Package cfgschema provides config schema parsing and field matching helpers.
package cfgschema

var managedSourceAliases = map[string]string{
	"ip":                 "game_server.ip",
	"server_port":        "game_server.port",
	"server_port_plus_1": "game_server.port_plus_1",
	"server_port_plus_2": "game_server.port_plus_2",
	"query_port":         "game_server.query_port",
	"query_port_plus_1":  "game_server.query_port_plus_1",
	"max_players":        "game_server.max_players",
	"server_name":        "game_server.server_name",
	"directory":          "game_server.directory",
}

// knownManagedSources lists the recognized canonical managed field source paths.
var knownManagedSources = map[string]bool{
	"game_server.ip":                true,
	"game_server.port":              true,
	"game_server.port_plus_1":       true,
	"game_server.port_plus_2":       true,
	"game_server.query_port":        true,
	"game_server.query_port_plus_1": true,
	"game_server.max_players":       true,
	"game_server.server_name":       true,
	"game_server.directory":         true,
	"xylona.local_console_enabled":  true,
	"xylona.local_console_password": true,
	"steam_gslt":                    true,
}

func normalizeManagedSource(source string) string {
	if canonicalSource, ok := managedSourceAliases[source]; ok {
		return canonicalSource
	}

	return source
}

func isKnownManagedSource(source string) bool {
	return knownManagedSources[normalizeManagedSource(source)]
}
