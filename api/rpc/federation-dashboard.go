package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/sysinfo"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// FederationGetNodeSystemInfo returns system info for the local node.
func (fs FederationService) FederationGetNodeSystemInfo(ctx context.Context, _ *connect.Request[xylona.FederationGetNodeSystemInfoRequest]) (*connect.Response[xylona.FederationGetNodeSystemInfoResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	info, errInfo := sysinfo.CollectSystemInfo()
	if errInfo != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to collect system info"))
	}

	return connect.NewResponse(&xylona.FederationGetNodeSystemInfoResponse{
		SystemInfo: sysInfoToProto(info),
	}), nil
}

// FederationGetNodeResourceSnapshot returns a live resource snapshot for the local node.
func (fs FederationService) FederationGetNodeResourceSnapshot(ctx context.Context, _ *connect.Request[xylona.FederationGetNodeResourceSnapshotRequest]) (*connect.Response[xylona.FederationGetNodeResourceSnapshotResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	snapshot, errSnap := sysinfo.CollectResourceSnapshot()
	if errSnap != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to collect resource snapshot"))
	}

	gsCount, _ := fs.db.CountGameServers()
	runningCount := 0
	if fs.supervisorInst != nil {
		for _, cmd := range fs.supervisorInst.ListCommands() {
			if cmd.Status() == xylona.Status_ONLINE || cmd.Status() == xylona.Status_INSTALLING || cmd.Status() == xylona.Status_UPDATING {
				runningCount++
			}
		}
	}
	userCount, _ := fs.db.CountUsers()

	return connect.NewResponse(&xylona.FederationGetNodeResourceSnapshotResponse{
		Snapshot: snapshotToProto(snapshot, gsCount, runningCount, userCount),
	}), nil
}

// FederationGetNodeMetricsHistory returns historical node metrics.
func (fs FederationService) FederationGetNodeMetricsHistory(ctx context.Context, request *connect.Request[xylona.FederationGetNodeMetricsHistoryRequest]) (*connect.Response[xylona.FederationGetNodeMetricsHistoryResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	localNodeID, errLocal := fs.db.GetLocalNodeID()
	if errLocal != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local node ID"))
	}

	since := request.Msg.GetSince().AsTime()
	until := request.Msg.GetUntil().AsTime()

	rows, errQuery := fs.db.GetNodeMetricsHistory(localNodeID, since, until)
	if errQuery != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to query metrics history"))
	}

	var points []*xylona.MetricsHistoryPoint
	for _, row := range rows {
		points = append(points, &xylona.MetricsHistoryPoint{
			Timestamp:       timestamppb.New(row.RecordedAt),
			CpuPercent:      row.CPUPercent,
			MemoryPercent:   row.MemoryPercent,
			DiskPercent:     row.DiskPercent,
			MemoryUsedBytes: row.MemoryUsedBytes,
			DiskUsedBytes:   row.DiskUsedBytes,
		})
	}

	return connect.NewResponse(&xylona.FederationGetNodeMetricsHistoryResponse{
		Points: points,
	}), nil
}

// FederationGetGameServerMetricsHistory returns historical game server metrics.
func (fs FederationService) FederationGetGameServerMetricsHistory(ctx context.Context, request *connect.Request[xylona.FederationGetGameServerMetricsHistoryRequest]) (*connect.Response[xylona.FederationGetGameServerMetricsHistoryResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	gameServerID := request.Msg.GetGameServerId()
	since := request.Msg.GetSince().AsTime()
	until := request.Msg.GetUntil().AsTime()

	rows, errQuery := fs.db.GetGameServerMetricsHistory(gameServerID, since, until)
	if errQuery != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to query game server metrics history"))
	}

	var points []*xylona.GameServerMetricsHistoryPoint
	for _, row := range rows {
		points = append(points, &xylona.GameServerMetricsHistoryPoint{
			Timestamp:      timestamppb.New(row.RecordedAt),
			CpuPercent:     row.CPUPercent,
			MemoryBytes:    row.MemoryBytes,
			MemoryPercent:  row.MemoryPercent,
			DiskUsageBytes: row.DiskUsageBytes,
			PlayerCount:    helpers.ClampInt32FromInt(row.PlayerCount),
		})
	}

	return connect.NewResponse(&xylona.FederationGetGameServerMetricsHistoryResponse{
		Points: points,
	}), nil
}
