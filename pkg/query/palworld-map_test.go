package query

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPalworldMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		gameData      string
		gameStatus    int
		metrics       string
		metricStatus  int
		players       string
		wantSource    string
		wantKinds     []PalworldMapActorKind
		wantNames     []string
		wantPartial   bool
		wantReasonHas []string
		wantNoReason  bool
		wantHealth    *PalworldMapHealth
	}{
		{
			name:       "sanitizes all documented world actor types",
			gameStatus: http.StatusOK,
			gameData: `{"Time":"2026-07-18 12:00:00","ActorData":[
				{"Type":"Character","UnitType":"Player","InstanceID":"player-secret","NickName":"Alex","userid":"steam-secret","ip":"203.0.113.4","LocationX":10,"LocationY":20,"LocationZ":30,"level":12,"IsActive":"true"},
				{"Type":"PalBox","GuildID":"guild-secret","GuildName":"Skyforge","LocationX":40,"LocationY":50,"LocationZ":60},
				{"Type":"PalBox","GuildID":"guild-secret","GuildName":"Skyforge","LocationX":140,"LocationY":150,"LocationZ":160},
				{"Type":"Character","UnitType":"BaseCampPal","InstanceID":"worker-secret","NickName":"Anubis","TrainerNickName":"Alex","GuildID":"guild-secret","GuildName":"Skyforge","LocationX":70,"LocationY":80,"IsActive":"false"},
				{"Type":"Character","UnitType":"OtomoPal","InstanceID":"companion-secret","NickName":"Lifmunk","LocationX":90,"LocationY":100},
				{"Type":"Character","UnitType":"WildPal","InstanceID":"wild-secret","Class":"Pal_SheepBall","LocationX":110,"LocationY":120},
				{"Type":"Character","UnitType":"NPC","InstanceID":"npc-secret","NickName":"Merchant","LocationX":130,"LocationY":140}
			]}`,
			metrics:      `{"serverfps":60,"currentplayernum":-1,"serverframetime":16.67,"maxplayernum":32,"uptime":-5,"basecampnum":3,"days":99}`,
			metricStatus: http.StatusOK,
			players:      `{"players":[]}`,
			wantSource:   "game-data",
			wantKinds: []PalworldMapActorKind{
				PalworldMapActorKindPlayer,
				PalworldMapActorKindBase,
				PalworldMapActorKindBase,
				PalworldMapActorKindBaseWorker,
				PalworldMapActorKindCompanionPal,
				PalworldMapActorKindWildPal,
				PalworldMapActorKindNPC,
			},
			wantNames:    []string{"Alex", "Skyforge", "Skyforge", "Anubis", "Lifmunk", "Pal_SheepBall", "Merchant"},
			wantNoReason: true,
			wantHealth: &PalworldMapHealth{
				ServerFPS:         60,
				ServerFrameTimeMS: 16.67,
				MaxPlayers:        32,
				BaseCampCount:     3,
				Days:              99,
			},
		},
		{
			name:          "reports the launch option when Palworld disables the GameData API",
			gameStatus:    http.StatusBadRequest,
			gameData:      `{"error":"PalGameDataBridge GameData API is not enabled"}`,
			metrics:       `{"serverfps":60}`,
			metricStatus:  http.StatusOK,
			players:       `{"players":[{"name":"Robin","userId":"steam-secret","location_x":1,"location_y":2,"level":20}]}`,
			wantSource:    "players",
			wantKinds:     []PalworldMapActorKind{PalworldMapActorKindPlayer},
			wantNames:     []string{"Robin"},
			wantPartial:   true,
			wantReasonHas: []string{"-enable-gamedata-api", "start arguments"},
			wantHealth:    &PalworldMapHealth{ServerFPS: 60},
		},
		{
			name:          "reports an outdated server when game-data is missing",
			gameStatus:    http.StatusNotFound,
			gameData:      `{}`,
			metrics:       `{"serverfps":60}`,
			metricStatus:  http.StatusOK,
			players:       `{"players":[{"name":"Robin","userId":"steam-secret","location_x":1,"location_y":2,"level":20}]}`,
			wantSource:    "players",
			wantKinds:     []PalworldMapActorKind{PalworldMapActorKindPlayer},
			wantNames:     []string{"Robin"},
			wantPartial:   true,
			wantReasonHas: []string{"404", "-enable-gamedata-api"},
			wantHealth:    &PalworldMapHealth{ServerFPS: 60},
		},
		{
			name:          "reports refused credentials without suggesting the launch option",
			gameStatus:    http.StatusUnauthorized,
			gameData:      `{}`,
			metrics:       `{"serverfps":60}`,
			metricStatus:  http.StatusOK,
			players:       `{"players":[{"name":"Robin","userId":"steam-secret","location_x":1,"location_y":2,"level":20}]}`,
			wantSource:    "players",
			wantKinds:     []PalworldMapActorKind{PalworldMapActorKindPlayer},
			wantNames:     []string{"Robin"},
			wantPartial:   true,
			wantReasonHas: []string{"401", "credentials"},
			wantHealth:    &PalworldMapHealth{ServerFPS: 60},
		},
		{
			name:         "falls back to positioned players and retains metrics",
			gameStatus:   http.StatusNotFound,
			gameData:     `{}`,
			metrics:      `{"serverfps":55,"currentplayernum":1,"serverframetime":18.2,"maxplayernum":16,"uptime":3600,"basecampnum":2,"days":7}`,
			metricStatus: http.StatusOK,
			players:      `{"players":[{"name":"Robin","userId":"steam-secret","ip":"198.51.100.2","location_x":123.5,"location_y":456.5,"level":20}]}`,
			wantSource:   "players",
			wantKinds:    []PalworldMapActorKind{PalworldMapActorKindPlayer},
			wantNames:    []string{"Robin"},
			wantPartial:  true,
			wantHealth: &PalworldMapHealth{
				ServerFPS:         55,
				ServerFrameTimeMS: 18.2,
				CurrentPlayers:    1,
				MaxPlayers:        16,
				UptimeSeconds:     3600,
				BaseCampCount:     2,
				Days:              7,
			},
		},
		{
			name:         "ignores metrics HTTP failures",
			gameStatus:   http.StatusOK,
			gameData:     `{"ActorData":[]}`,
			metrics:      `{"error":"unavailable"}`,
			metricStatus: http.StatusServiceUnavailable,
			players:      `{"players":[]}`,
			wantSource:   "game-data",
			wantKinds:    []PalworldMapActorKind{},
			wantNames:    []string{},
			wantNoReason: true,
		},
		{
			name:         "ignores malformed metrics",
			gameStatus:   http.StatusOK,
			gameData:     `{"ActorData":[]}`,
			metrics:      `{"serverfps":`,
			metricStatus: http.StatusOK,
			players:      `{"players":[]}`,
			wantSource:   "game-data",
			wantKinds:    []PalworldMapActorKind{},
			wantNames:    []string{},
			wantNoReason: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/v1/api/game-data":
					writer.WriteHeader(tc.gameStatus)
					fmt.Fprint(writer, tc.gameData)
				case "/v1/api/metrics":
					writer.WriteHeader(tc.metricStatus)
					fmt.Fprint(writer, tc.metrics)
				case "/v1/api/players":
					fmt.Fprint(writer, tc.players)
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()
			host, port := palworldTestAddress(t, server.URL)

			snapshot, errMap := PalworldMap(t.Context(), host, port, "admin", "secret")
			if errMap != nil {
				t.Fatalf("PalworldMap() error = %v", errMap)
			}
			if snapshot.Source != tc.wantSource || snapshot.Partial != tc.wantPartial {
				t.Fatalf("snapshot source/partial = %q/%t, want %q/%t", snapshot.Source, snapshot.Partial, tc.wantSource, tc.wantPartial)
			}
			if tc.wantNoReason && snapshot.PartialReason != "" {
				t.Fatalf("partial reason = %q, want empty", snapshot.PartialReason)
			}
			for _, fragment := range tc.wantReasonHas {
				if !strings.Contains(snapshot.PartialReason, fragment) {
					t.Fatalf("partial reason = %q, want it to contain %q", snapshot.PartialReason, fragment)
				}
			}
			if len(snapshot.Actors) != len(tc.wantKinds) {
				t.Fatalf("actors = %+v, want %d", snapshot.Actors, len(tc.wantKinds))
			}
			if !equalPalworldMapHealth(snapshot.Health, tc.wantHealth) {
				t.Fatalf("health = %+v, want %+v", snapshot.Health, tc.wantHealth)
			}
			actorKeys := make(map[string]struct{}, len(snapshot.Actors))
			for i, actor := range snapshot.Actors {
				if actor.Kind != tc.wantKinds[i] || actor.Name != tc.wantNames[i] {
					t.Fatalf("actor %d = %+v, want kind %d name %q", i, actor, tc.wantKinds[i], tc.wantNames[i])
				}
				if actor.Key == "" || actor.Key == "player-secret" || actor.Key == "steam-secret" {
					t.Fatalf("actor key was not sanitized: %q", actor.Key)
				}
				_, duplicateKey := actorKeys[actor.Key]
				if duplicateKey {
					t.Fatalf("actor key %q was reused by multiple world actors", actor.Key)
				}
				actorKeys[actor.Key] = struct{}{}
			}
			if tc.name == "sanitizes all documented world actor types" {
				guildKey := snapshot.Actors[1].GuildKey
				if guildKey == "" || guildKey == "guild-secret" || strings.Contains(guildKey, "guild-secret") {
					t.Fatalf("guild key was not sanitized: %q", guildKey)
				}
				if snapshot.Actors[2].GuildKey != guildKey || snapshot.Actors[3].GuildKey != guildKey {
					t.Fatalf("same guild produced different keys: %+v", snapshot.Actors[:4])
				}
			}
		})
	}
}

// TestPalworldMapPartialReason covers the failure classes that are impractical
// to drive through a live handler, and pins the rule that operator guidance
// never echoes the game server's own error text or address.
func TestPalworldMapPartialReason(t *testing.T) {
	t.Parallel()

	secretDetail := errors.New(`Get "http://10.0.0.7:8212/v1/api/game-data": connection refused`)
	tests := []struct {
		name       string
		failure    error
		wantHas    []string
		wantNotHas []string
	}{
		{
			name:    "oversize body is not reported as malformed",
			failure: &palworldGameDataFailure{decodeFailed: true, oversize: true},
			wantHas: []string{"exceeded the 16 MB", "player positions only"},
		},
		{
			name:    "malformed body is reported as undecodable",
			failure: &palworldGameDataFailure{decodeFailed: true},
			wantHas: []string{"could not be decoded"},
		},
		{
			name:       "transport failure never leaks the server address",
			failure:    &palworldGameDataFailure{err: secretDetail},
			wantHas:    []string{"did not complete"},
			wantNotHas: []string{"10.0.0.7", "8212", "connection refused"},
		},
		{
			name:    "unexpected error types still explain the fallback",
			failure: errors.New("something else"),
			wantHas: []string{"player positions only"},
		},
		{
			name:       "unmapped status reports the code without guessing a cause",
			failure:    &palworldGameDataFailure{statusCode: http.StatusInternalServerError},
			wantHas:    []string{"HTTP 500"},
			wantNotHas: []string{palworldGameDataLaunchOption},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reason := palworldMapPartialReason(tc.failure)
			for _, fragment := range tc.wantHas {
				if !strings.Contains(reason, fragment) {
					t.Fatalf("reason = %q, want it to contain %q", reason, fragment)
				}
			}
			for _, fragment := range tc.wantNotHas {
				if strings.Contains(reason, fragment) {
					t.Fatalf("reason = %q, want it to omit %q", reason, fragment)
				}
			}
		})
	}
}

func TestPalworldMapQueriesMetricsConcurrently(t *testing.T) {
	t.Parallel()

	metricsRequested := make(chan struct{})
	concurrent := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/api/game-data":
			select {
			case <-metricsRequested:
				concurrent <- true
			case <-time.After(time.Second):
				concurrent <- false
			}
			fmt.Fprint(writer, `{"ActorData":[]}`)
		case "/v1/api/metrics":
			close(metricsRequested)
			fmt.Fprint(writer, `{"serverfps":60}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	host, port := palworldTestAddress(t, server.URL)

	snapshot, errMap := PalworldMap(t.Context(), host, port, "admin", "secret")
	if errMap != nil {
		t.Fatalf("PalworldMap() error = %v", errMap)
	}
	if !<-concurrent {
		t.Fatal("metrics request did not run concurrently with game-data")
	}
	if snapshot.Health == nil || snapshot.Health.ServerFPS != 60 {
		t.Fatalf("health = %+v, want server FPS 60", snapshot.Health)
	}
}

func equalPalworldMapHealth(got *PalworldMapHealth, want *PalworldMapHealth) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}
