package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func seedInstalledModFixture(t *testing.T, conn *Connection) {
	t.Helper()
	seedRBACFixture(t, conn)
}

func TestInsertInstalledModAndGetByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-insert.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()
	setter := &models.InstalledModSetter{
		ID:                 omit.From("mod-1"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-abc"),
		ModName:            omit.From("TestMod"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("abc123"),
		AutoUpdate:         omit.From(int64(1)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	mod, errInsert := conn.InsertInstalledMod(conn.DB, setter)
	if errInsert != nil {
		t.Fatalf("InsertInstalledMod() error = %v", errInsert)
	}
	if mod.ID != "mod-1" {
		t.Errorf("InsertInstalledMod().ID = %q, want %q", mod.ID, "mod-1")
	}
	if mod.ModName != "TestMod" {
		t.Errorf("InsertInstalledMod().ModName = %q, want %q", mod.ModName, "TestMod")
	}

	fetched, errGet := conn.GetInstalledModByID("mod-1")
	if errGet != nil {
		t.Fatalf("GetInstalledModByID() error = %v", errGet)
	}
	if fetched.Source != "modrinth" {
		t.Errorf("GetInstalledModByID().Source = %q, want %q", fetched.Source, "modrinth")
	}
	if fetched.InstalledVersion != "1.0.0" {
		t.Errorf("GetInstalledModByID().InstalledVersion = %q, want %q", fetched.InstalledVersion, "1.0.0")
	}
}

func TestGetInstalledModsByGameServerID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-list.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()
	for i, id := range []string{"mod-a", "mod-b"} {
		setter := &models.InstalledModSetter{
			ID:                 omit.From(id),
			GameServerID:       omit.From("server-local-1"),
			Source:             omit.From("modrinth"),
			SourceID:           omit.From("src-" + id),
			ModName:            omit.From("Mod " + id),
			ModAuthor:          omit.From("Author"),
			InstalledVersion:   omit.From("1.0.0"),
			InstalledVersionID: omit.From("v1"),
			FileHash:           omit.From("hash"),
			AutoUpdate:         omit.From(int64(0)),
			Enabled:            omit.From(int64(1)),
			CreatedAt:          omit.From(now.Add(time.Duration(i) * time.Second)),
			UpdatedAt:          omit.From(now.Add(time.Duration(i) * time.Second)),
		}
		_, errInsert := conn.InsertInstalledMod(conn.DB, setter)
		if errInsert != nil {
			t.Fatalf("InsertInstalledMod(%q) error = %v", id, errInsert)
		}
	}

	mods, errGet := conn.GetInstalledModsByGameServerID("server-local-1")
	if errGet != nil {
		t.Fatalf("GetInstalledModsByGameServerID() error = %v", errGet)
	}
	if len(mods) != 2 {
		t.Errorf("GetInstalledModsByGameServerID() len = %d, want 2", len(mods))
	}
}

func TestGetInstalledModsByGameServerIDEmpty(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-list-empty.sqlite")
	seedInstalledModFixture(t, conn)

	mods, errGet := conn.GetInstalledModsByGameServerID("server-local-1")
	if errGet != nil {
		t.Fatalf("GetInstalledModsByGameServerID() error = %v", errGet)
	}
	if len(mods) != 0 {
		t.Errorf("GetInstalledModsByGameServerID() len = %d, want 0", len(mods))
	}
}

func TestUpdateInstalledMod(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-update.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()
	setter := &models.InstalledModSetter{
		ID:                 omit.From("mod-update"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-u"),
		ModName:            omit.From("UpdateMod"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("hash"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	mod, errInsert := conn.InsertInstalledMod(conn.DB, setter)
	if errInsert != nil {
		t.Fatalf("InsertInstalledMod() error = %v", errInsert)
	}

	updateSetter := &models.InstalledModSetter{
		AutoUpdate: omit.From(int64(1)),
		Enabled:    omit.From(int64(0)),
		UpdatedAt:  omit.From(time.Now().UTC()),
	}

	updated, errUpdate := conn.UpdateInstalledMod(conn.DB, mod, updateSetter)
	if errUpdate != nil {
		t.Fatalf("UpdateInstalledMod() error = %v", errUpdate)
	}
	if updated.AutoUpdate != 1 {
		t.Errorf("UpdateInstalledMod().AutoUpdate = %d, want 1", updated.AutoUpdate)
	}
	if updated.Enabled != 0 {
		t.Errorf("UpdateInstalledMod().Enabled = %d, want 0", updated.Enabled)
	}
}

func TestDeleteInstalledModByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-delete.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()
	setter := &models.InstalledModSetter{
		ID:                 omit.From("mod-del"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-del"),
		ModName:            omit.From("DeleteMe"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("hash"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	_, errInsert := conn.InsertInstalledMod(conn.DB, setter)
	if errInsert != nil {
		t.Fatalf("InsertInstalledMod() error = %v", errInsert)
	}

	errDelete := conn.DeleteInstalledModByID("mod-del")
	if errDelete != nil {
		t.Fatalf("DeleteInstalledModByID() error = %v", errDelete)
	}

	_, errGet := conn.GetInstalledModByID("mod-del")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetInstalledModByID() after delete error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestInstalledModUniqueConstraint(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-unique.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()
	setter := &models.InstalledModSetter{
		ID:                 omit.From("mod-uniq-1"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-uniq"),
		ModName:            omit.From("UniqueMod"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("hash"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	_, errFirst := conn.InsertInstalledMod(conn.DB, setter)
	if errFirst != nil {
		t.Fatalf("InsertInstalledMod(first) error = %v", errFirst)
	}

	setter2 := &models.InstalledModSetter{
		ID:                 omit.From("mod-uniq-2"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-uniq"),
		ModName:            omit.From("UniqueMod2"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("2.0.0"),
		InstalledVersionID: omit.From("v2"),
		FileHash:           omit.From("hash2"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	_, errSecond := conn.InsertInstalledMod(conn.DB, setter2)
	if errSecond == nil {
		t.Fatalf("InsertInstalledMod(duplicate) expected error, got nil")
	}
}

func TestGetInstalledModByIDNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-notfound.sqlite")

	_, errGet := conn.GetInstalledModByID("nonexistent")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetInstalledModByID() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestInsertAndGetInstalledModFiles(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-files.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()
	modSetter := &models.InstalledModSetter{
		ID:                 omit.From("mod-files"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-files"),
		ModName:            omit.From("FilesMod"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("hash"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	_, errInsertMod := conn.InsertInstalledMod(conn.DB, modSetter)
	if errInsertMod != nil {
		t.Fatalf("InsertInstalledMod() error = %v", errInsertMod)
	}

	fileSetter := &models.InstalledModFileSetter{
		ID:             omit.From("file-1"),
		InstalledModID: omit.From("mod-files"),
		FilePath:       omit.From("/mods/testmod.jar"),
		FileHash:       omit.From("filehash123"),
		FileSize:       omit.From(int64(1024)),
		IsPrimary:      omit.From(int64(1)),
	}

	file, errInsertFile := conn.InsertInstalledModFile(conn.DB, fileSetter)
	if errInsertFile != nil {
		t.Fatalf("InsertInstalledModFile() error = %v", errInsertFile)
	}
	if file.FilePath != "/mods/testmod.jar" {
		t.Errorf("InsertInstalledModFile().FilePath = %q, want %q", file.FilePath, "/mods/testmod.jar")
	}

	files, errGet := conn.GetInstalledModFilesByModID("mod-files")
	if errGet != nil {
		t.Fatalf("GetInstalledModFilesByModID() error = %v", errGet)
	}
	if len(files) != 1 {
		t.Errorf("GetInstalledModFilesByModID() len = %d, want 1", len(files))
	}
}

func TestDeleteInstalledModFilesByModID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-files-delete.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()
	modSetter := &models.InstalledModSetter{
		ID:                 omit.From("mod-fdel"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-fdel"),
		ModName:            omit.From("FileDelMod"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("hash"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	_, errInsertMod := conn.InsertInstalledMod(conn.DB, modSetter)
	if errInsertMod != nil {
		t.Fatalf("InsertInstalledMod() error = %v", errInsertMod)
	}

	for _, fileID := range []string{"f1", "f2"} {
		fileSetter := &models.InstalledModFileSetter{
			ID:             omit.From(fileID),
			InstalledModID: omit.From("mod-fdel"),
			FilePath:       omit.From("/mods/" + fileID + ".jar"),
			FileHash:       omit.From("h"),
			FileSize:       omit.From(int64(512)),
			IsPrimary:      omit.From(int64(0)),
		}
		_, errInsertFile := conn.InsertInstalledModFile(conn.DB, fileSetter)
		if errInsertFile != nil {
			t.Fatalf("InsertInstalledModFile(%q) error = %v", fileID, errInsertFile)
		}
	}

	errDelete := conn.DeleteInstalledModFilesByModID(conn.DB, "mod-fdel")
	if errDelete != nil {
		t.Fatalf("DeleteInstalledModFilesByModID() error = %v", errDelete)
	}

	files, errGet := conn.GetInstalledModFilesByModID("mod-fdel")
	if errGet != nil {
		t.Fatalf("GetInstalledModFilesByModID() after delete error = %v", errGet)
	}
	if len(files) != 0 {
		t.Errorf("GetInstalledModFilesByModID() after delete len = %d, want 0", len(files))
	}
}

func TestInstalledModCascadeDeleteFiles(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-cascade.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()
	modSetter := &models.InstalledModSetter{
		ID:                 omit.From("mod-cascade"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-cascade"),
		ModName:            omit.From("CascadeMod"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("hash"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	_, errInsertMod := conn.InsertInstalledMod(conn.DB, modSetter)
	if errInsertMod != nil {
		t.Fatalf("InsertInstalledMod() error = %v", errInsertMod)
	}

	fileSetter := &models.InstalledModFileSetter{
		ID:             omit.From("cascade-file-1"),
		InstalledModID: omit.From("mod-cascade"),
		FilePath:       omit.From("/mods/cascade.jar"),
		FileHash:       omit.From("ch"),
		FileSize:       omit.From(int64(256)),
		IsPrimary:      omit.From(int64(1)),
	}

	_, errInsertFile := conn.InsertInstalledModFile(conn.DB, fileSetter)
	if errInsertFile != nil {
		t.Fatalf("InsertInstalledModFile() error = %v", errInsertFile)
	}

	errDelete := conn.DeleteInstalledModByID("mod-cascade")
	if errDelete != nil {
		t.Fatalf("DeleteInstalledModByID() error = %v", errDelete)
	}

	files, errGet := conn.GetInstalledModFilesByModID("mod-cascade")
	if errGet != nil {
		t.Fatalf("GetInstalledModFilesByModID() after cascade delete error = %v", errGet)
	}
	if len(files) != 0 {
		t.Errorf("GetInstalledModFilesByModID() after cascade delete len = %d, want 0", len(files))
	}
}

func TestInsertInstalledModRespectsTransaction(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-tx-rollback.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()

	tx, errBegin := conn.SQLDb.BeginTx(context.Background(), nil)
	if errBegin != nil {
		t.Fatalf("BeginTx() error = %v", errBegin)
	}
	bobTx := bob.NewTx(tx)

	setter := &models.InstalledModSetter{
		ID:                 omit.From("mod-tx-test"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-tx"),
		ModName:            omit.From("TxMod"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("hash"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	_, errInsert := conn.InsertInstalledMod(bobTx, setter)
	if errInsert != nil {
		t.Fatalf("InsertInstalledMod() error = %v", errInsert)
	}

	errRollback := tx.Rollback()
	if errRollback != nil {
		t.Fatalf("Rollback() error = %v", errRollback)
	}

	_, errGet := conn.GetInstalledModByID("mod-tx-test")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetInstalledModByID() after rollback error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestInsertInstalledModFileRespectsTransaction(t *testing.T) {
	conn := newRBACMigratedConnection(t, "imod-file-tx-rollback.sqlite")
	seedInstalledModFixture(t, conn)

	now := time.Now().UTC()
	modSetter := &models.InstalledModSetter{
		ID:                 omit.From("mod-file-tx"),
		GameServerID:       omit.From("server-local-1"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-file-tx"),
		ModName:            omit.From("FileTxMod"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("hash"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	_, errInsertMod := conn.InsertInstalledMod(conn.DB, modSetter)
	if errInsertMod != nil {
		t.Fatalf("InsertInstalledMod() error = %v", errInsertMod)
	}

	tx, errBegin := conn.SQLDb.BeginTx(context.Background(), nil)
	if errBegin != nil {
		t.Fatalf("BeginTx() error = %v", errBegin)
	}
	bobTx := bob.NewTx(tx)

	fileSetter := &models.InstalledModFileSetter{
		ID:             omit.From("file-tx-1"),
		InstalledModID: omit.From("mod-file-tx"),
		FilePath:       omit.From("/mods/txmod.jar"),
		FileHash:       omit.From("filehash"),
		FileSize:       omit.From(int64(1024)),
		IsPrimary:      omit.From(int64(1)),
	}

	_, errInsertFile := conn.InsertInstalledModFile(bobTx, fileSetter)
	if errInsertFile != nil {
		t.Fatalf("InsertInstalledModFile() error = %v", errInsertFile)
	}

	errRollback := tx.Rollback()
	if errRollback != nil {
		t.Fatalf("Rollback() error = %v", errRollback)
	}

	files, errGet := conn.GetInstalledModFilesByModID("mod-file-tx")
	if errGet != nil {
		t.Fatalf("GetInstalledModFilesByModID() error = %v", errGet)
	}
	if len(files) != 0 {
		t.Errorf("GetInstalledModFilesByModID() after rollback len = %d, want 0", len(files))
	}
}
