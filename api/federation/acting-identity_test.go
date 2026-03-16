package federation

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
)

func newActingIdentityTestDB(t *testing.T) *db.Connection {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "acting-identity.sqlite")
	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close test db: %v", errClose)
		}
	})

	migrationSource := &migrate.FileMigrationSource{
		Dir: filepath.Join("..", "..", "sql", "migrations"),
	}
	migrate.SetTable("migrations")
	_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrate != nil {
		t.Fatalf("failed to apply migrations: %v", errMigrate)
	}

	_, errInsertNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, is_local, host, port, base_url, enabled) values (?, ?, ?, ?, ?, ?, ?)`,
		"node-local", "Local Node", true, "localhost", 8080, "http://localhost:8080", true,
	)
	if errInsertNode != nil {
		t.Fatalf("failed to insert local node: %v", errInsertNode)
	}

	_, errInsertSettings := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into local_settings (id, node_id) values (1, ?) on conflict(id) do update set node_id = excluded.node_id`,
		"node-local",
	)
	if errInsertSettings != nil {
		t.Fatalf("failed to insert local settings: %v", errInsertSettings)
	}

	now := time.Now().UTC()
	_, errInsertUser := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-normal", "normal", "normal@example.com", "Normal", "User", "hash", false, now, now, now,
	)
	if errInsertUser != nil {
		t.Fatalf("failed to insert normal user: %v", errInsertUser)
	}

	_, errInsertSuper := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-super", "super", "super@example.com", "Super", "User", "hash", true, now, now, now,
	)
	if errInsertSuper != nil {
		t.Fatalf("failed to insert super user: %v", errInsertSuper)
	}

	return conn
}

func TestApplyActingIdentityHeadersForUserID(t *testing.T) {
	conn := newActingIdentityTestDB(t)

	tests := []struct {
		name       string
		userID     string
		wantUserID string
		wantNodeID string
		wantSuper  bool
	}{
		{
			name:       "normal user sets user and node headers",
			userID:     "user-normal",
			wantUserID: "user-normal",
			wantNodeID: "node-local",
			wantSuper:  false,
		},
		{
			name:       "super user sets super header",
			userID:     "user-super",
			wantUserID: "user-super",
			wantNodeID: "node-local",
			wantSuper:  true,
		},
		{
			name:      "empty user id is no-op",
			userID:    "",
			wantSuper: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}

			errApply := ApplyActingIdentityHeadersForUserID(conn, header, tt.userID)
			if errApply != nil {
				t.Fatalf("ApplyActingIdentityHeadersForUserID() error = %v", errApply)
			}

			gotUserID, gotNodeID := helpers.GetFederatedActingIdentity(header)
			if gotUserID != tt.wantUserID {
				t.Errorf("acting user header = %q, want %q", gotUserID, tt.wantUserID)
			}
			if gotNodeID != tt.wantNodeID {
				t.Errorf("origin node header = %q, want %q", gotNodeID, tt.wantNodeID)
			}
			if gotSuper := helpers.FederatedActingIsSuperUser(header); gotSuper != tt.wantSuper {
				t.Errorf("super user header = %v, want %v", gotSuper, tt.wantSuper)
			}
		})
	}
}
