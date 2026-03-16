package rpc

import (
	"net/http"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/helpers"
)

func TestAuthorizeFederatedPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	server := FederationService{
		db: fixture.conn,
	}

	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote")

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
		name         string
		actingUserID string
		originNodeID string
		superHeader  string
		wantErr      bool
		wantCode     connect.Code
	}{
		{
			name:     "missing identity is denied",
			wantErr:  true,
			wantCode: connect.CodePermissionDenied,
		},
		{
			name:        "super-user header without identity is allowed",
			superHeader: "true",
			wantErr:     false,
		},
		{
			name:         "non-super user without grant is denied",
			actingUserID: "user-other",
			originNodeID: "node-remote",
			superHeader:  "false",
			wantErr:      true,
			wantCode:     connect.CodePermissionDenied,
		},
		{
			name:         "non-super user with grant is allowed",
			actingUserID: "user-owner",
			originNodeID: "node-remote",
			superHeader:  "false",
			wantErr:      false,
		},
		{
			name:         "super user bypasses federated grant checks",
			actingUserID: "user-admin",
			originNodeID: "node-remote",
			superHeader:  "true",
			wantErr:      false,
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

			errAuthorize := server.authorizeFederatedPermission(
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
