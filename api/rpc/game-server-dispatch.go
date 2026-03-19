package rpc

import (
	"database/sql"
	"errors"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// dispatchLocalOrRemote routes execution based on the local server lookup result.
// If the local server exists, localHandler is called. If not found, remoteHandler is called.
func dispatchLocalOrRemote[T any](
	gameServer *models.GameServer,
	errLookup error,
	localHandler func(*models.GameServer) (*connect.Response[T], error),
	remoteHandler func() (*connect.Response[T], error),
) (*connect.Response[T], error) {
	if errLookup == nil {
		if gameServer == nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		return localHandler(gameServer)
	}

	if errors.Is(errLookup, sql.ErrNoRows) {
		return remoteHandler()
	}

	return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

func dispatchGameServerRequest[T any](
	xs *XylonaService,
	serverID string,
	localHandler func(*models.GameServer) (*connect.Response[T], error),
	remoteHandler func() (*connect.Response[T], error),
) (*connect.Response[T], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(serverID)
	return dispatchLocalOrRemote(gameServer, errGetGameServer, localHandler, remoteHandler)
}
