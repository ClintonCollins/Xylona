package rpc

import (
	"context"

	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// SyncEngine is an interface for the federation sync engine.
// It allows the RPC layer to trigger sync operations without a direct dependency.
type SyncEngine interface {
	SyncPeer(peerNodeID string)
	RemovePeer(peerNodeID string)
}

type XylonaService struct {
	ctx            context.Context
	db             *db.Connection
	actionsInst    *actions.Instance
	supervisorInst *supervisor.Instance
	secureCookie   *securecookie.SecureCookie
	syncEngine     SyncEngine
	listCache      *remoteServerListCache
}

func NewXylonaService(ctx context.Context, db *db.Connection, actionsInst *actions.Instance, supervisorInst *supervisor.Instance, secureCookie *securecookie.SecureCookie) *XylonaService {
	return &XylonaService{
		ctx:            ctx,
		db:             db,
		actionsInst:    actionsInst,
		secureCookie:   secureCookie,
		supervisorInst: supervisorInst,
		listCache:      newRemoteServerListCache(remoteServerListCacheTTL),
	}
}

func (xs *XylonaService) SetSyncEngine(engine SyncEngine) {
	xs.syncEngine = engine
}
