// Package main wires together the Xylona application server, embedded frontend,
// and background services.
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/caarlos0/env/v10"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/securecookie"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/api/rpc"
	"github.com/ClintonCollins/Xylona/api/websocket"
	"github.com/ClintonCollins/Xylona/api/xylona-internal/games"
	dbpkg "github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/pkg/alerts"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/mailer"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/hangar"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/modrinth"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/mojang"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/papermc"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/steamworkshop"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
	"github.com/ClintonCollins/Xylona/pkg/scheduler"
	"github.com/ClintonCollins/Xylona/pkg/version"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/webhooks"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/steamcache"
	"github.com/ClintonCollins/Xylona/supervisor"
)

type Configuration struct {
	CookieHashKey  string `env:"COOKIE_HASH_KEY_BASE64"`
	CookieBlockKey string `env:"COOKIE_BLOCK_KEY_BASE64"`
	JWTSecretKey   string `env:"JWT_SECRET_KEY_BASE64"`
	// EncryptionKey is a dedicated base64-encoded key for encrypting sensitive DB
	// fields (notification channel configs, node API keys).
	EncryptionKey          string        `env:"ENCRYPTION_KEY_BASE64"`
	DBFilePath             string        `env:"DB_FILE_PATH" envDefault:"./data.sqlite"`
	LogLevel               string        `env:"LOG_LEVEL" envDefault:"info"`
	SecureCookies          bool          `env:"SECURE_COOKIES" envDefault:"false"`
	MetricsEnabled         bool          `env:"METRICS_ENABLED" envDefault:"false"`
	Host                   string        `env:"HOST" envDefault:""`
	HTTPPort               int           `env:"HTTP_PORT" envDefault:"8080"`
	HTTPReadTimeout        time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"15m"`
	HTTPWriteTimeout       time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"15m"`
	HTTPIdleTimeout        time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"30m"`
	FederationPort         int           `env:"FEDERATION_PORT" envDefault:"8443"`
	FederationReadTimeout  time.Duration `env:"FEDERATION_READ_TIMEOUT" envDefault:"15m"`
	FederationWriteTimeout time.Duration `env:"FEDERATION_WRITE_TIMEOUT" envDefault:"15m"`
	FederationIdleTimeout  time.Duration `env:"FEDERATION_IDLE_TIMEOUT" envDefault:"30m"`
	// DummyGameID enables the DummyTracker for E2E testing. When set, the game
	// with this ID is treated as a trackable server returning a simulated 1.0.0→2.0.0
	// update. Leave empty in production.
	DummyGameID string `env:"DUMMY_GAME_ID" envDefault:""`
}

func setupLogger() func() {
	zerolog.CallerMarshalFunc = func(_ uintptr, file string, line int) string {
		short := file
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				short = file[i+1:]
				break
			}
		}
		file = short
		return file + ":" + strconv.Itoa(line)
	}
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr}
	if logFile := os.Getenv("E2E_LOG_FILE"); logFile != "" {
		cleanLogFile := filepath.Clean(logFile)
		f, errOpen := os.OpenFile(cleanLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if errOpen == nil {
			multi := zerolog.MultiLevelWriter(consoleWriter, f)
			log.Logger = zerolog.New(multi).With().Caller().Timestamp().Logger()
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
			return func() {
				errClose := f.Close()
				if errClose != nil {
					log.Error().Err(errClose).Str("file", cleanLogFile).Msg("Failed to close E2E log file")
				}
			}
		}
	}
	log.Logger = log.Output(consoleWriter).With().Caller().Logger()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	return func() {}
}

func setDetectedIPs(db *dbpkg.Connection) {
	ips, err := helpers.GetIPs()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get IPs")
	}
	for _, ip := range ips {
		log.Debug().Str("ip", ip.String()).Bool("external", !ip.IsPrivate()).Msg("Detected IP")
		// Automatically add any external IPs...
		if !ip.IsPrivate() {
			_, errUpsertIP := db.UpsertIP(&models.IPSetter{
				Address:            omit.From(ip.String()),
				External:           omit.From(!ip.IsPrivate()),
				AutomaticallyAdded: omit.From(true),
			})
			if errUpsertIP != nil && !errors.Is(errUpsertIP, dbpkg.ErrIPConflict) {
				log.Fatal().Err(errUpsertIP).Msg("Failed to upsert IP on startup")
			}
		}
	}
}

func gracefulShutdown(ctxCancel context.CancelFunc, shutdownSignalType os.Signal, servers ...*http.Server) {
	log.Info().Str("Signal", shutdownSignalType.String()).
		Msgf("Received signal: %s. Shutting down.", shutdownSignalType)
	shutdownServers(ctxCancel, servers...)
}

func handleSPAFunc(frontendFS fs.FS) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		sFile, errStat := frontendFS.Open(strings.TrimPrefix(r.URL.Path, "/"))
		if errStat != nil {
			r.URL.Path = "/"
		}
		if errStat == nil {
			_ = sFile.Close()
		}
		http.FileServerFS(frontendFS).ServeHTTP(w, r)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy",
			`default-src 'self'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'; object-src 'none'; `+
				`img-src 'self' data: blob: https:; font-src 'self' data:; style-src 'self' 'unsafe-inline'; `+
				`script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; connect-src 'self' ws: wss: https: http:`)
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			w.Header().Set("Strict-Transport-Security", `max-age=31536000; includeSubDomains`)
		}
		next.ServeHTTP(w, r)
	})
}

func routerLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeStart := time.Now()
		defer func() {
			timeStop := time.Now()
			log.Info().Fields(map[string]any{
				"method":     r.Method,
				"url":        r.URL.Path,
				"ip":         r.RemoteAddr,
				"user-agent": r.UserAgent(),
				"latency":    timeStop.Sub(timeStart).String(),
			}).Msg("")
		}()
		next.ServeHTTP(w, r)
	})
}

// dbSMTPConfigResolver implements mailer.SystemConfigResolver by reading the
// system SMTP config from the database at send time. This ensures the mailer
// always uses the most recently saved configuration.
type dbSMTPConfigResolver struct {
	db *dbpkg.Connection
}

func (r *dbSMTPConfigResolver) ResolveSystemSMTPConfig() (*mailer.SMTPConfig, error) {
	jsonStr, errGet := r.db.GetSystemConfig("smtp_config")
	if errGet != nil {
		return nil, fmt.Errorf("main: load SMTP config: %w", errGet)
	}

	config := &xylona.SystemSMTPConfig{}
	errUnmarshal := protojson.Unmarshal([]byte(jsonStr), config)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("failed to unmarshal system SMTP config: %w", errUnmarshal)
	}

	return &mailer.SMTPConfig{
		Host:       config.GetHost(),
		Port:       int(config.GetPort()),
		User:       config.GetUser(),
		Password:   config.GetPassword(),
		From:       config.GetFromAddress(),
		TLSEnabled: config.GetTlsEnabled(),
	}, nil
}

// setupDatabase opens the database connection, runs migrations, and configures
// the encryption key for sensitive fields. It returns the ready-to-use
// connection or a fatal-level error.
func setupDatabase(ctx context.Context, cfg Configuration) (*dbpkg.Connection, error) {
	dbInst := dbpkg.NewConnection(ctx, cfg.DBFilePath)

	errMigrate := dbpkg.RunMigrations(dbInst.SQLDb, EmbeddedMigrations, "sql/migrations")
	if errMigrate != nil {
		return nil, fmt.Errorf("setupDatabase: run migrations: %w", errMigrate)
	}

	if cfg.EncryptionKey == "" {
		return nil, errors.New("setupDatabase: ENCRYPTION_KEY_BASE64 must be set")
	}
	encKeyBytes, errDecodeEnc := base64.StdEncoding.DecodeString(cfg.EncryptionKey)
	if errDecodeEnc != nil {
		return nil, fmt.Errorf("setupDatabase: decode ENCRYPTION_KEY_BASE64: %w", errDecodeEnc)
	}
	if len(encKeyBytes) < 32 {
		return nil, errors.New("setupDatabase: ENCRYPTION_KEY_BASE64 must decode to at least 32 bytes")
	}
	dbInst.SetEncryptionKey(encKeyBytes[:32])

	return dbInst, nil
}

// setupFederationIdentity loads or generates the federation mTLS identity and
// persists it to the database. It returns the mTLS instance, the local node ID,
// and the local settings.
func setupFederationIdentity(ctx context.Context, dbInst *dbpkg.Connection, cfg Configuration) (*federation.MTLS, *models.LocalSetting, error) {
	_ = ctx // reserved for future use

	settings, errSettings := dbInst.GetLocalSettings()
	if errSettings != nil {
		if !errors.Is(errSettings, sql.ErrNoRows) {
			return nil, nil, fmt.Errorf("setupFederationIdentity: get local settings: %w", errSettings)
		}
		// Create default settings.
		log.Warn().Msg("No settings found. Generating a node ID and default settings.")
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			return nil, nil, fmt.Errorf("setupFederationIdentity: generate unique ID: %w", errID)
		}
		settings = &models.LocalSetting{
			ID:     1,
			NodeID: newID.String(),
		}
		errInsert := dbInst.UpdateLocalSettings(settings)
		if errInsert != nil {
			return nil, nil, fmt.Errorf("setupFederationIdentity: insert local settings: %w", errInsert)
		}
		log.Info().Msgf("Generated ID for this node: %s", settings.NodeID)
	}

	// Update node ID in the database to be a real and unique ID.
	node, errGetNode := dbInst.GetNodeByID("1")
	if errGetNode != nil && !errors.Is(errGetNode, sql.ErrNoRows) {
		return nil, nil, fmt.Errorf("setupFederationIdentity: get node: %w", errGetNode)
	}
	if node != nil {
		_, errExec := dbInst.SQLDb.Exec(`update node set id = ? where id = 1`, settings.NodeID) //nolint:noctx // startup migration, context not meaningful
		if errExec != nil {
			return nil, nil, fmt.Errorf("setupFederationIdentity: update node ID: %w", errExec)
		}
	}
	errUpdateNodeIdentity := dbInst.UpdateNodeIdentity(
		settings.NodeID,
		version.SystemVersion,
		version.FederationProtocolVersion,
		version.FederationCapabilities,
		runtime.GOOS,
	)
	if errUpdateNodeIdentity != nil {
		return nil, nil, fmt.Errorf("setupFederationIdentity: stamp local node identity: %w", errUpdateNodeIdentity)
	}

	localIdentity, errLocalIdentity := dbInst.GetFederationLocalIdentity()
	if errLocalIdentity != nil && !errors.Is(errLocalIdentity, sql.ErrNoRows) {
		return nil, nil, fmt.Errorf("setupFederationIdentity: load federation local identity: %w", errLocalIdentity)
	}

	var certPEM []byte
	var keyPEM []byte
	if errLocalIdentity == nil {
		certPEM = []byte(localIdentity.CertPEM)
		keyPEM = []byte(localIdentity.KeyPEM)
	}

	federationMTLS, localCertFingerprint, errFederationMTLS := federation.NewMTLSFromPEM(cfg.FederationPort, certPEM, keyPEM)
	if errFederationMTLS != nil {
		log.Warn().
			Err(errFederationMTLS).
			Msg("No usable federation certificate in database; generating a new in-database identity")

		var errGenerateFederationPEM error
		certPEM, keyPEM, errGenerateFederationPEM = federation.GenerateCertificatePEM(settings.NodeID)
		if errGenerateFederationPEM != nil {
			return nil, nil, fmt.Errorf("setupFederationIdentity: generate federation mTLS identity: %w", errGenerateFederationPEM)
		}

		federationMTLS, localCertFingerprint, errFederationMTLS = federation.NewMTLSFromPEM(cfg.FederationPort, certPEM, keyPEM)
		if errFederationMTLS != nil {
			return nil, nil, fmt.Errorf("setupFederationIdentity: init federation mTLS from generated cert: %w", errFederationMTLS)
		}
	}

	errPersistFederationIdentity := dbInst.UpsertFederationLocalIdentity(
		settings.NodeID,
		string(certPEM),
		string(keyPEM),
		localCertFingerprint,
	)
	if errPersistFederationIdentity != nil {
		return nil, nil, fmt.Errorf("setupFederationIdentity: persist federation local identity: %w", errPersistFederationIdentity)
	}

	return federationMTLS, settings, nil
}

// setupAlertSystem initializes the alert evaluator and delivery pool. The
// caller must call alertDeliveryPool.Wait() during shutdown to drain pending
// deliveries.
func setupAlertSystem(ctx context.Context, dbInst *dbpkg.Connection, actionsInst *actions.Instance, localNodeID string) *alerts.DeliveryPool {
	actionsInst.StartAlertJobs(localNodeID)
	webhookSender := webhooks.NewSender()
	smtpResolver := &dbSMTPConfigResolver{db: dbInst}
	alertMailer := mailer.New(smtpResolver)
	alertEvaluator, alertJobChan := alerts.NewEvaluator(dbInst, eventbus.Get(), dbInst)
	alertEvaluator.Start(ctx)
	alertDeliveryPool := alerts.NewDeliveryPool(dbInst, dbInst, webhookSender, alertMailer, alertJobChan)
	alertDeliveryPool.Start(ctx)
	return alertDeliveryPool
}

func main() {
	config := Configuration{}
	_ = godotenv.Load()
	errParseConfig := env.Parse(&config)
	if errParseConfig != nil {
		log.Fatal().Err(errParseConfig).Msg("Error parsing config")
	}
	validatedConfig, errValidateConfig := validateConfiguration(config)
	if errValidateConfig != nil {
		log.Fatal().Err(errValidateConfig).Msg("Invalid runtime configuration")
	}

	cleanupLogger := setupLogger()

	ctx, ctxCancel := context.WithCancel(context.Background())

	secureCookie := securecookie.New(validatedConfig.cookieHashKey, validatedConfig.cookieBlockKey)

	superInst, errSupervisor := supervisor.New(ctx)
	if errSupervisor != nil {
		log.Fatal().Err(errSupervisor).Msg("Failed to create supervisor instance")
	}

	dbInst, errDB := setupDatabase(ctx, config)
	if errDB != nil {
		log.Fatal().Err(errDB).Msg("Failed to set up database")
	}

	federationMTLS, settings, errFederation := setupFederationIdentity(ctx, dbInst, config)
	if errFederation != nil {
		log.Fatal().Err(errFederation).Msg("Failed to set up federation identity")
	}

	modMgr := modmanager.New(dbInst)
	versionState := versiontracker.NewVersionStateMap()

	var dummyTracker *versiontracker.DummyTracker
	resolverConfig := versiontracker.ResolverConfig{}
	if config.DummyGameID != "" {
		log.Info().Str("dummy_game_id", config.DummyGameID).Msg("DummyTracker enabled for E2E testing")
		dummyTracker = versiontracker.NewDummyTracker()
		resolverConfig.DummyTracker = dummyTracker
		resolverConfig.DummyGameID = config.DummyGameID
	}

	actionsInst := actions.NewInstance(ctx, dbInst, superInst, federationMTLS, modMgr, versionState, resolverConfig)
	if dummyTracker != nil {
		actionsInst.SetDummyTracker(dummyTracker)
	}
	superInst.StartMetricsPoller(ctx)
	_ = actions.NewMetricsRecorder(ctx, dbInst, superInst, settings.NodeID, actionsInst)

	alertDeliveryPool := setupAlertSystem(ctx, dbInst, actionsInst, settings.NodeID)

	// Scheduled tasks scheduler.
	superAdapter := scheduler.NewSupervisorAdapter(superInst)
	taskScheduler, errScheduler := scheduler.New(ctx, dbInst, actionsInst, actionsInst, superAdapter)
	if errScheduler != nil {
		log.Fatal().Err(errScheduler).Msg("Failed to create task scheduler")
	}
	errSchedulerStart := taskScheduler.Start()
	if errSchedulerStart != nil {
		log.Fatal().Err(errSchedulerStart).Msg("Failed to start task scheduler")
	}

	syncEngine := actions.NewFederationSyncEngine(ctx, dbInst, federationMTLS)
	setDetectedIPs(dbInst)

	steamCache := steamcache.New()

	wsInst, websocketHandler := websocket.NewInstance(ctx, superInst, actionsInst, dbInst, secureCookie, federationMTLS)
	actionsInst.SetVersionBroadcaster(wsInst)
	actionsInst.SetBackupProgressBroadcaster(wsInst)

	router := chi.NewRouter()
	xylonaService := rpc.NewXylonaService(ctx, dbInst, actionsInst, superInst, secureCookie, federationMTLS, config.SecureCookies, steamCache, modMgr, versionState)
	xylonaService.SetSyncEngine(syncEngine)
	xylonaService.SetScheduler(taskScheduler)
	xylonaService.SetInstallBroadcaster(wsInst)
	xylonaService.SetUpdateBroadcaster(wsInst)
	if dummyTracker != nil {
		xylonaService.SetDummyTracker(dummyTracker)
	}
	syncEngine.SetStatusBroadcaster(wsInst)
	syncEngine.SetMetricsBroadcaster(wsInst)
	syncEngine.SetVersionBroadcaster(wsInst)
	syncEngine.SetActionsInstance(actionsInst)
	actionsInst.SetSyncEngine(syncEngine)

	xylonaAPIPath, handler := xylonaconnect.NewXylonaHandler(
		xylonaService,
		connect.WithInterceptors(
			rpc.NewSessionAuthInterceptor(dbInst, secureCookie),
			rpc.NewUnaryTimeoutInterceptor(60*time.Second),
		),
	)
	federationService := rpc.NewFederationService(ctx, dbInst, actionsInst, superInst, versionState)
	federationAPIPath, federationHandler := xylonaconnect.NewFederationHandler(federationService, connect.WithHandlerOptions())

	frontendFS, errLoadFrontend := Frontend()
	if errLoadFrontend != nil {
		log.Fatal().Err(errLoadFrontend).Msg("Failed to load frontend")
	}

	router.Use(middleware.RealIP)
	router.Use(routerLogger)
	router.Use(securityHeaders)
	router.Use(gatekeeper.AuthRateLimiter())
	router.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		errPing := dbInst.SQLDb.PingContext(r.Context())
		if errPing != nil {
			log.Error().Err(errPing).Msg("Health check failed")
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	registerMetricsRoute(router, config)
	router.Mount(xylonaAPIPath, handler)
	router.Mount("/api/websocket", websocketHandler)

	httpServer := newHTTPServer(config, router)

	federationRouter := chi.NewRouter()
	federationRouter.Use(middleware.RealIP)
	federationRouter.Use(routerLogger)
	federationRouter.Use(securityHeaders)

	// The complete-pairing endpoint is exempt from trust-store auth — the pairing token authenticates it.
	federationRouter.Post("/federation/complete-pairing", rpc.CompletePairingHandler(dbInst, federationMTLS))

	federationRouter.Group(func(r chi.Router) {
		r.Use(rpc.FederationPeerAuthMiddleware(dbInst))
		r.Mount(federationAPIPath, federationHandler)
		r.Post("/api/file/get", actionsInst.StreamFileToUser)
		r.Get("/api/file/download/{gameServerId}/{path}", actionsInst.UploadFileToUserGET)
		r.Post("/api/file/download", actionsInst.UploadFileToUserPOST)
		r.Post("/api/file/upload", actionsInst.DownloadGameServerFile)
	})

	federationServer := newFederationServer(config, federationRouter, federationMTLS.ServerTLSConfig())

	router.Group(func(r chi.Router) {
		r.Use(gatekeeper.RequireSessionAuth(dbInst, secureCookie))
		r.Get("/api/backups/download/{gameServerId}/{backupId}", xylonaService.DownloadGameServerBackupArchive)
		r.Post("/api/backups/upload", xylonaService.UploadGameServerBackupArchive)
		r.Post("/api/file/get", actionsInst.StreamFileToUser)
		r.Get("/api/file/download/{gameServerId}/{path}", actionsInst.UploadFileToUserGET)
		r.Post("/api/file/download", actionsInst.UploadFileToUserPOST)
		r.Post("/api/file/upload", actionsInst.DownloadGameServerFile)
	})
	router.HandleFunc("/*", handleSPAFunc(frontendFS))

	// Start the web server
	startupErrCh := make(chan error, 2)
	go func() {
		log.Info().Int("port", config.HTTPPort).Msg("Starting Xylona web server")
		startServer(ctxCancel, "start Xylona web server", httpServer.ListenAndServe, startupErrCh)
	}()

	go func() {
		log.Info().Int("port", config.FederationPort).Msg("Starting Xylona federation mTLS server")
		startServer(ctxCancel, "start Xylona federation mTLS server", func() error {
			return federationServer.ListenAndServeTLS("", "")
		}, startupErrCh)
	}()

	games.RegisterInternalGames()

	// Handle SIGINT and SIGTERM
	shutdownSignalChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownSignalChannel, os.Interrupt, syscall.SIGTERM)
	select {
	case errStartup := <-startupErrCh:
		shutdownServers(ctxCancel, httpServer, federationServer)
		log.Fatal().Err(errStartup).Msg("Startup failed")
	case shutdownSignalType := <-shutdownSignalChannel:
		gracefulShutdown(ctxCancel, shutdownSignalType, httpServer, federationServer)
	}
	alertDeliveryPool.Wait()
	log.Info().Msg("Alert delivery pool drained")
	errStopScheduler := taskScheduler.Stop()
	if errStopScheduler != nil {
		log.Error().Err(errStopScheduler).Msg("Error stopping task scheduler")
	}
	cleanupLogger()
}
