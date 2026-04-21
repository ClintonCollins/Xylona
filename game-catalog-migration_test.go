package main

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/cfgschema"
	dbpkg "github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/startargs"
)

func TestGameCatalogMigration(t *testing.T) {
	ctx := context.Background()
	conn, errNew := dbpkg.NewConnection(ctx, filepath.Join(t.TempDir(), "catalog.sqlite"))
	if errNew != nil {
		t.Fatalf("NewConnection() error = %v", errNew)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("Close() error = %v", errClose)
		}
	})

	errMigrate := dbpkg.RunMigrations(conn.SQLDb, EmbeddedMigrations, "sql/migrations")
	if errMigrate != nil {
		t.Fatalf("RunMigrations() error = %v", errMigrate)
	}

	var gameCount int
	errCount := conn.SQLDb.QueryRowContext(ctx, "select count(*) from game").Scan(&gameCount)
	if errCount != nil {
		t.Fatalf("count games: %v", errCount)
	}
	if gameCount < 250 {
		t.Fatalf("game count = %d, want at least 250", gameCount)
	}

	assertGamesPresent(ctx, t, conn, []string{
		"minecraft",
		"7_days_to_die",
		"v_rising",
		"valheim",
		"core_keeper",
		"palworld",
		"hytale",
		"enshrouded",
		"windrose",
		"fivem",
		"redm",
		"multi_theft_auto",
		"openra",
		"openttd",
		"xonotic",
	})
	assertGamesAbsent(ctx, t, conn, []string{
		"teamspeak_3",
		"mumble",
		"foundry_vtt",
		"wine_generic",
		"minecraft_bedrock",
		"terraria_tshock",
		"terraria_tmodloader",
		"papermc",
		"velocity",
		"waterfall",
		"age_of_chivalry",
		"battlefield_2",
		"battlefield_bad_company_2",
		"blood_frontier",
		"counter_strike_1_6",
		"dystopia",
		"et_legacy",
		"eternal_silence",
		"gearbox",
		"half_life",
		"hidden_source",
		"jedi_knight_2",
		"jedi_knight_jedi_academy",
		"just_cause_2_multiplayer",
		"medal_of_honor_openmohaa",
		"medal_of_honor_spearhead",
		"nexuiz",
		"openmp",
		"outlaws_of_the_old_west",
		"pirates_vikings_and_knights_ii",
		"qanga",
		"ricochet",
		"smashball",
		"smokin_guns",
		"soldat",
		"squad_44",
		"synergy",
		"the_bus",
		"venice_unleashed",
		"zombie_survival_game_online",
	})
	assertStartCommandsConfigured(ctx, t, conn)
	assertNoSupportedStartupValuesUnset(ctx, t, conn)
	assertNoShellWrappedStartCommands(ctx, t, conn)
	assertNoInvalidBaseCommandFragments(ctx, t, conn)
	assertNoShellOnlyStartArgTokens(ctx, t, conn)
	assertConfigSchemasValidate(ctx, t, conn)
	assertStartArgsTemplatesValidate(ctx, t, conn)
}

func TestGameCatalogMigrationRollsBack(t *testing.T) {
	ctx := context.Background()
	conn, errNew := dbpkg.NewConnection(ctx, filepath.Join(t.TempDir(), "catalog.sqlite"))
	if errNew != nil {
		t.Fatalf("NewConnection() error = %v", errNew)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("Close() error = %v", errClose)
		}
	})

	errMigrate := dbpkg.RunMigrations(conn.SQLDb, EmbeddedMigrations, "sql/migrations")
	if errMigrate != nil {
		t.Fatalf("RunMigrations() error = %v", errMigrate)
	}

	source := migrate.EmbedFileSystemMigrationSource{
		FileSystem: EmbeddedMigrations,
		Root:       "sql/migrations",
	}
	applied, errDown := migrate.ExecMax(conn.SQLDb, "sqlite3", source, migrate.Down, 1)
	if errDown != nil {
		t.Fatalf("ExecMax(Down) error = %v", errDown)
	}
	if applied != 1 {
		t.Fatalf("ExecMax(Down) applied %d migrations, want 1", applied)
	}

	assertGamesPresent(ctx, t, conn, []string{"hytale"})
	assertGamesAbsent(ctx, t, conn, []string{"windrose"})
}

func assertGamesPresent(ctx context.Context, t *testing.T, conn *dbpkg.Connection, ids []string) {
	t.Helper()
	for _, id := range ids {
		if !gameExists(ctx, t, conn, id) {
			t.Fatalf("expected game %q to exist", id)
		}
	}
}

func assertGamesAbsent(ctx context.Context, t *testing.T, conn *dbpkg.Connection, ids []string) {
	t.Helper()
	for _, id := range ids {
		if gameExists(ctx, t, conn, id) {
			t.Fatalf("expected game %q to be absent", id)
		}
	}
}

func gameExists(ctx context.Context, t *testing.T, conn *dbpkg.Connection, id string) bool {
	t.Helper()
	var count int
	errScan := conn.SQLDb.QueryRowContext(ctx, "select count(*) from game where id = ?", id).Scan(&count)
	if errScan != nil {
		t.Fatalf("query game %q: %v", id, errScan)
	}
	return count > 0
}

func assertConfigSchemasValidate(ctx context.Context, t *testing.T, conn *dbpkg.Connection) {
	t.Helper()
	rows, errQuery := conn.SQLDb.QueryContext(ctx, "select id, config_schemas from game where config_schemas is not null and trim(config_schemas) <> ''")
	if errQuery != nil {
		t.Fatalf("query config schemas: %v", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			t.Errorf("Rows.Close() error = %v", errClose)
		}
	}()

	for rows.Next() {
		var id string
		var schemas string
		errScan := rows.Scan(&id, &schemas)
		if errScan != nil {
			t.Fatalf("scan config schema row: %v", errScan)
		}
		errs := cfgschema.ValidateConfigSchemas(schemas)
		if len(errs) != 0 {
			t.Fatalf("config_schemas for %s failed validation: %s", id, strings.Join(errs, "; "))
		}
	}
	errRows := rows.Err()
	if errRows != nil {
		t.Fatalf("iterate config schema rows: %v", errRows)
	}
}

func assertStartCommandsConfigured(ctx context.Context, t *testing.T, conn *dbpkg.Connection) {
	t.Helper()

	var launchableCount int
	errCount := conn.SQLDb.QueryRowContext(ctx, `
select count(*)
from game
where (
    linux_support = 1
    and trim(linux_base_command) <> ''
    and linux_start_args_template is not null
    and trim(linux_start_args_template) <> ''
) or (
    windows_support = 1
    and trim(windows_base_command) <> ''
    and windows_start_args_template is not null
    and trim(windows_start_args_template) <> ''
)`).Scan(&launchableCount)
	if errCount != nil {
		t.Fatalf("count launchable games: %v", errCount)
	}
	if launchableCount < 240 {
		t.Fatalf("launchable game count = %d, want at least 240", launchableCount)
	}

	assertGamesHaveStartCommands(ctx, t, conn, []string{
		"fivem",
		"redm",
		"rust",
		"counter_strike_2",
		"ark_survival_evolved",
		"dayz",
		"arma_3",
		"factorio",
		"terraria",
		"multi_theft_auto",
		"openttd",
		"xonotic",
	})
}

func assertGamesHaveStartCommands(ctx context.Context, t *testing.T, conn *dbpkg.Connection, ids []string) {
	t.Helper()

	for _, id := range ids {
		var count int
		errScan := conn.SQLDb.QueryRowContext(ctx, `
select count(*)
from game
where id = ?
  and (
      (
          linux_support = 1
          and trim(linux_base_command) <> ''
          and linux_start_args_template is not null
          and trim(linux_start_args_template) <> ''
      )
      or (
          windows_support = 1
          and trim(windows_base_command) <> ''
          and windows_start_args_template is not null
          and trim(windows_start_args_template) <> ''
      )
  )`, id).Scan(&count)
		if errScan != nil {
			t.Fatalf("query start command for %q: %v", id, errScan)
		}
		if count == 0 {
			t.Fatalf("expected game %q to have a runtime command", id)
		}
	}
}

func assertNoSupportedStartupValuesUnset(ctx context.Context, t *testing.T, conn *dbpkg.Connection) {
	t.Helper()

	rows, errQuery := conn.SQLDb.QueryContext(ctx, `
select id,
       name,
       case
           when linux_support = 1
               and (
                   trim(coalesce(linux_base_command, '')) = ''
                   or linux_start_args_template is null
                   or trim(linux_start_args_template) = ''
               )
               then 'linux'
           else ''
       end as missing_linux,
       case
           when windows_support = 1
               and (
                   trim(coalesce(windows_base_command, '')) = ''
                   or windows_start_args_template is null
                   or trim(windows_start_args_template) = ''
               )
               then 'windows'
           else ''
       end as missing_windows
from game
where (
    linux_support = 1
    and (
        trim(coalesce(linux_base_command, '')) = ''
        or linux_start_args_template is null
        or trim(linux_start_args_template) = ''
    )
) or (
    windows_support = 1
    and (
        trim(coalesce(windows_base_command, '')) = ''
        or windows_start_args_template is null
        or trim(windows_start_args_template) = ''
    )
)
order by id`)
	if errQuery != nil {
		t.Fatalf("query games with unset startup values: %v", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			t.Errorf("Rows.Close() error = %v", errClose)
		}
	}()

	var unsetGames []string
	for rows.Next() {
		var id string
		var name string
		var missingLinux string
		var missingWindows string
		errScan := rows.Scan(&id, &name, &missingLinux, &missingWindows)
		if errScan != nil {
			t.Fatalf("scan game with unset startup values: %v", errScan)
		}

		var missing []string
		if missingLinux != "" {
			missing = append(missing, missingLinux)
		}
		if missingWindows != "" {
			missing = append(missing, missingWindows)
		}
		unsetGames = append(unsetGames, id+" ("+name+"): "+strings.Join(missing, ", "))
	}
	errRows := rows.Err()
	if errRows != nil {
		t.Fatalf("iterate games with unset startup values: %v", errRows)
	}
	if len(unsetGames) != 0 {
		t.Fatalf("games with unset startup values:\n%s", strings.Join(unsetGames, "\n"))
	}
}

func assertNoShellWrappedStartCommands(ctx context.Context, t *testing.T, conn *dbpkg.Connection) {
	t.Helper()

	rows, errQuery := conn.SQLDb.QueryContext(ctx, `
select id, name
from game
where lower(trim(coalesce(linux_base_command, ''))) in ('bash', 'sh', 'cmd', 'powershell', 'pwsh')
   or lower(trim(coalesce(windows_base_command, ''))) in ('bash', 'sh', 'cmd', 'powershell', 'pwsh')
   or coalesce(linux_start_args_template, '') like '%cd "{{INSTALL_DIR}}"%'
   or coalesce(windows_start_args_template, '') like '%cd /D "{{INSTALL_DIR}}"%'
order by id`)
	if errQuery != nil {
		t.Fatalf("query shell-wrapped start commands: %v", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			t.Errorf("Rows.Close() error = %v", errClose)
		}
	}()

	var wrappedGames []string
	for rows.Next() {
		var id string
		var name string
		errScan := rows.Scan(&id, &name)
		if errScan != nil {
			t.Fatalf("scan shell-wrapped start command: %v", errScan)
		}
		wrappedGames = append(wrappedGames, id+" ("+name+")")
	}
	errRows := rows.Err()
	if errRows != nil {
		t.Fatalf("iterate shell-wrapped start commands: %v", errRows)
	}
	if len(wrappedGames) != 0 {
		t.Fatalf("games with shell-wrapped start commands:\n%s", strings.Join(wrappedGames, "\n"))
	}
}

func assertNoInvalidBaseCommandFragments(ctx context.Context, t *testing.T, conn *dbpkg.Connection) {
	t.Helper()

	rows, errQuery := conn.SQLDb.QueryContext(ctx, `
select id, name, 'linux' as os_name, linux_base_command
from game
where trim(coalesce(linux_base_command, '')) <> ''
  and (
      lower(trim(linux_base_command)) in ('[', 'cd', 'rmv()', 'if', 'then', 'else', 'fi', 'while', 'until', 'do', 'done', 'echo', 'tail', 'trap', 'export')
      or instr(linux_base_command, '$') > 0
      or instr(linux_base_command, ';') > 0
      or instr(linux_base_command, '&') > 0
      or instr(linux_base_command, '|') > 0
      or instr(linux_base_command, '<') > 0
      or instr(linux_base_command, '>') > 0
  )
union all
select id, name, 'windows' as os_name, windows_base_command
from game
where trim(coalesce(windows_base_command, '')) <> ''
  and (
      lower(trim(windows_base_command)) in ('[', 'cd', 'rmv()', 'if', 'then', 'else', 'fi', 'while', 'until', 'do', 'done', 'echo', 'tail', 'trap', 'export')
      or instr(windows_base_command, '$') > 0
      or instr(windows_base_command, ';') > 0
      or instr(windows_base_command, '&') > 0
      or instr(windows_base_command, '|') > 0
      or instr(windows_base_command, '<') > 0
      or instr(windows_base_command, '>') > 0
  )
order by id, os_name`)
	if errQuery != nil {
		t.Fatalf("query invalid base command fragments: %v", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			t.Errorf("Rows.Close() error = %v", errClose)
		}
	}()

	var invalidGames []string
	for rows.Next() {
		var id string
		var name string
		var osName string
		var baseCommand string
		errScan := rows.Scan(&id, &name, &osName, &baseCommand)
		if errScan != nil {
			t.Fatalf("scan invalid base command fragment: %v", errScan)
		}
		invalidGames = append(invalidGames, id+" ("+name+") "+osName+": "+baseCommand)
	}
	errRows := rows.Err()
	if errRows != nil {
		t.Fatalf("iterate invalid base command fragments: %v", errRows)
	}
	if len(invalidGames) != 0 {
		t.Fatalf("games with invalid base command fragments:\n%s", strings.Join(invalidGames, "\n"))
	}
}

func assertNoShellOnlyStartArgTokens(ctx context.Context, t *testing.T, conn *dbpkg.Connection) {
	t.Helper()

	rows, errQuery := conn.SQLDb.QueryContext(ctx, `
select id, name, 'linux' as os_name, linux_base_command, linux_start_args_template
from game
where linux_start_args_template is not null and trim(linux_start_args_template) <> ''
union all
select id, name, 'windows' as os_name, windows_base_command, windows_start_args_template
from game
where windows_start_args_template is not null and trim(windows_start_args_template) <> ''
order by id, os_name`)
	if errQuery != nil {
		t.Fatalf("query start command templates: %v", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			t.Errorf("Rows.Close() error = %v", errClose)
		}
	}()

	var invalidTokens []string
	for rows.Next() {
		var id string
		var name string
		var osName string
		var baseCommand string
		var templateJSON string
		errScan := rows.Scan(&id, &name, &osName, &baseCommand, &templateJSON)
		if errScan != nil {
			t.Fatalf("scan start command template: %v", errScan)
		}

		for _, reason := range invalidBaseCommandReasons(baseCommand) {
			invalidTokens = append(invalidTokens, id+" ("+name+") "+osName+" base command "+baseCommand+": "+reason)
		}

		blocks, errParse := startargs.ParseTemplate(templateJSON)
		if errParse != nil {
			t.Fatalf("%s start args template for %s failed validation: %v", osName, id, errParse)
		}
		for _, block := range blocks {
			for tokenIndex, token := range block.Tokens {
				for _, reason := range invalidStartTokenReasons(token) {
					invalidTokens = append(invalidTokens, id+" ("+name+") "+osName+" token "+token+": "+reason)
				}
				for _, reason := range invalidStartTokenContextReasons(block.Tokens, tokenIndex) {
					invalidTokens = append(invalidTokens, id+" ("+name+") "+osName+" token "+token+": "+reason)
				}
			}
		}
	}
	errRows := rows.Err()
	if errRows != nil {
		t.Fatalf("iterate start command templates: %v", errRows)
	}
	if len(invalidTokens) != 0 {
		t.Fatalf("start command tokens incompatible with direct exec:\n%s", strings.Join(invalidTokens, "\n"))
	}
}

var (
	envStylePlaceholderPattern = regexp.MustCompile(`\$\{?[A-Za-z_][A-Za-z0-9_]*\}?`)
	joinedPlaceholderPattern   = regexp.MustCompile(`[A-Za-z0-9_]\{\{[A-Z_]+\}\}`)
	xylonaPlaceholderPattern   = regexp.MustCompile(`\{\{([A-Z_]+)\}\}`)
)

func invalidBaseCommandReasons(baseCommand string) []string {
	trimmed := strings.TrimSpace(baseCommand)
	if trimmed == "" {
		return nil
	}

	var reasons []string
	lowerBaseCommand := strings.ToLower(trimmed)
	switch lowerBaseCommand {
	case "rm", "winetricks":
		reasons = append(reasons, "base command is a shell/setup utility, not the server launcher")
	}
	if strings.ContainsAny(trimmed, "&;|<>") {
		reasons = append(reasons, "base command contains shell control characters")
	}
	if strings.Contains(trimmed, "$") {
		reasons = append(reasons, "base command contains shell variable syntax")
	}
	if strings.Contains(trimmed, "*") {
		reasons = append(reasons, "base command contains a glob that exec.Command will not expand")
	}
	if strings.Contains(trimmed, "=") && !strings.ContainsAny(trimmed, `/\`) {
		reasons = append(reasons, "base command looks like an environment assignment")
	}

	return reasons
}

func invalidStartTokenReasons(token string) []string {
	var reasons []string
	if token == "" {
		reasons = append(reasons, "empty token should be omitted")
		return reasons
	}
	if strings.Contains(token, "${") || envStylePlaceholderPattern.MatchString(token) {
		reasons = append(reasons, "uses shell/env placeholder syntax instead of Xylona placeholders")
	}
	if strings.Contains(token, "$(") || strings.Contains(token, "$?") {
		reasons = append(reasons, "uses shell expansion syntax")
	}
	if strings.Contains(token, "$") {
		reasons = append(reasons, "uses shell-only dollar syntax")
	}
	if strings.Contains(token, "&&") || strings.Contains(token, "||") || strings.HasPrefix(token, "|") || strings.HasSuffix(token, ";") {
		reasons = append(reasons, "contains shell control characters")
	}
	if strings.Contains(token, "{{") != strings.Contains(token, "}}") {
		reasons = append(reasons, "contains malformed Xylona placeholder braces")
	}
	if joinedPlaceholderPattern.MatchString(token) && !allowsJoinedPlaceholderValue(token) {
		reasons = append(reasons, "joins a placeholder to an argument name without a separator")
	}
	if strings.Contains(strings.ToLower(token), "checkbox") {
		reasons = append(reasons, "contains panel form text, not a server argument")
	}
	for _, placeholderMatch := range xylonaPlaceholderPattern.FindAllStringSubmatch(token, -1) {
		if len(placeholderMatch) < 2 {
			continue
		}
		if !isCatalogPlaceholderSupported(placeholderMatch[1]) {
			reasons = append(reasons, "uses unsupported Xylona placeholder "+placeholderMatch[1])
		}
	}

	switch strings.ToLower(strings.TrimSpace(token)) {
	case "&", "&&", "|", "||", ";", "then", "else", "fi", "echo", "printf", "tail", "sleep":
		reasons = append(reasons, "token is shell syntax, not a server argument")
	}

	return reasons
}

func allowsJoinedPlaceholderValue(token string) bool {
	if strings.HasPrefix(token, "-Xms") || strings.HasPrefix(token, "-Xmx") {
		return true
	}
	if len(token) > 2 && token[0] == '-' && token[2] == '{' {
		return true
	}
	return false
}

func invalidStartTokenContextReasons(tokens []string, tokenIndex int) []string {
	if tokenIndex < 0 || tokenIndex >= len(tokens) {
		return nil
	}

	token := strings.TrimSpace(tokens[tokenIndex])
	if token == "" {
		return nil
	}

	var reasons []string
	lowerToken := strings.ToLower(token)
	if startArgRequiresValue(lowerToken) && !hasFollowingValue(tokens, tokenIndex) {
		reasons = append(reasons, "requires a following value token")
	}
	if lowerToken == "+set" && !hasFollowingSetPair(tokens, tokenIndex) {
		reasons = append(reasons, "+set requires a cvar name and value")
	}

	return reasons
}

func startArgRequiresValue(token string) bool {
	switch token {
	case "+map",
		"+exec",
		"+servercfgfile",
		"+hostname",
		"+rcon_password",
		"+sv_setsteamaccount",
		"-game",
		"-ip",
		"+ip",
		"-port",
		"+port",
		"-maxplayers",
		"+maxplayers",
		"-pidfile":
		return true
	default:
		return false
	}
}

func hasFollowingValue(tokens []string, tokenIndex int) bool {
	nextIndex := tokenIndex + 1
	if nextIndex >= len(tokens) {
		return false
	}
	nextToken := strings.TrimSpace(tokens[nextIndex])
	if nextToken == "" {
		return false
	}
	return !looksLikeCommandSwitch(nextToken)
}

func hasFollowingSetPair(tokens []string, tokenIndex int) bool {
	cvarIndex := tokenIndex + 1
	valueIndex := tokenIndex + 2
	if valueIndex >= len(tokens) {
		return false
	}
	cvarToken := strings.TrimSpace(tokens[cvarIndex])
	valueToken := strings.TrimSpace(tokens[valueIndex])
	if cvarToken == "" || valueToken == "" {
		return false
	}
	return !looksLikeCommandSwitch(cvarToken) && !looksLikeCommandSwitch(valueToken)
}

func looksLikeCommandSwitch(token string) bool {
	if strings.HasPrefix(token, "{{") {
		return false
	}
	if strings.HasPrefix(token, "+") {
		return true
	}
	if strings.HasPrefix(token, "-") {
		_, errParse := strconv.ParseFloat(token, 64)
		return errParse != nil
	}
	return false
}

func isCatalogPlaceholderSupported(key string) bool {
	switch key {
	case "IP",
		"PORT",
		"QUERY_PORT",
		"MAX_MEMORY_MB",
		"MAX_PLAYERS",
		"SERVER_NAME",
		"RCON_PORT",
		"RCON_PASSWORD",
		"INSTALL_DIR",
		"STEAM_APPID",
		"SERVER_EXECUTABLE",
		"SERVER_ID",
		"BACKUP_DIR",
		"SET_PLAYERS":
		return true
	default:
		return false
	}
}

func assertStartArgsTemplatesValidate(ctx context.Context, t *testing.T, conn *dbpkg.Connection) {
	t.Helper()

	rows, errQuery := conn.SQLDb.QueryContext(ctx, `
select id, 'linux', linux_start_args_template
from game
where linux_start_args_template is not null and trim(linux_start_args_template) <> ''
union all
select id, 'windows', windows_start_args_template
from game
where windows_start_args_template is not null and trim(windows_start_args_template) <> ''`)
	if errQuery != nil {
		t.Fatalf("query start args templates: %v", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			t.Errorf("Rows.Close() error = %v", errClose)
		}
	}()

	for rows.Next() {
		var id string
		var osName string
		var templateJSON string
		errScan := rows.Scan(&id, &osName, &templateJSON)
		if errScan != nil {
			t.Fatalf("scan start args template row: %v", errScan)
		}
		_, errParse := startargs.ParseTemplate(templateJSON)
		if errParse != nil {
			t.Fatalf("%s start args template for %s failed validation: %v", osName, id, errParse)
		}
	}
	errRows := rows.Err()
	if errRows != nil {
		t.Fatalf("iterate start args template rows: %v", errRows)
	}
}
