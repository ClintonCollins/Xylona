package actions

import (
	"time"

	"github.com/rs/zerolog/log"
)

// historyPruner is the subset of db.Connection needed by the alert history
// pruner. db.Connection satisfies this interface automatically.
type historyPruner interface {
	PruneAlertHistory(olderThan time.Time) (int64, error)
}

// pruneAlertHistoryOnce deletes alert history records older than 90 days and
// returns the number of deleted rows.
func pruneAlertHistoryOnce(store historyPruner) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -90)
	n, errPrune := store.PruneAlertHistory(cutoff)
	if errPrune != nil {
		log.Error().Err(errPrune).Msg("Alert history pruner: failed to prune records")
		return 0, errPrune
	}
	if n > 0 {
		log.Info().Int64("deleted", n).Msg("Alert history pruner: pruned old records")
	}
	return n, nil
}

// backgroundJobAlertHistoryPruner runs daily and removes alert history records
// older than 90 days. It also runs once immediately on startup before entering
// the periodic loop so that stale records are cleaned up without waiting a
// full day.
func (inst *Instance) backgroundJobAlertHistoryPruner() {
	_, errPrune := pruneAlertHistoryOnce(inst.db)
	if errPrune != nil {
		log.Error().Err(errPrune).Msg("Alert history pruner: startup prune failed")
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-inst.ctx.Done():
			return
		case <-ticker.C:
			_, errTick := pruneAlertHistoryOnce(inst.db)
			if errTick != nil {
				log.Error().Err(errTick).Msg("Alert history pruner: periodic prune failed")
			}
		}
	}
}
