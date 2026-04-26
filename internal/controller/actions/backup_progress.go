package actions

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

type backupProgressReporter struct {
	inst           *Instance
	gameServerID   string
	gameServerName string
	backupID       string
	mu             sync.Mutex
	latestBytes    int64
	flushedBytes   int64
	closed         bool
	flushOnStop    bool
	stopCh         chan struct{}
	doneCh         chan struct{}
}

func newBackupProgressReporter(
	inst *Instance,
	gameServerID string,
	gameServerName string,
	backupID string,
) *backupProgressReporter {
	reporter := &backupProgressReporter{
		inst:           inst,
		gameServerID:   gameServerID,
		gameServerName: gameServerName,
		backupID:       backupID,
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
	}

	go reporter.run()

	return reporter
}

func (r *backupProgressReporter) Observe(sizeBytes int64) {
	if sizeBytes <= 0 {
		return
	}

	r.mu.Lock()
	if sizeBytes > r.latestBytes {
		r.latestBytes = sizeBytes
	}
	r.mu.Unlock()
}

func (r *backupProgressReporter) Close(finalSizeBytes int64) {
	r.shutdown(true, finalSizeBytes)
}

func (r *backupProgressReporter) Abort() {
	r.shutdown(false, 0)
}

func (r *backupProgressReporter) shutdown(flush bool, finalSizeBytes int64) {
	if r == nil {
		return
	}

	if flush {
		r.Observe(finalSizeBytes)
	}

	r.mu.Lock()
	if r.closed {
		doneCh := r.doneCh
		r.mu.Unlock()
		<-doneCh
		return
	}
	r.closed = true
	r.flushOnStop = flush
	close(r.stopCh)
	doneCh := r.doneCh
	r.mu.Unlock()

	<-doneCh
}

func (r *backupProgressReporter) run() {
	ticker := time.NewTicker(backupProgressUpdateInterval)
	defer ticker.Stop()
	defer close(r.doneCh)

	for {
		select {
		case <-ticker.C:
			r.flush()
		case <-r.stopCh:
			if r.shouldFlushOnStop() {
				r.flush()
			}
			return
		}
	}
}

func (r *backupProgressReporter) shouldFlushOnStop() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.flushOnStop
}

func (r *backupProgressReporter) flush() {
	r.mu.Lock()
	latestBytes := r.latestBytes
	if latestBytes <= r.flushedBytes {
		r.mu.Unlock()
		return
	}
	r.flushedBytes = latestBytes
	r.mu.Unlock()

	_, errUpdate := updateGameServerBackupProgress(r.inst.db, r.backupID, latestBytes)
	if errUpdate != nil {
		log.Error().
			Err(errUpdate).
			Str("backup_id", r.backupID).
			Msg("Failed to persist in-flight backup progress")
	}

	r.inst.broadcastBackupProgress(
		r.gameServerID,
		r.gameServerName,
		r.backupID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_ARCHIVING,
		50,
		latestBytes,
		"Archiving game server files",
	)
}

func backupCreateCancelErr(backupCtx context.Context) error {
	if backupCtx == nil {
		return nil
	}

	errCtx := backupCtx.Err()
	if errCtx != nil {
		return errors.Join(errBackupCreateCancelled, errCtx)
	}

	return nil
}

func (inst *Instance) registerBackupCreate(backupID string) (context.Context, func()) {
	if inst == nil {
		return context.Background(), nil
	}

	backupCtx, cancelBackup := context.WithCancel(inst.ctx)
	call := &backupCreateCall{
		cancel: cancelBackup,
		done:   make(chan struct{}),
	}

	inst.backupCreateMu.Lock()
	inst.backupCreateCalls[backupID] = call
	inst.backupCreateMu.Unlock()

	return backupCtx, func() {
		cancelBackup()
		inst.backupCreateMu.Lock()
		delete(inst.backupCreateCalls, backupID)
		close(call.done)
		inst.backupCreateMu.Unlock()
	}
}

func (inst *Instance) cancelBackupCreate(backupID string) <-chan struct{} {
	if inst == nil {
		return nil
	}

	inst.backupCreateMu.Lock()
	call := inst.backupCreateCalls[backupID]
	inst.backupCreateMu.Unlock()
	if call == nil {
		return nil
	}

	call.cancel()

	return call.done
}

func (inst *Instance) broadcastBackupProgress(
	gameServerID string,
	gameServerName string,
	backupID string,
	operation xylona.BackupProgressOperation,
	phase xylona.BackupProgressPhase,
	percent int32,
	sizeBytes int64,
	message string,
) {
	if inst == nil || inst.backupBroadcaster == nil {
		return
	}

	inst.backupBroadcaster.BroadcastBackupProgress(gameServerID, &xylona.BackupProgress{
		GameServerId:   gameServerID,
		GameServerName: gameServerName,
		BackupId:       backupID,
		Operation:      operation,
		Phase:          phase,
		Percent:        percent,
		SizeBytes:      sizeBytes,
		Message:        message,
	})
}
