package rpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestSevenDaysToDieMapAuthorizationSharingAndLastKnownState(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	_, errGame := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		"update game_server set game_id = '7_days_to_die' where id = 'server-local-1'",
	)
	if errGame != nil {
		t.Fatalf("set 7 Days to Die game: %v", errGame)
	}

	grantRequest := connect.NewRequest(&xylona.GrantGameServerAccessRequest{
		GameServerId: "server-local-1",
		UserId:       "user-other",
		RoleId:       "viewer",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, grantRequest, "user-owner")
	_, errGrant := fixture.service.GrantGameServerAccess(t.Context(), grantRequest)
	if errGrant != nil {
		t.Fatalf("grant viewer role: %v", errGrant)
	}

	collectedAt := time.Date(2026, time.July, 19, 16, 0, 0, 0, time.UTC)
	stored := storedSevenDaysToDieMapSnapshot{
		Enabled:    true,
		TileSize:   128,
		MaxZoom:    4,
		MapSize:    node.SevenDaysToDieMapVector{X: 6144, Y: 255, Z: 6144},
		SourceTime: "Day 3, 14:20",
		Players: []storedSevenDaysToDieMapPlayer{
			{
				ID:         "Steam_123",
				Name:       "Alex",
				Online:     true,
				Position:   node.SevenDaysToDieMapVector{X: 10, Y: 40, Z: -20},
				LastSeenAt: collectedAt,
			},
		},
	}
	storedJSON, errMarshal := json.Marshal(stored)
	if errMarshal != nil {
		t.Fatalf("marshal cached map snapshot: %v", errMarshal)
	}
	errStore := fixture.conn.StoreGameServerSevenDaysToDieMapSnapshot("server-local-1", string(storedJSON), collectedAt)
	if errStore != nil {
		t.Fatalf("store cached map snapshot: %v", errStore)
	}

	viewerRequest := connect.NewRequest(&xylona.GetSevenDaysToDieMapRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerRequest, "user-other")
	viewerResponse, errViewer := fixture.service.GetSevenDaysToDieMap(t.Context(), viewerRequest)
	if errViewer != nil {
		t.Fatalf("GetSevenDaysToDieMap(viewer) error = %v", errViewer)
	}
	mapView := viewerResponse.Msg.GetMap()
	if !mapView.GetStale() || len(mapView.GetPlayers()) != 1 || mapView.GetPlayers()[0].GetOnline() {
		t.Fatalf("GetSevenDaysToDieMap(viewer) map = %+v, want an offline last-known player", mapView)
	}
	if mapView.GetPlayers()[0].GetId() != "" {
		t.Fatalf("GetSevenDaysToDieMap(viewer) player ID = %q, want redacted", mapView.GetPlayers()[0].GetId())
	}

	viewerNotesRequest := connect.NewRequest(&xylona.UpdateSevenDaysToDieMapNotesRequest{
		GameServerId: "server-local-1",
		Markers: []*xylona.SevenDaysToDieMapMarker{
			{Id: "base", Name: "Main base", Icon: "home", X: 100, Z: -50},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerNotesRequest, "user-other")
	_, errViewerNotes := fixture.service.UpdateSevenDaysToDieMapNotes(t.Context(), viewerNotesRequest)
	if connect.CodeOf(errViewerNotes) != connect.CodePermissionDenied {
		t.Fatalf("UpdateSevenDaysToDieMapNotes(viewer) code = %v, want %v", connect.CodeOf(errViewerNotes), connect.CodePermissionDenied)
	}

	ownerNotesRequest := connect.NewRequest(viewerNotesRequest.Msg)
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, ownerNotesRequest, "user-owner")
	_, errOwnerNotes := fixture.service.UpdateSevenDaysToDieMapNotes(t.Context(), ownerNotesRequest)
	if errOwnerNotes != nil {
		t.Fatalf("UpdateSevenDaysToDieMapNotes(owner) error = %v", errOwnerNotes)
	}

	settingsRequest := connect.NewRequest(&xylona.GetOrCreateGameServerMapShareSettingsRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, settingsRequest, "user-owner")
	_, errSettings := fixture.service.GetOrCreateGameServerMapShareSettings(t.Context(), settingsRequest)
	if errSettings != nil {
		t.Fatalf("GetOrCreateGameServerMapShareSettings(owner) error = %v", errSettings)
	}
	shareRequest := connect.NewRequest(&xylona.UpdateGameServerMapShareSettingsRequest{
		GameServerId: "server-local-1", PublicIdentifier: "Navezgane_Map", Enabled: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, shareRequest, "user-owner")
	_, errShare := fixture.service.UpdateGameServerMapShareSettings(t.Context(), shareRequest)
	if errShare != nil {
		t.Fatalf("UpdateGameServerMapShareSettings(owner) error = %v", errShare)
	}
	publicRequest := connect.NewRequest(&xylona.GetPublicSevenDaysToDieMapRequest{PublicIdentifier: "Navezgane_Map"})
	publicResponse, errPublic := fixture.service.GetPublicSevenDaysToDieMap(t.Context(), publicRequest)
	if errPublic != nil {
		t.Fatalf("GetPublicSevenDaysToDieMap() error = %v", errPublic)
	}
	publicMap := publicResponse.Msg.GetMap()
	if publicMap.GetGameServerName() != "Local One" || len(publicMap.GetMarkers()) != 1 || publicMap.GetMarkers()[0].GetName() != "Main base" {
		t.Fatalf("GetPublicSevenDaysToDieMap() map = %+v", publicMap)
	}
	if len(publicMap.GetPlayers()) != 1 || publicMap.GetPlayers()[0].GetId() != "" {
		t.Fatalf("GetPublicSevenDaysToDieMap() players = %+v, want redacted IDs", publicMap.GetPlayers())
	}
	resolved, errResolve := fixture.service.ResolvePublicGameServerMap(t.Context(), connect.NewRequest(
		&xylona.ResolvePublicGameServerMapRequest{PublicIdentifier: "Navezgane_Map"},
	))
	if errResolve != nil || resolved.Msg.GetKind() != xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_SEVEN_DAYS_TO_DIE {
		t.Fatalf("ResolvePublicGameServerMap() = %+v, %v", resolved, errResolve)
	}
	gameServer, errGameServer := fixture.conn.GetGameServerByID("server-local-1")
	if errGameServer != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGameServer)
	}
	tileRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	tileRequest.Header.Set(sevenDaysToDieMapShareHeader, "Navezgane_Map")
	errTileAccess := fixture.service.authorizeSevenDaysToDieMapTile(tileRequest, gameServer)
	if errTileAccess != nil {
		t.Fatalf("authorizeSevenDaysToDieMapTile(enabled share) error = %v", errTileAccess)
	}

	shareRequest.Msg.PublicIdentifier = "Navezgane_Renamed"
	_, errRename := fixture.service.UpdateGameServerMapShareSettings(t.Context(), shareRequest)
	if errRename != nil {
		t.Fatalf("UpdateGameServerMapShareSettings(rename) error = %v", errRename)
	}
	errRenamedTile := fixture.service.authorizeSevenDaysToDieMapTile(tileRequest, gameServer)
	if errRenamedTile == nil {
		t.Fatal("authorizeSevenDaysToDieMapTile(old identifier) error = nil")
	}
	tileRequest.Header.Set(sevenDaysToDieMapShareHeader, "Navezgane_Renamed")
	errRenamedTileAccess := fixture.service.authorizeSevenDaysToDieMapTile(tileRequest, gameServer)
	if errRenamedTileAccess != nil {
		t.Fatalf("authorizeSevenDaysToDieMapTile(renamed share) error = %v", errRenamedTileAccess)
	}

	shareRequest.Msg.Enabled = false
	_, errDisable := fixture.service.UpdateGameServerMapShareSettings(t.Context(), shareRequest)
	if errDisable != nil {
		t.Fatalf("UpdateGameServerMapShareSettings(disable) error = %v", errDisable)
	}
	publicRequest.Msg.PublicIdentifier = "Navezgane_Renamed"
	_, errRevoked := fixture.service.GetPublicSevenDaysToDieMap(t.Context(), publicRequest)
	if connect.CodeOf(errRevoked) != connect.CodeNotFound {
		t.Fatalf("GetPublicSevenDaysToDieMap(revoked) code = %v, want %v", connect.CodeOf(errRevoked), connect.CodeNotFound)
	}
	errRevokedTile := fixture.service.authorizeSevenDaysToDieMapTile(tileRequest, gameServer)
	if errRevokedTile == nil {
		t.Fatal("authorizeSevenDaysToDieMapTile(disabled share) error = nil")
	}
}
