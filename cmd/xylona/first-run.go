package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/caarlos0/env/v10"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/ClintonCollins/Xylona/internal/cli/setupcmd"
	dbpkg "github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/firstsetup"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/sql/migrations"
)

type firstRunChoice int

const (
	firstRunChoiceUnset firstRunChoice = iota
	firstRunChoiceCLI
	firstRunChoiceBrowser
)

const (
	defaultControllerHost = "127.0.0.1"
	legacyControllerHost  = "0.0.0.0"
)

var (
	firstRunBindableIPs  = helpers.GetBindableIPs
	openSetupBrowserOnce bool
)

func chooseFirstRunAction(
	setupNeeded bool,
	isTerminal bool,
	flagsPresent bool,
	setupUsername string,
	setupPasswordStdin bool,
	choice firstRunChoice,
) (firstRunChoice, error) {
	if flagsPresent {
		if strings.TrimSpace(setupUsername) == "" {
			return firstRunChoiceUnset, errors.New("username is required for non-interactive setup")
		}
		if !setupPasswordStdin && !isTerminal {
			return firstRunChoiceUnset, errors.New("non-interactive setup requires --setup-password-stdin")
		}
		return firstRunChoiceCLI, nil
	}
	if !setupNeeded || !isTerminal {
		return firstRunChoiceUnset, nil
	}
	switch choice {
	case firstRunChoiceCLI:
		return firstRunChoiceCLI, nil
	case firstRunChoiceBrowser:
		return firstRunChoiceBrowser, nil
	default:
		return firstRunChoiceUnset, errors.New("first-run choice is required")
	}
}

func promptFirstRunChoice(stdin io.Reader, stdout io.Writer) (firstRunChoice, error) {
	_, errWrite := fmt.Fprint(stdout, `Xylona is not set up yet.

  [1] Configure here (CLI wizard)
  [2] Open a browser to configure

Choice [1/2]: `)
	if errWrite != nil {
		return firstRunChoiceUnset, fmt.Errorf("write first-run prompt: %w", errWrite)
	}
	reader := bufio.NewReader(stdin)
	line, errRead := reader.ReadString('\n')
	if errRead != nil && !errors.Is(errRead, io.EOF) {
		return firstRunChoiceUnset, fmt.Errorf("read first-run choice: %w", errRead)
	}
	switch strings.TrimSpace(line) {
	case "1":
		return firstRunChoiceCLI, nil
	case "2":
		return firstRunChoiceBrowser, nil
	default:
		return firstRunChoiceUnset, errors.New("choice must be 1 or 2")
	}
}

func ensureConfigurationSecrets(config *Configuration) error {
	workingDirectory, errWorkingDirectory := os.Getwd()
	if errWorkingDirectory != nil {
		return fmt.Errorf("get working directory: %w", errWorkingDirectory)
	}
	executablePath, errExecutable := resolveCLIExecutableDir()
	if errExecutable != nil {
		return fmt.Errorf("resolve executable path for first-run env file: %w", errExecutable)
	}
	executableDir := filepath.Dir(executablePath)
	envPath, errResolveEnvPath := firstsetup.ResolveEnvPath(workingDirectory, executableDir, config.DBFilePath)
	if errResolveEnvPath != nil {
		return fmt.Errorf("resolve first-run env path: %w", errResolveEnvPath)
	}
	fileSecrets, errLoadSecrets := firstsetup.LoadCurrentSecrets(envPath)
	if errLoadSecrets != nil {
		return fmt.Errorf("load first-run secrets: %w", errLoadSecrets)
	}
	configuredHost, hostConfigured, errLoadHost := loadConfiguredHost(envPath)
	if errLoadHost != nil {
		return errLoadHost
	}
	databaseExists, errDatabaseExists := databaseFileExists(config.DBFilePath)
	if errDatabaseExists != nil {
		return errDatabaseExists
	}
	resolvedHost, hostDefault := resolveControllerHost(configuredHost, hostConfigured, databaseExists)
	config.Host = resolvedHost
	secrets, errEnsure := firstsetup.EnsureSecrets(firstsetup.EnsureSecretsInput{
		Current:     fileSecrets,
		DBPath:      config.DBFilePath,
		EnvPath:     envPath,
		HostDefault: hostDefault,
	})
	if errEnsure != nil {
		return fmt.Errorf("ensure first-run secrets: %w", errEnsure)
	}
	errApply := firstsetup.ApplySecretsToEnv(secrets)
	if errApply != nil {
		return fmt.Errorf("apply first-run secrets: %w", errApply)
	}
	config.CookieHashKey = secrets.CookieHashKey
	config.CookieBlockKey = secrets.CookieBlockKey
	config.EncryptionKey = secrets.EncryptionKey
	return nil
}

func loadConfiguredHost(envPath string) (string, bool, error) {
	host, configured := os.LookupEnv("HOST")
	if configured {
		return host, true, nil
	}

	values, errRead := godotenv.Read(envPath)
	if errRead != nil {
		if os.IsNotExist(errRead) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("load HOST from env file: %w", errRead)
	}
	host, configured = values["HOST"]
	return host, configured, nil
}

func databaseFileExists(path string) (bool, error) {
	info, errStat := os.Stat(path)
	if errStat != nil {
		if os.IsNotExist(errStat) {
			return false, nil
		}
		return false, fmt.Errorf("inspect database for listener compatibility: %w", errStat)
	}
	return !info.IsDir(), nil
}

func resolveControllerHost(configuredHost string, hostConfigured bool, databaseExists bool) (string, string) {
	if hostConfigured {
		if strings.TrimSpace(configuredHost) == "" {
			return legacyControllerHost, ""
		}
		return configuredHost, ""
	}
	if databaseExists {
		return legacyControllerHost, ""
	}
	return defaultControllerHost, defaultControllerHost
}

func setupAccessURLs(config Configuration, token string) []string {
	host := strings.TrimSpace(config.Host)
	if host != "" && host != "0.0.0.0" && host != "::" {
		return []string{formatSetupURL(host, config.HTTPPort, token)}
	}

	loopbackHost := "127.0.0.1"
	if host == "::" {
		loopbackHost = "::1"
	}
	hosts := []string{loopbackHost}
	ips, errIPs := firstRunBindableIPs()
	if errIPs == nil {
		for _, ip := range ips {
			if ip == nil {
				continue
			}
			if host == "0.0.0.0" && ip.To4() == nil {
				continue
			}
			if host == "::" && ip.To4() != nil {
				continue
			}
			hosts = append(hosts, ip.String())
		}
	}
	slices.Sort(hosts[1:])

	urls := make([]string, len(hosts))
	for i, setupHost := range hosts {
		urls[i] = formatSetupURL(setupHost, config.HTTPPort, token)
	}
	return urls
}

func formatSetupURL(host string, port int, token string) string {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	return fmt.Sprintf("http://%s/setup?token=%s", address, token)
}

func maybeOpenSetupBrowser(url string) {
	if !openSetupBrowserOnce {
		return
	}
	openSetupBrowserOnce = false
	if !shouldLaunchLocalBrowser() {
		log.Info().Msg("Open the setup URL on your computer. Xylona will not launch a browser over SSH or without a display.")
		return
	}
	errOpen := openSetupBrowser(url)
	if errOpen != nil {
		log.Warn().Err(errOpen).Msg("Could not open a browser automatically")
	}
}

func shouldLaunchLocalBrowser() bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func openSetupBrowser(url string) error {
	ctx := context.Background()
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.CommandContext(ctx, "cmd", "/c", "start", "", url)
	case "darwin":
		command = exec.CommandContext(ctx, "open", url)
	default:
		command = exec.CommandContext(ctx, "xdg-open", url)
	}
	errStart := command.Start()
	if errStart != nil {
		return fmt.Errorf("open setup browser: %w", errStart)
	}
	return nil
}

func rootSetupFlagsPresent(cmd *cli.Command) bool {
	return cmd.IsSet("setup-username") || cmd.IsSet("setup-email") || cmd.Bool("setup-password-stdin")
}

func runRootAction(ctx context.Context, cmd *cli.Command, serviceAction func() int) error {
	errLoadEnv := godotenv.Load()
	if errLoadEnv != nil && !errors.Is(errLoadEnv, os.ErrNotExist) {
		return fmt.Errorf("load env file: %w", errLoadEnv)
	}
	config := Configuration{}
	errParseConfig := env.Parse(&config)
	if errParseConfig != nil {
		return fmt.Errorf("parse config: %w", errParseConfig)
	}
	resolvedDBPath, errResolveDBPath := dbpkg.ResolveDatabasePath(config.DBFilePath)
	if errResolveDBPath != nil {
		return fmt.Errorf("resolve database path: %w", errResolveDBPath)
	}
	config.DBFilePath = resolvedDBPath

	superUserCount, errCount := dbpkg.CountExistingSuperUsersReadOnly(ctx, config.DBFilePath)
	if errCount != nil {
		return fmt.Errorf("count superusers: %w", errCount)
	}

	isTerminal := term.IsTerminal(int(os.Stdin.Fd()))
	flagsPresent := rootSetupFlagsPresent(cmd)
	choice := firstRunChoiceUnset
	if superUserCount == 0 && isTerminal && !flagsPresent {
		promptedChoice, errChoice := promptFirstRunChoice(os.Stdin, os.Stdout)
		if errChoice != nil {
			return errChoice
		}
		choice = promptedChoice
	}

	action, errAction := chooseFirstRunAction(
		superUserCount == 0,
		isTerminal,
		flagsPresent,
		cmd.String("setup-username"),
		cmd.Bool("setup-password-stdin"),
		choice,
	)
	if errAction != nil {
		return errAction
	}
	if action == firstRunChoiceBrowser {
		openSetupBrowserOnce = true
	}
	if action == firstRunChoiceCLI {
		setupOptions := setupcmd.Options{
			Migrate: func(sqlDB *sql.DB) error {
				return dbpkg.RunMigrations(sqlDB, migrations.FS, migrations.Root)
			},
			ResolveDefaultDBPath: resolveDefaultCLIUserDBPath,
			DefaultDBPath:        config.DBFilePath,
		}
		var errSetup error
		if flagsPresent {
			errSetup = setupcmd.Run(ctx, rootSetupArgs(cmd), setupOptions)
		} else {
			errSetup = setupcmd.Run(ctx, []string{}, setupOptions)
		}
		if errSetup != nil {
			return fmt.Errorf("run first-run setup: %w", errSetup)
		}
	}
	serviceAction()
	return nil
}

func rootSetupArgs(cmd *cli.Command) []string {
	args := []string{"--username", strings.TrimSpace(cmd.String("setup-username"))}
	email := strings.TrimSpace(cmd.String("setup-email"))
	if email != "" {
		args = append(args, "--email", email)
	}
	if cmd.Bool("setup-password-stdin") {
		args = append(args, "--password-stdin")
	}
	return args
}
