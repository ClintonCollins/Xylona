package actions

import (
	"context"
	"strings"
	"testing"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestAddNormalEnvironmentPlaceholdersPreservesBuiltIns(t *testing.T) {
	variables := []launchenv.Variable{
		{Name: "steam_username", Value: "owner"},
		{Name: "PORT", Value: "malicious-override"},
	}
	placeholders := map[string]string{"PORT": "7777"}

	addNormalEnvironmentPlaceholders(variables, placeholders)

	if placeholders["STEAM_USERNAME"] != "owner" {
		t.Fatalf("STEAM_USERNAME placeholder = %q, want owner", placeholders["STEAM_USERNAME"])
	}
	if placeholders["PORT"] != "7777" {
		t.Fatalf("PORT placeholder = %q, want built-in value", placeholders["PORT"])
	}
}

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

func TestSteamGSLTIsResolvedOnlyAtLaunch(t *testing.T) {
	inst := newTestInstance(t)
	inst.db.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	_, errNode := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)
		 on conflict(id) do nothing`,
		"node-gslt", "GSLT Node", "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("insert node setup error = %v", errNode)
	}
	_, errIP := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)
		 on conflict(address, node_id) do nothing`,
		"127.0.0.1", true, false, "node-gslt",
	)
	if errIP != nil {
		t.Fatalf("insert IP setup error = %v", errIP)
	}
	_, errUser := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 on conflict(id) do nothing`,
		"user-gslt", "gslt-owner", "gslt@example.com", "GSLT", "Owner", "hash", false,
	)
	if errUser != nil {
		t.Fatalf("insert user setup error = %v", errUser)
	}
	_, errInsertServer := inst.db.InsertGameServer(inst.db.DB, &models.GameServerSetter{
		ID:               omit.From("server-gslt"),
		UserID:           omit.From("user-gslt"),
		Name:             omit.From("GSLT Server"),
		GameID:           omit.From("counter_strike_2"),
		Status:           omit.From("OFFLINE"),
		SetPlayers:       omit.From(int64(12)),
		MaxPlayers:       omit.From(int64(12)),
		Map:              omit.From("de_dust2"),
		IP:               omit.From("127.0.0.1"),
		Port:             omit.From(int64(27015)),
		QueryPort:        omit.From(int64(27015)),
		Directory:        omit.From(t.TempDir()),
		NodeID:           omit.From("node-gslt"),
		StartArgsPatches: omit.From("[]"),
	})
	if errInsertServer != nil {
		t.Fatalf("InsertGameServer() error = %v", errInsertServer)
	}

	gameServer, errGet := inst.db.GetGameServerByID("server-gslt")
	if errGet != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGet)
	}
	_, errMissing := inst.secretStartPlaceholderVars(gameServer)
	if errMissing == nil {
		t.Fatal("secretStartPlaceholderVars() error = nil, want missing-token error")
	}

	const token = "FAKE-GSLT-TOKEN-FOR-TEST" // #nosec G101 -- test-only value, not a credential.
	errSet := readiness.SetSteamGSLT(inst.db, gameServer.ID, token, gameServer.UserID)
	if errSet != nil {
		t.Fatalf("SetSteamGSLT() error = %v", errSet)
	}
	secretVars, errVars := inst.secretStartPlaceholderVars(gameServer)
	if errVars != nil {
		t.Fatalf("secretStartPlaceholderVars() error = %v", errVars)
	}
	if secretVars[steamGSLTPlaceholder] != token {
		t.Fatalf("secretStartPlaceholderVars()[%q] = %q, want test token", steamGSLTPlaceholder, secretVars[steamGSLTPlaceholder])
	}

	_, args, errResolve := inst.resolveStructuredStartCommandWithVars(gameServer, secretVars)
	if errResolve != nil {
		t.Fatalf("resolveStructuredStartCommandWithVars() error = %v", errResolve)
	}
	resolvedArgs := strings.Join(args, " ")
	if !strings.Contains(resolvedArgs, token) {
		t.Fatalf("resolved args do not contain the launch-only GSLT: %q", resolvedArgs)
	}
	if strings.Contains(resolvedArgs, "{{STEAM_GSLT}}") {
		t.Fatalf("resolved args still contain the GSLT placeholder: %q", resolvedArgs)
	}
}
