package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGameServerStatusPagePersistence(t *testing.T) {
	conn := newRBACMigratedConnection(t, "status-page.sqlite")
	seedRBACFixture(t, conn)

	t.Run("reserves case-sensitive identifiers transactionally", func(t *testing.T) {
		page, errCreate := conn.CreateGameServerStatusPage("user-owner", "owner", "Abc_123")
		if errCreate != nil {
			t.Fatalf("CreateGameServerStatusPage() error = %v", errCreate)
		}
		if page.Title != "owner" || page.PublicIdentifier != "Abc_123" || page.Enabled {
			t.Fatalf("CreateGameServerStatusPage() = %+v", page)
		}

		_, errCaseVariant := conn.CreateGameServerStatusPage("user-other", "other", "abc_123")
		if errCaseVariant != nil {
			t.Fatalf("case-sensitive identifier create error = %v", errCaseVariant)
		}

		_, errConflict := conn.CreateGameServerStatusPage("user-admin", "admin", "Abc_123")
		if !errors.Is(errConflict, ErrGameServerStatusPageIdentifierConflict) {
			t.Fatalf("duplicate identifier error = %v, want conflict", errConflict)
		}

		var adminPages int
		errCount := conn.SQLDb.QueryRowContext(
			context.Background(),
			`select count(*) from game_server_status_page where user_id = ?`,
			"user-admin",
		).Scan(&adminPages)
		if errCount != nil || adminPages != 0 {
			t.Fatalf("conflicting create left %d admin pages, error = %v", adminPages, errCount)
		}
	})

	t.Run("updates the page and complete address set atomically", func(t *testing.T) {
		updated, errUpdate := conn.UpdateGameServerStatusPage(GameServerStatusPageUpdate{
			UserID:           "user-owner",
			Title:            "Owner fleet",
			PublicIdentifier: "Owner_Status",
			Enabled:          true,
			ConnectionAddresses: map[string]string{
				"server-local-1": "play.example.test:25565",
			},
		})
		if errUpdate != nil {
			t.Fatalf("UpdateGameServerStatusPage() error = %v", errUpdate)
		}
		if updated.Title != "Owner fleet" || updated.PublicIdentifier != "Owner_Status" || !updated.Enabled {
			t.Fatalf("UpdateGameServerStatusPage() = %+v", updated)
		}

		_, errRetired := conn.GetEnabledGameServerStatusPageByIdentifier("Abc_123")
		if !errors.Is(errRetired, sql.ErrNoRows) {
			t.Fatalf("retired identifier lookup error = %v, want sql.ErrNoRows", errRetired)
		}
		current, errCurrent := conn.GetEnabledGameServerStatusPageByIdentifier("Owner_Status")
		if errCurrent != nil || current.UserID != "user-owner" {
			t.Fatalf("current identifier lookup = %+v, %v", current, errCurrent)
		}

		server, errServer := conn.GetGameServerByID("server-local-1")
		if errServer != nil {
			t.Fatalf("GetGameServerByID() error = %v", errServer)
		}
		if server.PublicConnectionAddress.GetOrZero() != "play.example.test:25565" {
			t.Fatalf("public address = %q", server.PublicConnectionAddress.GetOrZero())
		}

		var reservations int
		errReservations := conn.SQLDb.QueryRowContext(
			context.Background(),
			`select count(*) from game_server_status_page_identifier where identifier in (?, ?)`,
			"Abc_123",
			"Owner_Status",
		).Scan(&reservations)
		if errReservations != nil || reservations != 2 {
			t.Fatalf("identifier reservations = %d, error = %v", reservations, errReservations)
		}
	})

	t.Run("ownership transfer clears the public address", func(t *testing.T) {
		_, errUpdate := conn.UpdateGameServerForEdit(&models.GameServerSetter{
			ID:     omit.From("server-local-1"),
			UserID: omit.From("user-other"),
		}, "user-owner")
		if errUpdate != nil {
			t.Fatalf("UpdateGameServerForEdit() error = %v", errUpdate)
		}

		server, errServer := conn.GetGameServerByID("server-local-1")
		if errServer != nil {
			t.Fatalf("GetGameServerByID() error = %v", errServer)
		}
		if server.UserID != "user-other" || server.PublicConnectionAddress.IsValue() {
			t.Fatalf("transferred server = %+v", server)
		}
	})
}
