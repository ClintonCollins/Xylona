package federation

import (
	"net/http"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestApplyActingIdentityHeaders(t *testing.T) {
	tests := []struct {
		name             string
		initialSuper     string
		actingUser       *models.User
		localNodeID      string
		wantActingUserID string
		wantOriginNodeID string
		wantSuperHeader  string
		wantIsSuperUser  bool
	}{
		{
			name:             "nil user leaves headers unchanged",
			initialSuper:     "true",
			actingUser:       nil,
			localNodeID:      "node-local",
			wantActingUserID: "",
			wantOriginNodeID: "",
			wantSuperHeader:  "true",
			wantIsSuperUser:  true,
		},
		{
			name:         "non-super user sets identity and clears super header",
			initialSuper: "true",
			actingUser: &models.User{
				ID:        "user-owner",
				SuperUser: false,
			},
			localNodeID:      "node-local",
			wantActingUserID: "user-owner",
			wantOriginNodeID: "node-local",
			wantSuperHeader:  "",
			wantIsSuperUser:  false,
		},
		{
			name: "super user sets identity and super header",
			actingUser: &models.User{
				ID:        "user-admin",
				SuperUser: true,
			},
			localNodeID:      "node-local",
			wantActingUserID: "user-admin",
			wantOriginNodeID: "node-local",
			wantSuperHeader:  "true",
			wantIsSuperUser:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.initialSuper != "" {
				header.Set(ActingSuperHeader, tt.initialSuper)
			}

			ApplyActingIdentityHeaders(header, tt.actingUser, tt.localNodeID)

			gotActingUserID, gotOriginNodeID := GetActingIdentity(header)
			if gotActingUserID != tt.wantActingUserID {
				t.Errorf("acting user id = %q, want %q", gotActingUserID, tt.wantActingUserID)
			}
			if gotOriginNodeID != tt.wantOriginNodeID {
				t.Errorf("origin node id = %q, want %q", gotOriginNodeID, tt.wantOriginNodeID)
			}

			gotSuperHeader := header.Get(ActingSuperHeader)
			if gotSuperHeader != tt.wantSuperHeader {
				t.Errorf("super header = %q, want %q", gotSuperHeader, tt.wantSuperHeader)
			}

			gotIsSuperUser := ActingIsSuperUser(header)
			if gotIsSuperUser != tt.wantIsSuperUser {
				t.Errorf("is super user = %v, want %v", gotIsSuperUser, tt.wantIsSuperUser)
			}
		})
	}
}

func TestActingIsSuperUser(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "header missing", header: "", want: false},
		{name: "true lowercase", header: "true", want: true},
		{name: "true mixed case and spaces", header: "  TrUe  ", want: true},
		{name: "false", header: "false", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.header != "" {
				header.Set(ActingSuperHeader, tt.header)
			}
			got := ActingIsSuperUser(header)
			if got != tt.want {
				t.Errorf("ActingIsSuperUser() = %v, want %v", got, tt.want)
			}
		})
	}
}
