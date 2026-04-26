package actions

import (
	"context"
	"testing"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestLoadStartLaunchEnvMetadataMergesGameDefaults(t *testing.T) {
	inst := newTestInstance(t)
	_, errGame := inst.db.SQLDb.ExecContext(
		context.Background(),
		"update game set default_env_vars = ? where id = ?",
		`[{"name":"DEFAULT_ONLY","value":"base"},{"name":"OVERRIDE_ME","value":"base"}]`,
		"minecraft",
	)
	if errGame != nil {
		t.Fatalf("update game setup error = %v", errGame)
	}
	_, errServer := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)
		 on conflict(id) do nothing`,
		"node-local", "Local Node", "http://localhost:8080", true,
	)
	if errServer != nil {
		t.Fatalf("insert node setup error = %v", errServer)
	}
	_, errIP := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)
		 on conflict(address, node_id) do nothing`,
		"127.0.0.1", true, false, "node-local",
	)
	if errIP != nil {
		t.Fatalf("insert ip setup error = %v", errIP)
	}
	_, errUser := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 on conflict(id) do nothing`,
		"user-env", "owner", "owner@example.com", "Owner", "User", "hash", false,
	)
	if errUser != nil {
		t.Fatalf("insert user setup error = %v", errUser)
	}
	_, errInsertServer := inst.db.InsertGameServer(inst.db.DB, &models.GameServerSetter{
		ID:               omit.From("server-launch-env"),
		UserID:           omit.From("user-env"),
		Name:             omit.From("Launch Env Server"),
		GameID:           omit.From("minecraft"),
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
		EnvVars:          omit.From(`[{"name":"OVERRIDE_ME","value":"server"}]`),
	})
	if errInsertServer != nil {
		t.Fatalf("InsertGameServer() error = %v", errInsertServer)
	}

	gameServer, errGet := inst.db.GetGameServerByID("server-launch-env")
	if errGet != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGet)
	}

	normalEnv, secretStates, errLoad := inst.loadStartLaunchEnvMetadata(gameServer)
	if errLoad != nil {
		t.Fatalf("loadStartLaunchEnvMetadata() error = %v", errLoad)
	}
	if len(secretStates) != 0 {
		t.Fatalf("secretStates length = %d, want 0", len(secretStates))
	}
	if len(normalEnv) != 2 {
		t.Fatalf("normalEnv length = %d, want 2", len(normalEnv))
	}
	if normalEnv[0].Name != "DEFAULT_ONLY" {
		t.Fatalf("normalEnv[0].Name = %q, want DEFAULT_ONLY", normalEnv[0].Name)
	}
	if normalEnv[1].Value != "server" {
		t.Fatalf("normalEnv[1].Value = %q, want server", normalEnv[1].Value)
	}
}
