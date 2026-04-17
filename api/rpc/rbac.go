package rpc

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// TODO(hub-spoke step 7): the Federated* access grant RPCs are holdovers from
// the federation mesh. They're stubbed Unimplemented here and will be removed
// from the proto (and the frontend) in step 7 along with the other mesh RPCs.

// ListRoles returns all defined RBAC roles.
func (xs *XylonaService) ListRoles(_ context.Context, request *connect.Request[xylona.ListRolesRequest]) (*connect.Response[xylona.ListRolesResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	roles, permissionsByRole, errGetRoles := xs.db.GetAllRolesWithPermissions()
	if errGetRoles != nil {
		log.Error().Err(errGetRoles).Msg("failed to list roles")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list roles"))
	}

	resp := &xylona.ListRolesResponse{}
	for _, role := range roles {
		resp.Roles = append(resp.Roles, &xylona.Role{
			Id:            role.ID,
			Name:          role.Name,
			Description:   role.Description,
			IsSystem:      role.IsSystem,
			PermissionIds: permissionsByRole[role.ID],
		})
	}

	return connect.NewResponse(resp), nil
}

// ListPermissions returns all available RBAC permissions.
func (xs *XylonaService) ListPermissions(_ context.Context, request *connect.Request[xylona.ListPermissionsRequest]) (*connect.Response[xylona.ListPermissionsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	permissions, errGetPermissions := xs.db.GetAllPermissions()
	if errGetPermissions != nil {
		log.Error().Err(errGetPermissions).Msg("failed to list permissions")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list permissions"))
	}

	resp := &xylona.ListPermissionsResponse{}
	for _, permission := range permissions {
		resp.Permissions = append(resp.Permissions, &xylona.Permission{
			Id:          permission.ID,
			Name:        permission.Name,
			Description: permission.Description,
		})
	}

	return connect.NewResponse(resp), nil
}

// CreateRole creates a new RBAC role.
func (xs *XylonaService) CreateRole(_ context.Context, request *connect.Request[xylona.CreateRoleRequest]) (*connect.Response[xylona.CreateRoleResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("insufficient permissions")
	}

	name := strings.TrimSpace(request.Msg.GetName())
	if name == "" {
		return nil, invalidArg("name is required")
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		log.Error().Err(errID).Msg("failed to generate role id")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create role"))
	}

	errCreateRole := xs.db.CreateRoleWithPermissions(newID.String(), name, request.Msg.GetDescription(), request.Msg.GetPermissionIds())
	if errCreateRole != nil {
		if isSQLiteUniqueConstraintError(errCreateRole) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("role already exists"))
		}
		if isSQLiteForeignKeyError(errCreateRole) {
			return nil, invalidArg("one or more permission IDs are invalid")
		}
		log.Error().Err(errCreateRole).Str("role_name", name).Msg("failed to create role")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create role"))
	}

	role, errGetRole := xs.db.GetRoleByID(newID.String())
	if errGetRole != nil {
		log.Error().Err(errGetRole).Str("role_id", newID.String()).Msg("failed to fetch created role")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create role"))
	}

	permissionIDs, errGetPermissions := xs.db.GetPermissionsForRole(role.ID)
	if errGetPermissions != nil {
		log.Error().Err(errGetPermissions).Str("role_id", role.ID).Msg("failed to fetch role permissions")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create role"))
	}

	return connect.NewResponse(&xylona.CreateRoleResponse{
		Role: &xylona.Role{
			Id:            role.ID,
			Name:          role.Name,
			Description:   role.Description,
			IsSystem:      role.IsSystem,
			PermissionIds: permissionIDs,
		},
	}), nil
}

// DeleteRole deletes an RBAC role.
func (xs *XylonaService) DeleteRole(_ context.Context, request *connect.Request[xylona.DeleteRoleRequest]) (*connect.Response[xylona.DeleteRoleResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("insufficient permissions")
	}

	roleID := strings.TrimSpace(request.Msg.GetRoleId())
	if roleID == "" {
		return nil, invalidArg("role_id is required")
	}

	errDeleteRole := xs.db.DeleteRole(roleID)
	if errDeleteRole != nil {
		if errors.Is(errDeleteRole, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		if errors.Is(errDeleteRole, db.ErrRoleIsSystem) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("cannot delete system role"))
		}
		log.Error().Err(errDeleteRole).Str("role_id", roleID).Msg("failed to delete role")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete role"))
	}

	return connect.NewResponse(&xylona.DeleteRoleResponse{}), nil
}

// ListGameServerAccessGrants lists direct game server access grants for a
// controller-managed server.
func (xs *XylonaService) ListGameServerAccessGrants(_ context.Context, request *connect.Request[xylona.ListGameServerAccessGrantsRequest]) (*connect.Response[xylona.ListGameServerAccessGrantsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}

	gameServer, errLookup := xs.db.GetGameServerByID(gameServerID)
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}

	if !user.SuperUser && gameServer.UserID != user.ID {
		return nil, permissionDenied("insufficient permissions")
	}

	assignments, errGetAssignments := xs.db.GetUserRoleAssignmentsForServer(gameServer.ID)
	if errGetAssignments != nil {
		log.Error().Err(errGetAssignments).Str("game_server_id", gameServer.ID).Msg("failed to list server access grants")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	resp := &xylona.ListGameServerAccessGrantsResponse{}
	for _, assignment := range assignments {
		grant, errBuildGrant := xs.buildGameServerAccessGrant(assignment)
		if errBuildGrant != nil {
			return nil, errBuildGrant
		}
		resp.Grants = append(resp.Grants, grant)
	}

	return connect.NewResponse(resp), nil
}

// GrantGameServerAccess grants a user access to a game server.
func (xs *XylonaService) GrantGameServerAccess(_ context.Context, request *connect.Request[xylona.GrantGameServerAccessRequest]) (*connect.Response[xylona.GrantGameServerAccessResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	userID := strings.TrimSpace(request.Msg.GetUserId())
	roleID := strings.TrimSpace(request.Msg.GetRoleId())
	if userID == "" || roleID == "" {
		return nil, invalidArg("user_id and role_id are required")
	}

	gameServer, errLookup := xs.db.GetGameServerByID(gameServerID)
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}

	if !user.SuperUser && gameServer.UserID != user.ID {
		return nil, permissionDenied("insufficient permissions")
	}

	_, errGetTargetUser := xs.db.GetUserByID(userID)
	if errGetTargetUser != nil {
		if errors.Is(errGetTargetUser, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetTargetUser).Str("user_id", userID).Msg("failed to verify target user")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	_, errGetRole := xs.db.GetRoleByID(roleID)
	if errGetRole != nil {
		if errors.Is(errGetRole, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetRole).Str("role_id", roleID).Msg("failed to verify role")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		log.Error().Err(errID).Msg("failed to generate access grant id")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	errCreateGrant := xs.db.CreateUserRoleAssignment(
		newID.String(),
		userID,
		roleID,
		gameServer.ID,
		user.ID,
	)
	if errCreateGrant != nil {
		if isSQLiteUniqueConstraintError(errCreateGrant) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("grant already exists"))
		}
		log.Error().Err(errCreateGrant).
			Str("game_server_id", gameServer.ID).
			Str("user_id", userID).
			Str("role_id", roleID).
			Msg("failed to create access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	assignment, errGetAssignment := xs.db.GetUserRoleAssignmentByID(newID.String())
	if errGetAssignment != nil {
		log.Error().Err(errGetAssignment).Str("grant_id", newID.String()).Msg("failed to fetch created grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	grant, errBuildGrant := xs.buildGameServerAccessGrant(assignment)
	if errBuildGrant != nil {
		return nil, errBuildGrant
	}

	return connect.NewResponse(&xylona.GrantGameServerAccessResponse{Grant: grant}), nil
}

// RevokeGameServerAccess revokes a direct game server access grant.
func (xs *XylonaService) RevokeGameServerAccess(_ context.Context, request *connect.Request[xylona.RevokeGameServerAccessRequest]) (*connect.Response[xylona.RevokeGameServerAccessResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	grantID := strings.TrimSpace(request.Msg.GetGrantId())
	if grantID == "" {
		return nil, invalidArg("grant_id is required")
	}

	assignment, errGetAssignment := xs.db.GetUserRoleAssignmentByID(grantID)
	if errGetAssignment != nil {
		if errors.Is(errGetAssignment, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetAssignment).Str("grant_id", grantID).Msg("failed to fetch local access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
	}
	if !assignment.GameServerID.IsValue() || assignment.GameServerID.IsNull() {
		return nil, invalidArg("grant is not scoped to a game server")
	}
	errServer := xs.requireLocalServerOwnerOrSuper(user, assignment.GameServerID.MustGet())
	if errServer != nil {
		return nil, errServer
	}
	errDelete := xs.db.DeleteUserRoleAssignment(grantID)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("grant_id", grantID).Msg("failed to delete local access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
	}
	return connect.NewResponse(&xylona.RevokeGameServerAccessResponse{}), nil
}

func (xs *XylonaService) requireLocalServerOwnerOrSuper(user *models.User, gameServerID string) error {
	serverID := strings.TrimSpace(gameServerID)
	if serverID == "" {
		return invalidArg("game_server_id is required")
	}

	gameServer, errGetServer := xs.db.GetGameServerByID(serverID)
	if errGetServer != nil {
		if errors.Is(errGetServer, sql.ErrNoRows) {
			return notFoundErr()
		}
		log.Error().Err(errGetServer).Str("game_server_id", serverID).Msg("failed to fetch game server")
		return connect.NewError(connect.CodeInternal, errors.New("failed to authorize"))
	}

	if user.SuperUser || gameServer.UserID == user.ID {
		return nil
	}

	return permissionDenied("insufficient permissions")
}

func (xs *XylonaService) buildGameServerAccessGrant(assignment *models.UserRoleAssignment) (*xylona.GameServerAccessGrant, error) {
	role, errGetRole := xs.db.GetRoleByID(assignment.RoleID)
	if errGetRole != nil {
		log.Error().Err(errGetRole).Str("role_id", assignment.RoleID).Msg("failed to fetch role for access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	targetUser, errGetUser := xs.db.GetUserByID(assignment.UserID)
	if errGetUser != nil {
		log.Error().Err(errGetUser).Str("user_id", assignment.UserID).Msg("failed to fetch user for access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	grantedByUser, errGetGrantedBy := xs.db.GetUserByID(assignment.GrantedBy)
	if errGetGrantedBy != nil {
		log.Error().Err(errGetGrantedBy).Str("granted_by", assignment.GrantedBy).Msg("failed to fetch grantor for access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	gameServerID := ""
	if assignment.GameServerID.IsValue() && !assignment.GameServerID.IsNull() {
		gameServerID = assignment.GameServerID.MustGet()
	}

	return &xylona.GameServerAccessGrant{
		Id:                assignment.ID,
		UserId:            assignment.UserID,
		UserName:          targetUser.UserName,
		RoleId:            assignment.RoleID,
		RoleName:          role.Name,
		GameServerId:      gameServerID,
		GrantedByUserId:   assignment.GrantedBy,
		GrantedByUserName: grantedByUser.UserName,
		CreatedAt:         timestamppb.New(assignment.CreatedAt),
	}, nil
}
