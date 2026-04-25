// Package adminipc provides the host-local user-management transport used by
// `xylona user` live mode.
package adminipc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

var (
	// ErrServerClosed reports that the local admin server stopped normally.
	ErrServerClosed = errors.New(`admin ipc server closed`)
	// ErrLiveDaemonRunning reports that offline mode found a reachable live daemon.
	ErrLiveDaemonRunning = errors.New(`live local admin endpoint is reachable`)
	// ErrAppLockHeld reports that the database runtime lock is already held.
	ErrAppLockHeld = errors.New(`app lock is already held`)
)

// ServerConfig configures the local admin IPC server.
type ServerConfig struct {
	DBPath  string
	Handler xylonaconnect.XylonaHandler
}

// Server hosts the local-only admin listener for live CLI user management.
type Server struct {
	endpoint  string
	listener  net.Listener
	httpSrv   *http.Server
	cleanupFn func() error
}

// NewClient creates a local admin client for the database-backed endpoint.
func NewClient(dbPath string) (xylonaconnect.XylonaClient, error) {
	endpoint, errEndpoint := resolveEndpoint(dbPath)
	if errEndpoint != nil {
		return nil, errEndpoint
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialContext(ctx, endpoint)
			},
		},
	}

	return xylonaconnect.NewXylonaClient(httpClient, `http://xylona-local`), nil
}

// NewServer creates a local-only admin IPC server.
func NewServer(config ServerConfig) (*Server, error) {
	if config.Handler == nil {
		return nil, errors.New(`adminipc: handler is required`)
	}

	endpoint, errEndpoint := resolveEndpoint(config.DBPath)
	if errEndpoint != nil {
		return nil, errEndpoint
	}

	listener, cleanupFn, errListen := listen(endpoint)
	if errListen != nil {
		return nil, errListen
	}

	path, handler := xylonaconnect.NewXylonaHandler(config.Handler)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	return &Server{
		endpoint: endpoint,
		listener: listener,
		httpSrv: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
		cleanupFn: cleanupFn,
	}, nil
}

// Endpoint returns the resolved local endpoint for the server.
func (s *Server) Endpoint() string {
	return s.endpoint
}

// Serve runs the local admin server until it is closed.
func (s *Server) Serve() error {
	errServe := s.httpSrv.Serve(s.listener)
	if errServe == nil {
		return nil
	}
	if errors.Is(errServe, http.ErrServerClosed) || errors.Is(errServe, net.ErrClosed) {
		return ErrServerClosed
	}
	return fmt.Errorf(`adminipc: serve local admin server: %w`, errServe)
}

// Close stops the local admin server and cleans up its endpoint.
func (s *Server) Close() error {
	errClose := s.httpSrv.Close()
	errCleanup := s.cleanupFn()
	if errClose != nil && !errors.Is(errClose, http.ErrServerClosed) && !errors.Is(errClose, net.ErrClosed) {
		return fmt.Errorf(`adminipc: close local admin server: %w`, errClose)
	}
	if errCleanup != nil && !errors.Is(errCleanup, net.ErrClosed) {
		return fmt.Errorf(`adminipc: cleanup local admin endpoint: %w`, errCleanup)
	}
	return nil
}

// AcquireAppLock acquires the application-level lock used to guard DB access.
func AcquireAppLock(dbPath string) (*db.AppLock, error) {
	lock, errLock := db.AcquireOfflineMutationLock(dbPath)
	if errLock != nil {
		return nil, fmt.Errorf(`adminipc: acquire app lock: %w`, errLock)
	}
	return lock, nil
}

// GuardOfflineMutation ensures offline mutation is safe before opening the DB.
func GuardOfflineMutation(ctx context.Context, dbPath string) (*db.AppLock, error) {
	client, errClient := NewClient(dbPath)
	if errClient == nil {
		pingCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_, errListUsers := client.ListUsers(pingCtx, connect.NewRequest(&xylona.ListUsersRequest{}))
		if errListUsers == nil {
			return nil, ErrLiveDaemonRunning
		}
	}

	lock, errLock := AcquireAppLock(dbPath)
	if errLock != nil {
		return nil, fmt.Errorf(`%w: %w`, ErrAppLockHeld, errLock)
	}

	conn, _, errConn := db.OpenOfflineUserConnection(ctx, dbPath)
	if errConn != nil {
		_ = lock.Close()
		return nil, fmt.Errorf(`adminipc: open offline connection: %w`, errConn)
	}
	defer func() {
		_ = conn.SQLDb.Close()
	}()

	errVerify := db.VerifyOfflineWriteAccess(ctx, conn)
	if errVerify != nil {
		_ = lock.Close()
		return nil, fmt.Errorf(`adminipc: verify offline write access: %w`, errVerify)
	}

	return lock, nil
}

func resolveEndpoint(dbPath string) (string, error) {
	resolvedDBPath, errResolvePath := db.ResolveDatabasePath(dbPath)
	if errResolvePath != nil {
		return ``, fmt.Errorf(`adminipc: resolve db path: %w`, errResolvePath)
	}

	return endpointForResolvedDatabasePath(resolvedDBPath), nil
}

func endpointHash(resolvedDBPath string) string {
	hash := sha256.Sum256([]byte(endpointHashInput(resolvedDBPath)))
	return hex.EncodeToString(hash[:8])
}
