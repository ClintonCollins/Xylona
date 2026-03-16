package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestGetAllPermissions(t *testing.T) {
	conn := newRBACMigratedConnection(t, "permission.sqlite")

	permissions, errGetPermissions := conn.GetAllPermissions()
	if errGetPermissions != nil {
		t.Fatalf("GetAllPermissions() error = %v", errGetPermissions)
	}

	if len(permissions) < 10 {
		t.Fatalf("GetAllPermissions() len = %d, want at least 10", len(permissions))
	}

	found := false
	for _, permission := range permissions {
		if permission.ID == "game_server.start" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("GetAllPermissions() missing seeded permission %q", "game_server.start")
	}
}

func TestGetPermissionByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "permission-by-id.sqlite")

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{name: "existing", id: "game_server.view", wantErr: false},
		{name: "missing", id: "permission.does.not.exist", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			permission, errGetPermission := conn.GetPermissionByID(tt.id)
			if (errGetPermission != nil) != tt.wantErr {
				t.Fatalf("GetPermissionByID() error = %v, wantErr %v", errGetPermission, tt.wantErr)
			}
			if tt.wantErr {
				if errGetPermission != nil && !errors.Is(errGetPermission, sql.ErrNoRows) {
					t.Errorf("GetPermissionByID() error = %v, want %v", errGetPermission, sql.ErrNoRows)
				}
				return
			}
			if permission == nil {
				t.Fatalf("GetPermissionByID() returned nil permission")
			}
			if permission.ID != tt.id {
				t.Errorf("GetPermissionByID().ID = %q, want %q", permission.ID, tt.id)
			}
		})
	}
}
