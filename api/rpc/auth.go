package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/pkg/passwordhash"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const defaultSessionDuration = 30 * 24 * time.Hour

// CheckUserAuthenticated returns the current authenticated user and permissions.
func (xs *XylonaService) CheckUserAuthenticated(_ context.Context, request *connect.Request[xylona.CheckUserAuthenticatedRequest]) (*connect.Response[xylona.CheckUserAuthenticatedResponse], error) {
	sessionUnauthenticatedResponse := &connect.Response[xylona.CheckUserAuthenticatedResponse]{
		Msg: &xylona.CheckUserAuthenticatedResponse{
			Authenticated: false,
		},
	}

	user, err := xs.getUserFromHeader(request.Header())
	if err != nil {
		return sessionUnauthenticatedResponse, nil //nolint:nilerr // intentionally returning unauthenticated response instead of propagating the auth error
	}

	permissionIDs, errPermissionIDs := xs.authenticatedPermissionIDs(user)
	if errPermissionIDs != nil {
		log.Error().Err(errPermissionIDs).Str("user_id", user.ID).Msg("Failed to load global permission IDs for authenticated user")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load permissions"))
	}

	return &connect.Response[xylona.CheckUserAuthenticatedResponse]{
		Msg: &xylona.CheckUserAuthenticatedResponse{
			Authenticated: true,
			PermissionIds: permissionIDs,
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

func (xs *XylonaService) authenticatedPermissionIDs(user *models.User) ([]string, error) {
	if xs.permissionIDsForUserFn != nil {
		return xs.permissionIDsForUserFn(user)
	}
	if user.SuperUser {
		return xs.allPermissionIDs, nil
	}
	permissionIDs, errGet := xs.db.GetUserGlobalPermissionIDs(user.ID)
	if errGet != nil {
		return nil, fmt.Errorf("rpc: load user permission IDs: %w", errGet)
	}
	return permissionIDs, nil
}

// Login authenticates a user and creates session cookies.
func (xs *XylonaService) Login(_ context.Context, request *connect.Request[xylona.LoginRequest]) (*connect.Response[xylona.LoginResponse], error) {
	userName := request.Msg.GetUserName()
	password := request.Msg.GetPassword()

	user, errGetUser := xs.db.GetUser(userName)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid email or password"))
		}
		return nil, internalErr()
	}

	passwordMatches, errVerify := passwordhash.Verify(user.PasswordHash, password)
	if errVerify != nil || !passwordMatches {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid email or password"))
	}

	sessionID := uuid.New().String()
	x := &models.UserSessionSetter{
		ID:        omit.From(sessionID),
		UserID:    omit.From(user.ID),
		Token:     omit.From(uuid.New().String()),
		ExpiresAt: omit.From(time.Now().Add(defaultSessionDuration)),
	}

	log.Debug().Str("user_id", user.ID).Msg("Creating user session")
	newSession, errSession := xs.db.CreateUserSession(x)

	if errSession != nil {
		return nil, internalErr()
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
		return nil, internalErr()
	}

	tokenCookie := &http.Cookie{
		Name:     gatekeeper.SessionTokenCookieName,
		Value:    encodedSession,
		Path:     "/",
		Expires:  time.Now().Add(defaultSessionDuration),
		Secure:   xs.secureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	idCookie := &http.Cookie{
		Name:     gatekeeper.SessionIDCookieName,
		Value:    newSession.ID,
		Path:     "/",
		Expires:  time.Now().Add(defaultSessionDuration),
		Secure:   xs.secureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	resp.Header().Add("Set-Cookie", tokenCookie.String())
	resp.Header().Add("Set-Cookie", idCookie.String())

	return resp, nil
}

// Logout clears the current user session cookies.
func (xs *XylonaService) Logout(_ context.Context, request *connect.Request[xylona.LogoutRequest]) (*connect.Response[xylona.LogoutResponse], error) {
	sessionCookies, errGetSession := gatekeeper.GetSessionFromHeader(request.Header())
	if errGetSession == nil {
		errDeleteSession := xs.db.DeleteUserSession(sessionCookies.SessionID)
		if errDeleteSession != nil {
			log.Warn().Err(errDeleteSession).Msg("Failed to delete user session on logout")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete session"))
		}
	}

	resp := &connect.Response[xylona.LogoutResponse]{
		Msg: &xylona.LogoutResponse{},
	}

	clearTokenCookie := &http.Cookie{
		Name:     gatekeeper.SessionTokenCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		Secure:   xs.secureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	clearIDCookie := &http.Cookie{
		Name:     gatekeeper.SessionIDCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		Secure:   xs.secureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	resp.Header().Add("Set-Cookie", clearTokenCookie.String())
	resp.Header().Add("Set-Cookie", clearIDCookie.String())

	return resp, nil
}
