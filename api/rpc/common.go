package rpc

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs *XylonaService) getUserFromHeader(header http.Header) (*models.User, error) {
	sessionCookies, errGetSession := gatekeeper.GetSessionFromHeader(header)
	if errGetSession != nil {
		log.Debug().Err(errGetSession).Msg("Error getting session")
		return nil, fmt.Errorf("rpc: get session from header: %w", errGetSession)
	}

	user, errGetUser := gatekeeper.GetUserFromSession(sessionCookies.SessionID, sessionCookies.SessionToken, xs.db, xs.secureCookie)
	if errGetUser != nil {
		log.Debug().Err(errGetUser).Msg("Error getting user")
		return nil, fmt.Errorf("rpc: get user from session: %w", errGetUser)
	}
	return user, nil
}

func (xs *XylonaService) getGameServerFromID(gameServerID string) (*models.GameServer, error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(gameServerID)
	if errGetGameServer != nil {
		return nil, dbLookup(errGetGameServer)
	}
	return gameServer, nil
}

func (xs *XylonaService) getLocalGameServerStatus(gameServer *models.GameServer) xylona.Status {
	if xs.supervisorInst != nil {
		gameServerCmd, errGetCommand := xs.supervisorInst.GetCommandByID(gameServer.ID)
		if errGetCommand != nil {
			return xylona.Status_OFFLINE
		}
		return gameServerCmd.Status()
	}

	status, ok := xylona.Status_value[gameServer.Status]
	if !ok {
		return xylona.Status_UNKNOWN
	}

	return xylona.Status(status)
}

// getRemoteNodeForServer looks up the remote node that owns the given server ID.
// Returns the node and remote server cache entry, or an error.
func (xs *XylonaService) getRemoteNodeForServer(serverID string) (*models.Node, *models.RemoteServerCache, error) {
	remoteCache, errGetRemote := xs.db.GetRemoteServerCacheByRemoteServerID(serverID)
	if errGetRemote != nil {
		if errors.Is(errGetRemote, db.ErrAmbiguousRemoteServerCache) {
			log.Warn().
				Err(errGetRemote).
				Str("server_id", serverID).
				Msg("Ambiguous remote server cache lookup by remote server ID")
			return nil, nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("ambiguous remote server mapping"))
		}
		if errors.Is(errGetRemote, sql.ErrNoRows) {
			return nil, nil, notFoundErr()
		}
		log.Error().Err(errGetRemote).Str("server_id", serverID).Msg("Failed to load remote server cache entry")
		return nil, nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	node, errGetNode := xs.db.GetRemoteNodeByID(remoteCache.NodeID)
	if errGetNode != nil {
		if errors.Is(errGetNode, sql.ErrNoRows) {
			errDeleteCache := xs.db.DeleteRemoteServerCacheByCompositeKey(remoteCache.SourceNodeID, remoteCache.RemoteServerID)
			if errDeleteCache != nil {
				log.Warn().
					Err(errDeleteCache).
					Str("server_id", serverID).
					Str("node_id", remoteCache.NodeID).
					Msg("Failed to delete orphaned remote server cache row")
			}
			return nil, nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, nil, internalErrf("remote node not found")
	}

	return node, remoteCache, nil
}

// newRemoteFederationClient creates a federation client for the given node using mTLS and pinned fingerprint trust.
func (xs *XylonaService) newRemoteFederationClient(node *models.Node) (xylonaconnect.FederationClient, error) {
	if xs.federationMTLS == nil {
		return nil, errors.New("federation mTLS is not configured")
	}

	httpClient, federationBaseURL, errClient := xs.federationMTLS.NewTrustedPeerHTTPClientWithPort(
		15*time.Second,
		node.ID,
		node.BaseURL,
		xs.remoteFederationPort(node),
		xs.db,
	)
	if errClient != nil {
		return nil, fmt.Errorf("rpc: create remote federation client: %w", errClient)
	}

	return xylonaconnect.NewFederationClient(httpClient, federationBaseURL), nil
}

func (xs *XylonaService) remoteFederationPort(node *models.Node) int {
	if node != nil && node.Port > 0 {
		return int(node.Port)
	}
	if xs.federationMTLS != nil {
		return xs.federationMTLS.FederationPort()
	}
	return 0
}
