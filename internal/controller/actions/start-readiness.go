package actions

import (
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/placeholder"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// readyLogPatterns are console lines a built-in game prints once it accepts
// players. Games without one become ready on their first answered query.
var readyLogPatterns = map[string]string{
	// Vanilla, Paper, Purpur, Fabric and Folia: `Done (3.214s)! For help, type "help"`.
	"minecraft": `Done \([0-9.,]+s\)!`,
	"valheim":   `Game server connected`,
}

// startReadiness says how the node tells that a started server is ready for
// players. Nil means the game gives no signal, so the node reports ONLINE at
// spawn as before.
func (inst *Instance) startReadiness(gameServer *models.GameServer) *node.ProcessReadiness {
	readiness := &node.ProcessReadiness{LogPattern: readyLogPatterns[gameServer.GameID]}
	queryType := xylona.ServerQuery_Unknown
	if gameServer.R.Game != nil {
		queryType = getQueryInfoType(gameServer.R.Game)
	}
	if queryType != xylona.ServerQuery_Unknown {
		query := node.GameServerQueryRequest{
			Kind:       nodeQueryKind(queryType),
			IP:         gameServer.IP,
			QueryPort:  gameServerQueryPort(gameServer),
			MaxPlayers: placeholder.PlayerLimit(gameServer),
		}
		queryUsable := true
		if queryType == xylona.ServerQuery_Palworld {
			username, password, errCredentials := inst.palworldQueryCredentials(gameServer)
			if errCredentials != nil {
				log.Warn().Err(errCredentials).Str("game_server_id", gameServer.ID).
					Msg("Palworld query credentials unavailable; readiness will not probe the query")
				queryUsable = false
			}
			query.Username, query.Password = username, password
		}
		if queryUsable {
			readiness.Query = &query
		}
	}
	if readiness.LogPattern == "" && readiness.Query == nil {
		return nil
	}
	return readiness
}
