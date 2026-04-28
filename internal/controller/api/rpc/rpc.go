// Package rpc implements the ConnectRPC service handlers for Xylona.
package rpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/mailer"
	"github.com/ClintonCollins/Xylona/internal/modmanager"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/scheduler"
	"github.com/ClintonCollins/Xylona/internal/steamcache"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// ServerSoftwareInstallBroadcaster broadcasts server software install events.
type ServerSoftwareInstallBroadcaster interface {
	BroadcastServerSoftwareInstall(serverID string, serverName string, status string, softwareID string, errMsg string)
}

// UpdateProgressBroadcaster broadcasts update progress events to WebSocket clients.
type UpdateProgressBroadcaster interface {
	BroadcastUpdateProgress(serverID string, serverName string, step xylona.UpdateStep, stepStatus xylona.StepStatus, message string)
}

// SystemUpdateBroadcaster broadcasts controller/node update progress.
type SystemUpdateBroadcaster interface {
	BroadcastSystemUpdateProgress(progress *xylona.SystemUpdateProgress)
}

// XylonaService implements the primary ConnectRPC service for the panel API.
type XylonaService struct {
	ctx                            context.Context
	db                             *db.Connection
	actionsInst                    *actions.Instance
	nodeRegistry                   *noderegistry.Registry
	secureCookie                   *securecookie.SecureCookie
	secureCookies                  bool
	modManager                     *modmanager.ModManager
	steamCache                     *steamcache.Client
	installGameServerFn            func(game *models.Game, gameServer *models.GameServer, owner *models.User) (*models.GameServer, error)
	allPermissionIDs               []string
	permissionIDsForUserFn         func(user *models.User) ([]string, error)
	installTracker                 *modmanager.InstallTracker
	installBroadcast               ServerSoftwareInstallBroadcaster
	updateBroadcast                UpdateProgressBroadcaster
	systemUpdateBroadcast          SystemUpdateBroadcaster
	versionState                   *versiontracker.VersionStateMap
	dummyTracker                   *versiontracker.DummyTracker
	userService                    *usermgmt.Service
	testEmailSendFunc              func(ctx context.Context, cfg *mailer.SMTPConfig, to string, subject string, body string) error
	notificationChannelTestOnce    sync.Once
	notificationChannelTestLimiter *notificationChannelTestRateLimiter
	taskScheduler                  *scheduler.Scheduler
	remoteVersionRefreshMu         sync.Mutex
	remoteVersionRefreshCalls      map[string]*remoteVersionRefreshCall
	hytaleAuth                     *readiness.HytaleDeviceAuthManager
}

type remoteVersionRefreshCall struct {
	done chan struct{}
}

// NewXylonaService constructs the main RPC service implementation.
func NewXylonaService(
	ctx context.Context,
	database *db.Connection,
	actionsInst *actions.Instance,
	nodeRegistry *noderegistry.Registry,
	secureCookie *securecookie.SecureCookie,
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
		secureCookie:     secureCookie,
		secureCookies:    secureCookies,
		nodeRegistry:     nodeRegistry,
		modManager:       modMgr,
		steamCache:       steamCache,
		allPermissionIDs: permIDs,
		installTracker:   tracker,
		versionState:     versionState,
		userService:      usermgmt.NewService(database),
		hytaleAuth:       readiness.NewHytaleDeviceAuthManager(nil),
	}, nil
}

func (xs *XylonaService) hytaleAuthManager() *readiness.HytaleDeviceAuthManager {
	if xs.hytaleAuth == nil {
		xs.hytaleAuth = readiness.NewHytaleDeviceAuthManager(nil)
	}
	return xs.hytaleAuth
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

// SetInstallBroadcaster sets the broadcaster used to push server software install
// status updates over WebSocket.
func (xs *XylonaService) SetInstallBroadcaster(b ServerSoftwareInstallBroadcaster) {
	xs.installBroadcast = b
}

// SetUpdateBroadcaster sets the broadcaster used to push update progress events over WebSocket.
func (xs *XylonaService) SetUpdateBroadcaster(b UpdateProgressBroadcaster) {
	xs.updateBroadcast = b
}

// SetSystemUpdateBroadcaster sets the broadcaster used to push system update events over WebSocket.
func (xs *XylonaService) SetSystemUpdateBroadcaster(b SystemUpdateBroadcaster) {
	xs.systemUpdateBroadcast = b
}

// SetDummyTracker sets the dummy tracker used for testing update failure simulation.
func (xs *XylonaService) SetDummyTracker(dt *versiontracker.DummyTracker) {
	xs.dummyTracker = dt
}
