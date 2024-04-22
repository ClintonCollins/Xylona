package rpc

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/aarondl/opt/omit"
	connect_go "github.com/bufbuild/connect-go"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs XylonaService) CheckUserAuthenticated(ctx context.Context, request *connect_go.Request[xylona.CheckUserAuthenticatedRequest]) (*connect_go.Response[xylona.CheckUserAuthenticatedResponse], error) {
	sessionUnauthenticatedResponse := &connect_go.Response[xylona.CheckUserAuthenticatedResponse]{
		Msg: &xylona.CheckUserAuthenticatedResponse{
			Authenticated: false,
		},
	}

	user, err := xs.getUserFromHeader(request.Header())
	if err != nil {
		return sessionUnauthenticatedResponse, nil
	}

	return &connect_go.Response[xylona.CheckUserAuthenticatedResponse]{
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

func (xs XylonaService) Login(ctx context.Context, request *connect_go.Request[xylona.LoginRequest]) (*connect_go.Response[xylona.LoginResponse], error) {
	cookies := getCookiesFromHeader(request.Header().Get("Cookie"))
	sessionID := cookies.Get(SessionIDCookieName)
	sessionToken := cookies.Get(SessionTokenCookieName)
	log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionToken).Msg("Login request")

	userName := request.Msg.GetUserName()
	password := request.Msg.GetPassword()

	user, errGetUser := xs.db.GetUser(userName)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeUnauthenticated, errors.New("invalid email or password"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	errCompare := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if errCompare != nil {
		return nil, connect_go.NewError(connect_go.CodeUnauthenticated, errors.New("invalid email or password"))
	}

	newSession, errSession := xs.db.CreateUserSession(&models.UserSessionSetter{
		UserID:    omit.From(user.ID),
		ExpiresAt: omit.From(time.Now().Add(24 * time.Hour * 30)),
	})
	if errSession != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	resp := &connect_go.Response[xylona.LoginResponse]{
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

	encodedSession, errEncodeSession := xs.secureCookie.Encode(SessionTokenCookieName, newSession.Token)
	if errEncodeSession != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	tokenCookie := &http.Cookie{
		Name:     SessionTokenCookieName,
		Value:    encodedSession,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour * 30),
		Secure:   false,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	idCookie := &http.Cookie{
		Name:     SessionIDCookieName,
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

func (xs XylonaService) Logout(ctx context.Context, request *connect_go.Request[xylona.LogoutRequest]) (*connect_go.Response[xylona.LogoutResponse], error) {
	//TODO implement me
	panic("implement me")
}
