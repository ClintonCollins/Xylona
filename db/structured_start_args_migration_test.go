package db

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
)

func TestStructuredStartArgsMigrationNormalizesLegacyCommandTypes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "structured-start-args-legacy.sqlite")

	conn, errNewConnection := NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("NewConnection() error = %v", errNewConnection)
	}
	t.Cleanup(func() {
		_ = conn.SQLDb.Close()
	})

	migrationSource := &migrate.FileMigrationSource{
		Dir: filepath.Join("..", "sql", "migrations"),
	}
	migrate.SetTable("migrations")

	_, errMigrate := migrate.ExecVersion(
		conn.SQLDb,
		"sqlite3",
		migrationSource,
		migrate.Up,
		20260324113000,
	)
	if errMigrate != nil {
		t.Fatalf("migrate.ExecVersion(pre-structured-start-args) error = %v", errMigrate)
	}

	_, errIgnoreChecks := conn.SQLDb.ExecContext(context.Background(), `PRAGMA ignore_check_constraints = ON`)
	if errIgnoreChecks != nil {
		t.Fatalf("enable ignore_check_constraints error = %v", errIgnoreChecks)
	}

	_, errCorrupt := conn.SQLDb.ExecContext(
		context.Background(),
		`UPDATE game
SET linux_install_command_type = 'steamcmd',
    linux_update_command_type = 'mojang',
    windows_install_command_type = 'steamcmd',
    windows_update_command_type = 'papermc'
WHERE id = 'minecraft'`,
	)
	if errCorrupt != nil {
		t.Fatalf("seed legacy command types error = %v", errCorrupt)
	}

	_, errRestoreChecks := conn.SQLDb.ExecContext(context.Background(), `PRAGMA ignore_check_constraints = OFF`)
	if errRestoreChecks != nil {
		t.Fatalf("disable ignore_check_constraints error = %v", errRestoreChecks)
	}

	_, errMigrateRemaining := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrateRemaining != nil {
		t.Fatalf("migrate.Exec(remaining migrations) error = %v", errMigrateRemaining)
	}

	game, errGet := conn.GetGameByID("minecraft")
	if errGet != nil {
		t.Fatalf("GetGameByID() error = %v", errGet)
	}

	if game.LinuxInstallCommandType != "direct" {
		t.Fatalf("LinuxInstallCommandType = %q, want %q", game.LinuxInstallCommandType, "direct")
	}
	if game.LinuxUpdateCommandType != "internal" {
		t.Fatalf("LinuxUpdateCommandType = %q, want %q", game.LinuxUpdateCommandType, "internal")
	}
	if game.WindowsInstallCommandType != "direct" {
		t.Fatalf("WindowsInstallCommandType = %q, want %q", game.WindowsInstallCommandType, "direct")
	}
	if game.WindowsUpdateCommandType != "internal" {
		t.Fatalf("WindowsUpdateCommandType = %q, want %q", game.WindowsUpdateCommandType, "internal")
	}
}

func TestStructuredStartArgsMigrationSupportsLegacyXylonaInternalSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "structured-start-args-xylona-internal.sqlite")

	conn, errNewConnection := NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("NewConnection() error = %v", errNewConnection)
	}
	t.Cleanup(func() {
		_ = conn.SQLDb.Close()
	})

	migrationSource := &migrate.FileMigrationSource{
		Dir: filepath.Join("..", "sql", "migrations"),
	}
	migrate.SetTable("migrations")

	_, errMigrate := migrate.ExecVersion(
		conn.SQLDb,
		"sqlite3",
		migrationSource,
		migrate.Up,
		20260324113000,
	)
	if errMigrate != nil {
		t.Fatalf("migrate.ExecVersion(pre-structured-start-args) error = %v", errMigrate)
	}

	errRebuild := rebuildGameTableWithLegacyInternalTypes(conn)
	if errRebuild != nil {
		t.Fatalf("rebuildGameTableWithLegacyInternalTypes() error = %v", errRebuild)
	}

	_, errIgnoreChecks := conn.SQLDb.ExecContext(context.Background(), `PRAGMA ignore_check_constraints = ON`)
	if errIgnoreChecks != nil {
		t.Fatalf("enable ignore_check_constraints error = %v", errIgnoreChecks)
	}

	_, errCorrupt := conn.SQLDb.ExecContext(
		context.Background(),
		`UPDATE game
SET linux_install_command_type = 'mojang',
    linux_update_command_type = 'papermc',
    windows_install_command_type = 'xylona_internal',
    windows_update_command_type = 'pwsh'
WHERE id = 'minecraft'`,
	)
	if errCorrupt != nil {
		t.Fatalf("seed legacy command types error = %v", errCorrupt)
	}

	_, errRestoreChecks := conn.SQLDb.ExecContext(context.Background(), `PRAGMA ignore_check_constraints = OFF`)
	if errRestoreChecks != nil {
		t.Fatalf("disable ignore_check_constraints error = %v", errRestoreChecks)
	}

	_, errMigrateRemaining := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrateRemaining != nil {
		t.Fatalf("migrate.Exec(remaining migrations) error = %v", errMigrateRemaining)
	}

	game, errGet := conn.GetGameByID("minecraft")
	if errGet != nil {
		t.Fatalf("GetGameByID() error = %v", errGet)
	}

	if game.LinuxInstallCommandType != "xylona_internal" {
		t.Fatalf("LinuxInstallCommandType = %q, want %q", game.LinuxInstallCommandType, "xylona_internal")
	}
	if game.LinuxUpdateCommandType != "xylona_internal" {
		t.Fatalf("LinuxUpdateCommandType = %q, want %q", game.LinuxUpdateCommandType, "xylona_internal")
	}
	if game.WindowsInstallCommandType != "xylona_internal" {
		t.Fatalf("WindowsInstallCommandType = %q, want %q", game.WindowsInstallCommandType, "xylona_internal")
	}
	if game.WindowsUpdateCommandType != "pwsh" {
		t.Fatalf("WindowsUpdateCommandType = %q, want %q", game.WindowsUpdateCommandType, "pwsh")
	}

	hytale, errGetHytale := conn.GetGameByID("hytale")
	if errGetHytale != nil {
		t.Fatalf("GetGameByID(hytale) error = %v", errGetHytale)
	}

	if hytale.LinuxInstallCommandType != "xylona_internal" {
		t.Fatalf("Hytale LinuxInstallCommandType = %q, want %q", hytale.LinuxInstallCommandType, "xylona_internal")
	}
	if hytale.WindowsInstallCommandType != "xylona_internal" {
		t.Fatalf("Hytale WindowsInstallCommandType = %q, want %q", hytale.WindowsInstallCommandType, "xylona_internal")
	}
}

func TestStructuredStartArgsMigrationDownFailsFast(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "structured-start-args-rollback.sqlite")

	conn, errNewConnection := NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("NewConnection() error = %v", errNewConnection)
	}
	t.Cleanup(func() {
		_ = conn.SQLDb.Close()
	})

	migrationSource := &migrate.FileMigrationSource{
		Dir: filepath.Join("..", "sql", "migrations"),
	}
	migrate.SetTable("migrations")

	_, errMigrateUp := migrate.ExecVersion(
		conn.SQLDb,
		"sqlite3",
		migrationSource,
		migrate.Up,
		20260325120000,
	)
	if errMigrateUp != nil {
		t.Fatalf("migrate.ExecVersion(up to structured-start-args) error = %v", errMigrateUp)
	}

	_, errMigrateDown := migrate.ExecVersion(
		conn.SQLDb,
		"sqlite3",
		migrationSource,
		migrate.Down,
		20260324113000,
	)
	if errMigrateDown == nil {
		t.Fatal("migrate.ExecVersion(down before structured-start-args) error = nil, want unsupported rollback failure")
	}
	if !strings.Contains(errMigrateDown.Error(), "down migration unsupported") {
		t.Fatalf("migrate.ExecVersion(down before structured-start-args) error = %v, want unsupported rollback message", errMigrateDown)
	}
}

func rebuildGameTableWithLegacyInternalTypes(conn *Connection) error {
	_, errDisableForeignKeys := conn.SQLDb.ExecContext(context.Background(), `PRAGMA foreign_keys = OFF`)
	if errDisableForeignKeys != nil {
		return fmt.Errorf("disable foreign keys: %w", errDisableForeignKeys)
	}

	_, errCreateLegacy := conn.SQLDb.ExecContext(
		context.Background(),
		`CREATE TABLE game_legacy (
    id                                     text primary key not null,
    name                                   text             not null,
    default_port                           bigint           not null,
    default_query_port                     bigint           not null,
    default_max_players                    bigint           not null,
    require_dedicated_ip                   boolean          not null default false,
    binds_to_all_ips                       boolean          not null default false,
    uses_source_query                      boolean          not null default false,
    uses_steamcmd                          boolean          not null default false,
    steam_app_id                           text             not null default '' check ( uses_steamcmd == 0 or steam_app_id not null ),
    requires_steam_game_server_login_token boolean          not null default false,
    linux_support                          boolean          not null default false,
    linux_start_command                    text             not null default '' check ( linux_support == 0 or linux_start_command not null ),
    linux_stop_command                     text             not null default '',
    linux_install_command                  text             not null default '' check ( linux_support == 0 or linux_install_command not null ),
    linux_install_command_type             text             not null default 'direct',
    linux_update_command                   text             not null default '',
    linux_update_command_type              text             not null default 'direct',
    linux_working_directory                text             not null default '',
    windows_support                        boolean          not null default false,
    windows_start_command                  text             not null default '' check ( windows_support == 0 or windows_start_command not null ),
    windows_stop_command                   text             not null default '',
    windows_install_command                text             not null default '' check ( windows_support == 0 or windows_install_command not null ),
    windows_install_command_type           text             not null default 'direct',
    windows_update_command                 text             not null default '',
    windows_update_command_type            text             not null default 'direct',
    windows_working_directory              text             not null default '',
    created_at                             datetime         not null default current_timestamp,
    updated_at                             datetime         not null default current_timestamp,
    xylona_official                        boolean          not null default false,
    config_schemas                         text             default null,
    server_software                        text             default null,

    constraint linux_install_command_type_check check ( linux_install_command_type in ('direct', 'bash', 'xylona_internal') ),
    constraint linux_update_command_type_check check ( linux_update_command_type in ('direct', 'bash', 'xylona_internal') ),
    constraint windows_install_command_type_check check ( windows_install_command_type in ('direct', 'cmd', 'powershell', 'pwsh', 'xylona_internal') ),
    constraint windows_update_command_type_check check ( windows_update_command_type in ('direct', 'cmd', 'powershell', 'pwsh', 'xylona_internal') )
)`,
	)
	if errCreateLegacy != nil {
		return fmt.Errorf("create legacy game table: %w", errCreateLegacy)
	}

	_, errCopyRows := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO game_legacy (
    id,
    name,
    default_port,
    default_query_port,
    default_max_players,
    require_dedicated_ip,
    binds_to_all_ips,
    uses_source_query,
    uses_steamcmd,
    steam_app_id,
    requires_steam_game_server_login_token,
    linux_support,
    linux_start_command,
    linux_stop_command,
    linux_install_command,
    linux_install_command_type,
    linux_update_command,
    linux_update_command_type,
    linux_working_directory,
    windows_support,
    windows_start_command,
    windows_stop_command,
    windows_install_command,
    windows_install_command_type,
    windows_update_command,
    windows_update_command_type,
    windows_working_directory,
    created_at,
    updated_at,
    xylona_official,
    config_schemas,
    server_software
)
SELECT
    id,
    name,
    default_port,
    default_query_port,
    default_max_players,
    require_dedicated_ip,
    binds_to_all_ips,
    uses_source_query,
    uses_steamcmd,
    steam_app_id,
    requires_steam_game_server_login_token,
    linux_support,
    linux_start_command,
    linux_stop_command,
    linux_install_command,
    CASE
        WHEN lower(trim(linux_install_command_type)) = 'internal' THEN 'xylona_internal'
        ELSE linux_install_command_type
    END,
    linux_update_command,
    CASE
        WHEN lower(trim(linux_update_command_type)) = 'internal' THEN 'xylona_internal'
        ELSE linux_update_command_type
    END,
    linux_working_directory,
    windows_support,
    windows_start_command,
    windows_stop_command,
    windows_install_command,
    CASE
        WHEN lower(trim(windows_install_command_type)) = 'internal' THEN 'xylona_internal'
        ELSE windows_install_command_type
    END,
    windows_update_command,
    CASE
        WHEN lower(trim(windows_update_command_type)) = 'internal' THEN 'xylona_internal'
        ELSE windows_update_command_type
    END,
    windows_working_directory,
    created_at,
    updated_at,
    xylona_official,
    config_schemas,
    server_software
FROM game`,
	)
	if errCopyRows != nil {
		return fmt.Errorf("copy rows into legacy game table: %w", errCopyRows)
	}

	_, errDropGame := conn.SQLDb.ExecContext(context.Background(), `DROP TABLE game`)
	if errDropGame != nil {
		return fmt.Errorf("drop original game table: %w", errDropGame)
	}

	_, errRenameLegacy := conn.SQLDb.ExecContext(context.Background(), `ALTER TABLE game_legacy RENAME TO game`)
	if errRenameLegacy != nil {
		return fmt.Errorf("rename legacy game table: %w", errRenameLegacy)
	}

	_, errEnableForeignKeys := conn.SQLDb.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`)
	if errEnableForeignKeys != nil {
		return fmt.Errorf("enable foreign keys: %w", errEnableForeignKeys)
	}

	return nil
}
