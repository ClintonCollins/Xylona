package rpc

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// ListRemoteGameServerAccessGrants lists local direct access grants over federation.
func (fs FederationService) ListRemoteGameServerAccessGrants(ctx context.Context, request *connect.Request[xylona.FederationListGameServerAccessGrantsRequest]) (*connect.Response[xylona.FederationListGameServerAccessGrantsResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := strings.TrimSpace(request.Msg.GetGameServerId())
	if serverID == "" {
		return nil, invalidArg("game_server_id is required")
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	assignments, errGetAssignments := fs.db.GetUserRoleAssignmentsForServer(serverID)
	if errGetAssignments != nil {
		log.Error().Err(errGetAssignments).Str("server_id", serverID).Msg("failed to list remote game server access grants")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	resp := &xylona.FederationListGameServerAccessGrantsResponse{}
	for _, assignment := range assignments {
		grant, errBuild := fs.buildFederationGameServerAccessGrant(assignment)
		if errBuild != nil {
			return nil, errBuild
		}
		resp.Grants = append(resp.Grants, grant)
	}

	return connect.NewResponse(resp), nil
}

// GrantRemoteGameServerAccess creates a local direct access grant over federation.
func (fs FederationService) GrantRemoteGameServerAccess(ctx context.Context, request *connect.Request[xylona.FederationGrantGameServerAccessRequest]) (*connect.Response[xylona.FederationGrantGameServerAccessResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := strings.TrimSpace(request.Msg.GetGameServerId())
	targetUserID := strings.TrimSpace(request.Msg.GetUserId())
	roleID := strings.TrimSpace(request.Msg.GetRoleId())
	if serverID == "" || targetUserID == "" || roleID == "" {
		return nil, invalidArg("game_server_id, user_id, and role_id are required")
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	_, errGetServer := fs.db.GetGameServerByID(serverID)
	if errGetServer != nil {
		if errors.Is(errGetServer, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetServer).Str("server_id", serverID).Msg("failed to load game server for remote grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	_, errGetUser := fs.db.GetUserByID(targetUserID)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetUser).Str("user_id", targetUserID).Msg("failed to verify target user for remote grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	_, errGetRole := fs.db.GetRoleByID(roleID)
	if errGetRole != nil {
		if errors.Is(errGetRole, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetRole).Str("role_id", roleID).Msg("failed to verify role for remote grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	grantorUserID, errGrantor := fs.resolveGrantorUserIDForServer(request.Header(), serverID)
	if errGrantor != nil {
		if errors.Is(errGrantor, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGrantor).Str("server_id", serverID).Msg("failed to resolve grantor for remote grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		log.Error().Err(errID).Msg("failed to generate remote game server access grant id")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	errCreateGrant := fs.db.CreateUserRoleAssignment(newID.String(), targetUserID, roleID, serverID, grantorUserID)
	if errCreateGrant != nil {
		if isSQLiteUniqueConstraintError(errCreateGrant) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("grant already exists"))
		}
		log.Error().
			Err(errCreateGrant).
			Str("server_id", serverID).
			Str("user_id", targetUserID).
			Str("role_id", roleID).
			Msg("failed to create remote game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	assignment, errGetAssignment := fs.db.GetUserRoleAssignmentByID(newID.String())
	if errGetAssignment != nil {
		log.Error().Err(errGetAssignment).Str("grant_id", newID.String()).Msg("failed to fetch created remote game server grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	grant, errBuild := fs.buildFederationGameServerAccessGrant(assignment)
	if errBuild != nil {
		return nil, errBuild
	}

	return connect.NewResponse(&xylona.FederationGrantGameServerAccessResponse{
		Grant: grant,
	}), nil
}

// RevokeRemoteGameServerAccess removes a local direct access grant over federation.
func (fs FederationService) RevokeRemoteGameServerAccess(ctx context.Context, request *connect.Request[xylona.FederationRevokeGameServerAccessRequest]) (*connect.Response[xylona.FederationRevokeGameServerAccessResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	grantID := strings.TrimSpace(request.Msg.GetGrantId())
	if grantID == "" {
		return nil, invalidArg("grant_id is required")
	}

	assignment, errGetAssignment := fs.db.GetUserRoleAssignmentByID(grantID)
	if errGetAssignment != nil {
		if errors.Is(errGetAssignment, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetAssignment).Str("grant_id", grantID).Msg("failed to fetch remote game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
	}
	if !assignment.GameServerID.IsValue() || assignment.GameServerID.IsNull() {
		return nil, invalidArg("grant is not scoped to a game server")
	}

	serverID := assignment.GameServerID.MustGet()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	errDeleteGrant := fs.db.DeleteUserRoleAssignment(grantID)
	if errDeleteGrant != nil {
		log.Error().Err(errDeleteGrant).Str("grant_id", grantID).Msg("failed to revoke remote game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
	}

	return connect.NewResponse(&xylona.FederationRevokeGameServerAccessResponse{}), nil
}

// ListRemoteFederatedAccessGrants lists local federated access grants over federation.
func (fs FederationService) ListRemoteFederatedAccessGrants(ctx context.Context, request *connect.Request[xylona.FederationListFederatedAccessGrantsRequest]) (*connect.Response[xylona.FederationListFederatedAccessGrantsResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := strings.TrimSpace(request.Msg.GetGameServerId())
	if serverID == "" {
		return nil, invalidArg("game_server_id is required")
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	grants, errGetGrants := fs.db.GetFederatedAccessGrantsForServer(serverID)
	if errGetGrants != nil {
		log.Error().Err(errGetGrants).Str("server_id", serverID).Msg("failed to list remote federated access grants")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
	}

	resp := &xylona.FederationListFederatedAccessGrantsResponse{}
	for _, grantModel := range grants {
		grantInfo, errBuild := fs.buildFederationFederatedAccessGrantInfo(grantModel)
		if errBuild != nil {
			return nil, errBuild
		}
		resp.Grants = append(resp.Grants, grantInfo)
	}

	return connect.NewResponse(resp), nil
}

// GrantRemoteFederatedAccess creates a local federated access grant over federation.
func (fs FederationService) GrantRemoteFederatedAccess(ctx context.Context, request *connect.Request[xylona.FederationGrantFederatedAccessRequest]) (*connect.Response[xylona.FederationGrantFederatedAccessResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := strings.TrimSpace(request.Msg.GetGameServerId())
	remoteNodeID := strings.TrimSpace(request.Msg.GetRemoteNodeId())
	remoteUserID := strings.TrimSpace(request.Msg.GetRemoteUserId())
	roleID := strings.TrimSpace(request.Msg.GetRoleId())
	if serverID == "" || remoteNodeID == "" || remoteUserID == "" || roleID == "" {
		return nil, invalidArg("game_server_id, remote_node_id, remote_user_id, and role_id are required")
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	_, errGetServer := fs.db.GetGameServerByID(serverID)
	if errGetServer != nil {
		if errors.Is(errGetServer, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetServer).Str("server_id", serverID).Msg("failed to load game server for remote federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	_, errGetNode := fs.db.GetRemoteNodeByID(remoteNodeID)
	if errGetNode != nil {
		if errors.Is(errGetNode, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetNode).Str("remote_node_id", remoteNodeID).Msg("failed to verify remote node for federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	_, errGetRole := fs.db.GetRoleByID(roleID)
	if errGetRole != nil {
		if errors.Is(errGetRole, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetRole).Str("role_id", roleID).Msg("failed to verify role for federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	remoteUserName := strings.TrimSpace(request.Msg.GetRemoteUserName())
	if remoteUserName == "" {
		remoteUserName = remoteUserID
	}

	grantorUserID, errGrantor := fs.resolveGrantorUserIDForServer(request.Header(), serverID)
	if errGrantor != nil {
		if errors.Is(errGrantor, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGrantor).Str("server_id", serverID).Msg("failed to resolve grantor for remote federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		log.Error().Err(errID).Msg("failed to generate remote federated access grant id")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	errCreateGrant := fs.db.CreateFederatedAccessGrant(
		newID.String(),
		serverID,
		remoteNodeID,
		remoteUserID,
		remoteUserName,
		roleID,
		grantorUserID,
	)
	if errCreateGrant != nil {
		if isSQLiteUniqueConstraintError(errCreateGrant) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("grant already exists"))
		}
		log.Error().
			Err(errCreateGrant).
			Str("server_id", serverID).
			Str("remote_node_id", remoteNodeID).
			Str("remote_user_id", remoteUserID).
			Str("role_id", roleID).
			Msg("failed to create remote federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	grantModel, errGetGrant := fs.db.GetFederatedAccessGrantByID(newID.String())
	if errGetGrant != nil {
		log.Error().Err(errGetGrant).Str("grant_id", newID.String()).Msg("failed to fetch created remote federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	grantInfo, errBuild := fs.buildFederationFederatedAccessGrantInfo(grantModel)
	if errBuild != nil {
		return nil, errBuild
	}

	return connect.NewResponse(&xylona.FederationGrantFederatedAccessResponse{
		Grant: grantInfo,
	}), nil
}

// RevokeRemoteFederatedAccess removes a local federated access grant over federation.
func (fs FederationService) RevokeRemoteFederatedAccess(ctx context.Context, request *connect.Request[xylona.FederationRevokeFederatedAccessRequest]) (*connect.Response[xylona.FederationRevokeFederatedAccessResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	grantID := strings.TrimSpace(request.Msg.GetGrantId())
	if grantID == "" {
		return nil, invalidArg("grant_id is required")
	}

	grantModel, errGetGrant := fs.db.GetFederatedAccessGrantByID(grantID)
	if errGetGrant != nil {
		if errors.Is(errGetGrant, sql.ErrNoRows) {
			return nil, notFoundErr()
		}
		log.Error().Err(errGetGrant).Str("grant_id", grantID).Msg("failed to fetch remote federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke federated access"))
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", grantModel.GameServerID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	errDeleteGrant := fs.db.DeleteFederatedAccessGrant(grantID)
	if errDeleteGrant != nil {
		log.Error().Err(errDeleteGrant).Str("grant_id", grantID).Msg("failed to revoke remote federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke federated access"))
	}

	return connect.NewResponse(&xylona.FederationRevokeFederatedAccessResponse{}), nil
}

func (fs FederationService) buildFederationGameServerAccessGrant(assignment *models.UserRoleAssignment) (*xylona.FederationGameServerAccessGrant, error) {
	role, errGetRole := fs.db.GetRoleByID(assignment.RoleID)
	if errGetRole != nil {
		log.Error().Err(errGetRole).Str("role_id", assignment.RoleID).Msg("failed to fetch role for federation game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	targetUser, errGetUser := fs.db.GetUserByID(assignment.UserID)
	if errGetUser != nil {
		log.Error().Err(errGetUser).Str("user_id", assignment.UserID).Msg("failed to fetch user for federation game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	grantedByUser, errGetGrantedBy := fs.db.GetUserByID(assignment.GrantedBy)
	if errGetGrantedBy != nil {
		log.Error().Err(errGetGrantedBy).Str("granted_by", assignment.GrantedBy).Msg("failed to fetch grantor for federation game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	gameServerID := ""
	if assignment.GameServerID.IsValue() && !assignment.GameServerID.IsNull() {
		gameServerID = assignment.GameServerID.MustGet()
	}

	return &xylona.FederationGameServerAccessGrant{
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

func (fs FederationService) buildFederationFederatedAccessGrantInfo(grantModel *models.FederatedAccessGrant) (*xylona.FederationFederatedAccessGrantInfo, error) {
	role, errGetRole := fs.db.GetRoleByID(grantModel.RoleID)
	if errGetRole != nil {
		log.Error().Err(errGetRole).Str("role_id", grantModel.RoleID).Msg("failed to fetch role for federation federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
	}

	grantedByUser, errGetGrantedBy := fs.db.GetUserByID(grantModel.GrantedBy)
	if errGetGrantedBy != nil {
		log.Error().Err(errGetGrantedBy).Str("granted_by", grantModel.GrantedBy).Msg("failed to fetch grantor for federation federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
	}

	nodeName := ""
	node, errGetNode := fs.db.GetRemoteNodeByID(grantModel.RemoteNodeID)
	if errGetNode == nil && node != nil {
		nodeName = node.Name
	}

	return &xylona.FederationFederatedAccessGrantInfo{
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
