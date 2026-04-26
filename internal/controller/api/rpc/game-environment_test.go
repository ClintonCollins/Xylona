package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGameEnvironmentRoundTrip(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	updateReq := connect.NewRequest(&xylona.UpdateGameEnvironmentRequest{
		GameId: "minecraft",
		DefaultEnv: []*xylona.EnvironmentVariable{
			{Name: "HYTALE_AUTH_MODE", Value: "refresh_token"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-admin")

	updateResp, errUpdate := fixture.service.UpdateGameEnvironment(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("UpdateGameEnvironment() error = %v", errUpdate)
	}
	if len(updateResp.Msg.GetDefaultEnv()) != 1 {
		t.Fatalf("UpdateGameEnvironment().DefaultEnv length = %d, want 1", len(updateResp.Msg.GetDefaultEnv()))
	}

	getReq := connect.NewRequest(&xylona.GetGameEnvironmentRequest{
		GameId: "minecraft",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-admin")

	getResp, errGet := fixture.service.GetGameEnvironment(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetGameEnvironment() error = %v", errGet)
	}
	if getResp.Msg.GetDefaultEnv()[0].GetName() != "HYTALE_AUTH_MODE" {
		t.Fatalf("GetGameEnvironment().DefaultEnv[0].Name = %q, want HYTALE_AUTH_MODE", getResp.Msg.GetDefaultEnv()[0].GetName())
	}
}

func TestGameEnvironmentRequiresSuperuser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.UpdateGameEnvironmentRequest{
		GameId: "minecraft",
		DefaultEnv: []*xylona.EnvironmentVariable{
			{Name: "TOKEN", Value: "value"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-owner")

	_, errUpdate := fixture.service.UpdateGameEnvironment(context.Background(), req)
	if errUpdate == nil {
		t.Fatal("UpdateGameEnvironment(non-superuser) error = nil, want permission denied")
	}
	if connect.CodeOf(errUpdate) != connect.CodePermissionDenied {
		t.Fatalf("UpdateGameEnvironment(non-superuser) code = %v, want %v", connect.CodeOf(errUpdate), connect.CodePermissionDenied)
	}
}

func TestEditGamePreservesDefaultEnvironment(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	updateReq := connect.NewRequest(&xylona.UpdateGameEnvironmentRequest{
		GameId: "minecraft",
		DefaultEnv: []*xylona.EnvironmentVariable{
			{Name: "KEEP_ME", Value: "yes"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-admin")
	_, errUpdateEnv := fixture.service.UpdateGameEnvironment(context.Background(), updateReq)
	if errUpdateEnv != nil {
		t.Fatalf("UpdateGameEnvironment() setup error = %v", errUpdateEnv)
	}

	editReq := connect.NewRequest(&xylona.EditGameRequest{
		Game: newTestGame("minecraft", "Minecraft Edited"),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, editReq, "user-admin")
	_, errEdit := fixture.service.EditGame(context.Background(), editReq)
	if errEdit != nil {
		t.Fatalf("EditGame() error = %v", errEdit)
	}

	getReq := connect.NewRequest(&xylona.GetGameEnvironmentRequest{
		GameId: "minecraft",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-admin")
	getResp, errGet := fixture.service.GetGameEnvironment(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetGameEnvironment() error = %v", errGet)
	}
	if len(getResp.Msg.GetDefaultEnv()) != 1 {
		t.Fatalf("GetGameEnvironment().DefaultEnv length = %d, want 1", len(getResp.Msg.GetDefaultEnv()))
	}
	if getResp.Msg.GetDefaultEnv()[0].GetName() != "KEEP_ME" {
		t.Fatalf("GetGameEnvironment().DefaultEnv[0].Name = %q, want KEEP_ME", getResp.Msg.GetDefaultEnv()[0].GetName())
	}
}
