package rpc

import (
	"context"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/valheim"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func joinPasswordStateToProto(state readiness.JoinPasswordState) *xylona.JoinPasswordState {
	result := &xylona.JoinPasswordState{Supported: state.Supported, Configured: state.Configured, ValidationIssues: state.ValidationIssues}

	return result
}

// GetJoinPasswordState returns join password setup metadata.
func (xs *XylonaService) GetJoinPasswordState(_ context.Context, request *connect.Request[xylona.GetJoinPasswordStateRequest]) (*connect.Response[xylona.GetJoinPasswordStateResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	server, errServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errServer != nil {
		return nil, dbLookup(errServer)
	}
	errPermission := xs.ensureLocalServerPermission(user, server, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}
	return connect.NewResponse(&xylona.GetJoinPasswordStateResponse{State: joinPasswordStateToProto(readiness.GetJoinPasswordState(xs.db, server))}), nil
}

// SetJoinPassword sets or replaces the join password.
func (xs *XylonaService) SetJoinPassword(_ context.Context, request *connect.Request[xylona.SetJoinPasswordRequest]) (*connect.Response[xylona.SetJoinPasswordResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	server, errServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errServer != nil {
		return nil, dbLookup(errServer)
	}
	errPermission := xs.ensureJoinPasswordPermission(user, server)
	if errPermission != nil {
		return nil, errPermission
	}
	errValidate := valheim.ValidateJoinPassword(request.Msg.GetPassword(), server.Name)
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}
	errSet := xs.db.SetValheimJoinPassword(server, request.Msg.GetPassword())

	if errSet != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errSet)
	}
	return connect.NewResponse(&xylona.SetJoinPasswordResponse{State: joinPasswordStateToProto(readiness.GetJoinPasswordState(xs.db, server))}), nil
}

// ClearJoinPassword disables password protection on the next launch.
func (xs *XylonaService) ClearJoinPassword(_ context.Context, request *connect.Request[xylona.ClearJoinPasswordRequest]) (*connect.Response[xylona.ClearJoinPasswordResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	server, errServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errServer != nil {
		return nil, dbLookup(errServer)
	}
	errPermission := xs.ensureJoinPasswordPermission(user, server)
	if errPermission != nil {
		return nil, errPermission
	}
	errClear := xs.db.ClearValheimJoinPassword(server)

	if errClear != nil {
		return nil, internalErrf("join password could not be cleared")
	}
	return connect.NewResponse(&xylona.ClearJoinPasswordResponse{State: joinPasswordStateToProto(readiness.GetJoinPasswordState(xs.db, server))}), nil
}

func (xs *XylonaService) ensureJoinPasswordPermission(user *models.User, server *models.GameServer) error {
	errPermission := xs.ensureLocalServerPermission(user, server, "game_server.settings")
	if errPermission != nil {
		return errPermission
	}
	if server.GameID != "valheim" {
		return invalidArg("join passwords are supported only for Valheim")
	}
	return nil
}

func (xs *XylonaService) validateJoinPasswordServerName(existing, updated *models.GameServer) error {
	if updated.GameID != "valheim" || existing.Name == updated.Name {
		return nil
	}
	password, errPassword := db.GetValheimJoinPassword(existing)
	if errPassword != nil {
		return connect.NewError(connect.CodeFailedPrecondition, errPassword)
	}
	if password == "" {
		return nil
	}
	errValidate := valheim.ValidateJoinPassword(password, updated.Name)
	if errValidate != nil {
		return connect.NewError(connect.CodeInvalidArgument, errValidate)
	}
	return nil
}
