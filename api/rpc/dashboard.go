package rpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// GetNodeSystemInfo returns system hardware/OS info for the requested node,
// resolving through NodeClient so embedded and remote nodes behave identically.
// When node_id is empty, defaults to the controller's embedded node.
func (xs *XylonaService) GetNodeSystemInfo(ctx context.Context, request *connect.Request[xylona.GetNodeSystemInfoRequest]) (*connect.Response[xylona.GetNodeSystemInfoResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	nodeID := request.Msg.GetNodeId()
	if nodeID == "" {
		nodeID = xs.nodeRegistry.SelfID()
	}

	client, errClient := xs.nodeRegistry.Get(nodeID)
	if errClient != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("node not reachable: %w", errClient))
	}

	snapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	snap, errSnap := client.GetNodeSnapshot(snapCtx)
	cancel()
	if errSnap != nil {
		return nil, internalErrf(fmt.Sprintf("failed to collect system info: %v", errSnap))
	}

	return connect.NewResponse(&xylona.GetNodeSystemInfoResponse{
		SystemInfo: &xylona.NodeSystemInfo{
			CpuModel:         snap.CPUModel,
			CpuCores:         helpers.ClampInt32FromInt(snap.CPUCores),
			CpuThreads:       helpers.ClampInt32FromInt(snap.CPUThreads),
			TotalMemoryBytes: helpers.ClampInt64FromUint64(snap.TotalMemory),
			Os:               snap.OS,
			OsVersion:        snap.OSVersion,
			Architecture:     snap.Architecture,
			XylonaVersion:    snap.XylonaVersion,
		},
	}), nil
}

// GetNodeResourceSnapshot returns a live resource usage snapshot for the
// requested node, routing through NodeClient for full parity between
// embedded and remote. Defaults to the local node when node_id is empty.
func (xs *XylonaService) GetNodeResourceSnapshot(ctx context.Context, request *connect.Request[xylona.GetNodeResourceSnapshotRequest]) (*connect.Response[xylona.GetNodeResourceSnapshotResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	nodeID := request.Msg.GetNodeId()
	if nodeID == "" {
		nodeID = xs.nodeRegistry.SelfID()
	}

	client, errClient := xs.nodeRegistry.Get(nodeID)
	if errClient != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("node not reachable: %w", errClient))
	}

	snapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	snap, errSnap := client.GetNodeSnapshot(snapCtx)
	cancel()
	if errSnap != nil {
		return nil, internalErrf(fmt.Sprintf("failed to collect resource snapshot: %v", errSnap))
	}

	gsCount := 0
	allServers, _ := xs.db.GetAllGameServers()
	for _, gs := range allServers {
		if gs.NodeID == nodeID {
			gsCount++
		}
	}
	runningCount := 0
	for _, ps := range snap.Processes {
		switch ps.Status {
		case xylona.Status_ONLINE.String(),
			xylona.Status_INSTALLING.String(),
			xylona.Status_UPDATING.String():
			runningCount++
		}
	}
	userCount, _ := xs.db.CountUsers()

	return connect.NewResponse(&xylona.GetNodeResourceSnapshotResponse{
		Snapshot: &xylona.NodeResourceSnapshot{
			CpuPercent:             snap.CPUPercent,
			MemoryPercent:          snap.MemoryPercent,
			MemoryUsedBytes:        helpers.ClampInt64FromUint64(snap.MemoryUsed),
			MemoryTotalBytes:       helpers.ClampInt64FromUint64(snap.TotalMemory),
			DiskPercent:            snap.DiskPercent,
			DiskUsedBytes:          helpers.ClampInt64FromUint64(snap.DiskUsed),
			DiskTotalBytes:         helpers.ClampInt64FromUint64(snap.DiskTotal),
			GameServerCount:        helpers.ClampInt32FromInt(gsCount),
			RunningGameServerCount: helpers.ClampInt32FromInt(runningCount),
			UserCount:              helpers.ClampInt32FromInt(userCount),
			RecordedAt:             timestamppb.Now(),
		},
	}), nil
}

// GetDashboardOverview returns an overview of all registered nodes, pulling
// system info + per-node snapshot via NodeClient.GetNodeSnapshot for both
// embedded and remote nodes.
func (xs *XylonaService) GetDashboardOverview(ctx context.Context, request *connect.Request[xylona.GetDashboardOverviewRequest]) (*connect.Response[xylona.GetDashboardOverviewResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	var summaries []*xylona.DashboardNodeSummary

	allNodes, errNodes := xs.db.GetAllNodes()
	if errNodes != nil {
		return nil, internalErrf("failed to list nodes")
	}

	for _, nodeRow := range allNodes {
		summary := &xylona.DashboardNodeSummary{
			Node: helpers.NodeModelToProto(nodeRow),
		}

		client, errClient := xs.nodeRegistry.Get(nodeRow.ID)
		if errClient != nil {
			log.Debug().Err(errClient).Str("node_id", nodeRow.ID).
				Msg("GetDashboardOverview: node not currently reachable")
			summaries = append(summaries, summary)
			continue
		}

		snapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		snap, errSnap := client.GetNodeSnapshot(snapCtx)
		cancel()
		if errSnap != nil {
			log.Warn().Err(errSnap).Str("node_id", nodeRow.ID).
				Msg("GetDashboardOverview: node snapshot failed")
			summaries = append(summaries, summary)
			continue
		}

		summary.SystemInfo = &xylona.NodeSystemInfo{
			CpuModel:         snap.CPUModel,
			CpuCores:         helpers.ClampInt32FromInt(snap.CPUCores),
			CpuThreads:       helpers.ClampInt32FromInt(snap.CPUThreads),
			TotalMemoryBytes: helpers.ClampInt64FromUint64(snap.TotalMemory),
			Os:               snap.OS,
			OsVersion:        snap.OSVersion,
			Architecture:     snap.Architecture,
			XylonaVersion:    snap.XylonaVersion,
		}

		gsCount := 0
		allServers, _ := xs.db.GetAllGameServers()
		for _, gs := range allServers {
			if gs.NodeID == nodeRow.ID {
				gsCount++
			}
		}
		userCount, _ := xs.db.CountUsers()
		runningCount := 0
		for _, ps := range snap.Processes {
			if ps.Status == xylona.Status_ONLINE.String() ||
				ps.Status == xylona.Status_INSTALLING.String() ||
				ps.Status == xylona.Status_UPDATING.String() {
				runningCount++
			}
		}

		summary.Snapshot = &xylona.NodeResourceSnapshot{
			CpuPercent:             snap.CPUPercent,
			MemoryPercent:          snap.MemoryPercent,
			MemoryUsedBytes:        helpers.ClampInt64FromUint64(snap.MemoryUsed),
			MemoryTotalBytes:       helpers.ClampInt64FromUint64(snap.TotalMemory),
			DiskPercent:            snap.DiskPercent,
			DiskUsedBytes:          helpers.ClampInt64FromUint64(snap.DiskUsed),
			DiskTotalBytes:         helpers.ClampInt64FromUint64(snap.DiskTotal),
			GameServerCount:        helpers.ClampInt32FromInt(gsCount),
			RunningGameServerCount: helpers.ClampInt32FromInt(runningCount),
			UserCount:              helpers.ClampInt32FromInt(userCount),
		}

		summaries = append(summaries, summary)
	}

	return connect.NewResponse(&xylona.GetDashboardOverviewResponse{
		Nodes: summaries,
	}), nil
}

// GetNodeMetricsHistory returns historical metrics for the requested node.
// History rows are keyed by node_id in the controller's DB, so the same
// handler serves embedded and remote nodes uniformly. Defaults to the local
// node when node_id is empty.
func (xs *XylonaService) GetNodeMetricsHistory(_ context.Context, request *connect.Request[xylona.GetNodeMetricsHistoryRequest]) (*connect.Response[xylona.GetNodeMetricsHistoryResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	since := request.Msg.GetSince().AsTime()
	until := request.Msg.GetUntil().AsTime()

	nodeID := request.Msg.GetNodeId()
	if nodeID == "" {
		nodeID = xs.nodeRegistry.SelfID()
	}

	rows, errQuery := xs.db.GetNodeMetricsHistory(nodeID, since, until)
	if errQuery != nil {
		return nil, internalErrf("failed to query node metrics history")
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

	return connect.NewResponse(&xylona.GetNodeMetricsHistoryResponse{
		Points: points,
	}), nil
}

// GetGameServerMetricsHistory returns historical metrics for a game server
// owned by the controller's embedded node.
func (xs *XylonaService) GetGameServerMetricsHistory(_ context.Context, request *connect.Request[xylona.GetGameServerMetricsHistoryRequest]) (*connect.Response[xylona.GetGameServerMetricsHistoryResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	gameServerID := request.Msg.GetGameServerId()
	since := request.Msg.GetSince().AsTime()
	until := request.Msg.GetUntil().AsTime()

	gameServer, errLookup := xs.db.GetGameServerByID(gameServerID)
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}

	if !user.SuperUser && gameServer.UserID != user.ID {
		allowed, errPerm := helpers.HasPermission(xs.db, user, gameServerID, gameServer.UserID, "game_server.metrics")
		if errPerm != nil {
			return nil, internalErrf("failed to check permissions")
		}
		if !allowed {
			return nil, permissionDenied("access denied")
		}
	}

	return xs.queryLocalGameServerMetricsHistory(gameServerID, since, until)
}

func (xs *XylonaService) queryLocalGameServerMetricsHistory(gameServerID string, since, until time.Time) (*connect.Response[xylona.GetGameServerMetricsHistoryResponse], error) {
	rows, errQuery := xs.db.GetGameServerMetricsHistory(gameServerID, since, until)
	if errQuery != nil {
		return nil, internalErrf("failed to query game server metrics history")
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

	return connect.NewResponse(&xylona.GetGameServerMetricsHistoryResponse{
		Points: points,
	}), nil
}
