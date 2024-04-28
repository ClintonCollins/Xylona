package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/bufbuild/connect-go"
	"github.com/caarlos0/env/v10"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/securecookie"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/api/rpc"
	"github.com/ClintonCollins/Xylona/api/websocket"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/gsutils"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

type Configuration struct {
	CookieHashKey  string `env:"COOKIE_HASH_KEY_BASE64"`
	CookieBlockKey string `env:"COOKIE_BLOCK_KEY_BASE64"`
	DBFilePath     string `env:"DB_FILE_PATH" envDefault:"./data.sqlite"`
	LogLevel       string `env:"LOG_LEVEL" envDefault:"info"`
}

const (
	MaxRequestBodySize = 1024 * 1024 * 10 // 10 MB
)

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

func setDetectedIPs(db *db.Connection) {
	ips, err := helpers.GetIPs()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get IPs")
	}
	for _, ip := range ips {
		log.Debug().Str("ip", ip.String()).Bool("external", !ip.IsPrivate()).Msg("Detected IP")
		_, errUpsertIP := db.UpsertIP(&models.IPSetter{
			Address:  omit.From(ip.String()),
			External: omit.From(!ip.IsPrivate()),
		})
		if errUpsertIP != nil {
			log.Fatal().Err(errUpsertIP).Msg("Failed to upsert IP on startup")
		}
	}
}

func gracefulShutdown(ctxCancel context.CancelFunc, shutdownSignalType os.Signal, webServer *http.Server) {
	log.Info().Str("Signal", shutdownSignalType.String()).
		Msgf("Received signal: %s. Shutting down.", shutdownSignalType)
	ctxCancel()
	log.Debug().Msg("Graceful shutdown context cancelled")
	webServerCtx, webServerCtxCancel := context.WithTimeout(context.Background(), time.Second*5)
	defer webServerCtxCancel()
	log.Debug().Msg("Shutting down Xylona web server.")
	errShutdown := webServer.Shutdown(webServerCtx)
	if errShutdown != nil {
		log.Error().Err(errShutdown).Msg("Failed to shutdown web server")
	}
	log.Info().Msg("Xylona control panel backend fully stopped.")
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
		log.Fatal().Msg("Cookie keys not set correctly. You can use the generated key(s)" +
			" above to set the environment variables COOKIE_HASH_KEY_BASE64 and COOKIE_BLOCK_KEY_BASE64.")
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
	dbInst := db.NewConnection(ctx, "./data.sqlite")
	actionsInst := actions.NewInstance(context.Background(), dbInst, superInst)

	setDetectedIPs(dbInst)

	_, websocketHandler := websocket.NewInstance(ctx, superInst, dbInst, secureCookie)

	router := chi.NewRouter()
	xylonaService := rpc.NewXylonaService(ctx, dbInst, actionsInst, superInst, secureCookie)

	path, handler := xylonaconnect.NewXylonaHandler(xylonaService, connect.WithHandlerOptions())
	router.Mount(path, handler)
	router.Mount("/api/websocket", websocketHandler)

	httpServer := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  time.Second * 60,
		WriteTimeout: time.Second * 60,
		IdleTimeout:  time.Second * 300,
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

	router.Post("/api/file/get", func(w http.ResponseWriter, r *http.Request) {
		fileRequest := xylona.GetFileRequest{}
		bodyBytes, errReadBody := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize))
		if errReadBody != nil {
			log.Error().Err(errReadBody).Msg("Failed to read file request body")
			http.Error(w, "Failed to read file request body", http.StatusBadRequest)
			return
		}
		errDecode := protojson.Unmarshal(bodyBytes, &fileRequest)
		if errDecode != nil {
			log.Error().Err(errDecode).Msg("Failed to decode file request")
			http.Error(w, "Failed to decode file request", http.StatusBadRequest)
			return
		}
		log.Debug().Str("gameServerId", fileRequest.GameServerId).Str("path", fileRequest.Path).Msg("Get file request")
		gameServer, errGetGameServer := dbInst.GetGameServerByID(fileRequest.GameServerId)
		if errGetGameServer != nil {
			if errors.Is(errGetGameServer, sql.ErrNoRows) {
				log.Error().Err(errGetGameServer).Msg("Game server not found")
				http.Error(w, "Game server not found", http.StatusNotFound)
				return
			}
			log.Error().Err(errGetGameServer).Msg("Failed to get game server")
			http.Error(w, "Failed to get game server", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		errGetFile := actionsInst.GetGameServerFile(gameServer, fileRequest.Path, w)
		if errGetFile != nil {
			log.Error().Err(errGetFile).Msg("Failed to get file")
			http.Error(w, "Failed to get file", http.StatusInternalServerError)
			return
		}
	})

	// Start the web server
	go func() {
		log.Info().Msg("Starting Xylona web server on :8080")
		errListenAndServe := httpServer.ListenAndServe()
		if errListenAndServe != nil {
			if !errors.Is(http.ErrServerClosed, errListenAndServe) {
				log.Error().Err(errListenAndServe).Msg("Failed to start Xylona web server")
			}
		}
	}()

	// Handle SIGINT and SIGTERM
	shutdownSignalChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownSignalChannel, os.Interrupt, syscall.SIGTERM)
	shutdownSignalType := <-shutdownSignalChannel
	gracefulShutdown(ctxCancel, shutdownSignalType, httpServer)
}
