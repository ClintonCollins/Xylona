// Package placeholder resolves named template placeholders used across Xylona.
package placeholder

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// Placeholder represents a named variable that can be resolved in templates.
type Placeholder struct {
	Key         string
	Label       string
	Description string
}

// Registry of all built-in placeholders.
var Registry = []Placeholder{
	{Key: "IP", Label: "IP Address", Description: "The game server's bound IP address"},
	{Key: "PORT", Label: "Server Port", Description: "The game server's port"},
	{Key: "PORT_PLUS_1", Label: "Server Port + 1", Description: "The port immediately after the game server's port"},
	{Key: "PORT_PLUS_2", Label: "Server Port + 2", Description: "The second port after the game server's port"},
	{Key: "QUERY_PORT", Label: "Query Port", Description: "The query port"},
	{Key: "QUERY_PORT_PLUS_1", Label: "Query Port + 1", Description: "The port immediately after the query port"},
	{Key: "MAX_MEMORY_MB", Label: "Game Server Memory (MB)", Description: "The game server's configured memory limit in megabytes"},
	{Key: "MAX_PLAYERS", Label: "Max Players", Description: "Maximum player count"},
	{Key: "SERVER_NAME", Label: "Server Name", Description: "The game server's display name"},
	{Key: "RCON_PORT", Label: "RCON Port", Description: "The RCON port"},
	{Key: "RCON_PASSWORD", Label: "RCON Password", Description: "The RCON password"},
	{Key: "INSTALL_DIR", Label: "Install Directory", Description: "The game server's installation directory"},
	{Key: "STEAM_APPID", Label: "Steam App ID", Description: "The Steam application ID"},
	{Key: "SERVER_EXECUTABLE", Label: "Server Executable", Description: "The server software executable filename (e.g., paper-1.21.4-100.jar)"},
}

// ManagedSourceMapping maps managed source keys (used in config schemas frontend)
// to placeholder keys.
var ManagedSourceMapping = map[string]string{
	"ip":                 "IP",
	"server_port":        "PORT",
	"server_port_plus_1": "PORT_PLUS_1",
	"server_port_plus_2": "PORT_PLUS_2",
	"query_port":         "QUERY_PORT",
	"query_port_plus_1":  "QUERY_PORT_PLUS_1",
	"max_memory_mb":      "MAX_MEMORY_MB",
	"max_players":        "MAX_PLAYERS",
	"server_name":        "SERVER_NAME",
	"rcon_port":          "RCON_PORT",
	"rcon_password":      "RCON_PASSWORD",
}

// BackendManagedSourceMapping maps the backend managed source keys
// (game_server.* format used in cfgschema) to placeholder keys.
var BackendManagedSourceMapping = map[string]string{
	"game_server.ip":                "IP",
	"game_server.port":              "PORT",
	"game_server.port_plus_1":       "PORT_PLUS_1",
	"game_server.port_plus_2":       "PORT_PLUS_2",
	"game_server.query_port":        "QUERY_PORT",
	"game_server.query_port_plus_1": "QUERY_PORT_PLUS_1",
	"game_server.max_memory_mb":     "MAX_MEMORY_MB",
	"game_server.max_players":       "MAX_PLAYERS",
	"game_server.server_name":       "SERVER_NAME",
	"game_server.directory":         "INSTALL_DIR",
}

// legacyMapping maps old %GAMESERVER_*% format to new placeholder keys.
var legacyMapping = map[string]string{
	"%GAMESERVER_DIRECTORY%":        "INSTALL_DIR",
	"%GAMESERVER_ID%":               "SERVER_ID",
	"%GAMESERVER_BACKUP_DIRECTORY%": "BACKUP_DIR",
	"%GAMESERVER_NAME%":             "SERVER_NAME",
	"%GAMESERVER_IP%":               "IP",
	"%GAMESERVER_PORT%":             "PORT",
	"%GAMESERVER_QUERY_PORT%":       "QUERY_PORT",
	"%GAMESERVER_MAX_MEMORY_MB%":    "MAX_MEMORY_MB",
	"%GAMESERVER_MAX_PLAYERS%":      "MAX_PLAYERS",
	"%GAMESERVER_SET_PLAYERS%":      "SET_PLAYERS",
}

var placeholderRegex = regexp.MustCompile(`\{\{([A-Z_]+)\}\}`)

// LegacyToNewKey maps a legacy %GAMESERVER_*% placeholder to its new key.
func LegacyToNewKey(legacy string) (string, bool) {
	key, ok := legacyMapping[legacy]
	return key, ok
}

// Resolve replaces all {{PLACEHOLDER}} and legacy %GAMESERVER_*% occurrences
// in template with values from vars. Unresolved placeholders resolve to empty
// string and log a warning.
func Resolve(template string, vars map[string]string) string {
	if template == "" {
		return ""
	}
	return ResolveToken(template, vars)
}

// ResolveToken replaces all {{PLACEHOLDER}} and legacy %GAMESERVER_*%
// occurrences in a single token string. Unresolved placeholders resolve to
// empty string and log a warning.
func ResolveToken(token string, vars map[string]string) string {
	if token == "" {
		return ""
	}
	if vars == nil {
		vars = map[string]string{}
	}

	// Resolve legacy %GAMESERVER_*% format first.
	for legacy, newKey := range legacyMapping {
		if strings.Contains(token, legacy) {
			val, ok := vars[newKey]
			if !ok {
				log.Warn().Str("placeholder", legacy).Msg("Unresolved legacy placeholder")
			}
			token = strings.ReplaceAll(token, legacy, val)
		}
	}

	// Resolve {{PLACEHOLDER}} format.
	token = placeholderRegex.ReplaceAllStringFunc(token, func(match string) string {
		key := match[2 : len(match)-2]
		val, ok := vars[key]
		if !ok {
			log.Warn().Str("placeholder", key).Msg("Unresolved placeholder")
			return ""
		}
		return val
	})

	return token
}

// ResolveTokens resolves placeholders independently within each token and
// preserves token boundaries.
func ResolveTokens(tokens []string, vars map[string]string) []string {
	if len(tokens) == 0 {
		return nil
	}

	resolved := make([]string, 0, len(tokens))
	for _, token := range tokens {
		resolved = append(resolved, ResolveToken(token, vars))
	}

	return resolved
}

// BuildVarsFromGameServer creates a variable map from a game server instance.
// Only includes fields that exist on the models.GameServer struct.
// RCON_PORT, RCON_PASSWORD, and STEAM_APPID are registered in the placeholder
// Registry for future use but are NOT populated here because the GameServer
// model does not have those fields yet.
func BuildVarsFromGameServer(gs *models.GameServer) map[string]string {
	return map[string]string{
		"IP":                gs.IP,
		"PORT":              strconv.FormatInt(gs.Port, 10),
		"PORT_PLUS_1":       strconv.FormatInt(gs.Port+1, 10),
		"PORT_PLUS_2":       strconv.FormatInt(gs.Port+2, 10),
		"QUERY_PORT":        strconv.FormatInt(gs.QueryPort, 10),
		"QUERY_PORT_PLUS_1": strconv.FormatInt(gs.QueryPort+1, 10),
		"MAX_PLAYERS":       fmt.Sprintf("%d", gs.MaxPlayers),
		"SERVER_NAME":       gs.Name,
		"INSTALL_DIR":       gs.Directory,
		"SERVER_ID":         gs.ID,
		"BACKUP_DIR":        gs.BackupDirectory,
		"MAX_MEMORY_MB":     fmt.Sprintf("%d", gs.MaxMemoryMB),
		"SET_PLAYERS":       fmt.Sprintf("%d", gs.SetPlayers),
		"SERVER_EXECUTABLE": gs.ServerExecutable.GetOr(""),
	}
}
