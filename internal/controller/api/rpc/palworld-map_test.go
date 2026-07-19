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

	shareRequest := connect.NewRequest(&xylona.RegeneratePalworldMapShareRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, shareRequest, "user-owner")
	shareResponse, errShare := fixture.service.RegeneratePalworldMapShare(t.Context(), shareRequest)
	if errShare != nil {
		t.Fatalf("RegeneratePalworldMapShare(owner) error = %v", errShare)
	}
	publicRequest := connect.NewRequest(&xylona.GetPublicPalworldMapRequest{ShareToken: shareResponse.Msg.GetShareToken()})
	publicResponse, errPublic := fixture.service.GetPublicPalworldMap(t.Context(), publicRequest)
	if errPublic != nil || publicResponse.Msg.GetMap().GetServerName() != "Local One" {
		t.Fatalf("GetPublicPalworldMap() = %+v, %v", publicResponse, errPublic)
	}

	revokeRequest := connect.NewRequest(&xylona.RevokePalworldMapShareRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, revokeRequest, "user-owner")
	_, errRevoke := fixture.service.RevokePalworldMapShare(t.Context(), revokeRequest)
	if errRevoke != nil {
		t.Fatalf("RevokePalworldMapShare(owner) error = %v", errRevoke)
	}
	_, errRevoked := fixture.service.GetPublicPalworldMap(t.Context(), publicRequest)
	if connect.CodeOf(errRevoked) != connect.CodeNotFound {
		t.Fatalf("GetPublicPalworldMap(revoked) code = %v, want %v", connect.CodeOf(errRevoked), connect.CodeNotFound)
	}
}

func TestPalworldMapViewKeepsExactActorDataForViewers(t *testing.T) {
	now := time.Now().UTC()
	view := palworldMapView(
		actions.PalworldMapState{
			ServerID:     "palworld-1",
			ServerName:   "Palpagos",
			ServerOnline: true,
			Snapshot: &node.PalworldMapSnapshot{
				CollectedAt: now,
				Actors: []node.PalworldMapActor{
					{
						Key:       "player-1",
						Kind:      node.PalworldMapActorKindPlayer,
						Name:      "Alex",
						LocationX: 123.456,
						LocationY: -987.654,
						LocationZ: 42.25,
					},
				},
			},
		},
		nil,
		false,
		false,
		now,
	)

	if len(view.GetActors()) != 1 {
		t.Fatalf("PalworldMapView actors = %+v", view.GetActors())
	}
	actor := view.GetActors()[0]
	if actor.GetName() != "Alex" || actor.GetLocationX() != 123.456 || actor.GetLocationY() != -987.654 || actor.GetLocationZ() != 42.25 {
		t.Fatalf("PalworldMapView actor = %+v", actor)
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
			wantErr: "absolute HTTP or HTTPS URL",
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
