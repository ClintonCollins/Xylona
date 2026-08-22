package rpc

import (
	"errors"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGameServerStatusPageManagement(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.service.statusPageIdentifier = func() (string, error) { return "Owner_Page", nil }

	getRequest := connect.NewRequest(&xylona.GetOrCreateGameServerStatusPageSettingsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getRequest, "user-owner")
	getResponse, errGet := fixture.service.GetOrCreateGameServerStatusPageSettings(t.Context(), getRequest)
	if errGet != nil {
		t.Fatalf("GetOrCreateGameServerStatusPageSettings() error = %v", errGet)
	}
	settings := getResponse.Msg.GetSettings()
	if settings.GetTitle() != "owner" || settings.GetPublicIdentifier() != "Owner_Page" || settings.GetEnabled() {
		t.Fatalf("created settings = %+v", settings)
	}
	if settings.GetPublicPath() != "/status/Owner_Page" || len(settings.GetServers()) != 1 {
		t.Fatalf("created settings path/servers = %+v", settings)
	}

	otherOwnerRequest := connect.NewRequest(&xylona.GetOrCreateGameServerStatusPageSettingsRequest{OwnerId: "user-owner"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, otherOwnerRequest, "user-other")
	_, errOther := fixture.service.GetOrCreateGameServerStatusPageSettings(t.Context(), otherOwnerRequest)
	if connect.CodeOf(errOther) != connect.CodePermissionDenied {
		t.Fatalf("other owner error = %v, want permission denied", errOther)
	}

	updateRequest := connect.NewRequest(&xylona.UpdateGameServerStatusPageSettingsRequest{
		Title:            "Owner fleet",
		PublicIdentifier: "Owner_Fleet",
		Enabled:          true,
		ConnectionAddresses: []*xylona.GameServerStatusPageConnectionAddress{
			{GameServerId: "server-local-1", PublicConnectionAddress: "[2001:db8::1]:25565"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateRequest, "user-owner")
	updateResponse, errUpdate := fixture.service.UpdateGameServerStatusPageSettings(t.Context(), updateRequest)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerStatusPageSettings() error = %v", errUpdate)
	}
	updated := updateResponse.Msg.GetSettings()
	if !updated.GetEnabled() || updated.GetPublicPath() != "/status/Owner_Fleet" {
		t.Fatalf("updated settings = %+v", updated)
	}
	got := updated.GetServers()[0].GetEffectiveConnectionAddress()
	if got != "[2001:db8::1]:25565" {
		t.Fatalf("effective connection address = %q", got)
	}
}

func TestGameServerStatusPageValidation(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		wantErr bool
	}{
		{name: "title", field: "title", value: " Fleet ", wantErr: false},
		{name: "blank title", field: "title", value: " ", wantErr: true},
		{name: "identifier", field: "identifier", value: "Fleet_A-1", wantErr: false},
		{name: "short identifier", field: "identifier", value: "ab", wantErr: true},
		{name: "identifier slash", field: "identifier", value: "fleet/a", wantErr: true},
		{name: "DNS address", field: "address", value: "play.example.test:25565", wantErr: false},
		{name: "IPv6 address", field: "address", value: "[2001:db8::1]:25565", wantErr: false},
		{name: "scheme", field: "address", value: "https://example.test:25565", wantErr: true},
		{name: "missing port", field: "address", value: "example.test", wantErr: true},
		{name: "bad port", field: "address", value: "example.test:65536", wantErr: true},
		{name: "whitespace", field: "address", value: "example .test:25565", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var errValidate error
			switch test.field {
			case "title":
				_, errValidate = normalizeStatusPageTitle(test.value)
			case "identifier":
				errValidate = validateStatusPageIdentifier(test.value)
			case "address":
				_, errValidate = normalizePublicConnectionAddress(test.value)
			}
			if (errValidate != nil) != test.wantErr {
				t.Fatalf("validation error = %v, wantErr %v", errValidate, test.wantErr)
			}
		})
	}
}

func TestProjectPublicGameServerStatusPage(t *testing.T) {
	observedAt := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	servers := []*models.GameServer{
		statusPageTestServer(t, "source", "bravo", "Source Game", 16),
		statusPageTestServer(t, "minecraft", "Alpha", "Minecraft", 20),
		statusPageTestServer(t, "palworld", "charlie", "Palworld", 32),
	}
	queries := &xylona.AllServersQueryInfo{Servers: map[string]*xylona.ServerQuery{
		"minecraft": {
			ServerId: "minecraft",
			Type:     xylona.ServerQuery_Minecraft,
			Minecraft: &xylona.MinecraftQueryInfo{
				Responded:           true,
				PlayerListSupported: true,
				NumberOfPlayers:     0,
				MaxPlayers:          20,
			},
		},
		"source": {
			ServerId: "source",
			Type:     xylona.ServerQuery_Source,
			Source: &xylona.SourceQueryInfo{
				Responded:           true,
				PlayerListSupported: false,
				Players:             2,
				MaxPlayers:          16,
			},
		},
	}}
	telemetry := map[string]actions.GameServerQueryTelemetrySnapshot{
		"minecraft": {
			Status:              actions.GameServerQueryTelemetryStatusSuccess,
			QueryType:           xylona.ServerQuery_Minecraft,
			LastSuccessAt:       observedAt,
			PlayerCount:         0,
			PlayerCountValid:    true,
			PlayerCapacity:      20,
			PlayerCapacityValid: true,
		},
		"source": {
			Status:              actions.GameServerQueryTelemetryStatusSuccess,
			QueryType:           xylona.ServerQuery_Source,
			LastSuccessAt:       observedAt,
			PlayerCount:         2,
			PlayerCountValid:    true,
			PlayerCapacity:      16,
			PlayerCapacityValid: true,
		},
		"palworld": {
			Status:    actions.GameServerQueryTelemetryStatusFailure,
			QueryType: xylona.ServerQuery_Palworld,
		},
	}

	page := &db.GameServerStatusPage{UserID: "owner", Title: "Owner fleet", Enabled: true}
	projected := projectPublicGameServerStatusPage(
		page,
		servers,
		queries,
		func(id string) actions.GameServerQueryTelemetrySnapshot { return telemetry[id] },
		func(*models.GameServer) xylona.Status { return xylona.Status_ONLINE },
	)

	projectedServers := projected.GetServers()
	got := []string{projectedServers[0].GetName(), projectedServers[1].GetName(), projectedServers[2].GetName()}
	if !slices.Equal(got, []string{"Alpha", "bravo", "charlie"}) {
		t.Fatalf("server order = %v", got)
	}
	minecraft := projectedServers[0]
	if minecraft.CurrentPlayerCount == nil || minecraft.GetCurrentPlayerCount() != 0 || minecraft.GetRosterState() != xylona.GameServerStatusPageRosterState_GAME_SERVER_STATUS_PAGE_ROSTER_STATE_AVAILABLE {
		t.Fatalf("Minecraft public state = %+v", minecraft)
	}
	source := projectedServers[1]
	if source.CurrentPlayerCount == nil || source.GetCurrentPlayerCount() != 2 || source.GetRosterState() != xylona.GameServerStatusPageRosterState_GAME_SERVER_STATUS_PAGE_ROSTER_STATE_UNAVAILABLE {
		t.Fatalf("Source public state = %+v", source)
	}
	palworld := projectedServers[2]
	if palworld.CurrentPlayerCount != nil || palworld.GetRosterState() != xylona.GameServerStatusPageRosterState_GAME_SERVER_STATUS_PAGE_ROSTER_STATE_UNAVAILABLE {
		t.Fatalf("Palworld public state = %+v", palworld)
	}
	if projected.GetGeneratedAt() == nil || !projected.GetGeneratedAt().IsValid() {
		t.Fatal("generated_at is missing")
	}
}

func statusPageTestServer(t *testing.T, id string, name string, gameName string, maxPlayers int64) *models.GameServer {
	t.Helper()
	server := &models.GameServer{
		ID:         id,
		Name:       name,
		IP:         "127.0.0.1",
		Port:       25565,
		MaxPlayers: maxPlayers,
	}
	setGameServerRelation(t, server, &models.Game{ID: id, Name: gameName})
	return server
}

func TestPublicStatusPageNotFoundMetadata(t *testing.T) {
	err := statusPageNotFound()
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("statusPageNotFound() code = %v", connect.CodeOf(err))
	}
	connectError := new(connect.Error)
	if !errors.As(err, &connectError) {
		t.Fatalf("statusPageNotFound() type = %T", err)
	}
	if connectError.Meta().Get("Cache-Control") != "no-store" || connectError.Meta().Get("X-Robots-Tag") != "noindex, nofollow" {
		t.Fatalf("not found metadata = %v", connectError.Meta())
	}
	if connectError.Meta().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("referrer policy = %q", connectError.Meta().Get("Referrer-Policy"))
	}
}
