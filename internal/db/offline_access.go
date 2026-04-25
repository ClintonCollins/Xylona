package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AppLock tracks the application-level lock that coordinates DB access.
type AppLock struct {
	file   *os.File
	path   string
	locked bool
}

// ResolveDatabasePath converts a database path into a cleaned absolute path.
func ResolveDatabasePath(path string) (string, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == `` {
		return ``, errors.New(`db: database path is required`)
	}

	absPath, errAbs := filepath.Abs(trimmedPath)
	if errAbs != nil {
		return ``, fmt.Errorf(`db: resolve absolute database path: %w`, errAbs)
	}

	return filepath.Clean(absPath), nil
}

// OpenOfflineUserConnection opens the SQLite database for offline user commands.
func OpenOfflineUserConnection(ctx context.Context, path string) (*Connection, string, error) {
	resolvedPath, errResolvePath := ResolveDatabasePath(path)
	if errResolvePath != nil {
		return nil, ``, errResolvePath
	}

	conn, errNewConnection := NewConnection(ctx, resolvedPath)
	if errNewConnection != nil {
		return nil, ``, errNewConnection
	}

	return conn, resolvedPath, nil
}

// AcquireRuntimeDBLock acquires the runtime lock held while the daemon is active.
func AcquireRuntimeDBLock(dbPath string) (*AppLock, error) {
	return acquireAppLock(lockPathForDatabase(dbPath))
}

// AcquireOfflineMutationLock acquires the app-level lock used by offline mutation.
func AcquireOfflineMutationLock(dbPath string) (*AppLock, error) {
	return acquireAppLock(lockPathForDatabase(dbPath))
}

// VerifyOfflineWriteAccess checks that an offline command can take a write lock.
func VerifyOfflineWriteAccess(ctx context.Context, conn *Connection) error {
	sqlConn, errConn := conn.SQLDb.Conn(ctx)
	if errConn != nil {
		return fmt.Errorf(`db: reserve sqlite connection for offline write check: %w`, errConn)
	}
	defer func() {
		_ = sqlConn.Close()
	}()

	_, errBegin := sqlConn.ExecContext(ctx, `BEGIN IMMEDIATE`)
	if errBegin != nil {
		return fmt.Errorf(`db: begin immediate offline write check: %w`, errBegin)
	}

	_, errRollback := sqlConn.ExecContext(ctx, `ROLLBACK`)
	if errRollback != nil && !errors.Is(errRollback, sql.ErrTxDone) {
		return fmt.Errorf(`db: rollback offline write check: %w`, errRollback)
	}

	return nil
}

// Close releases the app lock and closes the underlying file handle.
func (l *AppLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	if l.locked {
		errUnlock := unlockFile(l.file)
		if errUnlock != nil {
			_ = l.file.Close()
			return errUnlock
		}
	}
	l.locked = false
	errClose := l.file.Close()
	if errClose != nil {
		return fmt.Errorf(`db: close app lock file: %w`, errClose)
	}
	return nil
}

func lockPathForDatabase(dbPath string) string {
	return dbPath + `.xylona.lock`
}

func acquireAppLock(lockPath string) (*AppLock, error) {
	resolvedLockPath, errResolvePath := ResolveDatabasePath(lockPath)
	if errResolvePath != nil {
		return nil, errResolvePath
	}

	lockDir := filepath.Dir(resolvedLockPath)
	errMkdir := os.MkdirAll(lockDir, 0o700)
	if errMkdir != nil {
		return nil, fmt.Errorf(`db: create lock directory: %w`, errMkdir)
	}

	lockFile, errOpen := os.OpenFile(resolvedLockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if errOpen != nil {
		return nil, fmt.Errorf(`db: open lock file: %w`, errOpen)
	}

	errLock := lockFileExclusiveNonBlocking(lockFile)
	if errLock != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf(`db: acquire app lock %s: %w`, resolvedLockPath, errLock)
	}

	return &AppLock{
		file:   lockFile,
		path:   resolvedLockPath,
		locked: true,
	}, nil
}
