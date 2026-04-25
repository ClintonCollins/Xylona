package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestPersistedVariantTarget(t *testing.T) {
	testCases := []struct {
		name       string
		kind       updateproviders.ProviderKind
		target     string
		pinTarget  bool
		wantTarget string
		wantPinned bool
	}{
		{
			name:       "mojang tracking latest clears stored target",
			kind:       updateproviders.ProviderKindMojang,
			target:     "1.21.4",
			pinTarget:  false,
			wantTarget: "",
			wantPinned: false,
		},
		{
			name:       "papermc pinned keeps target",
			kind:       updateproviders.ProviderKindPaperMC,
			target:     "1.21.4",
			pinTarget:  true,
			wantTarget: "1.21.4",
			wantPinned: true,
		},
		{
			name:       "steamcmd stays sticky",
			kind:       updateproviders.ProviderKindSteamCMD,
			target:     "latest_experimental",
			pinTarget:  false,
			wantTarget: "latest_experimental",
			wantPinned: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotTarget, gotPinned := persistedVariantTarget(tc.kind, tc.target, tc.pinTarget)
			if gotTarget != tc.wantTarget {
				t.Fatalf("persistedVariantTarget() target = %q, want %q", gotTarget, tc.wantTarget)
			}
			if gotPinned != tc.wantPinned {
				t.Fatalf("persistedVariantTarget() pinned = %v, want %v", gotPinned, tc.wantPinned)
			}
		})
	}
}

func TestSetServerSoftwareUsesLiveNodeStatus(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}

	wireServiceEmbeddedNode(t, fixture, supervisorInst)
	fixture.service.installTracker = modmanager.NewInstallTracker()

	gameServer, errGetServer := fixture.conn.GetGameServerByID("server-local-1")
	if errGetServer != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGetServer)
	}

	game, errGetGame := fixture.conn.GetGameByID(gameServer.GameID)
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}

	_, errUpdateGame := fixture.conn.UpdateGame(fixture.conn.DB, game, &models.GameSetter{
		ID: omit.From(game.ID),
		ServerSoftware: omitnull.From(`{
			"variants":[
				{
					"id":"paper",
					"name":"Paper"
				}
			]
		}`),
	})
	if errUpdateGame != nil {
		t.Fatalf("UpdateGame() error = %v", errUpdateGame)
	}

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:     omit.From(gameServer.ID),
		Status: omit.From(xylona.Status_ONLINE.String()),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	supervisorInst.GetCommandByIDOrCreateShell(gameServer.ID)

	request := connect.NewRequest(&xylona.SetServerVariantRequest{
		GameServerId: gameServer.ID,
		VariantId:    "paper",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errSet := fixture.service.SetServerVariant(context.Background(), request)
	if errSet != nil {
		t.Fatalf("SetServerVariant() error = %v", errSet)
	}
	if response.Msg.GetStatus() != modmanager.InstallStatusComplete {
		t.Fatalf("SetServerVariant().Status = %q, want %q", response.Msg.GetStatus(), modmanager.InstallStatusComplete)
	}

	updatedServer, errGetUpdated := fixture.conn.GetGameServerByID(gameServer.ID)
	if errGetUpdated != nil {
		t.Fatalf("GetGameServerByID(updated) error = %v", errGetUpdated)
	}
	if got := updatedServer.ServerSoftware.GetOr(""); got != "paper" {
		t.Fatalf("updated server software = %q, want %q", got, "paper")
	}
}
