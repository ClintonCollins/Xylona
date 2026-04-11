// Package rpc implements the ConnectRPC service handlers for Xylona.
package rpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/pkg/mailer"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/pkg/scheduler"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/steamcache"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// ServerSoftwareInstallBroadcaster broadcasts server software install events.
type ServerSoftwareInstallBroadcaster interface {
	BroadcastServerSoftwareInstall(serverID string, serverName string, status string, softwareID string, errMsg string)
}

// UpdateProgressBroadcaster broadcasts update progress events to WebSocket clients.
type UpdateProgressBroadcaster interface {
	BroadcastUpdateProgress(serverID string, serverName string, step xylona.UpdateStep, stepStatus xylona.StepStatus, message string)
}

// XylonaService implements the primary ConnectRPC service for the panel API.
type XylonaService struct {
	ctx                            context.Context
	db                             *db.Connection
	actionsInst                    *actions.Instance
	supervisorInst                 *supervisor.Instance
	federationMTLS                 *federation.MTLS
	secureCookie                   *securecookie.SecureCookie
	secureCookies                  bool
	syncEngine                     actions.SyncEngine
	modManager                     *modmanager.ModManager
	steamCache                     *steamcache.Client
	listCache                      *remoteServerListCache
	remoteFederationClientFactory  func(node *models.Node, serverID string) (xylonaconnect.FederationClient, error)
	installGameServerFn            func(game *models.Game, gameServer *models.GameServer, owner *models.User) (*models.GameServer, error)
	allPermissionIDs               []string
	permissionIDsForUserFn         func(user *models.User) ([]string, error)
	installTracker                 *modmanager.InstallTracker
	installBroadcast               ServerSoftwareInstallBroadcaster
	updateBroadcast                UpdateProgressBroadcaster
	versionState                   *versiontracker.VersionStateMap
	dummyTracker                   *versiontracker.DummyTracker
	testEmailSendFunc              func(ctx context.Context, cfg *mailer.SMTPConfig, to string, subject string, body string) error
	notificationChannelTestOnce    sync.Once
	notificationChannelTestLimiter *notificationChannelTestRateLimiter
	taskScheduler                  *scheduler.Scheduler
}

// NewXylonaService constructs the main RPC service implementation.
func NewXylonaService(
	ctx context.Context,
	database *db.Connection,
	actionsInst *actions.Instance,
	supervisorInst *supervisor.Instance,
	secureCookie *securecookie.SecureCookie,
	federationMTLS *federation.MTLS,
	secureCookies bool,
	steamCache *steamcache.Client,
	modMgr *modmanager.ModManager,
	versionState *versiontracker.VersionStateMap,
) (*XylonaService, error) {
	allPerms, errPerms := database.GetAllPermissions()
	if errPerms != nil {
		return nil, fmt.Errorf("rpc: load permission IDs: %w", errPerms)
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
		db:               database,
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
	}, nil
}

func (xs *XylonaService) getNotificationChannelTestLimiter() *notificationChannelTestRateLimiter {
	xs.notificationChannelTestOnce.Do(func() {
		xs.notificationChannelTestLimiter = newNotificationChannelTestRateLimiter(3, 15*time.Minute)
	})
	return xs.notificationChannelTestLimiter
}

func (xs *XylonaService) resolvedSendTestEmailFunc() func(ctx context.Context, cfg *mailer.SMTPConfig, to string) error {
	if xs.testEmailSendFunc != nil {
		return func(ctx context.Context, cfg *mailer.SMTPConfig, to string) error {
			return xs.testEmailSendFunc(ctx, cfg, to, "Xylona SMTP Test",
				"This is a test email from Xylona to verify your SMTP configuration.")
		}
	}
	return mailer.SendTestEmail
}

// SetSyncEngine configures the federation sync engine used by the RPC layer.
func (xs *XylonaService) SetSyncEngine(engine actions.SyncEngine) {
	xs.syncEngine = engine
}

// SetInstallBroadcaster sets the broadcaster used to push server software install
// status updates over WebSocket.
func (xs *XylonaService) SetInstallBroadcaster(b ServerSoftwareInstallBroadcaster) {
	xs.installBroadcast = b
}

// SetUpdateBroadcaster sets the broadcaster used to push update progress events over WebSocket.
func (xs *XylonaService) SetUpdateBroadcaster(b UpdateProgressBroadcaster) {
	xs.updateBroadcast = b
}

// SetDummyTracker sets the dummy tracker used for testing update failure simulation.
func (xs *XylonaService) SetDummyTracker(dt *versiontracker.DummyTracker) {
	xs.dummyTracker = dt
}
