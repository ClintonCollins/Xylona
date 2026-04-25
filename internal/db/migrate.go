package db

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/rs/zerolog/log"
	migrate "github.com/rubenv/sql-migrate"
)

// RunMigrations applies all pending SQL migrations to the given database.
// The caller provides the embedded filesystem and root path containing the .sql files.
func RunMigrations(sqlDB *sql.DB, migrationFS embed.FS, root string) error {
	source := migrate.EmbedFileSystemMigrationSource{
		FileSystem: migrationFS,
		Root:       root,
	}
	migrate.SetTable("migrations")
	totalMigrations, errMigrate := migrate.Exec(sqlDB, "sqlite3", source, migrate.Up)
	if totalMigrations > 0 {
		log.Debug().Msgf("Applied %d migrations", totalMigrations)
	}
	if errMigrate != nil {
		return fmt.Errorf("run migrations: %w", errMigrate)
	}
	return nil
}
