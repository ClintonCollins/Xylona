package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestStartGameServerPreservesServerNodeID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	conn := dbtest.NewMigratedConnection(t, "start-game-server-node-id.sqlite")
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	inst := NewInstance(
		ctx,
		conn,
		newSupervisorBackedNodeClient(ctx, t, supervisorInst, conn),
		nil,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	_, errNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)
		 on conflict(id) do nothing`,
		"node-local", "Local Node", "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("insert node: %v", errNode)
	}

	_, errIP := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)
		 on conflict(address, node_id) do nothing`,
		"127.0.0.1", true, false, "node-local",
	)
	if errIP != nil {
		t.Fatalf("insert ip: %v", errIP)
	}

	_, errUser := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 on conflict(id) do nothing`,
		"user-start-node-id", "owner", "owner@example.com", "Owner", "User", "hash", false,
	)
	if errUser != nil {
		t.Fatalf("insert user: %v", errUser)
	}

	_, errGame := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game
		 (id, name, default_port, default_query_port, default_max_players, windows_support,
		  linux_base_command, linux_start_args_template, windows_base_command, windows_start_args_template)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 on conflict(id) do nothing`,
		"game-start-node-id", "Start Node ID Game", 25565, 25565, 20, true,
		"sh", `[{"id":"exit","order":1,"ownership":"editable","tokens":["-c","exit 0"]}]`,
		"cmd", `[{"id":"exit","order":1,"ownership":"editable","tokens":["/c","exit 0"]}]`,
	)
	if errGame != nil {
		t.Fatalf("insert game: %v", errGame)
	}

	_, errInsertServer := conn.InsertGameServer(conn.DB, &models.GameServerSetter{
		ID:               omit.From("server-start-node-id"),
		UserID:           omit.From("user-start-node-id"),
		Name:             omit.From("Start Node ID Server"),
		GameID:           omit.From("game-start-node-id"),
		Status:           omit.From("OFFLINE"),
		SetPlayers:       omit.From(int64(20)),
		MaxPlayers:       omit.From(int64(20)),
		Map:              omit.From("world"),
		IP:               omit.From("127.0.0.1"),
		Port:             omit.From(int64(25565)),
		QueryPort:        omit.From(int64(25565)),
		Directory:        omit.From(t.TempDir()),
		NodeID:           omit.From("node-local"),
		StartArgsPatches: omit.From("[]"),
	})
	if errInsertServer != nil {
		t.Fatalf("InsertGameServer() error = %v", errInsertServer)
	}

	gameServer, errGetServer := conn.GetGameServerByID("server-start-node-id")
	if errGetServer != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGetServer)
	}

	_, errStart := inst.StartGameServer(gameServer)
	if errStart != nil {
		t.Fatalf("StartGameServer() error = %v", errStart)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cmd, errGetCommand := supervisorInst.GetCommandByID(gameServer.ID)
		if errGetCommand != nil {
			if errors.Is(errGetCommand, supervisor.ErrCommandDoesNotExist) {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			t.Fatalf("GetCommandByID() error = %v", errGetCommand)
		}

		if cmd.NodeID() == gameServer.NodeID {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	cmd, errGetCommand := supervisorInst.GetCommandByID(gameServer.ID)
	if errGetCommand != nil {
		t.Fatalf("GetCommandByID() after start error = %v", errGetCommand)
	}
	t.Fatalf("command node ID = %q, want %q", cmd.NodeID(), gameServer.NodeID)
}

func TestResolveStructuredStartCommand_BackfillsLegacyMinecraftExecutable(t *testing.T) {
	inst := newTestInstance(t)

	serverDir := t.TempDir()
	createVersionTestMinecraftJar(t, serverDir, "paper-1.21.4-100.jar", "1.21.4")

	_, errNode := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)
		 on conflict(id) do nothing`,
		"node-local", "Local", "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("insert node: %v", errNode)
	}

	_, errIP := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)
		 on conflict(address, node_id) do nothing`,
		"127.0.0.1", true, false, "node-local",
	)
	if errIP != nil {
		t.Fatalf("insert ip: %v", errIP)
	}

	_, errUser := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 on conflict(id) do nothing`,
		"user-1", "owner", "owner@example.com", "Owner", "User", "hash", false,
	)
	if errUser != nil {
		t.Fatalf("insert user: %v", errUser)
	}

	_, errInsert := inst.db.InsertGameServer(inst.db.DB, &models.GameServerSetter{
		ID:               omit.From("server-1"),
		UserID:           omit.From("user-1"),
		Name:             omit.From("Legacy Minecraft"),
		GameID:           omit.From("minecraft"),
		StartArgsPatches: omit.From("[]"),
		Status:           omit.From("OFFLINE"),
		SetPlayers:       omit.From(int64(20)),
		MaxPlayers:       omit.From(int64(20)),
		Map:              omit.From("world"),
		IP:               omit.From("127.0.0.1"),
		Port:             omit.From(int64(25565)),
		QueryPort:        omit.From(int64(25565)),
		Directory:        omit.From(serverDir),
		NodeID:           omit.From("node-local"),
		ServerSoftware:   omitnull.From("paper"),
		Version:          omit.From("1.21.4"),
	})
	if errInsert != nil {
		t.Fatalf("insert server: %v", errInsert)
	}

	gameServer, errGet := inst.db.GetGameServerByID("server-1")
	if errGet != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGet)
	}

	baseCommand, args, errResolve := inst.resolveStructuredStartCommand(gameServer)
	if errResolve != nil {
		t.Fatalf("resolveStructuredStartCommand() error = %v", errResolve)
	}

	if baseCommand != "java" {
		t.Fatalf("base command = %q, want %q", baseCommand, "java")
	}

	resolvedCommand := strings.Join(args, " ")
	if !strings.Contains(resolvedCommand, "-jar paper-1.21.4-100.jar") {
		t.Fatalf("resolved args = %q, want jar token with discovered executable", resolvedCommand)
	}

	updated, errUpdated := inst.db.GetGameServerByID("server-1")
	if errUpdated != nil {
		t.Fatalf("GetGameServerByID() after resolve error = %v", errUpdated)
	}

	if updated.ServerExecutable.GetOr("") != "paper-1.21.4-100.jar" {
		t.Fatalf("server executable = %q, want %q", updated.ServerExecutable.GetOr(""), "paper-1.21.4-100.jar")
	}
}
