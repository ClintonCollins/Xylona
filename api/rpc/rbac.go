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

func (xs *XylonaService) ListRoles(ctx context.Context, request *connect.Request[xylona.ListRolesRequest]) (*connect.Response[xylona.ListRolesResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
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

func (xs *XylonaService) ListPermissions(ctx context.Context, request *connect.Request[xylona.ListPermissionsRequest]) (*connect.Response[xylona.ListPermissionsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
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

func (xs *XylonaService) CreateRole(ctx context.Context, request *connect.Request[xylona.CreateRoleRequest]) (*connect.Response[xylona.CreateRoleResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	name := strings.TrimSpace(request.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
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
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("one or more permission IDs are invalid"))
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

func (xs *XylonaService) DeleteRole(ctx context.Context, request *connect.Request[xylona.DeleteRoleRequest]) (*connect.Response[xylona.DeleteRoleResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	roleID := strings.TrimSpace(request.Msg.GetRoleId())
	if roleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("role_id is required"))
	}

	errDeleteRole := xs.db.DeleteRole(roleID)
	if errDeleteRole != nil {
		if errors.Is(errDeleteRole, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
		}
		if errors.Is(errDeleteRole, db.ErrRoleIsSystem) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("cannot delete system role"))
		}
		log.Error().Err(errDeleteRole).Str("role_id", roleID).Msg("failed to delete role")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete role"))
	}

	return connect.NewResponse(&xylona.DeleteRoleResponse{}), nil
}

func (xs *XylonaService) ListGameServerAccessGrants(ctx context.Context, request *connect.Request[xylona.ListGameServerAccessGrantsRequest]) (*connect.Response[xylona.ListGameServerAccessGrantsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_server_id is required"))
	}

	return dispatchGameServerRequest(
		xs,
		gameServerID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.ListGameServerAccessGrantsResponse], error) {
			if !user.SuperUser && gameServer.UserID != user.ID {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
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
		},
		func() (*connect.Response[xylona.ListGameServerAccessGrantsResponse], error) {
			return xs.listRemoteGameServerAccessGrants(ctx, gameServerID, user)
		},
	)
}

func (xs *XylonaService) GrantGameServerAccess(ctx context.Context, request *connect.Request[xylona.GrantGameServerAccessRequest]) (*connect.Response[xylona.GrantGameServerAccessResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_server_id is required"))
	}
	userID := strings.TrimSpace(request.Msg.GetUserId())
	roleID := strings.TrimSpace(request.Msg.GetRoleId())
	if userID == "" || roleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id and role_id are required"))
	}

	return dispatchGameServerRequest(
		xs,
		gameServerID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GrantGameServerAccessResponse], error) {
			if !user.SuperUser && gameServer.UserID != user.ID {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
			}

			_, errGetTargetUser := xs.db.GetUserByID(userID)
			if errGetTargetUser != nil {
				if errors.Is(errGetTargetUser, sql.ErrNoRows) {
					return nil, connect.NewError(connect.CodeNotFound, errors.New("target user not found"))
				}
				log.Error().Err(errGetTargetUser).Str("user_id", userID).Msg("failed to verify target user")
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
			}

			_, errGetRole := xs.db.GetRoleByID(roleID)
			if errGetRole != nil {
				if errors.Is(errGetRole, sql.ErrNoRows) {
					return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
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
		},
		func() (*connect.Response[xylona.GrantGameServerAccessResponse], error) {
			return xs.grantRemoteGameServerAccess(ctx, gameServerID, userID, roleID, user)
		},
	)
}

func (xs *XylonaService) RevokeGameServerAccess(ctx context.Context, request *connect.Request[xylona.RevokeGameServerAccessRequest]) (*connect.Response[xylona.RevokeGameServerAccessResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	grantID := strings.TrimSpace(request.Msg.GetGrantId())
	if grantID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("grant_id is required"))
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID != "" {
		return dispatchGameServerRequest(
			xs,
			gameServerID,
			func(gameServer *models.GameServer) (*connect.Response[xylona.RevokeGameServerAccessResponse], error) {
				if !user.SuperUser && gameServer.UserID != user.ID {
					return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
				}

				assignment, errGetAssignment := xs.db.GetUserRoleAssignmentByID(grantID)
				if errGetAssignment != nil {
					if errors.Is(errGetAssignment, sql.ErrNoRows) {
						return nil, connect.NewError(connect.CodeNotFound, errors.New("grant not found"))
					}
					log.Error().Err(errGetAssignment).Str("grant_id", grantID).Msg("failed to fetch local access grant")
					return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
				}

				if !assignment.GameServerID.IsValue() || assignment.GameServerID.IsNull() {
					return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("grant is not scoped to a game server"))
				}
				if assignment.GameServerID.MustGet() != gameServer.ID {
					return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("grant does not belong to game_server_id"))
				}

				errDelete := xs.db.DeleteUserRoleAssignment(grantID)
				if errDelete != nil {
					log.Error().Err(errDelete).Str("grant_id", grantID).Msg("failed to delete local access grant")
					return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
				}

				return connect.NewResponse(&xylona.RevokeGameServerAccessResponse{}), nil
			},
			func() (*connect.Response[xylona.RevokeGameServerAccessResponse], error) {
				return xs.revokeRemoteGameServerAccess(ctx, gameServerID, grantID, user)
			},
		)
	}

	assignment, errGetAssignment := xs.db.GetUserRoleAssignmentByID(grantID)
	if errGetAssignment != nil {
		if errors.Is(errGetAssignment, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("grant not found"))
		}
		log.Error().Err(errGetAssignment).Str("grant_id", grantID).Msg("failed to fetch local access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
	}
	if !assignment.GameServerID.IsValue() || assignment.GameServerID.IsNull() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("grant is not scoped to a game server"))
	}
	_, errServer := xs.requireLocalServerOwnerOrSuper(user, assignment.GameServerID.MustGet())
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

func (xs *XylonaService) ListRemoteNodeUsers(ctx context.Context, request *connect.Request[xylona.ListRemoteNodeUsersRequest]) (*connect.Response[xylona.ListRemoteNodeUsersResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	searchTerm := strings.TrimSpace(request.Msg.GetSearch())

	// Non-super users must provide a search term to prevent full user list snooping.
	if !user.SuperUser && searchTerm == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("search term is required"))
	}

	nodeID := strings.TrimSpace(request.Msg.GetNodeId())
	if nodeID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("node_id is required"))
	}

	node, errNode := xs.db.GetRemoteNodeByID(nodeID)
	if errNode != nil {
		if errors.Is(errNode, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("node not found"))
		}
		log.Error().Err(errNode).Str("node_id", nodeID).Msg("failed to get remote node")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list remote users"))
	}

	client, errClient := xs.newRemoteFederationClient(node)
	if errClient != nil {
		log.Error().Err(errClient).Str("node_id", node.ID).Msg("failed to create remote federation client")
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("remote federation trust is not configured"))
	}

	fedReq := connect.NewRequest(&xylona.FederationListUserSummariesRequest{
		Limit: 1000,
	})
	fedResp, errFetch := client.ListUserSummaries(ctx, fedReq)
	if errFetch != nil {
		log.Error().Err(errFetch).Str("node_id", node.ID).Msg("failed to fetch remote users")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to fetch remote users"))
	}

	searchLower := strings.ToLower(searchTerm)

	resp := &xylona.ListRemoteNodeUsersResponse{}
	for _, remoteUser := range fedResp.Msg.GetUsers() {
		// If a search term is provided, filter results by username or user ID.
		if searchLower != "" {
			userNameLower := strings.ToLower(remoteUser.GetUserName())
			userIDLower := strings.ToLower(remoteUser.GetUserId())
			if !strings.Contains(userNameLower, searchLower) && !strings.Contains(userIDLower, searchLower) {
				continue
			}
		}

		resp.Users = append(resp.Users, &xylona.RemoteUser{
			UserId:    remoteUser.GetUserId(),
			UserName:  remoteUser.GetUserName(),
			Email:     remoteUser.GetEmail(),
			FirstName: remoteUser.GetFirstName(),
			LastName:  remoteUser.GetLastName(),
			NodeId:    node.ID,
			NodeName:  node.Name,
		})
	}

	return connect.NewResponse(resp), nil
}

func (xs *XylonaService) ListFederatedAccessGrants(ctx context.Context, request *connect.Request[xylona.ListFederatedAccessGrantsRequest]) (*connect.Response[xylona.ListFederatedAccessGrantsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_server_id is required"))
	}

	return dispatchGameServerRequest(
		xs,
		gameServerID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.ListFederatedAccessGrantsResponse], error) {
			if !user.SuperUser && gameServer.UserID != user.ID {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
			}

			grants, errGetGrants := xs.db.GetFederatedAccessGrantsForServer(gameServer.ID)
			if errGetGrants != nil {
				log.Error().Err(errGetGrants).Str("game_server_id", gameServer.ID).Msg("failed to list federated access grants")
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
			}

			resp := &xylona.ListFederatedAccessGrantsResponse{}
			for _, grantModel := range grants {
				grantInfo, errBuild := xs.buildFederatedAccessGrantInfo(grantModel)
				if errBuild != nil {
					return nil, errBuild
				}
				resp.Grants = append(resp.Grants, grantInfo)
			}

			return connect.NewResponse(resp), nil
		},
		func() (*connect.Response[xylona.ListFederatedAccessGrantsResponse], error) {
			return xs.listRemoteFederatedAccessGrants(ctx, gameServerID, user)
		},
	)
}

func (xs *XylonaService) GrantFederatedAccess(ctx context.Context, request *connect.Request[xylona.GrantFederatedAccessRequest]) (*connect.Response[xylona.GrantFederatedAccessResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_server_id is required"))
	}
	remoteNodeID := strings.TrimSpace(request.Msg.GetRemoteNodeId())
	remoteUserID := strings.TrimSpace(request.Msg.GetRemoteUserId())
	roleID := strings.TrimSpace(request.Msg.GetRoleId())
	if remoteNodeID == "" || remoteUserID == "" || roleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("remote_node_id, remote_user_id, and role_id are required"))
	}
	remoteUserName := strings.TrimSpace(request.Msg.GetRemoteUserName())

	return dispatchGameServerRequest(
		xs,
		gameServerID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GrantFederatedAccessResponse], error) {
			if !user.SuperUser && gameServer.UserID != user.ID {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
			}

			_, errGetNode := xs.db.GetRemoteNodeByID(remoteNodeID)
			if errGetNode != nil {
				if errors.Is(errGetNode, sql.ErrNoRows) {
					return nil, connect.NewError(connect.CodeNotFound, errors.New("remote node not found"))
				}
				log.Error().Err(errGetNode).Str("node_id", remoteNodeID).Msg("failed to verify remote node")
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
			}

			_, errGetRole := xs.db.GetRoleByID(roleID)
			if errGetRole != nil {
				if errors.Is(errGetRole, sql.ErrNoRows) {
					return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
				}
				log.Error().Err(errGetRole).Str("role_id", roleID).Msg("failed to verify role")
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
			}

			grantRemoteUserName := remoteUserName
			if grantRemoteUserName == "" {
				grantRemoteUserName = remoteUserID
			}

			newID, errID := helpers.GenerateUniqueID()
			if errID != nil {
				log.Error().Err(errID).Msg("failed to generate federated access grant id")
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
			}

			errCreateGrant := xs.db.CreateFederatedAccessGrant(
				newID.String(),
				gameServer.ID,
				remoteNodeID,
				remoteUserID,
				grantRemoteUserName,
				roleID,
				user.ID,
			)
			if errCreateGrant != nil {
				if isSQLiteUniqueConstraintError(errCreateGrant) {
					return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("grant already exists"))
				}
				log.Error().Err(errCreateGrant).
					Str("game_server_id", gameServer.ID).
					Str("remote_node_id", remoteNodeID).
					Str("remote_user_id", remoteUserID).
					Str("role_id", roleID).
					Msg("failed to create federated access grant")
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
			}

			grantModel, errGetGrant := xs.db.GetFederatedAccessGrantByID(newID.String())
			if errGetGrant != nil {
				log.Error().Err(errGetGrant).Str("grant_id", newID.String()).Msg("failed to fetch federated access grant")
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
			}

			grantInfo, errBuildGrant := xs.buildFederatedAccessGrantInfo(grantModel)
			if errBuildGrant != nil {
				return nil, errBuildGrant
			}

			return connect.NewResponse(&xylona.GrantFederatedAccessResponse{
				Grant: grantInfo,
			}), nil
		},
		func() (*connect.Response[xylona.GrantFederatedAccessResponse], error) {
			return xs.grantRemoteFederatedAccess(ctx, gameServerID, remoteNodeID, remoteUserID, remoteUserName, roleID, user)
		},
	)
}

func (xs *XylonaService) RevokeFederatedAccess(ctx context.Context, request *connect.Request[xylona.RevokeFederatedAccessRequest]) (*connect.Response[xylona.RevokeFederatedAccessResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	grantID := strings.TrimSpace(request.Msg.GetGrantId())
	if grantID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("grant_id is required"))
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID != "" {
		return dispatchGameServerRequest(
			xs,
			gameServerID,
			func(gameServer *models.GameServer) (*connect.Response[xylona.RevokeFederatedAccessResponse], error) {
				if !user.SuperUser && gameServer.UserID != user.ID {
					return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
				}

				grantModel, errGetGrant := xs.db.GetFederatedAccessGrantByID(grantID)
				if errGetGrant != nil {
					if errors.Is(errGetGrant, sql.ErrNoRows) {
						return nil, connect.NewError(connect.CodeNotFound, errors.New("grant not found"))
					}
					log.Error().Err(errGetGrant).Str("grant_id", grantID).Msg("failed to fetch federated access grant")
					return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke federated access"))
				}
				if grantModel.GameServerID != gameServer.ID {
					return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("grant does not belong to game_server_id"))
				}

				errDelete := xs.db.DeleteFederatedAccessGrant(grantID)
				if errDelete != nil {
					log.Error().Err(errDelete).Str("grant_id", grantID).Msg("failed to revoke federated access grant")
					return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke federated access"))
				}
				return connect.NewResponse(&xylona.RevokeFederatedAccessResponse{}), nil
			},
			func() (*connect.Response[xylona.RevokeFederatedAccessResponse], error) {
				return xs.revokeRemoteFederatedAccess(ctx, gameServerID, grantID, user)
			},
		)
	}

	grantModel, errGetGrant := xs.db.GetFederatedAccessGrantByID(grantID)
	if errGetGrant != nil {
		if errors.Is(errGetGrant, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("grant not found"))
		}
		log.Error().Err(errGetGrant).Str("grant_id", grantID).Msg("failed to fetch federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke federated access"))
	}
	_, errServer := xs.requireLocalServerOwnerOrSuper(user, grantModel.GameServerID)
	if errServer != nil {
		return nil, errServer
	}
	errDelete := xs.db.DeleteFederatedAccessGrant(grantID)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("grant_id", grantID).Msg("failed to revoke federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke federated access"))
	}
	return connect.NewResponse(&xylona.RevokeFederatedAccessResponse{}), nil
}

func (xs *XylonaService) listRemoteGameServerAccessGrants(ctx context.Context, gameServerID string, actingUser *models.User) (*connect.Response[xylona.ListGameServerAccessGrantsResponse], error) {
	node, _, errGetRemote := xs.getRemoteNodeForServer(gameServerID)
	if errGetRemote != nil {
		return nil, errGetRemote
	}

	client, errClient := xs.remoteFederationClient(node, gameServerID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationListGameServerAccessGrantsRequest{
		GameServerId: gameServerID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to apply federation identity headers for list remote game server access grants")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	resp, errList := client.ListRemoteGameServerAccessGrants(ctx, req)
	if errList != nil {
		log.Error().Err(errList).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to list remote game server access grants")
		if code := connect.CodeOf(errList); code != connect.CodeUnknown {
			return nil, connect.NewError(code, errors.New(errList.Error()))
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to list access grants"))
	}

	localResp := &xylona.ListGameServerAccessGrantsResponse{}
	for _, grant := range resp.Msg.GetGrants() {
		localResp.Grants = append(localResp.Grants, federationGameServerAccessGrantToXylona(grant))
	}

	return connect.NewResponse(localResp), nil
}

func (xs *XylonaService) grantRemoteGameServerAccess(ctx context.Context, gameServerID string, userID string, roleID string, actingUser *models.User) (*connect.Response[xylona.GrantGameServerAccessResponse], error) {
	node, _, errGetRemote := xs.getRemoteNodeForServer(gameServerID)
	if errGetRemote != nil {
		return nil, errGetRemote
	}

	client, errClient := xs.remoteFederationClient(node, gameServerID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationGrantGameServerAccessRequest{
		GameServerId: gameServerID,
		UserId:       userID,
		RoleId:       roleID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to apply federation identity headers for grant remote game server access")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	resp, errGrant := client.GrantRemoteGameServerAccess(ctx, req)
	if errGrant != nil {
		log.Error().Err(errGrant).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to grant remote game server access")
		if code := connect.CodeOf(errGrant); code != connect.CodeUnknown {
			return nil, connect.NewError(code, errors.New(errGrant.Error()))
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to grant access"))
	}

	return connect.NewResponse(&xylona.GrantGameServerAccessResponse{
		Grant: federationGameServerAccessGrantToXylona(resp.Msg.GetGrant()),
	}), nil
}

func (xs *XylonaService) revokeRemoteGameServerAccess(ctx context.Context, gameServerID string, grantID string, actingUser *models.User) (*connect.Response[xylona.RevokeGameServerAccessResponse], error) {
	node, _, errGetRemote := xs.getRemoteNodeForServer(gameServerID)
	if errGetRemote != nil {
		return nil, errGetRemote
	}

	client, errClient := xs.remoteFederationClient(node, gameServerID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationRevokeGameServerAccessRequest{
		GrantId: grantID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to apply federation identity headers for revoke remote game server access")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
	}

	_, errRevoke := client.RevokeRemoteGameServerAccess(ctx, req)
	if errRevoke != nil {
		log.Error().Err(errRevoke).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to revoke remote game server access")
		if code := connect.CodeOf(errRevoke); code != connect.CodeUnknown {
			return nil, connect.NewError(code, errors.New(errRevoke.Error()))
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to revoke access"))
	}

	return connect.NewResponse(&xylona.RevokeGameServerAccessResponse{}), nil
}

func (xs *XylonaService) listRemoteFederatedAccessGrants(ctx context.Context, gameServerID string, actingUser *models.User) (*connect.Response[xylona.ListFederatedAccessGrantsResponse], error) {
	node, _, errGetRemote := xs.getRemoteNodeForServer(gameServerID)
	if errGetRemote != nil {
		return nil, errGetRemote
	}

	client, errClient := xs.remoteFederationClient(node, gameServerID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationListFederatedAccessGrantsRequest{
		GameServerId: gameServerID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to apply federation identity headers for list remote federated access grants")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
	}

	resp, errList := client.ListRemoteFederatedAccessGrants(ctx, req)
	if errList != nil {
		log.Error().Err(errList).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to list remote federated access grants")
		if code := connect.CodeOf(errList); code != connect.CodeUnknown {
			return nil, connect.NewError(code, errors.New(errList.Error()))
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to list federated grants"))
	}

	localResp := &xylona.ListFederatedAccessGrantsResponse{}
	for _, grant := range resp.Msg.GetGrants() {
		localResp.Grants = append(localResp.Grants, federationFederatedAccessGrantInfoToXylona(grant))
	}

	return connect.NewResponse(localResp), nil
}

func (xs *XylonaService) grantRemoteFederatedAccess(
	ctx context.Context,
	gameServerID string,
	remoteNodeID string,
	remoteUserID string,
	remoteUserName string,
	roleID string,
	actingUser *models.User,
) (*connect.Response[xylona.GrantFederatedAccessResponse], error) {
	node, _, errGetRemote := xs.getRemoteNodeForServer(gameServerID)
	if errGetRemote != nil {
		return nil, errGetRemote
	}

	client, errClient := xs.remoteFederationClient(node, gameServerID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationGrantFederatedAccessRequest{
		GameServerId:   gameServerID,
		RemoteNodeId:   remoteNodeID,
		RemoteUserId:   remoteUserID,
		RemoteUserName: remoteUserName,
		RoleId:         roleID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to apply federation identity headers for grant remote federated access")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	resp, errGrant := client.GrantRemoteFederatedAccess(ctx, req)
	if errGrant != nil {
		log.Error().Err(errGrant).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to grant remote federated access")
		if code := connect.CodeOf(errGrant); code != connect.CodeUnknown {
			return nil, connect.NewError(code, errors.New(errGrant.Error()))
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to grant federated access"))
	}

	return connect.NewResponse(&xylona.GrantFederatedAccessResponse{
		Grant: federationFederatedAccessGrantInfoToXylona(resp.Msg.GetGrant()),
	}), nil
}

func (xs *XylonaService) revokeRemoteFederatedAccess(ctx context.Context, gameServerID string, grantID string, actingUser *models.User) (*connect.Response[xylona.RevokeFederatedAccessResponse], error) {
	node, _, errGetRemote := xs.getRemoteNodeForServer(gameServerID)
	if errGetRemote != nil {
		return nil, errGetRemote
	}

	client, errClient := xs.remoteFederationClient(node, gameServerID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationRevokeFederatedAccessRequest{
		GrantId: grantID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to apply federation identity headers for revoke remote federated access")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke federated access"))
	}

	_, errRevoke := client.RevokeRemoteFederatedAccess(ctx, req)
	if errRevoke != nil {
		log.Error().Err(errRevoke).Str("server_id", gameServerID).Str("node", node.Name).Msg("failed to revoke remote federated access")
		if code := connect.CodeOf(errRevoke); code != connect.CodeUnknown {
			return nil, connect.NewError(code, errors.New(errRevoke.Error()))
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to revoke federated access"))
	}

	return connect.NewResponse(&xylona.RevokeFederatedAccessResponse{}), nil
}

func federationGameServerAccessGrantToXylona(grant *xylona.FederationGameServerAccessGrant) *xylona.GameServerAccessGrant {
	if grant == nil {
		return nil
	}

	return &xylona.GameServerAccessGrant{
		Id:                grant.Id,
		UserId:            grant.UserId,
		UserName:          grant.UserName,
		RoleId:            grant.RoleId,
		RoleName:          grant.RoleName,
		GameServerId:      grant.GameServerId,
		GrantedByUserId:   grant.GrantedByUserId,
		GrantedByUserName: grant.GrantedByUserName,
		CreatedAt:         grant.CreatedAt,
	}
}

func federationFederatedAccessGrantInfoToXylona(grant *xylona.FederationFederatedAccessGrantInfo) *xylona.FederatedAccessGrantInfo {
	if grant == nil {
		return nil
	}

	return &xylona.FederatedAccessGrantInfo{
		Id:                grant.Id,
		GameServerId:      grant.GameServerId,
		RemoteNodeId:      grant.RemoteNodeId,
		RemoteNodeName:    grant.RemoteNodeName,
		RemoteUserId:      grant.RemoteUserId,
		RemoteUserName:    grant.RemoteUserName,
		RoleId:            grant.RoleId,
		RoleName:          grant.RoleName,
		GrantedByUserId:   grant.GrantedByUserId,
		GrantedByUserName: grant.GrantedByUserName,
		CreatedAt:         grant.CreatedAt,
	}
}

func (xs *XylonaService) requireLocalServerOwnerOrSuper(user *models.User, gameServerID string) (*models.GameServer, error) {
	serverID := strings.TrimSpace(gameServerID)
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_server_id is required"))
	}

	gameServer, errGetServer := xs.db.GetGameServerByID(serverID)
	if errGetServer != nil {
		if errors.Is(errGetServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game server not found"))
		}
		log.Error().Err(errGetServer).Str("game_server_id", serverID).Msg("failed to fetch game server")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize"))
	}

	if user.SuperUser || gameServer.UserID == user.ID {
		return gameServer, nil
	}

	return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
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

func (xs *XylonaService) buildFederatedAccessGrantInfo(grantModel *models.FederatedAccessGrant) (*xylona.FederatedAccessGrantInfo, error) {
	role, errGetRole := xs.db.GetRoleByID(grantModel.RoleID)
	if errGetRole != nil {
		log.Error().Err(errGetRole).Str("role_id", grantModel.RoleID).Msg("failed to fetch role for federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
	}

	grantedByUser, errGetGrantedBy := xs.db.GetUserByID(grantModel.GrantedBy)
	if errGetGrantedBy != nil {
		log.Error().Err(errGetGrantedBy).Str("granted_by", grantModel.GrantedBy).Msg("failed to fetch grantor for federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
	}

	nodeName := ""
	node, errGetNode := xs.db.GetRemoteNodeByID(grantModel.RemoteNodeID)
	if errGetNode == nil && node != nil {
		nodeName = node.Name
	}

	return &xylona.FederatedAccessGrantInfo{
		Id:                grantModel.ID,
		GameServerId:      grantModel.GameServerID,
		RemoteNodeId:      grantModel.RemoteNodeID,
		RemoteNodeName:    nodeName,
		RemoteUserId:      grantModel.RemoteUserID,
		RemoteUserName:    grantModel.RemoteUserName,
		RoleId:            grantModel.RoleID,
		RoleName:          role.Name,
		GrantedByUserId:   grantModel.GrantedBy,
		GrantedByUserName: grantedByUser.UserName,
		CreatedAt:         timestamppb.New(grantModel.CreatedAt),
	}, nil
}

func isSQLiteUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

func isSQLiteForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	errLower := strings.ToLower(err.Error())
	return strings.Contains(errLower, "foreign key constraint failed")
}
