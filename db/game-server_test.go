package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func seedGameServerFixture(t *testing.T, conn *Connection) {
	t.Helper()
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	// Insert a second game server for list/filter tests.
	_, errServer := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, start_command, status, set_players, max_players, map, ip, port, query_port, directory, node_id)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-local-2", "user-other", "Local Two", "minecraft", "java -jar server.jar", "OFFLINE",
		10, 10, "world", "127.0.0.1", 25566, 25566, "/tmp/server-local-2", "node-local",
	)
	if errServer != nil {
		t.Fatalf("failed to insert second game server: %v", errServer)
	}
}

func TestGetAllGameServers(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-all.sqlite")
	seedGameServerFixture(t, conn)

	servers, errGet := conn.GetAllGameServers()
	if errGet != nil {
		t.Fatalf("GetAllGameServers() error = %v", errGet)
	}
	if len(servers) != 2 {
		t.Errorf("GetAllGameServers() len = %d, want 2", len(servers))
	}
}

func TestGetGameServerByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-by-id.sqlite")
	seedGameServerFixture(t, conn)

	server, errGet := conn.GetGameServerByID("server-local-1")
	if errGet != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGet)
	}
	if server.Name != "Local One" {
		t.Errorf("GetGameServerByID().Name = %q, want %q", server.Name, "Local One")
	}
	if server.UserID != "user-owner" {
		t.Errorf("GetGameServerByID().UserID = %q, want %q", server.UserID, "user-owner")
	}
}

func TestGetGameServerByIDNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-not-found.sqlite")
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	_, errGet := conn.GetGameServerByID("nonexistent")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetGameServerByID() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetGameServersByUser(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-by-user.sqlite")
	seedGameServerFixture(t, conn)

	servers, errGet := conn.GetGameServersByUser("user-owner")
	if errGet != nil {
		t.Fatalf("GetGameServersByUser() error = %v", errGet)
	}
	if len(servers) != 1 {
		t.Fatalf("GetGameServersByUser() len = %d, want 1", len(servers))
	}
	if servers[0].ID != "server-local-1" {
		t.Errorf("GetGameServersByUser()[0].ID = %q, want %q", servers[0].ID, "server-local-1")
	}
}

func TestGetGameServersByUserNoResults(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-by-user-empty.sqlite")
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	servers, errGet := conn.GetGameServersByUser("user-admin")
	if errGet != nil {
		t.Fatalf("GetGameServersByUser() error = %v", errGet)
	}
	if len(servers) != 0 {
		t.Errorf("GetGameServersByUser() len = %d, want 0", len(servers))
	}
}

func TestGetGameServersByIP(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-by-ip.sqlite")
	seedGameServerFixture(t, conn)

	servers, errGet := conn.GetGameServersByIP("127.0.0.1")
	if errGet != nil {
		t.Fatalf("GetGameServersByIP() error = %v", errGet)
	}
	if len(servers) != 2 {
		t.Errorf("GetGameServersByIP() len = %d, want 2", len(servers))
	}
}

func TestGetGameServersByIPNoResults(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-by-ip-empty.sqlite")
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	servers, errGet := conn.GetGameServersByIP("10.0.0.1")
	if errGet != nil {
		t.Fatalf("GetGameServersByIP() error = %v", errGet)
	}
	if len(servers) != 0 {
		t.Errorf("GetGameServersByIP() len = %d, want 0", len(servers))
	}
}

func TestGetGameServersByGameID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-by-game.sqlite")
	seedGameServerFixture(t, conn)

	servers, errGet := conn.GetGameServersByGameID("minecraft")
	if errGet != nil {
		t.Fatalf("GetGameServersByGameID() error = %v", errGet)
	}
	if len(servers) != 2 {
		t.Errorf("GetGameServersByGameID() len = %d, want 2", len(servers))
	}
}

func TestGetGameServersByGameIDNoResults(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-by-game-empty.sqlite")
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	servers, errGet := conn.GetGameServersByGameID("nonexistent-game")
	if errGet != nil {
		t.Fatalf("GetGameServersByGameID() error = %v", errGet)
	}
	if len(servers) != 0 {
		t.Errorf("GetGameServersByGameID() len = %d, want 0", len(servers))
	}
}

func TestInsertGameServer(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-insert.sqlite")
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	now := time.Now().UTC()
	setter := &models.GameServerSetter{
		ID:           omit.From("server-new"),
		UserID:       omit.From("user-owner"),
		Name:         omit.From("New Server"),
		GameID:       omit.From("minecraft"),
		StartCommand: omit.From("java -jar server.jar"),
		Status:       omit.From("OFFLINE"),
		SetPlayers:   omit.From(int64(20)),
		MaxPlayers:   omit.From(int64(20)),
		Map:          omit.From("world"),
		IP:           omit.From("127.0.0.1"),
		Port:         omit.From(int64(25567)),
		QueryPort:    omit.From(int64(25567)),
		Directory:    omit.From("/tmp/server-new"),
		NodeID:       omit.From("node-local"),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	}

	server, errInsert := conn.InsertGameServer(conn.DB, setter)
	if errInsert != nil {
		t.Fatalf("InsertGameServer() error = %v", errInsert)
	}
	if server.ID != "server-new" {
		t.Errorf("InsertGameServer().ID = %q, want %q", server.ID, "server-new")
	}
	if server.Name != "New Server" {
		t.Errorf("InsertGameServer().Name = %q, want %q", server.Name, "New Server")
	}
}

func TestInsertGameServerDuplicateUserName(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-dup-name.sqlite")
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	now := time.Now().UTC()
	setter := &models.GameServerSetter{
		ID:           omit.From("server-dup"),
		UserID:       omit.From("user-owner"),
		Name:         omit.From("Local One"), // same name as seedRBACFixture's server
		GameID:       omit.From("minecraft"),
		StartCommand: omit.From("java -jar server.jar"),
		Status:       omit.From("OFFLINE"),
		SetPlayers:   omit.From(int64(20)),
		MaxPlayers:   omit.From(int64(20)),
		IP:           omit.From("127.0.0.1"),
		Port:         omit.From(int64(25568)),
		QueryPort:    omit.From(int64(25568)),
		Directory:    omit.From("/tmp/server-dup"),
		NodeID:       omit.From("node-local"),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	}

	_, errInsert := conn.InsertGameServer(conn.DB, setter)
	if errInsert == nil {
		t.Fatalf("InsertGameServer(duplicate user+name) expected error, got nil")
	}
}

func TestUpdateGameServer(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-update.sqlite")
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	now := time.Now().UTC()
	updateSetter := &models.GameServerSetter{
		ID:           omit.From("server-local-1"),
		Name:         omit.From("Updated Name"),
		UserID:       omit.From("user-owner"),
		GameID:       omit.From("minecraft"),
		StartCommand: omit.From("java -jar server.jar"),
		Status:       omit.From("OFFLINE"),
		SetPlayers:   omit.From(int64(30)),
		MaxPlayers:   omit.From(int64(30)),
		Map:          omit.From("nether"),
		IP:           omit.From("127.0.0.1"),
		Port:         omit.From(int64(25565)),
		QueryPort:    omit.From(int64(25565)),
		Directory:    omit.From("/tmp/server-local-1"),
		NodeID:       omit.From("node-local"),
		UpdatedAt:    omit.From(now),
	}

	updated, errUpdate := conn.UpdateGameServer(conn.DB, updateSetter)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}
	if updated.Name != "Updated Name" {
		t.Errorf("UpdateGameServer().Name = %q, want %q", updated.Name, "Updated Name")
	}
	if updated.MaxPlayers != 30 {
		t.Errorf("UpdateGameServer().MaxPlayers = %d, want 30", updated.MaxPlayers)
	}
	if updated.Map != "nether" {
		t.Errorf("UpdateGameServer().Map = %q, want %q", updated.Map, "nether")
	}
}

func TestDeleteGameServer(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-delete.sqlite")
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	errDelete := conn.DeleteGameServer("server-local-1")
	if errDelete != nil {
		t.Fatalf("DeleteGameServer() error = %v", errDelete)
	}

	_, errGet := conn.GetGameServerByID("server-local-1")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetGameServerByID() after delete error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetGameServersAccessibleByUser(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-accessible.sqlite")
	addMissingGameColumns(t, conn)
	seedRBACFixture(t, conn)

	// seedRBACFixture creates:
	//   user-owner  — owns server-local-1
	//   user-other  — no servers, no role assignments
	//   user-admin  — superuser, no servers

	// Step 1: grantee (user-other) has no ownership and no role assignments.
	servers, errGet := conn.GetGameServersAccessibleByUser("user-other")
	if errGet != nil {
		t.Fatalf("GetGameServersAccessibleByUser(grantee, before grant) error = %v", errGet)
	}
	if len(servers) != 0 {
		t.Errorf("GetGameServersAccessibleByUser(grantee, before grant) len = %d, want 0", len(servers))
	}

	// Step 2: create a role assignment granting user-other access to server-local-1.
	errAssign := conn.CreateUserRoleAssignment(
		"assignment-accessible-1",
		"user-other",
		"operator",
		"server-local-1",
		"user-owner",
	)
	if errAssign != nil {
		t.Fatalf("CreateUserRoleAssignment() error = %v", errAssign)
	}

	// Step 3: grantee now sees the granted server.
	servers, errGet = conn.GetGameServersAccessibleByUser("user-other")
	if errGet != nil {
		t.Fatalf("GetGameServersAccessibleByUser(grantee, after grant) error = %v", errGet)
	}
	if len(servers) != 1 {
		t.Fatalf("GetGameServersAccessibleByUser(grantee, after grant) len = %d, want 1", len(servers))
	}
	if servers[0].ID != "server-local-1" {
		t.Errorf("GetGameServersAccessibleByUser(grantee)[0].ID = %q, want %q", servers[0].ID, "server-local-1")
	}

	// Step 4: owner sees the server via ownership.
	servers, errGet = conn.GetGameServersAccessibleByUser("user-owner")
	if errGet != nil {
		t.Fatalf("GetGameServersAccessibleByUser(owner) error = %v", errGet)
	}
	if len(servers) != 1 {
		t.Fatalf("GetGameServersAccessibleByUser(owner) len = %d, want 1", len(servers))
	}
	if servers[0].ID != "server-local-1" {
		t.Errorf("GetGameServersAccessibleByUser(owner)[0].ID = %q, want %q", servers[0].ID, "server-local-1")
	}

	// Step 5: owner also gets a role assignment — should NOT produce duplicates.
	errOwnerAssign := conn.CreateUserRoleAssignment(
		"assignment-accessible-2",
		"user-owner",
		"operator",
		"server-local-1",
		"user-owner",
	)
	if errOwnerAssign != nil {
		t.Fatalf("CreateUserRoleAssignment(owner) error = %v", errOwnerAssign)
	}

	servers, errGet = conn.GetGameServersAccessibleByUser("user-owner")
	if errGet != nil {
		t.Fatalf("GetGameServersAccessibleByUser(owner, with role) error = %v", errGet)
	}
	if len(servers) != 1 {
		t.Errorf("GetGameServersAccessibleByUser(owner, with role) len = %d, want 1 (no duplicates)", len(servers))
	}

	// Step 6: user with no servers and no assignments returns empty.
	servers, errGet = conn.GetGameServersAccessibleByUser("user-admin")
	if errGet != nil {
		t.Fatalf("GetGameServersAccessibleByUser(admin, no servers) error = %v", errGet)
	}
	if len(servers) != 0 {
		t.Errorf("GetGameServersAccessibleByUser(admin, no servers) len = %d, want 0", len(servers))
	}

	// Step 7: nonexistent user returns empty, no error.
	servers, errGet = conn.GetGameServersAccessibleByUser("user-nonexistent")
	if errGet != nil {
		t.Fatalf("GetGameServersAccessibleByUser(nonexistent) error = %v", errGet)
	}
	if len(servers) != 0 {
		t.Errorf("GetGameServersAccessibleByUser(nonexistent) len = %d, want 0", len(servers))
	}
}
