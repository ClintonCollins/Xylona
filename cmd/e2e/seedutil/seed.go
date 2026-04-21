// Package seedutil contains shared database seeding logic for E2E test helpers.
package seedutil

import (
	"context"
	"fmt"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/passwordhash"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// Run migrates and seeds a Xylona database with an admin user and local settings.
func Run(dbPath string, username string, password string, migrationsDir string) (errResult error) {
	ctx := context.Background()
	conn, errNewConnection := db.NewConnection(ctx, dbPath)
	if errNewConnection != nil {
		return fmt.Errorf("open seed database: %w", errNewConnection)
	}
	defer func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil && errResult == nil {
			errResult = fmt.Errorf("close seed database: %w", errClose)
		}
	}()

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

	hashedPassword, errHash := passwordhash.Hash(password)
	if errHash != nil {
		return fmt.Errorf("hash seed password: %w", errHash)
	}

	userID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		return fmt.Errorf("generate seed user ID: %w", errID)
	}

	now := time.Now()
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
		return fmt.Errorf("create seed user: %w", errCreate)
	}

	log.Info().Str("username", username).Str("id", userID.String()).Msg("Created admin superuser")

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
		return fmt.Errorf("update local settings: %w", errSettings)
	}

	log.Info().Str("node_id", nodeID.String()).Msg("Created local settings with node ID")
	log.Info().Msg("Database seeded successfully")
	return nil
}
