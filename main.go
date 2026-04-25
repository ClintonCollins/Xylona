// Package main wires together the Xylona application server, embedded frontend,
// and background services.
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/api/events"
	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/api/rpc"
	"github.com/ClintonCollins/Xylona/api/websocket"
	"github.com/ClintonCollins/Xylona/api/xylona-internal/games"
	dbpkg "github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/pkg/adminipc"
	"github.com/ClintonCollins/Xylona/pkg/alerts"
	"github.com/ClintonCollins/Xylona/pkg/cli/usercmd"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/mailer"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/hangar"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/modrinth"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/mojang"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/papermc"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/steamworkshop"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
	xynode "github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
	"github.com/ClintonCollins/Xylona/pkg/scheduler"
	"github.com/ClintonCollins/Xylona/pkg/selfupdate"
	"github.com/ClintonCollins/Xylona/pkg/usermgmt"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/webhooks"
	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/steamcache"
)

type Configuration struct {
	CookieHashKey  string `env:"COOKIE_HASH_KEY_BASE64"`
	CookieBlockKey string `env:"COOKIE_BLOCK_KEY_BASE64"`
	JWTSecretKey   string `env:"JWT_SECRET_KEY_BASE64"`
	// EncryptionKey is a dedicated base64-encoded key for encrypting sensitive DB
	// fields (notification channel configs).
	EncryptionKey    string        `env:"ENCRYPTION_KEY_BASE64"`
	DBFilePath       string        `env:"DB_FILE_PATH" envDefault:"./data.sqlite"`
	LogLevel         string        `env:"LOG_LEVEL" envDefault:"info"`
	SecureCookies    bool          `env:"SECURE_COOKIES" envDefault:"false"`
	MetricsEnabled   bool          `env:"METRICS_ENABLED" envDefault:"false"`
	Host             string        `env:"HOST" envDefault:""`
	HTTPPort         int           `env:"HTTP_PORT" envDefault:"8080"`
	HTTPReadTimeout  time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"6h"`
	HTTPWriteTimeout time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"6h"`
	HTTPIdleTimeout  time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"24h"`
	// DummyGameID enables the DummyTracker for E2E testing. When set, the game
	// with this ID is treated as a trackable server returning a simulated 1.0.0→2.0.0
	// update. Leave empty in production.
	DummyGameID string `env:"DUMMY_GAME_ID" envDefault:""`
}

func parseLogLevel(logLevel string) (zerolog.Level, error) {
	trimmedLevel := strings.TrimSpace(logLevel)
	if trimmedLevel == "" {
		return zerolog.InfoLevel, nil
	}

	parsedLevel, errParseLevel := zerolog.ParseLevel(strings.ToLower(trimmedLevel))
	if errParseLevel != nil {
		return zerolog.NoLevel, fmt.Errorf("invalid LOG_LEVEL %q: %w", logLevel, errParseLevel)
	}

	return parsedLevel, nil
}

func setupLogger(logLevel string) (func(), error) {
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

	parsedLevel, errParseLevel := parseLogLevel(logLevel)
	if errParseLevel != nil {
		return func() {}, errParseLevel
	}

	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr}
	writer := io.Writer(consoleWriter)
	cleanup := func() {}

	logFile := os.Getenv("E2E_LOG_FILE")
	if logFile != "" {
		cleanLogFile := filepath.Clean(logFile)
		f, errOpen := os.OpenFile(cleanLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if errOpen == nil {
			writer = zerolog.MultiLevelWriter(consoleWriter, f)
			cleanup = func() {
				errClose := f.Close()
				if errClose != nil {
					log.Error().Err(errClose).Str("file", cleanLogFile).Msg("Failed to close E2E log file")
				}
			}
		}
	}

	log.Logger = zerolog.New(writer).With().Caller().Timestamp().Logger()
	zerolog.SetGlobalLevel(parsedLevel)
	return cleanup, nil
}

func setDetectedIPs(db *dbpkg.Connection, nodeID string) error {
	ips, err := helpers.GetBindableIPs()
	if err != nil {
		return fmt.Errorf("setDetectedIPs: get IPs: %w", err)
	}
	for _, ip := range ips {
		log.Debug().Str("ip", ip.String()).Bool("external", !ip.IsPrivate()).Msg("Detected IP")
		// Automatically add any external IPs...
		if !ip.IsPrivate() {
			_, errUpsertIP := db.UpsertIP(&models.IPSetter{
				Address:            omit.From(ip.String()),
				External:           omit.From(!ip.IsPrivate()),
				AutomaticallyAdded: omit.From(true),
				NodeID:             omit.From(nodeID),
			})
			if errUpsertIP != nil && !errors.Is(errUpsertIP, dbpkg.ErrIPConflict) {
				return fmt.Errorf("setDetectedIPs: upsert IP %s: %w", ip.String(), errUpsertIP)
			}
		}
	}

	return nil
}

func gracefulShutdown(ctxCancel context.CancelFunc, shutdownSignalType os.Signal, servers ...*http.Server) {
	log.Info().Str("Signal", shutdownSignalType.String()).
		Msgf("Received signal: %s. Shutting down.", shutdownSignalType)
	shutdownServers(ctxCancel, servers...)
}

func handleSPAFunc(frontendFS fs.FS) func(w http.ResponseWriter, r *http.Request) {
	fileServer := http.FileServerFS(frontendFS)
	return func(w http.ResponseWriter, r *http.Request) {
		sFile, errStat := frontendFS.Open(strings.TrimPrefix(r.URL.Path, "/"))
		if errStat != nil {
			r.URL.Path = "/"
		}
		if errStat == nil {
			_ = sFile.Close()
		}
		fileServer.ServeHTTP(w, r)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		connectSrc := `connect-src 'self' http: ws:`
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			connectSrc = `connect-src 'self' https: wss:`
		}
		w.Header().Set("Content-Security-Policy",
			`default-src 'self'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'; object-src 'none'; `+
				`img-src 'self' data: blob: https:; font-src 'self' data: https://fonts.gstatic.com; `+
				`style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; `+
				`script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; `+
				connectSrc)
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

type readinessPinger interface {
	PingContext(context.Context) error
}

func registerOperationalRoutes(router chi.Router, pinger readinessPinger) {
	router.Get("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	router.Get("/api/ready", func(w http.ResponseWriter, r *http.Request) {
		errPing := pinger.PingContext(r.Context())
		if errPing != nil {
			log.Error().Err(errPing).Msg("Readiness check failed")
			http.Error(w, "unready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
}

func startupFailure(cleanupLogger func(), ctxCancel context.CancelFunc, err error, message string) int {
	ctxCancel()
	cleanupLogger()
	log.Error().Err(err).Msg(message)
	return 1
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
// connection or an error.
func setupDatabase(ctx context.Context, cfg Configuration) (*dbpkg.Connection, error) {
	dbInst, errNewConnection := dbpkg.NewConnection(ctx, cfg.DBFilePath)
	if errNewConnection != nil {
		return nil, fmt.Errorf("setupDatabase: connect to database: %w", errNewConnection)
	}

	errMigrate := dbpkg.RunMigrations(dbInst.SQLDb, EmbeddedMigrations, "sql/migrations")
	if errMigrate != nil {
		_ = dbInst.SQLDb.Close()
		return nil, fmt.Errorf("setupDatabase: run migrations: %w", errMigrate)
	}

	if cfg.EncryptionKey == "" {
		_ = dbInst.SQLDb.Close()
		return nil, errors.New("setupDatabase: ENCRYPTION_KEY_BASE64 must be set")
	}
	encKeyBytes, errDecodeEnc := base64.StdEncoding.DecodeString(cfg.EncryptionKey)
	if errDecodeEnc != nil {
		_ = dbInst.SQLDb.Close()
		return nil, fmt.Errorf("setupDatabase: decode ENCRYPTION_KEY_BASE64: %w", errDecodeEnc)
	}
	if len(encKeyBytes) < xycrypt.EncryptionKeySize {
		_ = dbInst.SQLDb.Close()
		return nil, fmt.Errorf("setupDatabase: ENCRYPTION_KEY_BASE64 must decode to at least %d bytes", xycrypt.EncryptionKeySize)
	}
	// Preserve compatibility with older deployments that supplied longer
	// secrets while still pinning AES-256 to a 32-byte key.
	dbInst.SetEncryptionKey(encKeyBytes[:xycrypt.EncryptionKeySize])

	if cfg.JWTSecretKey != "" {
		jwtFallbackBytes, errDecodeJWTFallback := base64.StdEncoding.DecodeString(cfg.JWTSecretKey)
		if errDecodeJWTFallback == nil && len(jwtFallbackBytes) >= xycrypt.EncryptionKeySize {
			dbInst.SetFallbackEncryptionKey(jwtFallbackBytes[:xycrypt.EncryptionKeySize])
		}
	}

	return dbInst, nil
}

// setupSelfNodeIdentity loads or generates the controller's local node ID. In
// the hub-spoke model the controller's embedded node is registered into the
// node registry using this ID; remote nodes are dialed via the node registry
// once they're paired via the join-token flow (step 6).
func setupSelfNodeIdentity(dbInst *dbpkg.Connection) (*models.LocalSetting, error) {
	settings, errSettings := dbInst.GetLocalSettings()
	if errSettings != nil {
		if !errors.Is(errSettings, sql.ErrNoRows) {
			return nil, fmt.Errorf("setupSelfNodeIdentity: get local settings: %w", errSettings)
		}
		log.Warn().Msg("No settings found. Generating a node ID and default settings.")
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			return nil, fmt.Errorf("setupSelfNodeIdentity: generate unique ID: %w", errID)
		}
		settings = &models.LocalSetting{
			ID:     1,
			NodeID: newID.String(),
		}
		errInsert := dbInst.UpdateLocalSettings(settings)
		if errInsert != nil {
			return nil, fmt.Errorf("setupSelfNodeIdentity: insert local settings: %w", errInsert)
		}
		log.Info().Msgf("Generated ID for this node: %s", settings.NodeID)
	}

	// Ensure a matching row exists in the node table so game_server.node_id
	// foreign keys can resolve to the controller's self-node.
	_, errGetNode := dbInst.GetNodeByID(settings.NodeID)
	if errGetNode != nil {
		if !errors.Is(errGetNode, sql.ErrNoRows) {
			return nil, fmt.Errorf("setupSelfNodeIdentity: look up self node: %w", errGetNode)
		}
		errInsertSelf := dbInst.UpsertSelfNode(settings.NodeID, "Controller")
		if errInsertSelf != nil {
			return nil, fmt.Errorf("setupSelfNodeIdentity: insert self node: %w", errInsertSelf)
		}
	}

	return settings, nil
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
	os.Exit(run(os.Args))
}

var (
	rootCLIStdout           = io.Writer(os.Stdout)
	rootCLIStderr           = io.Writer(os.Stderr)
	runServiceFunc          = runService
	resolveCLIExecutableDir = os.Executable
)

func run(args []string) int {
	handledHelper, errHelper := selfupdate.RunHelperFromArgs(args)
	if handledHelper {
		if errHelper != nil {
			_, _ = fmt.Fprintln(rootCLIStderr, errHelper)
			return 1
		}
		return 0
	}

	serviceExitCode := 0
	rootCommand := newRootCommand(func() int {
		serviceExitCode = runServiceFunc()
		return serviceExitCode
	})

	errRun := rootCommand.Run(context.Background(), args)
	if errRun != nil {
		_, _ = fmt.Fprintln(rootCLIStderr, errRun)
		return 1
	}

	return serviceExitCode
}

func runService() int {
	var cleanupOnce sync.Once
	cleanupLogger := func() {}
	cleanup := func() {
		cleanupOnce.Do(func() {
			cleanupLogger()
		})
	}
	defer cleanup()

	config := Configuration{}
	_ = godotenv.Load()
	errParseConfig := env.Parse(&config)
	if errParseConfig != nil {
		log.Error().Err(errParseConfig).Msg("Error parsing config")
		return 1
	}

	resolvedDBPath, errResolveDBPath := dbpkg.ResolveDatabasePath(config.DBFilePath)
	if errResolveDBPath != nil {
		log.Error().Err(errResolveDBPath).Msg("Failed to resolve database path")
		return 1
	}
	config.DBFilePath = resolvedDBPath

	validatedConfig, errValidateConfig := validateConfiguration(config)
	if errValidateConfig != nil {
		log.Error().Err(errValidateConfig).Msg("Invalid runtime configuration")
		return 1
	}

	var errSetupLogger error
	cleanupLogger, errSetupLogger = setupLogger(config.LogLevel)
	if errSetupLogger != nil {
		log.Error().Err(errSetupLogger).Msg("Error configuring logger")
		return 1
	}
	if len(validatedConfig.cookieHashKey) != 32 && len(validatedConfig.cookieHashKey) != 64 {
		log.Warn().Int("decoded_bytes", len(validatedConfig.cookieHashKey)).Msg("COOKIE_HASH_KEY_BASE64 uses a non-recommended securecookie hash key size; 32 or 64 bytes is recommended")
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	runtimeDBLock, errRuntimeDBLock := dbpkg.AcquireRuntimeDBLock(config.DBFilePath)
	if errRuntimeDBLock != nil {
		return startupFailure(cleanup, ctxCancel, errRuntimeDBLock, "Failed to acquire runtime database lock")
	}
	defer func() {
		_ = runtimeDBLock.Close()
	}()

	secureCookie := securecookie.New(validatedConfig.cookieHashKey, validatedConfig.cookieBlockKey)

	superInst, errSupervisor := supervisor.New(ctx)
	if errSupervisor != nil {
		return startupFailure(cleanup, ctxCancel, errSupervisor, "Failed to create supervisor instance")
	}

	dbInst, errDB := setupDatabase(ctx, config)
	if errDB != nil {
		return startupFailure(cleanup, ctxCancel, errDB, "Failed to set up database")
	}

	errRuntimeSecurity := validateStartupRuntimeSecurity(config, dbInst)
	if errRuntimeSecurity != nil {
		return startupFailure(cleanup, ctxCancel, errRuntimeSecurity, "Runtime security validation failed")
	}

	settings, errSelfIdentity := setupSelfNodeIdentity(dbInst)
	if errSelfIdentity != nil {
		return startupFailure(cleanup, ctxCancel, errSelfIdentity, "Failed to set up self node identity")
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

	// Wrap the supervisor + db in an embedded node and register it with the
	// node registry. Step 2 of the hub-spoke migration routes controller
	// code through nodeRegistry.GetSelf() where the method set on NodeClient
	// already matches the supervisor surface area; direct supervisor calls
	// remain in place for richer APIs (callbacks, PreparedCommand) until
	// later steps extend the interface.
	embeddedNode := xynode.New(ctx, superInst, dbInst)
	embeddedClient := nodeclient.NewInProcessClient(settings.NodeID, embeddedNode)
	nodeRegistry := noderegistry.New(settings.NodeID, embeddedClient)

	actionsInst := actions.NewInstance(ctx, dbInst, embeddedClient, nodeRegistry, modMgr, versionState, resolverConfig)
	if dummyTracker != nil {
		actionsInst.SetDummyTracker(dummyTracker)
	}
	superInst.StartMetricsPoller(ctx)
	_ = actions.NewMetricsRecorder(ctx, dbInst, nodeRegistry, actionsInst)

	alertDeliveryPool := setupAlertSystem(ctx, dbInst, actionsInst, settings.NodeID)

	// Scheduled tasks scheduler.
	taskScheduler, errScheduler := scheduler.New(ctx, dbInst, actionsInst, actionsInst)
	if errScheduler != nil {
		return startupFailure(cleanup, ctxCancel, errScheduler, "Failed to create task scheduler")
	}
	errSchedulerStart := taskScheduler.Start()
	if errSchedulerStart != nil {
		return startupFailure(cleanup, ctxCancel, errSchedulerStart, "Failed to start task scheduler")
	}

	errSetDetectedIPs := setDetectedIPs(dbInst, settings.NodeID)
	if errSetDetectedIPs != nil {
		return startupFailure(cleanup, ctxCancel, errSetDetectedIPs, "Failed to detect startup IPs")
	}

	steamCache := steamcache.New()

	wsInst, websocketHandler := websocket.NewInstance(ctx, actionsInst, dbInst, nodeRegistry, secureCookie)
	actionsInst.SetVersionBroadcaster(wsInst)
	actionsInst.SetBackupProgressBroadcaster(wsInst)

	router := chi.NewRouter()
	xylonaService, errXylonaService := rpc.NewXylonaService(ctx, dbInst, actionsInst, nodeRegistry, secureCookie, config.SecureCookies, steamCache, modMgr, versionState)
	if errXylonaService != nil {
		return startupFailure(cleanup, ctxCancel, errXylonaService, "Failed to create Xylona RPC service")
	}
	localUserService := usermgmt.NewService(dbInst)
	localAdminServer, errLocalAdminServer := adminipc.NewServer(adminipc.ServerConfig{
		DBPath:  config.DBFilePath,
		Handler: adminipc.NewUserHandler(localUserService),
	})
	if errLocalAdminServer != nil {
		return startupFailure(cleanup, ctxCancel, errLocalAdminServer, "Failed to create local admin server")
	}
	// Wire the already-constructed services together here so the startup
	// sequence makes the dependency order explicit without pushing that
	// cross-component wiring into the constructors.
	xylonaService.SetScheduler(taskScheduler)
	xylonaService.SetInstallBroadcaster(wsInst)
	xylonaService.SetUpdateBroadcaster(wsInst)
	xylonaService.SetSystemUpdateBroadcaster(wsInst)
	xylonaService.ResumeSystemUpdateJobs(ctx)
	if dummyTracker != nil {
		xylonaService.SetDummyTracker(dummyTracker)
	}

	xylonaAPIPath, handler := xylonaconnect.NewXylonaHandler(
		xylonaService,
		connect.WithInterceptors(
			rpc.NewSessionAuthInterceptor(dbInst, secureCookie),
			rpc.NewUnaryTimeoutInterceptor(60*time.Second),
		),
	)

	frontendFS, errLoadFrontend := Frontend()
	if errLoadFrontend != nil {
		return startupFailure(cleanup, ctxCancel, errLoadFrontend, "Failed to load frontend")
	}

	router.Use(middleware.RealIP)
	router.Use(routerLogger)
	router.Use(securityHeaders)
	router.Use(gatekeeper.AuthRateLimiter())
	registerOperationalRoutes(router, dbInst.SQLDb)
	registerMetricsRoute(router, config)
	router.Mount(xylonaAPIPath, handler)
	router.Mount("/api/websocket", websocketHandler)
	// Node bootstrap endpoint — auth is by the one-shot join token in the
	// JSON body; does not require a session cookie.
	router.Post("/api/node/bootstrap", rpc.NodeBootstrapHandler(dbInst, nodeRegistry))

	errDialRemotes := dialRegisteredRemoteNodes(ctx, dbInst, settings.NodeID, nodeRegistry)
	if errDialRemotes != nil {
		log.Warn().Err(errDialRemotes).Msg("Some remote nodes could not be dialed at startup")
	}

	// Bridge remote-node event streams into the controller's eventbus so
	// websocket subscribers, auto-restart, and alert evaluators see remote
	// game-server lifecycle events identically to embedded ones.
	remoteEventBridge := events.NewRemoteEventBridge(ctx, nodeRegistry, dbInst, eventbus.Get())
	remoteEventBridge.Start()

	httpServer := newHTTPServer(config, router)

	router.Group(func(r chi.Router) {
		r.Use(gatekeeper.RequireSessionAuth(dbInst, secureCookie))
		r.Get("/api/backups/download/{gameServerId}/{backupId}", xylonaService.DownloadGameServerBackupArchive)
		r.Post("/api/file/get", actionsInst.StreamFileToUser)
		r.Get("/api/file/download/{gameServerId}/{path}", actionsInst.UploadFileToUserGET)
		r.Group(func(r chi.Router) {
			r.Use(gatekeeper.RequireSameOriginFormRequests())
			r.Post("/api/backups/upload", xylonaService.UploadGameServerBackupArchive)
			r.Post("/api/file/download", actionsInst.UploadFileToUserPOST)
			r.Post("/api/file/upload", actionsInst.DownloadGameServerFile)
		})
	})
	router.HandleFunc("/*", handleSPAFunc(frontendFS))

	// Start the web server
	startupErrCh := make(chan error, 3)
	go func() {
		log.Info().Str("endpoint", localAdminServer.Endpoint()).Msg("Starting Xylona local admin server")
		errServe := localAdminServer.Serve()
		if errServe != nil && !errors.Is(errServe, adminipc.ErrServerClosed) {
			startupErrCh <- errServe
		}
	}()
	go func() {
		log.Info().Str("address", fmt.Sprintf("%s:%d", config.Host, config.HTTPPort)).Msg("Starting Xylona web server")
		startServer(ctxCancel, "start Xylona web server", httpServer.ListenAndServe, startupErrCh)
	}()

	games.RegisterInternalGames()

	// Handle SIGINT and SIGTERM
	shutdownSignalChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownSignalChannel, os.Interrupt, syscall.SIGTERM)
	var startupFailed bool
	select {
	case errStartup := <-startupErrCh:
		shutdownLocalAdminServer(localAdminServer)
		shutdownServers(ctxCancel, httpServer)
		log.Error().Err(errStartup).Msg("Startup failed")
		startupFailed = true
	case shutdownSignalType := <-shutdownSignalChannel:
		shutdownLocalAdminServer(localAdminServer)
		gracefulShutdown(ctxCancel, shutdownSignalType, httpServer)
	}
	alertDeliveryPool.Wait()
	log.Info().Msg("Alert delivery pool drained")
	errStopScheduler := taskScheduler.Stop()
	if errStopScheduler != nil {
		log.Error().Err(errStopScheduler).Msg("Error stopping task scheduler")
	}
	if startupFailed {
		return 1
	}

	return 0
}

func newRootCommand(serviceAction func() int) *cli.Command {
	return &cli.Command{
		Name:      `xylona`,
		Usage:     `Run the Xylona service or a management subcommand`,
		UsageText: `xylona [command]`,
		Writer:    rootCLIStdout,
		ErrWriter: rootCLIStderr,
		Action: func(_ context.Context, _ *cli.Command) error {
			serviceAction()
			return nil
		},
		Commands: []*cli.Command{
			usercmd.NewCommand(usercmd.Options{
				Migrate: func(sqlDB *sql.DB) error {
					return dbpkg.RunMigrations(sqlDB, EmbeddedMigrations, `sql/migrations`)
				},
				ResolveDefaultDBPath: resolveDefaultCLIUserDBPath,
			}),
		},
	}
}

func resolveDefaultCLIUserDBPath(_ context.Context) (string, error) {
	dbPath, errResolveDBPath := resolveCLIUserDBPath()
	if errResolveDBPath != nil {
		return "", errResolveDBPath
	}

	resolvedDBPath, errResolveResolvedDBPath := dbpkg.ResolveDatabasePath(dbPath)
	if errResolveResolvedDBPath != nil {
		return ``, fmt.Errorf(`resolve database path for user command: %w`, errResolveResolvedDBPath)
	}

	return resolvedDBPath, nil
}

func resolveCLIUserDBPath() (string, error) {
	processDBPath, processDBPathSet := os.LookupEnv("DB_FILE_PATH")
	trimmedProcessDBPath := strings.TrimSpace(processDBPath)
	if processDBPathSet && trimmedProcessDBPath != "" {
		return trimmedProcessDBPath, nil
	}

	workingDirectory, errWorkingDirectory := os.Getwd()
	if errWorkingDirectory != nil {
		return "", fmt.Errorf("get current working directory for user command: %w", errWorkingDirectory)
	}

	cwdDBPath, foundCWDDBPath, errReadCWDDBPath := readCLIUserDBPathFromEnvFile(workingDirectory)
	if errReadCWDDBPath != nil {
		return "", errReadCWDDBPath
	}
	if foundCWDDBPath {
		return cwdDBPath, nil
	}

	executablePath, errExecutablePath := resolveCLIExecutableDir()
	if errExecutablePath == nil {
		executableDirectory := filepath.Dir(executablePath)
		if filepath.Clean(executableDirectory) != filepath.Clean(workingDirectory) {
			executableDBPath, foundExecutableDBPath, errReadExecutableDBPath := readCLIUserDBPathFromEnvFile(executableDirectory)
			if errReadExecutableDBPath != nil {
				return "", errReadExecutableDBPath
			}
			if foundExecutableDBPath {
				return executableDBPath, nil
			}
		}

		executableDefaultDBPath := filepath.Join(executableDirectory, "data.sqlite")
		executableDefaultDBInfo, errExecutableDefaultDBInfo := os.Stat(executableDefaultDBPath)
		if errExecutableDefaultDBInfo == nil && !executableDefaultDBInfo.IsDir() {
			return executableDefaultDBPath, nil
		}
		if errExecutableDefaultDBInfo != nil && !errors.Is(errExecutableDefaultDBInfo, os.ErrNotExist) {
			return "", fmt.Errorf("stat executable-adjacent database for user command: %w", errExecutableDefaultDBInfo)
		}
	}

	if errExecutablePath != nil && !errors.Is(errExecutablePath, os.ErrNotExist) {
		log.Debug().Err(errExecutablePath).Msg("Failed to resolve executable path for CLI user DB lookup")
	}

	return "./data.sqlite", nil
}

func readCLIUserDBPathFromEnvFile(baseDirectory string) (string, bool, error) {
	envPath := filepath.Join(baseDirectory, ".env")
	envMap, errReadEnv := godotenv.Read(envPath)
	if errReadEnv != nil {
		if errors.Is(errReadEnv, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read user command env file %q: %w", envPath, errReadEnv)
	}

	dbPath := strings.TrimSpace(envMap["DB_FILE_PATH"])
	if dbPath == "" {
		return "", false, nil
	}

	if filepath.IsAbs(dbPath) {
		return dbPath, true, nil
	}

	return filepath.Join(baseDirectory, dbPath), true, nil
}

func shutdownLocalAdminServer(server *adminipc.Server) {
	errShutdown := server.Close()
	if errShutdown != nil {
		log.Error().Err(errShutdown).Msg("Failed to shut down local admin server")
	}
}
