package actions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestStartGameServerRecordsPreStartDiagnosis(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "start-diagnosis.sqlite")
	seedMetricsEventServer(t, conn)
	inst := &Instance{ctx: t.Context(), db: conn}
	server, errServer := conn.GetGameServerByID("metrics-server")
	if errServer != nil {
		t.Fatal(errServer)
	}
	_, errStart := inst.StartGameServer(server)
	if !IsStartUnavailableError(errStart) {
		t.Fatalf("StartGameServer error = %v, want unavailable node", errStart)
	}
	report, errReport := conn.GetGameServerDiagnosis(t.Context(), server.ID)
	if errReport != nil {
		t.Fatal(errReport)
	}
	if report.ExecutionID == "" || report.AttemptStartedAt.IsZero() || report.Stage != diagnosis.StagePreStart || report.Category != diagnosis.CategoryNodeUnavailable {
		t.Fatalf("pre-start report = %+v", report)
	}
	// A failed report write must preserve the actionable start error.
	_, errDrop := conn.SQLDb.ExecContext(t.Context(), "drop table game_server_diagnosis")
	if errDrop != nil {
		t.Fatal(errDrop)
	}
	_, errStart = inst.StartGameServer(server)
	if !IsStartUnavailableError(errStart) {
		t.Fatalf("recording failure replaced start error: %v", errStart)
	}
}

func TestStartDiagnosisDistinguishesConfirmedAndUnknown(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		stage    string
		want     string
		category string
	}{
		{"dispatch timeout", startInternalError("start", context.DeadlineExceeded), diagnosis.StageLaunch, diagnosis.StageUnknownOutcome, diagnosis.CategoryNodeUnavailable},
		{"transport unavailable", startInternalError("start", connect.NewError(connect.CodeUnavailable, errors.New("disconnected"))), diagnosis.StageLaunch, diagnosis.StageUnknownOutcome, diagnosis.CategoryNodeUnavailable},
		{"preflight unavailable", startUnavailableError("node unavailable", context.DeadlineExceeded), diagnosis.StagePreStart, diagnosis.StagePreStart, diagnosis.CategoryNodeUnavailable},
		{"incomplete setup", startConfigurationError("setup incomplete", nil), diagnosis.StagePreStart, diagnosis.StagePreStart, diagnosis.CategoryIncompleteSetup},
		{"confirmed launch", startInternalError("start", &node.StartFailureError{Err: errors.New("launch rejected"), Report: diagnosis.Report{Stage: diagnosis.StageLaunch, Category: diagnosis.CategoryPermissionDenied, Error: "secret-token denied"}}), diagnosis.StageLaunch, diagnosis.StageLaunch, diagnosis.CategoryPermissionDenied},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			conn := dbtest.NewMigratedConnection(t, "start-outcome.sqlite")
			seedMetricsEventServer(t, conn)
			inst := &Instance{ctx: t.Context(), db: conn}
			server := &models.GameServer{ID: "metrics-server", NodeID: "metrics-node"}
			inst.recordStartDiagnosis(server, "attempt", time.Now(), tt.stage, tt.err, []string{"secret-token"})
			report, errRead := conn.GetGameServerDiagnosis(t.Context(), server.ID)
			if errRead != nil {
				t.Fatal(errRead)
			}
			if report.Stage != tt.want || report.Category != tt.category || strings.Contains(report.Error, "secret-token") {
				t.Fatalf("report = %+v", report)
			}
		})
	}
}

func TestLifecycleDiagnosisPersistsOnlyGameServerFailures(t *testing.T) {
	cases := []struct {
		name        string
		oldStatus   string
		newStatus   string
		intentional bool
		failure     bool
		code        int
		wantReport  bool
	}{
		{"crash", "ONLINE", "OFFLINE", false, true, 42, true},
		{"intentional stop", "ONLINE", "OFFLINE", true, true, 42, false},
		{"installer", "INSTALLING", "OFFLINE", false, true, 42, false},
		{"updater", "UPDATING", "OFFLINE", false, true, 42, false},
		{"clean exit", "ONLINE", "OFFLINE", false, false, 0, false},
		{"legacy crash", "ONLINE", "OFFLINE", false, false, 42, true},
		{"started", "OFFLINE", "ONLINE", false, false, 0, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			conn := dbtest.NewMigratedConnection(t, "lifecycle-diagnosis.sqlite")
			seedMetricsEventServer(t, conn)
			inst := &Instance{ctx: t.Context(), db: conn}
			event := eventbus.StatusChangedEvent{
				ServerID: "metrics-server", ServerNodeID: "metrics-node", ExecutionID: "attempt",
				OldStatus: tt.oldStatus, NewStatus: tt.newStatus, IntentionalStop: tt.intentional,
				ExitCodeKnown: true, ExitCode: tt.code, OccurredAt: time.Now(),
			}
			if tt.failure {
				report := diagnosis.Capture(nil, "final stderr")
				report.ExecutionID = event.ExecutionID
				report.AttemptStartedAt = event.OccurredAt.Add(-time.Minute)
				report.OccurredAt = event.OccurredAt
				report.Stage = diagnosis.StageRuntime
				event.Failure = &report
			}
			inst.recordLifecycleDiagnosis(event)
			report, errRead := conn.GetGameServerDiagnosis(t.Context(), event.ServerID)
			if !tt.wantReport {
				if !errors.Is(errRead, sql.ErrNoRows) {
					t.Fatalf("excluded event report = %+v, error = %v", report, errRead)
				}
				return
			}
			if errRead != nil {
				t.Fatal(errRead)
			}
			if tt.failure && report.Evidence != "final stderr" {
				t.Fatal(fmt.Errorf("final evidence not persisted: %+v", report))
			}
			// Starting again retains the previous report as historical evidence.
			event.OldStatus, event.NewStatus, event.Failure = "OFFLINE", "ONLINE", nil
			inst.recordLifecycleDiagnosis(event)
			retained, errRetained := conn.GetGameServerDiagnosis(t.Context(), event.ServerID)
			if errRetained != nil || retained.ExecutionID != report.ExecutionID {
				t.Fatalf("previous failure was lost: %v", errRetained)
			}
		})
	}
}

func TestLifecycleDiagnosisRetainsLatestLegacyCrash(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "legacy-diagnosis.sqlite")
	seedMetricsEventServer(t, conn)
	inst := &Instance{ctx: t.Context(), db: conn}
	first := time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC)
	latest := first.Add(time.Minute)
	for _, tt := range []struct {
		name       string
		occurredAt time.Time
		wantTime   time.Time
	}{
		{"first crash", first, first},
		{"later crash", latest, latest},
		{"delayed older crash", first, latest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inst.recordLifecycleDiagnosis(eventbus.StatusChangedEvent{
				ServerID: "metrics-server", ServerNodeID: "metrics-node",
				OldStatus: "ONLINE", NewStatus: "OFFLINE",
				ExitCodeKnown: true, ExitCode: 1, OccurredAt: tt.occurredAt,
			})
			report, errRead := conn.GetGameServerDiagnosis(t.Context(), "metrics-server")
			if errRead != nil {
				t.Fatal(errRead)
			}
			if !report.OccurredAt.Equal(tt.wantTime) {
				t.Fatalf("diagnosis occurrence = %v, want %v", report.OccurredAt, tt.wantTime)
			}
		})
	}
}
