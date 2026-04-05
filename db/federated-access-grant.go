package db

import (
	"fmt"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetFederatedAccessGrantsForServer returns all federated grants for a server.
func (c *Connection) GetFederatedAccessGrantsForServer(gameServerID string) ([]*models.FederatedAccessGrant, error) {
	grants, errGetGrants := models.FederatedAccessGrants.Query(
		models.SelectWhere.FederatedAccessGrants.GameServerID.EQ(gameServerID),
	).All(c.ctx, c.DB)
	if errGetGrants != nil {
		return nil, fmt.Errorf("get federated access grants for server: %w", errGetGrants)
	}

	return grants, nil
}

// GetFederatedAccessGrant returns matching federated grants for a remote user.
func (c *Connection) GetFederatedAccessGrant(gameServerID string, remoteNodeID string, remoteUserID string) ([]*models.FederatedAccessGrant, error) {
	grants, errGetGrant := models.FederatedAccessGrants.Query(
		models.SelectWhere.FederatedAccessGrants.GameServerID.EQ(gameServerID),
		models.SelectWhere.FederatedAccessGrants.RemoteNodeID.EQ(remoteNodeID),
		models.SelectWhere.FederatedAccessGrants.RemoteUserID.EQ(remoteUserID),
	).All(c.ctx, c.DB)
	if errGetGrant != nil {
		return nil, fmt.Errorf("get federated access grant: %w", errGetGrant)
	}

	return grants, nil
}

// CreateFederatedAccessGrant creates a federated access grant record.
func (c *Connection) CreateFederatedAccessGrant(id string, gameServerID string, remoteNodeID string, remoteUserID string, remoteUserName string, roleID string, grantedBy string) error {
	setter := &models.FederatedAccessGrantSetter{
		ID:             omit.From(id),
		GameServerID:   omit.From(gameServerID),
		RemoteNodeID:   omit.From(remoteNodeID),
		RemoteUserID:   omit.From(remoteUserID),
		RemoteUserName: omit.From(remoteUserName),
		RoleID:         omit.From(roleID),
		GrantedBy:      omit.From(grantedBy),
	}

	_, errCreateGrant := models.FederatedAccessGrants.Insert(setter).One(c.ctx, c.DB)
	if errCreateGrant != nil {
		return fmt.Errorf("create federated access grant: %w", errCreateGrant)
	}
	return nil
}

// DeleteFederatedAccessGrant deletes a federated access grant by ID.
func (c *Connection) DeleteFederatedAccessGrant(id string) error {
	grant, errGetGrant := models.FindFederatedAccessGrant(c.ctx, c.DB, id)
	if errGetGrant != nil {
		return fmt.Errorf("get federated access grant for delete: %w", errGetGrant)
	}

	errDeleteGrant := models.FederatedAccessGrantSlice{grant}.DeleteAll(c.ctx, c.DB)
	if errDeleteGrant != nil {
		return fmt.Errorf("delete federated access grant: %w", errDeleteGrant)
	}

	return nil
}

// FederatedUserHasPermissionOnServer checks a federated user's server permission.
func (c *Connection) FederatedUserHasPermissionOnServer(remoteNodeID string, remoteUserID string, gameServerID string, permissionID string) (bool, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(
		c.ctx,
		`SELECT COUNT(*)
		FROM federated_access_grant fag
		JOIN role_permission rp ON fag.role_id = rp.role_id
		WHERE fag.remote_node_id = ? AND fag.remote_user_id = ?
		AND fag.game_server_id = ?
		AND rp.permission_id = ?`,
		remoteNodeID, remoteUserID, gameServerID, permissionID,
	).Scan(&count)
	if errQuery != nil {
		return false, fmt.Errorf("check federated user permission on server: %w", errQuery)
	}

	return count > 0, nil
}

// GetFederatedUserPermissionIDsForServer returns the distinct permission IDs
// a federated (remote) user holds on a local game server via federated access grants.
// GetFederatedUserPermissionIDsForServer returns permission IDs for a federated user.
func (c *Connection) GetFederatedUserPermissionIDsForServer(remoteNodeID string, remoteUserID string, gameServerID string) ([]string, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`SELECT DISTINCT rp.permission_id
		FROM federated_access_grant fag
		JOIN role_permission rp ON fag.role_id = rp.role_id
		WHERE fag.remote_node_id = ? AND fag.remote_user_id = ?
		AND fag.game_server_id = ?`,
		remoteNodeID, remoteUserID, gameServerID,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("query federated user permission IDs for server: %w", errQuery)
	}
	defer func() { _ = rows.Close() }()

	var perms []string
	for rows.Next() {
		var perm string
		if errScan := rows.Scan(&perm); errScan != nil {
			return nil, fmt.Errorf("scan federated user permission ID: %w", errScan)
		}
		perms = append(perms, perm)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate federated user permission IDs for server: %w", errRows)
	}

	return perms, nil
}

// GetFederatedAccessGrantByID returns a federated access grant by ID.
func (c *Connection) GetFederatedAccessGrantByID(id string) (*models.FederatedAccessGrant, error) {
	grant, errGetGrant := models.FederatedAccessGrants.Query(
		models.SelectWhere.FederatedAccessGrants.ID.EQ(id),
	).One(c.ctx, c.DB)
	if errGetGrant != nil {
		return nil, fmt.Errorf("get federated access grant by ID: %w", errGetGrant)
	}

	return grant, nil
}
