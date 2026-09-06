package nodeclient

import (
	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

func processFailureFromProto(failure *nodeprotov1.ProcessFailure) *diagnosis.Report {
	if failure == nil || failure.GetExecutionId() == "" || len(failure.GetExecutionId()) > 128 ||
		failure.GetAttemptStartedAt() == nil || failure.GetOccurredAt() == nil {
		return nil
	}
	errAttempt := failure.GetAttemptStartedAt().CheckValid()
	errOccurred := failure.GetOccurredAt().CheckValid()
	if errAttempt != nil || errOccurred != nil {
		return nil
	}
	switch failure.GetStage() {
	case diagnosis.StagePreStart, diagnosis.StageLaunch, diagnosis.StageRuntime, diagnosis.StageUnknownOutcome:
	default:
		return nil
	}
	category := failure.GetCategory()
	switch category {
	case diagnosis.CategoryUnknown, diagnosis.CategoryMissingExecutable, diagnosis.CategoryPermissionDenied,
		diagnosis.CategoryPortInUse, diagnosis.CategoryDiskFull, diagnosis.CategoryIncompleteSetup, diagnosis.CategoryNodeUnavailable:
	default:
		category = diagnosis.CategoryUnknown
	}
	report := diagnosis.Bound(diagnosis.Report{
		ExecutionID:       failure.GetExecutionId(),
		AttemptStartedAt:  failure.GetAttemptStartedAt().AsTime(),
		OccurredAt:        failure.GetOccurredAt().AsTime(),
		Stage:             failure.GetStage(),
		Category:          category,
		Error:             failure.GetError(),
		Evidence:          failure.GetEvidence(),
		MatchedEvidence:   failure.GetMatchedEvidence(),
		Truncated:         failure.GetTruncated(),
		EvidenceAvailable: failure.GetEvidenceAvailable(),
	})
	if failure.ExitCode != nil && report.Stage == diagnosis.StageRuntime {
		exitCode := int(failure.GetExitCode())
		if int64(exitCode) == failure.GetExitCode() {
			report.ExitCode = &exitCode
		}
	}
	return &report
}
