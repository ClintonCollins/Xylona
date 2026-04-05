package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/securecookie"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/pkg/mailer"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

type notifChanFixture struct {
	conn         *db.Connection
	service      *XylonaService
	secureCookie *securecookie.SecureCookie
}

func newNotifChanFixture(t *testing.T) *notifChanFixture {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "notif-chan-rpc.sqlite")
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

	seedNotifChanFixture(t, conn)

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

	return &notifChanFixture{
		conn:         conn,
		service:      service,
		secureCookie: secureCookieInst,
	}
}

// seedNotifChanFixture creates:
// - "user-super": superuser (bypasses permission checks)
// - "user-alerts": non-super user with a global role carrying alerts.manage
// - "user-history": non-super user with a global role carrying alerts.view_history
// - "user-noperm": non-super user with no alert permissions
func seedNotifChanFixture(t *testing.T, conn *db.Connection) {
	t.Helper()

	ctx := context.Background()

	// Node + local_settings (required by schema constraints)
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

	// User with alerts.view_history via global role
	_, errHistory := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), datetime('now'))`,
		"user-history", "historyuser", "history@example.com", "History", "User", "hash", false,
	)
	if errHistory != nil {
		t.Fatalf("failed to insert history user: %v", errHistory)
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

	_, errHistoryRole := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO role (id, name, description, is_system) VALUES (?, ?, ?, ?)`,
		"role-alert-history", "Alert History Viewer", "Can view alert history", false,
	)
	if errHistoryRole != nil {
		t.Fatalf("failed to insert history role: %v", errHistoryRole)
	}

	_, errHistoryRolePerm := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
		"role-alert-history", "alerts.view_history",
	)
	if errHistoryRolePerm != nil {
		t.Fatalf("failed to insert history role_permission: %v", errHistoryRolePerm)
	}

	// Assign global role to user-alerts (game_server_id IS NULL)
	_, errAssign := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user_role_assignment (id, user_id, role_id, game_server_id, granted_by) VALUES (?, ?, ?, NULL, ?)`,
		"assign-alerts", "user-alerts", "role-alerts", "user-super",
	)
	if errAssign != nil {
		t.Fatalf("failed to insert user_role_assignment: %v", errAssign)
	}

	_, errAssignHistory := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user_role_assignment (id, user_id, role_id, game_server_id, granted_by) VALUES (?, ?, ?, NULL, ?)`,
		"assign-history", "user-history", "role-alert-history", "user-super",
	)
	if errAssignHistory != nil {
		t.Fatalf("failed to insert history user_role_assignment: %v", errAssignHistory)
	}
}

// ---------------------------------------------------------------------------
// Auth + permission gate tests
// ---------------------------------------------------------------------------

func TestCreateNotificationChannel_Unauthenticated(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "test",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"https://discord.com/api/webhooks/1/abc"}`,
		Enabled:     true,
	})

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateNotificationChannel(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeUnauthenticated)
	}
}

func TestCreateNotificationChannel_NoPermission(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "test",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"https://discord.com/api/webhooks/1/abc"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateNotificationChannel(no permission) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodePermissionDenied)
	}
}

func TestCreateNotificationChannel_SuperUser(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "discord-super",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"https://discord.com/api/webhooks/1/abc"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	resp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel(superuser) error = %v", errCreate)
	}
	if resp.Msg == nil || resp.Msg.Channel == nil {
		t.Fatalf("CreateNotificationChannel(superuser) returned nil channel")
	}
	if resp.Msg.Channel.Name != "discord-super" {
		t.Errorf("name = %q, want %q", resp.Msg.Channel.Name, "discord-super")
	}
	if resp.Msg.Channel.ChannelType != xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD {
		t.Errorf("channel_type = %v, want WEBHOOK_DISCORD", resp.Msg.Channel.ChannelType)
	}
	if resp.Msg.Channel.UserId != "user-super" {
		t.Errorf("user_id = %q, want %q", resp.Msg.Channel.UserId, "user-super")
	}
}

func TestCreateNotificationChannel_GlobalPermission(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "discord-alertuser",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_SLACK,
		Config:      `{"url":"https://hooks.slack.com/services/T00/B00/xxx"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	resp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel(global perm) error = %v", errCreate)
	}
	if resp.Msg == nil || resp.Msg.Channel == nil {
		t.Fatalf("CreateNotificationChannel(global perm) returned nil channel")
	}
	if resp.Msg.Channel.UserId != "user-alerts" {
		t.Errorf("user_id = %q, want %q", resp.Msg.Channel.UserId, "user-alerts")
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestCreateNotificationChannel_EmptyName(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"x"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateNotificationChannel(empty name) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateNotificationChannel_UnspecifiedType(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "bad-type",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_UNSPECIFIED,
		Config:      `{}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateNotificationChannel(unspecified type) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateNotificationChannel_InvalidWebhookURLScheme(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "bad-webhook",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_GENERIC,
		Config:      `{"url":"file:///tmp/webhook"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateNotificationChannel(invalid webhook scheme) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateNotificationChannel_InvalidEmailRecipient(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "bad-email",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"victim@example.com\r\nBcc: attacker@example.com","smtp_source":"node"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate == nil {
		t.Fatalf("CreateNotificationChannel(invalid email recipient) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateNotificationChannel_CustomSMTPRequiresUser(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "missing-user",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_password":"secret","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate == nil {
		t.Fatal("CreateNotificationChannel(custom missing smtp_user) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateNotificationChannel_CustomSMTPRequiresPassword(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "missing-password",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate == nil {
		t.Fatal("CreateNotificationChannel(custom missing smtp_password) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
}

func TestCreateNotificationChannel_CustomSMTPSanitizesPasswordInResponse(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "custom-email",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_password":"secret123","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	resp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel(custom smtp) error = %v", errCreate)
	}

	var config map[string]any
	errUnmarshal := json.Unmarshal([]byte(resp.Msg.Channel.Config), &config)
	if errUnmarshal != nil {
		t.Fatalf("response config unmarshal error = %v", errUnmarshal)
	}

	if gotPassword, ok := config["smtp_password"].(string); !ok || gotPassword != "" {
		t.Fatalf("smtp_password = %#v, want empty string", config["smtp_password"])
	}
	if gotConfigured, ok := config["smtp_password_configured"].(bool); !ok || !gotConfigured {
		t.Fatalf("smtp_password_configured = %#v, want true", config["smtp_password_configured"])
	}
}

// ---------------------------------------------------------------------------
// CRUD flow tests
// ---------------------------------------------------------------------------

func TestNotificationChannelCRUD(t *testing.T) {
	fixture := newNotifChanFixture(t)

	// Create
	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "my-channel",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_GENERIC,
		Config:      `{"url":"https://example.com/hook"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-alerts")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("Create error = %v", errCreate)
	}
	channel := createResp.Msg.Channel
	if channel.Id == "" {
		t.Fatalf("Create returned empty ID")
	}
	if !channel.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if channel.CreatedAt == nil {
		t.Errorf("CreatedAt is nil")
	}
	if channel.UpdatedAt == nil {
		t.Errorf("UpdatedAt is nil")
	}

	// List
	listReq := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq, "user-alerts")

	listResp, errList := fixture.service.ListNotificationChannels(context.Background(), listReq)
	if errList != nil {
		t.Fatalf("List error = %v", errList)
	}
	if len(listResp.Msg.Channels) != 1 {
		t.Fatalf("List returned %d channels, want 1", len(listResp.Msg.Channels))
	}
	if listResp.Msg.Channels[0].Id != channel.Id {
		t.Errorf("List[0].Id = %q, want %q", listResp.Msg.Channels[0].Id, channel.Id)
	}

	// Update
	updateReq := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
		Id:      channel.Id,
		Name:    "renamed-channel",
		Config:  `{"url":"https://example.com/hook2"}`,
		Enabled: false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-alerts")

	updateResp, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("Update error = %v", errUpdate)
	}
	if updateResp.Msg.Channel.Name != "renamed-channel" {
		t.Errorf("Update name = %q, want %q", updateResp.Msg.Channel.Name, "renamed-channel")
	}
	if updateResp.Msg.Channel.Enabled {
		t.Errorf("Update Enabled = true, want false")
	}

	// Delete
	deleteReq := connect.NewRequest(&xylona.DeleteNotificationChannelRequest{
		Id: channel.Id,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, deleteReq, "user-alerts")

	_, errDelete := fixture.service.DeleteNotificationChannel(context.Background(), deleteReq)
	if errDelete != nil {
		t.Fatalf("Delete error = %v", errDelete)
	}

	// List again → empty
	listReq2 := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq2, "user-alerts")

	listResp2, errList2 := fixture.service.ListNotificationChannels(context.Background(), listReq2)
	if errList2 != nil {
		t.Fatalf("List after delete error = %v", errList2)
	}
	if len(listResp2.Msg.Channels) != 0 {
		t.Errorf("List after delete returned %d channels, want 0", len(listResp2.Msg.Channels))
	}
}

// ---------------------------------------------------------------------------
// List isolation: each user only sees their own channels
// ---------------------------------------------------------------------------

func TestListNotificationChannels_UserIsolation(t *testing.T) {
	fixture := newNotifChanFixture(t)

	// Superuser creates a channel
	createReq1 := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "super-channel",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"super@example.com","smtp_source":"node"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq1, "user-super")

	_, errCreate1 := fixture.service.CreateNotificationChannel(context.Background(), createReq1)
	if errCreate1 != nil {
		t.Fatalf("Create for super error = %v", errCreate1)
	}

	// Alert user creates a channel
	createReq2 := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "alerts-channel",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"https://discord.com/api/webhooks/2/def"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq2, "user-alerts")

	_, errCreate2 := fixture.service.CreateNotificationChannel(context.Background(), createReq2)
	if errCreate2 != nil {
		t.Fatalf("Create for alerts user error = %v", errCreate2)
	}

	// Super sees only their channel
	listSuper := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listSuper, "user-super")

	listSuperResp, errListSuper := fixture.service.ListNotificationChannels(context.Background(), listSuper)
	if errListSuper != nil {
		t.Fatalf("List for super error = %v", errListSuper)
	}
	if len(listSuperResp.Msg.Channels) != 1 {
		t.Errorf("Super sees %d channels, want 1", len(listSuperResp.Msg.Channels))
	}

	// Alert user sees only their channel
	listAlerts := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listAlerts, "user-alerts")

	listAlertsResp, errListAlerts := fixture.service.ListNotificationChannels(context.Background(), listAlerts)
	if errListAlerts != nil {
		t.Fatalf("List for alerts user error = %v", errListAlerts)
	}
	if len(listAlertsResp.Msg.Channels) != 1 {
		t.Errorf("Alert user sees %d channels, want 1", len(listAlertsResp.Msg.Channels))
	}
}

// ---------------------------------------------------------------------------
// Cross-user write isolation: update/delete another user's channel
// ---------------------------------------------------------------------------

func TestUpdateNotificationChannel_CrossUserIsolation(t *testing.T) {
	fixture := newNotifChanFixture(t)

	// Superuser creates a channel
	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "super-owned",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"https://discord.com/api/webhooks/1/abc"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("Create error = %v", errCreate)
	}
	channelID := createResp.Msg.Channel.Id

	// user-alerts tries to update it → should get NotFound
	updateReq := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
		Id:      channelID,
		Name:    "hijacked",
		Config:  `{"url":"https://evil.com"}`,
		Enabled: false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-alerts")

	_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), updateReq)
	if errUpdate == nil {
		t.Fatalf("expected error updating another user's channel, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodeNotFound {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeNotFound)
	}
}

func TestDeleteNotificationChannel_CrossUserIsolation(t *testing.T) {
	fixture := newNotifChanFixture(t)

	// Superuser creates a channel
	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "super-owned",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"https://discord.com/api/webhooks/1/abc"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("Create error = %v", errCreate)
	}
	channelID := createResp.Msg.Channel.Id

	// user-alerts tries to delete it → should silently no-op (DB scopes by user_id)
	deleteReq := connect.NewRequest(&xylona.DeleteNotificationChannelRequest{Id: channelID})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, deleteReq, "user-alerts")

	_, errDelete := fixture.service.DeleteNotificationChannel(context.Background(), deleteReq)
	if errDelete != nil {
		t.Fatalf("Delete error = %v (expected silent no-op)", errDelete)
	}

	// Verify the channel still exists for the superuser
	listReq := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq, "user-super")

	listResp, errList := fixture.service.ListNotificationChannels(context.Background(), listReq)
	if errList != nil {
		t.Fatalf("List error = %v", errList)
	}
	if len(listResp.Msg.Channels) != 1 {
		t.Errorf("expected channel to survive cross-user delete, got %d channels", len(listResp.Msg.Channels))
	}
}

// ---------------------------------------------------------------------------
// Update + Delete permission and validation
// ---------------------------------------------------------------------------

func TestUpdateNotificationChannel_Unauthenticated(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
		Id:   "some-id",
		Name: "x",
	})

	_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeUnauthenticated)
	}
}

func TestUpdateNotificationChannel_NoPermission(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
		Id:   "some-id",
		Name: "x",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodePermissionDenied)
	}
}

func TestUpdateNotificationChannel_EmptyID(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
		Id:   "",
		Name: "x",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
	}
}

func TestUpdateNotificationChannel_EmptyName(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
		Id:   "some-id",
		Name: "",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), req)
	if errUpdate == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
	}
}

func TestUpdateNotificationChannel_InvalidWebhookURLScheme(t *testing.T) {
	fixture := newNotifChanFixture(t)

	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "webhook-channel",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_GENERIC,
		Config:      `{"url":"https://example.com/hook"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", errCreate)
	}

	updateReq := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
		Id:      createResp.Msg.Channel.Id,
		Name:    "webhook-channel",
		Config:  `{"url":"gopher://127.0.0.1/internal"}`,
		Enabled: true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-super")

	_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), updateReq)
	if errUpdate == nil {
		t.Fatalf("UpdateNotificationChannel(invalid webhook scheme) expected error, got nil")
	}
	if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
	}
}

func TestDeleteNotificationChannel_Unauthenticated(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.DeleteNotificationChannelRequest{Id: "x"})

	_, errDelete := fixture.service.DeleteNotificationChannel(context.Background(), req)
	if errDelete == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errDelete) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errDelete), connect.CodeUnauthenticated)
	}
}

func TestDeleteNotificationChannel_NoPermission(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.DeleteNotificationChannelRequest{Id: "x"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errDelete := fixture.service.DeleteNotificationChannel(context.Background(), req)
	if errDelete == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errDelete) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errDelete), connect.CodePermissionDenied)
	}
}

func TestDeleteNotificationChannel_EmptyID(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.DeleteNotificationChannelRequest{Id: ""})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errDelete := fixture.service.DeleteNotificationChannel(context.Background(), req)
	if errDelete == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errDelete) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errDelete), connect.CodeInvalidArgument)
	}
}

// ---------------------------------------------------------------------------
// ListNotificationChannels auth
// ---------------------------------------------------------------------------

func TestListNotificationChannels_Unauthenticated(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})

	_, errList := fixture.service.ListNotificationChannels(context.Background(), req)
	if errList == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errList) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errList), connect.CodeUnauthenticated)
	}
}

func TestListNotificationChannels_NoPermission(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errList := fixture.service.ListNotificationChannels(context.Background(), req)
	if errList == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errList) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errList), connect.CodePermissionDenied)
	}
}

func TestListNotificationChannels_ViewHistoryMasksConfig(t *testing.T) {
	fixture := newNotifChanFixture(t)

	_, errInsert := fixture.conn.InsertNotificationChannel(
		"user-history",
		"history-view",
		xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD.String(),
		`{"url":"https://discord.com/api/webhooks/123/secret-token"}`,
		true,
	)
	if errInsert != nil {
		t.Fatalf("InsertNotificationChannel() error = %v", errInsert)
	}

	req := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-history")

	resp, errList := fixture.service.ListNotificationChannels(context.Background(), req)
	if errList != nil {
		t.Fatalf("ListNotificationChannels() error = %v", errList)
	}
	if len(resp.Msg.Channels) != 1 {
		t.Fatalf("ListNotificationChannels() len = %d, want 1", len(resp.Msg.Channels))
	}
	if strings.Contains(resp.Msg.Channels[0].Config, "secret-token") {
		t.Fatalf("Config = %q, want masked secret", resp.Msg.Channels[0].Config)
	}
	if !strings.Contains(resp.Msg.Channels[0].Config, `"url":"********"`) {
		t.Fatalf("Config = %q, want masked url field", resp.Msg.Channels[0].Config)
	}
}

func TestListNotificationChannels_ManageKeepsConfigForEditing(t *testing.T) {
	fixture := newNotifChanFixture(t)

	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "discord-alertuser",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"https://discord.com/api/webhooks/1/original"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-alerts")

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", errCreate)
	}

	listReq := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq, "user-alerts")

	resp, errList := fixture.service.ListNotificationChannels(context.Background(), listReq)
	if errList != nil {
		t.Fatalf("ListNotificationChannels() error = %v", errList)
	}
	if len(resp.Msg.Channels) != 1 {
		t.Fatalf("ListNotificationChannels() len = %d, want 1", len(resp.Msg.Channels))
	}
	if resp.Msg.Channels[0].Config != `{"url":"https://discord.com/api/webhooks/1/original"}` {
		t.Fatalf("Config = %q, want original config", resp.Msg.Channels[0].Config)
	}
}

func TestGetLocalSMTPStatus_NoPermission(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.GetLocalSMTPStatusRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errGet := fixture.service.GetLocalSMTPStatus(context.Background(), req)
	if errGet == nil {
		t.Fatal("GetLocalSMTPStatus(no permission) expected error, got nil")
	}
	if connect.CodeOf(errGet) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errGet), connect.CodePermissionDenied)
	}
}

func TestGetLocalSMTPStatus_ConfiguredState(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.GetLocalSMTPStatusRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-alerts")

	resp, errGet := fixture.service.GetLocalSMTPStatus(context.Background(), req)
	if errGet != nil {
		t.Fatalf("GetLocalSMTPStatus(not configured) error = %v", errGet)
	}
	if resp.Msg.Configured {
		t.Fatal("configured = true, want false")
	}

	errSet := fixture.conn.SetSystemConfig(systemSMTPConfigKey, `{"host":"smtp.example.com","port":587,"user":"mailer","password":"secret","fromAddress":"noreply@example.com","tlsEnabled":true}`)
	if errSet != nil {
		t.Fatalf("SetSystemConfig() error = %v", errSet)
	}

	resp, errGet = fixture.service.GetLocalSMTPStatus(context.Background(), req)
	if errGet != nil {
		t.Fatalf("GetLocalSMTPStatus(configured) error = %v", errGet)
	}
	if !resp.Msg.Configured {
		t.Fatal("configured = false, want true")
	}
}

func TestListNotificationChannels_ManageMasksEmailPasswordButKeepsMetadata(t *testing.T) {
	fixture := newNotifChanFixture(t)

	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "custom-email",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_password":"secret123","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-alerts")

	_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", errCreate)
	}

	listReq := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq, "user-alerts")

	resp, errList := fixture.service.ListNotificationChannels(context.Background(), listReq)
	if errList != nil {
		t.Fatalf("ListNotificationChannels() error = %v", errList)
	}
	if len(resp.Msg.Channels) != 1 {
		t.Fatalf("ListNotificationChannels() len = %d, want 1", len(resp.Msg.Channels))
	}

	var config map[string]any
	errUnmarshal := json.Unmarshal([]byte(resp.Msg.Channels[0].Config), &config)
	if errUnmarshal != nil {
		t.Fatalf("config unmarshal error = %v", errUnmarshal)
	}

	if gotPassword, ok := config["smtp_password"].(string); !ok || gotPassword != "" {
		t.Fatalf("smtp_password = %#v, want empty string", config["smtp_password"])
	}
	if gotConfigured, ok := config["smtp_password_configured"].(bool); !ok || !gotConfigured {
		t.Fatalf("smtp_password_configured = %#v, want true", config["smtp_password_configured"])
	}
}

// ---------------------------------------------------------------------------
// TestNotificationChannel
// ---------------------------------------------------------------------------

func TestTestNotificationChannel_Unauthenticated(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: "x"})

	_, errTest := fixture.service.TestNotificationChannel(context.Background(), req)
	if errTest == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodeUnauthenticated)
	}
}

func TestTestNotificationChannel_NoPermission(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: "x"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")

	_, errTest := fixture.service.TestNotificationChannel(context.Background(), req)
	if errTest == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodePermissionDenied)
	}
}

func TestTestNotificationChannel_EmptyID(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: ""})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errTest := fixture.service.TestNotificationChannel(context.Background(), req)
	if errTest == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodeInvalidArgument)
	}
}

func TestTestNotificationChannel_NotFound(t *testing.T) {
	fixture := newNotifChanFixture(t)

	req := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: "nonexistent"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errTest := fixture.service.TestNotificationChannel(context.Background(), req)
	if errTest == nil {
		t.Fatalf("expected error, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodeNotFound {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodeNotFound)
	}
}

func TestTestNotificationChannel_WebhookNotImplemented(t *testing.T) {
	fixture := newNotifChanFixture(t)

	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "test-channel",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"https://discord.com/api/webhooks/1/abc"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("Create error = %v", errCreate)
	}

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.Channel.Id})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

	testResp, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest != nil {
		t.Fatalf("TestNotificationChannel error = %v", errTest)
	}
	if testResp.Msg.Success {
		t.Errorf("Success = true, want false (not implemented)")
	}
	if testResp.Msg.Error == "" {
		t.Errorf("Error message is empty, expected non-empty message")
	}
}

func TestTestNotificationChannel_EmailCustomSendSuccess(t *testing.T) {
	fixture := newNotifChanFixture(t)

	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "custom-email",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_password":"secret123","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", errCreate)
	}

	fixture.service.testEmailSendFunc = func(_ context.Context, cfg *mailer.SMTPConfig, to string, subject string, _ string) error {
		if cfg == nil {
			t.Fatal("cfg = nil, want custom SMTP config")
		}
		if cfg.Host != "smtp.example.com" {
			t.Errorf("cfg.Host = %q, want %q", cfg.Host, "smtp.example.com")
		}
		if cfg.User != "mailer" {
			t.Errorf("cfg.User = %q, want %q", cfg.User, "mailer")
		}
		if cfg.Password != "secret123" {
			t.Errorf("cfg.Password = %q, want %q", cfg.Password, "secret123")
		}
		if cfg.From != "noreply@example.com" {
			t.Errorf("cfg.From = %q, want %q", cfg.From, "noreply@example.com")
		}
		if to != "alerts@example.com" {
			t.Errorf("to = %q, want %q", to, "alerts@example.com")
		}
		if subject != "Xylona SMTP Test" {
			t.Errorf("subject = %q, want %q", subject, "Xylona SMTP Test")
		}
		return nil
	}

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.Channel.Id})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

	testResp, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest != nil {
		t.Fatalf("TestNotificationChannel() error = %v", errTest)
	}
	if !testResp.Msg.Success {
		t.Fatalf("success = false, want true: %s", testResp.Msg.Error)
	}
}

func TestTestNotificationChannel_EmailNodeSMTPSendSuccess(t *testing.T) {
	fixture := newNotifChanFixture(t)

	errSet := fixture.conn.SetSystemConfig(systemSMTPConfigKey, `{"host":"smtp.example.com","port":587,"user":"mailer","password":"node-secret","fromAddress":"noreply@example.com","tlsEnabled":true}`)
	if errSet != nil {
		t.Fatalf("SetSystemConfig() error = %v", errSet)
	}

	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "node-email",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"alerts@example.com","smtp_source":"node"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", errCreate)
	}

	fixture.service.testEmailSendFunc = func(_ context.Context, cfg *mailer.SMTPConfig, to string, _ string, _ string) error {
		if cfg == nil {
			t.Fatal("cfg = nil, want node SMTP config")
		}
		if cfg.Host != "smtp.example.com" {
			t.Errorf("cfg.Host = %q, want %q", cfg.Host, "smtp.example.com")
		}
		if cfg.Password != "node-secret" {
			t.Errorf("cfg.Password = %q, want %q", cfg.Password, "node-secret")
		}
		if to != "alerts@example.com" {
			t.Errorf("to = %q, want %q", to, "alerts@example.com")
		}
		return nil
	}

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.Channel.Id})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

	testResp, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest != nil {
		t.Fatalf("TestNotificationChannel() error = %v", errTest)
	}
	if !testResp.Msg.Success {
		t.Fatalf("success = false, want true: %s", testResp.Msg.Error)
	}
}

func TestTestNotificationChannel_EmailRateLimited(t *testing.T) {
	fixture := newNotifChanFixture(t)

	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "custom-email",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_password":"secret123","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", errCreate)
	}

	fixture.service.testEmailSendFunc = func(_ context.Context, _ *mailer.SMTPConfig, _ string, _ string, _ string) error {
		return nil
	}

	for range 3 {
		testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.Channel.Id})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

		testResp, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
		if errTest != nil {
			t.Fatalf("TestNotificationChannel() unexpected error before rate limit: %v", errTest)
		}
		if !testResp.Msg.Success {
			t.Fatalf("success = false before rate limit: %s", testResp.Msg.Error)
		}
	}

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.Channel.Id})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

	_, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest == nil {
		t.Fatal("expected rate limit error, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodeResourceExhausted {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodeResourceExhausted)
	}
}

func TestTestNotificationChannel_EmailSendFailureReturnsMessage(t *testing.T) {
	fixture := newNotifChanFixture(t)

	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "custom-email",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_password":"secret123","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", errCreate)
	}

	fixture.service.testEmailSendFunc = func(_ context.Context, _ *mailer.SMTPConfig, _ string, _ string, _ string) error {
		return errors.New("connection refused")
	}

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.Channel.Id})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

	_, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest == nil {
		t.Fatal("TestNotificationChannel() error = nil, want non-nil")
	}
	if connect.CodeOf(errTest) != connect.CodeUnavailable {
		t.Fatalf("TestNotificationChannel() code = %v, want %v", connect.CodeOf(errTest), connect.CodeUnavailable)
	}
	if !strings.Contains(errTest.Error(), "connection refused") {
		t.Errorf("error = %q, want to contain %q", errTest.Error(), "connection refused")
	}
}

func TestUpdateNotificationChannel_CustomSMTPBlankPasswordPreservesStoredPassword(t *testing.T) {
	fixture := newNotifChanFixture(t)

	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "test-channel",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
		Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_password":"secret123","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("Create error = %v", errCreate)
	}

	updateReq := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
		Id:      createResp.Msg.Channel.Id,
		Name:    "test-channel",
		Config:  `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_password":"","smtp_password_configured":true,"smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled: false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-super")

	_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("UpdateNotificationChannel() error = %v", errUpdate)
	}

	channel, errGet := fixture.conn.GetNotificationChannelByID(createResp.Msg.Channel.Id)
	if errGet != nil {
		t.Fatalf("GetNotificationChannelByID() error = %v", errGet)
	}

	if !strings.Contains(channel.Config, `"smtp_password":"secret123"`) {
		t.Fatalf("stored config = %s, want preserved smtp_password", channel.Config)
	}
}

func TestTestNotificationChannel_CrossUserIsolation(t *testing.T) {
	fixture := newNotifChanFixture(t)

	// Superuser creates a channel
	createReq := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
		Name:        "super-owned",
		ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
		Config:      `{"url":"https://discord.com/api/webhooks/1/abc"}`,
		Enabled:     true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-super")

	createResp, errCreate := fixture.service.CreateNotificationChannel(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("Create error = %v", errCreate)
	}

	// user-alerts tries to test it → should get NotFound
	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.Channel.Id})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-alerts")

	_, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest == nil {
		t.Fatalf("expected error testing another user's channel, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodeNotFound {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodeNotFound)
	}
}
