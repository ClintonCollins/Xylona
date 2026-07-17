package actions

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
)

func (inst *Instance) startMetricsEventRecorder(ctx context.Context) <-chan struct{} {
	return inst.startMetricsEventRecorderWithBus(ctx, eventbus.Get())
}

func (inst *Instance) startMetricsEventRecorderWithBus(ctx context.Context, bus *eventbus.EventBus) <-chan struct{} {
	done := make(chan struct{})
	if inst == nil || inst.db == nil || bus == nil {
		close(done)
		return done
	}
	if ctx == nil {
		ctx = inst.actionContext()
	}

	statusEvents := bus.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	go func() {
		defer close(done)
		defer bus.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusEvents)

		for {
			select {
			case <-ctx.Done():
				return
			case rawEvent, open := <-statusEvents:
				if !open {
					return
				}
				event, valid := rawEvent.(eventbus.StatusChangedEvent)
				if !valid {
					log.Warn().Msg("metrics event recorder received unexpected lifecycle event type")
					continue
				}
				inst.recordLifecycleStatusEvent(ctx, event)
			}
		}
	}()

	return done
}

func (inst *Instance) recordLifecycleStatusEvent(ctx context.Context, event eventbus.StatusChangedEvent) {
	if inst == nil || inst.db == nil {
		return
	}

	executionID := strings.TrimSpace(event.ExecutionID)
	if executionID == "" {
		// Legacy runtime events do not carry replay-safe correlation metadata.
		// Give each observed transition a durable identity instead of collapsing
		// unrelated legacy transitions onto the table's unique key.
		executionID = "legacy-" + uuid.NewString()
	}

	var exitCode *int
	if event.ExitCodeKnown {
		exitCodeValue := event.ExitCode
		exitCode = &exitCodeValue
	}
	observedAt := event.OccurredAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	_, errInsert := inst.db.InsertGameServerLifecycleEvent(ctx, db.InsertGameServerLifecycleEventParams{
		GameServerID:       event.ServerID,
		NodeID:             event.ServerNodeID,
		ExecutionID:        executionID,
		TransitionSequence: event.TransitionSequence,
		PreviousStatus:     event.OldStatus,
		Status:             event.NewStatus,
		IntentionalStop:    event.IntentionalStop,
		ExitCode:           exitCode,
		ObservedAt:         observedAt,
	})
	if errInsert != nil {
		if ctx.Err() != nil {
			return
		}
		log.Error().
			Err(errInsert).
			Str("game_server_id", event.ServerID).
			Str("node_id", event.ServerNodeID).
			Msg("Failed to record game server lifecycle event")
	}
}

func (inst *Instance) recordGameServerOperation(
	gameServerID string,
	operation db.GameServerOperation,
	phase db.GameServerOperationPhase,
	outcome db.GameServerOperationOutcome,
	startedAt time.Time,
	completedAt time.Time,
	bytesProcessed *int64,
	source db.GameServerOperationSource,
) {
	if inst == nil || inst.db == nil {
		return
	}

	durationMS := completedAt.Sub(startedAt).Milliseconds()
	actionCtx := inst.actionContext()
	if actionCtx.Err() != nil {
		return
	}
	writeCtx, cancelWrite := context.WithTimeout(context.WithoutCancel(actionCtx), 5*time.Second)
	defer cancelWrite()
	errInsert := inst.db.InsertGameServerOperationEvent(writeCtx, db.InsertGameServerOperationEventParams{
		GameServerID:   gameServerID,
		Operation:      operation,
		Phase:          phase,
		Outcome:        outcome,
		StartedAt:      startedAt,
		CompletedAt:    &completedAt,
		DurationMS:     &durationMS,
		BytesProcessed: bytesProcessed,
		Source:         source,
	})
	if errInsert != nil {
		if actionCtx.Err() != nil {
			return
		}
		log.Error().
			Err(errInsert).
			Str("game_server_id", gameServerID).
			Str("operation", string(operation)).
			Str("phase", string(phase)).
			Str("outcome", string(outcome)).
			Msg("Failed to record game server operation event")
	}
}
