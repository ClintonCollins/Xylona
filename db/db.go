package db

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	_ "modernc.org/sqlite"
)

type Connection struct {
	ctx           context.Context
	SQLDb         *sql.DB
	DB            bob.Executor
	encryptionKey []byte
}

// SetEncryptionKey sets the AES-256 key used to encrypt sensitive fields
// (e.g., node API keys) at rest. The key must be exactly 32 bytes.
func (c *Connection) SetEncryptionKey(key []byte) {
	c.encryptionKey = key
}

func sqliteDSNWithPragmas(path string, pragmas ...string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	var b strings.Builder
	b.WriteString(path)
	for _, p := range pragmas {
		b.WriteString(separator)
		b.WriteString("_pragma=")
		b.WriteString(url.QueryEscape(p))
		separator = "&"
	}
	return b.String()
}

func sqliteDSNWithPragma(path string, pragma string) string {
	return sqliteDSNWithPragmas(path, pragma)
}

func NewConnection(ctx context.Context, path string) *Connection {
	dsn := sqliteDSNWithPragmas(path,
		"foreign_keys(1)",
		"journal_mode(WAL)",
		"busy_timeout(5000)",
		"synchronous(NORMAL)",
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("Error connecting to database")
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	err = db.PingContext(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Error pinging database")
	}

	var journalMode string
	errJournalMode := db.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode)
	if errJournalMode != nil {
		log.Fatal().Err(errJournalMode).Msg("Error verifying SQLite journal mode")
	}
	if journalMode != "wal" {
		log.Fatal().Str("journal_mode", journalMode).Msg("SQLite journal mode is not WAL")
	}

	var foreignKeysEnabled int
	errForeignKeys := db.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&foreignKeysEnabled)
	if errForeignKeys != nil {
		log.Fatal().Err(errForeignKeys).Msg("Error verifying SQLite foreign key enforcement")
	}
	if foreignKeysEnabled != 1 {
		log.Fatal().Int("foreign_keys", foreignKeysEnabled).Msg("SQLite foreign key enforcement is disabled")
	}

	var syncPragma int
	errSync := db.QueryRowContext(context.Background(), `PRAGMA synchronous`).Scan(&syncPragma)
	if errSync != nil {
		log.Fatal().Err(errSync).Msg("Error verifying SQLite synchronous pragma")
	}
	if syncPragma != 1 { // 1 = NORMAL
		log.Fatal().Int("synchronous", syncPragma).Msg("SQLite synchronous pragma is not NORMAL")
	}

	bobDB := bob.NewDB(db)
	// bobDB := bob.Debug(bob.NewDB(db))
	return &Connection{ctx: ctx, SQLDb: db, DB: bobDB}
}
