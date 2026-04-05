package db

import (
	"sync"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGetOrCreateAlertState_Creates(t *testing.T) {
	conn := newRBACMigratedConnection(t, "as-create.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	state, errCreate := conn.GetOrCreateAlertState(rule.ID, "server", "server-local-1", "node-local")
	if errCreate != nil {
		t.Fatalf("GetOrCreateAlertState() error = %v", errCreate)
	}
	if state.ID == "" {
		t.Error("GetOrCreateAlertState() returned empty ID")
	}
	if state.AlertRuleID != rule.ID {
		t.Errorf("GetOrCreateAlertState().AlertRuleID = %q, want %q", state.AlertRuleID, rule.ID)
	}
	if state.EntityType != "server" {
		t.Errorf("GetOrCreateAlertState().EntityType = %q, want %q", state.EntityType, "server")
	}
	if state.EntityID != "server-local-1" {
		t.Errorf("GetOrCreateAlertState().EntityID = %q, want %q", state.EntityID, "server-local-1")
	}
	if state.EntityNodeID != "node-local" {
		t.Errorf("GetOrCreateAlertState().EntityNodeID = %q, want %q", state.EntityNodeID, "node-local")
	}
	if state.Triggered != 0 {
		t.Errorf("GetOrCreateAlertState().Triggered = %d, want 0", state.Triggered)
	}
}

func TestGetOrCreateAlertState_GetsExisting(t *testing.T) {
	conn := newRBACMigratedConnection(t, "as-existing.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	first, errFirst := conn.GetOrCreateAlertState(rule.ID, "server", "server-local-1", "node-local")
	if errFirst != nil {
		t.Fatalf("GetOrCreateAlertState(first) error = %v", errFirst)
	}

	second, errSecond := conn.GetOrCreateAlertState(rule.ID, "server", "server-local-1", "node-local")
	if errSecond != nil {
		t.Fatalf("GetOrCreateAlertState(second) error = %v", errSecond)
	}

	if first.ID != second.ID {
		t.Errorf("GetOrCreateAlertState() second call returned different ID: %q vs %q", first.ID, second.ID)
	}
}

func TestUpdateAlertStateTriggered(t *testing.T) {
	conn := newRBACMigratedConnection(t, "as-trigger.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	state, errCreate := conn.GetOrCreateAlertState(rule.ID, "server", "server-local-1", "node-local")
	if errCreate != nil {
		t.Fatalf("GetOrCreateAlertState() error = %v", errCreate)
	}

	// Trigger
	errTrigger := conn.UpdateAlertStateTriggered(state.ID, true)
	if errTrigger != nil {
		t.Fatalf("UpdateAlertStateTriggered(true) error = %v", errTrigger)
	}

	triggered, errGet := conn.GetOrCreateAlertState(rule.ID, "server", "server-local-1", "node-local")
	if errGet != nil {
		t.Fatalf("GetOrCreateAlertState() after trigger error = %v", errGet)
	}
	if triggered.Triggered != 1 {
		t.Errorf("UpdateAlertStateTriggered(true) Triggered = %d, want 1", triggered.Triggered)
	}
	_, triggeredAtSet := triggered.TriggeredAt.Get()
	if !triggeredAtSet {
		t.Error("UpdateAlertStateTriggered(true) TriggeredAt should be set")
	}

	// Un-trigger (resolve)
	errResolve := conn.UpdateAlertStateTriggered(state.ID, false)
	if errResolve != nil {
		t.Fatalf("UpdateAlertStateTriggered(false) error = %v", errResolve)
	}

	resolved, errGetResolved := conn.GetOrCreateAlertState(rule.ID, "server", "server-local-1", "node-local")
	if errGetResolved != nil {
		t.Fatalf("GetOrCreateAlertState() after resolve error = %v", errGetResolved)
	}
	if resolved.Triggered != 0 {
		t.Errorf("UpdateAlertStateTriggered(false) Triggered = %d, want 0", resolved.Triggered)
	}
	_, resolvedAtSet := resolved.ResolvedAt.Get()
	if !resolvedAtSet {
		t.Error("UpdateAlertStateTriggered(false) ResolvedAt should be set")
	}
}

func TestAlertState_UniqueConstraint(t *testing.T) {
	conn := newRBACMigratedConnection(t, "as-unique.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	// Same rule + entity combo returns same row.
	first, errFirst := conn.GetOrCreateAlertState(rule.ID, "server", "srv-1", "node-a")
	if errFirst != nil {
		t.Fatalf("GetOrCreateAlertState(first) error = %v", errFirst)
	}
	second, errSecond := conn.GetOrCreateAlertState(rule.ID, "server", "srv-1", "node-a")
	if errSecond != nil {
		t.Fatalf("GetOrCreateAlertState(second) error = %v", errSecond)
	}
	if first.ID != second.ID {
		t.Errorf("Same rule+entity should reuse row, got IDs %q vs %q", first.ID, second.ID)
	}

	// Different entity creates a new row.
	third, errThird := conn.GetOrCreateAlertState(rule.ID, "server", "srv-2", "node-a")
	if errThird != nil {
		t.Fatalf("GetOrCreateAlertState(third) error = %v", errThird)
	}
	if third.ID == first.ID {
		t.Error("Different entity_id should create new row, got same ID")
	}
}

func TestAlertState_CrossNodeIndependence(t *testing.T) {
	conn := newRBACMigratedConnection(t, "as-crossnode.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	stateA, errA := conn.GetOrCreateAlertState(rule.ID, "server", "srv-1", "node-a")
	if errA != nil {
		t.Fatalf("GetOrCreateAlertState(node-a) error = %v", errA)
	}
	stateB, errB := conn.GetOrCreateAlertState(rule.ID, "server", "srv-1", "node-b")
	if errB != nil {
		t.Fatalf("GetOrCreateAlertState(node-b) error = %v", errB)
	}

	if stateA.ID == stateB.ID {
		t.Error("Same entity_id but different entity_node_id should create separate rows")
	}
}

func TestGetOrCreateAlertState_ConcurrentCallsReuseSingleRow(t *testing.T) {
	conn := newRBACMigratedConnection(t, "as-concurrent.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	const goroutineCount = 20

	start := make(chan struct{})
	results := make(chan *models.AlertState, goroutineCount)
	errs := make(chan error, goroutineCount)
	var wg sync.WaitGroup

	for range goroutineCount {
		wg.Go(func() {
			<-start
			state, errGet := conn.GetOrCreateAlertState(rule.ID, "server", "srv-1", "node-a")
			if errGet != nil {
				errs <- errGet
				return
			}
			results <- state
		})
	}

	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for errGet := range errs {
		if errGet != nil {
			t.Fatalf("GetOrCreateAlertState(concurrent) error = %v", errGet)
		}
	}

	var firstID string
	count := 0
	for state := range results {
		count++
		if firstID == "" {
			firstID = state.ID
		}
		if state.ID != firstID {
			t.Fatalf("GetOrCreateAlertState(concurrent) returned mismatched IDs %q and %q", firstID, state.ID)
		}
	}

	if count != goroutineCount {
		t.Fatalf("GetOrCreateAlertState(concurrent) results = %d, want %d", count, goroutineCount)
	}
}
