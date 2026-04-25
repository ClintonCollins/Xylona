// Package cfgschema provides config schema parsing and field matching helpers.
package cfgschema

var managedSourceAliases = map[string]string{
	"ip":          "game_server.ip",
	"server_port": "game_server.port",
	"query_port":  "game_server.query_port",
}

// knownManagedSources lists the recognized canonical managed field source paths.
var knownManagedSources = map[string]bool{
	"game_server.ip":         true,
	"game_server.port":       true,
	"game_server.query_port": true,
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
