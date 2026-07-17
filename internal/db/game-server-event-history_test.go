package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestGameServerLifecycleEventHistory(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-server-lifecycle-events.sqlite")
	seedRBACFixture(t, conn)

	ctx := context.Background()
	baseTime := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	exitCode := 7
	events := []InsertGameServerLifecycleEventParams{
		{
			ID:                 "event-2",
			GameServerID:       "server-local-1",
			NodeID:             "node-local",
			ExecutionID:        "execution-1",
			TransitionSequence: 2,
			PreviousStatus:     "ONLINE",
			Status:             "OFFLINE",
			IntentionalStop:    false,
			ExitCode:           &exitCode,
			ObservedAt:         baseTime.Add(time.Minute),
		},
		{
			ID:                 "event-1",
			GameServerID:       "server-local-1",
			NodeID:             "node-local",
			ExecutionID:        "execution-1",
			TransitionSequence: 1,
			PreviousStatus:     "STARTING",
			Status:             "ONLINE",
			IntentionalStop:    false,
			ObservedAt:         baseTime,
		},
	}

	for _, event := range events {
		inserted, errInsert := conn.InsertGameServerLifecycleEvent(ctx, event)
		if errInsert != nil {
			t.Fatalf("InsertGameServerLifecycleEvent() error = %v", errInsert)
		}
		if !inserted {
			t.Fatalf("InsertGameServerLifecycleEvent() inserted = false for %s", event.ID)
		}
	}

	duplicate := events[0]
	duplicate.ID = "event-2-replay"
	inserted, errDuplicate := conn.InsertGameServerLifecycleEvent(ctx, duplicate)
	if errDuplicate != nil {
		t.Fatalf("InsertGameServerLifecycleEvent() duplicate error = %v", errDuplicate)
	}
	if inserted {
		t.Fatal("InsertGameServerLifecycleEvent() duplicate inserted = true, want false")
	}

	got, errGet := conn.GetGameServerLifecycleEvents(ctx, "server-local-1", baseTime.Add(-time.Minute), baseTime.Add(2*time.Minute))
	if errGet != nil {
		t.Fatalf("GetGameServerLifecycleEvents() error = %v", errGet)
	}
	if len(got) != 2 {
		t.Fatalf("GetGameServerLifecycleEvents() length = %d, want 2", len(got))
	}
	if got[0].TransitionSequence != 1 || got[1].TransitionSequence != 2 {
		t.Fatalf("GetGameServerLifecycleEvents() sequences = [%d, %d], want [1, 2]", got[0].TransitionSequence, got[1].TransitionSequence)
	}
	if got[0].ExitCode != nil {
		t.Fatalf("GetGameServerLifecycleEvents() first exit code = %v, want nil", got[0].ExitCode)
	}
	if got[1].ExitCode == nil || *got[1].ExitCode != exitCode {
		t.Fatalf("GetGameServerLifecycleEvents() second exit code = %v, want %d", got[1].ExitCode, exitCode)
	}
}

func TestGameServerOperationEventHistory(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-server-operation-events.sqlite")
	seedRBACFixture(t, conn)

	ctx := context.Background()
	baseTime := time.Date(2026, time.July, 17, 13, 0, 0, 0, time.UTC)
	completedAt := baseTime.Add(5 * time.Second)
	durationMS := int64(5000)
	bytesProcessed := int64(2048)
	events := []InsertGameServerOperationEventParams{
		{
			ID:           "operation-2",
			GameServerID: "server-local-1",
			Operation:    GameServerOperationUpdate,
			Phase:        GameServerOperationPhaseUpdating,
			Outcome:      GameServerOperationOutcomeFailed,
			StartedAt:    baseTime.Add(time.Minute),
			Source:       GameServerOperationSourceController,
		},
		{
			ID:             "operation-1",
			GameServerID:   "server-local-1",
			Operation:      GameServerOperationBackup,
			Phase:          GameServerOperationPhaseArchiving,
			Outcome:        GameServerOperationOutcomeSucceeded,
			StartedAt:      baseTime,
			CompletedAt:    &completedAt,
			DurationMS:     &durationMS,
			BytesProcessed: &bytesProcessed,
			Source:         GameServerOperationSourceManual,
		},
	}

	for _, event := range events {
		errInsert := conn.InsertGameServerOperationEvent(ctx, event)
		if errInsert != nil {
			t.Fatalf("InsertGameServerOperationEvent() error = %v", errInsert)
		}
	}

	got, errGet := conn.GetGameServerOperationEvents(ctx, "server-local-1", baseTime.Add(-time.Minute), baseTime.Add(2*time.Minute))
	if errGet != nil {
		t.Fatalf("GetGameServerOperationEvents() error = %v", errGet)
	}
	if len(got) != 2 {
		t.Fatalf("GetGameServerOperationEvents() length = %d, want 2", len(got))
	}
	if got[0].ID != "operation-1" || got[1].ID != "operation-2" {
		t.Fatalf("GetGameServerOperationEvents() IDs = [%s, %s], want [operation-1, operation-2]", got[0].ID, got[1].ID)
	}
	if got[0].DurationMS == nil || *got[0].DurationMS != durationMS {
		t.Fatalf("GetGameServerOperationEvents() duration = %v, want %d", got[0].DurationMS, durationMS)
	}
	if got[0].BytesProcessed == nil || *got[0].BytesProcessed != bytesProcessed {
		t.Fatalf("GetGameServerOperationEvents() bytes = %v, want %d", got[0].BytesProcessed, bytesProcessed)
	}
	if got[1].CompletedAt != nil || got[1].DurationMS != nil || got[1].BytesProcessed != nil {
		t.Fatalf("GetGameServerOperationEvents() nullable fields = (%v, %v, %v), want nil", got[1].CompletedAt, got[1].DurationMS, got[1].BytesProcessed)
	}

	overlapping, errOverlap := conn.GetGameServerOperationEvents(ctx, "server-local-1", baseTime.Add(2*time.Second), baseTime.Add(10*time.Second))
	if errOverlap != nil {
		t.Fatalf("GetGameServerOperationEvents() overlap error = %v", errOverlap)
	}
	if len(overlapping) != 1 || overlapping[0].ID != "operation-1" {
		t.Fatalf("GetGameServerOperationEvents() overlap IDs = %+v, want [operation-1]", overlapping)
	}
}

func TestGameServerEventHistoryRetention(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-server-event-retention.sqlite")
	seedRBACFixture(t, conn)

	ctx := context.Background()
	now := time.Date(2026, time.July, 17, 15, 0, 0, 0, time.UTC)
	cutoff := now.Add(-time.Hour)
	for index, observedAt := range []time.Time{now.Add(-2 * time.Hour), now} {
		_, errInsert := conn.InsertGameServerLifecycleEvent(ctx, InsertGameServerLifecycleEventParams{
			ID:                 fmt.Sprintf("lifecycle-%d", index),
			GameServerID:       "server-local-1",
			NodeID:             "node-local",
			ExecutionID:        fmt.Sprintf("execution-%d", index),
			TransitionSequence: 1,
			PreviousStatus:     "STARTING",
			Status:             "ONLINE",
			ObservedAt:         observedAt,
		})
		if errInsert != nil {
			t.Fatalf("InsertGameServerLifecycleEvent() error = %v", errInsert)
		}
		completedAt := observedAt.Add(time.Minute)
		durationMS := int64(time.Minute / time.Millisecond)
		errOperation := conn.InsertGameServerOperationEvent(ctx, InsertGameServerOperationEventParams{
			ID:           fmt.Sprintf("operation-%d", index),
			GameServerID: "server-local-1",
			Operation:    GameServerOperationBackup,
			Phase:        GameServerOperationPhaseArchiving,
			Outcome:      GameServerOperationOutcomeSucceeded,
			StartedAt:    observedAt,
			CompletedAt:  &completedAt,
			DurationMS:   &durationMS,
			Source:       GameServerOperationSourceManual,
		})
		if errOperation != nil {
			t.Fatalf("InsertGameServerOperationEvent() error = %v", errOperation)
		}
	}

	deletedLifecycle, errDeleteLifecycle := conn.DeleteGameServerLifecycleEventsOlderThan(cutoff)
	if errDeleteLifecycle != nil || deletedLifecycle != 1 {
		t.Fatalf("DeleteGameServerLifecycleEventsOlderThan() = (%d, %v), want (1, nil)", deletedLifecycle, errDeleteLifecycle)
	}
	deletedOperations, errDeleteOperations := conn.DeleteGameServerOperationEventsOlderThan(cutoff)
	if errDeleteOperations != nil || deletedOperations != 1 {
		t.Fatalf("DeleteGameServerOperationEventsOlderThan() = (%d, %v), want (1, nil)", deletedOperations, errDeleteOperations)
	}

	lifecycle, errLifecycle := conn.GetGameServerLifecycleEvents(ctx, "server-local-1", now.Add(-3*time.Hour), now.Add(time.Hour))
	if errLifecycle != nil || len(lifecycle) != 1 || lifecycle[0].ID != "lifecycle-1" {
		t.Fatalf("retained lifecycle events = (%+v, %v), want lifecycle-1", lifecycle, errLifecycle)
	}
	operations, errOperations := conn.GetGameServerOperationEvents(ctx, "server-local-1", now.Add(-3*time.Hour), now.Add(time.Hour))
	if errOperations != nil || len(operations) != 1 || operations[0].ID != "operation-1" {
		t.Fatalf("retained operation events = (%+v, %v), want operation-1", operations, errOperations)
	}
}

func TestInsertGameServerOperationEventValidation(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-server-operation-event-validation.sqlite")
	now := time.Date(2026, time.July, 17, 14, 0, 0, 0, time.UTC)
	valid := InsertGameServerOperationEventParams{
		GameServerID: "server-local-1",
		Operation:    GameServerOperationBackup,
		Phase:        GameServerOperationPhaseArchiving,
		Outcome:      GameServerOperationOutcomeSucceeded,
		StartedAt:    now,
		Source:       GameServerOperationSourceManual,
	}

	tests := []struct {
		name   string
		mutate func(*InsertGameServerOperationEventParams)
	}{
		{
			name: "operation",
			mutate: func(params *InsertGameServerOperationEventParams) {
				params.Operation = "delete"
			},
		},
		{
			name: "phase",
			mutate: func(params *InsertGameServerOperationEventParams) {
				params.Phase = "unknown"
			},
		},
		{
			name: "outcome",
			mutate: func(params *InsertGameServerOperationEventParams) {
				params.Outcome = "unknown"
			},
		},
		{
			name: "source",
			mutate: func(params *InsertGameServerOperationEventParams) {
				params.Source = "unknown"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := valid
			test.mutate(&params)
			errInsert := conn.InsertGameServerOperationEvent(context.Background(), params)
			if errInsert == nil {
				t.Fatal("InsertGameServerOperationEvent() error = nil, want validation error")
			}
		})
	}
}
