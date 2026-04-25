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
