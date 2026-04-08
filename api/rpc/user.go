package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// CreateUser creates a new local user account.
func (xs *XylonaService) CreateUser(_ context.Context, request *connect.Request[xylona.CreateUserRequest]) (*connect.Response[xylona.CreateUserResponse], error) {
	_, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	userName := strings.TrimSpace(request.Msg.GetUserName())
	email := strings.TrimSpace(request.Msg.GetEmail())
	firstName := strings.TrimSpace(request.Msg.GetFirstName())
	lastName := strings.TrimSpace(request.Msg.GetLastName())
	password := request.Msg.GetPassword()
	if userName == "" {
		return nil, invalidArg("user_name is required")
	}
	if email == "" {
		return nil, invalidArg("email is required")
	}
	if strings.TrimSpace(password) == "" {
		return nil, invalidArg("password is required")
	}

	passwordHash, errHashPassword := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if errHashPassword != nil {
		log.Error().Err(errHashPassword).Str("user_name", userName).Msg("failed to hash password for user create")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		log.Error().Err(errID).Str("user_name", userName).Msg("failed to generate user id")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	user := &models.UserSetter{
		ID:           omit.From(newID.String()),
		UserName:     omit.From(userName),
		Email:        omit.From(email),
		FirstName:    omit.From(firstName),
		LastName:     omit.From(lastName),
		PasswordHash: omit.From(string(passwordHash)),
		SuperUser:    omit.From(request.Msg.GetSuperUser()),
	}

	createdUser, errCreateUser := xs.db.CreateUser(user)
	if errCreateUser != nil {
		if isSQLiteUniqueConstraintError(errCreateUser) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("user already exists"))
		}
		log.Error().Err(errCreateUser).Str("user_name", userName).Msg("failed to create user")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	return connect.NewResponse(&xylona.CreateUserResponse{
		User: helpers.UserModelToProto(createdUser),
	}), nil
}

// ListUsers returns all local user accounts.
func (xs *XylonaService) ListUsers(_ context.Context, request *connect.Request[xylona.ListUsersRequest]) (*connect.Response[xylona.ListUsersResponse], error) {
	_, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	users, errGetUsers := xs.db.GetAllUsers()
	if errGetUsers != nil {
		if errors.Is(errGetUsers, sql.ErrNoRows) {
			return connect.NewResponse(&xylona.ListUsersResponse{
				Users: []*xylona.User{},
			}), nil
		}
		log.Error().Err(errGetUsers).Msg("failed to list users")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	userResponses := make([]*xylona.User, len(users))
	for i, user := range users {
		userProto := helpers.UserModelToProto(user)
		userResponses[i] = userProto
	}

	return connect.NewResponse(&xylona.ListUsersResponse{
		Users: userResponses,
	}), nil
}

// GetUser returns a local user account by ID.
func (xs *XylonaService) GetUser(_ context.Context, request *connect.Request[xylona.GetUserDetailsRequest]) (*connect.Response[xylona.GetUserDetailsResponse], error) {
	_, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	userID := strings.TrimSpace(request.Msg.GetId())
	if userID == "" {
		return nil, invalidArg("id is required")
	}

	targetUser, errGetUser := xs.db.GetUserByID(userID)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetUser).Str("user_id", userID).Msg("failed to get user")
		return nil, internalErr()
	}

	return connect.NewResponse(&xylona.GetUserDetailsResponse{
		User: helpers.UserModelToProto(targetUser),
	}), nil
}

// UpdateUser updates a local user account.
func (xs *XylonaService) UpdateUser(_ context.Context, request *connect.Request[xylona.UpdateUserRequest]) (*connect.Response[xylona.UpdateUserResponse], error) {
	actingUser, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	userID := strings.TrimSpace(request.Msg.GetId())
	userName := strings.TrimSpace(request.Msg.GetUserName())
	email := strings.TrimSpace(request.Msg.GetEmail())
	firstName := strings.TrimSpace(request.Msg.GetFirstName())
	lastName := strings.TrimSpace(request.Msg.GetLastName())

	if userID == "" {
		return nil, invalidArg("id is required")
	}
	if userName == "" {
		return nil, invalidArg("user_name is required")
	}
	if email == "" {
		return nil, invalidArg("email is required")
	}

	targetUser, errGetTargetUser := xs.db.GetUserByID(userID)
	if errGetTargetUser != nil {
		if errors.Is(errGetTargetUser, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetTargetUser).Str("user_id", userID).Msg("failed to load target user")
		return nil, internalErr()
	}

	if targetUser.SuperUser && !request.Msg.GetSuperUser() && actingUser.ID == targetUser.ID {
		superUserCount, errSuperUserCount := xs.countSuperUsers()
		if errSuperUserCount != nil {
			log.Error().Err(errSuperUserCount).Msg("failed to count super users for update")
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		if superUserCount <= 1 {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("cannot remove the last super user"))
		}
	}

	userSetter := &models.UserSetter{
		ID:        omit.From(userID),
		UserName:  omit.From(userName),
		Email:     omit.From(email),
		FirstName: omit.From(firstName),
		LastName:  omit.From(lastName),
		SuperUser: omit.From(request.Msg.GetSuperUser()),
		UpdatedAt: omit.From(time.Now().UTC()),
	}

	password := request.Msg.GetPassword()
	if password != "" {
		if strings.TrimSpace(password) == "" {
			return nil, invalidArg("password cannot be empty")
		}

		passwordHash, errHashPassword := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if errHashPassword != nil {
			log.Error().Err(errHashPassword).Str("user_id", userID).Msg("failed to hash password for user update")
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		userSetter.PasswordHash = omit.From(string(passwordHash))
	}

	errUpdateUser := xs.db.UpdateUser(userSetter)
	if errUpdateUser != nil {
		if isSQLiteUniqueConstraintError(errUpdateUser) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("user already exists"))
		}
		log.Error().Err(errUpdateUser).Str("user_id", userID).Msg("failed to update user")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	updatedUser, errGetUpdatedUser := xs.db.GetUserByID(userID)
	if errGetUpdatedUser != nil {
		log.Error().Err(errGetUpdatedUser).Str("user_id", userID).Msg("failed to fetch updated user")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	return connect.NewResponse(&xylona.UpdateUserResponse{
		User: helpers.UserModelToProto(updatedUser),
	}), nil
}

// DeleteUser deletes a local user account.
func (xs *XylonaService) DeleteUser(_ context.Context, request *connect.Request[xylona.DeleteUserRequest]) (*connect.Response[xylona.DeleteUserResponse], error) {
	actingUser, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	userID := strings.TrimSpace(request.Msg.GetId())
	if userID == "" {
		return nil, invalidArg("id is required")
	}

	targetUser, errGetTargetUser := xs.db.GetUserByID(userID)
	if errGetTargetUser != nil {
		if errors.Is(errGetTargetUser, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetTargetUser).Str("user_id", userID).Msg("failed to load target user for delete")
		return nil, internalErr()
	}

	if targetUser.SuperUser {
		superUserCount, errSuperUserCount := xs.countSuperUsers()
		if errSuperUserCount != nil {
			log.Error().Err(errSuperUserCount).Msg("failed to count super users for delete")
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		if superUserCount <= 1 {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("cannot delete the last super user"))
		}
	}

	if actingUser.ID == targetUser.ID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("cannot delete your own user"))
	}

	errDeleteUser := xs.db.DeleteUser(userID)
	if errDeleteUser != nil {
		if errors.Is(errDeleteUser, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errDeleteUser).Str("user_id", userID).Msg("failed to delete user")
		return nil, internalErr()
	}

	return connect.NewResponse(&xylona.DeleteUserResponse{}), nil
}

func (xs *XylonaService) requireSuperUserForUserManagement(header http.Header) (*models.User, error) {
	user, errUser := xs.getUserFromHeader(header)
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("insufficient permissions")
	}

	return user, nil
}

func (xs *XylonaService) countSuperUsers() (int, error) {
	count, errCount := xs.db.CountSuperUsers()
	if errCount != nil {
		return 0, fmt.Errorf("rpc: count superusers: %w", errCount)
	}
	return count, nil
}
