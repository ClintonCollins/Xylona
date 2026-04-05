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
	"golang.org/x/crypto/bcrypt"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
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

	ctx := context.Background()
	conn := db.NewConnection(ctx, *dbPath)

	// Run migrations using file-based source (no embed needed)
	migrationSource := &migrate.FileMigrationSource{
		Dir: *migrationsDir,
	}
	migrate.SetTable("migrations")
	totalMigrations, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrate != nil {
		log.Fatal().Err(errMigrate).Msg("Failed to run migrations")
	}
	if totalMigrations > 0 {
		log.Info().Msgf("Applied %d migrations", totalMigrations)
	}

	// Hash the password with bcrypt (must match auth.go's bcrypt.CompareHashAndPassword)
	hashedPassword, errHash := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if errHash != nil {
		log.Fatal().Err(errHash).Msg("Failed to hash password")
	}

	// Generate a unique user ID
	userID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		log.Fatal().Err(errID).Msg("Failed to generate user ID")
	}

	now := time.Now()

	// Create the admin superuser
	_, errCreate := conn.CreateUser(&models.UserSetter{
		ID:           omit.From(userID.String()),
		UserName:     omit.From(*username),
		Email:        omit.From(*username + "@localhost"),
		FirstName:    omit.From("Admin"),
		LastName:     omit.From("User"),
		PasswordHash: omit.From(string(hashedPassword)),
		SuperUser:    omit.From(true),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	})
	if errCreate != nil {
		log.Fatal().Err(errCreate).Msg("Failed to create admin user")
	}

	log.Info().Str("username", *username).Str("id", userID.String()).Msg("Created admin superuser")

	// Create default local settings with a generated node ID
	nodeID, errNodeID := helpers.GenerateUniqueID()
	if errNodeID != nil {
		log.Fatal().Err(errNodeID).Msg("Failed to generate node ID")
	}

	settings := &models.LocalSetting{
		ID:     1,
		NodeID: nodeID.String(),
	}
	errSettings := conn.UpdateLocalSettings(settings)
	if errSettings != nil {
		log.Fatal().Err(errSettings).Msg("Failed to create local settings")
	}

	log.Info().Str("node_id", nodeID.String()).Msg("Created local settings with node ID")
	log.Info().Msg("Database seeded successfully")
}
