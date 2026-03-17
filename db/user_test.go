package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestDeleteUser(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-delete.sqlite")
	seedRBACFixture(t, conn)

	errDeleteUser := conn.DeleteUser("user-other")
	if errDeleteUser != nil {
		t.Fatalf("DeleteUser() error = %v", errDeleteUser)
	}

	_, errGetUser := conn.GetUserByID("user-other")
	if !errors.Is(errGetUser, sql.ErrNoRows) {
		t.Errorf("GetUserByID() error = %v, want %v", errGetUser, sql.ErrNoRows)
	}
}

func TestDeleteUserMissingUser(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-delete-missing.sqlite")
	seedRBACFixture(t, conn)

	errDeleteUser := conn.DeleteUser("user-does-not-exist")
	if !errors.Is(errDeleteUser, sql.ErrNoRows) {
		t.Errorf("DeleteUser() error = %v, want %v", errDeleteUser, sql.ErrNoRows)
	}
}

func TestDeleteUserCascadesRoleAssignments(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-delete-cascade.sqlite")
	seedRBACFixture(t, conn)

	errCreateAssignment := conn.CreateUserRoleAssignment(
		"assignment-user-delete-cascade",
		"user-other",
		"viewer",
		"server-local-1",
		"user-owner",
	)
	if errCreateAssignment != nil {
		t.Fatalf("CreateUserRoleAssignment() error = %v", errCreateAssignment)
	}

	errDeleteUser := conn.DeleteUser("user-other")
	if errDeleteUser != nil {
		t.Fatalf("DeleteUser() error = %v", errDeleteUser)
	}

	_, errGetAssignment := conn.GetUserRoleAssignmentByID("assignment-user-delete-cascade")
	if !errors.Is(errGetAssignment, sql.ErrNoRows) {
		t.Errorf("GetUserRoleAssignmentByID() error = %v, want %v", errGetAssignment, sql.ErrNoRows)
	}
}
