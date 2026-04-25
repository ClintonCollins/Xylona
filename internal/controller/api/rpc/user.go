package rpc

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/pkg/usermgmt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// CreateUser creates a new local user account.
func (xs *XylonaService) CreateUser(_ context.Context, request *connect.Request[xylona.CreateUserRequest]) (*connect.Response[xylona.CreateUserResponse], error) {
	_, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	createdUser, errCreateUser := xs.userManagementService().Create(usermgmt.CreateInput{
		UserName:  request.Msg.GetUserName(),
		Email:     request.Msg.GetEmail(),
		FirstName: request.Msg.GetFirstName(),
		LastName:  request.Msg.GetLastName(),
		Password:  request.Msg.GetPassword(),
		SuperUser: request.Msg.GetSuperUser(),
	})
	if errCreateUser != nil {
		return nil, mapUserManagementError(errCreateUser)
	}

	return connect.NewResponse(&xylona.CreateUserResponse{
		User: userManagementUserToProto(createdUser),
	}), nil
}

// ListUsers returns all local user accounts.
func (xs *XylonaService) ListUsers(_ context.Context, request *connect.Request[xylona.ListUsersRequest]) (*connect.Response[xylona.ListUsersResponse], error) {
	_, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	users, errListUsers := xs.userManagementService().List()
	if errListUsers != nil {
		return nil, mapUserManagementError(errListUsers)
	}

	responseUsers := make([]*xylona.User, len(users))
	for i, user := range users {
		responseUsers[i] = userManagementUserToProto(user)
	}

	return connect.NewResponse(&xylona.ListUsersResponse{
		Users: responseUsers,
	}), nil
}

// GetUser returns a local user account by ID.
func (xs *XylonaService) GetUser(_ context.Context, request *connect.Request[xylona.GetUserDetailsRequest]) (*connect.Response[xylona.GetUserDetailsResponse], error) {
	_, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	targetUser, errGetUser := xs.userManagementService().GetByID(strings.TrimSpace(request.Msg.GetId()))
	if errGetUser != nil {
		return nil, mapUserManagementError(errGetUser)
	}

	return connect.NewResponse(&xylona.GetUserDetailsResponse{
		User: userManagementUserToProto(targetUser),
	}), nil
}

// UpdateUser updates a local user account.
func (xs *XylonaService) UpdateUser(_ context.Context, request *connect.Request[xylona.UpdateUserRequest]) (*connect.Response[xylona.UpdateUserResponse], error) {
	_, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	userName := request.Msg.GetUserName()
	email := request.Msg.GetEmail()
	firstName := request.Msg.GetFirstName()
	lastName := request.Msg.GetLastName()
	superUser := request.Msg.GetSuperUser()
	updateInput := usermgmt.UpdateInput{
		ID:        strings.TrimSpace(request.Msg.GetId()),
		UserName:  &userName,
		Email:     &email,
		FirstName: &firstName,
		LastName:  &lastName,
		SuperUser: &superUser,
	}
	password := request.Msg.GetPassword()
	if password != `` {
		updateInput.Password = &password
	}

	updatedUser, errUpdateUser := xs.userManagementService().Update(updateInput)
	if errUpdateUser != nil {
		return nil, mapUserManagementError(errUpdateUser)
	}

	return connect.NewResponse(&xylona.UpdateUserResponse{
		User: userManagementUserToProto(updatedUser),
	}), nil
}

// DeleteUser deletes a local user account.
func (xs *XylonaService) DeleteUser(_ context.Context, request *connect.Request[xylona.DeleteUserRequest]) (*connect.Response[xylona.DeleteUserResponse], error) {
	actingUser, errRequireSuperUser := xs.requireSuperUserForUserManagement(request.Header())
	if errRequireSuperUser != nil {
		return nil, errRequireSuperUser
	}

	errDeleteUser := xs.userManagementService().Delete(usermgmt.DeleteInput{
		ID:                strings.TrimSpace(request.Msg.GetId()),
		ActingUserID:      actingUser.ID,
		PreventSelfDelete: true,
	})
	if errDeleteUser != nil {
		return nil, mapUserManagementError(errDeleteUser)
	}

	return connect.NewResponse(&xylona.DeleteUserResponse{}), nil
}

func (xs *XylonaService) requireSuperUserForUserManagement(header http.Header) (*models.User, error) {
	user, errUser := xs.getUserFromHeader(header)
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied(`insufficient permissions`)
	}

	return user, nil
}

func mapUserManagementError(err error) error {
	switch {
	case errors.Is(err, usermgmt.ErrUserExists):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, usermgmt.ErrUserNotFound):
		return notFoundErr()
	case errors.Is(err, usermgmt.ErrLastSuperUser), errors.Is(err, usermgmt.ErrCannotDeleteSelf):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, usermgmt.ErrUserNameRequired),
		errors.Is(err, usermgmt.ErrEmailRequired),
		errors.Is(err, usermgmt.ErrPasswordRequired),
		errors.Is(err, usermgmt.ErrPasswordEmpty),
		errors.Is(err, usermgmt.ErrUserIDRequired):
		return invalidArg(err.Error())
	default:
		return connect.NewError(connect.CodeInternal, errors.New(`internal error`))
	}
}

func userManagementUserToProto(user *usermgmt.User) *xylona.User {
	userProto := &xylona.User{
		Id:        user.ID,
		UserName:  user.UserName,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		SuperUser: user.SuperUser,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}
	if user.LastLoginAt != nil {
		userProto.LastLogin = timestamppb.New(*user.LastLoginAt)
	}
	return userProto
}

func (xs *XylonaService) userManagementService() *usermgmt.Service {
	if xs.userService == nil {
		xs.userService = usermgmt.NewService(xs.db)
	}
	return xs.userService
}
