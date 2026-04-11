package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/db/dbtest"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

type autoRestartTestFixture struct {
	cancel     context.CancelFunc
	conn       *db.Connection
	inst       *Instance
	gameServer *models.GameServer
}

func newAutoRestartTestFixture(t *testing.T, maxRetries int64, cooldownSeconds int64) autoRestartTestFixture {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	conn := dbtest.NewMigratedConnection(t, "auto-restart-hv03.sqlite")
	supervisorInst, errNewSupervisor := supervisor.New(ctx)
	if errNewSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errNewSupervisor)
	}

	inst := &Instance{
		ctx:                ctx,
		supervisorInstance: supervisorInst,
		db:                 conn,
		restartState:       &restartStateMap{},
		versionState:       versiontracker.NewVersionStateMap(),
	}

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	userID := "user-auto-restart"
	nodeID := "node-auto-restart"
	gameID := "game-auto-restart"
	serverID := "server-auto-restart"
	serverDir := filepath.Join(t.TempDir(), "server-data")
	backupDir := filepath.Join(t.TempDir(), "backups")

	errMkdir := os.MkdirAll(serverDir, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(serverDir) error = %v", errMkdir)
	}
	errMkdir = os.MkdirAll(backupDir, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(backupDir) error = %v", errMkdir)
	}

	_, errCreateUser := conn.CreateUser(&models.UserSetter{
		ID:           omit.From(userID),
		UserName:     omit.From("auto-restart-user"),
		Email:        omit.From("auto-restart@example.com"),
		FirstName:    omit.From("Auto"),
		LastName:     omit.From("Restart"),
		PasswordHash: omit.From("hash"),
		SuperUser:    omit.From(false),
		LastLoginAt:  omit.From(now),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	})
	if errCreateUser != nil {
		t.Fatalf("CreateUser() error = %v", errCreateUser)
	}

	_, errInsertNode := conn.InsertNode(&models.NodeSetter{
		ID:      omit.From(nodeID),
		Name:    omit.From("Auto Restart Node"),
		IsLocal: omit.From(true),
		Host:    omit.From("localhost"),
		Port:    omit.From(int64(8080)),
		BaseURL: omit.From("http://localhost:8080"),
		Enabled: omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf("InsertNode() error = %v", errInsertNode)
	}

	_, errUpsertIP := conn.UpsertIP(&models.IPSetter{
		Address:            omit.From("127.0.0.1"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
	})
	if errUpsertIP != nil {
		t.Fatalf("UpsertIP() error = %v", errUpsertIP)
	}

	_, errInsertGame := conn.InsertGame(conn.DB, &models.GameSetter{
		ID:                omit.From(gameID),
		Name:              omit.From("Auto Restart Game"),
		DefaultPort:       omit.From(int64(25565)),
		DefaultQueryPort:  omit.From(int64(25565)),
		DefaultMaxPlayers: omit.From(int64(20)),
	})
	if errInsertGame != nil {
		t.Fatalf("InsertGame() error = %v", errInsertGame)
	}

	gameServer, errInsertServer := conn.InsertGameServer(conn.DB, &models.GameServerSetter{
		ID:                         omit.From(serverID),
		UserID:                     omit.From(userID),
		Name:                       omit.From("Auto Restart Server"),
		GameID:                     omit.From(gameID),
		Status:                     omit.From("OFFLINE"),
		SetPlayers:                 omit.From(int64(20)),
		MaxPlayers:                 omit.From(int64(20)),
		Map:                        omit.From("world"),
		IP:                         omit.From("127.0.0.1"),
		Port:                       omit.From(int64(25565)),
		QueryPort:                  omit.From(int64(25565)),
		Directory:                  omit.From(serverDir),
		BackupsEnabled:             omit.From(false),
		BackupDirectory:            omit.From(backupDir),
		MaxBackups:                 omit.From(int64(2)),
		NodeID:                     omit.From(nodeID),
		StartArgsPatches:           omit.From("[]"),
		AutoRestartEnabled:         omit.From(true),
		AutoRestartMaxRetries:      omit.From(maxRetries),
		AutoRestartCooldownSeconds: omit.From(cooldownSeconds),
		CreatedAt:                  omit.From(now),
		UpdatedAt:                  omit.From(now),
	})
	if errInsertServer != nil {
		t.Fatalf("InsertGameServer() error = %v", errInsertServer)
	}

	return autoRestartTestFixture{
		cancel:     cancel,
		conn:       conn,
		inst:       inst,
		gameServer: gameServer,
	}
}

func waitForConsoleOutputSubstring(t *testing.T, cmd *supervisor.Command, want string, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		buffer := cmd.GetOutputBuffer()
		if strings.Contains(buffer, want) {
			return buffer
		}
		time.Sleep(10 * time.Millisecond)
	}

	buffer := cmd.GetOutputBuffer()
	t.Fatalf("console output did not contain %q within %v; got %q", want, timeout, buffer)
	return ""
}

func TestHandleServerExitResetsRetryCounterAfterStableWindow(t *testing.T) {
	fixture := newAutoRestartTestFixture(t, 3, 0)
	defer fixture.cancel()

	cmd := fixture.inst.supervisorInstance.GetCommandByIDOrCreateShell(fixture.gameServer.ID)
	entry := fixture.inst.restartState.entry(fixture.gameServer.ID)
	entry.mu.Lock()
	entry.attemptCount = 2
	entry.lastStartTime = time.Now().Add(-(autoRestartStableWindow + time.Minute))
	entry.mu.Unlock()

	fixture.inst.handleServerExit(&supervisor.Command{}, fixture.gameServer.ID)

	buffer := waitForConsoleOutputSubstring(t, cmd, "Restarting in 1s (attempt 1/3)", time.Second)
	if !strings.Contains(buffer, "Restarting in 1s (attempt 1/3)") {
		t.Fatalf("console output = %q, want stable-window restart message", buffer)
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.attemptCount != 1 {
		t.Fatalf("retry count = %d, want %d", entry.attemptCount, 1)
	}
}

func TestHandleServerExitStopsAtRetryLimit(t *testing.T) {
	fixture := newAutoRestartTestFixture(t, 2, 0)
	defer fixture.cancel()

	cmd := fixture.inst.supervisorInstance.GetCommandByIDOrCreateShell(fixture.gameServer.ID)
	entry := fixture.inst.restartState.entry(fixture.gameServer.ID)
	entry.mu.Lock()
	entry.attemptCount = 2
	entry.lastStartTime = time.Now()
	entry.mu.Unlock()

	fixture.inst.handleServerExit(&supervisor.Command{}, fixture.gameServer.ID)

	buffer := waitForConsoleOutputSubstring(
		t,
		cmd,
		"Auto-restart limit reached (2/2). Server will not be restarted automatically.",
		time.Second,
	)
	if !strings.Contains(buffer, "Auto-restart limit reached (2/2). Server will not be restarted automatically.") {
		t.Fatalf("console output = %q, want retry limit exhaustion message", buffer)
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.attemptCount != 2 {
		t.Fatalf("retry count = %d, want %d", entry.attemptCount, 2)
	}
}

func TestHandleServerExitCancelsRestartWhenDisabledDuringCooldown(t *testing.T) {
	fixture := newAutoRestartTestFixture(t, 3, 0)
	defer fixture.cancel()

	cmd := fixture.inst.supervisorInstance.GetCommandByIDOrCreateShell(fixture.gameServer.ID)
	entry := fixture.inst.restartState.entry(fixture.gameServer.ID)
	entry.mu.Lock()
	entry.attemptCount = 0
	entry.lastStartTime = time.Now()
	entry.mu.Unlock()

	fixture.inst.handleServerExit(&supervisor.Command{}, fixture.gameServer.ID)

	_, errUpdate := fixture.conn.SQLDb.ExecContext(
		fixture.inst.ctx,
		`update game_server set auto_restart_enabled = 0 where id = ?`,
		fixture.gameServer.ID,
	)
	if errUpdate != nil {
		t.Fatalf("disable auto restart update error = %v", errUpdate)
	}

	buffer := waitForConsoleOutputSubstring(
		t,
		cmd,
		"Auto-restart was disabled during cooldown. Restart cancelled.",
		2*time.Second,
	)
	if strings.Contains(buffer, "Auto-restart: starting server") {
		t.Fatalf("console output = %q, want restart to stay cancelled", buffer)
	}
}
