package db

import (
	"database/sql"
	"errors"
	"slices"
	"testing"
)

func TestCreateRoleAndGetRoleByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "role-create.sqlite")

	errCreateRole := conn.CreateRole("custom-role-1", "Custom Role", "custom role")
	if errCreateRole != nil {
		t.Fatalf("CreateRole() error = %v", errCreateRole)
	}

	role, errGetRole := conn.GetRoleByID("custom-role-1")
	if errGetRole != nil {
		t.Fatalf("GetRoleByID() error = %v", errGetRole)
	}

	if role.Name != "Custom Role" {
		t.Errorf("GetRoleByID().Name = %q, want %q", role.Name, "Custom Role")
	}
	if role.IsSystem {
		t.Errorf("GetRoleByID().IsSystem = %v, want false", role.IsSystem)
	}
}

func TestGetAllRolesIncludesSystemRoles(t *testing.T) {
	conn := newRBACMigratedConnection(t, "role-list.sqlite")

	roles, errGetRoles := conn.GetAllRoles()
	if errGetRoles != nil {
		t.Fatalf("GetAllRoles() error = %v", errGetRoles)
	}

	roleIDs := make([]string, 0, len(roles))
	for _, role := range roles {
		roleIDs = append(roleIDs, role.ID)
	}

	for _, expectedRoleID := range []string{"viewer", "operator", "admin"} {
		if !slices.Contains(roleIDs, expectedRoleID) {
			t.Errorf("GetAllRoles() missing seeded role %q", expectedRoleID)
		}
	}
}

func TestCreateRoleDuplicateName(t *testing.T) {
	conn := newRBACMigratedConnection(t, "role-duplicate.sqlite")

	errCreateFirst := conn.CreateRole("custom-role-1", "Duplicate Name", "first")
	if errCreateFirst != nil {
		t.Fatalf("CreateRole(first) error = %v", errCreateFirst)
	}

	errCreateSecond := conn.CreateRole("custom-role-2", "Duplicate Name", "second")
	if errCreateSecond == nil {
		t.Fatalf("CreateRole(second) expected unique constraint error, got nil")
	}
}

func TestDeleteRole(t *testing.T) {
	conn := newRBACMigratedConnection(t, "role-delete.sqlite")

	errCreateRole := conn.CreateRole("custom-role-delete", "Delete Me", "custom")
	if errCreateRole != nil {
		t.Fatalf("CreateRole() error = %v", errCreateRole)
	}

	errDeleteRole := conn.DeleteRole("custom-role-delete")
	if errDeleteRole != nil {
		t.Fatalf("DeleteRole() error = %v", errDeleteRole)
	}

	_, errGetRole := conn.GetRoleByID("custom-role-delete")
	if !errors.Is(errGetRole, sql.ErrNoRows) {
		t.Errorf("GetRoleByID() error = %v, want %v", errGetRole, sql.ErrNoRows)
	}
}

func TestDeleteRoleSystemRole(t *testing.T) {
	conn := newRBACMigratedConnection(t, "role-delete-system.sqlite")

	errDeleteRole := conn.DeleteRole("viewer")
	if !errors.Is(errDeleteRole, ErrRoleIsSystem) {
		t.Errorf("DeleteRole() error = %v, want %v", errDeleteRole, ErrRoleIsSystem)
	}
}

func TestGetPermissionsForRole(t *testing.T) {
	conn := newRBACMigratedConnection(t, "role-permissions.sqlite")

	permissionIDs, errGetPermissions := conn.GetPermissionsForRole("operator")
	if errGetPermissions != nil {
		t.Fatalf("GetPermissionsForRole() error = %v", errGetPermissions)
	}

	for _, expectedPermissionID := range []string{
		"game_server.view",
		"game_server.start",
		"game_server.stop",
		"game_server.restart",
		"game_server.console",
	} {
		if !slices.Contains(permissionIDs, expectedPermissionID) {
			t.Errorf("GetPermissionsForRole() missing permission %q", expectedPermissionID)
		}
	}
}

func TestSetRolePermissionsReplacesAndDeduplicates(t *testing.T) {
	conn := newRBACMigratedConnection(t, "role-set-permissions.sqlite")

	errCreateRole := conn.CreateRole("custom-role-set", "Set Perms", "custom")
	if errCreateRole != nil {
		t.Fatalf("CreateRole() error = %v", errCreateRole)
	}

	errSetPermissions := conn.SetRolePermissions("custom-role-set", []string{
		"game_server.view",
		"game_server.start",
		"game_server.start",
	})
	if errSetPermissions != nil {
		t.Fatalf("SetRolePermissions() error = %v", errSetPermissions)
	}

	permissionIDs, errGetPermissions := conn.GetPermissionsForRole("custom-role-set")
	if errGetPermissions != nil {
		t.Fatalf("GetPermissionsForRole() error = %v", errGetPermissions)
	}

	if len(permissionIDs) != 2 {
		t.Fatalf("GetPermissionsForRole() len = %d, want 2", len(permissionIDs))
	}
	if !slices.Contains(permissionIDs, "game_server.view") {
		t.Errorf("GetPermissionsForRole() missing permission %q", "game_server.view")
	}
	if !slices.Contains(permissionIDs, "game_server.start") {
		t.Errorf("GetPermissionsForRole() missing permission %q", "game_server.start")
	}

	errReplacePermissions := conn.SetRolePermissions("custom-role-set", []string{"game_server.stop"})
	if errReplacePermissions != nil {
		t.Fatalf("SetRolePermissions(replace) error = %v", errReplacePermissions)
	}

	replacedPermissionIDs, errGetReplaced := conn.GetPermissionsForRole("custom-role-set")
	if errGetReplaced != nil {
		t.Fatalf("GetPermissionsForRole(replaced) error = %v", errGetReplaced)
	}

	if len(replacedPermissionIDs) != 1 || replacedPermissionIDs[0] != "game_server.stop" {
		t.Errorf("GetPermissionsForRole(replaced) = %v, want [game_server.stop]", replacedPermissionIDs)
	}
}

func TestSetRolePermissionsInvalidPermission(t *testing.T) {
	conn := newRBACMigratedConnection(t, "role-invalid-permission.sqlite")

	errCreateRole := conn.CreateRole("custom-role-invalid", "Invalid Perms", "custom")
	if errCreateRole != nil {
		t.Fatalf("CreateRole() error = %v", errCreateRole)
	}

	errSetPermissions := conn.SetRolePermissions("custom-role-invalid", []string{"permission.does.not.exist"})
	if errSetPermissions == nil {
		t.Fatalf("SetRolePermissions() expected foreign key error, got nil")
	}
}
