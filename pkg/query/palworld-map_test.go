package query

import (
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
		name         string
		gameData     string
		gameStatus   int
		metrics      string
		metricStatus int
		players      string
		wantSource   string
		wantKinds    []PalworldMapActorKind
		wantNames    []string
		wantPartial  bool
		wantHealth   *PalworldMapHealth
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
			wantNames: []string{"Alex", "Skyforge", "Skyforge", "Anubis", "Lifmunk", "Pal_SheepBall", "Merchant"},
			wantHealth: &PalworldMapHealth{
				ServerFPS:         60,
				ServerFrameTimeMS: 16.67,
				MaxPlayers:        32,
				BaseCampCount:     3,
				Days:              99,
			},
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
