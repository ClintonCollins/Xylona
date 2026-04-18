// Command e2e-seed seeds a Xylona database for end-to-end tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/passwordhash"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func main() {
	dbPath := flag.String("db", "", "Path to the SQLite database file (required)")
	username := flag.String("username", "admin", "Admin username to create")
	password := flag.String("password", "admin", "Admin password")
	migrationsDir := flag.String("migrations", "sql/migrations", "Path to SQL migrations directory")
	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "error: -db flag is required")
		flag.Usage()
		os.Exit(1)
	}

	errRun := runSeedCommand(*dbPath, *username, *password, *migrationsDir)
	if errRun != nil {
		log.Fatal().Err(errRun).Msg("Failed to seed database")
	}
}

func runSeedCommand(dbPath string, username string, password string, migrationsDir string) (errResult error) {
	ctx := context.Background()
	conn, errNewConnection := db.NewConnection(ctx, dbPath)
	if errNewConnection != nil {
		return fmt.Errorf("open database: %w", errNewConnection)
	}
	defer func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil && errResult == nil {
			errResult = fmt.Errorf("close database: %w", errClose)
		}
	}()

	// Run migrations using file-based source (no embed needed)
	migrationSource := &migrate.FileMigrationSource{
		Dir: migrationsDir,
	}
	migrate.SetTable("migrations")
	totalMigrations, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrate != nil {
		return fmt.Errorf("run migrations: %w", errMigrate)
	}
	if totalMigrations > 0 {
		log.Info().Msgf("Applied %d migrations", totalMigrations)
	}

	// Hash the password with Argon2id.
	hashedPassword, errHash := passwordhash.Hash(password)
	if errHash != nil {
		return fmt.Errorf("hash password: %w", errHash)
	}

	// Generate a unique user ID
	userID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		return fmt.Errorf("generate user ID: %w", errID)
	}

	now := time.Now()

	// Create the admin superuser
	_, errCreate := conn.CreateUser(&models.UserSetter{
		ID:           omit.From(userID.String()),
		UserName:     omit.From(username),
		Email:        omit.From(username + "@localhost"),
		FirstName:    omit.From("Admin"),
		LastName:     omit.From("User"),
		PasswordHash: omit.From(hashedPassword),
		SuperUser:    omit.From(true),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	})
	if errCreate != nil {
		return fmt.Errorf("create admin user: %w", errCreate)
	}

	log.Info().Str("username", username).Str("id", userID.String()).Msg("Created admin superuser")

	// Create default local settings with a generated node ID
	nodeID, errNodeID := helpers.GenerateUniqueID()
	if errNodeID != nil {
		return fmt.Errorf("generate node ID: %w", errNodeID)
	}

	settings := &models.LocalSetting{
		ID:     1,
		NodeID: nodeID.String(),
	}
	errSettings := conn.UpdateLocalSettings(settings)
	if errSettings != nil {
		return fmt.Errorf("create local settings: %w", errSettings)
	}

	log.Info().Str("node_id", nodeID.String()).Msg("Created local settings with node ID")
	log.Info().Msg("Database seeded successfully")
	return nil
}
