package rpc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/securecookie"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

type alertRulesFixture struct {
	conn         *db.Connection
	service      *XylonaService
	secureCookie *securecookie.SecureCookie
	channelID    string // notification channel belonging to user-alerts
}

func newAlertRulesFixture(t *testing.T) *alertRulesFixture {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "alert-rules-rpc.sqlite")
	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("failed to close test db: %v", errClose)
		}
	})

	migrationSource := &migrate.FileMigrationSource{
		Dir: filepath.Join("..", "..", "sql", "migrations"),
	}
	migrate.SetTable("migrations")
	_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrate != nil {
		t.Fatalf("failed to apply migrations: %v", errMigrate)
	}
	_, errAlterGame := conn.SQLDb.ExecContext(
		context.Background(),
		`alter table game add column binds_to_all_ips boolean not null default false`,
	)
	if errAlterGame != nil && !strings.Contains(strings.ToLower(errAlterGame.Error()), "duplicate column name") {
		t.Fatalf("failed to ensure game.binds_to_all_ips column: %v", errAlterGame)
	}

	channelID := seedAlertRulesFixture(t, conn)

	secureCookieInst := securecookie.New(
		[]byte("0123456789abcdef0123456789abcdef"),
		[]byte("0123456789abcdef"),
	)

	service := &XylonaService{
		ctx:          context.Background(),
		db:           conn,
		secureCookie: secureCookieInst,
		listCache:    newRemoteServerListCache(remoteServerListCacheTTL),
	}

	return &alertRulesFixture{
		conn:         conn,
		service:      service,
		secureCookie: secureCookieInst,
		channelID:    channelID,
	}
}

// seedAlertRulesFixture creates:
// - "node-local": local node
// - local_settings pointing to node-local
// - "user-super": superuser (bypasses permission checks)
// - "user-alerts": non-super user with a global role carrying alerts.manage
// - "user-noperm": non-super user with no alert permissions
// - "server-local-1": a local game server owned by user-alerts
// - a notification channel belonging to user-alerts (returned ID)
// - a notification channel belonging to user-super (for cross-user tests)
func seedAlertRulesFixture(t *testing.T, conn *db.Connection) string {
	t.Helper()

	ctx := context.Background()

	// Node + local_settings
	_, errNode := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO node (id, name, is_local, host, port, base_url, enabled) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"node-local", "Local Node", true, "localhost", 8080, "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("failed to insert node: %v", errNode)
	}

	_, errSettings := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO local_settings (id, node_id) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET node_id = excluded.node_id`,
		"node-local",
	)
	if errSettings != nil {
		t.Fatalf("failed to insert local_settings: %v", errSettings)
	}

	_, errIP := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO ip (address, usable, external) VALUES (?, ?, ?)`,
		"127.0.0.1", true, false,
	)
	if errIP != nil {
		t.Fatalf("failed to insert ip: %v", errIP)
	}

	// Superuser
	_, errSuper := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), datetime('now'))`,
		"user-super", "superadmin", "super@example.com", "Super", "User", "hash", true,
	)
	if errSuper != nil {
		t.Fatalf("failed to insert superuser: %v", errSuper)
	}

	// User with alerts.manage via global role
	_, errAlerts := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), datetime('now'))`,
		"user-alerts", "alertuser", "alerts@example.com", "Alert", "User", "hash", false,
	)
	if errAlerts != nil {
		t.Fatalf("failed to insert alerts user: %v", errAlerts)
	}

	// User without permission
	_, errNoPerm := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), datetime('now'))`,
		"user-noperm", "noperm", "noperm@example.com", "No", "Perm", "hash", false,
	)
	if errNoPerm != nil {
		t.Fatalf("failed to insert noperm user: %v", errNoPerm)
	}

	// Create a custom role with alerts.manage permission
	_, errRole := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO role (id, name, description, is_system) VALUES (?, ?, ?, ?)`,
		"role-alerts", "Alert Manager", "Can manage alerts", false,
	)
	if errRole != nil {
		t.Fatalf("failed to insert role: %v", errRole)
	}

	_, errRolePerm := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
		"role-alerts", "alerts.manage",
	)
	if errRolePerm != nil {
		t.Fatalf("failed to insert role_permission: %v", errRolePerm)
	}

	// Assign global role to user-alerts
	_, errAssign := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user_role_assignment (id, user_id, role_id, game_server_id, granted_by) VALUES (?, ?, ?, NULL, ?)`,
		"assign-alerts", "user-alerts", "role-alerts", "user-super",
	)
	if errAssign != nil {
		t.Fatalf("failed to insert user_role_assignment: %v", errAssign)
	}

	// A local game server owned by user-alerts
	_, errServer := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO game_server
		 (id, user_id, name, game_id, start_command, status, set_players, max_players, map, ip, port, query_port, directory, node_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-local-1", "user-alerts", "Test Server", "minecraft", "java -jar server.jar", "OFFLINE",
		20, 20, "world", "127.0.0.1", 25565, 25565, t.TempDir(), "node-local",
	)
	if errServer != nil {
		t.Fatalf("failed to insert game server: %v", errServer)
	}

	// Notification channel belonging to user-alerts
	channel, errChan := conn.InsertNotificationChannel(
		"user-alerts",
		"Test Discord",
		xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD.String(),
		`{"url":"https://discord.com/api/webhooks/1/abc"}`,
		true,
	)
	if errChan != nil {
		t.Fatalf("failed to insert notification channel: %v", errChan)
	}

	// Notification channel belonging to user-super (for cross-user tests)
	_, errSuperChan := conn.InsertNotificationChannel(
		"user-super",
		"Super Discord",
		xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD.String(),
		`{"url":"https://discord.com/api/webhooks/2/def"}`,
		true,
	)
	if errSuperChan != nil {
		t.Fatalf("failed to insert super notification channel: %v", errSuperChan)
	}

	return channel.ID
}

// ptrStr returns a pointer to the given string value.
//
//go:fix inline
func ptrStr(s string) *string {
	return new(s)
}

// ---------------------------------------------------------------------------
// Auth + permission gate tests
// ---------------------------------------------------------------------------

func TestCreateAlertRule_Unauthenticated(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeUnauthenticated)
	}
}

func TestCreateAlertRule_NoPermission(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(no permission) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodePermissionDenied)
	}
}

func TestUpdateAlertRule_Unauthenticated(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.UpdateAlertRuleRequest{
		Id:                    "some-id",
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})

	_, errUpdate := fixture.service.UpdateAlertRule(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("UpdateAlertRule(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeUnauthenticated)
	}
}

func TestUpdateAlertRule_NoPermission(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.UpdateAlertRuleRequest{
		Id:                    "some-id",
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errUpdate := fixture.service.UpdateAlertRule(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("UpdateAlertRule(no permission) expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodePermissionDenied)
	}
}

func TestDeleteAlertRule_Unauthenticated(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.DeleteAlertRuleRequest{
		Id: "some-id",
	})

	_, errDelete := fixture.service.DeleteAlertRule(context.Background(), req)
	if errDelete == nil {
		t.Fatalf("DeleteAlertRule(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errDelete) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errDelete), connect.CodeUnauthenticated)
	}
}

func TestDeleteAlertRule_NoPermission(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.DeleteAlertRuleRequest{
		Id: "some-id",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errDelete := fixture.service.DeleteAlertRule(context.Background(), req)
	if errDelete == nil {
		t.Fatalf("DeleteAlertRule(no permission) expected error, got nil")
	}
	if connect.CodeOf(errDelete) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errDelete), connect.CodePermissionDenied)
	}
}

func TestListAlertRules_Unauthenticated(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.ListAlertRulesRequest{})

	_, errList := fixture.service.ListAlertRules(context.Background(), req)
	if errList == nil {
		t.Fatalf("ListAlertRules(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errList) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errList), connect.CodeUnauthenticated)
	}
}

func TestListAlertRules_NoPermission(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.ListAlertRulesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errList := fixture.service.ListAlertRules(context.Background(), req)
	if errList == nil {
		t.Fatalf("ListAlertRules(no permission) expected error, got nil")
	}
	if connect.CodeOf(errList) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errList), connect.CodePermissionDenied)
	}
}

func TestGetAlertHistory_Unauthenticated(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.GetAlertHistoryRequest{})

	_, errGet := fixture.service.GetAlertHistory(context.Background(), req)
	if errGet == nil {
		t.Fatalf("GetAlertHistory(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errGet) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errGet), connect.CodeUnauthenticated)
	}
}

func TestGetAlertHistory_NoPermission(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.GetAlertHistoryRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errGet := fixture.service.GetAlertHistory(context.Background(), req)
	if errGet == nil {
		t.Fatalf("GetAlertHistory(no permission) expected error, got nil")
	}
	if connect.CodeOf(errGet) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errGet), connect.CodePermissionDenied)
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestCreateAlertRule_UnspecifiedEventType(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_UNSPECIFIED,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(unspecified event type) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateAlertRule_EmptyNotificationChannelID(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: "",
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(empty channel ID) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateAlertRule_ChannelNotBelongingToUser(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Get the channel belonging to user-super
	channels, errGet := fixture.conn.GetNotificationChannelsByUserID("user-super")
	if errGet != nil {
		t.Fatalf("failed to get super user channels: %v", errGet)
	}
	if len(channels) == 0 {
		t.Fatalf("expected at least one channel for user-super")
	}
	superChannelID := channels[0].ID

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: superChannelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(other user's channel) expected error, got nil")
	}
	code := connect.CodeOf(errCreate)
	if code != connect.CodeInvalidArgument && code != connect.CodeNotFound {
		t.Errorf("code = %v, want CodeInvalidArgument or CodeNotFound", code)
	}
}

func TestCreateAlertRule_ServerIDWithoutServerNodeID(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		ServerId:              new("server-local-1"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(server_id without server_node_id) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateAlertRule_ServerNodeIDWithoutServerID(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		ServerNodeId:          new("node-local"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(server_node_id without server_id) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateAlertRule_ServerIDAndNodeIDBothSet(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		ServerId:              new("server-local-1"),
		ServerNodeId:          new("node-local"),
		NodeId:                new("node-local"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(server_id and node_id both set) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateAlertRule_NodeEventWithServerID(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		ServerId:              new("server-local-1"),
		ServerNodeId:          new("node-local"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(node event with server_id) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateAlertRule_ServerEventWithNodeID(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		NodeId:                new("node-local"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateAlertRule(server event with node_id) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

// ---------------------------------------------------------------------------
// CRUD flow tests
// ---------------------------------------------------------------------------

func TestAlertRuleCRUD(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Create an all-servers crash rule
	createReq := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		Condition:             "",
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-alerts")

	createResp, errCreate := fixture.service.CreateAlertRule(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateAlertRule() error = %v", errCreate)
	}
	if createResp.Msg == nil || createResp.Msg.Rule == nil {
		t.Fatalf("CreateAlertRule() returned nil rule")
	}
	rule := createResp.Msg.Rule
	if rule.Id == "" {
		t.Errorf("CreateAlertRule() returned empty ID")
	}
	if rule.UserId != "user-alerts" {
		t.Errorf("user_id = %q, want %q", rule.UserId, "user-alerts")
	}
	if rule.EventType != xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH {
		t.Errorf("event_type = %v, want CRASH", rule.EventType)
	}
	if rule.NotificationChannelId != fixture.channelID {
		t.Errorf("notification_channel_id = %q, want %q", rule.NotificationChannelId, fixture.channelID)
	}
	if !rule.Enabled {
		t.Errorf("enabled = false, want true")
	}
	if rule.ServerId != nil {
		t.Errorf("server_id = %q, want nil", *rule.ServerId)
	}
	if rule.NodeId != nil {
		t.Errorf("node_id = %q, want nil", *rule.NodeId)
	}

	// List — should contain the new rule
	listReq := connect.NewRequest(&xylona.ListAlertRulesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq, "user-alerts")

	listResp, errList := fixture.service.ListAlertRules(context.Background(), listReq)
	if errList != nil {
		t.Fatalf("ListAlertRules() error = %v", errList)
	}
	if len(listResp.Msg.Rules) != 1 {
		t.Fatalf("ListAlertRules() len = %d, want 1", len(listResp.Msg.Rules))
	}
	if listResp.Msg.Rules[0].Id != rule.Id {
		t.Errorf("listed rule ID = %q, want %q", listResp.Msg.Rules[0].Id, rule.Id)
	}

	// Update — change to STATUS_CHANGE, disable
	updateReq := connect.NewRequest(&xylona.UpdateAlertRuleRequest{
		Id:                    rule.Id,
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_STATUS_CHANGE,
		Condition:             "status=OFFLINE",
		NotificationChannelId: fixture.channelID,
		Enabled:               false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-alerts")

	updateResp, errUpdate := fixture.service.UpdateAlertRule(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("UpdateAlertRule() error = %v", errUpdate)
	}
	if updateResp.Msg == nil || updateResp.Msg.Rule == nil {
		t.Fatalf("UpdateAlertRule() returned nil rule")
	}
	updated := updateResp.Msg.Rule
	if updated.EventType != xylona.AlertEventType_ALERT_EVENT_TYPE_STATUS_CHANGE {
		t.Errorf("event_type = %v, want STATUS_CHANGE", updated.EventType)
	}
	if updated.Condition != "status=OFFLINE" {
		t.Errorf("condition = %q, want %q", updated.Condition, "status=OFFLINE")
	}
	if updated.Enabled {
		t.Errorf("enabled = true, want false")
	}

	// Delete
	deleteReq := connect.NewRequest(&xylona.DeleteAlertRuleRequest{
		Id: rule.Id,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, deleteReq, "user-alerts")

	_, errDelete := fixture.service.DeleteAlertRule(context.Background(), deleteReq)
	if errDelete != nil {
		t.Fatalf("DeleteAlertRule() error = %v", errDelete)
	}

	// List — should be empty now
	listReq2 := connect.NewRequest(&xylona.ListAlertRulesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq2, "user-alerts")

	listResp2, errList2 := fixture.service.ListAlertRules(context.Background(), listReq2)
	if errList2 != nil {
		t.Fatalf("ListAlertRules() after delete error = %v", errList2)
	}
	if len(listResp2.Msg.Rules) != 0 {
		t.Errorf("ListAlertRules() after delete len = %d, want 0", len(listResp2.Msg.Rules))
	}
}

func TestCreateAlertRule_ServerScoped(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		ServerId:              new("server-local-1"),
		ServerNodeId:          new("node-local"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CPU_THRESHOLD,
		Condition:             ">90",
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	resp, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate != nil {
		t.Fatalf("CreateAlertRule(server-scoped) error = %v", errCreate)
	}
	rule := resp.Msg.Rule
	if rule.ServerId == nil || *rule.ServerId != "server-local-1" {
		t.Errorf("server_id = %v, want %q", rule.ServerId, "server-local-1")
	}
	if rule.ServerNodeId == nil || *rule.ServerNodeId != "node-local" {
		t.Errorf("server_node_id = %v, want %q", rule.ServerNodeId, "node-local")
	}
	if rule.NodeId != nil {
		t.Errorf("node_id = %v, want nil", rule.NodeId)
	}
}

func TestCreateAlertRule_NodeScoped(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		NodeId:                new("node-local"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD,
		Condition:             ">95",
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	resp, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate != nil {
		t.Fatalf("CreateAlertRule(node-scoped) error = %v", errCreate)
	}
	rule := resp.Msg.Rule
	if rule.NodeId == nil || *rule.NodeId != "node-local" {
		t.Errorf("node_id = %v, want %q", rule.NodeId, "node-local")
	}
	if rule.ServerId != nil {
		t.Errorf("server_id = %v, want nil", rule.ServerId)
	}
}

func TestCreateAlertRule_AllNodesNodeEvent(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Node event with no node_id means "all nodes"
	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD,
		Condition:             ">80",
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	resp, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate != nil {
		t.Fatalf("CreateAlertRule(all-nodes) error = %v", errCreate)
	}
	rule := resp.Msg.Rule
	if rule.NodeId != nil {
		t.Errorf("node_id = %v, want nil (all-nodes)", rule.NodeId)
	}
	if rule.ServerId != nil {
		t.Errorf("server_id = %v, want nil", rule.ServerId)
	}
}

func TestCreateAlertRule_SuperUser(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Superuser uses their own channel
	channels, errGet := fixture.conn.GetNotificationChannelsByUserID("user-super")
	if errGet != nil {
		t.Fatalf("failed to get super channels: %v", errGet)
	}
	if len(channels) == 0 {
		t.Fatalf("expected at least one channel for user-super")
	}

	req := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: channels[0].ID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	resp, errCreate := fixture.service.CreateAlertRule(context.Background(), req)
	if errCreate != nil {
		t.Fatalf("CreateAlertRule(superuser) error = %v", errCreate)
	}
	if resp.Msg.Rule.UserId != "user-super" {
		t.Errorf("user_id = %q, want %q", resp.Msg.Rule.UserId, "user-super")
	}
}

// ---------------------------------------------------------------------------
// ListAlertRules filtered by server
// ---------------------------------------------------------------------------

func TestListAlertRules_FilteredByServer(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Create a rule scoped to server-local-1
	createReq1 := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		ServerId:              new("server-local-1"),
		ServerNodeId:          new("node-local"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq1, "user-alerts")
	_, errCreate1 := fixture.service.CreateAlertRule(context.Background(), createReq1)
	if errCreate1 != nil {
		t.Fatalf("CreateAlertRule(server-scoped) error = %v", errCreate1)
	}

	// Create a global rule (no server)
	createReq2 := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_STATUS_CHANGE,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq2, "user-alerts")
	_, errCreate2 := fixture.service.CreateAlertRule(context.Background(), createReq2)
	if errCreate2 != nil {
		t.Fatalf("CreateAlertRule(global) error = %v", errCreate2)
	}

	// List all — should get 2
	listAllReq := connect.NewRequest(&xylona.ListAlertRulesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listAllReq, "user-alerts")
	listAllResp, errListAll := fixture.service.ListAlertRules(context.Background(), listAllReq)
	if errListAll != nil {
		t.Fatalf("ListAlertRules(all) error = %v", errListAll)
	}
	if len(listAllResp.Msg.Rules) != 2 {
		t.Errorf("ListAlertRules(all) len = %d, want 2", len(listAllResp.Msg.Rules))
	}

	// List filtered by server — should get 1
	listFilterReq := connect.NewRequest(&xylona.ListAlertRulesRequest{
		ServerId:     new("server-local-1"),
		ServerNodeId: new("node-local"),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listFilterReq, "user-alerts")
	listFilterResp, errListFilter := fixture.service.ListAlertRules(context.Background(), listFilterReq)
	if errListFilter != nil {
		t.Fatalf("ListAlertRules(filtered) error = %v", errListFilter)
	}
	if len(listFilterResp.Msg.Rules) != 1 {
		t.Errorf("ListAlertRules(filtered) len = %d, want 1", len(listFilterResp.Msg.Rules))
	}
}

func TestListAlertRules_FilteredByServerDoesNotLeakOtherUsersRules(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	createOwnReq := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		ServerId:              new("server-local-1"),
		ServerNodeId:          new("node-local"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createOwnReq, "user-alerts")

	_, errCreateOwn := fixture.service.CreateAlertRule(context.Background(), createOwnReq)
	if errCreateOwn != nil {
		t.Fatalf("CreateAlertRule(user-alerts) error = %v", errCreateOwn)
	}

	superChannels, errSuperChannels := fixture.conn.GetNotificationChannelsByUserID("user-super")
	if errSuperChannels != nil {
		t.Fatalf("GetNotificationChannelsByUserID(user-super) error = %v", errSuperChannels)
	}
	if len(superChannels) == 0 {
		t.Fatal("expected user-super to have at least one notification channel")
	}

	createSuperReq := connect.NewRequest(&xylona.CreateAlertRuleRequest{
		ServerId:              new("server-local-1"),
		ServerNodeId:          new("node-local"),
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_STATUS_CHANGE,
		NotificationChannelId: superChannels[0].ID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createSuperReq, "user-super")

	_, errCreateSuper := fixture.service.CreateAlertRule(context.Background(), createSuperReq)
	if errCreateSuper != nil {
		t.Fatalf("CreateAlertRule(user-super) error = %v", errCreateSuper)
	}

	listReq := connect.NewRequest(&xylona.ListAlertRulesRequest{
		ServerId:     new("server-local-1"),
		ServerNodeId: new("node-local"),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq, "user-alerts")

	listResp, errList := fixture.service.ListAlertRules(context.Background(), listReq)
	if errList != nil {
		t.Fatalf("ListAlertRules(filtered) error = %v", errList)
	}
	if len(listResp.Msg.Rules) != 1 {
		t.Fatalf("ListAlertRules(filtered) len = %d, want 1", len(listResp.Msg.Rules))
	}
	if listResp.Msg.Rules[0].UserId != "user-alerts" {
		t.Fatalf("ListAlertRules(filtered) user_id = %q, want %q", listResp.Msg.Rules[0].UserId, "user-alerts")
	}
}

// ---------------------------------------------------------------------------
// GetAlertHistory tests
// ---------------------------------------------------------------------------

func TestGetAlertHistory_Basic(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Insert some history records directly via DB
	_, errH1 := fixture.conn.InsertAlertHistory(
		"", "user-alerts", "", "", "", "ALERT_EVENT_TYPE_CRASH",
		`{"message":"server crashed"}`,
		"NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD",
		"DELIVERY_STATUS_SENT",
	)
	if errH1 != nil {
		t.Fatalf("InsertAlertHistory(1) error = %v", errH1)
	}

	_, errH2 := fixture.conn.InsertAlertHistory(
		"", "user-alerts", "server-local-1", "node-local", "", "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		`{"cpu":95}`,
		"NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD",
		"DELIVERY_STATUS_PENDING",
	)
	if errH2 != nil {
		t.Fatalf("InsertAlertHistory(2) error = %v", errH2)
	}

	// Query all history for user
	histReq := connect.NewRequest(&xylona.GetAlertHistoryRequest{
		Limit:  50,
		Offset: 0,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, histReq, "user-alerts")

	histResp, errHist := fixture.service.GetAlertHistory(context.Background(), histReq)
	if errHist != nil {
		t.Fatalf("GetAlertHistory() error = %v", errHist)
	}
	if len(histResp.Msg.Entries) != 2 {
		t.Errorf("GetAlertHistory() len = %d, want 2", len(histResp.Msg.Entries))
	}
}

func TestGetAlertHistory_Pagination(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Insert 3 history records
	for i := range 3 {
		_, errH := fixture.conn.InsertAlertHistory(
			"", "user-alerts", "", "", "", "ALERT_EVENT_TYPE_CRASH",
			`{"index":`+string(rune('0'+i))+`}`,
			"NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD",
			"DELIVERY_STATUS_SENT",
		)
		if errH != nil {
			t.Fatalf("InsertAlertHistory(%d) error = %v", i, errH)
		}
	}

	// Page 1: limit=2, offset=0
	req1 := connect.NewRequest(&xylona.GetAlertHistoryRequest{
		Limit:  2,
		Offset: 0,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req1, "user-alerts")

	resp1, errResp1 := fixture.service.GetAlertHistory(context.Background(), req1)
	if errResp1 != nil {
		t.Fatalf("GetAlertHistory(page1) error = %v", errResp1)
	}
	if len(resp1.Msg.Entries) != 2 {
		t.Errorf("GetAlertHistory(page1) len = %d, want 2", len(resp1.Msg.Entries))
	}

	// Page 2: limit=2, offset=2
	req2 := connect.NewRequest(&xylona.GetAlertHistoryRequest{
		Limit:  2,
		Offset: 2,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req2, "user-alerts")

	resp2, errResp2 := fixture.service.GetAlertHistory(context.Background(), req2)
	if errResp2 != nil {
		t.Fatalf("GetAlertHistory(page2) error = %v", errResp2)
	}
	if len(resp2.Msg.Entries) != 1 {
		t.Errorf("GetAlertHistory(page2) len = %d, want 1", len(resp2.Msg.Entries))
	}
}

func TestGetAlertHistory_FilteredByServer(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Insert one history with server, one without
	_, errH1 := fixture.conn.InsertAlertHistory(
		"", "user-alerts", "server-local-1", "node-local", "", "ALERT_EVENT_TYPE_CRASH",
		`{"server":"crash"}`,
		"NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD",
		"DELIVERY_STATUS_SENT",
	)
	if errH1 != nil {
		t.Fatalf("InsertAlertHistory(server-scoped) error = %v", errH1)
	}

	_, errH2 := fixture.conn.InsertAlertHistory(
		"", "user-alerts", "", "", "", "ALERT_EVENT_TYPE_CRASH",
		`{"global":"crash"}`,
		"NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD",
		"DELIVERY_STATUS_SENT",
	)
	if errH2 != nil {
		t.Fatalf("InsertAlertHistory(global) error = %v", errH2)
	}

	// Filter by server
	req := connect.NewRequest(&xylona.GetAlertHistoryRequest{
		ServerId:     new("server-local-1"),
		ServerNodeId: new("node-local"),
		Limit:        50,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	resp, errResp := fixture.service.GetAlertHistory(context.Background(), req)
	if errResp != nil {
		t.Fatalf("GetAlertHistory(filtered) error = %v", errResp)
	}
	if len(resp.Msg.Entries) != 1 {
		t.Errorf("GetAlertHistory(filtered) len = %d, want 1", len(resp.Msg.Entries))
	}
}

func TestGetAlertHistory_FilteredByServerDoesNotLeakOtherUsersEntries(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	_, errOwn := fixture.conn.InsertAlertHistory(
		"", "user-alerts", "server-local-1", "node-local", "", "ALERT_EVENT_TYPE_CRASH",
		`{"message":"own alert"}`,
		"NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD",
		"DELIVERY_STATUS_SENT",
	)
	if errOwn != nil {
		t.Fatalf("InsertAlertHistory(user-alerts) error = %v", errOwn)
	}

	_, errOther := fixture.conn.InsertAlertHistory(
		"", "user-super", "server-local-1", "node-local", "", "ALERT_EVENT_TYPE_STATUS_CHANGE",
		`{"message":"other user alert"}`,
		"NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD",
		"DELIVERY_STATUS_SENT",
	)
	if errOther != nil {
		t.Fatalf("InsertAlertHistory(user-super) error = %v", errOther)
	}

	req := connect.NewRequest(&xylona.GetAlertHistoryRequest{
		ServerId:     new("server-local-1"),
		ServerNodeId: new("node-local"),
		Limit:        50,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	resp, errResp := fixture.service.GetAlertHistory(context.Background(), req)
	if errResp != nil {
		t.Fatalf("GetAlertHistory(filtered) error = %v", errResp)
	}
	if len(resp.Msg.Entries) != 1 {
		t.Fatalf("GetAlertHistory(filtered) len = %d, want 1", len(resp.Msg.Entries))
	}
	if resp.Msg.Entries[0].UserId != "user-alerts" {
		t.Fatalf("GetAlertHistory(filtered) user_id = %q, want %q", resp.Msg.Entries[0].UserId, "user-alerts")
	}
}

func TestGetAlertHistory_DefaultLimit(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Request with limit=0 should use default (50)
	req := connect.NewRequest(&xylona.GetAlertHistoryRequest{
		Limit:  0,
		Offset: 0,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	// Should succeed (no entries, but no error)
	resp, errResp := fixture.service.GetAlertHistory(context.Background(), req)
	if errResp != nil {
		t.Fatalf("GetAlertHistory(default limit) error = %v", errResp)
	}
	if resp.Msg.Entries == nil {
		// entries should be non-nil (empty slice or nil is fine)
		_ = resp.Msg.Entries
	}
}

// ---------------------------------------------------------------------------
// Update validation tests
// ---------------------------------------------------------------------------

func TestUpdateAlertRule_UnspecifiedEventType(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.UpdateAlertRuleRequest{
		Id:                    "some-id",
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_UNSPECIFIED,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errUpdate := fixture.service.UpdateAlertRule(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("UpdateAlertRule(unspecified event type) expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
	}
}

func TestUpdateAlertRule_EmptyID(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.UpdateAlertRuleRequest{
		Id:                    "",
		EventType:             xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
		NotificationChannelId: fixture.channelID,
		Enabled:               true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errUpdate := fixture.service.UpdateAlertRule(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("UpdateAlertRule(empty id) expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
	}
}

func TestDeleteAlertRule_EmptyID(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.DeleteAlertRuleRequest{
		Id: "",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errDelete := fixture.service.DeleteAlertRule(context.Background(), req)
	if errDelete == nil {
		t.Fatalf("DeleteAlertRule(empty id) expected error, got nil")
	}
	if connect.CodeOf(errDelete) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errDelete), connect.CodeInvalidArgument)
	}
}

// ---------------------------------------------------------------------------
// Half-pair rejection tests
// ---------------------------------------------------------------------------

func TestListAlertRules_HalfPairServerID(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.ListAlertRulesRequest{
		ServerId: new("server-local-1"),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errList := fixture.service.ListAlertRules(context.Background(), req)
	if errList == nil {
		t.Fatalf("ListAlertRules(half-pair) expected error, got nil")
	}
	if connect.CodeOf(errList) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errList), connect.CodeInvalidArgument)
	}
}

func TestGetAlertHistory_HalfPairServerID(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	req := connect.NewRequest(&xylona.GetAlertHistoryRequest{
		ServerId: new("server-local-1"),
		Limit:    50,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	_, errHist := fixture.service.GetAlertHistory(context.Background(), req)
	if errHist == nil {
		t.Fatalf("GetAlertHistory(half-pair) expected error, got nil")
	}
	if connect.CodeOf(errHist) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errHist), connect.CodeInvalidArgument)
	}
}

// ---------------------------------------------------------------------------
// Superuser sees all alert history
// ---------------------------------------------------------------------------

func TestGetAlertHistory_SuperuserSeesAll(t *testing.T) {
	fixture := newAlertRulesFixture(t)

	// Insert history for user-alerts
	_, errH1 := fixture.conn.InsertAlertHistory(
		"", "user-alerts", "", "", "", "ALERT_EVENT_TYPE_CRASH",
		`{"msg":"alert user crash"}`,
		"NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD",
		"DELIVERY_STATUS_SENT",
	)
	if errH1 != nil {
		t.Fatalf("InsertAlertHistory(user-alerts) error = %v", errH1)
	}

	// Insert history for user-super
	_, errH2 := fixture.conn.InsertAlertHistory(
		"", "user-super", "", "", "", "ALERT_EVENT_TYPE_STATUS_CHANGE",
		`{"msg":"super user event"}`,
		"NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD",
		"DELIVERY_STATUS_SENT",
	)
	if errH2 != nil {
		t.Fatalf("InsertAlertHistory(user-super) error = %v", errH2)
	}

	// Superuser should see both records
	req := connect.NewRequest(&xylona.GetAlertHistoryRequest{
		Limit: 50,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	resp, errResp := fixture.service.GetAlertHistory(context.Background(), req)
	if errResp != nil {
		t.Fatalf("GetAlertHistory(superuser) error = %v", errResp)
	}
	if len(resp.Msg.Entries) != 2 {
		t.Errorf("GetAlertHistory(superuser) len = %d, want 2", len(resp.Msg.Entries))
	}

	// Non-superuser should only see their own
	req2 := connect.NewRequest(&xylona.GetAlertHistoryRequest{
		Limit: 50,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req2, "user-alerts")

	resp2, errResp2 := fixture.service.GetAlertHistory(context.Background(), req2)
	if errResp2 != nil {
		t.Fatalf("GetAlertHistory(user-alerts) error = %v", errResp2)
	}
	if len(resp2.Msg.Entries) != 1 {
		t.Errorf("GetAlertHistory(user-alerts) len = %d, want 1", len(resp2.Msg.Entries))
	}
}
