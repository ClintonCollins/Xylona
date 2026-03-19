package rpc

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"golang.org/x/crypto/bcrypt"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestUserManagementRequiresSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	tests := []struct {
		name string
		call func(t *testing.T, userID string) error
	}{
		{
			name: "create user",
			call: func(t *testing.T, userID string) error {
				request := connect.NewRequest(&xylona.CreateUserRequest{
					UserName:  "authz-create",
					Email:     "authz-create@example.com",
					FirstName: "Authz",
					LastName:  "Create",
					Password:  "password123",
				})
				if userID != "" {
					addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, userID)
				}

				_, errCreateUser := fixture.service.CreateUser(context.Background(), request)
				return errCreateUser
			},
		},
		{
			name: "list users",
			call: func(t *testing.T, userID string) error {
				request := connect.NewRequest(&xylona.ListUsersRequest{})
				if userID != "" {
					addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, userID)
				}

				_, errListUsers := fixture.service.ListUsers(context.Background(), request)
				return errListUsers
			},
		},
		{
			name: "get user",
			call: func(t *testing.T, userID string) error {
				request := connect.NewRequest(&xylona.GetUserDetailsRequest{
					Id: "user-owner",
				})
				if userID != "" {
					addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, userID)
				}

				_, errGetUser := fixture.service.GetUser(context.Background(), request)
				return errGetUser
			},
		},
		{
			name: "update user",
			call: func(t *testing.T, userID string) error {
				request := connect.NewRequest(&xylona.UpdateUserRequest{
					Id:        "user-owner",
					UserName:  "owner",
					Email:     "owner@example.com",
					FirstName: "Owner",
					LastName:  "User",
					SuperUser: false,
				})
				if userID != "" {
					addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, userID)
				}

				_, errUpdateUser := fixture.service.UpdateUser(context.Background(), request)
				return errUpdateUser
			},
		},
		{
			name: "delete user",
			call: func(t *testing.T, userID string) error {
				request := connect.NewRequest(&xylona.DeleteUserRequest{
					Id: "user-owner",
				})
				if userID != "" {
					addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, userID)
				}

				_, errDeleteUser := fixture.service.DeleteUser(context.Background(), request)
				return errDeleteUser
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/unauthenticated", func(t *testing.T) {
			errCall := tt.call(t, "")
			if errCall == nil {
				t.Fatalf("expected unauthenticated error, got nil")
			}
			if connect.CodeOf(errCall) != connect.CodeUnauthenticated {
				t.Errorf("code = %v, want %v", connect.CodeOf(errCall), connect.CodeUnauthenticated)
			}
		})

		t.Run(tt.name+"/non-super-user", func(t *testing.T) {
			errCall := tt.call(t, "user-owner")
			if errCall == nil {
				t.Fatalf("expected permission denied error, got nil")
			}
			if connect.CodeOf(errCall) != connect.CodePermissionDenied {
				t.Errorf("code = %v, want %v", connect.CodeOf(errCall), connect.CodePermissionDenied)
			}
		})
	}
}

func TestCreateListGetUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	createRequest := connect.NewRequest(&xylona.CreateUserRequest{
		UserName:  "user-crud",
		Email:     "user-crud@example.com",
		FirstName: "Crud",
		LastName:  "User",
		Password:  "password123",
		SuperUser: false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createRequest, "user-admin")

	createResponse, errCreateUser := fixture.service.CreateUser(context.Background(), createRequest)
	if errCreateUser != nil {
		t.Fatalf("CreateUser() error = %v", errCreateUser)
	}
	if createResponse.Msg == nil || createResponse.Msg.User == nil {
		t.Fatalf("CreateUser() returned empty response")
	}

	createdUserID := createResponse.Msg.User.Id

	listRequest := connect.NewRequest(&xylona.ListUsersRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listRequest, "user-admin")

	listResponse, errListUsers := fixture.service.ListUsers(context.Background(), listRequest)
	if errListUsers != nil {
		t.Fatalf("ListUsers() error = %v", errListUsers)
	}
	if len(listResponse.Msg.Users) < 1 {
		t.Fatalf("ListUsers() returned no users")
	}

	foundCreatedUser := false
	for _, user := range listResponse.Msg.Users {
		if user.Id == createdUserID {
			foundCreatedUser = true
			break
		}
	}
	if !foundCreatedUser {
		t.Fatalf("ListUsers() did not include created user %q", createdUserID)
	}

	getRequest := connect.NewRequest(&xylona.GetUserDetailsRequest{
		Id: createdUserID,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getRequest, "user-admin")

	getResponse, errGetUser := fixture.service.GetUser(context.Background(), getRequest)
	if errGetUser != nil {
		t.Fatalf("GetUser() error = %v", errGetUser)
	}
	if getResponse.Msg == nil || getResponse.Msg.User == nil {
		t.Fatalf("GetUser() returned empty response")
	}
	if getResponse.Msg.User.UserName != "user-crud" {
		t.Errorf("GetUser().UserName = %q, want %q", getResponse.Msg.User.UserName, "user-crud")
	}
}

func TestUpdateUserWithoutPasswordKeepsPasswordHash(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	createdUser := createUserForRPCUserTests(t, fixture, "user-update-no-pass", false)
	createdModel, errGetCreatedUser := fixture.conn.GetUserByID(createdUser.Id)
	if errGetCreatedUser != nil {
		t.Fatalf("GetUserByID(created) error = %v", errGetCreatedUser)
	}
	initialPasswordHash := createdModel.PasswordHash

	updateRequest := connect.NewRequest(&xylona.UpdateUserRequest{
		Id:        createdUser.Id,
		UserName:  "user-update-no-pass",
		Email:     "user-update-no-pass@example.com",
		FirstName: "Updated",
		LastName:  "Name",
		SuperUser: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateRequest, "user-admin")

	updateResponse, errUpdateUser := fixture.service.UpdateUser(context.Background(), updateRequest)
	if errUpdateUser != nil {
		t.Fatalf("UpdateUser() error = %v", errUpdateUser)
	}
	if updateResponse.Msg == nil || updateResponse.Msg.User == nil {
		t.Fatalf("UpdateUser() returned empty response")
	}
	if !updateResponse.Msg.User.SuperUser {
		t.Errorf("UpdateUser().User.SuperUser = %v, want %v", updateResponse.Msg.User.SuperUser, true)
	}

	updatedModel, errGetUpdatedUser := fixture.conn.GetUserByID(createdUser.Id)
	if errGetUpdatedUser != nil {
		t.Fatalf("GetUserByID(updated) error = %v", errGetUpdatedUser)
	}
	if updatedModel.PasswordHash != initialPasswordHash {
		t.Errorf("password hash changed unexpectedly")
	}
}

func TestUpdateUserWithPasswordRehashesPassword(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	createdUser := createUserForRPCUserTests(t, fixture, "user-update-pass", false)
	createdModel, errGetCreatedUser := fixture.conn.GetUserByID(createdUser.Id)
	if errGetCreatedUser != nil {
		t.Fatalf("GetUserByID(created) error = %v", errGetCreatedUser)
	}
	initialPasswordHash := createdModel.PasswordHash

	updateRequest := connect.NewRequest(&xylona.UpdateUserRequest{
		Id:        createdUser.Id,
		UserName:  "user-update-pass",
		Email:     "user-update-pass@example.com",
		FirstName: "Updated",
		LastName:  "Password",
		SuperUser: false,
		Password:  "new-password-123",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateRequest, "user-admin")

	_, errUpdateUser := fixture.service.UpdateUser(context.Background(), updateRequest)
	if errUpdateUser != nil {
		t.Fatalf("UpdateUser() error = %v", errUpdateUser)
	}

	updatedModel, errGetUpdatedUser := fixture.conn.GetUserByID(createdUser.Id)
	if errGetUpdatedUser != nil {
		t.Fatalf("GetUserByID(updated) error = %v", errGetUpdatedUser)
	}
	if updatedModel.PasswordHash == initialPasswordHash {
		t.Fatalf("password hash did not change")
	}

	errComparePassword := bcrypt.CompareHashAndPassword([]byte(updatedModel.PasswordHash), []byte("new-password-123"))
	if errComparePassword != nil {
		t.Errorf("updated password hash does not match password: %v", errComparePassword)
	}
}

func TestUpdateUserPreventsDemotingLastSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	updateRequest := connect.NewRequest(&xylona.UpdateUserRequest{
		Id:        "user-admin",
		UserName:  "admin",
		Email:     "admin@example.com",
		FirstName: "Admin",
		LastName:  "User",
		SuperUser: false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateRequest, "user-admin")

	_, errUpdateUser := fixture.service.UpdateUser(context.Background(), updateRequest)
	if errUpdateUser == nil {
		t.Fatalf("UpdateUser() expected failed precondition, got nil")
	}
	if connect.CodeOf(errUpdateUser) != connect.CodeFailedPrecondition {
		t.Errorf("UpdateUser() code = %v, want %v", connect.CodeOf(errUpdateUser), connect.CodeFailedPrecondition)
	}
}

func TestDeleteUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	createdUser := createUserForRPCUserTests(t, fixture, "user-delete-rpc", false)

	deleteRequest := connect.NewRequest(&xylona.DeleteUserRequest{
		Id: createdUser.Id,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, deleteRequest, "user-admin")

	_, errDeleteUser := fixture.service.DeleteUser(context.Background(), deleteRequest)
	if errDeleteUser != nil {
		t.Fatalf("DeleteUser() error = %v", errDeleteUser)
	}

	_, errGetUser := fixture.conn.GetUserByID(createdUser.Id)
	if !errors.Is(errGetUser, sql.ErrNoRows) {
		t.Errorf("GetUserByID() error = %v, want %v", errGetUser, sql.ErrNoRows)
	}
}

func TestDeleteUserPreventsSelfDelete(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_ = createUserForRPCUserTests(t, fixture, "user-extra-super", true)

	deleteRequest := connect.NewRequest(&xylona.DeleteUserRequest{
		Id: "user-admin",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, deleteRequest, "user-admin")

	_, errDeleteUser := fixture.service.DeleteUser(context.Background(), deleteRequest)
	if errDeleteUser == nil {
		t.Fatalf("DeleteUser() expected failed precondition, got nil")
	}
	if connect.CodeOf(errDeleteUser) != connect.CodeFailedPrecondition {
		t.Errorf("DeleteUser() code = %v, want %v", connect.CodeOf(errDeleteUser), connect.CodeFailedPrecondition)
	}
}

func TestDeleteUserPreventsDeletingLastSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	deleteRequest := connect.NewRequest(&xylona.DeleteUserRequest{
		Id: "user-admin",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, deleteRequest, "user-admin")

	_, errDeleteUser := fixture.service.DeleteUser(context.Background(), deleteRequest)
	if errDeleteUser == nil {
		t.Fatalf("DeleteUser() expected failed precondition, got nil")
	}
	if connect.CodeOf(errDeleteUser) != connect.CodeFailedPrecondition {
		t.Errorf("DeleteUser() code = %v, want %v", connect.CodeOf(errDeleteUser), connect.CodeFailedPrecondition)
	}
}

func createUserForRPCUserTests(t *testing.T, fixture *rbacRPCFixture, userName string, superUser bool) *xylona.User {
	t.Helper()

	request := connect.NewRequest(&xylona.CreateUserRequest{
		UserName:  userName,
		Email:     userName + "@example.com",
		FirstName: "Test",
		LastName:  "User",
		Password:  "password123",
		SuperUser: superUser,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	response, errCreateUser := fixture.service.CreateUser(context.Background(), request)
	if errCreateUser != nil {
		t.Fatalf("CreateUser() error = %v", errCreateUser)
	}
	if response.Msg == nil || response.Msg.User == nil {
		t.Fatalf("CreateUser() returned empty response")
	}

	return response.Msg.User
}

func TestCreateUserDuplicateUsername(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_ = createUserForRPCUserTests(t, fixture, "dup-username", false)

	request := connect.NewRequest(&xylona.CreateUserRequest{
		UserName:  "dup-username",
		Email:     "dup-username-2@example.com",
		FirstName: "Dup",
		LastName:  "User",
		Password:  "password123",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errCreate := fixture.service.CreateUser(context.Background(), request)
	if errCreate == nil {
		t.Fatalf("CreateUser(duplicate username) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeAlreadyExists {
		t.Errorf("CreateUser(duplicate username) code = %v, want %v", connect.CodeOf(errCreate), connect.CodeAlreadyExists)
	}
}

func TestCreateUserEmptyFields(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	tests := []struct {
		name     string
		userName string
		email    string
		password string
	}{
		{name: "empty username", userName: "", email: "valid@example.com", password: "password123"},
		{name: "empty email", userName: "valid-user", email: "", password: "password123"},
		{name: "empty password", userName: "valid-user", email: "valid@example.com", password: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := connect.NewRequest(&xylona.CreateUserRequest{
				UserName:  tt.userName,
				Email:     tt.email,
				FirstName: "Test",
				LastName:  "User",
				Password:  tt.password,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

			_, errCreate := fixture.service.CreateUser(context.Background(), request)
			if errCreate == nil {
				t.Fatalf("CreateUser(%s) expected error, got nil", tt.name)
			}
			if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
				t.Errorf("CreateUser(%s) code = %v, want %v", tt.name, connect.CodeOf(errCreate), connect.CodeInvalidArgument)
			}
		})
	}
}

func TestGetUserNotFound(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.GetUserDetailsRequest{
		Id: "nonexistent-user-id",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errGet := fixture.service.GetUser(context.Background(), request)
	if errGet == nil {
		t.Fatalf("GetUser(nonexistent) expected error, got nil")
	}
	if connect.CodeOf(errGet) != connect.CodeNotFound {
		t.Errorf("GetUser(nonexistent) code = %v, want %v", connect.CodeOf(errGet), connect.CodeNotFound)
	}
}

func TestUpdateUserNotFound(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.UpdateUserRequest{
		Id:        "nonexistent-user-id",
		UserName:  "nonexistent",
		Email:     "nonexistent@example.com",
		FirstName: "Non",
		LastName:  "Existent",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errUpdate := fixture.service.UpdateUser(context.Background(), request)
	if errUpdate == nil {
		t.Fatalf("UpdateUser(nonexistent) expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodeNotFound {
		t.Errorf("UpdateUser(nonexistent) code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeNotFound)
	}
}

func TestDeleteUserNotFound(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Need a second super user so the fixture admin can delete
	_ = createUserForRPCUserTests(t, fixture, "extra-super-for-delete-notfound", true)

	request := connect.NewRequest(&xylona.DeleteUserRequest{
		Id: "nonexistent-user-id",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errDelete := fixture.service.DeleteUser(context.Background(), request)
	if errDelete == nil {
		t.Fatalf("DeleteUser(nonexistent) expected error, got nil")
	}
	if connect.CodeOf(errDelete) != connect.CodeNotFound {
		t.Errorf("DeleteUser(nonexistent) code = %v, want %v", connect.CodeOf(errDelete), connect.CodeNotFound)
	}
}
