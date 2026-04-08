package rpc

import (
	"net/http"
	"testing"

	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestApplyFederatedActingIdentity(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	tests := []struct {
		name             string
		user             *models.User
		wantActingUserID string
		wantOriginNodeID string
		wantSuperUser    bool
	}{
		{
			name:             "nil user leaves headers empty",
			user:             nil,
			wantActingUserID: "",
			wantOriginNodeID: "",
			wantSuperUser:    false,
		},
		{
			name: "super user propagates acting identity with super-user flag",
			user: &models.User{
				ID:        "user-admin",
				SuperUser: true,
			},
			wantActingUserID: "user-admin",
			wantOriginNodeID: "node-local",
			wantSuperUser:    true,
		},
		{
			name: "non-super user propagates acting identity without super-user flag",
			user: &models.User{
				ID:        "user-owner",
				SuperUser: false,
			},
			wantActingUserID: "user-owner",
			wantOriginNodeID: "node-local",
			wantSuperUser:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}

			errApply := fixture.service.applyFederatedActingIdentity(header, tt.user)
			if errApply != nil {
				t.Fatalf("applyFederatedActingIdentity() error = %v", errApply)
			}

			gotActingUserID, gotOriginNodeID := federation.GetActingIdentity(header)
			if gotActingUserID != tt.wantActingUserID {
				t.Errorf("acting user id = %q, want %q", gotActingUserID, tt.wantActingUserID)
			}
			if gotOriginNodeID != tt.wantOriginNodeID {
				t.Errorf("origin node id = %q, want %q", gotOriginNodeID, tt.wantOriginNodeID)
			}
			gotSuperUser := federation.ActingIsSuperUser(header)
			if gotSuperUser != tt.wantSuperUser {
				t.Errorf("acting super-user = %v, want %v", gotSuperUser, tt.wantSuperUser)
			}
		})
	}
}
