package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/db/dbtest"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestValidateStartupRuntimeSecurityIgnoresRemoteServers(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home", "clinton"))
	t.Setenv("USER", "clinton")
	t.Setenv("USERPROFILE", filepath.Join(t.TempDir(), "Users", "clinton"))

	conn := dbtest.NewMigratedConnection(t, "runtime-security-startup.sqlite")
	seedRuntimeSecurityTestFixture(t, conn)

	pathsRoot := t.TempDir()
	localServerDir := filepath.Join(pathsRoot, "local", "server")
	localBackupDir := filepath.Join(pathsRoot, "local", "backups")
	remoteServerDir := filepath.Join(pathsRoot, "remote", "server")
	remoteBackupDir := filepath.Join(pathsRoot, "remote", "backups")

	mustMkdirAllRuntimeSecurityTest(t, localServerDir)
	mustMkdirAllRuntimeSecurityTest(t, localBackupDir)
	mustMkdirAllRuntimeSecurityTest(t, remoteServerDir)
	mustMkdirAllRuntimeSecurityTest(t, remoteBackupDir)

	config := Configuration{
		DBFilePath: filepath.Join(remoteServerDir, "data.sqlite"),
	}

	insertRuntimeSecurityTestGameServer(t, conn, "server-local", "node-local", localServerDir, localBackupDir)
	insertRuntimeSecurityTestGameServer(t, conn, "server-remote", "node-remote", remoteServerDir, remoteBackupDir)

	errValidate := validateStartupRuntimeSecurity(config, conn)
	if errValidate != nil {
		t.Fatalf("validateStartupRuntimeSecurity() error = %v, want nil", errValidate)
	}
}

func seedRuntimeSecurityTestFixture(t *testing.T, conn *db.Connection) {
	t.Helper()

	_, errLocalNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)`,
		"node-local", "Local Node", "http://localhost:8080", true,
	)
	if errLocalNode != nil {
		t.Fatalf("failed to insert local node: %v", errLocalNode)
	}

	_, errRemoteNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)`,
		"node-remote", "Remote Node", "http://remotehost:8080", true,
	)
	if errRemoteNode != nil {
		t.Fatalf("failed to insert remote node: %v", errRemoteNode)
	}

	errSettings := conn.UpdateLocalSettings(&models.LocalSetting{
		ID:     1,
		NodeID: "node-local",
	})
	if errSettings != nil {
		t.Fatalf("failed to insert local settings: %v", errSettings)
	}

	_, errIP := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into ip (address, usable, external) values (?, ?, ?)`,
		"127.0.0.1",
		true,
		false,
	)
	if errIP != nil {
		t.Fatalf("failed to insert ip: %v", errIP)
	}

	now := time.Now().UTC()
	_, errUser := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-owner", "owner", "owner@example.com", "Owner", "User", "hash", false, now, now, now,
	)
	if errUser != nil {
		t.Fatalf("failed to insert user: %v", errUser)
	}
}

func insertRuntimeSecurityTestGameServer(
	t *testing.T,
	conn *db.Connection,
	serverID string,
	nodeID string,
	directory string,
	backupDirectory string,
) {
	t.Helper()

	_, errServer := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, backup_directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		serverID,
		"user-owner",
		serverID,
		"minecraft",
		"OFFLINE",
		20,
		20,
		"world",
		"127.0.0.1",
		25565,
		25565,
		directory,
		backupDirectory,
		nodeID,
		"[]",
	)
	if errServer != nil {
		t.Fatalf("failed to insert game server %q: %v", serverID, errServer)
	}
}

func mustMkdirAllRuntimeSecurityTest(t *testing.T, path string) {
	t.Helper()

	errMkdirAll := os.MkdirAll(path, 0o750)
	if errMkdirAll != nil {
		t.Fatalf("mkdirAll(%q) error = %v", path, errMkdirAll)
	}
}
