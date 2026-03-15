package db

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/version"
)

func TestUpdateNodeIdentityDoesNotOverwriteName(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "peer-node.sqlite")
	conn := NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})

	_, errCreate := conn.SQLDb.Exec(`
		CREATE TABLE node (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			version TEXT NOT NULL DEFAULT '',
			protocol_version INTEGER NOT NULL DEFAULT 0,
			capabilities TEXT NOT NULL DEFAULT '',
			updated_at DATETIME
		)
	`)
	if errCreate != nil {
		t.Fatalf("failed to create table: %v", errCreate)
	}

	_, errInsert := conn.SQLDb.Exec(fmt.Sprintf(`
		INSERT INTO node (id, name, version, protocol_version, capabilities)
		VALUES ('node-1', 'Custom Remote Name', '%s', %d, 'server_list')
	`, version.SoftwareVersion, version.FederationProtocolVersion))
	if errInsert != nil {
		t.Fatalf("failed to insert node: %v", errInsert)
	}

	errUpdate := conn.UpdateNodeIdentity("node-1", "0.2.0", 2, "server_list,status_streaming")
	if errUpdate != nil {
		t.Fatalf("UpdateNodeIdentity() error = %v", errUpdate)
	}

	var gotName string
	var gotVersion string
	var gotProtocolVersion int
	var gotCapabilities string
	errQuery := conn.SQLDb.QueryRow(`
		SELECT name, version, protocol_version, capabilities
		FROM node
		WHERE id = 'node-1'
	`).Scan(&gotName, &gotVersion, &gotProtocolVersion, &gotCapabilities)
	if errQuery != nil {
		t.Fatalf("failed to read node: %v", errQuery)
	}

	if gotName != "Custom Remote Name" {
		t.Errorf("name = %q, want %q", gotName, "Custom Remote Name")
	}
	if gotVersion != "0.2.0" {
		t.Errorf("version = %q, want %q", gotVersion, "0.2.0")
	}
	if gotProtocolVersion != 2 {
		t.Errorf("protocol_version = %d, want %d", gotProtocolVersion, 2)
	}
	if gotCapabilities != "server_list,status_streaming" {
		t.Errorf("capabilities = %q, want %q", gotCapabilities, "server_list,status_streaming")
	}
}

func TestGetNodeSyncIntervalSeconds(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "peer-node-sync-interval.sqlite")
	conn := NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})

	_, errCreate := conn.SQLDb.Exec(`
		CREATE TABLE node (
			id TEXT PRIMARY KEY NOT NULL,
			sync_interval_seconds INTEGER NOT NULL DEFAULT 60
		)
	`)
	if errCreate != nil {
		t.Fatalf("failed to create table: %v", errCreate)
	}

	_, errInsert := conn.SQLDb.Exec(`
		INSERT INTO node (id, sync_interval_seconds)
		VALUES ('node-1', 25)
	`)
	if errInsert != nil {
		t.Fatalf("failed to insert node: %v", errInsert)
	}

	gotInterval, errGetInterval := conn.GetNodeSyncIntervalSeconds("node-1")
	if errGetInterval != nil {
		t.Fatalf("GetNodeSyncIntervalSeconds() error = %v", errGetInterval)
	}
	if gotInterval != 25 {
		t.Errorf("sync interval = %d, want %d", gotInterval, 25)
	}
}
