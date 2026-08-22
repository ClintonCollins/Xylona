package rpc

import (
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGameServerMapShareSettingsAndResolution(t *testing.T) {
	t.Run("management and public resolution", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		fixture.service.statusPageIdentifier = func() (string, error) { return "InitialSlug", nil }

		viewerRequest := connect.NewRequest(&xylona.GetOrCreateGameServerMapShareSettingsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerRequest, "user-other")
		_, errViewer := fixture.service.GetOrCreateGameServerMapShareSettings(t.Context(), viewerRequest)
		if connect.CodeOf(errViewer) != connect.CodePermissionDenied {
			t.Fatalf("GetOrCreateGameServerMapShareSettings(viewer) code = %v, want %v", connect.CodeOf(errViewer), connect.CodePermissionDenied)
		}

		ownerRequest := connect.NewRequest(&xylona.GetOrCreateGameServerMapShareSettingsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, ownerRequest, "user-owner")
		ownerResponse, errOwner := fixture.service.GetOrCreateGameServerMapShareSettings(t.Context(), ownerRequest)
		if errOwner != nil {
			t.Fatalf("GetOrCreateGameServerMapShareSettings(owner) error = %v", errOwner)
		}
		settings := ownerResponse.Msg.GetSettings()
		if settings.GetPublicIdentifier() != "InitialSlug" || settings.GetEnabled() || settings.GetPublicPath() != "/maps/InitialSlug" {
			t.Fatalf("GetOrCreateGameServerMapShareSettings(owner) settings = %+v", settings)
		}

		invalidRequest := connect.NewRequest(&xylona.UpdateGameServerMapShareSettingsRequest{
			GameServerId: "server-local-1", PublicIdentifier: "bad slug",
		})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, invalidRequest, "user-owner")
		_, errInvalid := fixture.service.UpdateGameServerMapShareSettings(t.Context(), invalidRequest)
		if connect.CodeOf(errInvalid) != connect.CodeInvalidArgument {
			t.Fatalf("UpdateGameServerMapShareSettings(invalid) code = %v, want %v", connect.CodeOf(errInvalid), connect.CodeInvalidArgument)
		}

		enableRequest := connect.NewRequest(&xylona.UpdateGameServerMapShareSettingsRequest{
			GameServerId: "server-local-1", PublicIdentifier: "Minecraft_Map", Enabled: true,
		})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, enableRequest, "user-owner")
		_, errNotConfigured := fixture.service.UpdateGameServerMapShareSettings(t.Context(), enableRequest)
		if connect.CodeOf(errNotConfigured) != connect.CodeFailedPrecondition {
			t.Fatalf("UpdateGameServerMapShareSettings(unconfigured) code = %v, want %v", connect.CodeOf(errNotConfigured), connect.CodeFailedPrecondition)
		}

		errMapConfig := fixture.conn.UpdateGameServerMinecraftMapConfig("server-local-1", true, "world", true, "user-owner")
		if errMapConfig != nil {
			t.Fatalf("enable Minecraft map: %v", errMapConfig)
		}
		enableResponse, errEnable := fixture.service.UpdateGameServerMapShareSettings(t.Context(), enableRequest)
		if errEnable != nil {
			t.Fatalf("UpdateGameServerMapShareSettings(enable) error = %v", errEnable)
		}
		if !enableResponse.Msg.GetSettings().GetEnabled() || enableResponse.Msg.GetSettings().GetPublicPath() != "/maps/Minecraft_Map" {
			t.Fatalf("UpdateGameServerMapShareSettings(enable) settings = %+v", enableResponse.Msg.GetSettings())
		}

		resolveRequest := connect.NewRequest(&xylona.ResolvePublicGameServerMapRequest{PublicIdentifier: "Minecraft_Map"})
		resolveResponse, errResolve := fixture.service.ResolvePublicGameServerMap(t.Context(), resolveRequest)
		if errResolve != nil || resolveResponse.Msg.GetKind() != xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_MINECRAFT {
			t.Fatalf("ResolvePublicGameServerMap() = %+v, %v", resolveResponse, errResolve)
		}
		if resolveResponse.Header().Get("X-Robots-Tag") != "noindex, nofollow" || resolveResponse.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("ResolvePublicGameServerMap() headers = %+v", resolveResponse.Header())
		}

		insertTestGameServer(t, fixture, "server-local-2")
		_, errSecondShare := fixture.conn.GetOrCreateGameServerMapShare("server-local-2", "SecondSlug")
		if errSecondShare != nil {
			t.Fatalf("create second map share: %v", errSecondShare)
		}
		conflictRequest := connect.NewRequest(&xylona.UpdateGameServerMapShareSettingsRequest{
			GameServerId: "server-local-2", PublicIdentifier: "Minecraft_Map",
		})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, conflictRequest, "user-owner")
		_, errConflict := fixture.service.UpdateGameServerMapShareSettings(t.Context(), conflictRequest)
		if connect.CodeOf(errConflict) != connect.CodeAlreadyExists {
			t.Fatalf("UpdateGameServerMapShareSettings(conflict) code = %v, want %v", connect.CodeOf(errConflict), connect.CodeAlreadyExists)
		}

		_, errMalformed := fixture.service.ResolvePublicGameServerMap(t.Context(), connect.NewRequest(
			&xylona.ResolvePublicGameServerMapRequest{PublicIdentifier: "bad slug"},
		))
		if connect.CodeOf(errMalformed) != connect.CodeNotFound {
			t.Fatalf("ResolvePublicGameServerMap(malformed) code = %v, want %v", connect.CodeOf(errMalformed), connect.CodeNotFound)
		}
	})

	t.Run("lazy creation retries identifier conflicts", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		insertTestGameServer(t, fixture, "server-local-2")
		_, errTaken := fixture.conn.GetOrCreateGameServerMapShare("server-local-1", "TakenSlug")
		if errTaken != nil {
			t.Fatalf("create conflicting share: %v", errTaken)
		}
		identifiers := []string{"TakenSlug", "FreshSlug"}
		calls := 0
		fixture.service.statusPageIdentifier = func() (string, error) {
			identifier := identifiers[calls]
			calls++
			return identifier, nil
		}
		share, errCreate := fixture.service.getOrCreateGameServerMapShare("server-local-2")
		if errCreate != nil {
			t.Fatalf("getOrCreateGameServerMapShare() error = %v", errCreate)
		}
		if calls != 2 || share.PublicIdentifier != "FreshSlug" {
			t.Fatalf("getOrCreateGameServerMapShare() = %+v after %d calls", share, calls)
		}

		fixture.service.statusPageIdentifier = func() (string, error) { return "", errors.New("entropy unavailable") }
		_, errGenerate := fixture.service.getOrCreateGameServerMapShare("server-local-2")
		if errGenerate == nil {
			t.Fatal("getOrCreateGameServerMapShare(generator failure) error = nil")
		}
	})

	t.Run("rejects unsupported game settings", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		_, errGame := fixture.conn.SQLDb.ExecContext(
			t.Context(),
			`insert into game (id, name, default_port, default_query_port, default_max_players, windows_support)
			 values (?, ?, ?, ?, ?, ?)`,
			"unsupported", "Unsupported", 27015, 27015, 16, true,
		)
		if errGame != nil {
			t.Fatalf("insert unsupported game: %v", errGame)
		}
		_, errUpdate := fixture.conn.SQLDb.ExecContext(
			t.Context(),
			"update game_server set game_id = ? where id = ?",
			"unsupported",
			"server-local-1",
		)
		if errUpdate != nil {
			t.Fatalf("set unsupported game: %v", errUpdate)
		}
		request := connect.NewRequest(&xylona.GetOrCreateGameServerMapShareSettingsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
		_, errSettings := fixture.service.GetOrCreateGameServerMapShareSettings(t.Context(), request)
		if connect.CodeOf(errSettings) != connect.CodeFailedPrecondition {
			t.Fatalf("GetOrCreateGameServerMapShareSettings(unsupported) code = %v, want %v", connect.CodeOf(errSettings), connect.CodeFailedPrecondition)
		}
	})
}

func insertTestGameServer(t *testing.T, fixture *rbacRPCFixture, gameServerID string) {
	t.Helper()
	_, errInsert := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 select ?, user_id, ?, game_id, status, set_players, max_players, map, ip, port + 10, query_port + 10, ?, node_id, start_args_patches
		 from game_server where id = ?`,
		gameServerID,
		"Second Server",
		"/tmp/"+gameServerID,
		"server-local-1",
	)
	if errInsert != nil {
		t.Fatalf("insert test game server: %v", errInsert)
	}
}
