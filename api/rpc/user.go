package rpc

import (
	"context"
	"database/sql"
	"errors"

	"github.com/aarondl/opt/omit"
	connect_go "github.com/bufbuild/connect-go"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs XylonaService) CreateUser(ctx context.Context, request *connect_go.Request[xylona.CreateUserRequest]) (*connect_go.Response[xylona.CreateUserResponse], error) {
	password := request.Msg.GetPassword()
	passwordHash, errHashPassword := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if errHashPassword != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	user := &models.UserSetter{
		UserName:     omit.From(request.Msg.GetUserName()),
		Email:        omit.From(request.Msg.GetEmail()),
		FirstName:    omit.From(request.Msg.GetFirstName()),
		LastName:     omit.From(request.Msg.GetLastName()),
		PasswordHash: omit.From(string(passwordHash)),
		SuperUser:    omit.From(request.Msg.GetSuperUser()),
	}

	createdUser, errCreateUser := xs.db.CreateUser(user)
	if errCreateUser != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	return &connect_go.Response[xylona.CreateUserResponse]{
		Msg: &xylona.CreateUserResponse{
			User: &xylona.User{
				Id:        createdUser.ID,
				UserName:  createdUser.UserName,
				Email:     createdUser.Email,
				FirstName: createdUser.FirstName,
				LastName:  createdUser.LastName,
				SuperUser: createdUser.SuperUser,
				LastLogin: timestamppb.New(createdUser.LastLoginAt),
				CreatedAt: timestamppb.New(createdUser.CreatedAt),
			},
		},
	}, nil
}

func (xs XylonaService) ListUsers(ctx context.Context, request *connect_go.Request[xylona.ListUsersRequest]) (*connect_go.Response[xylona.ListUsersResponse], error) {
	users, errGetUsers := xs.db.GetAllUsers()
	if errGetUsers != nil {
		if errors.Is(errGetUsers, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("no users found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	userResponses := make([]*xylona.User, len(users))
	for i, user := range users {
		userProto := helpers.UserModelToProto(user)
		userResponses[i] = userProto
	}
	response := &connect_go.Response[xylona.ListUsersResponse]{
		Msg: &xylona.ListUsersResponse{
			Users: userResponses,
		},
	}
	return response, nil
}

func (xs XylonaService) GetUser(ctx context.Context, request *connect_go.Request[xylona.GetUserDetailsRequest]) (*connect_go.Response[xylona.GetUserDetailsResponse], error) {
	user, errGetUser := xs.db.GetUserByID(request.Msg.GetId())
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("user not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	userProto := helpers.UserModelToProto(user)
	response := &connect_go.Response[xylona.GetUserDetailsResponse]{
		Msg: &xylona.GetUserDetailsResponse{
			User: userProto,
		},
	}
	return response, nil
}
