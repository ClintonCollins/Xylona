package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/mailer"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

type notifChanFixture struct {
	conn         *db.Connection
	service      *XylonaService
	secureCookie *securecookie.SecureCookie
}

func newNotifChanFixture(t *testing.T) *notifChanFixture {
	t.Helper()

	conn := newRPCFixtureConnection(t, "notif-chan-rpc.sqlite")

	seedNotifChanFixture(t, conn)

	secureCookieInst := securecookie.New(
		[]byte("0123456789abcdef0123456789abcdef"),
		[]byte("0123456789abcdef"),
	)

	service := &XylonaService{
		ctx:          context.Background(),
		db:           conn,
		secureCookie: secureCookieInst,
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
// - "user-noperm": non-super user with no alert permissions.
func seedNotifChanFixture(t *testing.T, conn *db.Connection) {
	t.Helper()

	ctx := context.Background()

	// Node + local_settings (required by schema constraints)
	_, errNode := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO node (id, name, listen_url, enabled) VALUES (?, ?, ?, ?)`,
		"node-local", "Local Node", "http://localhost:8080", true,
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
		`INSERT INTO ip (address, usable, external, node_id) VALUES (?, ?, ?, ?)`,
		"127.0.0.1", true, false, "node-local",
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

func TestNotificationChannelRPC_Unauthenticated(t *testing.T) {
	t.Parallel()

	fixture := newNotifChanFixture(t)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "create",
			call: func() error {
				req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
					Name:        "test",
					ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
					Config:      `{"url":"https://discord.com/api/webhooks/1/abc"}`,
					Enabled:     true,
				})
				_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
				return errCreate
			},
		},
		{
			name: "update",
			call: func() error {
				req := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
					Id:   "some-id",
					Name: "x",
				})
				_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), req)
				return errUpdate
			},
		},
		{
			name: "delete",
			call: func() error {
				req := connect.NewRequest(&xylona.DeleteNotificationChannelRequest{Id: "x"})
				_, errDelete := fixture.service.DeleteNotificationChannel(context.Background(), req)
				return errDelete
			},
		},
		{
			name: "list",
			call: func() error {
				req := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
				_, errList := fixture.service.ListNotificationChannels(context.Background(), req)
				return errList
			},
		},
		{
			name: "test channel",
			call: func() error {
				req := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: "x"})
				_, errTest := fixture.service.TestNotificationChannel(context.Background(), req)
				return errTest
			},
		},
		{
			name: "get local smtp status",
			call: func() error {
				req := connect.NewRequest(&xylona.GetLocalSMTPStatusRequest{})
				_, errGet := fixture.service.GetLocalSMTPStatus(context.Background(), req)
				return errGet
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errCall := tt.call()
			if errCall == nil {
				t.Fatalf("%s expected error, got nil", tt.name)
			}

			code := connect.CodeOf(errCall)
			if code != connect.CodeUnauthenticated {
				t.Errorf("%s code = %v, want %v", tt.name, code, connect.CodeUnauthenticated)
			}
		})
	}
}

func TestNotificationChannelRPC_NoPermission(t *testing.T) {
	t.Parallel()

	fixture := newNotifChanFixture(t)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "create",
			call: func() error {
				req := connect.NewRequest(&xylona.CreateNotificationChannelRequest{
					Name:        "test",
					ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
					Config:      `{"url":"https://discord.com/api/webhooks/1/abc"}`,
					Enabled:     true,
				})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")
				_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
				return errCreate
			},
		},
		{
			name: "update",
			call: func() error {
				req := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
					Id:   "some-id",
					Name: "x",
				})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")
				_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), req)
				return errUpdate
			},
		},
		{
			name: "delete",
			call: func() error {
				req := connect.NewRequest(&xylona.DeleteNotificationChannelRequest{Id: "x"})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")
				_, errDelete := fixture.service.DeleteNotificationChannel(context.Background(), req)
				return errDelete
			},
		},
		{
			name: "list",
			call: func() error {
				req := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")
				_, errList := fixture.service.ListNotificationChannels(context.Background(), req)
				return errList
			},
		},
		{
			name: "test channel",
			call: func() error {
				req := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: "x"})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")
				_, errTest := fixture.service.TestNotificationChannel(context.Background(), req)
				return errTest
			},
		},
		{
			name: "get local smtp status",
			call: func() error {
				req := connect.NewRequest(&xylona.GetLocalSMTPStatusRequest{})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-noperm")
				_, errGet := fixture.service.GetLocalSMTPStatus(context.Background(), req)
				return errGet
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errCall := tt.call()
			if errCall == nil {
				t.Fatalf("%s expected error, got nil", tt.name)
			}

			code := connect.CodeOf(errCall)
			if code != connect.CodePermissionDenied {
				t.Errorf("%s code = %v, want %v", tt.name, code, connect.CodePermissionDenied)
			}
		})
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
	if resp.Msg == nil || resp.Msg.GetChannel() == nil {
		t.Fatalf("CreateNotificationChannel(superuser) returned nil channel")
	}
	if resp.Msg.GetChannel().GetName() != "discord-super" {
		t.Errorf("name = %q, want %q", resp.Msg.GetChannel().GetName(), "discord-super")
	}
	if resp.Msg.GetChannel().GetChannelType() != xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD {
		t.Errorf("channel_type = %v, want WEBHOOK_DISCORD", resp.Msg.GetChannel().GetChannelType())
	}
	if resp.Msg.GetChannel().GetUserId() != "user-super" {
		t.Errorf("user_id = %q, want %q", resp.Msg.GetChannel().GetUserId(), "user-super")
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
	if resp.Msg == nil || resp.Msg.GetChannel() == nil {
		t.Fatalf("CreateNotificationChannel(global perm) returned nil channel")
	}
	if resp.Msg.GetChannel().GetUserId() != "user-alerts" {
		t.Errorf("user_id = %q, want %q", resp.Msg.GetChannel().GetUserId(), "user-alerts")
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestCreateNotificationChannel_InvalidInput(t *testing.T) {
	tests := []struct {
		name string
		req  *xylona.CreateNotificationChannelRequest
	}{
		{
			name: "empty name",
			req: &xylona.CreateNotificationChannelRequest{
				Name:        "",
				ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD,
				Config:      `{"url":"x"}`,
				Enabled:     true,
			},
		},
		{
			name: "unspecified type",
			req: &xylona.CreateNotificationChannelRequest{
				Name:        "bad-type",
				ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_UNSPECIFIED,
				Config:      `{}`,
				Enabled:     true,
			},
		},
		{
			name: "invalid webhook url scheme",
			req: &xylona.CreateNotificationChannelRequest{
				Name:        "bad-webhook",
				ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_WEBHOOK_GENERIC,
				Config:      `{"url":"file:///tmp/webhook"}`,
				Enabled:     true,
			},
		},
		{
			name: "invalid email recipient",
			req: &xylona.CreateNotificationChannelRequest{
				Name:        "bad-email",
				ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
				Config:      `{"to":"victim@example.com\r\nBcc: attacker@example.com","smtp_source":"node"}`,
				Enabled:     true,
			},
		},
		{
			name: "custom smtp missing user",
			req: &xylona.CreateNotificationChannelRequest{
				Name:        "missing-user",
				ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
				Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_password":"secret","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
				Enabled:     true,
			},
		},
		{
			name: "custom smtp missing password",
			req: &xylona.CreateNotificationChannelRequest{
				Name:        "missing-password",
				ChannelType: xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_EMAIL,
				Config:      `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
				Enabled:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newNotifChanFixture(t)
			req := connect.NewRequest(tt.req)
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

			_, errCreate := fixture.service.CreateNotificationChannel(context.Background(), req)
			if errCreate == nil {
				t.Fatalf("CreateNotificationChannel() error = nil, want invalid argument")
			}
			if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
				t.Errorf("code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
			}
		})
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
	errUnmarshal := json.Unmarshal([]byte(resp.Msg.GetChannel().GetConfig()), &config)
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
	channel := createResp.Msg.GetChannel()
	if channel.GetId() == "" {
		t.Fatalf("Create returned empty ID")
	}
	if !channel.GetEnabled() {
		t.Errorf("Enabled = false, want true")
	}
	if channel.GetCreatedAt() == nil {
		t.Errorf("CreatedAt is nil")
	}
	if channel.GetUpdatedAt() == nil {
		t.Errorf("UpdatedAt is nil")
	}

	// List
	listReq := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq, "user-alerts")

	listResp, errList := fixture.service.ListNotificationChannels(context.Background(), listReq)
	if errList != nil {
		t.Fatalf("List error = %v", errList)
	}
	if len(listResp.Msg.GetChannels()) != 1 {
		t.Fatalf("List returned %d channels, want 1", len(listResp.Msg.GetChannels()))
	}
	if listResp.Msg.GetChannels()[0].GetId() != channel.GetId() {
		t.Errorf("List[0].Id = %q, want %q", listResp.Msg.GetChannels()[0].GetId(), channel.GetId())
	}

	// Update
	updateReq := connect.NewRequest(&xylona.UpdateNotificationChannelRequest{
		Id:      channel.GetId(),
		Name:    "renamed-channel",
		Config:  `{"url":"https://example.com/hook2"}`,
		Enabled: false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-alerts")

	updateResp, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("Update error = %v", errUpdate)
	}
	if updateResp.Msg.GetChannel().GetName() != "renamed-channel" {
		t.Errorf("Update name = %q, want %q", updateResp.Msg.GetChannel().GetName(), "renamed-channel")
	}
	if updateResp.Msg.GetChannel().GetEnabled() {
		t.Errorf("Update Enabled = true, want false")
	}

	// Delete
	deleteReq := connect.NewRequest(&xylona.DeleteNotificationChannelRequest{
		Id: channel.GetId(),
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
	if len(listResp2.Msg.GetChannels()) != 0 {
		t.Errorf("List after delete returned %d channels, want 0", len(listResp2.Msg.GetChannels()))
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
	if len(listSuperResp.Msg.GetChannels()) != 1 {
		t.Errorf("Super sees %d channels, want 1", len(listSuperResp.Msg.GetChannels()))
	}

	// Alert user sees only their channel
	listAlerts := connect.NewRequest(&xylona.ListNotificationChannelsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listAlerts, "user-alerts")

	listAlertsResp, errListAlerts := fixture.service.ListNotificationChannels(context.Background(), listAlerts)
	if errListAlerts != nil {
		t.Fatalf("List for alerts user error = %v", errListAlerts)
	}
	if len(listAlertsResp.Msg.GetChannels()) != 1 {
		t.Errorf("Alert user sees %d channels, want 1", len(listAlertsResp.Msg.GetChannels()))
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
	channelID := createResp.Msg.GetChannel().GetId()

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
	channelID := createResp.Msg.GetChannel().GetId()

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
	if len(listResp.Msg.GetChannels()) != 1 {
		t.Errorf("expected channel to survive cross-user delete, got %d channels", len(listResp.Msg.GetChannels()))
	}
}

// ---------------------------------------------------------------------------
// Update + Delete permission and validation
// ---------------------------------------------------------------------------

func TestUpdateNotificationChannel_InvalidInput(t *testing.T) {
	tests := []struct {
		name string
		req  func(t *testing.T, fixture *notifChanFixture) *xylona.UpdateNotificationChannelRequest
	}{
		{
			name: "empty id",
			req: func(_ *testing.T, _ *notifChanFixture) *xylona.UpdateNotificationChannelRequest {
				return &xylona.UpdateNotificationChannelRequest{
					Id:   "",
					Name: "x",
				}
			},
		},
		{
			name: "empty name",
			req: func(_ *testing.T, _ *notifChanFixture) *xylona.UpdateNotificationChannelRequest {
				return &xylona.UpdateNotificationChannelRequest{
					Id:   "some-id",
					Name: "",
				}
			},
		},
		{
			name: "invalid webhook url scheme",
			req: func(t *testing.T, fixture *notifChanFixture) *xylona.UpdateNotificationChannelRequest {
				t.Helper()

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

				return &xylona.UpdateNotificationChannelRequest{
					Id:      createResp.Msg.GetChannel().GetId(),
					Name:    "webhook-channel",
					Config:  `{"url":"gopher://127.0.0.1/internal"}`,
					Enabled: true,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newNotifChanFixture(t)
			updateReq := connect.NewRequest(tt.req(t, fixture))
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-super")

			_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), updateReq)
			if errUpdate == nil {
				t.Fatalf("UpdateNotificationChannel() error = nil, want invalid argument")
			}
			if connect.CodeOf(errUpdate) != connect.CodeInvalidArgument {
				t.Errorf("code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeInvalidArgument)
			}
		})
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
	if len(resp.Msg.GetChannels()) != 1 {
		t.Fatalf("ListNotificationChannels() len = %d, want 1", len(resp.Msg.GetChannels()))
	}
	if strings.Contains(resp.Msg.GetChannels()[0].GetConfig(), "secret-token") {
		t.Fatalf("Config = %q, want masked secret", resp.Msg.GetChannels()[0].GetConfig())
	}
	if !strings.Contains(resp.Msg.GetChannels()[0].GetConfig(), `"url":"********"`) {
		t.Fatalf("Config = %q, want masked url field", resp.Msg.GetChannels()[0].GetConfig())
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
	if len(resp.Msg.GetChannels()) != 1 {
		t.Fatalf("ListNotificationChannels() len = %d, want 1", len(resp.Msg.GetChannels()))
	}
	if resp.Msg.GetChannels()[0].GetConfig() != `{"url":"https://discord.com/api/webhooks/1/original"}` {
		t.Fatalf("Config = %q, want original config", resp.Msg.GetChannels()[0].GetConfig())
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
	if resp.Msg.GetConfigured() {
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
	if !resp.Msg.GetConfigured() {
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
	if len(resp.Msg.GetChannels()) != 1 {
		t.Fatalf("ListNotificationChannels() len = %d, want 1", len(resp.Msg.GetChannels()))
	}

	var config map[string]any
	errUnmarshal := json.Unmarshal([]byte(resp.Msg.GetChannels()[0].GetConfig()), &config)
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

func TestTestNotificationChannel_InvalidLookup(t *testing.T) {
	tests := []struct {
		name string
		id   string
		code connect.Code
	}{
		{name: "empty id", id: "", code: connect.CodeInvalidArgument},
		{name: "not found", id: "nonexistent", code: connect.CodeNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newNotifChanFixture(t)

			req := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: tt.id})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

			_, errTest := fixture.service.TestNotificationChannel(context.Background(), req)
			if errTest == nil {
				t.Fatalf("TestNotificationChannel() error = nil, want %v", tt.code)
			}
			if connect.CodeOf(errTest) != tt.code {
				t.Errorf("code = %v, want %v", connect.CodeOf(errTest), tt.code)
			}
		})
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

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.GetChannel().GetId()})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

	testResp, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest != nil {
		t.Fatalf("TestNotificationChannel error = %v", errTest)
	}
	if testResp.Msg.GetSuccess() {
		t.Errorf("Success = true, want false (not implemented)")
	}
	if testResp.Msg.GetError() == "" {
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

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.GetChannel().GetId()})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

	testResp, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest != nil {
		t.Fatalf("TestNotificationChannel() error = %v", errTest)
	}
	if !testResp.Msg.GetSuccess() {
		t.Fatalf("success = false, want true: %s", testResp.Msg.GetError())
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

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.GetChannel().GetId()})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

	testResp, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest != nil {
		t.Fatalf("TestNotificationChannel() error = %v", errTest)
	}
	if !testResp.Msg.GetSuccess() {
		t.Fatalf("success = false, want true: %s", testResp.Msg.GetError())
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
		testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.GetChannel().GetId()})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-super")

		testResp, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
		if errTest != nil {
			t.Fatalf("TestNotificationChannel() unexpected error before rate limit: %v", errTest)
		}
		if !testResp.Msg.GetSuccess() {
			t.Fatalf("success = false before rate limit: %s", testResp.Msg.GetError())
		}
	}

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.GetChannel().GetId()})
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

	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.GetChannel().GetId()})
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
		Id:      createResp.Msg.GetChannel().GetId(),
		Name:    "test-channel",
		Config:  `{"to":"alerts@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_password":"","smtp_password_configured":true,"smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
		Enabled: false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-super")

	_, errUpdate := fixture.service.UpdateNotificationChannel(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("UpdateNotificationChannel() error = %v", errUpdate)
	}

	channel, errGet := fixture.conn.GetNotificationChannelByID(createResp.Msg.GetChannel().GetId())
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
	testReq := connect.NewRequest(&xylona.TestNotificationChannelRequest{Id: createResp.Msg.GetChannel().GetId()})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, testReq, "user-alerts")

	_, errTest := fixture.service.TestNotificationChannel(context.Background(), testReq)
	if errTest == nil {
		t.Fatalf("expected error testing another user's channel, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodeNotFound {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodeNotFound)
	}
}
