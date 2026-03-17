package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/gorilla/securecookie"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

var testSessionCounter atomic.Int64

type rbacRPCFixture struct {
	conn         *db.Connection
	service      XylonaService
	secureCookie *securecookie.SecureCookie
}

func newRBACRPCFixture(t *testing.T) *rbacRPCFixture {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "rbac-rpc.sqlite")
	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
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

	seedRBACRPCFixture(t, conn)

	secureCookieInst := securecookie.New(
		[]byte("0123456789abcdef0123456789abcdef"),
		[]byte("0123456789abcdef"),
	)

	service := XylonaService{
		ctx:          context.Background(),
		db:           conn,
		secureCookie: secureCookieInst,
		listCache:    newRemoteServerListCache(remoteServerListCacheTTL),
	}

	return &rbacRPCFixture{
		conn:         conn,
		service:      service,
		secureCookie: secureCookieInst,
	}
}

func seedRBACRPCFixture(t *testing.T, conn *db.Connection) {
	t.Helper()

	_, errNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, is_local, host, port, base_url, enabled)
		 values (?, ?, ?, ?, ?, ?, ?)`,
		"node-local", "Local Node", true, "localhost", 8080, "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("failed to insert node: %v", errNode)
	}

	_, errSettings := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into local_settings (id, node_id) values (1, ?) on conflict(id) do update set node_id = excluded.node_id`,
		"node-local",
	)
	if errSettings != nil {
		t.Fatalf("failed to insert local settings: %v", errSettings)
	}

	_, errIP := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into ip (address, usable, external) values (?, ?, ?)`,
		"127.0.0.1", true, false,
	)
	if errIP != nil {
		t.Fatalf("failed to insert ip: %v", errIP)
	}

	now := time.Now().UTC()
	_, errOwner := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-owner", "owner", "owner@example.com", "Owner", "User", "hash", false, now, now, now,
	)
	if errOwner != nil {
		t.Fatalf("failed to insert owner user: %v", errOwner)
	}

	_, errOther := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-other", "other", "other@example.com", "Other", "User", "hash", false, now, now, now,
	)
	if errOther != nil {
		t.Fatalf("failed to insert other user: %v", errOther)
	}

	_, errAdmin := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-admin", "admin", "admin@example.com", "Admin", "User", "hash", true, now, now, now,
	)
	if errAdmin != nil {
		t.Fatalf("failed to insert admin user: %v", errAdmin)
	}

	_, errServer := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, start_command, status, set_players, max_players, map, ip, port, query_port, directory, node_id)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-local-1", "user-owner", "Local One", "minecraft", "java -jar server.jar", "OFFLINE",
		20, 20, "world", "127.0.0.1", 25565, 25565, "/tmp/server-local-1", "node-local",
	)
	if errServer != nil {
		t.Fatalf("failed to insert game server: %v", errServer)
	}
}

func seedRemoteNodeForRBACRPCTests(t *testing.T, conn *db.Connection, nodeID string) {
	t.Helper()

	host := fmt.Sprintf("%s.remote.test", nodeID)
	baseURL := fmt.Sprintf("https://%s", host)

	_, errInsertNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, is_local, host, port, base_url, enabled)
			 values (?, ?, ?, ?, ?, ?, ?)`,
		nodeID, "Remote Node", false, host, 8443, baseURL, true,
	)
	if errInsertNode != nil {
		t.Fatalf("failed to insert remote node: %v", errInsertNode)
	}
}

func addSessionCookieHeader[T any](
	t *testing.T,
	conn *db.Connection,
	secureCookieInst *securecookie.SecureCookie,
	request *connect.Request[T],
	userID string,
) {
	t.Helper()

	now := time.Now().UTC()
	seq := testSessionCounter.Add(1)
	sessionID := fmt.Sprintf("session-%s-%d", userID, seq)
	sessionToken := "a"

	_, errCreateSession := conn.CreateUserSession(&models.UserSessionSetter{
		ID:        omit.From(sessionID),
		UserID:    omit.From(userID),
		Token:     omit.From(sessionToken),
		ExpiresAt: omit.From(now.Add(24 * time.Hour)),
	})
	if errCreateSession != nil {
		t.Fatalf("failed to create user session: %v", errCreateSession)
	}

	encodedToken, errEncode := secureCookieInst.Encode(gatekeeper.SessionTokenCookieName, sessionToken)
	if errEncode != nil {
		t.Fatalf("failed to encode session token: %v", errEncode)
	}

	cookieHeader := gatekeeper.SessionIDCookieName + "=" + sessionID + "; " +
		gatekeeper.SessionTokenCookieName + "=" + encodedToken
	request.Header().Set("Cookie", cookieHeader)

	sessionCookies, errGetSession := gatekeeper.GetSessionFromHeader(request.Header())
	if errGetSession != nil {
		t.Fatalf("failed to parse auth cookies from request header: %v (header=%q)", errGetSession, cookieHeader)
	}
	if sessionCookies.SessionID != sessionID {
		t.Fatalf("parsed session id = %q, want %q", sessionCookies.SessionID, sessionID)
	}
}

func TestGrantGameServerAccessAuthorizationAndShape(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	tests := []struct {
		name     string
		userID   string
		targetID string
		roleID   string
		wantErr  bool
		wantCode connect.Code
	}{
		{
			name:     "owner can grant",
			userID:   "user-owner",
			targetID: "user-other",
			roleID:   "viewer",
			wantErr:  false,
		},
		{
			name:     "super user can grant",
			userID:   "user-admin",
			targetID: "user-other",
			roleID:   "operator",
			wantErr:  false,
		},
		{
			name:     "non-owner non-super denied",
			userID:   "user-other",
			targetID: "user-owner",
			roleID:   "viewer",
			wantErr:  true,
			wantCode: connect.CodePermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := connect.NewRequest(&xylona.GrantGameServerAccessRequest{
				GameServerId: "server-local-1",
				UserId:       tt.targetID,
				RoleId:       tt.roleID,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, tt.userID)

			response, errGrant := fixture.service.GrantGameServerAccess(context.Background(), request)
			if !tt.wantErr {
				if errGrant != nil {
					t.Fatalf("GrantGameServerAccess() error = %v", errGrant)
				}
				if response == nil || response.Msg == nil || response.Msg.Grant == nil {
					t.Fatalf("GrantGameServerAccess() returned empty response")
				}
				if response.Msg.Grant.GameServerId != "server-local-1" {
					t.Errorf("GrantGameServerAccess().Grant.GameServerId = %q, want %q", response.Msg.Grant.GameServerId, "server-local-1")
				}
				if response.Msg.Grant.RoleId != tt.roleID {
					t.Errorf("GrantGameServerAccess().Grant.RoleId = %q, want %q", response.Msg.Grant.RoleId, tt.roleID)
				}
				if response.Msg.Grant.UserId != tt.targetID {
					t.Errorf("GrantGameServerAccess().Grant.UserId = %q, want %q", response.Msg.Grant.UserId, tt.targetID)
				}
				if response.Msg.Grant.UserName == "" || response.Msg.Grant.RoleName == "" || response.Msg.Grant.GrantedByUserName == "" {
					t.Errorf("GrantGameServerAccess() response missing expected display fields: %+v", response.Msg.Grant)
				}
				return
			}

			if errGrant == nil {
				t.Fatalf("GrantGameServerAccess() expected error with code %v, got nil", tt.wantCode)
			}
			if connect.CodeOf(errGrant) != tt.wantCode {
				t.Errorf("GrantGameServerAccess() code = %v, want %v", connect.CodeOf(errGrant), tt.wantCode)
			}
		})
	}
}

func TestRevokeGameServerAccessAuthorization(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	createRequest := connect.NewRequest(&xylona.GrantGameServerAccessRequest{
		GameServerId: "server-local-1",
		UserId:       "user-other",
		RoleId:       "viewer",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createRequest, "user-owner")

	createResponse, errCreate := fixture.service.GrantGameServerAccess(context.Background(), createRequest)
	if errCreate != nil {
		t.Fatalf("GrantGameServerAccess() setup error = %v", errCreate)
	}
	grantID := createResponse.Msg.Grant.Id

	denyRequest := connect.NewRequest(&xylona.RevokeGameServerAccessRequest{
		GrantId:      grantID,
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, denyRequest, "user-other")

	_, errDeny := fixture.service.RevokeGameServerAccess(context.Background(), denyRequest)
	if connect.CodeOf(errDeny) != connect.CodePermissionDenied {
		t.Fatalf("RevokeGameServerAccess(non-owner) code = %v, want %v", connect.CodeOf(errDeny), connect.CodePermissionDenied)
	}

	allowRequest := connect.NewRequest(&xylona.RevokeGameServerAccessRequest{
		GrantId:      grantID,
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, allowRequest, "user-owner")

	_, errAllow := fixture.service.RevokeGameServerAccess(context.Background(), allowRequest)
	if errAllow != nil {
		t.Fatalf("RevokeGameServerAccess(owner) error = %v", errAllow)
	}

	_, errGetAssignment := fixture.conn.GetUserRoleAssignmentByID(grantID)
	if !errors.Is(errGetAssignment, sql.ErrNoRows) {
		t.Errorf("GetUserRoleAssignmentByID() error = %v, want %v", errGetAssignment, sql.ErrNoRows)
	}
}

func TestGrantFederatedAccessAuthorizationAndShape(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote-1")

	tests := []struct {
		name     string
		userID   string
		roleID   string
		wantErr  bool
		wantCode connect.Code
	}{
		{
			name:    "owner can grant federated access",
			userID:  "user-owner",
			roleID:  "viewer",
			wantErr: false,
		},
		{
			name:    "super user can grant federated access",
			userID:  "user-admin",
			roleID:  "operator",
			wantErr: false,
		},
		{
			name:     "non-owner non-super denied",
			userID:   "user-other",
			roleID:   "admin",
			wantErr:  true,
			wantCode: connect.CodePermissionDenied,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := connect.NewRequest(&xylona.GrantFederatedAccessRequest{
				GameServerId:   "server-local-1",
				RemoteNodeId:   "node-remote-1",
				RemoteUserId:   "remote-user-" + string(rune('a'+i)),
				RemoteUserName: "Remote User",
				RoleId:         tt.roleID,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, tt.userID)

			response, errGrant := fixture.service.GrantFederatedAccess(context.Background(), request)
			if !tt.wantErr {
				if errGrant != nil {
					t.Fatalf("GrantFederatedAccess() error = %v", errGrant)
				}
				if response == nil || response.Msg == nil || response.Msg.Grant == nil {
					t.Fatalf("GrantFederatedAccess() returned empty response")
				}
				if response.Msg.Grant.GameServerId != "server-local-1" {
					t.Errorf("GrantFederatedAccess().Grant.GameServerId = %q, want %q", response.Msg.Grant.GameServerId, "server-local-1")
				}
				if response.Msg.Grant.RemoteNodeId != "node-remote-1" {
					t.Errorf("GrantFederatedAccess().Grant.RemoteNodeId = %q, want %q", response.Msg.Grant.RemoteNodeId, "node-remote-1")
				}
				if response.Msg.Grant.RoleId != tt.roleID {
					t.Errorf("GrantFederatedAccess().Grant.RoleId = %q, want %q", response.Msg.Grant.RoleId, tt.roleID)
				}
				if response.Msg.Grant.RoleName == "" || response.Msg.Grant.GrantedByUserName == "" {
					t.Errorf("GrantFederatedAccess() response missing expected display fields: %+v", response.Msg.Grant)
				}
				return
			}

			if errGrant == nil {
				t.Fatalf("GrantFederatedAccess() expected error with code %v, got nil", tt.wantCode)
			}
			if connect.CodeOf(errGrant) != tt.wantCode {
				t.Errorf("GrantFederatedAccess() code = %v, want %v", connect.CodeOf(errGrant), tt.wantCode)
			}
		})
	}
}

func TestListRemoteNodeUsersRequiresSearchForNonSuper(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	requestUnauthenticated := connect.NewRequest(&xylona.ListRemoteNodeUsersRequest{NodeId: "node-remote-1"})
	_, errUnauthenticated := fixture.service.ListRemoteNodeUsers(context.Background(), requestUnauthenticated)
	if connect.CodeOf(errUnauthenticated) != connect.CodeUnauthenticated {
		t.Fatalf("ListRemoteNodeUsers(unauthenticated) code = %v, want %v", connect.CodeOf(errUnauthenticated), connect.CodeUnauthenticated)
	}

	// Non-super user without search term should get InvalidArgument.
	requestNoSearch := connect.NewRequest(&xylona.ListRemoteNodeUsersRequest{NodeId: "node-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, requestNoSearch, "user-owner")

	_, errNoSearch := fixture.service.ListRemoteNodeUsers(context.Background(), requestNoSearch)
	if connect.CodeOf(errNoSearch) != connect.CodeInvalidArgument {
		t.Fatalf("ListRemoteNodeUsers(non-super, no search) code = %v, want %v", connect.CodeOf(errNoSearch), connect.CodeInvalidArgument)
	}

	// Non-super user with search term should be allowed (will fail at node lookup, which is expected).
	requestWithSearch := connect.NewRequest(&xylona.ListRemoteNodeUsersRequest{NodeId: "node-remote-1", Search: "someuser"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, requestWithSearch, "user-owner")

	_, errWithSearch := fixture.service.ListRemoteNodeUsers(context.Background(), requestWithSearch)
	// Should fail with NotFound (node doesn't exist in test fixture), not PermissionDenied.
	if connect.CodeOf(errWithSearch) != connect.CodeNotFound {
		t.Fatalf("ListRemoteNodeUsers(non-super, with search) code = %v, want %v", connect.CodeOf(errWithSearch), connect.CodeNotFound)
	}
}
