package rpc

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func federationPeerTestContext() context.Context {
	return context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID:      "node-remote-peer",
		PeerNodeID:  "peer-node",
		Fingerprint: "peer-fingerprint",
	})
}

func TestGrantRemoteGameServerAccessFallsBackGrantorToOwner(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	server := FederationService{db: fixture.conn}
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote-peer")

	errCreateFederatedGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-auth-grant-1",
		"server-local-1",
		"node-remote-peer",
		"external-user-id",
		"External User",
		"admin",
		"user-owner",
	)
	if errCreateFederatedGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateFederatedGrant)
	}

	request := connect.NewRequest(&xylona.FederationGrantGameServerAccessRequest{
		GameServerId: "server-local-1",
		UserId:       "user-other",
		RoleId:       "viewer",
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "external-user-id")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-remote-peer")

	response, errGrant := server.GrantRemoteGameServerAccess(federationPeerTestContext(), request)
	if errGrant != nil {
		t.Fatalf("GrantRemoteGameServerAccess() error = %v", errGrant)
	}
	if response == nil || response.Msg == nil || response.Msg.GetGrant() == nil {
		t.Fatalf("GrantRemoteGameServerAccess() returned empty response")
	}

	assignment, errGetAssignment := fixture.conn.GetUserRoleAssignmentByID(response.Msg.GetGrant().GetId())
	if errGetAssignment != nil {
		t.Fatalf("GetUserRoleAssignmentByID() error = %v", errGetAssignment)
	}
	if assignment.GrantedBy != "user-owner" {
		t.Errorf("assignment.GrantedBy = %q, want %q", assignment.GrantedBy, "user-owner")
	}
}

func TestRevokeRemoteGameServerAccessDeletesGrant(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	server := FederationService{db: fixture.conn}
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote-peer")

	errCreateFederatedGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-auth-grant-2",
		"server-local-1",
		"node-remote-peer",
		"external-user-id",
		"External User",
		"admin",
		"user-owner",
	)
	if errCreateFederatedGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateFederatedGrant)
	}

	errCreate := fixture.conn.CreateUserRoleAssignment(
		"grant-to-revoke",
		"user-other",
		"viewer",
		"server-local-1",
		"user-owner",
	)
	if errCreate != nil {
		t.Fatalf("CreateUserRoleAssignment() error = %v", errCreate)
	}

	request := connect.NewRequest(&xylona.FederationRevokeGameServerAccessRequest{
		GrantId: "grant-to-revoke",
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "external-user-id")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-remote-peer")

	_, errRevoke := server.RevokeRemoteGameServerAccess(federationPeerTestContext(), request)
	if errRevoke != nil {
		t.Fatalf("RevokeRemoteGameServerAccess() error = %v", errRevoke)
	}

	_, errGetAssignment := fixture.conn.GetUserRoleAssignmentByID("grant-to-revoke")
	if !errors.Is(errGetAssignment, sql.ErrNoRows) {
		t.Errorf("GetUserRoleAssignmentByID() error = %v, want %v", errGetAssignment, sql.ErrNoRows)
	}
}

func TestGrantRemoteFederatedAccessFallsBackGrantorToOwner(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	server := FederationService{db: fixture.conn}
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote-peer")
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote-1")

	errCreateFederatedGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-auth-grant-3",
		"server-local-1",
		"node-remote-peer",
		"external-user-id",
		"External User",
		"admin",
		"user-owner",
	)
	if errCreateFederatedGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateFederatedGrant)
	}

	request := connect.NewRequest(&xylona.FederationGrantFederatedAccessRequest{
		GameServerId:   "server-local-1",
		RemoteNodeId:   "node-remote-1",
		RemoteUserId:   "remote-user-1",
		RemoteUserName: "Remote User",
		RoleId:         "viewer",
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "external-user-id")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-remote-peer")

	response, errGrant := server.GrantRemoteFederatedAccess(federationPeerTestContext(), request)
	if errGrant != nil {
		t.Fatalf("GrantRemoteFederatedAccess() error = %v", errGrant)
	}
	if response == nil || response.Msg == nil || response.Msg.GetGrant() == nil {
		t.Fatalf("GrantRemoteFederatedAccess() returned empty response")
	}

	grantModel, errGetGrant := fixture.conn.GetFederatedAccessGrantByID(response.Msg.GetGrant().GetId())
	if errGetGrant != nil {
		t.Fatalf("GetFederatedAccessGrantByID() error = %v", errGetGrant)
	}
	if grantModel.GrantedBy != "user-owner" {
		t.Errorf("grantModel.GrantedBy = %q, want %q", grantModel.GrantedBy, "user-owner")
	}
}
