package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

var testSessionCounter atomic.Int64

type rbacRPCFixture struct {
	conn         *db.Connection
	service      *XylonaService
	secureCookie *securecookie.SecureCookie
}

func newRBACRPCFixture(t *testing.T) *rbacRPCFixture {
	t.Helper()

	conn := newRPCFixtureConnection(t, "rbac-rpc.sqlite")

	seedRBACRPCFixture(t, conn)

	secureCookieInst := securecookie.New(
		[]byte("0123456789abcdef0123456789abcdef"),
		[]byte("0123456789abcdef"),
	)

	service := &XylonaService{
		ctx:          context.Background(),
		db:           conn,
		secureCookie: secureCookieInst,
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
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)`,
		"node-local", "Local Node", "http://localhost:8080", true,
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
		`insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)`,
		"127.0.0.1", true, false, "node-local",
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
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-local-1", "user-owner", "Local One", "minecraft", "OFFLINE",
		20, 20, "world", "127.0.0.1", 25565, 25565, "/tmp/server-local-1", "node-local", "[]",
	)
	if errServer != nil {
		t.Fatalf("failed to insert game server: %v", errServer)
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

func addSessionCookieHeaderHTTP(
	t *testing.T,
	conn *db.Connection,
	secureCookieInst *securecookie.SecureCookie,
	request *http.Request,
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

	request.Header.Set(
		"Cookie",
		gatekeeper.SessionIDCookieName+"="+sessionID+"; "+gatekeeper.SessionTokenCookieName+"="+encodedToken,
	)
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
				if response == nil || response.Msg == nil || response.Msg.GetGrant() == nil {
					t.Fatalf("GrantGameServerAccess() returned empty response")
				}
				if response.Msg.GetGrant().GetGameServerId() != "server-local-1" {
					t.Errorf("GrantGameServerAccess().Grant.GameServerId = %q, want %q", response.Msg.GetGrant().GetGameServerId(), "server-local-1")
				}
				if response.Msg.GetGrant().GetRoleId() != tt.roleID {
					t.Errorf("GrantGameServerAccess().Grant.RoleId = %q, want %q", response.Msg.GetGrant().GetRoleId(), tt.roleID)
				}
				if response.Msg.GetGrant().GetUserId() != tt.targetID {
					t.Errorf("GrantGameServerAccess().Grant.UserId = %q, want %q", response.Msg.GetGrant().GetUserId(), tt.targetID)
				}
				if response.Msg.GetGrant().GetUserName() == "" || response.Msg.GetGrant().GetRoleName() == "" || response.Msg.GetGrant().GetGrantedByUserName() == "" {
					t.Errorf("GrantGameServerAccess() response missing expected display fields: %+v", response.Msg.GetGrant())
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
	grantID := createResponse.Msg.GetGrant().GetId()

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

func TestListRoles(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Unauthenticated → CodeUnauthenticated
	unauthedReq := connect.NewRequest(&xylona.ListRolesRequest{})
	_, errUnauthed := fixture.service.ListRoles(context.Background(), unauthedReq)
	if errUnauthed == nil {
		t.Fatalf("ListRoles(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errUnauthed) != connect.CodeUnauthenticated {
		t.Errorf("ListRoles(unauthenticated) code = %v, want %v", connect.CodeOf(errUnauthed), connect.CodeUnauthenticated)
	}

	// Authenticated → returns seeded system roles
	authedReq := connect.NewRequest(&xylona.ListRolesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, authedReq, "user-owner")

	resp, errList := fixture.service.ListRoles(context.Background(), authedReq)
	if errList != nil {
		t.Fatalf("ListRoles() error = %v", errList)
	}
	if resp.Msg == nil || len(resp.Msg.GetRoles()) == 0 {
		t.Fatalf("ListRoles() returned no roles")
	}

	foundSystem := false
	for _, role := range resp.Msg.GetRoles() {
		if role.GetIsSystem() {
			foundSystem = true
			break
		}
	}
	if !foundSystem {
		t.Errorf("ListRoles() expected at least one system role")
	}
}

func TestListPermissions(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Unauthenticated → CodeUnauthenticated
	unauthedReq := connect.NewRequest(&xylona.ListPermissionsRequest{})
	_, errUnauthed := fixture.service.ListPermissions(context.Background(), unauthedReq)
	if errUnauthed == nil {
		t.Fatalf("ListPermissions(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errUnauthed) != connect.CodeUnauthenticated {
		t.Errorf("ListPermissions(unauthenticated) code = %v, want %v", connect.CodeOf(errUnauthed), connect.CodeUnauthenticated)
	}

	// Authenticated → returns seeded permissions
	authedReq := connect.NewRequest(&xylona.ListPermissionsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, authedReq, "user-owner")

	resp, errList := fixture.service.ListPermissions(context.Background(), authedReq)
	if errList != nil {
		t.Fatalf("ListPermissions() error = %v", errList)
	}
	if resp.Msg == nil || len(resp.Msg.GetPermissions()) == 0 {
		t.Fatalf("ListPermissions() returned no permissions")
	}
}

func TestCreateRole(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Non-super → CodePermissionDenied
	nonSuperReq := connect.NewRequest(&xylona.CreateRoleRequest{
		Name:        "test-role",
		Description: "A test role",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, nonSuperReq, "user-owner")

	_, errNonSuper := fixture.service.CreateRole(context.Background(), nonSuperReq)
	if errNonSuper == nil {
		t.Fatalf("CreateRole(non-super) expected error, got nil")
	}
	if connect.CodeOf(errNonSuper) != connect.CodePermissionDenied {
		t.Errorf("CreateRole(non-super) code = %v, want %v", connect.CodeOf(errNonSuper), connect.CodePermissionDenied)
	}

	// Empty name → CodeInvalidArgument
	emptyNameReq := connect.NewRequest(&xylona.CreateRoleRequest{
		Name: "",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, emptyNameReq, "user-admin")

	_, errEmptyName := fixture.service.CreateRole(context.Background(), emptyNameReq)
	if errEmptyName == nil {
		t.Fatalf("CreateRole(empty name) expected error, got nil")
	}
	if connect.CodeOf(errEmptyName) != connect.CodeInvalidArgument {
		t.Errorf("CreateRole(empty name) code = %v, want %v", connect.CodeOf(errEmptyName), connect.CodeInvalidArgument)
	}

	// Get valid permission IDs from the seeded data
	listPermReq := connect.NewRequest(&xylona.ListPermissionsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listPermReq, "user-admin")
	listPermResp, errListPerm := fixture.service.ListPermissions(context.Background(), listPermReq)
	if errListPerm != nil {
		t.Fatalf("ListPermissions() error = %v", errListPerm)
	}
	var validPermIDs []string
	if len(listPermResp.Msg.GetPermissions()) > 0 {
		validPermIDs = append(validPermIDs, listPermResp.Msg.GetPermissions()[0].GetId())
	}

	// Super user creates role with valid permissions → success
	createReq := connect.NewRequest(&xylona.CreateRoleRequest{
		Name:          "custom-role",
		Description:   "A custom role",
		PermissionIds: validPermIDs,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-admin")

	createResp, errCreate := fixture.service.CreateRole(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateRole() error = %v", errCreate)
	}
	if createResp.Msg == nil || createResp.Msg.GetRole() == nil {
		t.Fatalf("CreateRole() returned empty response")
	}
	if createResp.Msg.GetRole().GetName() != "custom-role" {
		t.Errorf("CreateRole().Role.Name = %q, want %q", createResp.Msg.GetRole().GetName(), "custom-role")
	}
	if createResp.Msg.GetRole().GetIsSystem() {
		t.Errorf("CreateRole().Role.IsSystem = true, want false")
	}

	// Duplicate name → CodeAlreadyExists
	dupReq := connect.NewRequest(&xylona.CreateRoleRequest{
		Name: "custom-role",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, dupReq, "user-admin")

	_, errDup := fixture.service.CreateRole(context.Background(), dupReq)
	if errDup == nil {
		t.Fatalf("CreateRole(duplicate) expected error, got nil")
	}
	if connect.CodeOf(errDup) != connect.CodeAlreadyExists {
		t.Errorf("CreateRole(duplicate) code = %v, want %v", connect.CodeOf(errDup), connect.CodeAlreadyExists)
	}

	// Invalid permission IDs → error
	invalidPermReq := connect.NewRequest(&xylona.CreateRoleRequest{
		Name:          "role-with-bad-perms",
		PermissionIds: []string{"nonexistent-perm-id"},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, invalidPermReq, "user-admin")

	_, errInvalidPerm := fixture.service.CreateRole(context.Background(), invalidPermReq)
	if errInvalidPerm == nil {
		t.Fatalf("CreateRole(invalid perms) expected error, got nil")
	}
}

func TestDeleteRole(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Non-super → CodePermissionDenied
	nonSuperReq := connect.NewRequest(&xylona.DeleteRoleRequest{
		RoleId: "viewer",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, nonSuperReq, "user-owner")

	_, errNonSuper := fixture.service.DeleteRole(context.Background(), nonSuperReq)
	if errNonSuper == nil {
		t.Fatalf("DeleteRole(non-super) expected error, got nil")
	}
	if connect.CodeOf(errNonSuper) != connect.CodePermissionDenied {
		t.Errorf("DeleteRole(non-super) code = %v, want %v", connect.CodeOf(errNonSuper), connect.CodePermissionDenied)
	}

	// System role → CodeFailedPrecondition
	sysRoleReq := connect.NewRequest(&xylona.DeleteRoleRequest{
		RoleId: "viewer",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, sysRoleReq, "user-admin")

	_, errSysRole := fixture.service.DeleteRole(context.Background(), sysRoleReq)
	if errSysRole == nil {
		t.Fatalf("DeleteRole(system role) expected error, got nil")
	}
	if connect.CodeOf(errSysRole) != connect.CodeFailedPrecondition {
		t.Errorf("DeleteRole(system role) code = %v, want %v", connect.CodeOf(errSysRole), connect.CodeFailedPrecondition)
	}

	// Nonexistent → CodeNotFound
	nonexistentReq := connect.NewRequest(&xylona.DeleteRoleRequest{
		RoleId: "nonexistent-role-id",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, nonexistentReq, "user-admin")

	_, errNonexistent := fixture.service.DeleteRole(context.Background(), nonexistentReq)
	if errNonexistent == nil {
		t.Fatalf("DeleteRole(nonexistent) expected error, got nil")
	}
	if connect.CodeOf(errNonexistent) != connect.CodeNotFound {
		t.Errorf("DeleteRole(nonexistent) code = %v, want %v", connect.CodeOf(errNonexistent), connect.CodeNotFound)
	}

	// Create a custom role then delete it → success
	createReq := connect.NewRequest(&xylona.CreateRoleRequest{
		Name:        "deletable-role",
		Description: "Will be deleted",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-admin")

	createResp, errCreate := fixture.service.CreateRole(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateRole() error = %v", errCreate)
	}

	deleteReq := connect.NewRequest(&xylona.DeleteRoleRequest{
		RoleId: createResp.Msg.GetRole().GetId(),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, deleteReq, "user-admin")

	_, errDelete := fixture.service.DeleteRole(context.Background(), deleteReq)
	if errDelete != nil {
		t.Fatalf("DeleteRole() error = %v", errDelete)
	}
}

func TestListGameServerAccessGrants(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Empty server ID → CodeInvalidArgument
	emptyReq := connect.NewRequest(&xylona.ListGameServerAccessGrantsRequest{
		GameServerId: "",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, emptyReq, "user-owner")

	_, errEmpty := fixture.service.ListGameServerAccessGrants(context.Background(), emptyReq)
	if errEmpty == nil {
		t.Fatalf("ListGameServerAccessGrants(empty ID) expected error, got nil")
	}
	if connect.CodeOf(errEmpty) != connect.CodeInvalidArgument {
		t.Errorf("ListGameServerAccessGrants(empty ID) code = %v, want %v", connect.CodeOf(errEmpty), connect.CodeInvalidArgument)
	}

	// Owner can list
	ownerReq := connect.NewRequest(&xylona.ListGameServerAccessGrantsRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, ownerReq, "user-owner")

	ownerResp, errOwner := fixture.service.ListGameServerAccessGrants(context.Background(), ownerReq)
	if errOwner != nil {
		t.Fatalf("ListGameServerAccessGrants(owner) error = %v", errOwner)
	}
	if ownerResp.Msg == nil {
		t.Fatalf("ListGameServerAccessGrants(owner) returned nil message")
	}

	// Super user can list
	superReq := connect.NewRequest(&xylona.ListGameServerAccessGrantsRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, superReq, "user-admin")

	_, errSuper := fixture.service.ListGameServerAccessGrants(context.Background(), superReq)
	if errSuper != nil {
		t.Fatalf("ListGameServerAccessGrants(super) error = %v", errSuper)
	}

	// Non-owner non-super → CodePermissionDenied
	otherReq := connect.NewRequest(&xylona.ListGameServerAccessGrantsRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, otherReq, "user-other")

	_, errOther := fixture.service.ListGameServerAccessGrants(context.Background(), otherReq)
	if errOther == nil {
		t.Fatalf("ListGameServerAccessGrants(non-owner) expected error, got nil")
	}
	if connect.CodeOf(errOther) != connect.CodePermissionDenied {
		t.Errorf("ListGameServerAccessGrants(non-owner) code = %v, want %v", connect.CodeOf(errOther), connect.CodePermissionDenied)
	}
}
