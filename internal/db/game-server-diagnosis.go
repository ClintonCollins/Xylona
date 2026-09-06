package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
)

// SaveGameServerDiagnosis retains the newest attempt and enriches replayed reports.
// Ownership and ordering are checked in the same statement as the write.
func (c *Connection) SaveGameServerDiagnosis(ctx context.Context, serverID, nodeID string, report diagnosis.Report) error {
	report = diagnosis.Bound(report)
	var attemptStartedAt int64
	if !report.AttemptStartedAt.IsZero() {
		attemptStartedAt = report.AttemptStartedAt.UnixNano()
	}
	_, errSave := c.SQLDb.ExecContext(ctx, `
		insert into game_server_diagnosis
			(game_server_id, node_id, execution_id, attempt_started_at, occurred_at,
			 stage, category, error, evidence, matched_evidence, truncated, evidence_available, exit_code, quality)
		select id, node_id, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		from game_server where id = ? and node_id = ?
		on conflict(game_server_id) do update set
			node_id = excluded.node_id,
			execution_id = excluded.execution_id,
			attempt_started_at = case
				when excluded.execution_id = game_server_diagnosis.execution_id and excluded.attempt_started_at = 0
				then game_server_diagnosis.attempt_started_at else excluded.attempt_started_at end,
			occurred_at = excluded.occurred_at,
			stage = excluded.stage,
			category = excluded.category,
			error = case
				when excluded.execution_id = game_server_diagnosis.execution_id
				and excluded.stage = game_server_diagnosis.stage and excluded.error = ''
				then game_server_diagnosis.error else excluded.error end,
			evidence = case
				when excluded.execution_id = game_server_diagnosis.execution_id
				and length(cast(excluded.evidence as blob)) < length(cast(game_server_diagnosis.evidence as blob))
				then game_server_diagnosis.evidence else excluded.evidence end,
			matched_evidence = case
				when excluded.execution_id = game_server_diagnosis.execution_id
				and excluded.category = game_server_diagnosis.category and excluded.matched_evidence = ''
				then game_server_diagnosis.matched_evidence else excluded.matched_evidence end,
			truncated = case
				when excluded.execution_id = game_server_diagnosis.execution_id
				and length(cast(excluded.evidence as blob)) < length(cast(game_server_diagnosis.evidence as blob))
				then game_server_diagnosis.truncated else excluded.truncated end,
			evidence_available = case
				when excluded.execution_id = game_server_diagnosis.execution_id
				then max(excluded.evidence_available, game_server_diagnosis.evidence_available)
				else excluded.evidence_available end,
			exit_code = case
				when excluded.execution_id = game_server_diagnosis.execution_id
				then coalesce(excluded.exit_code, game_server_diagnosis.exit_code) else excluded.exit_code end,
			quality = excluded.quality
		where
			(excluded.execution_id = game_server_diagnosis.execution_id
			 and excluded.node_id = game_server_diagnosis.node_id
			 and excluded.quality > game_server_diagnosis.quality)
			or (excluded.execution_id != game_server_diagnosis.execution_id
			 and (excluded.attempt_started_at > game_server_diagnosis.attempt_started_at
				or (excluded.attempt_started_at > 0 and excluded.attempt_started_at = game_server_diagnosis.attempt_started_at
					and excluded.execution_id > game_server_diagnosis.execution_id)
				or (excluded.attempt_started_at = 0 and game_server_diagnosis.attempt_started_at = 0
					and excluded.occurred_at > game_server_diagnosis.occurred_at)))`,
		report.ExecutionID, attemptStartedAt, report.OccurredAt.UnixNano(),
		report.Stage, report.Category, report.Error, report.Evidence, report.MatchedEvidence,
		report.Truncated, report.EvidenceAvailable, report.ExitCode, diagnosisQuality(report), serverID, nodeID)
	if errSave != nil {
		return fmt.Errorf("save game server diagnosis: %w", errSave)
	}
	return nil
}

// GetGameServerDiagnosis returns the latest failure, or sql.ErrNoRows when none exists.
func (c *Connection) GetGameServerDiagnosis(ctx context.Context, serverID string) (*diagnosis.Report, error) {
	var report diagnosis.Report
	var attemptStartedAt, occurredAt int64
	var exitCode sql.NullInt64
	errRead := c.SQLDb.QueryRowContext(ctx, `
		select execution_id, attempt_started_at, occurred_at, stage, category, error,
			evidence, matched_evidence, truncated, evidence_available, exit_code
		from game_server_diagnosis where game_server_id = ?`, serverID).Scan(
		&report.ExecutionID, &attemptStartedAt, &occurredAt, &report.Stage, &report.Category,
		&report.Error, &report.Evidence, &report.MatchedEvidence, &report.Truncated,
		&report.EvidenceAvailable, &exitCode)
	if errRead != nil {
		return nil, fmt.Errorf("get game server diagnosis: %w", errRead)
	}
	if attemptStartedAt != 0 {
		report.AttemptStartedAt = time.Unix(0, attemptStartedAt).UTC()
	}
	report.OccurredAt = time.Unix(0, occurredAt).UTC()
	if exitCode.Valid {
		report.ExitCode = new(int(exitCode.Int64))
	}
	return &report, nil
}

func diagnosisQuality(report diagnosis.Report) int {
	quality := 0
	switch report.Stage {
	case diagnosis.StagePreStart, diagnosis.StageUnknown:
		quality = 1 << 22
	case diagnosis.StageLaunch:
		quality = 2 << 22
	case diagnosis.StageRuntime:
		quality = 3 << 22
	}
	if report.EvidenceAvailable {
		quality += 1 << 20
	}
	if report.Category != "" && report.Category != diagnosis.CategoryUnknown {
		quality += 4
	}
	if report.Error != "" {
		quality += 2
	}
	if report.ExitCode != nil {
		quality++
	}
	return quality + min(len(report.Evidence), diagnosis.MaxEvidenceBytes)*8
}
