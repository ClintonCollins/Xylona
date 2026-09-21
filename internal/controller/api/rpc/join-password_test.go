package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestJoinPasswordRPCStateAuthorizationAndClear(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	ctx := context.Background()
	_, errGame := fixture.conn.SQLDb.ExecContext(ctx, "insert into game (id, name, default_port, default_query_port, default_max_players) values ('valheim', 'Valheim', 2456, 2457, 10) on conflict(id) do nothing")
	if errGame != nil {
		t.Fatal(errGame)
	}
	_, errServer := fixture.conn.SQLDb.ExecContext(ctx, "update game_server set game_id = 'valheim' where id = 'server-local-1'")
	if errServer != nil {
		t.Fatal(errServer)
	}
	password := "fixture password ' with spaces"
	for _, userID := range []string{"user-other", "user-admin"} {
		request := connect.NewRequest(&xylona.SetJoinPasswordRequest{ServerId: "server-local-1", Password: password})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, userID)
		response, errSet := fixture.service.SetJoinPassword(ctx, request)
		if userID == "user-other" {
			if connect.CodeOf(errSet) != connect.CodePermissionDenied {
				t.Fatalf("unauthorized set = %v", errSet)
			}
			continue
		}
		if errSet != nil {
			t.Fatal(errSet)
		}
		if !response.Msg.GetState().GetConfigured() || len(response.Msg.GetState().GetValidationIssues()) != 0 {
			t.Fatal("set did not return configured state")
		}

	}
	getRequest := connect.NewRequest(&xylona.GetJoinPasswordStateRequest{ServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getRequest, "user-admin")
	getResponse, errGet := fixture.service.GetJoinPasswordState(ctx, getRequest)
	if errGet != nil {
		t.Fatal(errGet)
	}
	if !getResponse.Msg.GetState().GetConfigured() {
		t.Fatal("read lost configured state")
	}
	server, errLoad := fixture.conn.GetGameServerByID("server-local-1")
	if errLoad != nil {
		t.Fatal(errLoad)
	}
	errReady := readiness.ValidateJoinPasswordStart(fixture.conn, server)
	if errReady != nil {
		t.Fatal(errReady)
	}
	renamed := *server
	renamed.Name = "Server " + password
	errName := fixture.service.validateJoinPasswordServerName(server, &renamed)
	if connect.CodeOf(errName) != connect.CodeInvalidArgument {
		t.Fatal("server name containing configured password was accepted")
	}
	clearRequest := connect.NewRequest(&xylona.ClearJoinPasswordRequest{ServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, clearRequest, "user-admin")
	clearResponse, errClear := fixture.service.ClearJoinPassword(ctx, clearRequest)
	if errClear != nil {
		t.Fatal(errClear)
	}
	if clearResponse.Msg.GetState().GetConfigured() {
		t.Fatal("clear retained configured state")
	}
	server, errLoad = fixture.conn.GetGameServerByID(server.ID)
	if errLoad != nil {
		t.Fatal(errLoad)
	}
	errReady = readiness.ValidateJoinPasswordStart(fixture.conn, server)
	if errReady != nil {
		t.Fatalf("cleared password blocked next start: %v", errReady)
	}
}
