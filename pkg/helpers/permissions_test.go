package helpers

import (
	"errors"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/sql/models"
)

type permissionLookupStub struct {
	result bool
	err    error
	called bool
}

func (p *permissionLookupStub) UserHasPermissionOnServer(_ string, _ string, _ string) (bool, error) {
	p.called = true
	return p.result, p.err
}

func TestHasPermission(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name                  string
		user                  *models.User
		gameServerID          string
		gameServerOwnerUserID string
		permissionID          string
		lookupResult          bool
		lookupErr             error
		want                  bool
		wantErr               bool
		wantLookupCall        bool
	}{
		{
			name:                  "nil user denied",
			user:                  nil,
			gameServerID:          "server-1",
			gameServerOwnerUserID: "owner-1",
			permissionID:          "game_server.view",
			want:                  false,
			wantLookupCall:        false,
		},
		{
			name: "super user allowed without lookup",
			user: &models.User{
				ID:        "admin-1",
				SuperUser: true,
				CreatedAt: now,
				UpdatedAt: now,
			},
			gameServerID:          "server-1",
			gameServerOwnerUserID: "owner-1",
			permissionID:          "game_server.delete",
			want:                  true,
			wantLookupCall:        false,
		},
		{
			name: "owner allowed without lookup",
			user: &models.User{
				ID:        "owner-1",
				SuperUser: false,
				CreatedAt: now,
				UpdatedAt: now,
			},
			gameServerID:          "server-1",
			gameServerOwnerUserID: "owner-1",
			permissionID:          "game_server.delete",
			want:                  true,
			wantLookupCall:        false,
		},
		{
			name: "role assignment allows permission",
			user: &models.User{
				ID:        "user-1",
				SuperUser: false,
				CreatedAt: now,
				UpdatedAt: now,
			},
			gameServerID:          "server-1",
			gameServerOwnerUserID: "owner-1",
			permissionID:          "game_server.start",
			lookupResult:          true,
			want:                  true,
			wantLookupCall:        true,
		},
		{
			name: "role assignment denies permission",
			user: &models.User{
				ID:        "user-1",
				SuperUser: false,
				CreatedAt: now,
				UpdatedAt: now,
			},
			gameServerID:          "server-1",
			gameServerOwnerUserID: "owner-1",
			permissionID:          "game_server.delete",
			lookupResult:          false,
			want:                  false,
			wantLookupCall:        true,
		},
		{
			name: "lookup error is returned",
			user: &models.User{
				ID:        "user-1",
				SuperUser: false,
				CreatedAt: now,
				UpdatedAt: now,
			},
			gameServerID:          "server-1",
			gameServerOwnerUserID: "owner-1",
			permissionID:          "game_server.view",
			lookupErr:             errors.New("lookup failed"),
			wantErr:               true,
			wantLookupCall:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &permissionLookupStub{
				result: tt.lookupResult,
				err:    tt.lookupErr,
			}

			got, errHasPermission := HasPermission(stub, tt.user, tt.gameServerID, tt.gameServerOwnerUserID, tt.permissionID)
			if (errHasPermission != nil) != tt.wantErr {
				t.Fatalf("HasPermission() error = %v, wantErr %v", errHasPermission, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("HasPermission() = %v, want %v", got, tt.want)
			}
			if stub.called != tt.wantLookupCall {
				t.Errorf("HasPermission() lookup called = %v, want %v", stub.called, tt.wantLookupCall)
			}
		})
	}
}
