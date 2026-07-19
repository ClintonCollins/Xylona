package query

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPalworldMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		gameData    string
		gameStatus  int
		players     string
		wantSource  string
		wantKinds   []PalworldMapActorKind
		wantNames   []string
		wantPartial bool
	}{
		{
			name:       "sanitizes all documented world actor types",
			gameStatus: http.StatusOK,
			gameData: `{"Time":"2026-07-18 12:00:00","ActorData":[
				{"Type":"Character","UnitType":"Player","InstanceID":"player-secret","NickName":"Alex","userid":"steam-secret","ip":"203.0.113.4","LocationX":10,"LocationY":20,"LocationZ":30,"level":12,"IsActive":"true"},
				{"Type":"PalBox","GuildID":"guild-secret","GuildName":"Skyforge","LocationX":40,"LocationY":50,"LocationZ":60},
				{"Type":"Character","UnitType":"BaseCampPal","InstanceID":"worker-secret","NickName":"Anubis","TrainerNickName":"Alex","GuildName":"Skyforge","LocationX":70,"LocationY":80,"IsActive":"false"},
				{"Type":"Character","UnitType":"OtomoPal","InstanceID":"companion-secret","NickName":"Lifmunk","LocationX":90,"LocationY":100},
				{"Type":"Character","UnitType":"WildPal","InstanceID":"wild-secret","Class":"Pal_SheepBall","LocationX":110,"LocationY":120},
				{"Type":"Character","UnitType":"NPC","InstanceID":"npc-secret","NickName":"Merchant","LocationX":130,"LocationY":140}
			]}`,
			players:    `{"players":[]}`,
			wantSource: "game-data",
			wantKinds: []PalworldMapActorKind{
				PalworldMapActorKindPlayer,
				PalworldMapActorKindBase,
				PalworldMapActorKindBaseWorker,
				PalworldMapActorKindCompanionPal,
				PalworldMapActorKindWildPal,
				PalworldMapActorKindNPC,
			},
			wantNames: []string{"Alex", "Skyforge", "Anubis", "Lifmunk", "Pal_SheepBall", "Merchant"},
		},
		{
			name:        "falls back to positioned players",
			gameStatus:  http.StatusNotFound,
			gameData:    `{}`,
			players:     `{"players":[{"name":"Robin","userId":"steam-secret","ip":"198.51.100.2","location_x":123.5,"location_y":456.5,"level":20}]}`,
			wantSource:  "players",
			wantKinds:   []PalworldMapActorKind{PalworldMapActorKindPlayer},
			wantNames:   []string{"Robin"},
			wantPartial: true,
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
			for i, actor := range snapshot.Actors {
				if actor.Kind != tc.wantKinds[i] || actor.Name != tc.wantNames[i] {
					t.Fatalf("actor %d = %+v, want kind %d name %q", i, actor, tc.wantKinds[i], tc.wantNames[i])
				}
				if actor.Key == "" || actor.Key == "player-secret" || actor.Key == "steam-secret" {
					t.Fatalf("actor key was not sanitized: %q", actor.Key)
				}
			}
		})
	}
}
