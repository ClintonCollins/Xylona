package db

import (
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) GetFederatedAccessGrantsForServer(gameServerID string) ([]*models.FederatedAccessGrant, error) {
	grants, errGetGrants := models.FederatedAccessGrants.Query(
		models.SelectWhere.FederatedAccessGrants.GameServerID.EQ(gameServerID),
	).All(c.ctx, c.DB)
	if errGetGrants != nil {
		return nil, errGetGrants
	}

	return grants, nil
}

func (c *Connection) GetFederatedAccessGrant(gameServerID string, remoteNodeID string, remoteUserID string) ([]*models.FederatedAccessGrant, error) {
	grants, errGetGrant := models.FederatedAccessGrants.Query(
		models.SelectWhere.FederatedAccessGrants.GameServerID.EQ(gameServerID),
		models.SelectWhere.FederatedAccessGrants.RemoteNodeID.EQ(remoteNodeID),
		models.SelectWhere.FederatedAccessGrants.RemoteUserID.EQ(remoteUserID),
	).All(c.ctx, c.DB)
	if errGetGrant != nil {
		return nil, errGetGrant
	}

	return grants, nil
}

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
	return errCreateGrant
}

func (c *Connection) DeleteFederatedAccessGrant(id string) error {
	grant, errGetGrant := models.FindFederatedAccessGrant(c.ctx, c.DB, id)
	if errGetGrant != nil {
		return errGetGrant
	}

	errDeleteGrant := models.FederatedAccessGrantSlice{grant}.DeleteAll(c.ctx, c.DB)
	if errDeleteGrant != nil {
		return errDeleteGrant
	}

	return nil
}

func (c *Connection) FederatedUserHasPermissionOnServer(remoteNodeID string, remoteUserID string, gameServerID string, permissionID string) (bool, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(
		c.ctx,
		`SELECT COUNT(*)
		FROM federated_access_grant fag
		JOIN role_permission rp ON fag.role_id = rp.role_id
		WHERE fag.remote_node_id = ? AND fag.remote_user_id = ?
		AND fag.game_server_id = ? AND rp.permission_id = ?`,
		remoteNodeID, remoteUserID, gameServerID, permissionID,
	).Scan(&count)
	if errQuery != nil {
		return false, errQuery
	}

	return count > 0, nil
}

func (c *Connection) GetFederatedAccessGrantByID(id string) (*models.FederatedAccessGrant, error) {
	grant, errGetGrant := models.FederatedAccessGrants.Query(
		models.SelectWhere.FederatedAccessGrants.ID.EQ(id),
	).One(c.ctx, c.DB)
	if errGetGrant != nil {
		return nil, errGetGrant
	}

	return grant, nil
}
