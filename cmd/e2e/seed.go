package main

import (
	"context"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"
	migrate "github.com/rubenv/sql-migrate"
	"golang.org/x/crypto/bcrypt"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func runSeed(dbPath, username, password, migrationsDir string) error {
	ctx := context.Background()
	conn := db.NewConnection(ctx, dbPath)

	// Run migrations using file-based source.
	migrationSource := &migrate.FileMigrationSource{
		Dir: migrationsDir,
	}
	migrate.SetTable("migrations")
	totalMigrations, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrate != nil {
		return errMigrate
	}
	if totalMigrations > 0 {
		log.Info().Msgf("Applied %d migrations", totalMigrations)
	}

	// Hash the password with bcrypt.
	hashedPassword, errHash := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if errHash != nil {
		return errHash
	}

	// Generate a unique user ID.
	userID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		return errID
	}

	now := time.Now()

	// Create the admin superuser.
	_, errCreate := conn.CreateUser(&models.UserSetter{
		ID:           omit.From(userID.String()),
		UserName:     omit.From(username),
		Email:        omit.From(username + "@localhost"),
		FirstName:    omit.From("Admin"),
		LastName:     omit.From("User"),
		PasswordHash: omit.From(string(hashedPassword)),
		SuperUser:    omit.From(true),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	})
	if errCreate != nil {
		return errCreate
	}

	log.Info().Str("username", username).Str("id", userID.String()).Msg("Created admin superuser")

	// Create default local settings with a generated node ID.
	nodeID, errNodeID := helpers.GenerateUniqueID()
	if errNodeID != nil {
		return errNodeID
	}

	settings := &models.LocalSetting{
		ID:     1,
		NodeID: nodeID.String(),
	}
	errSettings := conn.UpdateLocalSettings(settings)
	if errSettings != nil {
		return errSettings
	}

	log.Info().Str("node_id", nodeID.String()).Msg("Created local settings with node ID")
	log.Info().Msg("Database seeded successfully")
	return nil
}
