package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestCreateGameServerBackup(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-backup-create.sqlite")
	seedRBACFixture(t, conn)

	completedAt := time.Date(2026, 4, 1, 12, 45, 0, 0, time.UTC)

	backup, errCreate := conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     "/backups/server-local-1/manual-2026-04-01.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       4096,
		RetentionExempt: true,
		ErrorMessage:    "",
		CreatedAt:       time.Time{},
		CompletedAt:     &completedAt,
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreate)
	}

	if backup.ID == "" {
		t.Fatal("expected backup to have an ID")
	}
	if backup.GameServerID != "server-local-1" {
		t.Errorf("CreateGameServerBackup().GameServerID = %q, want %q", backup.GameServerID, "server-local-1")
	}
	if backup.NodeID != "node-local" {
		t.Errorf("CreateGameServerBackup().NodeID = %q, want %q", backup.NodeID, "node-local")
	}
	createdBy, createdBySet := backup.CreatedBy.Get()
	if !createdBySet || createdBy != "user-owner" {
		t.Errorf("CreateGameServerBackup().CreatedBy = (%q, %v), want (%q, true)", createdBy, createdBySet, "user-owner")
	}
	if backup.TriggerSource != "manual" {
		t.Errorf("CreateGameServerBackup().TriggerSource = %q, want %q", backup.TriggerSource, "manual")
	}
	if backup.ArchivePath != "/backups/server-local-1/manual-2026-04-01.zip" {
		t.Errorf("CreateGameServerBackup().ArchivePath = %q, want %q", backup.ArchivePath, "/backups/server-local-1/manual-2026-04-01.zip")
	}
	if backup.ArchiveFormat != "zip" {
		t.Errorf("CreateGameServerBackup().ArchiveFormat = %q, want %q", backup.ArchiveFormat, "zip")
	}
	if backup.Status != "completed" {
		t.Errorf("CreateGameServerBackup().Status = %q, want %q", backup.Status, "completed")
	}
	if backup.SizeBytes != 4096 {
		t.Errorf("CreateGameServerBackup().SizeBytes = %d, want %d", backup.SizeBytes, 4096)
	}
	if !backup.RetentionExempt {
		t.Error("CreateGameServerBackup().RetentionExempt = false, want true")
	}
	if !backup.CreatedAt.Equal(backup.CreatedAt.UTC()) {
		t.Errorf("CreateGameServerBackup().CreatedAt location = %v, want UTC", backup.CreatedAt.Location())
	}
	if time.Since(backup.CreatedAt) > 5*time.Second {
		t.Errorf("CreateGameServerBackup().CreatedAt = %v, want recent timestamp", backup.CreatedAt)
	}
	gotCompletedAt, completedAtSet := backup.CompletedAt.Get()
	if !completedAtSet || !gotCompletedAt.Equal(completedAt) {
		t.Errorf("CreateGameServerBackup().CompletedAt = (%v, %v), want (%v, true)", gotCompletedAt, completedAtSet, completedAt)
	}

	stored, errGet := conn.GetGameServerBackupByID(backup.ID)
	if errGet != nil {
		t.Fatalf("GetGameServerBackupByID() error = %v", errGet)
	}
	if stored.ID != backup.ID {
		t.Errorf("GetGameServerBackupByID().ID = %q, want %q", stored.ID, backup.ID)
	}
}

func TestListGameServerBackupsByGameServerIDNewestFirst(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-backup-list.sqlite")
	seedRBACFixture(t, conn)

	oldest := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	middle := oldest.Add(1 * time.Hour)
	newest := oldest.Add(2 * time.Hour)

	_, errCreate := conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "scheduled",
		ArchivePath:     "/backups/server-local-1/oldest.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       100,
		RetentionExempt: false,
		CreatedAt:       oldest,
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(oldest) error = %v", errCreate)
	}

	_, errCreate = conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "scheduled",
		ArchivePath:     "/backups/server-local-1/middle.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       200,
		RetentionExempt: false,
		CreatedAt:       middle,
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(middle) error = %v", errCreate)
	}

	_, errCreate = conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     "/backups/server-local-1/newest.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       300,
		RetentionExempt: true,
		CreatedAt:       newest,
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(newest) error = %v", errCreate)
	}

	backups, errList := conn.ListGameServerBackupsByGameServerID("server-local-1")
	if errList != nil {
		t.Fatalf("ListGameServerBackupsByGameServerID() error = %v", errList)
	}
	if len(backups) != 3 {
		t.Fatalf("ListGameServerBackupsByGameServerID() len = %d, want 3", len(backups))
	}
	if backups[0].ArchivePath != "/backups/server-local-1/newest.zip" {
		t.Errorf("backups[0].ArchivePath = %q, want %q", backups[0].ArchivePath, "/backups/server-local-1/newest.zip")
	}
	if backups[1].ArchivePath != "/backups/server-local-1/middle.zip" {
		t.Errorf("backups[1].ArchivePath = %q, want %q", backups[1].ArchivePath, "/backups/server-local-1/middle.zip")
	}
	if backups[2].ArchivePath != "/backups/server-local-1/oldest.zip" {
		t.Errorf("backups[2].ArchivePath = %q, want %q", backups[2].ArchivePath, "/backups/server-local-1/oldest.zip")
	}
}

func TestUpdateGameServerBackupResult(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-backup-update.sqlite")
	seedRBACFixture(t, conn)

	createdAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	backup, errCreate := conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "scheduled",
		ArchivePath:     "/backups/server-local-1/pending.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "pending",
		SizeBytes:       0,
		RetentionExempt: false,
		CreatedAt:       createdAt,
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreate)
	}

	completedAt := createdAt.Add(15 * time.Minute)
	updated, errUpdate := conn.UpdateGameServerBackupResult(backup.ID, UpdateGameServerBackupResultParams{
		Status:       "failed",
		SizeBytes:    8192,
		ErrorMessage: "disk full",
		CompletedAt:  &completedAt,
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerBackupResult() error = %v", errUpdate)
	}

	if updated.Status != "failed" {
		t.Errorf("UpdateGameServerBackupResult().Status = %q, want %q", updated.Status, "failed")
	}
	if updated.SizeBytes != 8192 {
		t.Errorf("UpdateGameServerBackupResult().SizeBytes = %d, want %d", updated.SizeBytes, 8192)
	}
	errorMessage, errorMessageSet := updated.ErrorMessage.Get()
	if !errorMessageSet || errorMessage != "disk full" {
		t.Errorf("UpdateGameServerBackupResult().ErrorMessage = (%q, %v), want (%q, true)", errorMessage, errorMessageSet, "disk full")
	}
	gotCompletedAt, completedAtSet := updated.CompletedAt.Get()
	if !completedAtSet || !gotCompletedAt.Equal(completedAt) {
		t.Errorf("UpdateGameServerBackupResult().CompletedAt = (%v, %v), want (%v, true)", gotCompletedAt, completedAtSet, completedAt)
	}

	stored, errGet := conn.GetGameServerBackupByID(backup.ID)
	if errGet != nil {
		t.Fatalf("GetGameServerBackupByID() error = %v", errGet)
	}
	if stored.Status != "failed" {
		t.Errorf("stored.Status = %q, want %q", stored.Status, "failed")
	}
	if stored.SizeBytes != 8192 {
		t.Errorf("stored.SizeBytes = %d, want %d", stored.SizeBytes, 8192)
	}
	storedErrorMessage, storedErrorMessageSet := stored.ErrorMessage.Get()
	if !storedErrorMessageSet || storedErrorMessage != "disk full" {
		t.Errorf("stored.ErrorMessage = (%q, %v), want (%q, true)", storedErrorMessage, storedErrorMessageSet, "disk full")
	}
	storedCompletedAt, storedCompletedAtSet := stored.CompletedAt.Get()
	if !storedCompletedAtSet || !storedCompletedAt.Equal(completedAt) {
		t.Errorf("stored.CompletedAt = (%v, %v), want (%v, true)", storedCompletedAt, storedCompletedAtSet, completedAt)
	}
}

func TestUpdateGameServerBackupProgress(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-backup-progress.sqlite")
	seedRBACFixture(t, conn)

	createdAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	backup, errCreate := conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     "/backups/server-local-1/pending.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "pending",
		SizeBytes:       0,
		RetentionExempt: true,
		CreatedAt:       createdAt,
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreate)
	}

	updated, errUpdate := conn.UpdateGameServerBackupProgress(backup.ID, 4096)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerBackupProgress() error = %v", errUpdate)
	}
	if updated.Status != "pending" {
		t.Errorf("UpdateGameServerBackupProgress().Status = %q, want %q", updated.Status, "pending")
	}
	if updated.SizeBytes != 4096 {
		t.Errorf("UpdateGameServerBackupProgress().SizeBytes = %d, want %d", updated.SizeBytes, 4096)
	}
	_, completedAtSet := updated.CompletedAt.Get()
	if completedAtSet {
		t.Error("UpdateGameServerBackupProgress().CompletedAt was set, want nil")
	}
}

func TestDeleteGameServerBackup(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-backup-delete.sqlite")
	seedRBACFixture(t, conn)

	backup, errCreate := conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     "/backups/server-local-1/delete-me.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       2048,
		RetentionExempt: true,
		CreatedAt:       time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup() error = %v", errCreate)
	}

	errDelete := conn.DeleteGameServerBackup(backup.ID)
	if errDelete != nil {
		t.Fatalf("DeleteGameServerBackup() error = %v", errDelete)
	}

	_, errGet := conn.GetGameServerBackupByID(backup.ID)
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetGameServerBackupByID() after delete error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestPruneScheduledGameServerBackupsSkipsManualRows(t *testing.T) {
	conn := newRBACMigratedConnection(t, "gs-backup-prune.sqlite")
	seedRBACFixture(t, conn)

	base := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)

	_, errCreate := conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "manual",
		ArchivePath:     "/backups/server-local-1/manual-keep.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       100,
		RetentionExempt: false,
		CreatedAt:       base.Add(5 * time.Hour),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(manual keep) error = %v", errCreate)
	}

	_, errCreate = conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "scheduled",
		ArchivePath:     "/backups/server-local-1/exempt-keep.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       101,
		RetentionExempt: true,
		CreatedAt:       base.Add(4 * time.Hour),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(exempt keep) error = %v", errCreate)
	}

	_, errNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)`,
		"node-remote", "Remote Node", "http://remotehost:9080", true,
	)
	if errNode != nil {
		t.Fatalf("insert node-remote error = %v", errNode)
	}

	_, errCreate = conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-remote",
		CreatedBy:       "user-owner",
		TriggerSource:   "scheduled",
		ArchivePath:     "/backups/server-local-1/other-node-ignore.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       106,
		RetentionExempt: false,
		CreatedAt:       base.Add(6 * time.Hour),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(other node ignore) error = %v", errCreate)
	}

	_, errCreate = conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "scheduled",
		ArchivePath:     "/backups/server-local-1/pending-ignore.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "pending",
		SizeBytes:       102,
		RetentionExempt: false,
		CreatedAt:       base.Add(3 * time.Hour),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(pending ignore) error = %v", errCreate)
	}

	_, errCreate = conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "scheduled",
		ArchivePath:     "/backups/server-local-1/scheduled-newest-keep.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       103,
		RetentionExempt: false,
		CreatedAt:       base.Add(2 * time.Hour),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(scheduled newest keep) error = %v", errCreate)
	}

	_, errCreate = conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "scheduled",
		ArchivePath:     "/backups/server-local-1/scheduled-middle-prune.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       104,
		RetentionExempt: false,
		CreatedAt:       base.Add(1 * time.Hour),
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(scheduled middle prune) error = %v", errCreate)
	}

	oldestScheduled, errCreate := conn.CreateGameServerBackup(CreateGameServerBackupParams{
		GameServerID:    "server-local-1",
		NodeID:          "node-local",
		CreatedBy:       "user-owner",
		TriggerSource:   "scheduled",
		ArchivePath:     "/backups/server-local-1/scheduled-oldest-prune.zip",
		ArchiveRoot:     "/backups",
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       105,
		RetentionExempt: false,
		CreatedAt:       base,
	})
	if errCreate != nil {
		t.Fatalf("CreateGameServerBackup(scheduled oldest prune) error = %v", errCreate)
	}

	pruneCandidates, errPrune := conn.PruneScheduledGameServerBackups("server-local-1", "node-local", 1)
	if errPrune != nil {
		t.Fatalf("PruneScheduledGameServerBackups() error = %v", errPrune)
	}
	if len(pruneCandidates) != 2 {
		t.Fatalf("PruneScheduledGameServerBackups() len = %d, want 2", len(pruneCandidates))
	}
	if pruneCandidates[0].ArchivePath != "/backups/server-local-1/scheduled-middle-prune.zip" {
		t.Errorf("pruneCandidates[0].ArchivePath = %q, want %q", pruneCandidates[0].ArchivePath, "/backups/server-local-1/scheduled-middle-prune.zip")
	}
	if pruneCandidates[1].ID != oldestScheduled.ID {
		t.Errorf("pruneCandidates[1].ID = %q, want %q", pruneCandidates[1].ID, oldestScheduled.ID)
	}

	normalizedCandidates, errNormalize := conn.PruneScheduledGameServerBackups("server-local-1", "node-local", 0)
	if errNormalize != nil {
		t.Fatalf("PruneScheduledGameServerBackups(keepCount=0) error = %v", errNormalize)
	}
	if len(normalizedCandidates) != 2 {
		t.Fatalf("PruneScheduledGameServerBackups(keepCount=0) len = %d, want 2", len(normalizedCandidates))
	}
}
