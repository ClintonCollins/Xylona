package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
)

func TestMetricsEventRecorderPersistsLifecycleAndStopsWithContext(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "metrics-event-recorder.sqlite")
	seedMetricsEventServer(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	inst := &Instance{ctx: ctx, db: conn}
	bus := eventbus.Get()
	done := inst.startMetricsEventRecorderWithBus(ctx, bus)

	start := time.Now().UTC().Add(-time.Second)
	occurredAt := start.Add(100 * time.Millisecond)
	bus.Publish(eventbus.TopicGameServerStatusChanged, eventbus.StatusChangedEvent{
		ServerID:           "metrics-server",
		ServerNodeID:       "metrics-node",
		OldStatus:          "ONLINE",
		NewStatus:          "OFFLINE",
		ExecutionID:        "execution-1",
		TransitionSequence: 4,
		IntentionalStop:    true,
		ExitCode:           0,
		ExitCodeKnown:      true,
		OccurredAt:         occurredAt,
	})

	events := waitForLifecycleEvents(t, conn, start, 1)
	if len(events) != 1 {
		t.Fatalf("lifecycle event count = %d, want 1", len(events))
	}
	if !events[0].IntentionalStop {
		t.Fatal("lifecycle event intentional stop = false, want true")
	}
	if events[0].ExitCode == nil || *events[0].ExitCode != 0 {
		t.Fatalf("lifecycle event exit code = %v, want 0", events[0].ExitCode)
	}
	if !events[0].ObservedAt.Equal(occurredAt) {
		t.Fatalf("lifecycle observed time = %v, want %v", events[0].ObservedAt, occurredAt)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("metrics event recorder did not stop after context cancellation")
	}

	bus.Publish(eventbus.TopicGameServerStatusChanged, eventbus.StatusChangedEvent{
		ServerID:           "metrics-server",
		ServerNodeID:       "metrics-node",
		OldStatus:          "OFFLINE",
		NewStatus:          "STARTING",
		ExecutionID:        "execution-2",
		TransitionSequence: 1,
	})
	time.Sleep(25 * time.Millisecond)
	afterCancel, errGet := conn.GetGameServerLifecycleEvents(context.Background(), "metrics-server", start, time.Now().UTC().Add(time.Second))
	if errGet != nil {
		t.Fatalf("GetGameServerLifecycleEvents() after cancel error = %v", errGet)
	}
	if len(afterCancel) != 1 {
		t.Fatalf("lifecycle event count after cancel = %d, want 1", len(afterCancel))
	}
}

func TestRecordGameServerOperationPersistsBoundedResult(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "metrics-operation-recorder.sqlite")
	seedMetricsEventServer(t, conn)
	inst := &Instance{ctx: context.Background(), db: conn}
	startedAt := time.Date(2026, time.July, 17, 15, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(2500 * time.Millisecond)
	bytesProcessed := int64(4096)

	inst.recordGameServerOperation(
		"metrics-server",
		db.GameServerOperationBackup,
		db.GameServerOperationPhaseArchiving,
		db.GameServerOperationOutcomeSucceeded,
		startedAt,
		completedAt,
		&bytesProcessed,
		db.GameServerOperationSourceManual,
	)

	events, errGet := conn.GetGameServerOperationEvents(context.Background(), "metrics-server", startedAt.Add(-time.Second), completedAt.Add(time.Second))
	if errGet != nil {
		t.Fatalf("GetGameServerOperationEvents() error = %v", errGet)
	}
	if len(events) != 1 {
		t.Fatalf("operation event count = %d, want 1", len(events))
	}
	if events[0].DurationMS == nil || *events[0].DurationMS != 2500 {
		t.Fatalf("operation duration = %v, want 2500", events[0].DurationMS)
	}
	if events[0].BytesProcessed == nil || *events[0].BytesProcessed != bytesProcessed {
		t.Fatalf("operation bytes = %v, want %d", events[0].BytesProcessed, bytesProcessed)
	}
}

func TestScheduledBackupRecordsOperationOutcome(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	errWrite := os.WriteFile(filepath.Join(fixture.gameServer.Directory, "state.txt"), []byte("current-state"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(state.txt) error = %v", errWrite)
	}

	startedAt := time.Now().UTC().Add(-time.Second)
	backup, errCreate := inst.CreateScheduledBackup(fixture.gameServer)
	if errCreate != nil {
		t.Fatalf("CreateScheduledBackup() error = %v", errCreate)
	}
	events, errGet := inst.db.GetGameServerOperationEvents(context.Background(), fixture.gameServer.ID, startedAt, time.Now().UTC().Add(time.Second))
	if errGet != nil {
		t.Fatalf("GetGameServerOperationEvents() error = %v", errGet)
	}
	if len(events) != 1 {
		t.Fatalf("backup operation event count = %d, want 1", len(events))
	}
	event := events[0]
	if event.Operation != db.GameServerOperationBackup ||
		event.Phase != db.GameServerOperationPhaseArchiving ||
		event.Outcome != db.GameServerOperationOutcomeSucceeded ||
		event.Source != db.GameServerOperationSourceScheduled {
		t.Fatalf("backup operation event = (%q, %q, %q, %q), want successful scheduled archive", event.Operation, event.Phase, event.Outcome, event.Source)
	}
	if event.BytesProcessed == nil || *event.BytesProcessed != backup.SizeBytes {
		t.Fatalf("backup operation bytes = %v, want %d", event.BytesProcessed, backup.SizeBytes)
	}
}

func TestUpdateFailureRecordsOperationPhase(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	inst.embeddedNodeClient = nil
	startedAt := time.Now().UTC().Add(-time.Second)

	inst.runUpdateWithBackup(fixture.gameServer, nil)

	events, errGet := inst.db.GetGameServerOperationEvents(context.Background(), fixture.gameServer.ID, startedAt, time.Now().UTC().Add(time.Second))
	if errGet != nil {
		t.Fatalf("GetGameServerOperationEvents() error = %v", errGet)
	}
	if len(events) != 1 {
		t.Fatalf("update operation event count = %d, want 1", len(events))
	}
	event := events[0]
	if event.Operation != db.GameServerOperationUpdate ||
		event.Phase != db.GameServerOperationPhasePreparing ||
		event.Outcome != db.GameServerOperationOutcomeFailed ||
		event.Source != db.GameServerOperationSourceController {
		t.Fatalf("update operation event = (%q, %q, %q, %q), want failed controller preparation", event.Operation, event.Phase, event.Outcome, event.Source)
	}
}

func seedMetricsEventServer(t *testing.T, conn *db.Connection) {
	t.Helper()
	now := time.Now().UTC()
	_, errNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)`,
		"metrics-node",
		"Metrics Node",
		"http://localhost:8080",
		true,
	)
	if errNode != nil {
		t.Fatalf("insert metrics node: %v", errNode)
	}
	_, errIP := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)`,
		"127.0.0.1",
		true,
		false,
		"metrics-node",
	)
	if errIP != nil {
		t.Fatalf("insert metrics IP: %v", errIP)
	}
	_, errUser := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"metrics-user",
		"metrics-user",
		"metrics@example.com",
		"Metrics",
		"User",
		"hash",
		false,
		now,
		now,
		now,
	)
	if errUser != nil {
		t.Fatalf("insert metrics user: %v", errUser)
	}
	_, errGame := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game (id, name, default_port, default_query_port, default_max_players, windows_support)
		 values (?, ?, ?, ?, ?, ?)`,
		"metrics-game",
		"Metrics Game",
		25565,
		25565,
		20,
		true,
	)
	if errGame != nil {
		t.Fatalf("insert metrics game: %v", errGame)
	}
	_, errServer := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"metrics-server",
		"metrics-user",
		"Metrics Server",
		"metrics-game",
		"OFFLINE",
		20,
		20,
		"world",
		"127.0.0.1",
		25565,
		25565,
		"/tmp/metrics-server",
		"metrics-node",
		"[]",
	)
	if errServer != nil {
		t.Fatalf("insert metrics game server: %v", errServer)
	}
}

func waitForLifecycleEvents(t *testing.T, conn *db.Connection, start time.Time, want int) []db.GameServerLifecycleEvent {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		events, errGet := conn.GetGameServerLifecycleEvents(context.Background(), "metrics-server", start, time.Now().UTC().Add(time.Second))
		if errGet != nil {
			t.Fatalf("GetGameServerLifecycleEvents() error = %v", errGet)
		}
		if len(events) >= want {
			return events
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d lifecycle events", want)
	return nil
}
