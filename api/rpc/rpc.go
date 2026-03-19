package rpc

import (
	"context"

	"github.com/gorilla/securecookie"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// SyncEngine is an interface for the federation sync engine.
// It allows the RPC layer to trigger sync operations without a direct dependency.
type SyncEngine interface {
	SyncPeer(peerNodeID string)
	RemovePeer(peerNodeID string)
}

type XylonaService struct {
	ctx              context.Context
	db               *db.Connection
	actionsInst      *actions.Instance
	supervisorInst   *supervisor.Instance
	federationMTLS   *helpers.FederationMTLS
	secureCookie     *securecookie.SecureCookie
	secureCookies    bool
	syncEngine       SyncEngine
	listCache        *remoteServerListCache
	allPermissionIDs []string
}

func NewXylonaService(
	ctx context.Context,
	db *db.Connection,
	actionsInst *actions.Instance,
	supervisorInst *supervisor.Instance,
	secureCookie *securecookie.SecureCookie,
	federationMTLS *helpers.FederationMTLS,
	secureCookies bool,
) *XylonaService {
	allPerms, errPerms := db.GetAllPermissions()
	if errPerms != nil {
		log.Fatal().Err(errPerms).Msg("Failed to load permission IDs")
	}
	permIDs := make([]string, len(allPerms))
	for i, p := range allPerms {
		permIDs[i] = p.ID
	}

	return &XylonaService{
		ctx:              ctx,
		db:               db,
		actionsInst:      actionsInst,
		federationMTLS:   federationMTLS,
		secureCookie:     secureCookie,
		secureCookies:    secureCookies,
		supervisorInst:   supervisorInst,
		listCache:        newRemoteServerListCache(remoteServerListCacheTTL),
		allPermissionIDs: permIDs,
	}
}

func (xs *XylonaService) SetSyncEngine(engine SyncEngine) {
	xs.syncEngine = engine
}
