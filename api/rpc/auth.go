package rpc

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs XylonaService) CheckUserAuthenticated(ctx context.Context, request *connect.Request[xylona.CheckUserAuthenticatedRequest]) (*connect.Response[xylona.CheckUserAuthenticatedResponse], error) {
	sessionUnauthenticatedResponse := &connect.Response[xylona.CheckUserAuthenticatedResponse]{
		Msg: &xylona.CheckUserAuthenticatedResponse{
			Authenticated: false,
		},
	}

	user, err := xs.getUserFromHeader(request.Header())
	if err != nil {
		return sessionUnauthenticatedResponse, nil
	}

	return &connect.Response[xylona.CheckUserAuthenticatedResponse]{
		Msg: &xylona.CheckUserAuthenticatedResponse{
			Authenticated: true,
			User: &xylona.User{
				Id:        user.ID,
				UserName:  user.UserName,
				Email:     user.Email,
				FirstName: user.FirstName,
				LastName:  user.LastName,
				SuperUser: user.SuperUser,
				LastLogin: timestamppb.New(user.LastLoginAt),
				CreatedAt: timestamppb.New(user.CreatedAt),
			},
		},
	}, nil
}

func (xs XylonaService) Login(ctx context.Context, request *connect.Request[xylona.LoginRequest]) (*connect.Response[xylona.LoginResponse], error) {
	userName := request.Msg.GetUserName()
	password := request.Msg.GetPassword()

	user, errGetUser := xs.db.GetUser(userName)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid email or password"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	errCompare := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if errCompare != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid email or password"))
	}

	x := &models.UserSessionSetter{
		ID:        omit.From(""),
		UserID:    omit.From(user.ID),
		Token:     omit.From(""),
		ExpiresAt: omit.From(time.Now().Add(24 * time.Hour * 30)),
	}

	log.Printf("User session: %+v", x)
	newSession, errSession := xs.db.CreateUserSession(x)

	if errSession != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	resp := &connect.Response[xylona.LoginResponse]{
		Msg: &xylona.LoginResponse{
			User: &xylona.User{
				Id:        user.ID,
				UserName:  user.UserName,
				Email:     user.Email,
				FirstName: user.FirstName,
				LastName:  user.LastName,
				SuperUser: user.SuperUser,
				LastLogin: timestamppb.New(user.LastLoginAt),
				CreatedAt: timestamppb.New(user.CreatedAt),
			},
		},
	}

	encodedSession, errEncodeSession := xs.secureCookie.Encode(gatekeeper.SessionTokenCookieName, newSession.Token)
	if errEncodeSession != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	tokenCookie := &http.Cookie{
		Name:     gatekeeper.SessionTokenCookieName,
		Value:    encodedSession,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour * 30),
		Secure:   false,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	idCookie := &http.Cookie{
		Name:     gatekeeper.SessionIDCookieName,
		Value:    newSession.ID,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour * 30),
		Secure:   false,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	resp.Header().Add("Set-Cookie", tokenCookie.String())
	resp.Header().Add("Set-Cookie", idCookie.String())

	return resp, nil
}

func (xs XylonaService) Logout(ctx context.Context, request *connect.Request[xylona.LogoutRequest]) (*connect.Response[xylona.LogoutResponse], error) {
	//TODO implement me
	panic("implement me")
}
