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

	t.Run("updates the page and complete server details atomically", func(t *testing.T) {
		updated, errUpdate := conn.UpdateGameServerStatusPage(GameServerStatusPageUpdate{
			UserID:           "user-owner",
			Title:            "Owner fleet",
			PublicIdentifier: "Owner_Status",
			Enabled:          true,
			ServerDetails: map[string]GameServerStatusPageServerDetails{
				"server-local-1": {
					ConnectionAddress: "play.example.test:25565",
					PublicNote:        new("Bring snacks"),
					PublicPassword:    new("join-me"),
				},
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
		details, errDetails := conn.GetGameServerStatusPagePublicServerDetails([]string{"server-local-1"})
		if errDetails != nil {
			t.Fatalf("GetGameServerStatusPagePublicServerDetails() error = %v", errDetails)
		}
		gotDetails := details["server-local-1"]
		if gotDetails.PublicNote.String != "Bring snacks" || gotDetails.PublicPassword.String != "join-me" {
			t.Fatalf("public details = %+v", gotDetails)
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

	t.Run("preserves details omitted by older clients", func(t *testing.T) {
		_, errUpdate := conn.UpdateGameServerStatusPage(GameServerStatusPageUpdate{
			UserID:           "user-owner",
			Title:            "Owner fleet",
			PublicIdentifier: "Owner_Status",
			Enabled:          true,
			ServerDetails: map[string]GameServerStatusPageServerDetails{
				"server-local-1": {ConnectionAddress: "new.example.test:25565"},
			},
		})
		if errUpdate != nil {
			t.Fatalf("UpdateGameServerStatusPage() error = %v", errUpdate)
		}
		details, errDetails := conn.GetGameServerStatusPagePublicServerDetails([]string{"server-local-1"})
		if errDetails != nil {
			t.Fatalf("GetGameServerStatusPagePublicServerDetails() error = %v", errDetails)
		}
		gotDetails := details["server-local-1"]
		if gotDetails.PublicNote.String != "Bring snacks" || gotDetails.PublicPassword.String != "join-me" {
			t.Fatalf("preserved public details = %+v", gotDetails)
		}
	})

	t.Run("rejects unowned details without partial changes", func(t *testing.T) {
		_, errUpdate := conn.UpdateGameServerStatusPage(GameServerStatusPageUpdate{
			UserID:           "user-owner",
			Title:            "Should roll back",
			PublicIdentifier: "Owner_Status",
			Enabled:          true,
			ServerDetails: map[string]GameServerStatusPageServerDetails{
				"server-local-1": {PublicNote: new("Changed")},
				"missing-server": {PublicNote: new("Not owned")},
			},
		})
		if !errors.Is(errUpdate, ErrGameServerStatusPageServerNotOwned) {
			t.Fatalf("UpdateGameServerStatusPage() error = %v, want not owned", errUpdate)
		}
		page, errPage := conn.GetEnabledGameServerStatusPageByIdentifier("Owner_Status")
		if errPage != nil || page.Title != "Owner fleet" {
			t.Fatalf("rolled back page = %+v, %v", page, errPage)
		}
		details, errDetails := conn.GetGameServerStatusPagePublicServerDetails([]string{"server-local-1"})
		if errDetails != nil {
			t.Fatalf("GetGameServerStatusPagePublicServerDetails() error = %v", errDetails)
		}
		if details["server-local-1"].PublicNote.String != "Bring snacks" {
			t.Fatalf("rolled back public details = %+v", details["server-local-1"])
		}
	})

	t.Run("blank details clear published values", func(t *testing.T) {
		_, errUpdate := conn.UpdateGameServerStatusPage(GameServerStatusPageUpdate{
			UserID:           "user-owner",
			Title:            "Owner fleet",
			PublicIdentifier: "Owner_Status",
			Enabled:          true,
			ServerDetails: map[string]GameServerStatusPageServerDetails{
				"server-local-1": {
					PublicNote:     new(""),
					PublicPassword: new(""),
				},
			},
		})
		if errUpdate != nil {
			t.Fatalf("UpdateGameServerStatusPage() error = %v", errUpdate)
		}
		server, errServer := conn.GetGameServerByID("server-local-1")
		if errServer != nil {
			t.Fatalf("GetGameServerByID() error = %v", errServer)
		}
		details, errDetails := conn.GetGameServerStatusPagePublicServerDetails([]string{"server-local-1"})
		if errDetails != nil {
			t.Fatalf("GetGameServerStatusPagePublicServerDetails() error = %v", errDetails)
		}
		if server.PublicConnectionAddress.IsValue() || details["server-local-1"].PublicNote.Valid || details["server-local-1"].PublicPassword.Valid {
			t.Fatalf("cleared server = %+v, details = %+v", server, details["server-local-1"])
		}
	})

	t.Run("batch loads map share and Minecraft state", func(t *testing.T) {
		_, errCreateShare := conn.GetOrCreateGameServerMapShare("server-local-1", "Status_Map")
		if errCreateShare != nil {
			t.Fatalf("GetOrCreateGameServerMapShare() error = %v", errCreateShare)
		}
		_, errEnableShare := conn.UpdateGameServerMapShare("server-local-1", "Status_Map", true)
		if errEnableShare != nil {
			t.Fatalf("UpdateGameServerMapShare() error = %v", errEnableShare)
		}
		errEnableMap := conn.UpdateGameServerMinecraftMapConfig("server-local-1", true, "world", true, "user-owner")
		if errEnableMap != nil {
			t.Fatalf("UpdateGameServerMinecraftMapConfig() error = %v", errEnableMap)
		}
		details, errDetails := conn.GetGameServerStatusPagePublicServerDetails([]string{"server-local-1"})
		if errDetails != nil {
			t.Fatalf("GetGameServerStatusPagePublicServerDetails() error = %v", errDetails)
		}
		gotDetails := details["server-local-1"]
		if gotDetails.MapPublicIdentifier.String != "Status_Map" || !gotDetails.MapShareEnabled || !gotDetails.MinecraftMapEnabled {
			t.Fatalf("enabled map details = %+v", gotDetails)
		}

		_, errDisableShare := conn.UpdateGameServerMapShare("server-local-1", "Status_Map", false)
		if errDisableShare != nil {
			t.Fatalf("UpdateGameServerMapShare(disable) error = %v", errDisableShare)
		}
		errDisableMap := conn.UpdateGameServerMinecraftMapConfig("server-local-1", false, "world", false, "user-owner")
		if errDisableMap != nil {
			t.Fatalf("UpdateGameServerMinecraftMapConfig(disable) error = %v", errDisableMap)
		}
		details, errDetails = conn.GetGameServerStatusPagePublicServerDetails([]string{"server-local-1"})
		if errDetails != nil {
			t.Fatalf("GetGameServerStatusPagePublicServerDetails() disabled error = %v", errDetails)
		}
		gotDetails = details["server-local-1"]
		if gotDetails.MapShareEnabled || gotDetails.MinecraftMapEnabled {
			t.Fatalf("disabled map details = %+v", gotDetails)
		}
	})

	t.Run("ownership transfer clears public status details", func(t *testing.T) {
		_, errPrepare := conn.UpdateGameServerStatusPage(GameServerStatusPageUpdate{
			UserID:           "user-owner",
			Title:            "Owner fleet",
			PublicIdentifier: "Owner_Status",
			Enabled:          true,
			ServerDetails: map[string]GameServerStatusPageServerDetails{
				"server-local-1": {
					ConnectionAddress: "play.example.test:25565",
					PublicNote:        new("Bring snacks"),
					PublicPassword:    new("join-me"),
				},
			},
		})
		if errPrepare != nil {
			t.Fatalf("prepare public details: %v", errPrepare)
		}
		_, errCreateShare := conn.GetOrCreateGameServerMapShare("server-local-1", "Owner_Map")
		if errCreateShare != nil {
			t.Fatalf("GetOrCreateGameServerMapShare() error = %v", errCreateShare)
		}
		_, errEnableShare := conn.UpdateGameServerMapShare("server-local-1", "Owner_Map", true)
		if errEnableShare != nil {
			t.Fatalf("UpdateGameServerMapShare() error = %v", errEnableShare)
		}
		_, errSameOwner := conn.UpdateGameServerForEdit(&models.GameServerSetter{
			ID:     omit.From("server-local-1"),
			UserID: omit.From("user-owner"),
		}, "user-owner")
		if errSameOwner != nil {
			t.Fatalf("UpdateGameServerForEdit(same owner) error = %v", errSameOwner)
		}
		preserved, errPreserved := conn.GetEnabledGameServerMapShareByIdentifier("Owner_Map")
		if errPreserved != nil || preserved.GameServerID != "server-local-1" || !preserved.Enabled {
			t.Fatalf("same-owner map share = %+v, %v", preserved, errPreserved)
		}
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
		details, errDetails := conn.GetGameServerStatusPagePublicServerDetails([]string{"server-local-1"})
		if errDetails != nil {
			t.Fatalf("GetGameServerStatusPagePublicServerDetails() after transfer error = %v", errDetails)
		}
		gotDetails := details["server-local-1"]
		if gotDetails.PublicNote.Valid || gotDetails.PublicPassword.Valid {
			t.Fatalf("transferred public details = %+v", gotDetails)
		}
		_, errOldShare := conn.GetEnabledGameServerMapShareByIdentifier("Owner_Map")
		if !errors.Is(errOldShare, sql.ErrNoRows) {
			t.Fatalf("old map identifier after transfer error = %v, want sql.ErrNoRows", errOldShare)
		}
		_, errDeletedShare := conn.GetGameServerMapShareByGameServerID("server-local-1")
		if !errors.Is(errDeletedShare, sql.ErrNoRows) {
			t.Fatalf("map share after transfer error = %v, want sql.ErrNoRows", errDeletedShare)
		}

	})

}
