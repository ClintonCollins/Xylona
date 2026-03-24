package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

func TestSetServerSoftwareUsesLiveSupervisorStatus(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}

	fixture.service.supervisorInst = supervisorInst
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
		ID:             omit.From(game.ID),
		ServerSoftware: omitnull.From(`[{"id":"paper","name":"Paper"}]`),
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

	request := connect.NewRequest(&xylona.SetServerSoftwareRequest{
		GameServerId: gameServer.ID,
		SoftwareId:   "paper",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errSet := fixture.service.SetServerSoftware(context.Background(), request)
	if errSet != nil {
		t.Fatalf("SetServerSoftware() error = %v", errSet)
	}
	if response.Msg.GetStatus() != modmanager.InstallStatusComplete {
		t.Fatalf("SetServerSoftware().Status = %q, want %q", response.Msg.GetStatus(), modmanager.InstallStatusComplete)
	}

	updatedServer, errGetUpdated := fixture.conn.GetGameServerByID(gameServer.ID)
	if errGetUpdated != nil {
		t.Fatalf("GetGameServerByID(updated) error = %v", errGetUpdated)
	}
	if got := updatedServer.ServerSoftware.GetOr(""); got != "paper" {
		t.Fatalf("updated server software = %q, want %q", got, "paper")
	}
}
