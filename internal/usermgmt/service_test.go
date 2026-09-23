package usermgmt

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/pkg/passwordhash"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// These tests pin down the shared user-management seam described in
// docs/specs/2026-04-11-cli-03-user-management-commands.md. They assume the
// implementation introduces an internal/usermgmt Service with shared mutation rules
// that can be reused by RPC, local admin IPC, and offline CLI execution.

func TestCreateUserRejectsDuplicateUsername(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, `usermgmt-duplicate.sqlite`)
	_ = seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        `user-existing`,
		userName:  `duplicate-user`,
		email:     `duplicate@example.com`,
		firstName: `Dup`,
		lastName:  `Existing`,
		password:  `original-password`,
		superUser: true,
	})

	service := NewService(conn)

	_, errCreateUser := service.Create(CreateInput{
		UserName:  `duplicate-user`,
		Email:     `second@example.com`,
		FirstName: `Second`,
		LastName:  `User`,
		Password:  `password-123`,
		SuperUser: false,
	})
	if errCreateUser == nil {
		t.Fatal(`CreateUser() error = nil, want duplicate-username error`)
	}
	if !errors.Is(errCreateUser, ErrDuplicateUsername) {
		t.Fatalf(`CreateUser() error = %v, want %v`, errCreateUser, ErrDuplicateUsername)
	}

	errText := strings.ToLower(errCreateUser.Error())
	if !strings.Contains(errText, `already exists`) {
		t.Fatalf(`CreateUser() error = %q, want message mentioning existing username`, errCreateUser.Error())
	}
}

func TestCreateUserHashesPasswordWithArgon2id(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, `usermgmt-create-password.sqlite`)
	service := NewService(conn)

	createdUser, errCreateUser := service.Create(CreateInput{
		UserName:  `create-password`,
		Email:     `create-password@example.com`,
		FirstName: `Create`,
		LastName:  `Password`,
		Password:  `new-password-123`,
		SuperUser: false,
	})
	if errCreateUser != nil {
		t.Fatalf(`Create() error = %v`, errCreateUser)
	}

	storedUser, errGetUser := conn.GetUserByID(createdUser.ID)
	if errGetUser != nil {
		t.Fatalf(`GetUserByID() error = %v`, errGetUser)
	}

	match, errVerify := passwordhash.Verify(storedUser.PasswordHash, `new-password-123`)
	if errVerify != nil {
		t.Fatalf(`Verify() error = %v`, errVerify)
	}
	if !match {
		t.Fatal(`Verify() = false, want true`)
	}
}

func TestUpdateOrDeleteLastSuperUserIsPrevented(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action func(t *testing.T, service *Service, adminUser *models.User) error
		verify func(t *testing.T, conn *db.Connection, adminUser *models.User)
	}{
		{
			name: `delete`,
			action: func(_ *testing.T, service *Service, adminUser *models.User) error {
				return service.Delete(DeleteInput{
					ID: adminUser.ID,
				})
			},
			verify: func(t *testing.T, conn *db.Connection, adminUser *models.User) {
				t.Helper()

				stillPresent, errGetUser := conn.GetUserByID(adminUser.ID)
				if errGetUser != nil {
					t.Fatalf(`GetUserByID() error = %v`, errGetUser)
				}
				if stillPresent.ID != adminUser.ID {
					t.Fatalf(`GetUserByID().ID = %q, want %q`, stillPresent.ID, adminUser.ID)
				}
			},
		},
		{
			name: `demote`,
			action: func(_ *testing.T, service *Service, adminUser *models.User) error {
				demote := false
				return mustNoUserResult(service.Update(UpdateInput{
					ID:        adminUser.ID,
					SuperUser: &demote,
				}))
			},
			verify: func(t *testing.T, conn *db.Connection, adminUser *models.User) {
				t.Helper()

				stillPresent, errGetUser := conn.GetUserByID(adminUser.ID)
				if errGetUser != nil {
					t.Fatalf(`GetUserByID() error = %v`, errGetUser)
				}
				if !stillPresent.SuperUser {
					t.Fatal(`last superuser was demoted unexpectedly`)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newUserMgmtTestConnection(t, `usermgmt-last-superuser-`+tt.name+`.sqlite`)
			adminUser := seedUserMgmtUser(t, conn, userMgmtSeedUser{
				id:        `user-admin`,
				userName:  `admin`,
				email:     `admin@example.com`,
				firstName: `Admin`,
				lastName:  `User`,
				password:  `password-123`,
				superUser: true,
			})
			_ = seedUserMgmtUser(t, conn, userMgmtSeedUser{
				id:        `user-regular`,
				userName:  `regular`,
				email:     `regular@example.com`,
				firstName: `Regular`,
				lastName:  `User`,
				password:  `password-123`,
				superUser: false,
			})

			service := NewService(conn)

			errAction := tt.action(t, service, adminUser)
			if errAction == nil {
				t.Fatal(`mutation error = nil, want last-superuser protection`)
			}
			if !errors.Is(errAction, ErrLastSuperUser) {
				t.Fatalf(`mutation error = %v, want %v`, errAction, ErrLastSuperUser)
			}

			errText := strings.ToLower(errAction.Error())
			if !strings.Contains(errText, `last super`) {
				t.Fatalf(`mutation error = %q, want message mentioning last superuser`, errAction.Error())
			}

			tt.verify(t, conn, adminUser)
		})
	}
}

func TestUpdateUserWithoutPasswordKeepsPasswordHash(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, `usermgmt-keep-password.sqlite`)
	createdUser := seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        `user-keep-password`,
		userName:  `keep-password`,
		email:     `keep-password@example.com`,
		firstName: `Keep`,
		lastName:  `Password`,
		password:  `old-password-123`,
		superUser: false,
	})
	initialPasswordHash := createdUser.PasswordHash

	service := NewService(conn)

	firstName := `Updated`
	lastName := `Name`
	errUpdateUser := mustNoUserResult(service.Update(UpdateInput{
		ID:        createdUser.ID,
		FirstName: &firstName,
		LastName:  &lastName,
	}))
	if errUpdateUser != nil {
		t.Fatalf(`UpdateUser() error = %v`, errUpdateUser)
	}

	updatedUser, errGetUser := conn.GetUserByID(createdUser.ID)
	if errGetUser != nil {
		t.Fatalf(`GetUserByID() error = %v`, errGetUser)
	}
	if updatedUser.PasswordHash != initialPasswordHash {
		t.Fatal(`password hash changed even though password was left unspecified`)
	}
	if updatedUser.FirstName != `Updated` {
		t.Fatalf(`GetUserByID().FirstName = %q, want %q`, updatedUser.FirstName, `Updated`)
	}
}

func TestUpdateUserWithPasswordRehashesPassword(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, `usermgmt-change-password.sqlite`)
	createdUser := seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        `user-change-password`,
		userName:  `change-password`,
		email:     `change-password@example.com`,
		firstName: `Change`,
		lastName:  `Password`,
		password:  `old-password-123`,
		superUser: false,
	})
	initialPasswordHash := createdUser.PasswordHash

	service := NewService(conn)

	password := `new-password-123`
	errUpdateUser := mustNoUserResult(service.Update(UpdateInput{
		ID:       createdUser.ID,
		Password: &password,
	}))
	if errUpdateUser != nil {
		t.Fatalf(`UpdateUser() error = %v`, errUpdateUser)
	}

	updatedUser, errGetUser := conn.GetUserByID(createdUser.ID)
	if errGetUser != nil {
		t.Fatalf(`GetUserByID() error = %v`, errGetUser)
	}
	if updatedUser.PasswordHash == initialPasswordHash {
		t.Fatal(`password hash did not change after password update`)
	}

	match, errVerify := passwordhash.Verify(updatedUser.PasswordHash, `new-password-123`)
	if errVerify != nil {
		t.Fatalf(`Verify() error = %v`, errVerify)
	}
	if !match {
		t.Fatal(`Verify() = false, want true`)
	}
}

func TestDeleteUserAllowsRemovingSuperUserWhenAnotherSuperUserExists(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, `usermgmt-delete-superuser.sqlite`)
	adminUser := seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        `user-admin`,
		userName:  `admin`,
		email:     `admin@example.com`,
		firstName: `Admin`,
		lastName:  `User`,
		password:  `password-123`,
		superUser: true,
	})
	_ = seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        `user-admin-2`,
		userName:  `admin-2`,
		email:     `admin-2@example.com`,
		firstName: `Second`,
		lastName:  `Admin`,
		password:  `password-123`,
		superUser: true,
	})

	service := NewService(conn)
	revokedUserID := ""
	service.SetSessionsRevokedHandler(func(userID string, _ string) {
		revokedUserID = userID
	})

	errDeleteUser := service.Delete(DeleteInput{
		ID: adminUser.ID,
	})
	if errDeleteUser != nil {
		t.Fatalf(`DeleteUser() error = %v`, errDeleteUser)
	}

	_, errGetUser := conn.GetUserByID(adminUser.ID)
	if !errors.Is(errGetUser, sql.ErrNoRows) {
		t.Fatalf(`GetUserByID() error = %v, want deleted user to be gone`, errGetUser)
	}
	if revokedUserID != adminUser.ID {
		t.Fatalf("revoked user ID = %q, want %q", revokedUserID, adminUser.ID)
	}
}

func TestDeleteUserNamesBlockersAndDeletesSchedules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		userID       string
		actingUserID string
		wantErr      error
		wantMessage  string
	}{
		{
			name:        "owner must transfer their game servers first",
			userID:      `user-owner`,
			wantErr:     ErrUserOwnsGameServers,
			wantMessage: `transfer ownership of "Blocked Server" first`,
		},
		{
			name:        "grantor without an acting admin must remove the access they gave first",
			userID:      `user-grantor`,
			wantErr:     ErrUserGaveAccess,
			wantMessage: `remove the access they granted on "Blocked Server"`,
		},
		{
			name:         "grantor deleted by an admin hands the access they gave to the admin",
			userID:       `user-grantor`,
			actingUserID: `user-admin`,
		},
		{
			name:   "schedule creator is deleted with their schedules",
			userID: `user-scheduler`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := newUserMgmtTestConnection(t, `usermgmt-delete-blockers.sqlite`)
			scheduleID := seedUserDeletionFixture(t, conn)
			service := NewService(conn)

			errDelete := service.Delete(DeleteInput{ID: tt.userID, ActingUserID: tt.actingUserID})
			if tt.wantErr != nil {
				if !errors.Is(errDelete, tt.wantErr) {
					t.Fatalf(`Delete() error = %v, want %v`, errDelete, tt.wantErr)
				}
				if !strings.Contains(errDelete.Error(), tt.wantMessage) {
					t.Fatalf(`Delete() error = %q, want it to contain %q`, errDelete.Error(), tt.wantMessage)
				}
				_, errGetUser := conn.GetUserByID(tt.userID)
				if errGetUser != nil {
					t.Fatalf(`GetUserByID() error = %v, want blocked user kept`, errGetUser)
				}
				return
			}

			if errDelete != nil {
				t.Fatalf(`Delete() error = %v`, errDelete)
			}
			_, errGetTask := conn.GetScheduledTaskByID(scheduleID)
			if deleted := errors.Is(errGetTask, sql.ErrNoRows); deleted != (tt.userID == `user-scheduler`) {
				t.Fatalf(`GetScheduledTaskByID() error = %v, want schedule deleted only with its creator`, errGetTask)
			}
			if tt.actingUserID != `` {
				grant, errGrant := conn.GetUserRoleAssignmentByID(`grant-blocked`)
				if errGrant != nil {
					t.Fatalf(`GetUserRoleAssignmentByID() error = %v, want the grant kept`, errGrant)
				}
				if grant.GrantedBy != tt.actingUserID {
					t.Fatalf(`GrantedBy = %q, want %q`, grant.GrantedBy, tt.actingUserID)
				}
			}
		})
	}
}

// seedUserDeletionFixture creates a game server owned by user-owner, access to
// it that user-grantor gave user-scheduler, a schedule user-scheduler created,
// and user-admin with nothing attached. It returns the schedule ID.
func seedUserDeletionFixture(t *testing.T, conn *db.Connection) string {
	t.Helper()

	exec := func(query string, args ...any) {
		t.Helper()
		_, errExec := conn.SQLDb.ExecContext(t.Context(), query, args...)
		if errExec != nil {
			t.Fatalf(`seed %q: %v`, query, errExec)
		}
	}

	now := time.Now().UTC()
	for _, id := range []string{`user-owner`, `user-grantor`, `user-scheduler`, `user-admin`} {
		exec(`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
			values (?, ?, ?, '', '', 'hash', 0, ?, ?, ?)`, id, id, id+`@example.com`, now, now, now)
	}
	exec(`insert into node (id, name, listen_url, enabled) values ('node-delete', 'Node', '', 1)`)
	exec(`insert into ip (address, usable, external, node_id) values ('127.0.0.1', 1, 0, 'node-delete')`)
	exec(`insert into game (id, name, default_port, default_query_port, default_max_players, windows_support)
		values ('usermgmt-game', 'Game', 27015, 27015, 10, 1)`)
	exec(`insert into game_server
		(id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		values ('server-blocked', 'user-owner', 'Blocked Server', 'usermgmt-game', 'OFFLINE', 10, 10, '', '127.0.0.1', 27015, 27015, '/tmp/blocked', 'node-delete', '[]')`)

	errGrant := conn.CreateUserRoleAssignment(`grant-blocked`, `user-scheduler`, `viewer`, `server-blocked`, `user-grantor`)
	if errGrant != nil {
		t.Fatalf(`CreateUserRoleAssignment() error = %v`, errGrant)
	}
	task, errTask := conn.InsertScheduledTask(`server-blocked`, `user-scheduler`, `Nightly`, `backup`, `0 3 * * *`, `UTC`, ``, true)
	if errTask != nil {
		t.Fatalf(`InsertScheduledTask() error = %v`, errTask)
	}
	return task.ID
}

func TestUpdateUserAllowsDemotingSuperUserWhenAnotherSuperUserExists(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, `usermgmt-demote-superuser.sqlite`)
	adminUser := seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        `user-admin`,
		userName:  `admin`,
		email:     `admin@example.com`,
		firstName: `Admin`,
		lastName:  `User`,
		password:  `password-123`,
		superUser: true,
	})
	_ = seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        `user-admin-2`,
		userName:  `admin-2`,
		email:     `admin-2@example.com`,
		firstName: `Second`,
		lastName:  `Admin`,
		password:  `password-123`,
		superUser: true,
	})

	service := NewService(conn)

	demote := false
	errUpdateUser := mustNoUserResult(service.Update(UpdateInput{
		ID:        adminUser.ID,
		SuperUser: &demote,
	}))
	if errUpdateUser != nil {
		t.Fatalf(`UpdateUser() error = %v`, errUpdateUser)
	}

	updatedUser, errGetUser := conn.GetUserByID(adminUser.ID)
	if errGetUser != nil {
		t.Fatalf(`GetUserByID() error = %v`, errGetUser)
	}
	if updatedUser.SuperUser {
		t.Fatal(`GetUserByID().SuperUser = true, want false`)
	}
}

func TestUpdateUserPasswordRevokesSessions(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, "usermgmt-revoke-password.sqlite")
	createdUser := seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        "user-revoke-password",
		userName:  "revoke-password",
		email:     "revoke-password@example.com",
		firstName: "Revoke",
		lastName:  "Password",
		password:  "old-password-123",
		superUser: false,
	})
	createUserMgmtSession(t, conn, "session-revoke-password", createdUser.ID)

	service := NewService(conn)
	revokedUserID := ""
	service.SetSessionsRevokedHandler(func(userID string, _ string) {
		revokedUserID = userID
	})
	password := "new-password-123"
	errUpdateUser := mustNoUserResult(service.Update(UpdateInput{
		ID:       createdUser.ID,
		Password: &password,
	}))
	if errUpdateUser != nil {
		t.Fatalf("Update() error = %v", errUpdateUser)
	}

	_, errGetSession := conn.GetUserSession("session-revoke-password")
	if !errors.Is(errGetSession, sql.ErrNoRows) {
		t.Fatalf("GetUserSession() error = %v, want revoked session", errGetSession)
	}
	if revokedUserID != createdUser.ID {
		t.Fatalf("revoked user ID = %q, want %q", revokedUserID, createdUser.ID)
	}
}

func TestUpdateUserPasswordKeepsRequestedSession(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, "usermgmt-keep-session.sqlite")
	createdUser := seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        "user-keep-session",
		userName:  "keep-session",
		email:     "keep-session@example.com",
		firstName: "Keep",
		lastName:  "Session",
		password:  "old-password-123",
		superUser: false,
	})
	createUserMgmtSession(t, conn, "session-current", createdUser.ID)
	createUserMgmtSession(t, conn, "session-other-device", createdUser.ID)

	service := NewService(conn)
	keptSessionID := ""
	service.SetSessionsRevokedHandler(func(_ string, keepSessionID string) {
		keptSessionID = keepSessionID
	})
	password := "new-password-123"
	errUpdateUser := mustNoUserResult(service.Update(UpdateInput{
		ID:            createdUser.ID,
		Password:      &password,
		KeepSessionID: "session-current",
	}))
	if errUpdateUser != nil {
		t.Fatalf("Update() error = %v", errUpdateUser)
	}

	_, errGetCurrent := conn.GetUserSession("session-current")
	if errGetCurrent != nil {
		t.Fatalf("GetUserSession(current) error = %v, want kept session", errGetCurrent)
	}
	_, errGetOther := conn.GetUserSession("session-other-device")
	if !errors.Is(errGetOther, sql.ErrNoRows) {
		t.Fatalf("GetUserSession(other) error = %v, want revoked session", errGetOther)
	}
	if keptSessionID != "session-current" {
		t.Fatalf("handler keepSessionID = %q, want %q", keptSessionID, "session-current")
	}
}

func TestUpdateUserDemotionRevokesSessions(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, "usermgmt-revoke-demote.sqlite")
	adminUser := seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        "user-admin-revoke",
		userName:  "admin-revoke",
		email:     "admin-revoke@example.com",
		firstName: "Admin",
		lastName:  "Revoke",
		password:  "password-123",
		superUser: true,
	})
	_ = seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        "user-admin-keep",
		userName:  "admin-keep",
		email:     "admin-keep@example.com",
		firstName: "Admin",
		lastName:  "Keep",
		password:  "password-123",
		superUser: true,
	})
	createUserMgmtSession(t, conn, "session-admin-revoke", adminUser.ID)

	service := NewService(conn)
	revokedUserID := ""
	service.SetSessionsRevokedHandler(func(userID string, _ string) {
		revokedUserID = userID
	})
	demote := false
	errUpdateUser := mustNoUserResult(service.Update(UpdateInput{
		ID:        adminUser.ID,
		SuperUser: &demote,
	}))
	if errUpdateUser != nil {
		t.Fatalf("Update() error = %v", errUpdateUser)
	}

	_, errGetSession := conn.GetUserSession("session-admin-revoke")
	if !errors.Is(errGetSession, sql.ErrNoRows) {
		t.Fatalf("GetUserSession() error = %v, want revoked session", errGetSession)
	}
	if revokedUserID != adminUser.ID {
		t.Fatalf("revoked user ID = %q, want %q", revokedUserID, adminUser.ID)
	}
}

func TestUpdateUserNameKeepsSessions(t *testing.T) {
	t.Parallel()

	conn := newUserMgmtTestConnection(t, "usermgmt-keep-sessions.sqlite")
	createdUser := seedUserMgmtUser(t, conn, userMgmtSeedUser{
		id:        "user-keep-sessions",
		userName:  "keep-sessions",
		email:     "keep-sessions@example.com",
		firstName: "Keep",
		lastName:  "Sessions",
		password:  "password-123",
		superUser: false,
	})
	createUserMgmtSession(t, conn, "session-keep", createdUser.ID)

	service := NewService(conn)
	revokedUserID := ""
	service.SetSessionsRevokedHandler(func(userID string, _ string) {
		revokedUserID = userID
	})
	firstName := "Updated"
	errUpdateUser := mustNoUserResult(service.Update(UpdateInput{
		ID:        createdUser.ID,
		FirstName: &firstName,
	}))
	if errUpdateUser != nil {
		t.Fatalf("Update() error = %v", errUpdateUser)
	}

	session, errGetSession := conn.GetUserSession("session-keep")
	if errGetSession != nil {
		t.Fatalf("GetUserSession() error = %v, want session to remain", errGetSession)
	}
	if session.ID != "session-keep" {
		t.Fatalf("GetUserSession().ID = %q, want session-keep", session.ID)
	}
	if revokedUserID != "" {
		t.Fatalf("revoked user ID = %q, want no revocation", revokedUserID)
	}
}

func createUserMgmtSession(t *testing.T, conn *db.Connection, sessionID string, userID string) {
	t.Helper()

	now := time.Now().UTC()
	_, errCreate := conn.CreateUserSession(&models.UserSessionSetter{
		ID:        omit.From(sessionID),
		UserID:    omit.From(userID),
		Token:     omit.From(sessionID + "-token"),
		CreatedAt: omit.From(now),
		UpdatedAt: omit.From(now),
		ExpiresAt: omit.From(now.Add(24 * time.Hour)),
	})
	if errCreate != nil {
		t.Fatalf("CreateUserSession() error = %v", errCreate)
	}
}

type userMgmtSeedUser struct {
	id        string
	userName  string
	email     string
	firstName string
	lastName  string
	password  string
	superUser bool
}

func newUserMgmtTestConnection(t *testing.T, sqliteFileName string) *db.Connection {
	t.Helper()

	return dbtest.NewMigratedConnection(t, sqliteFileName)
}

func seedUserMgmtUser(t *testing.T, conn *db.Connection, user userMgmtSeedUser) *models.User {
	t.Helper()

	passwordHash, errHashPassword := passwordhash.Hash(user.password)
	if errHashPassword != nil {
		t.Fatalf(`Hash() error = %v`, errHashPassword)
	}

	now := time.Date(2026, time.April, 11, 12, 0, 0, 0, time.UTC)
	createdUser, errCreateUser := conn.CreateUser(&models.UserSetter{
		ID:           omit.From(user.id),
		UserName:     omit.From(user.userName),
		Email:        omit.From(user.email),
		FirstName:    omit.From(user.firstName),
		LastName:     omit.From(user.lastName),
		PasswordHash: omit.From(passwordHash),
		SuperUser:    omit.From(user.superUser),
		LastLoginAt:  omit.From(now),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	})
	if errCreateUser != nil {
		t.Fatalf(`CreateUser() error = %v`, errCreateUser)
	}

	return createdUser
}

func mustNoUserResult(_ *User, err error) error {
	return err
}
