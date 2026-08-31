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
	if viewerMap.GetCanManage() {
		t.Fatalf("GetMinecraftMap(viewer) map = %+v", viewerMap)
	}
	assertMinecraftMapViewerURL(t, viewerMap.GetViewerUrl(), MinecraftMapViewerPathPrefix)
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

	settingsRequest := connect.NewRequest(&xylona.GetOrCreateGameServerMapShareSettingsRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, settingsRequest, "user-owner")
	_, errSettings := fixture.service.GetOrCreateGameServerMapShareSettings(t.Context(), settingsRequest)
	if errSettings != nil {
		t.Fatalf("GetOrCreateGameServerMapShareSettings(owner) error = %v", errSettings)
	}
	shareRequest := connect.NewRequest(&xylona.UpdateGameServerMapShareSettingsRequest{
		GameServerId: "server-local-1", PublicIdentifier: "Minecraft_Map", Enabled: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, shareRequest, "user-owner")
	_, errShare := fixture.service.UpdateGameServerMapShareSettings(t.Context(), shareRequest)
	if errShare != nil {
		t.Fatalf("UpdateGameServerMapShareSettings(owner) error = %v", errShare)
	}
	publicRequest := connect.NewRequest(&xylona.GetPublicMinecraftMapRequest{PublicIdentifier: "Minecraft_Map"})
	publicResponse, errPublic := fixture.service.GetPublicMinecraftMap(t.Context(), publicRequest)
	if errPublic != nil {
		t.Fatalf("GetPublicMinecraftMap() error = %v", errPublic)
	}
	publicMap := publicResponse.Msg.GetMap()
	if publicMap.GetGameServerName() != "Local One" || publicMap.GetCanManage() {
		t.Fatalf("GetPublicMinecraftMap() map = %+v", publicMap)
	}
	assertMinecraftMapViewerURL(t, publicMap.GetViewerUrl(), MinecraftMapSharedPathPrefix)

	disableMapRequest := connect.NewRequest(&xylona.UpdateMinecraftMapConfigRequest{
		GameServerId: "server-local-1", Enabled: false, WorldName: "world",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, disableMapRequest, "user-owner")
	_, errDisable := fixture.service.UpdateMinecraftMapConfig(t.Context(), disableMapRequest)
	if errDisable != nil {
		t.Fatalf("UpdateMinecraftMapConfig(disable) error = %v", errDisable)
	}
	disabledShare, errDisabledShare := fixture.conn.GetGameServerMapShareByGameServerID("server-local-1")
	if errDisabledShare != nil || disabledShare.Enabled || disabledShare.PublicIdentifier != "Minecraft_Map" {
		t.Fatalf("Minecraft map share after config disable = %+v, %v", disabledShare, errDisabledShare)
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
		RuntimeCapabilitiesResult: node.RuntimeCapabilities{
			ProtocolVersion: minecraftMapNodeProtocol,
			MinecraftMap:    true,
		},
		EnsureMinecraftMapResult: node.MinecraftMapStatus{
			Installed:     true,
			Running:       true,
			Ready:         true,
			Provider:      "managed",
			Status:        "ready",
			StatusMessage: "Live map online.",
		},
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
	viewerRequest := connect.NewRequest(&xylona.GetMinecraftMapRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerRequest, "user-other")
	viewerResponse, errViewer := fixture.service.GetMinecraftMap(t.Context(), viewerRequest)
	if errViewer != nil {
		t.Fatalf("GetMinecraftMap() error = %v", errViewer)
	}
	viewerURL := viewerResponse.Msg.GetMap().GetViewerUrl()
	assertMinecraftMapViewerURL(t, viewerURL, MinecraftMapViewerPathPrefix)

	router := chi.NewRouter()
	router.Get(MinecraftMapViewerPathPrefix+"/{gameServerId}/{token}/*", fixture.service.MinecraftMapAsset)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, viewerURL+"index.html", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "<html>BlueMap</html>" {
		t.Fatalf("MinecraftMapAsset() response = %d %q", response.Code, response.Body.String())
	}
	csp := response.Header().Get("Content-Security-Policy")
	if response.Header().Get("X-Frame-Options") != "SAMEORIGIN" ||
		response.Header().Get("Referrer-Policy") != "no-referrer" ||
		response.Header().Get("Access-Control-Allow-Origin") != "*" ||
		response.Header().Get("Cross-Origin-Resource-Policy") != "cross-origin" ||
		!strings.Contains(csp, "sandbox allow-scripts") ||
		strings.Contains(csp, "allow-same-origin") ||
		strings.Contains(csp, "default-src 'self'") {
		t.Fatalf("MinecraftMapAsset() security headers = %+v", response.Header())
	}
	if response.Header().Get("Cache-Control") != "private, max-age=300" {
		t.Fatalf("MinecraftMapAsset() Cache-Control = %q, want private cache", response.Header().Get("Cache-Control"))
	}

	tileRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, viewerURL+"maps/world/0/0/0.png", nil)
	tileResponse := httptest.NewRecorder()
	router.ServeHTTP(tileResponse, tileRequest)
	if tileResponse.Code != http.StatusOK {
		t.Fatalf("MinecraftMapAsset(relative tile) status = %d, want %d", tileResponse.Code, http.StatusOK)
	}
	if len(fakeNode.MinecraftMapAssetCalls) != 2 ||
		fakeNode.MinecraftMapAssetCalls[0].AssetPath != "index.html" ||
		fakeNode.MinecraftMapAssetCalls[1].AssetPath != "maps/world/0/0/0.png" {
		t.Fatalf("MinecraftMapAsset calls = %+v", fakeNode.MinecraftMapAssetCalls)
	}

	sessionWithoutToken := httptest.NewRequestWithContext(t.Context(), http.MethodGet, MinecraftMapViewerPathPrefix+"/server-local-1/index.html", nil)
	addSessionCookieHeaderHTTP(t, fixture.conn, fixture.secureCookie, sessionWithoutToken, "user-other")
	sessionWithoutTokenResponse := httptest.NewRecorder()
	router.ServeHTTP(sessionWithoutTokenResponse, sessionWithoutToken)
	if sessionWithoutTokenResponse.Code != http.StatusNotFound {
		t.Fatalf("MinecraftMapAsset(session without token) status = %d, want %d", sessionWithoutTokenResponse.Code, http.StatusNotFound)
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
	revokedPrivateRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, viewerURL+"index.html", nil)
	revokedPrivateResponse := httptest.NewRecorder()
	router.ServeHTTP(revokedPrivateResponse, revokedPrivateRequest)
	if revokedPrivateResponse.Code != http.StatusNotFound {
		t.Fatalf("MinecraftMapAsset(revoked viewer) status = %d, want %d", revokedPrivateResponse.Code, http.StatusNotFound)
	}

	badRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, MinecraftMapViewerPathPrefix+"/server-local-1/not-a-token/index.html", nil)
	badResponse := httptest.NewRecorder()
	router.ServeHTTP(badResponse, badRequest)
	if badResponse.Code != http.StatusNotFound {
		t.Fatalf("MinecraftMapAsset(invalid grant) status = %d, want %d", badResponse.Code, http.StatusNotFound)
	}

	_, errShare := fixture.conn.GetOrCreateGameServerMapShare("server-local-1", "MinecraftAssets")
	if errShare != nil {
		t.Fatalf("GetOrCreateGameServerMapShare() error = %v", errShare)
	}
	_, errEnableShare := fixture.conn.UpdateGameServerMapShare("server-local-1", "MinecraftAssets", true)
	if errEnableShare != nil {
		t.Fatalf("UpdateGameServerMapShare(enable) error = %v", errEnableShare)
	}
	sharedRouter := chi.NewRouter()
	sharedRouter.Get(MinecraftMapSharedPathPrefix+"/{gameServerId}/{token}/*", fixture.service.MinecraftMapAsset)
	publicRequest := connect.NewRequest(&xylona.GetPublicMinecraftMapRequest{PublicIdentifier: "MinecraftAssets"})
	publicResponse, errPublic := fixture.service.GetPublicMinecraftMap(t.Context(), publicRequest)
	if errPublic != nil {
		t.Fatalf("GetPublicMinecraftMap() error = %v", errPublic)
	}
	sharedURL := publicResponse.Msg.GetMap().GetViewerUrl()
	assertMinecraftMapViewerURL(t, sharedURL, MinecraftMapSharedPathPrefix)
	sharedRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, sharedURL+"assets/app.js", nil)
	sharedResponse := httptest.NewRecorder()
	sharedRouter.ServeHTTP(sharedResponse, sharedRequest)
	if sharedResponse.Code != http.StatusOK || sharedResponse.Header().Get("Cache-Control") != "private, max-age=300" {
		t.Fatalf("MinecraftMapAsset(shared) response = %d cache %q", sharedResponse.Code, sharedResponse.Header().Get("Cache-Control"))
	}
	_, errRename := fixture.conn.UpdateGameServerMapShare("server-local-1", "MinecraftRenamed", true)
	if errRename != nil {
		t.Fatalf("UpdateGameServerMapShare(rename) error = %v", errRename)
	}
	revokedResponse := httptest.NewRecorder()
	sharedRouter.ServeHTTP(revokedResponse, sharedRequest)
	if revokedResponse.Code != http.StatusNotFound {
		t.Fatalf("MinecraftMapAsset(revoked share) status = %d, want %d", revokedResponse.Code, http.StatusNotFound)
	}
	renamedPublicRequest := connect.NewRequest(&xylona.GetPublicMinecraftMapRequest{PublicIdentifier: "MinecraftRenamed"})
	renamedPublicResponse, errRenamedPublic := fixture.service.GetPublicMinecraftMap(t.Context(), renamedPublicRequest)
	if errRenamedPublic != nil {
		t.Fatalf("GetPublicMinecraftMap(renamed) error = %v", errRenamedPublic)
	}
	renamedURL := renamedPublicResponse.Msg.GetMap().GetViewerUrl()
	assertMinecraftMapViewerURL(t, renamedURL, MinecraftMapSharedPathPrefix)
	renamedRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, renamedURL+"assets/app.js", nil)
	renamedResponse := httptest.NewRecorder()
	sharedRouter.ServeHTTP(renamedResponse, renamedRequest)
	if renamedResponse.Code != http.StatusOK {
		t.Fatalf("MinecraftMapAsset(renamed share) status = %d, want %d", renamedResponse.Code, http.StatusOK)
	}
	_, errDisable := fixture.conn.UpdateGameServerMapShare("server-local-1", "MinecraftRenamed", false)
	if errDisable != nil {
		t.Fatalf("UpdateGameServerMapShare(disable) error = %v", errDisable)
	}
	disabledResponse := httptest.NewRecorder()
	sharedRouter.ServeHTTP(disabledResponse, renamedRequest)
	if disabledResponse.Code != http.StatusNotFound {
		t.Fatalf("MinecraftMapAsset(disabled share) status = %d, want %d", disabledResponse.Code, http.StatusNotFound)
	}
}

func assertMinecraftMapViewerURL(t *testing.T, viewerURL, prefix string) {
	t.Helper()
	if !strings.HasPrefix(viewerURL, prefix+"/") || !strings.HasSuffix(viewerURL, "/") {
		t.Fatalf("viewer URL = %q, want %s/<id>/<token>/", viewerURL, prefix)
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(viewerURL, prefix+"/"), "/")
	serverID, token, found := strings.Cut(rest, "/")
	if !found || serverID == "" || token == "" || strings.Contains(token, "/") {
		t.Fatalf("viewer URL token = %q", rest)
	}
}

func TestRedactMinecraftMapAssetPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "viewer asset token is redacted",
			path: MinecraftMapViewerPathPrefix + "/server-local-1/secret-token/index.html",
			want: MinecraftMapViewerPathPrefix + "/server-local-1/REDACTED/index.html",
		},
		{
			name: "shared nested asset token is redacted",
			path: MinecraftMapSharedPathPrefix + "/server-local-1/secret-token/maps/world/0/0.png",
			want: MinecraftMapSharedPathPrefix + "/server-local-1/REDACTED/maps/world/0/0.png",
		},
		{
			name: "token-only path is redacted",
			path: MinecraftMapViewerPathPrefix + "/server-local-1/secret-token",
			want: MinecraftMapViewerPathPrefix + "/server-local-1/REDACTED",
		},
		{
			name: "unrelated API path is unchanged",
			path: "/api/xylona.Xylona/GetMinecraftMap",
			want: "/api/xylona.Xylona/GetMinecraftMap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactMinecraftMapAssetPath(tt.path)
			if got != tt.want {
				t.Fatalf("RedactMinecraftMapAssetPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
