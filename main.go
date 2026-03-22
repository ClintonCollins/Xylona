package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
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

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/api/rpc"
	"github.com/ClintonCollins/Xylona/api/websocket"
	"github.com/ClintonCollins/Xylona/api/xylona-internal/games"
	dbpkg "github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/gsutils"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/hangar"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/modrinth"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/papermc"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/steamworkshop"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
	"github.com/ClintonCollins/Xylona/pkg/version"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/steamcache"
	"github.com/ClintonCollins/Xylona/supervisor"
)

type Configuration struct {
	CookieHashKey      string `env:"COOKIE_HASH_KEY_BASE64"`
	CookieBlockKey     string `env:"COOKIE_BLOCK_KEY_BASE64"`
	JWTSecretKey       string `env:"JWT_SECRET_KEY_BASE64"`
	DBFilePath         string `env:"DB_FILE_PATH" envDefault:"./data.sqlite"`
	LogLevel           string `env:"LOG_LEVEL" envDefault:"info"`
	SecureCookies      bool   `env:"SECURE_COOKIES" envDefault:"false"`
	HTTPPort           int    `env:"HTTP_PORT" envDefault:"8080"`
	FederationPort     int    `env:"FEDERATION_PORT" envDefault:"8443"`
	FederationCertPath string `env:"FEDERATION_CERT_PATH" envDefault:"./federation/node.crt"`
	FederationKeyPath  string `env:"FEDERATION_KEY_PATH" envDefault:"./federation/node.key"`
}

func setupLogger() {
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
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
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
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
	ctxCancel()
	log.Debug().Msg("Graceful shutdown context cancelled")
	shutdownCtx, shutdownCtxCancel := context.WithTimeout(context.Background(), time.Second*5)
	defer shutdownCtxCancel()

	for _, server := range servers {
		if server == nil {
			continue
		}
		errShutdown := server.Shutdown(shutdownCtx)
		if errShutdown != nil {
			log.Error().Err(errShutdown).Str("addr", server.Addr).Msg("Failed to shutdown server")
		}
	}
	log.Info().Msg("Xylona control panel backend fully stopped.")
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

func main() {
	config := Configuration{}
	_ = godotenv.Load()
	errParseConfig := env.Parse(&config)
	if errParseConfig != nil {
		log.Fatal().Err(errParseConfig).Msg("Error parsing config")
	}
	setupLogger()

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	foundCookieError := false
	if config.CookieHashKey == "" {
		log.Error().Msg("Cookie hash key not set.")
		newCookieHashKey := securecookie.GenerateRandomKey(64)
		encodedHashKey := base64.StdEncoding.EncodeToString(newCookieHashKey)
		log.Info().Str("newCookieHashKey", encodedHashKey).Msg("Generated cookie hash key")
		foundCookieError = true
	}
	if config.CookieBlockKey == "" {
		log.Error().Msg("Cookie block key not set.")
		newCookieBlockKey := securecookie.GenerateRandomKey(32)
		encodedBlockKey := base64.StdEncoding.EncodeToString(newCookieBlockKey)
		log.Info().Str("newCookieBlockKey", encodedBlockKey).Msg("Generated cookie block key")
		foundCookieError = true
	}
	if foundCookieError {
		//nolint:gocritic // log.Fatal calls os.Exit; deferred ctxCancel is intentionally skipped on fatal startup error
		log.Fatal().Msg("Cookie keys not set correctly. You can use the generated key(s)" +
			" above to set the environment variables: COOKIE_HASH_KEY_BASE64 and COOKIE_BLOCK_KEY_BASE64")
	}

	if config.JWTSecretKey == "" {
		log.Error().Msg("JWT secret key not set.")
		newJWTSecretKey := securecookie.GenerateRandomKey(64)
		encodedJWTSecretKey := base64.StdEncoding.EncodeToString(newJWTSecretKey)
		log.Info().Str("newJWTSecretKey", encodedJWTSecretKey).Msg("Generated JWT secret key")
		log.Fatal().Msg("JWT secret key not set correctly. You can use the generated key above to set" +
			" the environment variable: JWT_SECRET_KEY_BASE64")
	}

	cookieHashKey, errDecodeHashKey := base64.StdEncoding.DecodeString(config.CookieHashKey)
	if errDecodeHashKey != nil {
		log.Fatal().Err(errDecodeHashKey).Msg("Error decoding cookie hash key")
	}
	cookieBlockKey, errDecodeBlockKey := base64.StdEncoding.DecodeString(config.CookieBlockKey)
	if errDecodeBlockKey != nil {
		log.Fatal().Err(errDecodeBlockKey).Msg("Error decoding cookie block key")
	}

	secureCookie := securecookie.New(cookieHashKey, cookieBlockKey)

	superInst, errSupervisor := supervisor.New(ctx)
	if errSupervisor != nil {
		log.Fatal().Err(errSupervisor).Msg("Failed to create supervisor instance")
	}
	dbInst := dbpkg.NewConnection(ctx, config.DBFilePath)

	// Run database migrations
	errMigrate := dbpkg.RunMigrations(dbInst.SQLDb, EmbeddedMigrations, "sql/migrations")
	if errMigrate != nil {
		log.Fatal().Err(errMigrate).Msg("Error running migrations")
		return
	}

	settings, errSettings := dbInst.GetLocalSettings()
	if errSettings != nil {
		if !errors.Is(errSettings, sql.ErrNoRows) {
			log.Fatal().Err(errSettings).Msg("Failed to get local settings")
		}
		// Create default settings
		log.Warn().Msg("No settings found. Generating a node ID and default settings.")
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			log.Fatal().Err(errID).Msg("Failed to generate unique ID")
		}
		settings = &models.LocalSetting{
			ID:     1,
			NodeID: newID.String(),
		}
		errInsert := dbInst.UpdateLocalSettings(settings)
		if errInsert != nil {
			log.Fatal().Err(errInsert).Msg("Failed to insert local settings")
		}
		log.Info().Msgf("Generated ID for this node: %s", settings.NodeID)
	}

	// Update node ID in the database to be a real and unique ID.
	node, errGetNode := dbInst.GetNodeByID("1")
	if errGetNode != nil && !errors.Is(errGetNode, sql.ErrNoRows) {
		log.Fatal().Err(errGetNode).Msg("Failed to get node")
	}
	if node != nil {
		_, errExec := dbInst.SQLDb.Exec(`update node set id = ? where id = 1`, settings.NodeID) //nolint:noctx // startup migration, context not meaningful
		if errExec != nil {
			log.Fatal().Err(errExec).Msg("Failed to update node ID")
		}
	}
	errUpdateNodeIdentity := dbInst.UpdateNodeIdentity(
		settings.NodeID,
		version.SystemVersion,
		version.FederationProtocolVersion,
		version.FederationCapabilities,
	)
	if errUpdateNodeIdentity != nil {
		log.Fatal().Err(errUpdateNodeIdentity).Str("node_id", settings.NodeID).Msg("Failed to stamp local node identity")
	}

	localIdentity, errLocalIdentity := dbInst.GetFederationLocalIdentity()
	if errLocalIdentity != nil && !errors.Is(errLocalIdentity, sql.ErrNoRows) {
		log.Fatal().Err(errLocalIdentity).Msg("Failed to load federation local identity")
	}

	var certPEM []byte
	var keyPEM []byte
	if errLocalIdentity == nil {
		certPEM = []byte(localIdentity.CertPEM)
		keyPEM = []byte(localIdentity.KeyPEM)
	}

	federationMTLS, localCertFingerprint, errFederationMTLS := helpers.NewFederationMTLSFromPEM(config.FederationPort, certPEM, keyPEM)
	if errFederationMTLS != nil {
		log.Warn().
			Err(errFederationMTLS).
			Msg("No usable federation certificate in database; generating a new in-database identity")

		var errGenerateFederationPEM error
		certPEM, keyPEM, errGenerateFederationPEM = helpers.GenerateFederationCertificatePEM(settings.NodeID)
		if errGenerateFederationPEM != nil {
			log.Fatal().Err(errGenerateFederationPEM).Msg("Failed to generate federation mTLS identity")
		}

		federationMTLS, localCertFingerprint, errFederationMTLS = helpers.NewFederationMTLSFromPEM(config.FederationPort, certPEM, keyPEM)
		if errFederationMTLS != nil {
			log.Fatal().Err(errFederationMTLS).Msg("Failed to initialize federation mTLS identity from generated certificate")
		}
	}

	errPersistFederationIdentity := dbInst.UpsertFederationLocalIdentity(
		settings.NodeID,
		string(certPEM),
		string(keyPEM),
		localCertFingerprint,
	)
	if errPersistFederationIdentity != nil {
		log.Fatal().Err(errPersistFederationIdentity).Msg("Failed to persist federation local identity")
	}

	modMgr := modmanager.New(dbInst)
	actionsInst := actions.NewInstance(ctx, dbInst, superInst, federationMTLS, modMgr)
	superInst.StartMetricsPoller(ctx)
	_ = actions.NewMetricsRecorder(ctx, dbInst, superInst, settings.NodeID, actionsInst)
	syncEngine := actions.NewFederationSyncEngine(ctx, dbInst, federationMTLS)
	setDetectedIPs(dbInst)

	steamCache := steamcache.New()

	wsInst, websocketHandler := websocket.NewInstance(ctx, superInst, actionsInst, dbInst, secureCookie, federationMTLS)

	router := chi.NewRouter()
	xylonaService := rpc.NewXylonaService(ctx, dbInst, actionsInst, superInst, secureCookie, federationMTLS, config.SecureCookies, steamCache, modMgr)
	xylonaService.SetSyncEngine(syncEngine)
	syncEngine.SetStatusBroadcaster(wsInst)
	syncEngine.SetMetricsBroadcaster(wsInst)
	syncEngine.SetActionsInstance(actionsInst)
	actionsInst.SetSyncEngine(syncEngine)

	xylonaAPIPath, handler := xylonaconnect.NewXylonaHandler(xylonaService, connect.WithHandlerOptions())
	federationService := rpc.NewFederationService(ctx, dbInst, actionsInst, superInst)
	federationAPIPath, federationHandler := xylonaconnect.NewFederationHandler(federationService, connect.WithHandlerOptions())

	frontendFS, errLoadFrontend := Frontend()
	if errLoadFrontend != nil {
		log.Fatal().Err(errLoadFrontend).Msg("Failed to load frontend")
	}

	router.Use(middleware.RealIP)
	router.Use(routerLogger)
	router.Use(securityHeaders)
	router.Use(gatekeeper.AuthRateLimiter())
	router.Mount(xylonaAPIPath, handler)
	router.Mount("/api/websocket", websocketHandler)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", config.HTTPPort),
		Handler:      router,
		ReadTimeout:  time.Hour * 6,
		WriteTimeout: time.Hour * 6,
		IdleTimeout:  time.Hour * 24,
	}

	federationRouter := chi.NewRouter()
	federationRouter.Use(middleware.RealIP)
	federationRouter.Use(routerLogger)

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

	federationServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", config.FederationPort),
		Handler:      federationRouter,
		ReadTimeout:  time.Hour * 6,
		WriteTimeout: time.Hour * 6,
		IdleTimeout:  time.Hour * 24,
		TLSConfig:    federationMTLS.ServerTLSConfig(),
	}

	router.Get("/api/test/{appid}", func(w http.ResponseWriter, r *http.Request) {
		appID := chi.URLParam(r, "appid")
		branches, err := gsutils.SteamGetBranchesByAppID(appID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get branches")
			http.Error(w, "Failed to get branches", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		errEncode := json.NewEncoder(w).Encode(branches)
		if errEncode != nil {
			log.Error().Err(errEncode).Msg("Failed to encode branches")
			http.Error(w, "Failed to encode branches", http.StatusInternalServerError)
			return
		}
	})

	router.Get("/api/test/{appid}/latest", func(w http.ResponseWriter, r *http.Request) {
		appID := chi.URLParam(r, "appid")
		branch, errGetLatest := gsutils.SteamGetLatestVersionByAppID(appID)
		if errGetLatest != nil {
			log.Error().Err(errGetLatest).Msg("Failed to get latest version")
			http.Error(w, "Failed to get latest version", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		errEncode := json.NewEncoder(w).Encode(branch)
		if errEncode != nil {
			log.Error().Err(errEncode).Msg("Failed to encode latest version")
			http.Error(w, "Failed to encode latest version", http.StatusInternalServerError)
			return
		}
	})

	router.Group(func(r chi.Router) {
		r.Use(gatekeeper.RequireSessionAuth(dbInst, secureCookie))
		r.Post("/api/file/get", actionsInst.StreamFileToUser)
		r.Get("/api/file/download/{gameServerId}/{path}", actionsInst.UploadFileToUserGET)
		r.Post("/api/file/download", actionsInst.UploadFileToUserPOST)
		r.Post("/api/file/upload", actionsInst.DownloadGameServerFile)
	})
	router.HandleFunc("/*", handleSPAFunc(frontendFS))

	// Start the web server
	go func() {
		log.Info().Int("port", config.HTTPPort).Msg("Starting Xylona web server")
		errListenAndServe := httpServer.ListenAndServe()
		if errListenAndServe != nil {
			if !errors.Is(errListenAndServe, http.ErrServerClosed) {
				log.Error().Err(errListenAndServe).Msg("Failed to start Xylona web server")
			}
		}
	}()

	go func() {
		log.Info().Int("port", config.FederationPort).Msg("Starting Xylona federation mTLS server")
		errListenAndServeTLS := federationServer.ListenAndServeTLS("", "")
		if errListenAndServeTLS != nil {
			if !errors.Is(errListenAndServeTLS, http.ErrServerClosed) {
				log.Error().Err(errListenAndServeTLS).Msg("Failed to start Xylona federation mTLS server")
			}
		}
	}()

	games.RegisterInternalGames()

	// Handle SIGINT and SIGTERM
	shutdownSignalChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownSignalChannel, os.Interrupt, syscall.SIGTERM)
	shutdownSignalType := <-shutdownSignalChannel
	gracefulShutdown(ctxCancel, shutdownSignalType, httpServer, federationServer)
}
