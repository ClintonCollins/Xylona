// Package setupcmd implements the `xylona setup` command for first-run setup.
package setupcmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/ClintonCollins/Xylona/internal/cli/usercmd"
	"github.com/ClintonCollins/Xylona/internal/firstsetup"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
)

// Options configures the `xylona setup` command.
type Options = usercmd.Options

// Input is the first-run account to create.
type Input struct {
	UserName string
	Email    string
	Password string
	Offline  bool
	DBPath   string
}

var (
	commandStdin        io.Reader = os.Stdin
	commandStdout       io.Writer = os.Stdout
	commandStderr       io.Writer = os.Stderr
	isTerminalFunc                = term.IsTerminal
	readPasswordFunc              = term.ReadPassword
	newUserServiceFunc            = openUserService
	ensureSecretsFunc             = firstsetup.EnsureSecrets
	getwdFunc                     = os.Getwd
	executableDirFunc             = os.Executable
	stdinFileDescriptor           = func() int { return int(os.Stdin.Fd()) }
)

// Run executes `xylona setup`.
func Run(ctx context.Context, args []string, options Options) error {
	command := NewCommand(options)
	return wrapCommandError("run setup cli", command.Run(ctx, append([]string{"xylona setup"}, args...)))
}

// NewCommand builds the `xylona setup` command.
func NewCommand(options Options) *cli.Command {
	return &cli.Command{
		Name:      "setup",
		Usage:     "Complete first-run setup (secrets and the first superuser)",
		UsageText: "xylona setup [command options]",
		Writer:    commandStdout,
		ErrWriter: commandStderr,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "username", Usage: "First superuser username"},
			&cli.StringFlag{Name: "email", Usage: "First superuser email"},
			&cli.BoolFlag{Name: "password-stdin", Usage: "Read the password from stdin"},
			&cli.BoolFlag{Name: "offline", Usage: "Use direct database access instead of the live local admin transport"},
			&cli.StringFlag{Name: "db", Usage: "Use direct database access for the given SQLite file"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			input, errInput := resolveInput(cmd)
			if errInput != nil {
				return errInput
			}
			return RunWithInput(ctx, options, input)
		},
	}
}

// RunWithInput creates the first superuser through the live or offline service.
func RunWithInput(ctx context.Context, options Options, input Input) (errResult error) {
	resolvedOptions, errResolve := usercmd.ResolveOptions(ctx, options)
	if errResolve != nil {
		return wrapCommandError("resolve options", errResolve)
	}

	dbPath := resolvedOptions.DefaultDBPath
	if strings.TrimSpace(input.DBPath) != "" {
		dbPath = input.DBPath
	}

	forceOffline := input.Offline || strings.TrimSpace(input.DBPath) != ""
	service, cleanup, errService := newUserServiceFunc(ctx, resolvedOptions, dbPath, forceOffline)
	if errService != nil {
		return errService
	}
	defer func() {
		errResult = errors.Join(errResult, cleanup())
	}()

	createdUser, errCreate := service.CreateFirstSuperUser(usermgmt.CreateInput{
		UserName: input.UserName,
		Email:    defaultEmail(input.UserName, input.Email),
		Password: input.Password,
	})
	if errCreate != nil {
		return wrapCommandError("create first superuser", errCreate)
	}

	_, errWrite := fmt.Fprintf(
		commandStdout,
		"Created first superuser %s (%s)\n",
		createdUser.UserName,
		createdUser.Email,
	)
	if errWrite != nil {
		return wrapCommandError("write setup result", errWrite)
	}
	return nil
}

func ensureOfflineSecrets(dbPath string) error {
	workingDirectory, errWorkingDirectory := getwdFunc()
	if errWorkingDirectory != nil {
		return wrapCommandError("get working directory", errWorkingDirectory)
	}
	executablePath, errExecutable := executableDirFunc()
	if errExecutable != nil {
		return wrapCommandError("resolve executable path", errExecutable)
	}
	executableDir := filepath.Dir(executablePath)

	envPath, errResolveEnvPath := firstsetup.ResolveEnvPath(workingDirectory, executableDir, dbPath)
	if errResolveEnvPath != nil {
		return wrapCommandError("resolve env path", errResolveEnvPath)
	}
	currentSecrets, errLoadSecrets := firstsetup.LoadCurrentSecrets(envPath)
	if errLoadSecrets != nil {
		return wrapCommandError("load current secrets", errLoadSecrets)
	}
	secrets, errEnsure := ensureSecretsFunc(firstsetup.EnsureSecretsInput{
		Current: currentSecrets,
		DBPath:  dbPath,
		EnvPath: envPath,
	})
	if errEnsure != nil {
		return wrapCommandError("ensure secrets", errEnsure)
	}
	errApply := firstsetup.ApplySecretsToEnv(secrets)
	if errApply != nil {
		return wrapCommandError("apply secrets", errApply)
	}
	return nil
}

func resolveInput(cmd *cli.Command) (Input, error) {
	input := Input{
		UserName: strings.TrimSpace(cmd.String("username")),
		Email:    strings.TrimSpace(cmd.String("email")),
		Offline:  cmd.Bool("offline"),
		DBPath:   strings.TrimSpace(cmd.String("db")),
	}

	passwordStdin := cmd.Bool("password-stdin")
	interactive := isTerminalFunc(stdinFileDescriptor())

	if !interactive && !passwordStdin {
		return Input{}, errors.New("interactive setup requires a TTY; use --password-stdin for automation")
	}

	if input.UserName == "" {
		if !interactive {
			return Input{}, errors.New("username is required")
		}
		userName, errUserName := promptLine("Username")
		if errUserName != nil {
			return Input{}, errUserName
		}
		input.UserName = userName
	}
	if input.UserName == "" {
		return Input{}, usermgmt.ErrUserNameRequired
	}

	if input.Email == "" && interactive && !passwordStdin {
		email, errEmail := promptLine(fmt.Sprintf("Email [%s]", defaultEmail(input.UserName, "")))
		if errEmail != nil {
			return Input{}, errEmail
		}
		input.Email = email
	}
	input.Email = defaultEmail(input.UserName, input.Email)

	if passwordStdin {
		password, errPassword := readPasswordFromStdin()
		if errPassword != nil {
			return Input{}, errPassword
		}
		input.Password = password
		return input, nil
	}

	password, errPassword := promptForPassword()
	if errPassword != nil {
		return Input{}, errPassword
	}
	input.Password = password
	return input, nil
}

func defaultEmail(userName string, email string) string {
	trimmedEmail := strings.TrimSpace(email)
	if trimmedEmail != "" {
		return trimmedEmail
	}
	trimmedUserName := strings.TrimSpace(userName)
	if trimmedUserName == "" {
		return ""
	}
	return trimmedUserName + "@localhost"
}

func promptLine(label string) (string, error) {
	_, errWrite := fmt.Fprintf(commandStdout, "%s: ", label)
	if errWrite != nil {
		return "", fmt.Errorf("write %s prompt: %w", label, errWrite)
	}
	reader := bufio.NewReader(commandStdin)
	line, errRead := reader.ReadString('\n')
	if errRead != nil && !errors.Is(errRead, io.EOF) {
		return "", fmt.Errorf("read %s: %w", label, errRead)
	}
	return strings.TrimSpace(line), nil
}

func readPasswordFromStdin() (string, error) {
	data, errRead := io.ReadAll(commandStdin)
	if errRead != nil {
		return "", fmt.Errorf("read password from stdin: %w", errRead)
	}
	password := strings.TrimRight(string(data), "\r\n")
	if strings.TrimSpace(password) == "" {
		return "", usermgmt.ErrPasswordEmpty
	}
	return password, nil
}

func promptForPassword() (string, error) {
	if !isTerminalFunc(stdinFileDescriptor()) {
		return "", errors.New("interactive password entry requires a TTY; use --password-stdin for automation")
	}

	_, errWrite := fmt.Fprint(commandStderr, "Password: ")
	if errWrite != nil {
		return "", fmt.Errorf("write password prompt: %w", errWrite)
	}
	passwordBytes, errPassword := readPasswordFunc(stdinFileDescriptor())
	_, errWriteNewline := fmt.Fprintln(commandStderr)
	if errPassword != nil {
		return "", errors.Join(
			fmt.Errorf("read password: %w", errPassword),
			wrapCommandError("write password prompt newline", errWriteNewline),
		)
	}
	if errWriteNewline != nil {
		return "", fmt.Errorf("write password prompt newline: %w", errWriteNewline)
	}
	password := string(passwordBytes)
	if strings.TrimSpace(password) == "" {
		return "", usermgmt.ErrPasswordEmpty
	}

	_, errWriteConfirm := fmt.Fprint(commandStderr, "Confirm password: ")
	if errWriteConfirm != nil {
		return "", fmt.Errorf("write password confirm prompt: %w", errWriteConfirm)
	}
	confirmBytes, errConfirm := readPasswordFunc(stdinFileDescriptor())
	_, errWriteConfirmNewline := fmt.Fprintln(commandStderr)
	if errConfirm != nil {
		return "", errors.Join(
			fmt.Errorf("read password confirmation: %w", errConfirm),
			wrapCommandError("write password confirmation newline", errWriteConfirmNewline),
		)
	}
	if errWriteConfirmNewline != nil {
		return "", fmt.Errorf("write password confirmation newline: %w", errWriteConfirmNewline)
	}
	if password != string(confirmBytes) {
		return "", errors.New("passwords do not match")
	}
	return password, nil
}

func wrapCommandError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("setupcmd: %s: %w", action, err)
}
