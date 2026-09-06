package rpc

import (
	"context"
	"database/sql"
	"errors"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// GetGameServerDiagnosis returns durable failure metadata and authorized evidence.
func (xs *XylonaService) GetGameServerDiagnosis(ctx context.Context, request *connect.Request[xylona.GetGameServerDiagnosisRequest]) (*connect.Response[xylona.GetGameServerDiagnosisResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}
	canReadEvidence, errConsolePermission := db.HasPermission(xs.db, user, gameServer.ID, gameServer.UserID, permissionConsole)
	if errConsolePermission != nil {
		log.Error().Err(errConsolePermission).Str("server_id", gameServer.ID).Msg("Failed to check diagnosis evidence permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check permissions"))
	}
	report, errReport := xs.db.GetGameServerDiagnosis(ctx, gameServer.ID)
	if errors.Is(errReport, sql.ErrNoRows) {
		return connect.NewResponse(&xylona.GetGameServerDiagnosisResponse{}), nil
	}
	if errReport != nil {
		log.Error().Err(errReport).Str("server_id", gameServer.ID).Msg("Failed to read game server diagnosis")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to read game server diagnosis"))
	}
	result := &xylona.GameServerDiagnosis{
		ExecutionId:        report.ExecutionID,
		OccurredAt:         timestamppb.New(report.OccurredAt),
		Stage:              report.Stage,
		Category:           report.Category,
		Truncated:          report.Truncated,
		EvidenceAvailable:  report.EvidenceAvailable,
		EvidenceRestricted: !canReadEvidence,
		Inferred:           report.MatchedEvidence != "",
	}
	if !report.AttemptStartedAt.IsZero() {
		result.AttemptStartedAt = timestamppb.New(report.AttemptStartedAt)
	}
	if report.ExitCode != nil {
		result.ExitCode = new(int64(*report.ExitCode))
	}
	if canReadEvidence {
		result.Error = report.Error
		result.Evidence = report.Evidence
		result.MatchedEvidence = report.MatchedEvidence
	}
	return connect.NewResponse(&xylona.GetGameServerDiagnosisResponse{Diagnosis: result}), nil
}
