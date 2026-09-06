package nodeclient

import (
	"strings"
	"testing"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

func TestProcessFailureWireValidation(t *testing.T) {
	for _, test := range []struct {
		name       string
		stage      string
		omitTime   bool
		wantReport bool
	}{
		{name: "runtime", stage: diagnosis.StageRuntime, wantReport: true},
		{name: "launch discards synthetic exit", stage: diagnosis.StageLaunch, wantReport: true},
		{name: "future stage", stage: "future"},
		{name: "missing metadata", stage: diagnosis.StageRuntime, omitTime: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			exitCode := int64(1)
			wire := &nodeprotov1.ProcessFailure{
				ExecutionId: "execution", AttemptStartedAt: timestamppb.Now(), OccurredAt: timestamppb.Now(),
				Stage: test.stage, Category: "future", ExitCode: &exitCode,
				Evidence: strings.Repeat("untrusted line\n", 3000) + string([]byte{0xff}),
				Error:    strings.Repeat("界", 4096), MatchedEvidence: "first\nsecond", EvidenceAvailable: true,
			}
			if test.omitTime {
				wire.AttemptStartedAt = nil
			}
			report := processFailureFromProto(wire)
			if (report != nil) != test.wantReport {
				t.Fatalf("report = %+v", report)
			}
			if report == nil {
				return
			}
			if report.Category != diagnosis.CategoryUnknown || !report.Truncated || !utf8.ValidString(report.Evidence) ||
				len(report.Evidence) > diagnosis.MaxEvidenceBytes || len(report.Error) > diagnosis.MaxErrorBytes ||
				len(strings.Split(report.Evidence, "\n")) > diagnosis.MaxEvidenceLines || strings.Contains(report.MatchedEvidence, "\n") {
				t.Fatalf("unbounded report = %+v", report)
			}
			if (report.ExitCode != nil) != (test.stage == diagnosis.StageRuntime) {
				t.Fatalf("exit code present for stage %s = %v", test.stage, report.ExitCode)
			}
		})
	}
	if processFailureFromProto(nil) != nil {
		t.Fatal("older node missing evidence must remain absent")
	}
}
