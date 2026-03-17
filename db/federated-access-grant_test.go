package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestCreateFederatedAccessGrantAndQueries(t *testing.T) {
	conn := newRBACMigratedConnection(t, "federated-grant.sqlite")
	seedRBACFixture(t, conn)
	seedRemoteNodeForFederatedTests(t, conn, "node-remote-1")

	errCreateGrant := conn.CreateFederatedAccessGrant(
		"fed-grant-1",
		"server-local-1",
		"node-remote-1",
		"remote-user-1",
		"Remote User",
		"operator",
		"user-owner",
	)
	if errCreateGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateGrant)
	}

	grantsByServer, errGetByServer := conn.GetFederatedAccessGrantsForServer("server-local-1")
	if errGetByServer != nil {
		t.Fatalf("GetFederatedAccessGrantsForServer() error = %v", errGetByServer)
	}
	if len(grantsByServer) != 1 {
		t.Fatalf("GetFederatedAccessGrantsForServer() len = %d, want 1", len(grantsByServer))
	}

	grantsByComposite, errGetByComposite := conn.GetFederatedAccessGrant("server-local-1", "node-remote-1", "remote-user-1")
	if errGetByComposite != nil {
		t.Fatalf("GetFederatedAccessGrant() error = %v", errGetByComposite)
	}
	if len(grantsByComposite) != 1 {
		t.Fatalf("GetFederatedAccessGrant() len = %d, want 1", len(grantsByComposite))
	}

	gotGrant, errGetByID := conn.GetFederatedAccessGrantByID("fed-grant-1")
	if errGetByID != nil {
		t.Fatalf("GetFederatedAccessGrantByID() error = %v", errGetByID)
	}
	if gotGrant.RemoteUserName != "Remote User" {
		t.Errorf("GetFederatedAccessGrantByID().RemoteUserName = %q, want %q", gotGrant.RemoteUserName, "Remote User")
	}
}

func TestCreateFederatedAccessGrantUniqueConstraint(t *testing.T) {
	conn := newRBACMigratedConnection(t, "federated-grant-unique.sqlite")
	seedRBACFixture(t, conn)
	seedRemoteNodeForFederatedTests(t, conn, "node-remote-1")

	errFirst := conn.CreateFederatedAccessGrant(
		"fed-grant-unique-1",
		"server-local-1",
		"node-remote-1",
		"remote-user-1",
		"Remote User",
		"operator",
		"user-owner",
	)
	if errFirst != nil {
		t.Fatalf("CreateFederatedAccessGrant(first) error = %v", errFirst)
	}

	errSecond := conn.CreateFederatedAccessGrant(
		"fed-grant-unique-2",
		"server-local-1",
		"node-remote-1",
		"remote-user-1",
		"Remote User",
		"operator",
		"user-owner",
	)
	if errSecond == nil {
		t.Fatalf("CreateFederatedAccessGrant(second) expected unique constraint error, got nil")
	}
}

func TestFederatedUserHasPermissionOnServer(t *testing.T) {
	conn := newRBACMigratedConnection(t, "federated-grant-permission.sqlite")
	seedRBACFixture(t, conn)
	seedRemoteNodeForFederatedTests(t, conn, "node-remote-1")

	errCreateGrant := conn.CreateFederatedAccessGrant(
		"fed-grant-perm-1",
		"server-local-1",
		"node-remote-1",
		"remote-user-1",
		"Remote User",
		"operator",
		"user-owner",
	)
	if errCreateGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateGrant)
	}

	tests := []struct {
		name         string
		remoteNodeID string
		remoteUserID string
		serverID     string
		permissionID string
		want         bool
	}{
		{
			name:         "matching permission",
			remoteNodeID: "node-remote-1",
			remoteUserID: "remote-user-1",
			serverID:     "server-local-1",
			permissionID: "game_server.start",
			want:         true,
		},
		{
			name:         "permission not granted by role",
			remoteNodeID: "node-remote-1",
			remoteUserID: "remote-user-1",
			serverID:     "server-local-1",
			permissionID: "game_server.delete",
			want:         false,
		},
		{
			name:         "different remote user",
			remoteNodeID: "node-remote-1",
			remoteUserID: "remote-user-2",
			serverID:     "server-local-1",
			permissionID: "game_server.start",
			want:         false,
		},
		{
			name:         "different node id does not match",
			remoteNodeID: "node-remote-peer-2",
			remoteUserID: "remote-user-1",
			serverID:     "server-local-1",
			permissionID: "game_server.start",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errPermission := conn.FederatedUserHasPermissionOnServer(tt.remoteNodeID, tt.remoteUserID, tt.serverID, tt.permissionID)
			if errPermission != nil {
				t.Fatalf("FederatedUserHasPermissionOnServer() error = %v", errPermission)
			}
			if got != tt.want {
				t.Errorf("FederatedUserHasPermissionOnServer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeleteFederatedAccessGrant(t *testing.T) {
	conn := newRBACMigratedConnection(t, "federated-grant-delete.sqlite")
	seedRBACFixture(t, conn)
	seedRemoteNodeForFederatedTests(t, conn, "node-remote-1")

	errCreateGrant := conn.CreateFederatedAccessGrant(
		"fed-grant-delete-1",
		"server-local-1",
		"node-remote-1",
		"remote-user-1",
		"Remote User",
		"operator",
		"user-owner",
	)
	if errCreateGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateGrant)
	}

	errDeleteGrant := conn.DeleteFederatedAccessGrant("fed-grant-delete-1")
	if errDeleteGrant != nil {
		t.Fatalf("DeleteFederatedAccessGrant() error = %v", errDeleteGrant)
	}

	_, errGetGrant := conn.GetFederatedAccessGrantByID("fed-grant-delete-1")
	if !errors.Is(errGetGrant, sql.ErrNoRows) {
		t.Errorf("GetFederatedAccessGrantByID() error = %v, want %v", errGetGrant, sql.ErrNoRows)
	}
}

func TestFederatedAccessGrantCascadesOnGameServerDelete(t *testing.T) {
	conn := newRBACMigratedConnection(t, "federated-grant-cascade.sqlite")
	seedRBACFixture(t, conn)
	seedRemoteNodeForFederatedTests(t, conn, "node-remote-1")

	errCreateGrant := conn.CreateFederatedAccessGrant(
		"fed-grant-cascade-1",
		"server-local-1",
		"node-remote-1",
		"remote-user-1",
		"Remote User",
		"operator",
		"user-owner",
	)
	if errCreateGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateGrant)
	}

	_, errDeleteServer := conn.SQLDb.ExecContext(
		conn.ctx,
		`delete from game_server where id = ?`,
		"server-local-1",
	)
	if errDeleteServer != nil {
		t.Fatalf("delete game_server error = %v", errDeleteServer)
	}

	_, errGetGrant := conn.GetFederatedAccessGrantByID("fed-grant-cascade-1")
	if !errors.Is(errGetGrant, sql.ErrNoRows) {
		t.Errorf("GetFederatedAccessGrantByID() error = %v, want %v", errGetGrant, sql.ErrNoRows)
	}
}

func TestCreateFederatedAccessGrantMissingReference(t *testing.T) {
	conn := newRBACMigratedConnection(t, "federated-grant-fk.sqlite")
	seedRBACFixture(t, conn)
	seedRemoteNodeForFederatedTests(t, conn, "node-remote-1")

	errCreateGrant := conn.CreateFederatedAccessGrant(
		"fed-grant-fk-1",
		"server-local-1",
		"node-remote-1",
		"remote-user-1",
		"Remote User",
		"role-does-not-exist",
		"user-owner",
	)
	if errCreateGrant == nil {
		t.Fatalf("CreateFederatedAccessGrant() expected foreign key error, got nil")
	}
}

func seedRemoteNodeForFederatedTests(t *testing.T, conn *Connection, nodeID string) {
	t.Helper()

	_, errInsertNode := conn.SQLDb.ExecContext(
		conn.ctx,
		`insert into node (id, name, is_local, host, port, base_url, enabled)
		 values (?, ?, ?, ?, ?, ?, ?)`,
		nodeID, "Remote Node", false, "remote-host", 8443, "https://remote.example.com", true,
	)
	if errInsertNode != nil {
		t.Fatalf("failed to insert remote node: %v", errInsertNode)
	}
}
