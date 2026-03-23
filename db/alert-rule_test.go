package db

import (
	"context"
	"strings"
	"testing"
	"time"
)

func seedNotificationChannel(t *testing.T, conn *Connection, id, userID string) {
	t.Helper()
	now := time.Now().UTC()
	_, errInsert := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO notification_channel (id, user_id, name, channel_type, config, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		id, userID, "Test Channel "+id, "discord", `{"webhook":"https://example.com"}`, now, now,
	)
	if errInsert != nil {
		t.Fatalf("seedNotificationChannel(%q) error = %v", id, errInsert)
	}
}

func TestInsertAlertRule(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-insert.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errInsert := conn.InsertAlertRule(
		"user-owner", "server-local-1", "node-local", "", "server.crash", `{"threshold":3}`, "chan-1", true,
	)
	if errInsert != nil {
		t.Fatalf("InsertAlertRule() error = %v", errInsert)
	}
	if rule.ID == "" {
		t.Error("InsertAlertRule() returned empty ID")
	}
	if rule.UserID != "user-owner" {
		t.Errorf("InsertAlertRule().UserID = %q, want %q", rule.UserID, "user-owner")
	}

	serverID, serverIDSet := rule.ServerID.Get()
	if !serverIDSet || serverID != "server-local-1" {
		t.Errorf("InsertAlertRule().ServerID = (%q, %v), want (%q, true)", serverID, serverIDSet, "server-local-1")
	}

	serverNodeID, serverNodeIDSet := rule.ServerNodeID.Get()
	if !serverNodeIDSet || serverNodeID != "node-local" {
		t.Errorf("InsertAlertRule().ServerNodeID = (%q, %v), want (%q, true)", serverNodeID, serverNodeIDSet, "node-local")
	}

	_, nodeIDSet := rule.NodeID.Get()
	if nodeIDSet {
		t.Error("InsertAlertRule().NodeID should be NULL for server-scoped rule")
	}

	if rule.EventType != "server.crash" {
		t.Errorf("InsertAlertRule().EventType = %q, want %q", rule.EventType, "server.crash")
	}

	condition, conditionSet := rule.Condition.Get()
	if !conditionSet || condition != `{"threshold":3}` {
		t.Errorf("InsertAlertRule().Condition = (%q, %v), want (%q, true)", condition, conditionSet, `{"threshold":3}`)
	}

	if rule.NotificationChannelID != "chan-1" {
		t.Errorf("InsertAlertRule().NotificationChannelID = %q, want %q", rule.NotificationChannelID, "chan-1")
	}
	if rule.Enabled != 1 {
		t.Errorf("InsertAlertRule().Enabled = %d, want 1", rule.Enabled)
	}
}

func TestInsertAlertRule_AllServers(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-allservers.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errInsert := conn.InsertAlertRule(
		"user-owner", "", "", "", "server.start", "", "chan-1", true,
	)
	if errInsert != nil {
		t.Fatalf("InsertAlertRule() error = %v", errInsert)
	}

	_, serverIDSet := rule.ServerID.Get()
	if serverIDSet {
		t.Error("InsertAlertRule().ServerID should be NULL for all-servers rule")
	}
	_, serverNodeIDSet := rule.ServerNodeID.Get()
	if serverNodeIDSet {
		t.Error("InsertAlertRule().ServerNodeID should be NULL for all-servers rule")
	}
	_, nodeIDSet := rule.NodeID.Get()
	if nodeIDSet {
		t.Error("InsertAlertRule().NodeID should be NULL for all-servers rule")
	}
}

func TestInsertAlertRule_NodeScoped(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-nodescoped.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errInsert := conn.InsertAlertRule(
		"user-owner", "", "", "node-local", "node.offline", "", "chan-1", true,
	)
	if errInsert != nil {
		t.Fatalf("InsertAlertRule() error = %v", errInsert)
	}

	_, serverIDSet := rule.ServerID.Get()
	if serverIDSet {
		t.Error("InsertAlertRule().ServerID should be NULL for node-scoped rule")
	}
	nodeID, nodeIDSet := rule.NodeID.Get()
	if !nodeIDSet || nodeID != "node-local" {
		t.Errorf("InsertAlertRule().NodeID = (%q, %v), want (%q, true)", nodeID, nodeIDSet, "node-local")
	}
}

func TestGetAlertRulesByUserID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-byuser.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-owner", "user-owner")
	seedNotificationChannel(t, conn, "chan-other", "user-other")

	_, errFirst := conn.InsertAlertRule("user-owner", "", "", "", "server.start", "", "chan-owner", true)
	if errFirst != nil {
		t.Fatalf("InsertAlertRule(1) error = %v", errFirst)
	}
	_, errSecond := conn.InsertAlertRule("user-owner", "", "", "", "server.stop", "", "chan-owner", true)
	if errSecond != nil {
		t.Fatalf("InsertAlertRule(2) error = %v", errSecond)
	}
	_, errOther := conn.InsertAlertRule("user-other", "", "", "", "server.start", "", "chan-other", true)
	if errOther != nil {
		t.Fatalf("InsertAlertRule(3) error = %v", errOther)
	}

	rules, errGet := conn.GetAlertRulesByUserID("user-owner")
	if errGet != nil {
		t.Fatalf("GetAlertRulesByUserID() error = %v", errGet)
	}
	if len(rules) != 2 {
		t.Errorf("GetAlertRulesByUserID() len = %d, want 2", len(rules))
	}
}

func TestGetAlertRulesByServerID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-byserver.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	_, errFirst := conn.InsertAlertRule("user-owner", "server-local-1", "node-local", "", "server.crash", "", "chan-1", true)
	if errFirst != nil {
		t.Fatalf("InsertAlertRule(server-local-1) error = %v", errFirst)
	}
	_, errSecond := conn.InsertAlertRule("user-owner", "server-other", "node-other", "", "server.crash", "", "chan-1", true)
	if errSecond != nil {
		t.Fatalf("InsertAlertRule(server-other) error = %v", errSecond)
	}

	rules, errGet := conn.GetAlertRulesByServerID("server-local-1", "node-local")
	if errGet != nil {
		t.Fatalf("GetAlertRulesByServerID() error = %v", errGet)
	}
	if len(rules) != 1 {
		t.Errorf("GetAlertRulesByServerID() len = %d, want 1", len(rules))
	}
}

func TestGetEnabledAlertRulesByEventType(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-enabled.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	_, errEnabled := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", true)
	if errEnabled != nil {
		t.Fatalf("InsertAlertRule(enabled) error = %v", errEnabled)
	}
	_, errDisabled := conn.InsertAlertRule("user-owner", "", "", "", "server.crash", "", "chan-1", false)
	if errDisabled != nil {
		t.Fatalf("InsertAlertRule(disabled) error = %v", errDisabled)
	}
	_, errDifferent := conn.InsertAlertRule("user-owner", "", "", "", "server.start", "", "chan-1", true)
	if errDifferent != nil {
		t.Fatalf("InsertAlertRule(different event) error = %v", errDifferent)
	}

	rules, errGet := conn.GetEnabledAlertRulesByEventType("server.crash")
	if errGet != nil {
		t.Fatalf("GetEnabledAlertRulesByEventType() error = %v", errGet)
	}
	if len(rules) != 1 {
		t.Errorf("GetEnabledAlertRulesByEventType() len = %d, want 1", len(rules))
	}
}

func TestUpdateAlertRule(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-update.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")
	seedNotificationChannel(t, conn, "chan-2", "user-owner")

	rule, errInsert := conn.InsertAlertRule("user-owner", "", "", "", "server.start", "", "chan-1", true)
	if errInsert != nil {
		t.Fatalf("InsertAlertRule() error = %v", errInsert)
	}

	errUpdate := conn.UpdateAlertRule(
		rule.ID, "user-owner", "server-local-1", "node-local", "", "server.crash", `{"new":true}`, "chan-2", false,
	)
	if errUpdate != nil {
		t.Fatalf("UpdateAlertRule() error = %v", errUpdate)
	}

	rules, errGet := conn.GetAlertRulesByUserID("user-owner")
	if errGet != nil {
		t.Fatalf("GetAlertRulesByUserID() error = %v", errGet)
	}
	if len(rules) != 1 {
		t.Fatalf("GetAlertRulesByUserID() len = %d, want 1", len(rules))
	}
	updated := rules[0]
	if updated.EventType != "server.crash" {
		t.Errorf("UpdateAlertRule().EventType = %q, want %q", updated.EventType, "server.crash")
	}
	if updated.NotificationChannelID != "chan-2" {
		t.Errorf("UpdateAlertRule().NotificationChannelID = %q, want %q", updated.NotificationChannelID, "chan-2")
	}
	if updated.Enabled != 0 {
		t.Errorf("UpdateAlertRule().Enabled = %d, want 0", updated.Enabled)
	}
	serverID, serverIDSet := updated.ServerID.Get()
	if !serverIDSet || serverID != "server-local-1" {
		t.Errorf("UpdateAlertRule().ServerID = (%q, %v), want (%q, true)", serverID, serverIDSet, "server-local-1")
	}
}

func TestDeleteAlertRule(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-delete.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	rule, errInsert := conn.InsertAlertRule("user-owner", "", "", "", "server.start", "", "chan-1", true)
	if errInsert != nil {
		t.Fatalf("InsertAlertRule() error = %v", errInsert)
	}

	errDelete := conn.DeleteAlertRule(rule.ID, "user-owner")
	if errDelete != nil {
		t.Fatalf("DeleteAlertRule() error = %v", errDelete)
	}

	rules, errGet := conn.GetAlertRulesByUserID("user-owner")
	if errGet != nil {
		t.Fatalf("GetAlertRulesByUserID() error = %v", errGet)
	}
	if len(rules) != 0 {
		t.Errorf("GetAlertRulesByUserID() after delete len = %d, want 0", len(rules))
	}
}

func TestAlertRule_CheckConstraint_ServerPair(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-check-pair.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	// Insert via raw SQL with server_id set but server_node_id NULL — should fail CHECK.
	_, errExec := conn.SQLDb.Exec(
		`INSERT INTO alert_rule (id, user_id, server_id, server_node_id, event_type, notification_channel_id, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, NULL, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"bad-rule-1", "user-owner", "some-server", "server.crash", "chan-1",
	)
	if errExec == nil {
		t.Fatal("Expected CHECK constraint failure for server_id without server_node_id, but insert succeeded")
	}
	if !strings.Contains(strings.ToUpper(errExec.Error()), "CHECK") &&
		!strings.Contains(strings.ToUpper(errExec.Error()), "CONSTRAINT") {
		t.Errorf("Expected CHECK constraint error, got: %v", errExec)
	}
}

func TestAlertRule_CheckConstraint_MutualExclusive(t *testing.T) {
	conn := newRBACMigratedConnection(t, "ar-check-mutual.sqlite")
	seedRBACFixture(t, conn)
	seedNotificationChannel(t, conn, "chan-1", "user-owner")

	// Insert via raw SQL with both server_id and node_id set — should fail CHECK.
	_, errExec := conn.SQLDb.Exec(
		`INSERT INTO alert_rule (id, user_id, server_id, server_node_id, node_id, event_type, notification_channel_id, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"bad-rule-2", "user-owner", "some-server", "some-node", "node-local", "server.crash", "chan-1",
	)
	if errExec == nil {
		t.Fatal("Expected CHECK constraint failure for both server_id and node_id set, but insert succeeded")
	}
	if !strings.Contains(strings.ToUpper(errExec.Error()), "CHECK") &&
		!strings.Contains(strings.ToUpper(errExec.Error()), "CONSTRAINT") {
		t.Errorf("Expected CHECK constraint error, got: %v", errExec)
	}
}
