package rpc

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/sysinfo"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func sysInfoToProto(info *sysinfo.SystemInfo) *xylona.NodeSystemInfo {
	return &xylona.NodeSystemInfo{
		CpuModel:      info.CPUModel,
		CpuCores:      int32(info.CPUCores),
		CpuThreads:    int32(info.CPUThreads),
		TotalMemoryBytes: int64(info.TotalMemory),
		Os:            info.OS,
		OsVersion:     info.OSVersion,
		Architecture:  info.Architecture,
		XylonaVersion: info.XylonaVersion,
	}
}

func snapshotToProto(snap *sysinfo.ResourceSnapshot, gsCount, runningCount, userCount int) *xylona.NodeResourceSnapshot {
	return &xylona.NodeResourceSnapshot{
		CpuPercent:             snap.CPUPercent,
		MemoryPercent:          snap.MemoryPercent,
		MemoryUsedBytes:        int64(snap.MemoryUsed),
		MemoryTotalBytes:       int64(snap.MemoryTotal),
		DiskPercent:            snap.DiskPercent,
		DiskUsedBytes:          int64(snap.DiskUsed),
		DiskTotalBytes:         int64(snap.DiskTotal),
		GameServerCount:        int32(gsCount),
		RunningGameServerCount: int32(runningCount),
		UserCount:              int32(userCount),
		RecordedAt:             timestamppb.Now(),
	}
}

// GetNodeSystemInfo returns system hardware/OS info for a node.
func (xs XylonaService) GetNodeSystemInfo(ctx context.Context, request *connect.Request[xylona.GetNodeSystemInfoRequest]) (*connect.Response[xylona.GetNodeSystemInfoResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser access required"))
	}

	nodeID := request.Msg.GetNodeId()

	// Check if this is a remote node request.
	localNodeID, errLocal := xs.db.GetLocalNodeID()
	if errLocal != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local node ID"))
	}

	if nodeID != "" && nodeID != localNodeID {
		// Proxy to remote node via federation.
		node, errGetNode := xs.db.GetRemoteNodeByID(nodeID)
		if errGetNode != nil {
			if errors.Is(errGetNode, sql.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, errors.New("node not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get node"))
		}

		client, errClient := xs.newRemoteFederationClient(node)
		if errClient != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create federation client"))
		}

		resp, errRPC := client.FederationGetNodeSystemInfo(ctx, connect.NewRequest(&xylona.FederationGetNodeSystemInfoRequest{}))
		if errRPC != nil {
			log.Error().Err(errRPC).Str("node_id", nodeID).Msg("Failed to get remote node system info")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get remote node system info"))
		}

		return connect.NewResponse(&xylona.GetNodeSystemInfoResponse{
			SystemInfo: resp.Msg.GetSystemInfo(),
		}), nil
	}

	// Local node.
	info, errInfo := sysinfo.CollectSystemInfo()
	if errInfo != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to collect system info"))
	}

	return connect.NewResponse(&xylona.GetNodeSystemInfoResponse{
		SystemInfo: sysInfoToProto(info),
	}), nil
}

// GetNodeResourceSnapshot returns a live resource usage snapshot for a node.
func (xs XylonaService) GetNodeResourceSnapshot(ctx context.Context, request *connect.Request[xylona.GetNodeResourceSnapshotRequest]) (*connect.Response[xylona.GetNodeResourceSnapshotResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser access required"))
	}

	nodeID := request.Msg.GetNodeId()
	localNodeID, errLocal := xs.db.GetLocalNodeID()
	if errLocal != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local node ID"))
	}

	if nodeID != "" && nodeID != localNodeID {
		node, errGetNode := xs.db.GetRemoteNodeByID(nodeID)
		if errGetNode != nil {
			if errors.Is(errGetNode, sql.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, errors.New("node not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get node"))
		}

		client, errClient := xs.newRemoteFederationClient(node)
		if errClient != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create federation client"))
		}

		resp, errRPC := client.FederationGetNodeResourceSnapshot(ctx, connect.NewRequest(&xylona.FederationGetNodeResourceSnapshotRequest{}))
		if errRPC != nil {
			log.Error().Err(errRPC).Str("node_id", nodeID).Msg("Failed to get remote node resource snapshot")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get remote node resource snapshot"))
		}

		return connect.NewResponse(&xylona.GetNodeResourceSnapshotResponse{
			Snapshot: resp.Msg.GetSnapshot(),
		}), nil
	}

	snapshot, gsCount, runningCount, userCount, errCollect := xs.collectLocalSnapshot()
	if errCollect != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to collect resource snapshot"))
	}

	return connect.NewResponse(&xylona.GetNodeResourceSnapshotResponse{
		Snapshot: snapshotToProto(snapshot, gsCount, runningCount, userCount),
	}), nil
}

// GetDashboardOverview returns an overview of all nodes with live resource snapshots.
func (xs XylonaService) GetDashboardOverview(ctx context.Context, request *connect.Request[xylona.GetDashboardOverviewRequest]) (*connect.Response[xylona.GetDashboardOverviewResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser access required"))
	}

	var summaries []*xylona.DashboardNodeSummary

	// Local node.
	allNodes, errNodes := xs.db.GetAllNodes()
	if errNodes != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list nodes"))
	}

	for _, node := range allNodes {
		if node.IsLocal {
			info, errInfo := sysinfo.CollectSystemInfo()
			if errInfo != nil {
				log.Error().Err(errInfo).Msg("Failed to collect local system info")
				continue
			}

			snapshot, gsCount, runningCount, userCount, errSnap := xs.collectLocalSnapshot()
			if errSnap != nil {
				log.Error().Err(errSnap).Msg("Failed to collect local resource snapshot")
				continue
			}

			nodeProto := helpers.NodeModelToProto(node)
			// The local node never has its health_status set by the sync engine
			// (which only monitors remote nodes), so override it here.
			if nodeProto.HealthStatus == "" {
				nodeProto.HealthStatus = "healthy"
			}

			summaries = append(summaries, &xylona.DashboardNodeSummary{
				Node:       nodeProto,
				SystemInfo: sysInfoToProto(info),
				Snapshot:   snapshotToProto(snapshot, gsCount, runningCount, userCount),
			})
			continue
		}

		// Remote node — try to fetch live data, fall back to cached info.
		client, errClient := xs.newRemoteFederationClient(node)
		if errClient != nil {
			log.Warn().Err(errClient).Str("node_id", node.ID).Msg("Failed to create federation client for dashboard")
			summaries = append(summaries, &xylona.DashboardNodeSummary{
				Node: helpers.NodeModelToProto(node),
			})
			continue
		}

		sysResp, errSys := client.FederationGetNodeSystemInfo(ctx, connect.NewRequest(&xylona.FederationGetNodeSystemInfoRequest{}))
		var sysInfo *xylona.NodeSystemInfo
		if errSys == nil {
			sysInfo = sysResp.Msg.GetSystemInfo()
		} else {
			log.Debug().Err(errSys).Str("node_id", node.ID).Msg("Failed to fetch remote system info for dashboard")
		}

		snapResp, errSnap := client.FederationGetNodeResourceSnapshot(ctx, connect.NewRequest(&xylona.FederationGetNodeResourceSnapshotRequest{}))
		var snap *xylona.NodeResourceSnapshot
		if errSnap == nil {
			snap = snapResp.Msg.GetSnapshot()
		} else {
			log.Debug().Err(errSnap).Str("node_id", node.ID).Msg("Failed to fetch remote resource snapshot for dashboard")
		}

		summaries = append(summaries, &xylona.DashboardNodeSummary{
			Node:       helpers.NodeModelToProto(node),
			SystemInfo: sysInfo,
			Snapshot:   snap,
		})
	}

	return connect.NewResponse(&xylona.GetDashboardOverviewResponse{
		Nodes: summaries,
	}), nil
}

// GetNodeMetricsHistory returns historical metrics for a node.
func (xs XylonaService) GetNodeMetricsHistory(ctx context.Context, request *connect.Request[xylona.GetNodeMetricsHistoryRequest]) (*connect.Response[xylona.GetNodeMetricsHistoryResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser access required"))
	}

	nodeID := request.Msg.GetNodeId()
	since := request.Msg.GetSince().AsTime()
	until := request.Msg.GetUntil().AsTime()

	localNodeID, errLocal := xs.db.GetLocalNodeID()
	if errLocal != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local node ID"))
	}

	if nodeID != "" && nodeID != localNodeID {
		node, errGetNode := xs.db.GetRemoteNodeByID(nodeID)
		if errGetNode != nil {
			if errors.Is(errGetNode, sql.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, errors.New("node not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get node"))
		}

		client, errClient := xs.newRemoteFederationClient(node)
		if errClient != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create federation client"))
		}

		resp, errRPC := client.FederationGetNodeMetricsHistory(ctx, connect.NewRequest(&xylona.FederationGetNodeMetricsHistoryRequest{
			Since: request.Msg.GetSince(),
			Until: request.Msg.GetUntil(),
		}))
		if errRPC != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get remote node metrics history"))
		}

		return connect.NewResponse(&xylona.GetNodeMetricsHistoryResponse{
			Points: resp.Msg.GetPoints(),
		}), nil
	}

	rows, errQuery := xs.db.GetNodeMetricsHistory(localNodeID, since, until)
	if errQuery != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to query node metrics history"))
	}

	var points []*xylona.MetricsHistoryPoint
	for _, row := range rows {
		points = append(points, &xylona.MetricsHistoryPoint{
			Timestamp:      timestamppb.New(row.RecordedAt),
			CpuPercent:     row.CPUPercent,
			MemoryPercent:  row.MemoryPercent,
			DiskPercent:    row.DiskPercent,
			MemoryUsedBytes: row.MemoryUsedBytes,
			DiskUsedBytes:  row.DiskUsedBytes,
		})
	}

	return connect.NewResponse(&xylona.GetNodeMetricsHistoryResponse{
		Points: points,
	}), nil
}

// GetGameServerMetricsHistory returns historical metrics for a game server.
func (xs XylonaService) GetGameServerMetricsHistory(ctx context.Context, request *connect.Request[xylona.GetGameServerMetricsHistoryRequest]) (*connect.Response[xylona.GetGameServerMetricsHistoryResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	gameServerID := request.Msg.GetGameServerId()
	since := request.Msg.GetSince().AsTime()
	until := request.Msg.GetUntil().AsTime()

	return dispatchGameServerRequest(
		xs,
		gameServerID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GetGameServerMetricsHistoryResponse], error) {
			// Local server — check access: owner, superuser, or RBAC grant.
			if !user.SuperUser && gameServer.UserID != user.ID {
				allowed, errPerm := helpers.HasPermission(xs.db, user, gameServerID, gameServer.UserID, "game_server.metrics")
				if errPerm != nil {
					return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check permissions"))
				}
				if !allowed {
					return nil, connect.NewError(connect.CodePermissionDenied, errors.New("access denied"))
				}
			}

			return xs.queryLocalGameServerMetricsHistory(gameServerID, since, until)
		},
		func() (*connect.Response[xylona.GetGameServerMetricsHistoryResponse], error) {
			return xs.getRemoteGameServerMetricsHistory(ctx, gameServerID, since, until, user, request.Msg)
		},
	)
}

func (xs XylonaService) queryLocalGameServerMetricsHistory(gameServerID string, since, until time.Time) (*connect.Response[xylona.GetGameServerMetricsHistoryResponse], error) {
	rows, errQuery := xs.db.GetGameServerMetricsHistory(gameServerID, since, until)
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
			PlayerCount:    int32(row.PlayerCount),
		})
	}

	return connect.NewResponse(&xylona.GetGameServerMetricsHistoryResponse{
		Points: points,
	}), nil
}

func (xs XylonaService) getRemoteGameServerMetricsHistory(ctx context.Context, serverID string, since, until time.Time, actingUser *models.User, originalMsg *xylona.GetGameServerMetricsHistoryRequest) (*connect.Response[xylona.GetGameServerMetricsHistoryResponse], error) {
	peerNode, remoteCache, errGetPeer := xs.getRemoteNodeForServer(serverID)
	if errGetPeer != nil {
		return nil, errGetPeer
	}

	client, errClient := xs.newRemoteFederationClient(peerNode)
	if errClient != nil {
		log.Error().Err(errClient).Str("server_id", serverID).Msg("Failed to create federation client for game server metrics history")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create federation client"))
	}

	req := connect.NewRequest(&xylona.FederationGetGameServerMetricsHistoryRequest{
		GameServerId: remoteCache.RemoteServerID,
		Since:        originalMsg.GetSince(),
		Until:        originalMsg.GetUntil(),
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Msg("Failed to apply federation identity for game server metrics history")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve federation identity"))
	}

	resp, errRPC := client.FederationGetGameServerMetricsHistory(ctx, req)
	if errRPC != nil {
		log.Error().Err(errRPC).Str("server_id", serverID).Str("remote_server_id", remoteCache.RemoteServerID).Msg("Failed to get remote game server metrics history")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get remote game server metrics history"))
	}

	return connect.NewResponse(&xylona.GetGameServerMetricsHistoryResponse{
		Points: resp.Msg.GetPoints(),
	}), nil
}

func (xs XylonaService) collectLocalSnapshot() (*sysinfo.ResourceSnapshot, int, int, int, error) {
	snapshot, errSnap := sysinfo.CollectResourceSnapshot()
	if errSnap != nil {
		return nil, 0, 0, 0, errSnap
	}

	gsCount, errGS := xs.db.CountGameServers()
	if errGS != nil {
		log.Error().Err(errGS).Msg("Failed to count game servers")
	}

	// Use the supervisor to get the real running count rather than relying on
	// potentially stale DB status values. ListCommands() returns all commands
	// that have ever been started (including stopped ones still in the map),
	// so we must filter by status.
	runningCount := 0
	if xs.supervisorInst != nil {
		for _, cmd := range xs.supervisorInst.ListCommands() {
			if cmd.Status() == xylona.Status_ONLINE || cmd.Status() == xylona.Status_INSTALLING || cmd.Status() == xylona.Status_UPDATING {
				runningCount++
			}
		}
	}

	userCount, errUsers := xs.db.CountUsers()
	if errUsers != nil {
		log.Error().Err(errUsers).Msg("Failed to count users")
	}

	return snapshot, gsCount, runningCount, userCount, nil
}
