package db

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	"github.com/ClintonCollins/Xylona/sql/migrations"
)

func newDiagnosisConnection(t *testing.T) *Connection {
	t.Helper()
	sqlDB, errOpen := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if errOpen != nil {
		t.Fatal(errOpen)
	}
	conn := &Connection{SQLDb: sqlDB, ctx: t.Context()}
	conn.SQLDb.SetMaxOpenConns(1)
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Error(errClose)
		}
	})
	setTableOnce.Do(func() { migrate.SetTable("migrations") })
	_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrate.EmbedFileSystemMigrationSource{
		FileSystem: migrations.FS, Root: migrations.Root,
	}, migrate.Up)
	if errMigrate != nil {
		t.Fatal(errMigrate)
	}
	seedRBACFixture(t, conn)
	return conn
}

func TestGameServerDiagnosisOrderingAndEnrichment(t *testing.T) {
	t.Parallel()
	conn := newDiagnosisConnection(t)
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	unknown := diagnosis.Report{
		ExecutionID: "attempt-1", AttemptStartedAt: now, OccurredAt: now.Add(time.Second),
		Stage: diagnosis.StageUnknownOutcome, Category: diagnosis.CategoryUnknown,
		Error: "start request timed out",
	}
	confirmed := unknown
	confirmed.Stage = diagnosis.StageRuntime
	confirmed.Category = diagnosis.CategoryPortInUse
	confirmed.Error = ""
	confirmed.Evidence = "failed to bind: address already in use"
	confirmed.MatchedEvidence = confirmed.Evidence
	confirmed.EvidenceAvailable = true
	confirmed.ExitCode = new(1)
	confirmed.AttemptStartedAt = time.Time{}
	richer := confirmed
	richer.Evidence = "previous terminal line\n" + confirmed.Evidence
	newer := diagnosis.Report{
		ExecutionID: "attempt-2", AttemptStartedAt: now.Add(time.Minute), OccurredAt: now.Add(time.Minute),
		Stage: diagnosis.StagePreStart, Category: diagnosis.CategoryIncompleteSetup,
	}
	legacy := confirmed
	legacy.ExecutionID = "legacy-attempt"
	legacy.OccurredAt = now.Add(time.Hour)
	for _, tt := range []struct {
		name         string
		report       diagnosis.Report
		wantID       string
		wantStage    string
		wantEvidence string
	}{
		{"initial uncertain outcome", unknown, "attempt-1", diagnosis.StageUnknownOutcome, ""},
		{"confirmed event enriches attempt", confirmed, "attempt-1", diagnosis.StageRuntime, confirmed.Evidence},
		{"richer terminal evidence enriches report", richer, "attempt-1", diagnosis.StageRuntime, richer.Evidence},
		{"duplicate replay", richer, "attempt-1", diagnosis.StageRuntime, richer.Evidence},
		{"poorer replay cannot wipe evidence", confirmed, "attempt-1", diagnosis.StageRuntime, richer.Evidence},
		{"delayed uncertain outcome", unknown, "attempt-1", diagnosis.StageRuntime, richer.Evidence},
		{"newer attempt replaces report", newer, "attempt-2", diagnosis.StagePreStart, ""},
		{"older runtime event cannot replace newer attempt", confirmed, "attempt-2", diagnosis.StagePreStart, ""},
		{"missing attempt time cannot replace dated attempt", legacy, "attempt-2", diagnosis.StagePreStart, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			errSave := conn.SaveGameServerDiagnosis(t.Context(), "server-local-1", "node-local", tt.report)
			if errSave != nil {
				t.Fatal(errSave)
			}
			got, errRead := conn.GetGameServerDiagnosis(t.Context(), "server-local-1")
			if errRead != nil {
				t.Fatal(errRead)
			}
			if got.ExecutionID != tt.wantID || got.Stage != tt.wantStage || got.Evidence != tt.wantEvidence {
				t.Fatalf("report = %+v", got)
			}
			if got.AttemptStartedAt.IsZero() {
				t.Fatal("enrichment discarded the controller attempt timestamp")
			}
			if tt.wantStage == diagnosis.StageRuntime && (got.ExitCode == nil || *got.ExitCode != 1) {
				t.Fatalf("exit code = %v, want 1", got.ExitCode)
			}
			if tt.wantStage == diagnosis.StageRuntime && got.Error != "" {
				t.Fatalf("confirmed failure retained uncertain dispatch error: %q", got.Error)
			}
		})
	}
}

func TestGameServerDiagnosisOlderNodeOrdering(t *testing.T) {
	conn := newDiagnosisConnection(t)
	firstID := "0198f015-0000-7000-8000-000000000001"
	secondID := "0198f015-0000-7001-8000-000000000001"
	first := diagnosis.Report{
		ExecutionID: firstID, AttemptStartedAt: diagnosis.AttemptTime(firstID), OccurredAt: time.Now(),
		Stage: diagnosis.StagePreStart, Category: diagnosis.CategoryIncompleteSetup,
	}
	// Older nodes echo the execution identifier but not the added timestamp.
	second := diagnosis.Report{
		ExecutionID: secondID, OccurredAt: time.Now(),
		Stage: diagnosis.StageUnknown, Category: diagnosis.CategoryUnknown,
	}
	for _, report := range []diagnosis.Report{first, second, first} {
		errSave := conn.SaveGameServerDiagnosis(t.Context(), "server-local-1", "node-local", report)
		if errSave != nil {
			t.Fatal(errSave)
		}
	}
	got, errRead := conn.GetGameServerDiagnosis(t.Context(), "server-local-1")
	if errRead != nil {
		t.Fatal(errRead)
	}
	if got.ExecutionID != secondID || got.AttemptStartedAt.IsZero() {
		t.Fatalf("older node's newer failure was lost: %+v", got)
	}
}

func TestGameServerDiagnosisOwnershipBoundsAndCascade(t *testing.T) {
	t.Parallel()
	conn := newDiagnosisConnection(t)
	_, errMissing := conn.GetGameServerDiagnosis(t.Context(), "server-local-1")
	if !errors.Is(errMissing, sql.ErrNoRows) {
		t.Fatalf("absent report error = %v", errMissing)
	}
	report := diagnosis.Report{
		ExecutionID: "attempt-1", AttemptStartedAt: time.Now().UTC(), OccurredAt: time.Now().UTC(),
		Stage: diagnosis.StageRuntime, Category: diagnosis.CategoryUnknown,
		Evidence:          strings.Repeat("long line with unicode é\n", 4000) + "final stderr\xff",
		EvidenceAvailable: true,
	}
	errWrongNode := conn.SaveGameServerDiagnosis(t.Context(), "server-local-1", "previous-node", report)
	if errWrongNode != nil {
		t.Fatal(errWrongNode)
	}
	_, errRejected := conn.GetGameServerDiagnosis(t.Context(), "server-local-1")
	if !errors.Is(errRejected, sql.ErrNoRows) {
		t.Fatalf("wrong node report was stored: %v", errRejected)
	}
	errSave := conn.SaveGameServerDiagnosis(t.Context(), "server-local-1", "node-local", report)
	if errSave != nil {
		t.Fatal(errSave)
	}
	got, errRead := conn.GetGameServerDiagnosis(t.Context(), "server-local-1")
	if errRead != nil {
		t.Fatal(errRead)
	}
	if !got.Truncated || len(got.Evidence) > diagnosis.MaxEvidenceBytes || strings.Count(got.Evidence, "\n") >= diagnosis.MaxEvidenceLines || !utf8.ValidString(got.Evidence) || !strings.Contains(got.Evidence, "final stderr") {
		t.Fatalf("invalid bounded evidence: bytes=%d, truncated=%v", len(got.Evidence), got.Truncated)
	}
	_, errReassign := conn.SQLDb.ExecContext(t.Context(), `insert into node (id, name, listen_url, enabled) values ('new-node', 'New node', 'https://new-node', true);
		insert into ip (address, node_id) values ('127.0.0.1', 'new-node');
		update game_server set node_id = 'new-node' where id = 'server-local-1'`)
	if errReassign != nil {
		t.Fatal(errReassign)
	}
	report.ExecutionID = "attempt-2"
	report.AttemptStartedAt = report.AttemptStartedAt.Add(time.Minute)
	errPreviousOwner := conn.SaveGameServerDiagnosis(t.Context(), "server-local-1", "node-local", report)
	if errPreviousOwner != nil {
		t.Fatal(errPreviousOwner)
	}
	got, errRead = conn.GetGameServerDiagnosis(t.Context(), "server-local-1")
	if errRead != nil || got.ExecutionID != "attempt-1" {
		t.Fatalf("previous node replaced report: report=%+v error=%v", got, errRead)
	}
	_, errDelete := conn.SQLDb.ExecContext(t.Context(), "delete from game_server where id = ?", "server-local-1")
	if errDelete != nil {
		t.Fatal(errDelete)
	}
	_, errGone := conn.GetGameServerDiagnosis(t.Context(), "server-local-1")
	if !errors.Is(errGone, sql.ErrNoRows) {
		t.Fatalf("deleted server retained report: %v", errGone)
	}
}

func TestGameServerDiagnosisPersistsAcrossConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("requires file-backed database")
	}
	conn := newRBACMigratedConnection(t, "diagnosis-persistence.sqlite")
	seedRBACFixture(t, conn)
	report := diagnosis.Report{
		ExecutionID: "persisted-attempt", AttemptStartedAt: time.Now().UTC(), OccurredAt: time.Now().UTC(),
		Stage: diagnosis.StageLaunch, Category: diagnosis.CategoryMissingExecutable, Error: "missing executable",
	}
	errSave := conn.SaveGameServerDiagnosis(t.Context(), "server-local-1", "node-local", report)
	if errSave != nil {
		t.Fatal(errSave)
	}
	var dbPath string
	errPath := conn.SQLDb.QueryRowContext(t.Context(), "select file from pragma_database_list where name = 'main'").Scan(&dbPath)
	if errPath != nil {
		t.Fatal(errPath)
	}
	errClose := conn.SQLDb.Close()
	if errClose != nil {
		t.Fatal(errClose)
	}
	reopened, errOpen := NewConnection(t.Context(), dbPath)
	if errOpen != nil {
		t.Fatal(errOpen)
	}
	t.Cleanup(func() {
		errCloseReopened := reopened.SQLDb.Close()
		if errCloseReopened != nil {
			t.Error(errCloseReopened)
		}
	})
	got, errRead := reopened.GetGameServerDiagnosis(t.Context(), "server-local-1")
	if errRead != nil || got.ExecutionID != report.ExecutionID || !got.OccurredAt.Equal(report.OccurredAt) {
		t.Fatalf("reopened diagnosis = %+v, error = %v", got, errRead)
	}
}
