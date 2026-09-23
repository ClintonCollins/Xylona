package actions

import (
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestStartReadiness(t *testing.T) {
	tests := []struct {
		name        string
		gameID      string
		pattern     string
		sourceQuery bool
		wantQuery   node.GameServerQueryKind
		wantNil     bool
	}{
		{name: "pattern game waits for its line, not its query", gameID: "valheim", pattern: `Game server connected`, sourceQuery: true},
		{name: "minecraft with a pattern skips its query", gameID: "minecraft", pattern: `Done \(`},
		{name: "pattern game without a query", gameID: "terraria", pattern: `Server started`},
		{name: "source query game without a pattern waits for the query", gameID: "rust", sourceQuery: true, wantQuery: node.GameServerQueryKindSource},
		{name: "minecraft without a pattern waits for its query", gameID: "minecraft", wantQuery: node.GameServerQueryKindMinecraft},
		{name: "game without a signal is online at spawn", gameID: "satisfactory", wantNil: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameServer := &models.GameServer{ID: "gs-1", GameID: test.gameID, IP: "127.0.0.1", QueryPort: 27015}
			gameServer.R.Game = &models.Game{ID: test.gameID, UsesSourceQuery: test.sourceQuery, ReadyLogPattern: test.pattern}

			readiness := (&Instance{}).startReadiness(gameServer)
			if test.wantNil {
				if readiness != nil {
					t.Fatalf("startReadiness() = %+v, want nil", readiness)
				}
				return
			}
			if readiness == nil {
				t.Fatal("startReadiness() = nil")
			}
			if readiness.LogPattern != test.pattern {
				t.Fatalf("LogPattern = %q, want %q", readiness.LogPattern, test.pattern)
			}
			if test.pattern != "" {
				if readiness.Query != nil {
					t.Fatalf("Query = %+v, want none for a pattern game", readiness.Query)
				}
				return
			}
			if readiness.Query == nil || readiness.Query.Kind != test.wantQuery {
				t.Fatalf("Query = %+v, want kind %v", readiness.Query, test.wantQuery)
			}
		})
	}
}
