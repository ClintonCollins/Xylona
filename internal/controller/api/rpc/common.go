package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/internal/controller/protomap"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func isSQLiteUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

func isSQLiteForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	errLower := strings.ToLower(err.Error())
	return strings.Contains(errLower, "foreign key constraint failed")
}

func contextConnectCode(err error) connect.Code {
	if errors.Is(err, context.Canceled) {
		return connect.CodeCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.CodeDeadlineExceeded
	}
	return connect.CodeUnknown
}

func (xs *XylonaService) getUserFromHeader(header http.Header) (*models.User, error) {
	sessionCookies, errGetSession := gatekeeper.GetSessionFromHeader(header)
	if errGetSession != nil {
		log.Debug().Err(errGetSession).Msg("Error getting session")
		return nil, fmt.Errorf("rpc: get session from header: %w", errGetSession)
	}

	user, errGetUser := gatekeeper.GetUserFromSession(sessionCookies.SessionID, sessionCookies.SessionToken, xs.db, xs.secureCookie)
	if errGetUser != nil {
		log.Debug().Err(errGetUser).Msg("Error getting user")
		return nil, fmt.Errorf("rpc: get user from session: %w", errGetUser)
	}
	return user, nil
}

func (xs *XylonaService) getGameServerFromID(gameServerID string) (*models.GameServer, error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(gameServerID)
	if errGetGameServer != nil {
		return nil, dbLookup(errGetGameServer)
	}
	return gameServer, nil
}

// resolveNodeClient returns the NodeClient registered for the game server's
// owning node. Returns a connect.CodeFailedPrecondition error when the node
// is not reachable (not registered yet, or offline).
func (xs *XylonaService) resolveNodeClient(gameServer *models.GameServer) (nodeclient.NodeClient, error) {
	if xs.nodeRegistry == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node registry unavailable"))
	}
	client, errGet := xs.nodeRegistry.Get(gameServer.NodeID)
	if errGet != nil {
		if errors.Is(errGet, noderegistry.ErrNodeNotRegistered) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("node %q is not currently reachable", gameServer.NodeID))
		}
		log.Error().Err(errGet).Str("node_id", gameServer.NodeID).Msg("failed to resolve node client")
		return nil, internalErrf("failed to resolve node client")
	}
	return client, nil
}

// buildProtectionPolicy constructs a node.ProtectionPolicy from the game's
// configured server executable and platform-appropriate base command. Passed
// to node-side write operations so the node can re-run the controller's
// IsProtectedServerPath check. Returns a zero-value policy when the game
// definition is missing — in that case the node simply skips the check.
func (xs *XylonaService) buildProtectionPolicy(gameServer *models.GameServer) node.ProtectionPolicy {
	if gameServer == nil {
		return node.ProtectionPolicy{}
	}

	policy := node.ProtectionPolicy{
		ServerExecutable: gameServer.ServerExecutable.GetOr(""),
	}
	if gameServer.R.Game != nil {
		game := gameServer.R.Game
		nodeOS := xs.resolveNodeGOOS(gameServer.NodeID)
		if nodeOS == "windows" {
			policy.BaseCommand = game.WindowsBaseCommand
		} else {
			policy.BaseCommand = game.LinuxBaseCommand
		}
	}
	return policy
}

func (xs *XylonaService) resolveNodeGOOS(nodeID string) string {
	nodeOS, errResolve := xs.resolveNodeGOOSRequired(nodeID)
	if errResolve != nil {
		return runtime.GOOS
	}
	return nodeOS
}

func (xs *XylonaService) resolveNodeGOOSRequired(nodeID string) (string, error) {
	if xs == nil {
		return "", errors.New("rpc: service is nil")
	}
	if xs.nodeRegistry == nil {
		return runtime.GOOS, nil
	}
	targetID := strings.TrimSpace(nodeID)
	selfID := strings.TrimSpace(xs.nodeRegistry.SelfID())
	if targetID == "" || targetID == selfID {
		return runtime.GOOS, nil
	}

	client, errGetClient := xs.nodeRegistry.Get(nodeID)
	if errGetClient != nil {
		return "", fmt.Errorf("rpc: get node %q for operating system: %w", nodeID, errGetClient)
	}

	baseCtx := context.Background()
	if xs.ctx != nil {
		baseCtx = xs.ctx
	}
	snapCtx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	snapshot, errSnapshot := client.GetNodeSnapshot(snapCtx)
	if errSnapshot != nil {
		return "", fmt.Errorf("rpc: get node %q operating system: %w", nodeID, errSnapshot)
	}
	if snapshot == nil {
		return "", fmt.Errorf("rpc: get node %q operating system: empty snapshot", nodeID)
	}

	nodeOS := strings.ToLower(strings.TrimSpace(snapshot.OS))
	switch nodeOS {
	case "windows", "linux", "darwin":
		return nodeOS, nil
	default:
		return "", fmt.Errorf("rpc: node %q reported unsupported operating system %q", nodeID, snapshot.OS)
	}
}

func (xs *XylonaService) selfNodeID() string {
	if xs == nil {
		return ""
	}
	if xs.nodeRegistry != nil {
		nodeID := strings.TrimSpace(xs.nodeRegistry.SelfID())
		if nodeID != "" {
			return nodeID
		}
	}
	if xs.db == nil {
		return ""
	}
	localSettings, errSettings := xs.db.GetLocalSettings()
	if errSettings != nil {
		return ""
	}
	return strings.TrimSpace(localSettings.NodeID)
}

type nodeRuntimeState struct {
	snapshot           *node.NodeSnapshot
	updateCapabilities *node.UpdateCapabilities
}

type gameServerNodeSnapshotState struct {
	processes  map[string]*node.ProcessSnapshot
	err        error
	observedAt time.Time
}

func (xs *XylonaService) nodeSnapshotBaseContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	if xs != nil && xs.ctx != nil {
		return xs.ctx
	}
	return context.Background()
}

// collectGameServerNodeSnapshots obtains one bounded snapshot per owning node
// concurrently. Callers can then fan process state out to every server on the
// node without turning one unavailable node into a per-server timeout.
func (xs *XylonaService) collectGameServerNodeSnapshots(
	ctx context.Context,
	gameServers []*models.GameServer,
) map[string]gameServerNodeSnapshotState {
	states := make(map[string]gameServerNodeSnapshotState)
	nodeClients := make(map[string]nodeclient.NodeClient)
	selfNodeID := xs.selfNodeID()

	for _, gameServer := range gameServers {
		if gameServer == nil {
			continue
		}
		nodeID := strings.TrimSpace(gameServer.NodeID)
		if nodeID == "" {
			nodeID = selfNodeID
		}
		if nodeID == "" {
			continue
		}
		_, exists := states[nodeID]
		if exists {
			continue
		}
		if xs == nil || xs.nodeRegistry == nil {
			states[nodeID] = gameServerNodeSnapshotState{
				err: errors.New("node registry unavailable"),
			}
			continue
		}
		client, errClient := xs.nodeRegistry.Get(nodeID)
		if errClient != nil {
			states[nodeID] = gameServerNodeSnapshotState{
				err: fmt.Errorf("resolve node %q: %w", nodeID, errClient),
			}
			continue
		}
		states[nodeID] = gameServerNodeSnapshotState{}
		nodeClients[nodeID] = client
	}

	baseCtx := xs.nodeSnapshotBaseContext(ctx)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for nodeID, client := range nodeClients {
		wg.Add(1)
		go func(nodeID string, client nodeclient.NodeClient) {
			defer wg.Done()

			snapshotCtx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
			defer cancel()
			snapshot, errSnapshot := client.GetNodeSnapshot(snapshotCtx)
			state := gameServerNodeSnapshotState{}
			switch {
			case errSnapshot != nil:
				state.err = fmt.Errorf("get node %q snapshot: %w", nodeID, errSnapshot)
			case snapshot == nil:
				state.err = fmt.Errorf("get node %q snapshot: empty response", nodeID)
			default:
				state.observedAt = snapshot.Collected
				if state.observedAt.IsZero() {
					state.observedAt = time.Now().UTC()
				}
				state.processes = make(map[string]*node.ProcessSnapshot, len(snapshot.Processes))
				for i := range snapshot.Processes {
					processSnapshot := &snapshot.Processes[i]
					state.processes[processSnapshot.ID] = processSnapshot
				}
			}

			mu.Lock()
			states[nodeID] = state
			mu.Unlock()
		}(nodeID, client)
	}
	wg.Wait()
	return states
}

func (xs *XylonaService) gameServerNodeSnapshotState(
	gameServer *models.GameServer,
	states map[string]gameServerNodeSnapshotState,
) gameServerNodeSnapshotState {
	if gameServer == nil {
		return gameServerNodeSnapshotState{err: errors.New("game server is nil")}
	}
	nodeID := strings.TrimSpace(gameServer.NodeID)
	if nodeID == "" {
		nodeID = xs.selfNodeID()
	}
	state, ok := states[nodeID]
	if !ok {
		return gameServerNodeSnapshotState{
			err: fmt.Errorf("node %q snapshot unavailable", nodeID),
		}
	}
	return state
}

func processStateFromNodeSnapshot(
	gameServerID string,
	state gameServerNodeSnapshotState,
) (xylona.Status, *node.ProcessSnapshot, error) {
	if state.err != nil {
		return xylona.Status_UNKNOWN, nil, state.err
	}
	processSnapshot, found := state.processes[gameServerID]
	if !found || processSnapshot == nil {
		return xylona.Status_OFFLINE, nil, nil
	}
	statusValue, ok := xylona.Status_value[processSnapshot.Status]
	if !ok {
		return xylona.Status_UNKNOWN, processSnapshot, nil
	}
	return xylona.Status(statusValue), processSnapshot, nil
}

func (xs *XylonaService) collectNodeRuntimeState(ctx context.Context, nodeRows []*models.Node) map[string]nodeRuntimeState {
	states := make(map[string]nodeRuntimeState, len(nodeRows))
	if xs == nil || xs.nodeRegistry == nil {
		return states
	}

	baseCtx := xs.nodeSnapshotBaseContext(ctx)

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, nodeRow := range nodeRows {
		if nodeRow == nil {
			continue
		}
		if !nodeRow.Enabled {
			continue
		}
		nodeID := strings.TrimSpace(nodeRow.ID)
		if nodeID == "" {
			continue
		}

		client, errGet := xs.nodeRegistry.Get(nodeID)
		if errGet != nil {
			continue
		}

		wg.Add(1)
		go func(nodeID string, client nodeclient.NodeClient) {
			defer wg.Done()

			snapCtx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
			defer cancel()

			snapshot, errSnapshot := client.GetNodeSnapshot(snapCtx)

			var capsPtr *node.UpdateCapabilities
			capsCtx, capsCancel := context.WithTimeout(baseCtx, 2*time.Second)
			caps, errCaps := client.GetUpdateCapabilities(capsCtx)
			capsCancel()
			if errCaps == nil {
				capsPtr = &caps
			}

			if errSnapshot != nil || snapshot == nil {
				if capsPtr == nil {
					return
				}
			}
			mu.Lock()
			states[nodeID] = nodeRuntimeState{snapshot: snapshot, updateCapabilities: capsPtr}
			mu.Unlock()
		}(nodeID, client)
	}

	wg.Wait()
	return states
}

func (xs *XylonaService) nodeProtoWithRuntimeState(
	nodeRow *models.Node,
	selfNodeID string,
	runtimeState map[string]nodeRuntimeState,
) *xylona.Node {
	if nodeRow == nil {
		return &xylona.Node{}
	}

	proto := protomap.NodeModelToProto(nodeRow)
	isLocal := strings.TrimSpace(nodeRow.ID) != "" && nodeRow.ID == selfNodeID
	proto.Local = isLocal

	if !nodeRow.Enabled {
		proto.HealthStatus = "disabled"
		if isLocal {
			proto.Os = runtime.GOOS
		}
		return proto
	}

	if xs == nil || xs.nodeRegistry == nil {
		if isLocal {
			proto.HealthStatus = "healthy"
			proto.Os = runtime.GOOS
		} else {
			proto.HealthStatus = "offline"
		}
		return proto
	}

	state, ok := runtimeState[nodeRow.ID]
	if !ok || state.snapshot == nil {
		proto.HealthStatus = "offline"
		if isLocal {
			proto.Os = runtime.GOOS
		}
		return proto
	}

	proto.HealthStatus = "healthy"
	nodeOS := strings.ToLower(strings.TrimSpace(state.snapshot.OS))
	if nodeOS == "" && isLocal {
		nodeOS = runtime.GOOS
	}
	proto.Os = nodeOS
	proto.Version = state.snapshot.XylonaVersion
	if state.updateCapabilities != nil {
		proto.ProtocolVersion = state.updateCapabilities.ProtocolVersion
		if state.updateCapabilities.Supported {
			proto.Capabilities = "self-update"
		} else {
			proto.Capabilities = strings.TrimSpace(state.updateCapabilities.Reason)
		}
	}

	return proto
}

func (xs *XylonaService) nodeProtoWithRuntime(ctx context.Context, nodeRow *models.Node) *xylona.Node {
	selfNodeID := xs.selfNodeID()
	runtimeState := xs.collectNodeRuntimeState(ctx, []*models.Node{nodeRow})
	return xs.nodeProtoWithRuntimeState(nodeRow, selfNodeID, runtimeState)
}

func (xs *XylonaService) getLocalGameServerStatus(ctx context.Context, gameServer *models.GameServer) xylona.Status {
	// Route through the owning node's NodeClient so embedded and remote
	// servers both surface status uniformly. Only a successful lookup with no
	// tracked process is authoritative OFFLINE.
	if xs.nodeRegistry == nil || gameServer == nil {
		return xylona.Status_UNKNOWN
	}
	client, errGet := xs.nodeRegistry.Get(gameServer.NodeID)
	if errGet != nil {
		return xylona.Status_UNKNOWN
	}
	baseCtx := xs.nodeSnapshotBaseContext(ctx)
	snapshotCtx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()
	snap, found, errSnap := client.GetProcessSnapshot(snapshotCtx, gameServer.ID)
	if errSnap != nil {
		return xylona.Status_UNKNOWN
	}
	if !found || snap == nil {
		return xylona.Status_OFFLINE
	}
	statusVal, ok := xylona.Status_value[snap.Status]
	if !ok {
		return xylona.Status_UNKNOWN
	}
	return xylona.Status(statusVal)
}

// applyProcessMetricsToProto fills the per-process metric fields on a
// GameServer proto from a node.ProcessSnapshot. Used by GetGameServer and
// ListGameServers so per-node metrics render identically. When snap is nil
// the fields are zero-valued (server offline).
func applyProcessMetricsToProto(dst *xylona.GameServer, snap *node.ProcessSnapshot) {
	if dst == nil || snap == nil {
		return
	}
	dst.CpuPercent = int64(snap.CPUPercent)
	dst.MemoryBytes = helpers.ClampInt64FromUint64(snap.MemoryVMS)
	dst.MemoryWorkingSetBytes = helpers.ClampInt64FromUint64(snap.MemoryRSS)
	dst.MemoryPercent = float64(snap.MemoryPercent)
	dst.CpuCores = snap.CPUCores
	dst.NumberOfThreads = int64(snap.NumThreads)
	dst.DiskUsageBytes = helpers.ClampInt64FromUint64(snap.DiskUsageBytes)
	dst.IoReadRate = snap.IOReadRate
	dst.IoWriteRate = snap.IOWriteRate
	dst.ConnectionCount = snap.ConnectionCount
	if snap.UnixStartedAt > 0 {
		dst.UptimeSeconds = time.Now().Unix() - snap.UnixStartedAt
	}
}

// resolveProcessSnapshot looks up the current process snapshot for a game
// server via the NodeClient. Returns (nil, error) on transport failure,
// (nil, nil) when the process isn't running.
//
//nolint:nilnil // (nil, nil) intentionally signals "process not tracked" to callers.
func (xs *XylonaService) resolveProcessSnapshot(ctx context.Context, gameServer *models.GameServer) (*node.ProcessSnapshot, error) {
	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	snap, found, errSnap := client.GetProcessSnapshot(ctx, gameServer.ID)
	if errSnap != nil {
		return nil, fmt.Errorf("rpc: get process snapshot: %w", errSnap)
	}
	if !found {
		return nil, nil
	}
	return snap, nil
}

// resolveGameServerRuntimeState returns the live runtime status for a game
// server. A successful lookup with no tracked process is authoritative
// OFFLINE. Transport and node-resolution failures remain UNKNOWN so callers
// cannot mistake an unavailable node for a stopped server.
func (xs *XylonaService) resolveGameServerRuntimeState(ctx context.Context, gameServer *models.GameServer) (xylona.Status, *node.ProcessSnapshot, error) {
	if gameServer == nil {
		return xylona.Status_UNKNOWN, nil, nil
	}

	snap, errSnap := xs.resolveProcessSnapshot(ctx, gameServer)
	if errSnap != nil {
		return xylona.Status_UNKNOWN, nil, errSnap
	}
	if snap == nil {
		return xylona.Status_OFFLINE, nil, nil
	}

	statusValue, ok := xylona.Status_value[snap.Status]
	if !ok {
		return xylona.Status_UNKNOWN, snap, nil
	}
	return xylona.Status(statusValue), snap, nil
}
