package rpc

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs XylonaService) getUserFromHeader(header http.Header) (*models.User, error) {
	sessionCookies, errGetSession := gatekeeper.GetSessionFromHeader(header)
	if errGetSession != nil {
		log.Debug().Err(errGetSession).Msg("Error getting session")
		return nil, errGetSession
	}

	user, errGetUser := gatekeeper.GetUserFromSession(sessionCookies.SessionID, sessionCookies.SessionToken, xs.db, xs.secureCookie)
	if errGetUser != nil {
		log.Debug().Err(errGetUser).Msg("Error getting user")
		return nil, errGetUser
	}
	return user, nil
}

func (xs XylonaService) getGameServerFromID(gameServerID string) (*models.GameServer, error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(gameServerID)
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	return gameServer, nil
}

// getRemoteNodeForServer looks up the remote node that owns the given server ID.
// Returns the node and remote server cache entry, or an error.
func (xs XylonaService) getRemoteNodeForServer(serverID string) (*models.Node, *models.RemoteServerCache, error) {
	remoteCache, errGetRemote := xs.db.GetRemoteServerCacheByRemoteServerID(serverID)
	if errGetRemote != nil {
		return nil, nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
	}

	node, errGetNode := xs.db.GetRemoteNodeByID(remoteCache.NodeID)
	if errGetNode != nil {
		return nil, nil, connect.NewError(connect.CodeInternal, errors.New("remote node not found"))
	}

	return node, remoteCache, nil
}

// newRemoteFederationClient creates a federation client for the given node with auth header set.
func newRemoteFederationClient(node *models.Node) (xylonaconnect.FederationClient, string) {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	client := xylonaconnect.NewFederationClient(httpClient, node.BaseURL)
	secretKey := node.SecretKey.GetOr("")
	return client, secretKey
}
