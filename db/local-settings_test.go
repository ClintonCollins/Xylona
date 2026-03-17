package db

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGetLocalSettingsAfterSeed(t *testing.T) {
	conn := newRBACMigratedConnection(t, "settings-get.sqlite")
	seedRBACFixture(t, conn)

	settings, errGet := conn.GetLocalSettings()
	if errGet != nil {
		t.Fatalf("GetLocalSettings() error = %v", errGet)
	}
	if settings.NodeID != "node-local" {
		t.Errorf("GetLocalSettings().NodeID = %q, want %q", settings.NodeID, "node-local")
	}
}

func TestGetLocalSettingsNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "settings-empty.sqlite")

	_, errGet := conn.GetLocalSettings()
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetLocalSettings() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestUpdateLocalSettings(t *testing.T) {
	conn := newRBACMigratedConnection(t, "settings-update.sqlite")
	seedRBACFixture(t, conn)

	// Update the settings to point to a different node.
	// First, insert a second node.
	_, errNode := conn.SQLDb.Exec(
		`insert into node (id, name, is_local, host, port, base_url, enabled)
		 values (?, ?, ?, ?, ?, ?, ?)`,
		"node-remote", "Remote Node", false, "10.0.0.1", 8080, "http://10.0.0.1:8080", true,
	)
	if errNode != nil {
		t.Fatalf("failed to insert second node: %v", errNode)
	}

	errUpdate := conn.UpdateLocalSettings(&models.LocalSetting{
		ID:     1,
		NodeID: "node-remote",
	})
	if errUpdate != nil {
		t.Fatalf("UpdateLocalSettings() error = %v", errUpdate)
	}

	settings, errGet := conn.GetLocalSettings()
	if errGet != nil {
		t.Fatalf("GetLocalSettings() error = %v", errGet)
	}
	if settings.NodeID != "node-remote" {
		t.Errorf("GetLocalSettings().NodeID = %q, want %q", settings.NodeID, "node-remote")
	}
}

func TestUpdateLocalSettingsUpsert(t *testing.T) {
	conn := newRBACMigratedConnection(t, "settings-upsert.sqlite")

	// Insert a node first (local_settings.node_id has no FK, but we need a valid value).
	_, errNode := conn.SQLDb.Exec(
		`insert into node (id, name, is_local, host, port, base_url, enabled)
		 values (?, ?, ?, ?, ?, ?, ?)`,
		"node-upsert", "Upsert Node", true, "localhost", 8080, "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("failed to insert node: %v", errNode)
	}

	// No local_settings row exists yet; UpdateLocalSettings should upsert.
	errUpdate := conn.UpdateLocalSettings(&models.LocalSetting{
		ID:     1,
		NodeID: "node-upsert",
	})
	if errUpdate != nil {
		t.Fatalf("UpdateLocalSettings(upsert) error = %v", errUpdate)
	}

	settings, errGet := conn.GetLocalSettings()
	if errGet != nil {
		t.Fatalf("GetLocalSettings() error = %v", errGet)
	}
	if settings.NodeID != "node-upsert" {
		t.Errorf("GetLocalSettings().NodeID = %q, want %q", settings.NodeID, "node-upsert")
	}
}
