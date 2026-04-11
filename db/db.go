// Package db provides Xylona's SQLite data-access layer.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/stephenafamo/bob"
	_ "modernc.org/sqlite" // Register the SQLite driver.
)

// Connection wraps the SQLite database and ORM executor used by Xylona.
type Connection struct {
	ctx                   context.Context
	SQLDb                 *sql.DB
	DB                    bob.Executor
	encryptionKey         []byte
	fallbackEncryptionKey []byte
}

// SetEncryptionKey sets the required AES-256 key used to encrypt sensitive
// fields (e.g., node API keys) at rest. The key must be exactly 32 bytes.
func (c *Connection) SetEncryptionKey(key []byte) {
	c.encryptionKey = key
}

// SetFallbackEncryptionKey sets a secondary decryption key tried when the
// primary key fails. This supports migration from older ciphertext that used
// the JWT secret-derived database key.
func (c *Connection) SetFallbackEncryptionKey(key []byte) {
	c.fallbackEncryptionKey = key
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

// NewConnection opens the SQLite database and verifies the required pragmas.
func NewConnection(ctx context.Context, path string) (*Connection, error) {
	dsn := sqliteDSNWithPragmas(path,
		"foreign_keys(1)",
		"journal_mode(WAL)",
		"busy_timeout(5000)",
		"synchronous(NORMAL)",
	)
	sqlDb, errOpen := sql.Open("sqlite", dsn)
	if errOpen != nil {
		return nil, fmt.Errorf("db: connect to database: %w", errOpen)
	}
	sqlDb.SetMaxOpenConns(4)
	sqlDb.SetConnMaxLifetime(30 * time.Minute)
	errPing := sqlDb.PingContext(ctx)
	if errPing != nil {
		_ = sqlDb.Close()
		return nil, fmt.Errorf("db: ping database: %w", errPing)
	}

	var journalMode string
	errJournalMode := sqlDb.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode)
	if errJournalMode != nil {
		_ = sqlDb.Close()
		return nil, fmt.Errorf("db: verify SQLite journal mode: %w", errJournalMode)
	}
	if journalMode != "wal" {
		_ = sqlDb.Close()
		return nil, fmt.Errorf("db: SQLite journal mode is %q, want wal", journalMode)
	}

	var foreignKeysEnabled int
	errForeignKeys := sqlDb.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeysEnabled)
	if errForeignKeys != nil {
		_ = sqlDb.Close()
		return nil, fmt.Errorf("db: verify SQLite foreign key enforcement: %w", errForeignKeys)
	}
	if foreignKeysEnabled != 1 {
		_ = sqlDb.Close()
		return nil, errors.New("db: SQLite foreign key enforcement is disabled")
	}

	var syncPragma int
	errSync := sqlDb.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&syncPragma)
	if errSync != nil {
		_ = sqlDb.Close()
		return nil, fmt.Errorf("db: verify SQLite synchronous pragma: %w", errSync)
	}
	if syncPragma != 1 { // 1 = NORMAL
		_ = sqlDb.Close()
		return nil, errors.New("db: SQLite synchronous pragma is not NORMAL")
	}

	bobDB := bob.NewDB(sqlDb)
	// bobDB := bob.Debug(bob.NewDB(db))
	return &Connection{ctx: ctx, SQLDb: sqlDb, DB: bobDB}, nil
}
