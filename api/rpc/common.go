package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
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
		// Use the controller's own OS to pick the matching base command.
		// In a homogeneous deployment this matches the node; heterogeneous
		// deployments see a reduced node-side check but the executable
		// check (which is OS-agnostic) still applies.
		if runtime.GOOS == "windows" {
			policy.BaseCommand = game.WindowsBaseCommand
		} else {
			policy.BaseCommand = game.LinuxBaseCommand
		}
	}
	return policy
}

func (xs *XylonaService) getLocalGameServerStatus(gameServer *models.GameServer) xylona.Status {
	// Route through the owning node's NodeClient so embedded and remote
	// servers both surface status uniformly. Falls back to the DB's
	// stored status when the node isn't reachable.
	if xs.nodeRegistry != nil {
		client, errGet := xs.nodeRegistry.Get(gameServer.NodeID)
		if errGet == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			snap, found, errSnap := client.GetProcessSnapshot(ctx, gameServer.ID)
			if errSnap == nil {
				if !found {
					return xylona.Status_OFFLINE
				}
				statusVal, ok := xylona.Status_value[snap.Status]
				if ok {
					return xylona.Status(statusVal)
				}
				return xylona.Status_UNKNOWN
			}
		}
	}

	if xs.supervisorInst != nil {
		gameServerCmd, errGetCommand := xs.supervisorInst.GetCommandByID(gameServer.ID)
		if errGetCommand != nil {
			return xylona.Status_OFFLINE
		}
		return gameServerCmd.Status()
	}

	status, ok := xylona.Status_value[gameServer.Status]
	if !ok {
		return xylona.Status_UNKNOWN
	}

	return xylona.Status(status)
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
