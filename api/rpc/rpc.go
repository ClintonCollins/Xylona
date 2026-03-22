package rpc

import (
	"context"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/steamcache"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// SyncEngine is an interface for the federation sync engine.
// It allows the RPC layer to trigger sync operations without a direct dependency.
type SyncEngine interface {
	SyncPeer(peerNodeID string)
	RemovePeer(peerNodeID string)
	BroadcastPeerChange(changeType xylona.PeerChangeType, peer *xylona.PeerInfo, initiatedByNodeID string, initiatedByNodeName string)
}

// ServerSoftwareInstallBroadcaster broadcasts server software install events.
type ServerSoftwareInstallBroadcaster interface {
	BroadcastServerSoftwareInstall(serverID string, status string, softwareID string, errMsg string)
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
	modManager       *modmanager.ModManager
	steamCache       *steamcache.Client
	listCache        *remoteServerListCache
	allPermissionIDs []string
	installTracker   *modmanager.InstallTracker
	installBroadcast ServerSoftwareInstallBroadcaster
	versionState     *versiontracker.VersionStateMap
	dummyTracker     *versiontracker.DummyTracker
}

func NewXylonaService(
	ctx context.Context,
	db *db.Connection,
	actionsInst *actions.Instance,
	supervisorInst *supervisor.Instance,
	secureCookie *securecookie.SecureCookie,
	federationMTLS *helpers.FederationMTLS,
	secureCookies bool,
	steamCache *steamcache.Client,
	modMgr *modmanager.ModManager,
	versionState *versiontracker.VersionStateMap,
) *XylonaService {
	allPerms, errPerms := db.GetAllPermissions()
	if errPerms != nil {
		log.Fatal().Err(errPerms).Msg("Failed to load permission IDs")
	}
	permIDs := make([]string, len(allPerms))
	for i, p := range allPerms {
		permIDs[i] = p.ID
	}

	tracker := modmanager.NewInstallTracker()

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tracker.Cleanup()
			}
		}
	}()

	return &XylonaService{
		ctx:              ctx,
		db:               db,
		actionsInst:      actionsInst,
		federationMTLS:   federationMTLS,
		secureCookie:     secureCookie,
		secureCookies:    secureCookies,
		supervisorInst:   supervisorInst,
		modManager:       modMgr,
		steamCache:       steamCache,
		listCache:        newRemoteServerListCache(remoteServerListCacheTTL),
		allPermissionIDs: permIDs,
		installTracker:   tracker,
		versionState:     versionState,
	}
}

func (xs *XylonaService) SetSyncEngine(engine SyncEngine) {
	xs.syncEngine = engine
}

// SetInstallBroadcaster sets the broadcaster used to push server software install
// status updates over WebSocket.
func (xs *XylonaService) SetInstallBroadcaster(b ServerSoftwareInstallBroadcaster) {
	xs.installBroadcast = b
}

// SetDummyTracker sets the dummy tracker used for testing update failure simulation.
func (xs *XylonaService) SetDummyTracker(dt *versiontracker.DummyTracker) {
	xs.dummyTracker = dt
}
