package db

import (
	"testing"
	"time"
)

func TestInsertAlertHistory(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ah-insert.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "server-local-1", "node-local", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	history, errInsert := conn.InsertAlertHistory(
		rule.ID, "user-owner", "server-local-1", "node-local", "", "server.crash", `{"detail":"oom"}`, "discord", "pending",
	)
	if errInsert != nil {
		t.Fatalf("InsertAlertHistory() error = %v", errInsert)
	}
	if history.ID == "" {
		t.Error("InsertAlertHistory() returned empty ID")
	}
	if history.UserID != "user-owner" {
		t.Errorf("InsertAlertHistory().UserID = %q, want %q", history.UserID, "user-owner")
	}

	ruleID, ruleIDSet := history.AlertRuleID.Get()
	if !ruleIDSet || ruleID != rule.ID {
		t.Errorf("InsertAlertHistory().AlertRuleID = (%q, %v), want (%q, true)", ruleID, ruleIDSet, rule.ID)
	}

	serverID, serverIDSet := history.ServerID.Get()
	if !serverIDSet || serverID != "server-local-1" {
		t.Errorf("InsertAlertHistory().ServerID = (%q, %v), want (%q, true)", serverID, serverIDSet, "server-local-1")
	}

	eventData, eventDataSet := history.EventData.Get()
	if !eventDataSet || eventData != `{"detail":"oom"}` {
		t.Errorf("InsertAlertHistory().EventData = (%q, %v), want (%q, true)", eventData, eventDataSet, `{"detail":"oom"}`)
	}

	if history.ChannelType != "discord" {
		t.Errorf("InsertAlertHistory().ChannelType = %q, want %q", history.ChannelType, "discord")
	}
	if history.DeliveryStatus != "pending" {
		t.Errorf("InsertAlertHistory().DeliveryStatus = %q, want %q", history.DeliveryStatus, "pending")
	}
}

func TestUpdateAlertHistoryDeliveryStatus(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ah-status.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	history, errInsert := conn.InsertAlertHistory(
		rule.ID, "user-owner", "", "", "", "server.crash", "", "discord", "pending",
	)
	if errInsert != nil {
		t.Fatalf("InsertAlertHistory() error = %v", errInsert)
	}

	// Update to sent.
	errSent := conn.UpdateAlertHistoryDeliveryStatus(history.ID, "sent", "")
	if errSent != nil {
		t.Fatalf("UpdateAlertHistoryDeliveryStatus(sent) error = %v", errSent)
	}

	// Update to failed with error message.
	errFailed := conn.UpdateAlertHistoryDeliveryStatus(history.ID, "failed", "webhook returned 500")
	if errFailed != nil {
		t.Fatalf("UpdateAlertHistoryDeliveryStatus(failed) error = %v", errFailed)
	}

	// Verify via GetAlertHistoryByUserID.
	results, errGet := conn.GetAlertHistoryByUserID("user-owner", 10, 0)
	if errGet != nil {
		t.Fatalf("GetAlertHistoryByUserID() error = %v", errGet)
	}
	if len(results) != 1 {
		t.Fatalf("GetAlertHistoryByUserID() len = %d, want 1", len(results))
	}
	if results[0].DeliveryStatus != "failed" {
		t.Errorf("UpdateAlertHistoryDeliveryStatus() DeliveryStatus = %q, want %q", results[0].DeliveryStatus, "failed")
	}
	deliveryError, deliveryErrorSet := results[0].DeliveryError.Get()
	if !deliveryErrorSet || deliveryError != "webhook returned 500" {
		t.Errorf("UpdateAlertHistoryDeliveryStatus() DeliveryError = (%q, %v), want (%q, true)", deliveryError, deliveryErrorSet, "webhook returned 500")
	}
}

func TestGetAlertHistoryByUserID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ah-byuser.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-owner", "user-owner")
	seedNotificationChannel(t, conn, "chan-other", "user-other")

	ruleOwner, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-owner", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule(owner) error = %v", errRule)
	}
	ruleOther, errRuleOther := conn.InsertAlertRule("user-other", "", "", "", "server.crash", "", "chan-other", true)
	if errRuleOther != nil {
		t.Fatalf("InsertAlertRule(other) error = %v", errRuleOther)
	}

	for i := 0; i < 5; i++ {
		_, errInsert := conn.InsertAlertHistory(ruleOwner.ID, "user-owner", "", "", "", "server.crash", "", "discord", "sent")
		if errInsert != nil {
			t.Fatalf("InsertAlertHistory(owner %d) error = %v", i, errInsert)
		}
	}
	_, errInsertOther := conn.InsertAlertHistory(ruleOther.ID, "user-other", "", "", "", "server.crash", "", "discord", "sent")
	if errInsertOther != nil {
		t.Fatalf("InsertAlertHistory(other) error = %v", errInsertOther)
	}

	// Page 1: limit 3, offset 0.
	page1, errPage1 := conn.GetAlertHistoryByUserID("user-owner", 3, 0)
	if errPage1 != nil {
		t.Fatalf("GetAlertHistoryByUserID(page1) error = %v", errPage1)
	}
	if len(page1) != 3 {
		t.Errorf("GetAlertHistoryByUserID(page1) len = %d, want 3", len(page1))
	}

	// Page 2: limit 3, offset 3.
	page2, errPage2 := conn.GetAlertHistoryByUserID("user-owner", 3, 3)
	if errPage2 != nil {
		t.Fatalf("GetAlertHistoryByUserID(page2) error = %v", errPage2)
	}
	if len(page2) != 2 {
		t.Errorf("GetAlertHistoryByUserID(page2) len = %d, want 2", len(page2))
	}
}

func TestGetAlertHistoryByServerID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ah-byserver.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	_, errA := conn.InsertAlertHistory(rule.ID, "user-owner", "server-local-1", "node-local", "", "server.crash", "", "discord", "sent")
	if errA != nil {
		t.Fatalf("InsertAlertHistory(A) error = %v", errA)
	}
	_, errB := conn.InsertAlertHistory(rule.ID, "user-owner", "server-local-1", "node-local", "", "server.crash", "", "discord", "sent")
	if errB != nil {
		t.Fatalf("InsertAlertHistory(B) error = %v", errB)
	}
	_, errC := conn.InsertAlertHistory(rule.ID, "user-owner", "server-other", "node-other", "", "server.crash", "", "discord", "sent")
	if errC != nil {
		t.Fatalf("InsertAlertHistory(C) error = %v", errC)
	}

	results, errGet := conn.GetAlertHistoryByServerID("server-local-1", "node-local", 10, 0)
	if errGet != nil {
		t.Fatalf("GetAlertHistoryByServerID() error = %v", errGet)
	}
	if len(results) != 2 {
		t.Errorf("GetAlertHistoryByServerID() len = %d, want 2", len(results))
	}

	// Pagination.
	paginated, errPaginated := conn.GetAlertHistoryByServerID("server-local-1", "node-local", 1, 0)
	if errPaginated != nil {
		t.Fatalf("GetAlertHistoryByServerID(paginated) error = %v", errPaginated)
	}
	if len(paginated) != 1 {
		t.Errorf("GetAlertHistoryByServerID(paginated) len = %d, want 1", len(paginated))
	}
}

func TestPruneAlertHistory(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ah-prune.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errRule := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errRule != nil {
		t.Fatalf("InsertAlertRule() error = %v", errRule)
	}

	// Insert old record.
	oldTime := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
	_, errOld := conn.SQLDb.Exec(
		`INSERT INTO alert_history (id, alert_rule_id, user_id, event_type, channel_type, delivery_status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"old-1", rule.ID, "user-owner", "server.crash", "discord", "sent", oldTime,
	)
	if errOld != nil {
		t.Fatalf("Insert old record error = %v", errOld)
	}

	// Insert recent record.
	_, errRecent := conn.InsertAlertHistory(rule.ID, "user-owner", "", "", "", "server.crash", "", "discord", "sent")
	if errRecent != nil {
		t.Fatalf("InsertAlertHistory(recent) error = %v", errRecent)
	}

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	deleted, errPrune := conn.PruneAlertHistory(cutoff)
	if errPrune != nil {
		t.Fatalf("PruneAlertHistory() error = %v", errPrune)
	}
	if deleted != 1 {
		t.Errorf("PruneAlertHistory() deleted = %d, want 1", deleted)
	}

	// Verify recent record still exists.
	remaining, errGet := conn.GetAlertHistoryByUserID("user-owner", 10, 0)
	if errGet != nil {
		t.Fatalf("GetAlertHistoryByUserID() after prune error = %v", errGet)
	}
	if len(remaining) != 1 {
		t.Errorf("GetAlertHistoryByUserID() after prune len = %d, want 1", len(remaining))
	}
}
