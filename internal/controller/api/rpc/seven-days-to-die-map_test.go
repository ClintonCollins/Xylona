package rpc

import (
	"encoding/json"
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

	shareRequest := connect.NewRequest(&xylona.RegenerateSevenDaysToDieMapShareRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, shareRequest, "user-owner")
	shareResponse, errShare := fixture.service.RegenerateSevenDaysToDieMapShare(t.Context(), shareRequest)
	if errShare != nil {
		t.Fatalf("RegenerateSevenDaysToDieMapShare(owner) error = %v", errShare)
	}
	secondShareRequest := connect.NewRequest(shareRequest.Msg)
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, secondShareRequest, "user-owner")
	secondShareResponse, errSecondShare := fixture.service.RegenerateSevenDaysToDieMapShare(t.Context(), secondShareRequest)
	if errSecondShare != nil {
		t.Fatalf("RegenerateSevenDaysToDieMapShare(owner, second) error = %v", errSecondShare)
	}
	listRequest := connect.NewRequest(&xylona.ListSevenDaysToDieMapSharesRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listRequest, "user-owner")
	listResponse, errList := fixture.service.ListSevenDaysToDieMapShares(t.Context(), listRequest)
	if errList != nil {
		t.Fatalf("ListSevenDaysToDieMapShares(owner) error = %v", errList)
	}
	if len(listResponse.Msg.GetShares()) != 2 {
		t.Fatalf("ListSevenDaysToDieMapShares(owner) count = %d, want 2", len(listResponse.Msg.GetShares()))
	}
	viewerListRequest := connect.NewRequest(listRequest.Msg)
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerListRequest, "user-other")
	_, errViewerList := fixture.service.ListSevenDaysToDieMapShares(t.Context(), viewerListRequest)
	if connect.CodeOf(errViewerList) != connect.CodePermissionDenied {
		t.Fatalf("ListSevenDaysToDieMapShares(viewer) code = %v, want %v", connect.CodeOf(errViewerList), connect.CodePermissionDenied)
	}
	publicRequest := connect.NewRequest(&xylona.GetPublicSevenDaysToDieMapRequest{ShareToken: shareResponse.Msg.GetShareToken()})
	publicResponse, errPublic := fixture.service.GetPublicSevenDaysToDieMap(t.Context(), publicRequest)
	if errPublic != nil {
		t.Fatalf("GetPublicSevenDaysToDieMap() error = %v", errPublic)
	}
	publicMap := publicResponse.Msg.GetMap()
	if publicMap.GetGameServerName() != "Local One" || len(publicMap.GetMarkers()) != 1 || publicMap.GetMarkers()[0].GetName() != "Main base" {
		t.Fatalf("GetPublicSevenDaysToDieMap() map = %+v", publicMap)
	}

	revokeRequest := connect.NewRequest(&xylona.RevokeSevenDaysToDieMapShareRequest{
		GameServerId: "server-local-1",
		ShareId:      shareResponse.Msg.GetShare().GetId(),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, revokeRequest, "user-owner")
	_, errRevoke := fixture.service.RevokeSevenDaysToDieMapShare(t.Context(), revokeRequest)
	if errRevoke != nil {
		t.Fatalf("RevokeSevenDaysToDieMapShare(owner) error = %v", errRevoke)
	}
	_, errRevoked := fixture.service.GetPublicSevenDaysToDieMap(t.Context(), publicRequest)
	if connect.CodeOf(errRevoked) != connect.CodeNotFound {
		t.Fatalf("GetPublicSevenDaysToDieMap(revoked) code = %v, want %v", connect.CodeOf(errRevoked), connect.CodeNotFound)
	}
	secondPublicRequest := connect.NewRequest(&xylona.GetPublicSevenDaysToDieMapRequest{ShareToken: secondShareResponse.Msg.GetShareToken()})
	_, errSecondPublic := fixture.service.GetPublicSevenDaysToDieMap(t.Context(), secondPublicRequest)
	if errSecondPublic != nil {
		t.Fatalf("GetPublicSevenDaysToDieMap(remaining share) error = %v", errSecondPublic)
	}
}
