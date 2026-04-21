package rpc

import (
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// seedAlternateNodeAndIP inserts a second node row plus a secondary IP so
// port-validation tests can exercise multi-node game server layouts without
// bringing up a real remote node.
func seedAlternateNodeAndIP(t *testing.T, fixture *rbacRPCFixture) {
	t.Helper()

	_, errNode := fixture.conn.InsertNode(&models.NodeSetter{
		ID:        omit.From("node-alt"),
		Name:      omit.From("Alternate Node"),
		ListenURL: omit.From("http://node-alt.local:8081"),
		Enabled:   omit.From(true),
	})
	if errNode != nil {
		t.Fatalf("InsertNode() error = %v", errNode)
	}

	_, errIP := fixture.conn.UpsertIP(&models.IPSetter{
		Address:            omit.From("127.0.0.2"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
		NodeID:             omit.From("node-alt"),
	})
	if errIP != nil {
		t.Fatalf("UpsertIP() error = %v", errIP)
	}
}

const testGameID = "test-game"

func seedTestGame(t *testing.T, fixture *rbacRPCFixture) {
	t.Helper()

	now := time.Now().UTC()
	_, errInsert := fixture.conn.InsertGame(fixture.conn.DB, &models.GameSetter{
		ID:                omit.From(testGameID),
		Name:              omit.From("Test Game"),
		DefaultPort:       omit.From(int64(28000)),
		DefaultQueryPort:  omit.From(int64(28001)),
		DefaultMaxPlayers: omit.From(int64(48)),
		WindowsSupport:    omit.From(true),
		CreatedAt:         omit.From(now),
		UpdatedAt:         omit.From(now),
	})
	if errInsert != nil {
		t.Fatalf("InsertGame() error = %v", errInsert)
	}
}
