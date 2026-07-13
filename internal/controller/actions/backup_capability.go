package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	backupUnsupportedReason = "Backups are not supported for this game on the selected node platform."
	backupUnavailableReason = "Backup support could not be determined because the selected node is unavailable."
)

var (
	// ErrBackupsUnsupported reports that the selected game explicitly disables
	// backup creation on the owning node's operating system.
	ErrBackupsUnsupported = errors.New("backups are not supported for this game on the selected node platform")
	// ErrBackupCapabilityUnavailable reports that the owning node's operating
	// system could not be determined authoritatively.
	ErrBackupCapabilityUnavailable = errors.New("backup support could not be determined")
)

// BackupCapability describes whether new backup artifacts may be created for
// a game server. Historical backup management does not depend on this value.
type BackupCapability struct {
	Supported      bool
	DisabledReason string
}

// ResolveBackupCapability determines backup support using the controller OS
// for the embedded node and a fresh node snapshot for remote nodes.
func ResolveBackupCapability(
	ctx context.Context,
	database *db.Connection,
	registry *noderegistry.Registry,
	gameServer *models.GameServer,
	game *models.Game,
) (BackupCapability, error) {
	if gameServer == nil {
		return unavailableBackupCapability(errors.New("game server is nil"))
	}

	resolvedGame := game
	if resolvedGame == nil && gameServer.R.Game != nil {
		resolvedGame = gameServer.R.Game
	}
	if resolvedGame == nil {
		if database == nil {
			return unavailableBackupCapability(errors.New("database is unavailable"))
		}
		var errGetGame error
		resolvedGame, errGetGame = database.GetGameByID(gameServer.GameID)
		if errGetGame != nil {
			return unavailableBackupCapability(fmt.Errorf("load game definition: %w", errGetGame))
		}
	}

	nodeOS := OperatingSystem
	if registry != nil && gameServer.NodeID != registry.SelfID() {
		client, errClient := registry.Get(gameServer.NodeID)
		if errClient != nil {
			return unavailableBackupCapability(fmt.Errorf("resolve node client: %w", errClient))
		}

		snapshotCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		snapshot, errSnapshot := client.GetNodeSnapshot(snapshotCtx)
		if errSnapshot != nil {
			return unavailableBackupCapability(fmt.Errorf("get node snapshot: %w", errSnapshot))
		}
		if snapshot == nil {
			return unavailableBackupCapability(errors.New("node snapshot is nil"))
		}

		var detected bool
		nodeOS, detected = detectOperatingSystem(strings.ToLower(strings.TrimSpace(snapshot.OS)))
		if !detected {
			return unavailableBackupCapability(fmt.Errorf("unsupported node operating system %q", snapshot.OS))
		}
	}

	var supported bool
	switch nodeOS {
	case Windows:
		supported = resolvedGame.WindowsAllowBackups
	case Linux, Darwin:
		supported = resolvedGame.LinuxAllowBackups
	default:
		return unavailableBackupCapability(fmt.Errorf("unsupported controller operating system %q", nodeOS))
	}
	if !supported {
		return BackupCapability{
			Supported:      false,
			DisabledReason: backupUnsupportedReason,
		}, nil
	}

	return BackupCapability{Supported: true}, nil
}

func unavailableBackupCapability(cause error) (BackupCapability, error) {
	return BackupCapability{
		Supported:      false,
		DisabledReason: backupUnavailableReason,
	}, fmt.Errorf("%w: %w", ErrBackupCapabilityUnavailable, cause)
}

func (inst *Instance) resolveBackupCapability(ctx context.Context, gameServer *models.GameServer) (BackupCapability, error) {
	if inst == nil {
		return unavailableBackupCapability(errors.New("actions instance is nil"))
	}

	return ResolveBackupCapability(ctx, inst.db, inst.nodeRegistry, gameServer, nil)
}
