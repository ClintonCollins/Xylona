package rpc

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/palworldmap"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestPalworldMapAuthorizationAndSharing(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	_, errPalworld := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		"update game_server set game_id = 'palworld' where id = 'server-local-1'",
	)
	if errPalworld != nil {
		t.Fatalf("set Palworld game: %v", errPalworld)
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

	actionsContext, cancelActions := context.WithCancel(t.Context())
	t.Cleanup(cancelActions)
	fixture.service.actionsInst = actions.NewInstance(
		actionsContext,
		fixture.conn,
		nil,
		nil,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	viewerRequest := connect.NewRequest(&xylona.GetPalworldMapRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerRequest, "user-other")
	viewerResponse, errViewer := fixture.service.GetPalworldMap(t.Context(), viewerRequest)
	if errViewer != nil {
		t.Fatalf("GetPalworldMap(viewer) error = %v", errViewer)
	}
	if viewerResponse.Msg.GetMap().GetCanManageShare() {
		t.Fatal("viewer unexpectedly received map-management access")
	}

	viewerSettingsRequest := connect.NewRequest(&xylona.UpdatePalworldMapConfigRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerSettingsRequest, "user-other")
	_, errViewerSettings := fixture.service.UpdatePalworldMapConfig(t.Context(), viewerSettingsRequest)
	if connect.CodeOf(errViewerSettings) != connect.CodePermissionDenied {
		t.Fatalf("UpdatePalworldMapConfig(viewer) code = %v, want %v", connect.CodeOf(errViewerSettings), connect.CodePermissionDenied)
	}
	viewerInstallRequest := connect.NewRequest(&xylona.InstallPalworldMapTilesRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, viewerInstallRequest, "user-other")
	_, errViewerInstall := fixture.service.InstallPalworldMapTiles(t.Context(), viewerInstallRequest)
	if connect.CodeOf(errViewerInstall) != connect.CodePermissionDenied {
		t.Fatalf("InstallPalworldMapTiles(viewer) code = %v, want %v", connect.CodeOf(errViewerInstall), connect.CodePermissionDenied)
	}

	settingsRequest := connect.NewRequest(&xylona.GetOrCreateGameServerMapShareSettingsRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, settingsRequest, "user-owner")
	_, errSettings := fixture.service.GetOrCreateGameServerMapShareSettings(t.Context(), settingsRequest)
	if errSettings != nil {
		t.Fatalf("GetOrCreateGameServerMapShareSettings(owner) error = %v", errSettings)
	}
	shareRequest := connect.NewRequest(&xylona.UpdateGameServerMapShareSettingsRequest{
		GameServerId: "server-local-1", PublicIdentifier: "Palpagos_Map", Enabled: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, shareRequest, "user-owner")
	_, errShare := fixture.service.UpdateGameServerMapShareSettings(t.Context(), shareRequest)
	if errShare != nil {
		t.Fatalf("UpdateGameServerMapShareSettings(owner) error = %v", errShare)
	}
	publicRequest := connect.NewRequest(&xylona.GetPublicPalworldMapRequest{PublicIdentifier: "Palpagos_Map"})
	publicResponse, errPublic := fixture.service.GetPublicPalworldMap(t.Context(), publicRequest)
	if errPublic != nil || publicResponse.Msg.GetMap().GetServerName() != "Local One" {
		t.Fatalf("GetPublicPalworldMap() = %+v, %v", publicResponse, errPublic)
	}
	resolved, errResolve := fixture.service.ResolvePublicGameServerMap(t.Context(), connect.NewRequest(
		&xylona.ResolvePublicGameServerMapRequest{PublicIdentifier: "Palpagos_Map"},
	))
	if errResolve != nil || resolved.Msg.GetKind() != xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_PALWORLD {
		t.Fatalf("ResolvePublicGameServerMap() = %+v, %v", resolved, errResolve)
	}

	shareRequest.Msg.Enabled = false
	_, errDisable := fixture.service.UpdateGameServerMapShareSettings(t.Context(), shareRequest)
	if errDisable != nil {
		t.Fatalf("UpdateGameServerMapShareSettings(disable) error = %v", errDisable)
	}
	_, errRevoked := fixture.service.GetPublicPalworldMap(t.Context(), publicRequest)
	if connect.CodeOf(errRevoked) != connect.CodeNotFound {
		t.Fatalf("GetPublicPalworldMap(revoked) code = %v, want %v", connect.CodeOf(errRevoked), connect.CodeNotFound)
	}
}

func TestInstallPalworldMapTiles(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	_, errPalworld := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		"update game_server set game_id = 'palworld' where id = 'server-local-1'",
	)
	if errPalworld != nil {
		t.Fatalf("set Palworld game: %v", errPalworld)
	}
	installer := &fakePalworldMapTileInstaller{
		layers: []palworldmap.Layer{
			{
				ID:          "default",
				Label:       "Palpagos",
				Attribution: "Palworld © Pocketpair",
				MinZoom:     0,
				MaxZoom:     4,
				TileSize:    512,
				TransformA:  1,
				TransformC:  -1,
				MinX:        -100,
				MinY:        -100,
				MaxX:        100,
				MaxY:        100,
			},
		},
	}
	fixture.service.SetPalworldMapTileInstaller(installer)

	request := connect.NewRequest(&xylona.InstallPalworldMapTilesRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	response, errInstall := fixture.service.InstallPalworldMapTiles(t.Context(), request)
	if errInstall != nil {
		t.Fatalf("InstallPalworldMapTiles() error = %v", errInstall)
	}
	if installer.calls != 1 {
		t.Fatalf("tile installer calls = %d, want 1", installer.calls)
	}
	if len(response.Msg.GetLayers()) != 1 || response.Msg.GetLayers()[0].GetTileUrlTemplate() != "/palworld-map-tiles/default/{z}/{x}/{y}.webp" {
		t.Fatalf("InstallPalworldMapTiles() layers = %+v", response.Msg.GetLayers())
	}
	settings, errSettings := fixture.conn.GetGameServerPalworldMap("server-local-1")
	if errSettings != nil {
		t.Fatalf("GetGameServerPalworldMap() error = %v", errSettings)
	}
	layers, errDecode := decodePalworldMapLayers(settings.LayersJSON)
	if errDecode != nil {
		t.Fatalf("decodePalworldMapLayers() error = %v", errDecode)
	}
	if len(layers) != 1 || layers[0].TileURLTemplate != "/palworld-map-tiles/default/{z}/{x}/{y}.webp" {
		t.Fatalf("stored layers = %+v", layers)
	}
}

func TestPalworldMapViewForwardsSafeIntelligenceDataForPrivateAndPublicViewers(t *testing.T) {
	now := time.Now().UTC()
	state := actions.PalworldMapState{
		ServerID:     "palworld-1",
		ServerName:   "Palpagos",
		ServerOnline: true,
		Snapshot: &node.PalworldMapSnapshot{
			CollectedAt: now,
			Health: &node.PalworldMapHealth{
				ServerFPS:         60,
				ServerFrameTimeMS: 16.67,
				CurrentPlayers:    4,
				MaxPlayers:        32,
				UptimeSeconds:     3600,
				BaseCampCount:     3,
				Days:              99,
			},
			Actors: []node.PalworldMapActor{
				{
					Key:       "player-1",
					Kind:      node.PalworldMapActorKindPlayer,
					Name:      "Alex",
					GuildKey:  "guild-key",
					LocationX: 123.456,
					LocationY: -987.654,
					LocationZ: 42.25,
				},
			},
		},
	}
	tests := []struct {
		name      string
		canManage bool
	}{
		{name: "private", canManage: true},
		{name: "public"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			view := palworldMapView(
				state,
				nil,
				tc.canManage,
				now,
			)

			if len(view.GetActors()) != 1 {
				t.Fatalf("PalworldMapView actors = %+v", view.GetActors())
			}
			actor := view.GetActors()[0]
			if actor.GetName() != "Alex" || actor.GetGuildKey() != "guild-key" || actor.GetLocationX() != 123.456 || actor.GetLocationY() != -987.654 || actor.GetLocationZ() != 42.25 {
				t.Fatalf("PalworldMapView actor = %+v", actor)
			}
			health := view.GetHealth()
			if health == nil ||
				health.GetServerFps() != 60 ||
				health.GetServerFrameTimeMs() != 16.67 ||
				health.GetCurrentPlayers() != 4 ||
				health.GetMaxPlayers() != 32 ||
				health.GetUptimeSeconds() != 3600 ||
				health.GetBaseCampCount() != 3 ||
				health.GetDays() != 99 {
				t.Fatalf("PalworldMapView health = %+v", health)
			}
			if view.GetCanManageShare() != tc.canManage {
				t.Fatalf("PalworldMapView can_manage_share = %t, want %t", view.GetCanManageShare(), tc.canManage)
			}
		})
	}
}

func TestValidatePalworldMapLayers(t *testing.T) {
	t.Parallel()

	validLayer := func(id string) *xylona.PalworldMapLayer {
		return &xylona.PalworldMapLayer{
			Id:              id,
			Label:           "World map",
			TileUrlTemplate: "https://maps.example.test/{z}/{x}/{y}.png",
			Attribution:     "Example map",
			MinZoom:         0,
			MaxZoom:         6,
			TileSize:        512,
			TransformA:      1,
			TransformC:      -1,
			MinX:            -100,
			MinY:            -100,
			MaxX:            100,
			MaxY:            100,
		}
	}

	tests := []struct {
		name    string
		layers  []*xylona.PalworldMapLayer
		wantLen int
		wantErr string
	}{
		{name: "empty coordinate grid", wantLen: 0},
		{name: "valid HTTPS tiles", layers: []*xylona.PalworldMapLayer{validLayer("world")}, wantLen: 1},
		{
			name: "valid same-origin tiles",
			layers: []*xylona.PalworldMapLayer{
				func() *xylona.PalworldMapLayer {
					layer := validLayer("world")
					layer.TileUrlTemplate = "/palworld-map-tiles/world/{z}/{x}/{y}.webp"
					return layer
				}(),
			},
			wantLen: 1,
		},
		{
			name: "duplicate IDs",
			layers: []*xylona.PalworldMapLayer{
				validLayer("world"),
				validLayer("world"),
			},
			wantErr: "duplicated",
		},
		{
			name: "non-HTTP tile URL",
			layers: []*xylona.PalworldMapLayer{
				func() *xylona.PalworldMapLayer {
					layer := validLayer("world")
					layer.TileUrlTemplate = "javascript:{z}/{x}/{y}"
					return layer
				}(),
			},
			wantErr: "same-origin path or an absolute HTTP or HTTPS URL",
		},
		{
			name: "missing coordinate placeholder",
			layers: []*xylona.PalworldMapLayer{
				func() *xylona.PalworldMapLayer {
					layer := validLayer("world")
					layer.TileUrlTemplate = "https://maps.example.test/{z}/{x}/tile.png"
					return layer
				}(),
			},
			wantErr: "must contain {z}, {x}, and {y}",
		},
		{
			name: "non-finite transform",
			layers: []*xylona.PalworldMapLayer{
				func() *xylona.PalworldMapLayer {
					layer := validLayer("world")
					layer.TransformA = math.NaN()
					return layer
				}(),
			},
			wantErr: "finite numbers",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			layers, errValidate := validatePalworldMapLayers(test.layers)
			if test.wantErr != "" {
				if errValidate == nil || !strings.Contains(errValidate.Error(), test.wantErr) {
					t.Fatalf("validatePalworldMapLayers() error = %v, want containing %q", errValidate, test.wantErr)
				}
				return
			}
			if errValidate != nil {
				t.Fatalf("validatePalworldMapLayers() error = %v", errValidate)
			}
			if len(layers) != test.wantLen {
				t.Fatalf("validatePalworldMapLayers() len = %d, want %d", len(layers), test.wantLen)
			}
		})
	}
}

type fakePalworldMapTileInstaller struct {
	layers []palworldmap.Layer
	err    error
	calls  int
}

func (f *fakePalworldMapTileInstaller) Install(_ context.Context) error {
	f.calls++
	return f.err
}

func (f *fakePalworldMapTileInstaller) Layers() []palworldmap.Layer {
	return f.layers
}
