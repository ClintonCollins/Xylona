package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
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

func TestCountSuperUsers(t *testing.T) {
	tests := []struct {
		name      string
		setupSQL  []string
		wantCount int
	}{
		{
			name:      "zero super users",
			setupSQL:  nil,
			wantCount: 0,
		},
		{
			name: "one super user",
			setupSQL: []string{
				`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
				 values ('su-1', 'super1', 'su1@example.com', 'Super', 'One', 'hash', true, current_timestamp, current_timestamp, current_timestamp)`,
			},
			wantCount: 1,
		},
		{
			name: "multiple super users with regular users",
			setupSQL: []string{
				`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
				 values ('su-a', 'superA', 'a@example.com', 'A', 'User', 'hash', true, current_timestamp, current_timestamp, current_timestamp)`,
				`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
				 values ('su-b', 'superB', 'b@example.com', 'B', 'User', 'hash', true, current_timestamp, current_timestamp, current_timestamp)`,
				`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
				 values ('reg-c', 'regularC', 'c@example.com', 'C', 'User', 'hash', false, current_timestamp, current_timestamp, current_timestamp)`,
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newRBACMigratedConnection(t, "user-count-super-"+tt.name+".sqlite")

			for _, stmt := range tt.setupSQL {
				_, errExec := conn.SQLDb.ExecContext(context.Background(), stmt)
				if errExec != nil {
					t.Fatalf("setup SQL error = %v", errExec)
				}
			}

			count, errCount := conn.CountSuperUsers()
			if errCount != nil {
				t.Fatalf("CountSuperUsers() error = %v", errCount)
			}
			if count != tt.wantCount {
				t.Errorf("CountSuperUsers() = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestCreateUserAndGetUser(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-create.sqlite")

	now := time.Now().UTC()
	setter := &models.UserSetter{
		ID:           omit.From("user-new"),
		UserName:     omit.From("newuser"),
		Email:        omit.From("new@example.com"),
		FirstName:    omit.From("New"),
		LastName:     omit.From("User"),
		PasswordHash: omit.From("hash123"),
		SuperUser:    omit.From(false),
		LastLoginAt:  omit.From(now),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	}

	user, errCreate := conn.CreateUser(setter)
	if errCreate != nil {
		t.Fatalf("CreateUser() error = %v", errCreate)
	}
	if user.UserName != "newuser" {
		t.Errorf("CreateUser().UserName = %q, want %q", user.UserName, "newuser")
	}

	fetched, errGet := conn.GetUser("newuser")
	if errGet != nil {
		t.Fatalf("GetUser() error = %v", errGet)
	}
	if fetched.Email != "new@example.com" {
		t.Errorf("GetUser().Email = %q, want %q", fetched.Email, "new@example.com")
	}
}

func TestPruneExpiredUserSessions(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-session-prune.sqlite")
	seedRBACFixture(t, conn)

	expiredAt := "2026-04-10 11:30:00"
	validAt := "2026-04-10 13:30:00"

	_, errExpired := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user_session (id, user_id, token, created_at, updated_at, expires_at)
		 values (?, ?, ?, ?, ?, ?)`,
		"session-expired", "user-owner", "expired-token", "2026-04-10 11:00:00", "2026-04-10 11:00:00", expiredAt,
	)
	if errExpired != nil {
		t.Fatalf("insert expired session error = %v", errExpired)
	}

	_, errValid := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user_session (id, user_id, token, created_at, updated_at, expires_at)
		 values (?, ?, ?, ?, ?, ?)`,
		"session-valid", "user-owner", "valid-token", "2026-04-10 12:00:00", "2026-04-10 12:00:00", validAt,
	)
	if errValid != nil {
		t.Fatalf("insert valid session error = %v", errValid)
	}

	deleted, errPrune := conn.PruneExpiredUserSessions(time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	if errPrune != nil {
		t.Fatalf("PruneExpiredUserSessions() error = %v", errPrune)
	}
	if deleted != 1 {
		t.Fatalf("PruneExpiredUserSessions() deleted = %d, want 1", deleted)
	}

	_, errGetExpired := conn.GetUserSession("session-expired")
	if !errors.Is(errGetExpired, sql.ErrNoRows) {
		t.Fatalf("GetUserSession(expired) error = %v, want %v", errGetExpired, sql.ErrNoRows)
	}

	validSession, errGetValid := conn.GetUserSession("session-valid")
	if errGetValid != nil {
		t.Fatalf("GetUserSession(valid) error = %v", errGetValid)
	}
	if validSession.ID != "session-valid" {
		t.Fatalf("GetUserSession(valid).ID = %q, want %q", validSession.ID, "session-valid")
	}
}

func TestDeleteUserSessionsByUserID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-session-revoke.sqlite")
	seedRBACFixture(t, conn)

	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour)
	_, errOwnerSession := conn.CreateUserSession(&models.UserSessionSetter{
		ID:        omit.From("session-owner-1"),
		UserID:    omit.From("user-owner"),
		Token:     omit.From("token-owner-1"),
		CreatedAt: omit.From(now),
		UpdatedAt: omit.From(now),
		ExpiresAt: omit.From(expiresAt),
	})
	if errOwnerSession != nil {
		t.Fatalf("CreateUserSession(owner) error = %v", errOwnerSession)
	}
	_, errOwnerSessionTwo := conn.CreateUserSession(&models.UserSessionSetter{
		ID:        omit.From("session-owner-2"),
		UserID:    omit.From("user-owner"),
		Token:     omit.From("token-owner-2"),
		CreatedAt: omit.From(now),
		UpdatedAt: omit.From(now),
		ExpiresAt: omit.From(expiresAt),
	})
	if errOwnerSessionTwo != nil {
		t.Fatalf("CreateUserSession(owner 2) error = %v", errOwnerSessionTwo)
	}
	_, errOtherSession := conn.CreateUserSession(&models.UserSessionSetter{
		ID:        omit.From("session-other"),
		UserID:    omit.From("user-other"),
		Token:     omit.From("token-other"),
		CreatedAt: omit.From(now),
		UpdatedAt: omit.From(now),
		ExpiresAt: omit.From(expiresAt),
	})
	if errOtherSession != nil {
		t.Fatalf("CreateUserSession(other) error = %v", errOtherSession)
	}

	deleted, errDelete := conn.DeleteUserSessionsByUserID("user-owner")
	if errDelete != nil {
		t.Fatalf("DeleteUserSessionsByUserID() error = %v", errDelete)
	}
	if deleted != 2 {
		t.Fatalf("DeleteUserSessionsByUserID() deleted = %d, want 2", deleted)
	}

	_, errGetOwner := conn.GetUserSession("session-owner-1")
	if !errors.Is(errGetOwner, sql.ErrNoRows) {
		t.Fatalf("GetUserSession(owner) error = %v, want %v", errGetOwner, sql.ErrNoRows)
	}
	otherSession, errGetOther := conn.GetUserSession("session-other")
	if errGetOther != nil {
		t.Fatalf("GetUserSession(other) error = %v", errGetOther)
	}
	if otherSession.ID != "session-other" {
		t.Fatalf("GetUserSession(other).ID = %q, want session-other", otherSession.ID)
	}
}

func TestGetUserByIDNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-not-found.sqlite")

	_, errGet := conn.GetUserByID("nonexistent")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetUserByID() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetAllUsers(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-all.sqlite")
	seedRBACFixture(t, conn)

	users, errGet := conn.GetAllUsers()
	if errGet != nil {
		t.Fatalf("GetAllUsers() error = %v", errGet)
	}
	// seedRBACFixture inserts user-owner, user-other, user-admin.
	if len(users) < 3 {
		t.Errorf("GetAllUsers() len = %d, want >= 3", len(users))
	}
}

func TestUpdateUser(t *testing.T) {
	conn := newRBACMigratedConnection(t, "user-update.sqlite")
	seedRBACFixture(t, conn)

	now := time.Now().UTC()
	updateSetter := &models.UserSetter{
		ID:        omit.From("user-other"),
		FirstName: omit.From("Updated"),
		LastName:  omit.From("Name"),
		UpdatedAt: omit.From(now),
	}

	errUpdate := conn.UpdateUser(updateSetter)
	if errUpdate != nil {
		t.Fatalf("UpdateUser() error = %v", errUpdate)
	}

	fetched, errGet := conn.GetUserByID("user-other")
	if errGet != nil {
		t.Fatalf("GetUserByID() error = %v", errGet)
	}
	if fetched.FirstName != "Updated" {
		t.Errorf("GetUserByID().FirstName = %q, want %q", fetched.FirstName, "Updated")
	}
	if fetched.LastName != "Name" {
		t.Errorf("GetUserByID().LastName = %q, want %q", fetched.LastName, "Name")
	}
}
