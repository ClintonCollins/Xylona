package db

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
)

func TestCreateUserRoleAssignmentAndQueries(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment.sqlite")
	seedRBACFixture(t, conn)

	errCreateAssignment := conn.CreateUserRoleAssignment(
		"assignment-1",
		"user-other",
		"operator",
		"server-local-1",
		"user-owner",
	)
	if errCreateAssignment != nil {
		t.Fatalf("CreateUserRoleAssignment() error = %v", errCreateAssignment)
	}

	assignmentsForServer, errServerAssignments := conn.GetUserRoleAssignmentsForServer("server-local-1")
	if errServerAssignments != nil {
		t.Fatalf("GetUserRoleAssignmentsForServer() error = %v", errServerAssignments)
	}
	if len(assignmentsForServer) != 1 {
		t.Fatalf("GetUserRoleAssignmentsForServer() len = %d, want 1", len(assignmentsForServer))
	}

	assignmentsForUser, errUserAssignments := conn.GetUserRoleAssignmentsForUser("user-other")
	if errUserAssignments != nil {
		t.Fatalf("GetUserRoleAssignmentsForUser() error = %v", errUserAssignments)
	}
	if len(assignmentsForUser) != 1 {
		t.Fatalf("GetUserRoleAssignmentsForUser() len = %d, want 1", len(assignmentsForUser))
	}

	gotAssignment, errGetAssignment := conn.GetUserRoleAssignmentByID("assignment-1")
	if errGetAssignment != nil {
		t.Fatalf("GetUserRoleAssignmentByID() error = %v", errGetAssignment)
	}
	if gotAssignment.RoleID != "operator" {
		t.Errorf("GetUserRoleAssignmentByID().RoleID = %q, want %q", gotAssignment.RoleID, "operator")
	}
}

func TestCreateUserRoleAssignmentGlobalScope(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment-global.sqlite")
	seedRBACFixture(t, conn)

	errCreateAssignment := conn.CreateUserRoleAssignment(
		"assignment-global-1",
		"user-other",
		"viewer",
		"",
		"user-owner",
	)
	if errCreateAssignment != nil {
		t.Fatalf("CreateUserRoleAssignment() error = %v", errCreateAssignment)
	}

	gotAssignment, errGetAssignment := conn.GetUserRoleAssignmentByID("assignment-global-1")
	if errGetAssignment != nil {
		t.Fatalf("GetUserRoleAssignmentByID() error = %v", errGetAssignment)
	}

	if gotAssignment.GameServerID.IsValue() && !gotAssignment.GameServerID.IsNull() {
		t.Errorf("GetUserRoleAssignmentByID().GameServerID should be NULL when set")
	}

	compositeAssignment, errComposite := conn.GetUserRoleAssignmentByComposite("user-other", "viewer", "")
	if errComposite != nil {
		t.Fatalf("GetUserRoleAssignmentByComposite() error = %v", errComposite)
	}
	if compositeAssignment.ID != "assignment-global-1" {
		t.Errorf("GetUserRoleAssignmentByComposite().ID = %q, want %q", compositeAssignment.ID, "assignment-global-1")
	}
}

func TestCreateUserRoleAssignmentUniqueConstraint(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment-unique.sqlite")
	seedRBACFixture(t, conn)

	errFirst := conn.CreateUserRoleAssignment(
		"assignment-unique-1",
		"user-other",
		"operator",
		"server-local-1",
		"user-owner",
	)
	if errFirst != nil {
		t.Fatalf("CreateUserRoleAssignment(first) error = %v", errFirst)
	}

	errSecond := conn.CreateUserRoleAssignment(
		"assignment-unique-2",
		"user-other",
		"operator",
		"server-local-1",
		"user-owner",
	)
	if errSecond == nil {
		t.Fatalf("CreateUserRoleAssignment(second) expected unique constraint error, got nil")
	}
}

func TestUserHasPermissionOnServer(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment-permission.sqlite")
	seedRBACFixture(t, conn)

	errCreateServerScope := conn.CreateUserRoleAssignment(
		"assignment-server-scope",
		"user-other",
		"operator",
		"server-local-1",
		"user-owner",
	)
	if errCreateServerScope != nil {
		t.Fatalf("CreateUserRoleAssignment(server) error = %v", errCreateServerScope)
	}

	errCreateGlobalScope := conn.CreateUserRoleAssignment(
		"assignment-global-scope",
		"user-other",
		"viewer",
		"",
		"user-owner",
	)
	if errCreateGlobalScope != nil {
		t.Fatalf("CreateUserRoleAssignment(global) error = %v", errCreateGlobalScope)
	}

	tests := []struct {
		name         string
		userID       string
		serverID     string
		permissionID string
		want         bool
	}{
		{
			name:         "server-scoped permission matches",
			userID:       "user-other",
			serverID:     "server-local-1",
			permissionID: "game_server.start",
			want:         true,
		},
		{
			name:         "global permission matches",
			userID:       "user-other",
			serverID:     "server-local-1",
			permissionID: "game_server.view",
			want:         true,
		},
		{
			name:         "permission does not match",
			userID:       "user-other",
			serverID:     "server-local-1",
			permissionID: "game_server.delete",
			want:         false,
		},
		{
			name:         "user does not have assignments",
			userID:       "user-owner",
			serverID:     "server-local-1",
			permissionID: "game_server.view",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errPermission := conn.UserHasPermissionOnServer(tt.userID, tt.serverID, tt.permissionID)
			if errPermission != nil {
				t.Fatalf("UserHasPermissionOnServer() error = %v", errPermission)
			}
			if got != tt.want {
				t.Errorf("UserHasPermissionOnServer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeleteUserRoleAssignment(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment-delete.sqlite")
	seedRBACFixture(t, conn)

	errCreateAssignment := conn.CreateUserRoleAssignment(
		"assignment-delete-1",
		"user-other",
		"operator",
		"server-local-1",
		"user-owner",
	)
	if errCreateAssignment != nil {
		t.Fatalf("CreateUserRoleAssignment() error = %v", errCreateAssignment)
	}

	errDeleteAssignment := conn.DeleteUserRoleAssignment("assignment-delete-1")
	if errDeleteAssignment != nil {
		t.Fatalf("DeleteUserRoleAssignment() error = %v", errDeleteAssignment)
	}

	_, errGetAssignment := conn.GetUserRoleAssignmentByID("assignment-delete-1")
	if !errors.Is(errGetAssignment, sql.ErrNoRows) {
		t.Errorf("GetUserRoleAssignmentByID() error = %v, want %v", errGetAssignment, sql.ErrNoRows)
	}
}

func TestUserRoleAssignmentCascadesOnGameServerDelete(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment-cascade.sqlite")
	seedRBACFixture(t, conn)

	errCreateAssignment := conn.CreateUserRoleAssignment(
		"assignment-cascade-1",
		"user-other",
		"operator",
		"server-local-1",
		"user-owner",
	)
	if errCreateAssignment != nil {
		t.Fatalf("CreateUserRoleAssignment() error = %v", errCreateAssignment)
	}

	_, errDeleteServer := conn.SQLDb.ExecContext(
		conn.ctx,
		`delete from game_server where id = ?`,
		"server-local-1",
	)
	if errDeleteServer != nil {
		t.Fatalf("delete game_server error = %v", errDeleteServer)
	}

	_, errGetAssignment := conn.GetUserRoleAssignmentByID("assignment-cascade-1")
	if !errors.Is(errGetAssignment, sql.ErrNoRows) {
		t.Errorf("GetUserRoleAssignmentByID() error = %v, want %v", errGetAssignment, sql.ErrNoRows)
	}
}

func TestCreateUserRoleAssignmentMissingReference(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment-fk.sqlite")
	seedRBACFixture(t, conn)

	errCreateAssignment := conn.CreateUserRoleAssignment(
		"assignment-fk-1",
		"user-missing",
		"operator",
		"server-local-1",
		"user-owner",
	)
	if errCreateAssignment == nil {
		t.Fatalf("CreateUserRoleAssignment() expected foreign key error, got nil")
	}
}

func TestGetUserPermissionIDsForServer(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-perm-test.sqlite")
	seedRBACFixture(t, conn)

	t.Run("user with no assignments returns empty", func(t *testing.T) {
		perms, errPerms := conn.GetUserPermissionIDsForServer("user-owner", "server-local-1")
		if errPerms != nil {
			t.Fatalf("GetUserPermissionIDsForServer() error = %v", errPerms)
		}
		if len(perms) != 0 {
			t.Errorf("GetUserPermissionIDsForServer() len = %d, want 0", len(perms))
		}
	})

	t.Run("operator role includes player management", func(t *testing.T) {
		errCreateAssignment := conn.CreateUserRoleAssignment(
			"assignment-perm-1",
			"user-other",
			"operator",
			"server-local-1",
			"user-owner",
		)
		if errCreateAssignment != nil {
			t.Fatalf("CreateUserRoleAssignment() error = %v", errCreateAssignment)
		}

		perms, errPerms := conn.GetUserPermissionIDsForServer("user-other", "server-local-1")
		if errPerms != nil {
			t.Fatalf("GetUserPermissionIDsForServer() error = %v", errPerms)
		}
		if len(perms) != 7 {
			t.Errorf("GetUserPermissionIDsForServer() len = %d, want 7; got %v", len(perms), perms)
		}
		if !slices.Contains(perms, "game_server.players.manage") {
			t.Errorf("GetUserPermissionIDsForServer() missing game_server.players.manage; got %v", perms)
		}
	})
}

func TestGetUserPermissionIDsForServers(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-perm-bulk-test.sqlite")
	seedRBACFixture(t, conn)

	t.Run("empty server list returns empty map", func(t *testing.T) {
		result, errResult := conn.GetUserPermissionIDsForServers("user-other", []string{})
		if errResult != nil {
			t.Fatalf("GetUserPermissionIDsForServers() error = %v", errResult)
		}
		if len(result) != 0 {
			t.Errorf("GetUserPermissionIDsForServers() len = %d, want 0", len(result))
		}
	})

	t.Run("operator on one server, nonexistent server not in result", func(t *testing.T) {
		errCreateAssignment := conn.CreateUserRoleAssignment(
			"assignment-bulk-1",
			"user-other",
			"operator",
			"server-local-1",
			"user-owner",
		)
		if errCreateAssignment != nil {
			t.Fatalf("CreateUserRoleAssignment() error = %v", errCreateAssignment)
		}

		result, errResult := conn.GetUserPermissionIDsForServers("user-other", []string{"server-local-1", "nonexistent-server"})
		if errResult != nil {
			t.Fatalf("GetUserPermissionIDsForServers() error = %v", errResult)
		}

		perms, ok := result["server-local-1"]
		if !ok {
			t.Fatalf("GetUserPermissionIDsForServers() missing key server-local-1; got %v", result)
		}
		if len(perms) != 7 {
			t.Errorf("GetUserPermissionIDsForServers()[server-local-1] len = %d, want 7; got %v", len(perms), perms)
		}
		if !slices.Contains(perms, "game_server.players.manage") {
			t.Errorf("GetUserPermissionIDsForServers()[server-local-1] missing game_server.players.manage; got %v", perms)
		}

		if _, exists := result["nonexistent-server"]; exists {
			t.Errorf("GetUserPermissionIDsForServers() has unexpected key nonexistent-server")
		}
	})
}

func TestUserHasGlobalPermission(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment-global-perm.sqlite")
	seedRBACFixture(t, conn)

	// Create a custom role with alerts.manage
	_, errRole := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role (id, name, description, is_system) VALUES (?, ?, ?, ?)`,
		"role-alert-mgr", "Alert Manager", "Can manage alerts", false,
	)
	if errRole != nil {
		t.Fatalf("failed to insert role: %v", errRole)
	}

	_, errRolePerm := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
		"role-alert-mgr", "alerts.manage",
	)
	if errRolePerm != nil {
		t.Fatalf("failed to insert role_permission: %v", errRolePerm)
	}

	// Assign globally to user-other (game_server_id IS NULL)
	errGlobal := conn.CreateUserRoleAssignment("assign-global", "user-other", "role-alert-mgr", "", "user-owner")
	if errGlobal != nil {
		t.Fatalf("CreateUserRoleAssignment(global) error = %v", errGlobal)
	}

	// Assign server-scoped to user-owner (game_server_id = "server-local-1")
	errScoped := conn.CreateUserRoleAssignment("assign-scoped", "user-owner", "role-alert-mgr", "server-local-1", "user-admin")
	if errScoped != nil {
		t.Fatalf("CreateUserRoleAssignment(scoped) error = %v", errScoped)
	}

	tests := []struct {
		name         string
		userID       string
		permissionID string
		want         bool
	}{
		{
			name:         "global assignment matches",
			userID:       "user-other",
			permissionID: "alerts.manage",
			want:         true,
		},
		{
			name:         "server-scoped assignment does not match global check",
			userID:       "user-owner",
			permissionID: "alerts.manage",
			want:         false,
		},
		{
			name:         "no assignments at all",
			userID:       "user-admin",
			permissionID: "alerts.manage",
			want:         false,
		},
		{
			name:         "global assignment but wrong permission",
			userID:       "user-other",
			permissionID: "alerts.view_history",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errPerm := conn.UserHasGlobalPermission(tt.userID, tt.permissionID)
			if errPerm != nil {
				t.Fatalf("UserHasGlobalPermission() error = %v", errPerm)
			}
			if got != tt.want {
				t.Errorf("UserHasGlobalPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUserGlobalPermissionIDs(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment-global-perm-ids.sqlite")
	seedRBACFixture(t, conn)

	_, errRole := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role (id, name, description, is_system) VALUES (?, ?, ?, ?)`,
		"role-alert-auditor", "Alert Auditor", "Can view and manage alerts", false,
	)
	if errRole != nil {
		t.Fatalf("failed to insert role: %v", errRole)
	}

	for _, permissionID := range []string{"alerts.manage", "alerts.view_history"} {
		_, errRolePerm := conn.SQLDb.ExecContext(
			context.Background(),
			`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
			"role-alert-auditor", permissionID,
		)
		if errRolePerm != nil {
			t.Fatalf("failed to insert role_permission %q: %v", permissionID, errRolePerm)
		}
	}

	errAssignGlobal := conn.CreateUserRoleAssignment(
		"assign-global-alert-auditor",
		"user-other",
		"role-alert-auditor",
		"",
		"user-owner",
	)
	if errAssignGlobal != nil {
		t.Fatalf("CreateUserRoleAssignment(global) error = %v", errAssignGlobal)
	}

	errAssignScoped := conn.CreateUserRoleAssignment(
		"assign-scoped-alert-auditor",
		"user-other",
		"role-alert-auditor",
		"server-local-1",
		"user-owner",
	)
	if errAssignScoped != nil {
		t.Fatalf("CreateUserRoleAssignment(scoped) error = %v", errAssignScoped)
	}

	permissionIDs, errPerms := conn.GetUserGlobalPermissionIDs("user-other")
	if errPerms != nil {
		t.Fatalf("GetUserGlobalPermissionIDs() error = %v", errPerms)
	}

	if len(permissionIDs) != 2 {
		t.Fatalf("GetUserGlobalPermissionIDs() len = %d, want 2; got %v", len(permissionIDs), permissionIDs)
	}

	got := make(map[string]struct{}, len(permissionIDs))
	for _, permissionID := range permissionIDs {
		got[permissionID] = struct{}{}
	}

	for _, permissionID := range []string{"alerts.manage", "alerts.view_history"} {
		if _, ok := got[permissionID]; !ok {
			t.Errorf("GetUserGlobalPermissionIDs() missing %q; got %v", permissionID, permissionIDs)
		}
	}
}

func TestUserHasAnyGlobalPermission(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-role-assignment-any-global-perm.sqlite")
	seedRBACFixture(t, conn)

	_, errRole := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role (id, name, description, is_system) VALUES (?, ?, ?, ?)`,
		"role-alert-auditor-any", "Alert Auditor", "Can view alert history", false,
	)
	if errRole != nil {
		t.Fatalf("failed to insert role: %v", errRole)
	}

	_, errManagePerm := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
		"role-alert-auditor-any", "alerts.manage",
	)
	if errManagePerm != nil {
		t.Fatalf("failed to insert alerts.manage role_permission: %v", errManagePerm)
	}

	_, errViewPerm := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
		"role-alert-auditor-any", "alerts.view_history",
	)
	if errViewPerm != nil {
		t.Fatalf("failed to insert alerts.view_history role_permission: %v", errViewPerm)
	}

	errAssignGlobal := conn.CreateUserRoleAssignment(
		"assign-global-any",
		"user-other",
		"role-alert-auditor-any",
		"",
		"user-owner",
	)
	if errAssignGlobal != nil {
		t.Fatalf("CreateUserRoleAssignment(global) error = %v", errAssignGlobal)
	}

	errAssignScoped := conn.CreateUserRoleAssignment(
		"assign-scoped-any",
		"user-admin",
		"role-alert-auditor-any",
		"server-local-1",
		"user-owner",
	)
	if errAssignScoped != nil {
		t.Fatalf("CreateUserRoleAssignment(scoped) error = %v", errAssignScoped)
	}

	tests := []struct {
		name          string
		userID        string
		permissionIDs []string
		want          bool
	}{
		{
			name:          "matches one of multiple global permissions",
			userID:        "user-other",
			permissionIDs: []string{"alerts.view_history", "nodes.view"},
			want:          true,
		},
		{
			name:          "returns false when none match",
			userID:        "user-other",
			permissionIDs: []string{"nodes.view", "nodes.manage"},
			want:          false,
		},
		{
			name:          "server scoped permissions do not satisfy global check",
			userID:        "user-admin",
			permissionIDs: []string{"alerts.manage", "alerts.view_history"},
			want:          false,
		},
		{
			name:          "empty permission list returns false",
			userID:        "user-other",
			permissionIDs: nil,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errPerm := conn.UserHasAnyGlobalPermission(tt.userID, tt.permissionIDs)
			if errPerm != nil {
				t.Fatalf("UserHasAnyGlobalPermission() error = %v", errPerm)
			}
			if got != tt.want {
				t.Errorf("UserHasAnyGlobalPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}
