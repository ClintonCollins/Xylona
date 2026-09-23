package actions

import (
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/placeholder"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// startReadiness says how the node tells that a started server is ready for
// players: the game definition's ready log pattern, or for a game without one
// its first answered query. A game with a pattern waits for it even when it
// has a query, since some (Valheim's Steam query) answer before players can
// join. Nil means the game gives no signal, so the node reports ONLINE at
// spawn as before.
func (inst *Instance) startReadiness(gameServer *models.GameServer) *node.ProcessReadiness {
	game := gameServer.R.Game
	if game == nil {
		return nil
	}
	if game.ReadyLogPattern != "" {
		return &node.ProcessReadiness{LogPattern: game.ReadyLogPattern}
	}
	queryType := getQueryInfoType(game)
	if queryType == xylona.ServerQuery_Unknown {
		return nil
	}
	query := node.GameServerQueryRequest{
		Kind:       nodeQueryKind(queryType),
		IP:         gameServer.IP,
		QueryPort:  gameServerQueryPort(gameServer),
		MaxPlayers: placeholder.PlayerLimit(gameServer),
	}
	if queryType == xylona.ServerQuery_Palworld {
		username, password, errCredentials := inst.palworldQueryCredentials(gameServer)
		if errCredentials != nil {
			log.Warn().Err(errCredentials).Str("game_server_id", gameServer.ID).
				Msg("Palworld query credentials unavailable; readiness will not probe the query")
			return nil
		}
		query.Username, query.Password = username, password
	}
	return &node.ProcessReadiness{Query: &query}
}
