package db

import (
	"context"
	"database/sql"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	_ "modernc.org/sqlite"
)

type Connection struct {
	ctx   context.Context
	SQLDb *sql.DB
	DB    bob.Executor
}

func sqliteDSNWithPragma(path string, pragma string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=" + url.QueryEscape(pragma)
}

func NewConnection(ctx context.Context, path string) *Connection {
	dsn := sqliteDSNWithPragma(path, "foreign_keys(1)")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("Error connecting to database")
	}
	err = db.PingContext(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Error pinging database")
	}

	var foreignKeysEnabled int
	errForeignKeys := db.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&foreignKeysEnabled)
	if errForeignKeys != nil {
		log.Fatal().Err(errForeignKeys).Msg("Error verifying SQLite foreign key enforcement")
	}
	if foreignKeysEnabled != 1 {
		log.Fatal().Int("foreign_keys", foreignKeysEnabled).Msg("SQLite foreign key enforcement is disabled")
	}

	bobDB := bob.NewDB(db)
	// bobDB := bob.Debug(bob.NewDB(db))
	return &Connection{ctx, db, bobDB}
}
