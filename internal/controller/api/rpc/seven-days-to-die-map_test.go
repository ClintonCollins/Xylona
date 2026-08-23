package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
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
	viewerPlayerID := mapView.GetPlayers()[0].GetId()
	if viewerPlayerID == "" || viewerPlayerID == "Steam_123" {
		t.Fatalf("GetSevenDaysToDieMap(viewer) player ID = %q, want opaque ID", viewerPlayerID)
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
	if len(publicMap.GetPlayers()) != 1 || publicMap.GetPlayers()[0].GetId() != viewerPlayerID {
		t.Fatalf("GetPublicSevenDaysToDieMap() players = %+v, want stable opaque ID %q", publicMap.GetPlayers(), viewerPlayerID)
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

func TestSevenDaysToDieMapTacticalProjectionAndFailureClearing(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	_, errGame := fixture.conn.SQLDb.ExecContext(
		t.Context(), "update game_server set game_id = '7_days_to_die' where id = 'server-local-1'",
	)
	if errGame != nil {
		t.Fatalf("set 7 Days to Die game: %v", errGame)
	}
	grantRequest := connect.NewRequest(&xylona.GrantGameServerAccessRequest{
		GameServerId: "server-local-1", UserId: "user-other", RoleId: "viewer",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, grantRequest, "user-owner")
	_, errGrant := fixture.service.GrantGameServerAccess(t.Context(), grantRequest)
	if errGrant != nil {
		t.Fatalf("grant viewer role: %v", errGrant)
	}

	available := node.SevenDaysToDieWebAPIValueStateAvailable
	fakeNode := &nodeclient.FakeNodeClient{
		NodeID: "node-local",
		QuerySevenDaysToDieMapResult: &node.SevenDaysToDieMapSnapshot{
			Enabled: true, TileSize: 128, MaxZoom: 4,
			MapSize:           node.SevenDaysToDieMapVector{X: 6144, Y: 255, Z: 6144},
			Players:           []node.SevenDaysToDieMapPlayer{{ID: "private-player", Name: "Alex", Online: true}},
			NativeMarkerState: available,
			NativeMarkers: []node.SevenDaysToDieMapMarker{{
				ID: "private-marker", Name: "Secret marker", Position: node.SevenDaysToDieMapVector{X: 10, Z: 20},
			}},
			ClaimsState: available,
			Claims: []node.SevenDaysToDieLandClaim{{
				OwnerID: "private-owner", OwnerName: "Secret owner", Active: true,
				Position: node.SevenDaysToDieMapVector{X: 30, Y: 5, Z: 40}, Size: 41,
			}},
			BloodMoonState: available,
			BloodMoon: &node.SevenDaysToDieBloodMoon{
				GameTime: node.SevenDaysToDieGameTime{Day: 7, Hour: 22}, Active: true,
				NextBloodMoon:    node.SevenDaysToDieGameTime{Day: 14, Hour: 22},
				NextBloodMoonEnd: node.SevenDaysToDieGameTime{Day: 15, Hour: 4},
			},
			HostileState: available,
			Hostiles:     []node.SevenDaysToDieMapEntity{{Name: "Secret zombie", Position: node.SevenDaysToDieMapVector{X: 50, Z: 60}}},
			AnimalState:  available,
			Animals:      []node.SevenDaysToDieMapEntity{{Name: "Secret wolf", Position: node.SevenDaysToDieMapVector{X: 70, Z: 80}}},
		},
	}
	registry := noderegistry.New("node-local", fakeNode)
	fixture.service.nodeRegistry = registry
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(), fixture.conn, fakeNode, registry, nil,
		versiontracker.NewVersionStateMap(), versiontracker.ResolverConfig{},
	)

	ownerRequest := connect.NewRequest(&xylona.GetSevenDaysToDieMapRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, ownerRequest, "user-owner")
	ownerResponse, errOwner := fixture.service.GetSevenDaysToDieMap(t.Context(), ownerRequest)
	if errOwner != nil {
		t.Fatalf("GetSevenDaysToDieMap(owner) error = %v", errOwner)
	}
	ownerMap := ownerResponse.Msg.GetMap()
	if len(ownerMap.GetNativeMarkers()) != 1 || len(ownerMap.GetClaims()) != 1 || ownerMap.GetBloodMoon() == nil ||
		len(ownerMap.GetHostiles()) != 1 || len(ownerMap.GetAnimals()) != 1 ||
		ownerMap.GetNativeMarkers()[0].GetId() != "private-marker" || ownerMap.GetClaims()[0].GetOwnerId() != "private-owner" {
		t.Fatalf("GetSevenDaysToDieMap(owner) tactical map = %+v", ownerMap)
	}

	viewerRequest := connect.NewRequest(&xylona.GetSevenDaysToDieMapRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerRequest, "user-other")
	viewerResponse, errViewer := fixture.service.GetSevenDaysToDieMap(t.Context(), viewerRequest)
	if errViewer != nil {
		t.Fatalf("GetSevenDaysToDieMap(viewer) error = %v", errViewer)
	}
	assertSevenDaysToDieTacticalMapRedacted(t, viewerResponse.Msg.GetMap())

	settingsRequest := connect.NewRequest(&xylona.GetOrCreateGameServerMapShareSettingsRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, settingsRequest, "user-owner")
	_, errSettings := fixture.service.GetOrCreateGameServerMapShareSettings(t.Context(), settingsRequest)
	if errSettings != nil {
		t.Fatalf("create map share settings: %v", errSettings)
	}
	shareRequest := connect.NewRequest(&xylona.UpdateGameServerMapShareSettingsRequest{
		GameServerId: "server-local-1", PublicIdentifier: "Private_Tactical_Test", Enabled: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, shareRequest, "user-owner")
	_, errShare := fixture.service.UpdateGameServerMapShareSettings(t.Context(), shareRequest)
	if errShare != nil {
		t.Fatalf("enable map share: %v", errShare)
	}
	publicResponse, errPublic := fixture.service.GetPublicSevenDaysToDieMap(t.Context(), connect.NewRequest(
		&xylona.GetPublicSevenDaysToDieMapRequest{PublicIdentifier: "Private_Tactical_Test"},
	))
	if errPublic != nil {
		t.Fatalf("GetPublicSevenDaysToDieMap() error = %v", errPublic)
	}
	publicMap := publicResponse.Msg.GetMap()
	publicAvailable := xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
	publicUnspecified := xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED
	if len(publicMap.GetNativeMarkers()) != 0 || publicMap.GetNativeMarkerState() != publicUnspecified ||
		len(publicMap.GetClaims()) != 1 ||
		publicMap.GetClaims()[0].GetOwnerName() != "Secret owner" || publicMap.GetClaims()[0].GetOwnerId() != "" ||
		publicMap.GetBloodMoon() == nil || len(publicMap.GetHostiles()) != 1 || len(publicMap.GetAnimals()) != 1 ||
		!publicMap.GetClaimsSupported() || publicMap.GetClaimsState() != publicAvailable ||
		publicMap.GetBloodMoonState() != publicAvailable ||
		publicMap.GetHostileState() != publicAvailable || publicMap.GetAnimalState() != publicAvailable {
		t.Fatalf("GetPublicSevenDaysToDieMap() tactical map = %+v", publicMap)
	}
	if len(fakeNode.QuerySevenDaysToDieMapCalls) != 3 || !fakeNode.QuerySevenDaysToDieMapCalls[0].IncludeTactical ||
		fakeNode.QuerySevenDaysToDieMapCalls[1].IncludeTactical || !fakeNode.QuerySevenDaysToDieMapCalls[2].IncludeTactical {
		t.Fatalf("map tactical request flags = %+v, want owner/public true and viewer false", fakeNode.QuerySevenDaysToDieMapCalls)
	}

	viewerWire, errViewerMarshal := proto.Marshal(viewerResponse.Msg)
	if errViewerMarshal != nil {
		t.Fatalf("marshal viewer response: %v", errViewerMarshal)
	}
	for _, secret := range []string{"private-marker", "Secret marker", "Secret owner", "private-owner", "Secret zombie", "Secret wolf"} {
		if bytes.Contains(viewerWire, []byte(secret)) {
			t.Fatalf("viewer response wire contains %q", secret)
		}
	}
	for _, response := range []proto.Message{viewerResponse.Msg, publicResponse.Msg} {
		wire, errMarshal := proto.Marshal(response)
		if errMarshal != nil {
			t.Fatalf("marshal map response: %v", errMarshal)
		}
		for _, secret := range []string{"private-player", "private-marker", "private-owner"} {
			if bytes.Contains(wire, []byte(secret)) {
				t.Fatalf("map response wire contains raw ID %q", secret)
			}
		}
		if response == publicResponse.Msg && bytes.Contains(wire, []byte("Secret marker")) {
			t.Fatal("public map response contains native marker")
		}
		for _, call := range fakeNode.QuerySevenDaysToDieMapCalls {
			if call.TokenName != "" && bytes.Contains(wire, []byte(call.TokenName)) {
				t.Fatal("redacted response contains native token name")
			}
			if call.TokenSecret != "" && bytes.Contains(wire, []byte(call.TokenSecret)) {
				t.Fatal("redacted response contains native token secret")
			}
		}
	}

	fakeNode.QuerySevenDaysToDieMapResult = &node.SevenDaysToDieMapSnapshot{
		Enabled: true, TileSize: 128, MaxZoom: 4,
		MapSize: node.SevenDaysToDieMapVector{X: 6144, Y: 255, Z: 6144},
		Players: []node.SevenDaysToDieMapPlayer{{ID: "old-player", Name: "Alex", Online: true}},
		NativeMarkers: []node.SevenDaysToDieMapMarker{{
			ID: "old-marker", Name: "Old marker", Position: node.SevenDaysToDieMapVector{X: 10, Z: 20},
		}},
		Claims: []node.SevenDaysToDieLandClaim{{
			OwnerID: "old-owner", OwnerName: "Old owner", Position: node.SevenDaysToDieMapVector{X: 30, Z: 40},
		}},
		BloodMoon: &node.SevenDaysToDieBloodMoon{
			GameTime:      node.SevenDaysToDieGameTime{Day: 7, Hour: 22},
			NextBloodMoon: node.SevenDaysToDieGameTime{Day: 14, Hour: 22}, NextBloodMoonEnd: node.SevenDaysToDieGameTime{Day: 15, Hour: 4},
		},
		Hostiles: []node.SevenDaysToDieMapEntity{{Name: "Old zombie", Position: node.SevenDaysToDieMapVector{X: 50, Z: 60}}},
		Animals:  []node.SevenDaysToDieMapEntity{{Name: "Old wolf", Position: node.SevenDaysToDieMapVector{X: 70, Z: 80}}},
	}
	oldNodeResponse, errOldNode := fixture.service.GetSevenDaysToDieMap(t.Context(), ownerRequest)
	if errOldNode != nil {
		t.Fatalf("GetSevenDaysToDieMap(old node) error = %v", errOldNode)
	}
	if len(oldNodeResponse.Msg.GetMap().GetPlayers()) == 0 {
		t.Fatalf("old-node base map = %+v", oldNodeResponse.Msg.GetMap())
	}
	assertSevenDaysToDieTacticalMapUnavailable(t, oldNodeResponse.Msg.GetMap())

	fakeNode.QuerySevenDaysToDieMapErr = errors.New("node disconnected")
	fallbackResponse, errFallback := fixture.service.GetSevenDaysToDieMap(t.Context(), ownerRequest)
	if errFallback != nil {
		t.Fatalf("GetSevenDaysToDieMap(fallback) error = %v", errFallback)
	}
	if !fallbackResponse.Msg.GetMap().GetStale() || len(fallbackResponse.Msg.GetMap().GetPlayers()) == 0 {
		t.Fatalf("fallback map = %+v, want cached base/player snapshot", fallbackResponse.Msg.GetMap())
	}
	assertSevenDaysToDieTacticalMapUnavailable(t, fallbackResponse.Msg.GetMap())
	if len(fakeNode.QuerySevenDaysToDieMapCalls) != 5 || !fakeNode.QuerySevenDaysToDieMapCalls[4].IncludeTactical {
		t.Fatalf("fallback map request flags = %+v, want tactical retry", fakeNode.QuerySevenDaysToDieMapCalls)
	}
}

func assertSevenDaysToDieTacticalMapUnavailable(t *testing.T, mapView *xylona.SevenDaysToDieMapView) {
	t.Helper()
	unavailable := xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE
	if len(mapView.GetNativeMarkers()) != 0 || len(mapView.GetClaims()) != 0 || mapView.GetBloodMoon() != nil ||
		len(mapView.GetHostiles()) != 0 || len(mapView.GetAnimals()) != 0 || mapView.GetClaimsSupported() ||
		mapView.GetNativeMarkerState() != unavailable || mapView.GetClaimsState() != unavailable ||
		mapView.GetBloodMoonState() != unavailable || mapView.GetHostileState() != unavailable ||
		mapView.GetAnimalState() != unavailable {
		t.Fatalf("tactical map was not explicitly unavailable: %+v", mapView)
	}
}

func assertSevenDaysToDieTacticalMapRedacted(t *testing.T, mapView *xylona.SevenDaysToDieMapView) {
	t.Helper()
	if len(mapView.GetNativeMarkers()) != 0 || len(mapView.GetClaims()) != 0 || mapView.GetBloodMoon() != nil ||
		len(mapView.GetHostiles()) != 0 || len(mapView.GetAnimals()) != 0 || mapView.GetClaimsSupported() ||
		mapView.GetNativeMarkerState() != xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED ||
		mapView.GetClaimsState() != xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED ||
		mapView.GetBloodMoonState() != xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED ||
		mapView.GetHostileState() != xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED ||
		mapView.GetAnimalState() != xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED {
		t.Fatalf("tactical map was not redacted: %+v", mapView)
	}
}
