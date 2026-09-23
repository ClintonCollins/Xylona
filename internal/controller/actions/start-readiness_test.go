package actions

import (
	"regexp"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestStartReadiness(t *testing.T) {
	tests := []struct {
		name        string
		gameID      string
		sourceQuery bool
		wantPattern bool
		wantQuery   node.GameServerQueryKind
		wantNil     bool
	}{
		{name: "minecraft uses its Done line and query", gameID: "minecraft", wantPattern: true, wantQuery: node.GameServerQueryKindMinecraft},
		{name: "valheim uses its connected line and query", gameID: "valheim", sourceQuery: true, wantPattern: true, wantQuery: node.GameServerQueryKindSource},
		{name: "source query game waits for the query", gameID: "rust", sourceQuery: true, wantQuery: node.GameServerQueryKindSource},
		{name: "game without a signal is online at spawn", gameID: "factorio", wantNil: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameServer := &models.GameServer{ID: "gs-1", GameID: test.gameID, IP: "127.0.0.1", QueryPort: 27015}
			gameServer.R.Game = &models.Game{ID: test.gameID, UsesSourceQuery: test.sourceQuery}

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
			if (readiness.LogPattern != "") != test.wantPattern {
				t.Fatalf("LogPattern = %q, want pattern %v", readiness.LogPattern, test.wantPattern)
			}
			if readiness.Query == nil || readiness.Query.Kind != test.wantQuery {
				t.Fatalf("Query = %+v, want kind %v", readiness.Query, test.wantQuery)
			}
		})
	}
}

func TestReadyLogPatternsMatchGameOutput(t *testing.T) {
	tests := []struct {
		gameID string
		line   string
		want   bool
	}{
		{gameID: "minecraft", line: `[12:01:02 INFO]: Done (7.512s)! For help, type "help"`, want: true},
		{gameID: "minecraft", line: `[Server thread/INFO]: Done (12,4s)! For help, type "help"`, want: true},
		{gameID: "minecraft", line: `[12:00:58 INFO]: Preparing spawn area: 84%`, want: false},
		{gameID: "valheim", line: `09/22/2026 18:01:02: Game server connected`, want: true},
		{gameID: "valheim", line: `09/22/2026 18:00:40: Steam game server initialized`, want: false},
	}
	for _, test := range tests {
		pattern := regexp.MustCompile(readyLogPatterns[test.gameID])
		if got := pattern.MatchString(test.line); got != test.want {
			t.Errorf("%s pattern on %q = %v, want %v", test.gameID, test.line, got, test.want)
		}
	}
}
