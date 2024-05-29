package main

import (
	"database/sql"
	"embed"

	"github.com/rs/zerolog/log"
	migrate "github.com/rubenv/sql-migrate"
)

//go:embed sql/migrations/*
var embeddedMigrations embed.FS

func runMigrations(db *sql.DB) error {
	migrationFS := migrate.EmbedFileSystemMigrationSource{
		FileSystem: embeddedMigrations,
		Root:       "sql/migrations",
	}
	migrate.SetTable("migrations")
	totalMigrations, err := migrate.Exec(db, "sqlite3", migrationFS, migrate.Up)
	if totalMigrations > 0 {
		log.Debug().Msgf("Applied %d migrations", totalMigrations)
	}
	return err
}
