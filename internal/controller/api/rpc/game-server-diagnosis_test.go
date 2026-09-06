package rpc

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGetGameServerDiagnosisPermissions(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name         string
		userID       string
		permissions  []string
		wantCode     connect.Code
		wantEvidence bool
	}{
		{name: "unauthenticated", wantCode: connect.CodeUnauthenticated},
		{name: "no view permission", userID: "user-other", wantCode: connect.CodePermissionDenied},
		{name: "console alone cannot view", userID: "user-other", permissions: []string{permissionConsole}, wantCode: connect.CodePermissionDenied},
		{name: "view metadata only", userID: "user-other", permissions: []string{"game_server.view"}},
		{name: "view and console", userID: "user-other", permissions: []string{"game_server.view", permissionConsole}, wantEvidence: true},
		{name: "owner", userID: "user-owner", wantEvidence: true},
		{name: "admin", userID: "user-admin", wantEvidence: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := newRBACRPCFixture(t)
			if len(tt.permissions) > 0 {
				assignScheduledTaskRole(t, fixture, tt.userID, "diagnosis-reader", tt.permissions...)
			}
			report := diagnosis.Report{
				ExecutionID: "attempt-1", AttemptStartedAt: time.Now().UTC(), OccurredAt: time.Now().UTC(),
				Stage: diagnosis.StageRuntime, Category: diagnosis.CategoryPortInUse,
				Error: "private error detail", Evidence: "private console excerpt", MatchedEvidence: "private matching line",
				EvidenceAvailable: true, ExitCode: new(1),
			}
			errSave := fixture.conn.SaveGameServerDiagnosis(t.Context(), "server-local-1", "node-local", report)
			if errSave != nil {
				t.Fatal(errSave)
			}
			request := connect.NewRequest(&xylona.GetGameServerDiagnosisRequest{ServerId: "server-local-1"})
			if tt.userID != "" {
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, tt.userID)
			}
			response, errGet := fixture.service.GetGameServerDiagnosis(t.Context(), request)
			if tt.wantCode != 0 {
				if connect.CodeOf(errGet) != tt.wantCode {
					t.Fatalf("RPC error = %v, want %v", errGet, tt.wantCode)
				}
				return
			}
			if errGet != nil {
				t.Fatal(errGet)
			}
			got := response.Msg.GetDiagnosis()
			if got.GetStage() != report.Stage || got.GetCategory() != report.Category || got.GetExitCode() != 1 || !got.GetEvidenceAvailable() || !got.GetInferred() {
				t.Fatalf("missing generic report metadata: %v", got)
			}
			if tt.wantEvidence {
				if got.GetEvidenceRestricted() || got.GetError() != report.Error || got.GetEvidence() != report.Evidence || got.GetMatchedEvidence() != report.MatchedEvidence {
					t.Fatalf("authorized evidence was removed: %v", got)
				}
			} else if !got.GetEvidenceRestricted() || got.GetError() != "" || got.GetEvidence() != "" || got.GetMatchedEvidence() != "" {
				t.Fatalf("restricted response exposed evidence: %v", got)
			}
		})
	}
}

func TestGetGameServerDiagnosisNoReport(t *testing.T) {
	t.Parallel()
	fixture := newRBACRPCFixture(t)
	request := connect.NewRequest(&xylona.GetGameServerDiagnosisRequest{ServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	response, errGet := fixture.service.GetGameServerDiagnosis(t.Context(), request)
	if errGet != nil {
		t.Fatal(errGet)
	}
	if response.Msg.GetDiagnosis() != nil {
		t.Fatalf("expected no report, got %v", response.Msg.GetDiagnosis())
	}
}
