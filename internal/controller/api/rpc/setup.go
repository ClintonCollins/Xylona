package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"

	"github.com/ClintonCollins/Xylona/internal/firstsetup"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// SetSetupToken installs the in-memory first-run token. Nil clears it.
func (xs *XylonaService) SetSetupToken(token *firstsetup.Token) {
	xs.setupToken = token
}

// GetSetupStatus reports whether first-run setup is still required.
func (xs *XylonaService) GetSetupStatus(
	_ context.Context,
	_ *connect.Request[xylona.GetSetupStatusRequest],
) (*connect.Response[xylona.GetSetupStatusResponse], error) {
	superUserCount, errCount := xs.db.CountSuperUsers()
	if errCount != nil {
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.GetSetupStatusResponse{
		Needed: superUserCount == 0,
	}), nil
}

// CompleteSetup creates the first superuser when first-run setup is still open.
func (xs *XylonaService) CompleteSetup(
	_ context.Context,
	request *connect.Request[xylona.CompleteSetupRequest],
) (*connect.Response[xylona.CompleteSetupResponse], error) {
	token := strings.TrimSpace(request.Msg.GetToken())
	setupToken := xs.setupToken
	if setupToken == nil || !setupToken.Valid(token) {
		return nil, connect.NewError(connect.CodePermissionDenied, firstsetup.ErrSetupTokenInvalid)
	}

	email := strings.TrimSpace(request.Msg.GetEmail())
	userName := strings.TrimSpace(request.Msg.GetUserName())
	if email == "" && userName != "" {
		email = userName + "@localhost"
	}

	createdUser, errCreate := firstsetup.CreateFirstSuperUser(xs.userManagementService(), usermgmt.CreateInput{
		UserName: userName,
		Email:    email,
		Password: request.Msg.GetPassword(),
	})
	if errCreate != nil {
		if errors.Is(errCreate, firstsetup.ErrAlreadyInstalled) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errCreate)
		}
		return nil, mapUserManagementError(errCreate)
	}
	if !setupToken.Consume(token) {
		return nil, internalErr()
	}

	sessionID := uuid.New().String()
	newSession, errSession := xs.db.CreateUserSession(&models.UserSessionSetter{
		ID:        omit.From(sessionID),
		UserID:    omit.From(createdUser.ID),
		Token:     omit.From(uuid.New().String()),
		ExpiresAt: omit.From(time.Now().Add(defaultSessionDuration)),
	})
	if errSession != nil {
		return nil, internalErr()
	}

	resp := connect.NewResponse(&xylona.CompleteSetupResponse{
		User: userManagementUserToProto(createdUser),
	})
	errCookies := xs.appendSessionCookies(resp.Header(), request.Header(), newSession)
	if errCookies != nil {
		return nil, internalErr()
	}
	return resp, nil
}
