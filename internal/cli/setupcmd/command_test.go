package setupcmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/firstsetup"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
)

func TestRunCreatesFirstSuperUserFromFlags(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	commandStdin = strings.NewReader("stdin-secret\n")
	stdout := &bytes.Buffer{}
	commandStdout = stdout
	commandStderr = &bytes.Buffer{}
	isTerminalFunc = func(int) bool { return false }

	service := &fakeUserService{
		created: &usermgmt.User{ID: "user-1", UserName: "admin", Email: "admin@localhost", SuperUser: true},
	}
	newUserServiceFunc = func(context.Context, Options, string, bool) (firstUserCreator, func() error, error) {
		return service, func() error { return nil }, nil
	}
	ensureSecretsFunc = func(firstsetup.EnsureSecretsInput) (firstsetup.Secrets, error) {
		return firstsetup.Secrets{}, nil
	}

	errRun := Run(context.Background(), []string{
		"--username", "admin",
		"--password-stdin",
	}, Options{DefaultDBPath: "data.sqlite"})
	if errRun != nil {
		t.Fatalf("Run() error = %v", errRun)
	}
	if service.createCalls != 1 {
		t.Fatalf("CreateFirstSuperUser() calls = %d, want 1", service.createCalls)
	}
	if service.createInput.UserName != "admin" {
		t.Fatalf("Create() username = %q, want admin", service.createInput.UserName)
	}
	if service.createInput.Email != "admin@localhost" {
		t.Fatalf("Create() email = %q, want default admin@localhost", service.createInput.Email)
	}
	if service.createInput.Password != "stdin-secret" {
		t.Fatalf("Create() password = %q, want stdin-secret", service.createInput.Password)
	}
	if !strings.Contains(stdout.String(), "admin") {
		t.Fatalf("stdout = %q, want created username", stdout.String())
	}
}

func TestRunRequiresTTYOrPasswordStdin(t *testing.T) {
	restore := setCommandTestEnv(t)
	defer restore()

	commandStdin = strings.NewReader("")
	commandStdout = io.Discard
	commandStderr = io.Discard
	isTerminalFunc = func(int) bool { return false }
	newUserServiceFunc = func(context.Context, Options, string, bool) (firstUserCreator, func() error, error) {
		t.Fatal("newUserServiceFunc should not be called")
		return nil, nil, nil
	}

	errRun := Run(context.Background(), []string{"--username", "admin"}, Options{DefaultDBPath: "data.sqlite"})
	if errRun == nil {
		t.Fatal("Run() error = nil, want TTY or --password-stdin error")
	}
	if !strings.Contains(errRun.Error(), "--password-stdin") {
		t.Fatalf("Run() error = %q, want --password-stdin mention", errRun.Error())
	}
}

type fakeUserService struct {
	created     *usermgmt.User
	createInput usermgmt.CreateInput
	createCalls int
}

func (f *fakeUserService) CreateFirstSuperUser(input usermgmt.CreateInput) (*usermgmt.User, error) {
	f.createCalls++
	f.createInput = input
	return f.created, nil
}

func setCommandTestEnv(t *testing.T) func() {
	t.Helper()

	originalStdin := commandStdin
	originalStdout := commandStdout
	originalStderr := commandStderr
	originalTerminal := isTerminalFunc
	originalReadPassword := readPasswordFunc
	originalNewService := newUserServiceFunc
	originalEnsure := ensureSecretsFunc
	originalGetwd := getwdFunc
	originalExecutableDir := executableDirFunc

	return func() {
		commandStdin = originalStdin
		commandStdout = originalStdout
		commandStderr = originalStderr
		isTerminalFunc = originalTerminal
		readPasswordFunc = originalReadPassword
		newUserServiceFunc = originalNewService
		ensureSecretsFunc = originalEnsure
		getwdFunc = originalGetwd
		executableDirFunc = originalExecutableDir
	}
}
