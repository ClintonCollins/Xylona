package db

import (
	"database/sql"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/migrations"
)

func TestRunMigrationsWithPreviouslyAppliedDiagnosis(t *testing.T) {
	sqlDB, errOpen := sql.Open("sqlite", ":memory:")
	if errOpen != nil {
		t.Fatal(errOpen)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		errClose := sqlDB.Close()
		if errClose != nil {
			t.Error(errClose)
		}
	})

	errMigrate := RunMigrations(sqlDB, migrations.FS, migrations.Root)
	if errMigrate != nil {
		t.Fatal(errMigrate)
	}
	_, errRecord := sqlDB.ExecContext(t.Context(), "insert or ignore into migrations (id, applied_at) values (?, CURRENT_TIMESTAMP)", "20260906000000-add-game-server-diagnosis.sql")
	if errRecord != nil {
		t.Fatal(errRecord)
	}
	errMigrate = RunMigrations(sqlDB, migrations.FS, migrations.Root)
	if errMigrate != nil {
		t.Fatalf("startup with previously applied migration: %v", errMigrate)
	}
}
