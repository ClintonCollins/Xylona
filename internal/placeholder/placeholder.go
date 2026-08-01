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
