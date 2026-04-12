package adminipc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/pkg/usermgmt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

// UserHandler exposes shared user-management operations on the local IPC transport.
type UserHandler struct {
	xylonaconnect.UnimplementedXylonaHandler

	service *usermgmt.Service
}

// NewUserHandler creates a local IPC handler backed by the shared user service.
func NewUserHandler(service *usermgmt.Service) *UserHandler {
	return &UserHandler{service: service}
}

// CreateUser creates a local user through the local admin transport.
func (h *UserHandler) CreateUser(_ context.Context, request *connect.Request[xylona.CreateUserRequest]) (*connect.Response[xylona.CreateUserResponse], error) {
	user, errCreate := h.service.Create(usermgmt.CreateInput{
		UserName:  request.Msg.GetUserName(),
		Email:     request.Msg.GetEmail(),
		FirstName: request.Msg.GetFirstName(),
		LastName:  request.Msg.GetLastName(),
		Password:  request.Msg.GetPassword(),
		SuperUser: request.Msg.GetSuperUser(),
	})
	if errCreate != nil {
		return nil, mapUserManagementError(errCreate)
	}
	return connect.NewResponse(&xylona.CreateUserResponse{User: userToProto(user)}), nil
}

// ListUsers lists local users through the local admin transport.
func (h *UserHandler) ListUsers(_ context.Context, _ *connect.Request[xylona.ListUsersRequest]) (*connect.Response[xylona.ListUsersResponse], error) {
	users, errList := h.service.List()
	if errList != nil {
		return nil, mapUserManagementError(errList)
	}

	responseUsers := make([]*xylona.User, len(users))
	for i, user := range users {
		responseUsers[i] = userToProto(user)
	}

	return connect.NewResponse(&xylona.ListUsersResponse{Users: responseUsers}), nil
}

// GetUser looks up a local user by ID through the local admin transport.
func (h *UserHandler) GetUser(_ context.Context, request *connect.Request[xylona.GetUserDetailsRequest]) (*connect.Response[xylona.GetUserDetailsResponse], error) {
	user, errGetUser := h.service.GetByID(request.Msg.GetId())
	if errGetUser != nil {
		return nil, mapUserManagementError(errGetUser)
	}
	return connect.NewResponse(&xylona.GetUserDetailsResponse{User: userToProto(user)}), nil
}

// UpdateUser updates a local user through the local admin transport.
func (h *UserHandler) UpdateUser(_ context.Context, request *connect.Request[xylona.UpdateUserRequest]) (*connect.Response[xylona.UpdateUserResponse], error) {
	userName := request.Msg.GetUserName()
	email := request.Msg.GetEmail()
	firstName := request.Msg.GetFirstName()
	lastName := request.Msg.GetLastName()
	superUser := request.Msg.GetSuperUser()

	updateInput := usermgmt.UpdateInput{
		ID:        request.Msg.GetId(),
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

	user, errUpdate := h.service.Update(updateInput)
	if errUpdate != nil {
		return nil, mapUserManagementError(errUpdate)
	}
	return connect.NewResponse(&xylona.UpdateUserResponse{User: userToProto(user)}), nil
}

// DeleteUser deletes a local user through the local admin transport.
func (h *UserHandler) DeleteUser(_ context.Context, request *connect.Request[xylona.DeleteUserRequest]) (*connect.Response[xylona.DeleteUserResponse], error) {
	errDelete := h.service.Delete(usermgmt.DeleteInput{ID: request.Msg.GetId()})
	if errDelete != nil {
		return nil, mapUserManagementError(errDelete)
	}
	return connect.NewResponse(&xylona.DeleteUserResponse{}), nil
}

func mapUserManagementError(err error) error {
	switch {
	case errors.Is(err, usermgmt.ErrUserExists):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, usermgmt.ErrUserNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, usermgmt.ErrLastSuperUser), errors.Is(err, usermgmt.ErrCannotDeleteSelf):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, usermgmt.ErrUserNameRequired),
		errors.Is(err, usermgmt.ErrEmailRequired),
		errors.Is(err, usermgmt.ErrPasswordRequired),
		errors.Is(err, usermgmt.ErrPasswordEmpty),
		errors.Is(err, usermgmt.ErrUserIDRequired):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, errors.New(`internal error`))
	}
}

func userToProto(user *usermgmt.User) *xylona.User {
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
