package rpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
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

	// The running count is derived from these IDs, so a failed listing would
	// report 0 running rather than just 0 assigned: fail instead of guessing.
	allServers, errServers := xs.db.GetAllGameServers()
	if errServers != nil {
		log.Error().Err(errServers).Str("node_id", nodeID).Msg("Failed to list game servers for node resource snapshot")
		return nil, internalErrf("failed to list game servers")
	}
	gameServerIDs := map[string]struct{}{}
	for _, gs := range allServers {
		if gs.NodeID == nodeID {
			gameServerIDs[gs.ID] = struct{}{}
		}
	}
	userCount, errUserCount := xs.db.CountUsers()
	if errUserCount != nil {
		log.Warn().Err(errUserCount).Str("node_id", nodeID).Msg("Failed to count users for node resource snapshot")
	}

	return connect.NewResponse(&xylona.GetNodeResourceSnapshotResponse{
		Snapshot: &xylona.NodeResourceSnapshot{
			CpuPercent:             snap.CPUPercent,
			MemoryPercent:          snap.MemoryPercent,
			MemoryUsedBytes:        helpers.ClampInt64FromUint64(snap.MemoryUsed),
			MemoryTotalBytes:       helpers.ClampInt64FromUint64(snap.TotalMemory),
			DiskPercent:            snap.DiskPercent,
			DiskUsedBytes:          helpers.ClampInt64FromUint64(snap.DiskUsed),
			DiskTotalBytes:         helpers.ClampInt64FromUint64(snap.DiskTotal),
			GameServerCount:        helpers.ClampInt32FromInt(len(gameServerIDs)),
			RunningGameServerCount: helpers.ClampInt32FromInt(snap.RunningGameServerCount(gameServerIDs)),
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

	selfNodeID := xs.selfNodeID()
	runtimeState := xs.collectNodeRuntimeState(ctx, allNodes)

	serverIDsByNodeID := map[string]map[string]struct{}{}
	allServers, errServers := xs.db.GetAllGameServers()
	if errServers != nil {
		log.Warn().Err(errServers).Msg("Failed to list game servers for dashboard overview; server counts will read 0")
	} else {
		for _, gameServer := range allServers {
			if serverIDsByNodeID[gameServer.NodeID] == nil {
				serverIDsByNodeID[gameServer.NodeID] = map[string]struct{}{}
			}
			serverIDsByNodeID[gameServer.NodeID][gameServer.ID] = struct{}{}
		}
	}

	userCount, errUserCount := xs.db.CountUsers()
	if errUserCount != nil {
		log.Warn().Err(errUserCount).Msg("Failed to count users for dashboard overview")
		userCount = 0
	}

	for _, nodeRow := range allNodes {
		summary := &xylona.DashboardNodeSummary{
			Node: xs.nodeProtoWithRuntimeState(nodeRow, selfNodeID, runtimeState),
		}

		state, ok := runtimeState[nodeRow.ID]
		if !ok || state.snapshot == nil {
			summaries = append(summaries, summary)
			continue
		}

		snap := state.snapshot

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

		nodeServerIDs := serverIDsByNodeID[nodeRow.ID]

		summary.Snapshot = &xylona.NodeResourceSnapshot{
			CpuPercent:             snap.CPUPercent,
			MemoryPercent:          snap.MemoryPercent,
			MemoryUsedBytes:        helpers.ClampInt64FromUint64(snap.MemoryUsed),
			MemoryTotalBytes:       helpers.ClampInt64FromUint64(snap.TotalMemory),
			DiskPercent:            snap.DiskPercent,
			DiskUsedBytes:          helpers.ClampInt64FromUint64(snap.DiskUsed),
			DiskTotalBytes:         helpers.ClampInt64FromUint64(snap.DiskTotal),
			GameServerCount:        helpers.ClampInt32FromInt(len(nodeServerIDs)),
			RunningGameServerCount: helpers.ClampInt32FromInt(snap.RunningGameServerCount(nodeServerIDs)),
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

	maxPoints := int(request.Msg.GetMaxPoints())
	if maxPoints <= 0 {
		maxPoints = defaultNodeMetricsMaxPoints
	}
	maxPoints = min(maxPoints, maximumNodeMetricsMaxPoints)

	rows, errQuery := xs.db.GetNodeMetricsHistory(nodeID, since, until)
	if errQuery != nil {
		return nil, internalErrf("failed to query node metrics history")
	}
	rows, sampleInterval := downsampleNodeMetricsRows(rows, since, until, maxPoints)

	var points []*xylona.MetricsHistoryPoint
	for _, row := range rows {
		points = append(points, &xylona.MetricsHistoryPoint{
			Timestamp:              timestamppb.New(row.RecordedAt),
			CpuPercent:             row.CPUPercent,
			MemoryPercent:          row.MemoryPercent,
			DiskPercent:            row.DiskPercent,
			MemoryUsedBytes:        row.MemoryUsedBytes,
			DiskUsedBytes:          row.DiskUsedBytes,
			GameServerCount:        helpers.ClampInt32FromInt(row.GameServerCount),
			RunningGameServerCount: helpers.ClampInt32FromInt(row.RunningGameServerCount),
		})
	}

	return connect.NewResponse(&xylona.GetNodeMetricsHistoryResponse{
		Points:                points,
		SampleIntervalSeconds: sampleInterval,
	}), nil
}

// GetGameServerMetricsHistory returns controller-recorded historical metrics
// and events for an authorized game server.
func (xs *XylonaService) GetGameServerMetricsHistory(ctx context.Context, request *connect.Request[xylona.GetGameServerMetricsHistoryRequest]) (*connect.Response[xylona.GetGameServerMetricsHistoryResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	gameServerID := request.Msg.GetGameServerId()
	since, until, maxPoints, errRange := validateGameServerMetricsHistoryRequest(request.Msg)
	if errRange != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errRange)
	}

	gameServer, errLookup := xs.db.GetGameServerByID(gameServerID)
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}

	if !user.SuperUser && gameServer.UserID != user.ID {
		allowed, errPerm := db.HasPermission(xs.db, user, gameServerID, gameServer.UserID, "game_server.metrics")
		if errPerm != nil {
			return nil, internalErrf("failed to check permissions")
		}
		if !allowed {
			return nil, permissionDenied("access denied")
		}
	}

	return xs.queryLocalGameServerMetricsHistory(ctx, gameServerID, since, until, maxPoints)
}

func (xs *XylonaService) queryLocalGameServerMetricsHistory(ctx context.Context, gameServerID string, since, until time.Time, maxPoints int) (*connect.Response[xylona.GetGameServerMetricsHistoryResponse], error) {
	rows, errQuery := xs.db.GetGameServerMetricsHistory(gameServerID, since, until)
	if errQuery != nil {
		return nil, internalErrf("failed to query game server metrics history")
	}

	series := buildGameServerMetricsHistory(rows, since, until, maxPoints)
	lifecycleRows, errLifecycle := xs.db.GetGameServerLifecycleEvents(ctx, gameServerID, since, until)
	if errLifecycle != nil {
		return nil, internalErrf("failed to query game server lifecycle history")
	}
	operationRows, errOperations := xs.db.GetGameServerOperationEvents(ctx, gameServerID, since, until)
	if errOperations != nil {
		return nil, internalErrf("failed to query game server operation history")
	}

	return connect.NewResponse(&xylona.GetGameServerMetricsHistoryResponse{
		Points:                series.points,
		LifecycleEvents:       mapGameServerLifecycleEvents(lifecycleRows),
		OperationEvents:       mapGameServerOperationEvents(operationRows),
		Resolution:            series.resolution,
		SampleIntervalSeconds: series.sampleIntervalSeconds,
		HasMixedResolution:    series.hasMixedResolution,
	}), nil
}
