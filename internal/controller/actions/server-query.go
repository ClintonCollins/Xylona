package actions

import (
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// GetServerQueries returns the latest query snapshot for all tracked servers.
func (inst *Instance) GetServerQueries() *xylona.AllServersQueryInfo {
	inst.serverQueriesMutex.RLock()
	defer inst.serverQueriesMutex.RUnlock()
	allServerQueryInfo := &xylona.AllServersQueryInfo{Servers: make(map[string]*xylona.ServerQuery)}
	for _, serverQuery := range inst.serverQueriesInfoMap {
		allServerQueryInfo.Servers[serverQuery.GetServerId()] = serverQuery
	}
	return allServerQueryInfo
}

// GetPlayerCount returns the current player count for a game server from the
// most recent query result. Returns 0 if no query data is available.
func (inst *Instance) GetPlayerCount(gameServerID string) int {
	inst.serverQueriesMutex.RLock()
	defer inst.serverQueriesMutex.RUnlock()
	sq, ok := inst.serverQueriesInfoMap[gameServerID]
	if !ok {
		return 0
	}
	if sq.GetMinecraft() != nil {
		return int(sq.GetMinecraft().GetNumberOfPlayers())
	}
	if sq.GetSource() != nil {
		return int(sq.GetSource().GetPlayers())
	}
	return 0
}
