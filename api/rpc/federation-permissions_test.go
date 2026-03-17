package rpc

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/supervisor"
)

func TestAuthorizeFederatedPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	server := FederationService{
		db: fixture.conn,
	}

	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote")
	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID:     "node-remote",
		PeerNodeID: "peer-node-id",
	})

	errCreateGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-grant-1",
		"server-local-1",
		"node-remote",
		"user-owner",
		"owner",
		"viewer",
		"user-admin",
	)
	if errCreateGrant != nil {
		t.Fatalf("failed to create federated access grant: %v", errCreateGrant)
	}

	tests := []struct {
		name             string
		withPeerIdentity bool
		actingUserID     string
		originNodeID     string
		superHeader      string
		wantErr          bool
		wantCode         connect.Code
	}{
		{
			name:             "missing authenticated peer identity is denied",
			withPeerIdentity: false,
			wantErr:          true,
			wantCode:         connect.CodePermissionDenied,
		},
		{
			name:             "missing acting user identity is denied",
			withPeerIdentity: true,
			wantErr:          true,
			wantCode:         connect.CodePermissionDenied,
		},
		{
			name:             "origin mismatch is denied",
			withPeerIdentity: true,
			actingUserID:     "user-owner",
			originNodeID:     "node-other",
			wantErr:          true,
			wantCode:         connect.CodePermissionDenied,
		},
		{
			name:             "non-super user without grant is denied",
			withPeerIdentity: true,
			actingUserID:     "user-other",
			originNodeID:     "node-remote",
			wantErr:          true,
			wantCode:         connect.CodePermissionDenied,
		},
		{
			name:             "non-super user with grant is allowed",
			withPeerIdentity: true,
			actingUserID:     "user-owner",
			originNodeID:     "node-remote",
			wantErr:          false,
		},
		{
			name:             "super-user header bypasses grant checks with valid identity",
			withPeerIdentity: true,
			actingUserID:     "user-admin",
			originNodeID:     "node-remote",
			superHeader:      "true",
			wantErr:          false,
		},
		{
			name:             "super-user header with missing origin is denied",
			withPeerIdentity: true,
			actingUserID:     "user-admin",
			superHeader:      "true",
			wantErr:          true,
			wantCode:         connect.CodePermissionDenied,
		},
		{
			name:             "super-user header with mismatched origin is denied",
			withPeerIdentity: true,
			actingUserID:     "user-admin",
			originNodeID:     "node-other",
			superHeader:      "true",
			wantErr:          true,
			wantCode:         connect.CodePermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.actingUserID != "" {
				header.Set(helpers.FederationActingUserIDHeader, tt.actingUserID)
			}
			if tt.originNodeID != "" {
				header.Set(helpers.FederationOriginNodeIDHeader, tt.originNodeID)
			}
			if tt.superHeader != "" {
				header.Set(helpers.FederationActingSuperHeader, tt.superHeader)
			}

			testCtx := context.Background()
			if tt.withPeerIdentity {
				testCtx = peerCtx
			}

			errAuthorize := server.authorizeFederatedPermission(
				testCtx,
				header,
				"",
				"",
				"server-local-1",
				"game_server.view",
			)
			if (errAuthorize != nil) != tt.wantErr {
				t.Fatalf("authorizeFederatedPermission() error = %v, wantErr %v", errAuthorize, tt.wantErr)
			}
			if !tt.wantErr {
				return
			}

			gotCode := connect.CodeOf(errAuthorize)
			if gotCode != tt.wantCode {
				t.Errorf("authorizeFederatedPermission() code = %v, want %v", gotCode, tt.wantCode)
			}
		})
	}
}

func TestListServerSummariesFiltersByFederatedViewPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote")

	_, errInsertServer := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, start_command, status, set_players, max_players, map, ip, port, query_port, directory, node_id)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-local-2", "user-owner", "Local Two", "minecraft", "java -jar server.jar", "OFFLINE",
		20, 20, "world-two", "127.0.0.1", 25566, 25566, "/tmp/server-local-2", "node-local",
	)
	if errInsertServer != nil {
		t.Fatalf("failed to insert second local server: %v", errInsertServer)
	}

	errCreateGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-grant-allowed",
		"server-local-1",
		"node-remote",
		"user-owner",
		"owner",
		"viewer",
		"user-admin",
	)
	if errCreateGrant != nil {
		t.Fatalf("failed to create federated grant: %v", errCreateGrant)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	service := FederationService{
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-remote",
	})

	request := connect.NewRequest(&xylona.FederationListServerSummariesRequest{})
	request.Header().Set(helpers.FederationActingUserIDHeader, "user-owner")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-remote")

	response, errList := service.ListServerSummaries(peerCtx, request)
	if errList != nil {
		t.Fatalf("ListServerSummaries() error = %v", errList)
	}

	if len(response.Msg.Servers) != 1 {
		t.Fatalf("len(response.Msg.Servers) = %d, want 1", len(response.Msg.Servers))
	}
	if response.Msg.Servers[0].ServerId != "server-local-1" {
		t.Fatalf("response.Msg.Servers[0].ServerId = %q, want %q", response.Msg.Servers[0].ServerId, "server-local-1")
	}

	requestNoActingIdentity := connect.NewRequest(&xylona.FederationListServerSummariesRequest{})
	responseNoIdentity, errNoIdentity := service.ListServerSummaries(peerCtx, requestNoActingIdentity)
	if errNoIdentity != nil {
		t.Fatalf("ListServerSummaries(no identity) error = %v", errNoIdentity)
	}
	if len(responseNoIdentity.Msg.Servers) != 2 {
		t.Fatalf("len(responseNoIdentity.Msg.Servers) = %d, want 2", len(responseNoIdentity.Msg.Servers))
	}

	requestSuper := connect.NewRequest(&xylona.FederationListServerSummariesRequest{})
	requestSuper.Header().Set(helpers.FederationActingUserIDHeader, "user-admin")
	requestSuper.Header().Set(helpers.FederationOriginNodeIDHeader, "node-remote")
	requestSuper.Header().Set(helpers.FederationActingSuperHeader, "true")
	responseSuper, errSuper := service.ListServerSummaries(peerCtx, requestSuper)
	if errSuper != nil {
		t.Fatalf("ListServerSummaries(super) error = %v", errSuper)
	}
	if len(responseSuper.Msg.Servers) != 2 {
		t.Fatalf("len(responseSuper.Msg.Servers) = %d, want 2", len(responseSuper.Msg.Servers))
	}
}

func TestListServerSummariesTreatsPersistedOnlineAsOfflineWhenCommandMissing(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_, errSetStatus := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game_server set status = ? where id = ?`,
		xylona.Status_ONLINE.String(),
		"server-local-1",
	)
	if errSetStatus != nil {
		t.Fatalf("failed to set game server status: %v", errSetStatus)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	service := FederationService{
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-remote",
	})

	request := connect.NewRequest(&xylona.FederationListServerSummariesRequest{})
	response, errList := service.ListServerSummaries(peerCtx, request)
	if errList != nil {
		t.Fatalf("ListServerSummaries() error = %v", errList)
	}
	if len(response.Msg.Servers) != 1 {
		t.Fatalf("len(response.Msg.Servers) = %d, want 1", len(response.Msg.Servers))
	}
	if response.Msg.Servers[0].Status != xylona.Status_OFFLINE {
		t.Fatalf("response.Msg.Servers[0].Status = %v, want %v", response.Msg.Servers[0].Status, xylona.Status_OFFLINE)
	}
}

func TestGetServerDetailTreatsPersistedOnlineAsOfflineWhenCommandMissing(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote")

	_, errSetStatus := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game_server set status = ? where id = ?`,
		xylona.Status_ONLINE.String(),
		"server-local-1",
	)
	if errSetStatus != nil {
		t.Fatalf("failed to set game server status: %v", errSetStatus)
	}

	errCreateGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-grant-detail-status",
		"server-local-1",
		"node-remote",
		"user-owner",
		"owner",
		"viewer",
		"user-admin",
	)
	if errCreateGrant != nil {
		t.Fatalf("failed to create federated grant: %v", errCreateGrant)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	service := FederationService{
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-remote",
	})

	request := connect.NewRequest(&xylona.FederationGetServerDetailRequest{
		ServerId: "server-local-1",
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "user-owner")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-remote")

	response, errDetail := service.GetServerDetail(peerCtx, request)
	if errDetail != nil {
		t.Fatalf("GetServerDetail() error = %v", errDetail)
	}
	if response.Msg == nil || response.Msg.Server == nil {
		t.Fatalf("GetServerDetail() returned empty server")
	}
	if response.Msg.Server.Status != xylona.Status_OFFLINE {
		t.Fatalf("response.Msg.Server.Status = %v, want %v", response.Msg.Server.Status, xylona.Status_OFFLINE)
	}
}
