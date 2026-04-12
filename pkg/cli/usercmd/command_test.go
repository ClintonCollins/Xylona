package usercmd

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/ClintonCollins/Xylona/pkg/usermgmt"
)

type fakeRunner struct {
	listUsers           []*usermgmt.User
	userByID            *usermgmt.User
	userByUsername      *usermgmt.User
	createdUser         *usermgmt.User
	updatedUser         *usermgmt.User
	createInput         usermgmt.CreateInput
	updateInput         usermgmt.UpdateInput
	deletedUserID       string
	createCalled        bool
	updateCalled        bool
	deleteCalled        bool
	getByIDCalled       bool
	getByUsernameCalled bool
}

func (f *fakeRunner) List(context.Context) ([]*usermgmt.User, error) {
	return f.listUsers, nil
}

func (f *fakeRunner) GetByID(context.Context, string) (*usermgmt.User, error) {
	f.getByIDCalled = true
	return f.userByID, nil
}

func (f *fakeRunner) GetByUsername(context.Context, string) (*usermgmt.User, error) {
	f.getByUsernameCalled = true
	return f.userByUsername, nil
}

func (f *fakeRunner) Create(_ context.Context, input usermgmt.CreateInput) (*usermgmt.User, error) {
	f.createCalled = true
	f.createInput = input
	return f.createdUser, nil
}

func (f *fakeRunner) Update(_ context.Context, input usermgmt.UpdateInput) (*usermgmt.User, error) {
	f.updateCalled = true
	f.updateInput = input
	return f.updatedUser, nil
}

func (f *fakeRunner) Delete(_ context.Context, userID string) error {
	f.deleteCalled = true
	f.deletedUserID = userID
	return nil
}

func TestRunCreateReadsPasswordFromStdin(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	commandStdin = strings.NewReader("stdin-secret\n")
	commandStdout = &bytes.Buffer{}
	commandStderr = &bytes.Buffer{}

	runner := &fakeRunner{
		createdUser: &usermgmt.User{
			ID:       `user-created`,
			UserName: `alice`,
			Email:    `alice@example.com`,
		},
	}

	var capturedFlags modeFlags
	var capturedMutating bool
	promptCalled := false
	interactiveCreateInputFunc = func(*cli.Command) (usermgmt.CreateInput, error) {
		promptCalled = true
		return usermgmt.CreateInput{}, nil
	}
	newRunnerFunc = func(_ context.Context, flags *modeFlags, options Options, mutating bool) (commandRunner, func(), error) {
		capturedFlags = *flags
		capturedMutating = mutating
		if options.DefaultDBPath != `default.sqlite` {
			t.Fatalf("options.DefaultDBPath = %q, want %q", options.DefaultDBPath, `default.sqlite`)
		}
		return runner, func() {}, nil
	}

	errRun := Run(context.Background(), []string{
		`create`,
		`--offline`,
		`--username`, `alice`,
		`--email`, `alice@example.com`,
		`--password-stdin`,
		`--format`, `json`,
	}, Options{DefaultDBPath: `default.sqlite`})
	if errRun != nil {
		t.Fatalf("Run() error = %v", errRun)
	}
	if !runner.createCalled {
		t.Fatal("Create() was not called")
	}
	if runner.createInput.Password != `stdin-secret` {
		t.Fatalf("Create().Password = %q, want %q", runner.createInput.Password, `stdin-secret`)
	}
	if promptCalled {
		t.Fatal("interactive prompt should not be used when --password-stdin is set")
	}
	if !capturedFlags.offline {
		t.Fatal("offline flag was not forwarded to the runner")
	}
	if !capturedMutating {
		t.Fatal("mutating runner path was not selected for create")
	}
}

func TestRunCreateWithoutFlagsUsesInteractivePrompt(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	commandStdout = &bytes.Buffer{}
	commandStderr = &bytes.Buffer{}
	isTerminalFunc = func(int) bool { return true }

	runner := &fakeRunner{
		createdUser: &usermgmt.User{
			ID:        `user-created`,
			UserName:  `alice`,
			Email:     `alice@example.com`,
			FirstName: `Alice`,
			LastName:  `Example`,
			SuperUser: true,
		},
	}

	promptCalled := false
	interactiveCreateInputFunc = func(*cli.Command) (usermgmt.CreateInput, error) {
		promptCalled = true
		return usermgmt.CreateInput{
			UserName:  `alice`,
			Email:     `alice@example.com`,
			FirstName: `Alice`,
			LastName:  `Example`,
			Password:  `prompt-secret`,
			SuperUser: true,
		}, nil
	}

	newRunnerFunc = func(context.Context, *modeFlags, Options, bool) (commandRunner, func(), error) {
		return runner, func() {}, nil
	}

	errRun := Run(context.Background(), []string{`create`}, Options{DefaultDBPath: `default.sqlite`})
	if errRun != nil {
		t.Fatalf("Run() error = %v", errRun)
	}
	if !promptCalled {
		t.Fatal("interactive prompt was not used for bare create")
	}
	if !runner.createCalled {
		t.Fatal("Create() was not called")
	}
	if runner.createInput.UserName != `alice` {
		t.Fatalf("Create().UserName = %q, want %q", runner.createInput.UserName, `alice`)
	}
	if runner.createInput.Email != `alice@example.com` {
		t.Fatalf("Create().Email = %q, want %q", runner.createInput.Email, `alice@example.com`)
	}
	if runner.createInput.Password != `prompt-secret` {
		t.Fatalf("Create().Password = %q, want %q", runner.createInput.Password, `prompt-secret`)
	}
	if !runner.createInput.SuperUser {
		t.Fatal("Create().SuperUser = false, want true")
	}
}

func TestRunCreateWithoutTTYFailsBeforeRunner(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	commandStdout = &bytes.Buffer{}
	commandStderr = &bytes.Buffer{}
	isTerminalFunc = func(int) bool { return false }

	runnerCalled := false
	promptCalled := false
	interactiveCreateInputFunc = func(*cli.Command) (usermgmt.CreateInput, error) {
		promptCalled = true
		return usermgmt.CreateInput{}, nil
	}
	newRunnerFunc = func(context.Context, *modeFlags, Options, bool) (commandRunner, func(), error) {
		runnerCalled = true
		return &fakeRunner{}, func() {}, nil
	}

	errRun := Run(context.Background(), []string{
		`create`,
		`--username`, `alice`,
		`--email`, `alice@example.com`,
	}, Options{DefaultDBPath: `default.sqlite`})
	if errRun == nil {
		t.Fatal("Run() error = nil, want TTY failure")
	}
	if !strings.Contains(errRun.Error(), `requires a TTY`) {
		t.Fatalf("Run() error = %q, want TTY guidance", errRun.Error())
	}
	if runnerCalled {
		t.Fatal("runner should not be created when password prompting fails")
	}
	if promptCalled {
		t.Fatal("interactive prompt should not run without a TTY")
	}
}

func TestRunDeleteWithYesSkipsConfirmation(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	commandStdout = &bytes.Buffer{}
	commandStderr = &bytes.Buffer{}

	runner := &fakeRunner{
		userByUsername: &usermgmt.User{
			ID:       `user-delete`,
			UserName: `alice`,
			Email:    `alice@example.com`,
		},
	}
	newRunnerFunc = func(context.Context, *modeFlags, Options, bool) (commandRunner, func(), error) {
		return runner, func() {}, nil
	}

	errRun := Run(context.Background(), []string{
		`delete`,
		`--yes`,
		`alice`,
	}, Options{DefaultDBPath: `default.sqlite`})
	if errRun != nil {
		t.Fatalf("Run() error = %v", errRun)
	}
	if !runner.getByUsernameCalled {
		t.Fatal("GetByUsername() was not called")
	}
	if !runner.deleteCalled {
		t.Fatal("Delete() was not called")
	}
	if runner.deletedUserID != `user-delete` {
		t.Fatalf("Delete() userID = %q, want %q", runner.deletedUserID, `user-delete`)
	}
}

func TestRunListPrintsJSON(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	stdout := &bytes.Buffer{}
	commandStdout = stdout
	commandStderr = &bytes.Buffer{}

	runner := &fakeRunner{
		listUsers: []*usermgmt.User{
			{
				ID:        `user-admin`,
				UserName:  `admin`,
				Email:     `admin@example.com`,
				SuperUser: true,
			},
		},
	}
	newRunnerFunc = func(context.Context, *modeFlags, Options, bool) (commandRunner, func(), error) {
		return runner, func() {}, nil
	}

	errRun := Run(context.Background(), []string{
		`list`,
		`--format`, `json`,
	}, Options{DefaultDBPath: `default.sqlite`})
	if errRun != nil {
		t.Fatalf("Run() error = %v", errRun)
	}
	if !strings.Contains(stdout.String(), `"username": "admin"`) {
		t.Fatalf("stdout = %q, want JSON user payload", stdout.String())
	}
}

func TestRunHelpPrintsBuiltInHelp(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	stdout := &bytes.Buffer{}
	commandStdout = stdout
	commandStderr = &bytes.Buffer{}

	errRun := Run(context.Background(), []string{`--help`}, Options{DefaultDBPath: `default.sqlite`})
	if errRun != nil {
		t.Fatalf("Run() error = %v", errRun)
	}
	if !strings.Contains(stdout.String(), `xylona user <command> [command options]`) {
		t.Fatalf("stdout = %q, want top-level help usage", stdout.String())
	}
	if !strings.Contains(stdout.String(), `list`) {
		t.Fatalf("stdout = %q, want subcommand listing", stdout.String())
	}
}

func TestRunBareCommandPrintsBuiltInHelp(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	stdout := &bytes.Buffer{}
	commandStdout = stdout
	commandStderr = &bytes.Buffer{}

	errRun := Run(context.Background(), []string{}, Options{DefaultDBPath: `default.sqlite`})
	if errRun != nil {
		t.Fatalf("Run() error = %v", errRun)
	}
	if !strings.Contains(stdout.String(), `xylona user <command> [command options]`) {
		t.Fatalf("stdout = %q, want top-level help usage", stdout.String())
	}
	if !strings.Contains(stdout.String(), `COMMANDS:`) {
		t.Fatalf("stdout = %q, want built-in command help", stdout.String())
	}
}

func TestRunSubcommandHelpPrintsFlagHelp(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	stdout := &bytes.Buffer{}
	commandStdout = stdout
	commandStderr = &bytes.Buffer{}

	errRun := Run(context.Background(), []string{`list`, `--help`}, Options{DefaultDBPath: `default.sqlite`})
	if errRun != nil {
		t.Fatalf("Run() error = %v", errRun)
	}
	if !strings.Contains(stdout.String(), `xylona user list [command options]`) {
		t.Fatalf("stdout = %q, want list help usage", stdout.String())
	}
	if !strings.Contains(stdout.String(), `--format`) {
		t.Fatalf("stdout = %q, want list flag help", stdout.String())
	}
}

func TestNewRunnerOfflineRunsMigrations(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	dbPath := filepath.Join(t.TempDir(), `offline.sqlite`)
	called := false
	sentinel := errors.New(`migrate sentinel`)

	_, cleanup, errRunner := newRunner(context.Background(), &modeFlags{offline: true}, Options{
		DefaultDBPath: dbPath,
		Migrate: func(*sql.DB) error {
			called = true
			return sentinel
		},
	}, false)
	if cleanup != nil {
		cleanup()
	}
	if !called {
		t.Fatal("offline runner did not invoke the migration callback")
	}
	if errRunner == nil {
		t.Fatal("newRunner() error = nil, want migration failure")
	}
	if !errors.Is(errRunner, sentinel) {
		t.Fatalf("newRunner() error = %v, want %v", errRunner, sentinel)
	}
}

func TestNewRunnerOfflineReadWithMigrateUsesGuard(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	dbPath := filepath.Join(t.TempDir(), `offline-guard.sqlite`)
	guardCalled := false
	migrateCalled := false

	guardOfflineMutationFunc = func(context.Context, string) (io.Closer, error) {
		guardCalled = true
		return noopCloser{}, nil
	}

	runner, cleanup, errRunner := newRunner(context.Background(), &modeFlags{offline: true}, Options{
		DefaultDBPath: dbPath,
		Migrate: func(*sql.DB) error {
			migrateCalled = true
			return nil
		},
	}, false)
	if errRunner != nil {
		t.Fatalf("newRunner() error = %v", errRunner)
	}
	if cleanup == nil {
		t.Fatal("newRunner() cleanup = nil, want cleanup")
	}
	cleanup()
	if runner == nil {
		t.Fatal("newRunner() runner = nil, want service runner")
	}
	if !guardCalled {
		t.Fatal("offline read runner did not acquire the offline guard before migrations")
	}
	if !migrateCalled {
		t.Fatal("offline read runner did not invoke migrations")
	}
}

func setCommandTestEnv(t *testing.T) func() {
	t.Helper()

	originalStdin := commandStdin
	originalStdout := commandStdout
	originalStderr := commandStderr
	originalRunner := newRunnerFunc
	originalGuard := guardOfflineMutationFunc
	originalIsTerminal := isTerminalFunc
	originalReadPassword := readPasswordFunc
	originalInteractiveCreateInput := interactiveCreateInputFunc

	return func() {
		commandStdin = originalStdin
		commandStdout = originalStdout
		commandStderr = originalStderr
		newRunnerFunc = originalRunner
		guardOfflineMutationFunc = originalGuard
		isTerminalFunc = originalIsTerminal
		readPasswordFunc = originalReadPassword
		interactiveCreateInputFunc = originalInteractiveCreateInput
	}
}

type noopCloser struct{}

func (noopCloser) Close() error {
	return nil
}
