package main

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

func processFailureToProto(failure *diagnosis.Report) *nodeprotov1.ProcessFailure {
	if failure == nil {
		return nil
	}
	report := diagnosis.Bound(*failure)
	out := &nodeprotov1.ProcessFailure{
		ExecutionId:       report.ExecutionID,
		AttemptStartedAt:  timestamppb.New(report.AttemptStartedAt),
		OccurredAt:        timestamppb.New(report.OccurredAt),
		Stage:             report.Stage,
		Category:          report.Category,
		Error:             report.Error,
		Evidence:          report.Evidence,
		MatchedEvidence:   report.MatchedEvidence,
		Truncated:         report.Truncated,
		EvidenceAvailable: report.EvidenceAvailable,
	}
	if report.ExitCode != nil {
		exitCode := int64(*report.ExitCode)
		out.ExitCode = &exitCode
	}
	return out
}
