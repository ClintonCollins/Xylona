package actions

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (inst *Instance) recordStartDiagnosis(server *models.GameServer, executionID string, startedAt time.Time, stage string, err error, secrets []string) {
	report := diagnosis.Capture(err, "", secrets...)
	report.EvidenceAvailable = false
	report.Stage = stage
	var launchFailure *node.StartFailureError
	switch {
	case errors.As(err, &launchFailure):
		report = launchFailure.Report
		redacted := diagnosis.Capture(errors.New(report.Error), report.Evidence, secrets...)
		report.Error = redacted.Error
		report.Evidence = redacted.Evidence
		report.MatchedEvidence = redacted.MatchedEvidence
		report.Truncated = report.Truncated || redacted.Truncated
	case stage == diagnosis.StageLaunch && uncertainStartOutcome(err):
		report.Stage = diagnosis.StageUnknownOutcome
		report.Category = diagnosis.CategoryNodeUnavailable
	case IsStartUnavailableError(err):
		report.Category = diagnosis.CategoryNodeUnavailable
	case IsStartConfigurationError(err) && report.Category == diagnosis.CategoryUnknown:
		report.Category = diagnosis.CategoryIncompleteSetup
	}
	report.ExecutionID = executionID
	report.AttemptStartedAt = startedAt
	if report.OccurredAt.IsZero() {
		report.OccurredAt = time.Now().UTC()
	}
	inst.saveGameServerDiagnosis(server.ID, server.NodeID, report)
}

func uncertainStartOutcome(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	switch connect.CodeOf(err) {
	case connect.CodeCanceled, connect.CodeDeadlineExceeded, connect.CodeUnavailable:
		return true
	default:
		return false
	}
}

func (inst *Instance) recordLifecycleDiagnosis(event eventbus.StatusChangedEvent) {
	if event.IntentionalStop || !strings.EqualFold(event.NewStatus, "OFFLINE") || !strings.EqualFold(event.OldStatus, "ONLINE") {
		return
	}
	if event.Failure != nil {
		inst.saveGameServerDiagnosis(event.ServerID, event.ServerNodeID, *event.Failure)
		return
	}
	// Older nodes supply lifecycle metadata without a captured console tail.
	// Never read their current console to reconstruct a previous execution.
	if !event.ExitCodeKnown || event.ExitCode == 0 {
		return
	}
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	executionID := strings.TrimSpace(event.ExecutionID)
	if executionID == "" {
		// Keep unrelated legacy crashes distinct so occurrence time orders them.
		executionID = "legacy-" + uuid.NewString()
	}
	report := diagnosis.Report{
		ExecutionID: executionID,
		OccurredAt:  occurredAt,
		Stage:       diagnosis.StageUnknown,
		Category:    diagnosis.CategoryUnknown,
	}
	// Legacy nodes also use a synthetic exit code for launch errors. Without
	// captured failure metadata neither the stage nor the process exit code is known.
	inst.saveGameServerDiagnosis(event.ServerID, event.ServerNodeID, report)
}

func (inst *Instance) saveGameServerDiagnosis(serverID, nodeID string, report diagnosis.Report) {
	if inst == nil || inst.db == nil {
		return
	}
	writeCtx, cancelWrite := context.WithTimeout(context.WithoutCancel(inst.actionContext()), 5*time.Second)
	defer cancelWrite()
	errSave := inst.db.SaveGameServerDiagnosis(writeCtx, serverID, nodeID, report)
	if errSave != nil {
		log.Error().Err(errSave).Str("game_server_id", serverID).Str("node_id", nodeID).
			Msg("Failed to record game server diagnosis")
	}
}
