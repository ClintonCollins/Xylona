package rpc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestMinecraftMapAuthorizationLifecycleAndSharing(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fakeNode := &nodeclient.FakeNodeClient{
		NodeID: "node-local",
		RuntimeCapabilitiesResult: node.RuntimeCapabilities{
			ProtocolVersion: minecraftMapNodeProtocol,
			MinecraftMap:    true,
		},
		EnsureMinecraftMapResult: node.MinecraftMapStatus{
			Installed:            true,
			Running:              true,
			Ready:                true,
			Provider:             "managed",
			Status:               "ready",
			StatusMessage:        "Live map online.",
			BlueMapVersion:       "5.16",
			LivePlayersAvailable: true,
		},
	}
	fixture.service.nodeRegistry = noderegistry.New("node-local", fakeNode)

	grantRequest := connect.NewRequest(&xylona.GrantGameServerAccessRequest{
		GameServerId: "server-local-1",
		UserId:       "user-other",
		RoleId:       "viewer",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, grantRequest, "user-owner")
	_, errGrant := fixture.service.GrantGameServerAccess(t.Context(), grantRequest)
	if errGrant != nil {
		t.Fatalf("grant Minecraft map viewer role: %v", errGrant)
	}

	missingAcceptance := connect.NewRequest(&xylona.UpdateMinecraftMapConfigRequest{
		GameServerId: "server-local-1", Enabled: true, WorldName: "world",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, missingAcceptance, "user-owner")
	_, errAcceptance := fixture.service.UpdateMinecraftMapConfig(t.Context(), missingAcceptance)
	if connect.CodeOf(errAcceptance) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateMinecraftMapConfig(without acceptance) code = %v, want %v", connect.CodeOf(errAcceptance), connect.CodeInvalidArgument)
	}

	enableRequest := connect.NewRequest(&xylona.UpdateMinecraftMapConfigRequest{
		GameServerId: "server-local-1", Enabled: true, WorldName: "world", AcceptBluemapDownload: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, enableRequest, "user-owner")
	enableResponse, errEnable := fixture.service.UpdateMinecraftMapConfig(t.Context(), enableRequest)
	if errEnable != nil {
		t.Fatalf("UpdateMinecraftMapConfig(owner) error = %v", errEnable)
	}
	if !enableResponse.Msg.GetMap().GetAvailable() ||
		enableResponse.Msg.GetMap().GetProvider() != "managed" ||
		!enableResponse.Msg.GetMap().GetBluemapDownloadAccepted() {
		t.Fatalf("UpdateMinecraftMapConfig(owner) map = %+v", enableResponse.Msg.GetMap())
	}

	viewerRequest := connect.NewRequest(&xylona.GetMinecraftMapRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerRequest, "user-other")
	viewerResponse, errViewer := fixture.service.GetMinecraftMap(t.Context(), viewerRequest)
	if errViewer != nil {
		t.Fatalf("GetMinecraftMap(viewer) error = %v", errViewer)
	}
	viewerMap := viewerResponse.Msg.GetMap()
	if viewerMap.GetCanManage() || viewerMap.GetViewerUrl() != MinecraftMapViewerPathPrefix+"/server-local-1/" {
		t.Fatalf("GetMinecraftMap(viewer) map = %+v", viewerMap)
	}
	if len(fakeNode.EnsureMinecraftMapCalls) == 0 || fakeNode.EnsureMinecraftMapCalls[0].WorldName != "world" {
		t.Fatalf("EnsureMinecraftMap calls = %+v", fakeNode.EnsureMinecraftMapCalls)
	}

	viewerUpdate := connect.NewRequest(&xylona.UpdateMinecraftMapConfigRequest{
		GameServerId: "server-local-1", Enabled: false, WorldName: "world",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerUpdate, "user-other")
	_, errViewerUpdate := fixture.service.UpdateMinecraftMapConfig(t.Context(), viewerUpdate)
	if connect.CodeOf(errViewerUpdate) != connect.CodePermissionDenied {
		t.Fatalf("UpdateMinecraftMapConfig(viewer) code = %v, want %v", connect.CodeOf(errViewerUpdate), connect.CodePermissionDenied)
	}

	shareRequest := connect.NewRequest(&xylona.RegenerateMinecraftMapShareRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, shareRequest, "user-owner")
	shareResponse, errShare := fixture.service.RegenerateMinecraftMapShare(t.Context(), shareRequest)
	if errShare != nil {
		t.Fatalf("RegenerateMinecraftMapShare(owner) error = %v", errShare)
	}
	publicRequest := connect.NewRequest(&xylona.GetPublicMinecraftMapRequest{ShareToken: shareResponse.Msg.GetShareToken()})
	publicResponse, errPublic := fixture.service.GetPublicMinecraftMap(t.Context(), publicRequest)
	if errPublic != nil {
		t.Fatalf("GetPublicMinecraftMap() error = %v", errPublic)
	}
	publicMap := publicResponse.Msg.GetMap()
	if publicMap.GetGameServerName() != "Local One" || publicMap.GetViewerUrl() != MinecraftMapSharedPathPrefix+"/server-local-1/" || publicMap.GetCanManage() {
		t.Fatalf("GetPublicMinecraftMap() map = %+v", publicMap)
	}
	if !strings.Contains(publicResponse.Header().Get("Set-Cookie"), minecraftMapShareCookieName+"=") {
		t.Fatalf("GetPublicMinecraftMap() Set-Cookie = %q", publicResponse.Header().Get("Set-Cookie"))
	}

	revokeRequest := connect.NewRequest(&xylona.RevokeMinecraftMapShareRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, revokeRequest, "user-owner")
	_, errRevoke := fixture.service.RevokeMinecraftMapShare(t.Context(), revokeRequest)
	if errRevoke != nil {
		t.Fatalf("RevokeMinecraftMapShare(owner) error = %v", errRevoke)
	}
	_, errRevoked := fixture.service.GetPublicMinecraftMap(t.Context(), publicRequest)
	if connect.CodeOf(errRevoked) != connect.CodeNotFound {
		t.Fatalf("GetPublicMinecraftMap(revoked) code = %v, want %v", connect.CodeOf(errRevoked), connect.CodeNotFound)
	}
}

func TestUpdateMinecraftMapConfigRejectsInvalidRCONPort(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	_, errPort := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		"update game_server set query_port = ? where id = ?",
		65535,
		"server-local-1",
	)
	if errPort != nil {
		t.Fatalf("set query port: %v", errPort)
	}
	request := connect.NewRequest(&xylona.UpdateMinecraftMapConfigRequest{
		GameServerId:          "server-local-1",
		Enabled:               true,
		WorldName:             "world",
		AcceptBluemapDownload: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	_, errUpdate := fixture.service.UpdateMinecraftMapConfig(t.Context(), request)
	if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateMinecraftMapConfig() code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
	}
}

func TestMinecraftMapAssetUsesCurrentAuthorizationAndSandboxHeaders(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	errEnable := fixture.conn.UpdateGameServerMinecraftMapConfig("server-local-1", true, "world", true, "user-owner")
	if errEnable != nil {
		t.Fatalf("enable Minecraft map: %v", errEnable)
	}
	fakeNode := &nodeclient.FakeNodeClient{
		NodeID: "node-local",
		MinecraftMapAssetResult: node.MinecraftMapAsset{
			Content:      []byte("<html>BlueMap</html>"),
			ContentType:  "text/html; charset=utf-8",
			CacheControl: "public, max-age=31536000, immutable",
		},
	}
	fixture.service.nodeRegistry = noderegistry.New("node-local", fakeNode)
	grantRequest := connect.NewRequest(&xylona.GrantGameServerAccessRequest{
		GameServerId: "server-local-1",
		UserId:       "user-other",
		RoleId:       "viewer",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, grantRequest, "user-owner")
	grantResponse, errGrant := fixture.service.GrantGameServerAccess(t.Context(), grantRequest)
	if errGrant != nil {
		t.Fatalf("GrantGameServerAccess() error = %v", errGrant)
	}
	router := chi.NewRouter()
	router.Get(MinecraftMapViewerPathPrefix+"/{gameServerId}/*", fixture.service.MinecraftMapAsset)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, MinecraftMapViewerPathPrefix+"/server-local-1/index.html", nil)
	addSessionCookieHeaderHTTP(t, fixture.conn, fixture.secureCookie, request, "user-other")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "<html>BlueMap</html>" {
		t.Fatalf("MinecraftMapAsset() response = %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Frame-Options") != "SAMEORIGIN" || response.Header().Get("Referrer-Policy") != "no-referrer" || !strings.Contains(response.Header().Get("Content-Security-Policy"), "sandbox allow-same-origin allow-scripts") {
		t.Fatalf("MinecraftMapAsset() security headers = %+v", response.Header())
	}
	if response.Header().Get("Cache-Control") != "private, max-age=300" {
		t.Fatalf("MinecraftMapAsset() Cache-Control = %q, want private cache", response.Header().Get("Cache-Control"))
	}
	if len(fakeNode.MinecraftMapAssetCalls) != 1 || fakeNode.MinecraftMapAssetCalls[0].AssetPath != "index.html" {
		t.Fatalf("MinecraftMapAsset calls = %+v", fakeNode.MinecraftMapAssetCalls)
	}
	revokeAccessRequest := connect.NewRequest(&xylona.RevokeGameServerAccessRequest{
		GrantId:      grantResponse.Msg.GetGrant().GetId(),
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, revokeAccessRequest, "user-owner")
	_, errRevokeAccess := fixture.service.RevokeGameServerAccess(t.Context(), revokeAccessRequest)
	if errRevokeAccess != nil {
		t.Fatalf("RevokeGameServerAccess() error = %v", errRevokeAccess)
	}
	revokedPrivateRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, MinecraftMapViewerPathPrefix+"/server-local-1/index.html", nil)
	addSessionCookieHeaderHTTP(t, fixture.conn, fixture.secureCookie, revokedPrivateRequest, "user-other")
	revokedPrivateResponse := httptest.NewRecorder()
	router.ServeHTTP(revokedPrivateResponse, revokedPrivateRequest)
	if revokedPrivateResponse.Code != http.StatusNotFound {
		t.Fatalf("MinecraftMapAsset(revoked viewer) status = %d, want %d", revokedPrivateResponse.Code, http.StatusNotFound)
	}

	badRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, MinecraftMapViewerPathPrefix+"/server-local-1/index.html", nil)
	badResponse := httptest.NewRecorder()
	router.ServeHTTP(badResponse, badRequest)
	if badResponse.Code != http.StatusNotFound {
		t.Fatalf("MinecraftMapAsset(invalid grant) status = %d, want %d", badResponse.Code, http.StatusNotFound)
	}

	shareToken, errShare := fixture.conn.RegenerateGameServerMinecraftMapShare("server-local-1", "user-owner")
	if errShare != nil {
		t.Fatalf("RegenerateGameServerMinecraftMapShare() error = %v", errShare)
	}
	sharedRouter := chi.NewRouter()
	sharedRouter.Get(MinecraftMapSharedPathPrefix+"/{gameServerId}/*", fixture.service.MinecraftMapAsset)
	publicRequest := connect.NewRequest(&xylona.GetPublicMinecraftMapRequest{ShareToken: shareToken})
	publicResponse, errPublic := fixture.service.GetPublicMinecraftMap(t.Context(), publicRequest)
	if errPublic != nil {
		t.Fatalf("GetPublicMinecraftMap() error = %v", errPublic)
	}
	shareCookie := strings.Split(publicResponse.Header().Get("Set-Cookie"), ";")[0]
	sharedRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, MinecraftMapSharedPathPrefix+"/server-local-1/assets/app.js", nil)
	sharedRequest.Header.Set("Cookie", shareCookie)
	sharedResponse := httptest.NewRecorder()
	sharedRouter.ServeHTTP(sharedResponse, sharedRequest)
	if sharedResponse.Code != http.StatusOK || sharedResponse.Header().Get("Cache-Control") != "private, max-age=300" {
		t.Fatalf("MinecraftMapAsset(shared) response = %d cache %q", sharedResponse.Code, sharedResponse.Header().Get("Cache-Control"))
	}
	errRevoke := fixture.conn.RevokeGameServerMinecraftMapShare("server-local-1", "user-owner")
	if errRevoke != nil {
		t.Fatalf("RevokeGameServerMinecraftMapShare() error = %v", errRevoke)
	}
	revokedResponse := httptest.NewRecorder()
	sharedRouter.ServeHTTP(revokedResponse, sharedRequest)
	if revokedResponse.Code != http.StatusNotFound {
		t.Fatalf("MinecraftMapAsset(revoked share) status = %d, want %d", revokedResponse.Code, http.StatusNotFound)
	}
}
