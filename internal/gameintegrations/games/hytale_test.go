package games

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestHytaleInstall(t *testing.T) {
	requestPaths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestPaths <- request.URL.Path
		switch request.URL.Path {
		case "/maven-metadata.xml":
			writer.Header().Set("Content-Type", "application/xml")
			_, errWrite := io.WriteString(writer, `<metadata><versioning><latest>0.5.6</latest></versioning></metadata>`)
			if errWrite != nil {
				t.Errorf("write metadata response: %v", errWrite)
			}
		case "/0.5.6/Server-0.5.6.jar":
			_, errWrite := writer.Write([]byte("PK\x03\x04hytale-test-jar"))
			if errWrite != nil {
				t.Errorf("write JAR response: %v", errWrite)
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	directory := t.TempDir()
	game := &Hytale{mavenBaseURL: server.URL, httpClient: server.Client()}
	var output bytes.Buffer
	errInstall := game.Install(&models.GameServer{Directory: directory}, &output, io.Discard)
	if errInstall != nil {
		t.Fatalf("Install() error = %v", errInstall)
	}

	seedJAR, errReadJAR := os.ReadFile(filepath.Join(directory, hytaleSeedJARName))
	if errReadJAR != nil {
		t.Fatalf("read installed seed JAR: %v", errReadJAR)
	}
	if string(seedJAR) != "PK\x03\x04hytale-test-jar" {
		t.Errorf("installed seed JAR = %q", seedJAR)
	}
	launcher, errReadLauncher := os.ReadFile(filepath.Join(directory, hytaleLauncherFileName))
	if errReadLauncher != nil {
		t.Fatalf("read installed launcher: %v", errReadLauncher)
	}
	if !bytes.Equal(launcher, []byte(hytaleLauncherSource)) {
		t.Error("installed launcher does not match the built-in source")
	}
	serverInfo, errServerDir := os.Stat(filepath.Join(directory, "Server"))
	if errServerDir != nil || !serverInfo.IsDir() {
		t.Fatalf("Server directory was not created: info=%v error=%v", serverInfo, errServerDir)
	}
	if !strings.Contains(output.String(), "bootstrap 0.5.6 is ready") {
		t.Errorf("Install() output = %q", output.String())
	}

	for _, expectedPath := range []string{"/maven-metadata.xml", "/0.5.6/Server-0.5.6.jar"} {
		select {
		case actualPath := <-requestPaths:
			if actualPath != expectedPath {
				t.Errorf("request path = %q, want %q", actualPath, expectedPath)
			}
		default:
			t.Fatalf("missing request for %q", expectedPath)
		}
	}
}

func TestReplaceHytaleFile(t *testing.T) {
	directory := t.TempDir()
	targetPath := filepath.Join(directory, "target.jar")
	temporaryPath := filepath.Join(directory, "replacement.jar")
	errTarget := os.WriteFile(targetPath, []byte("old"), 0o600)
	if errTarget != nil {
		t.Fatalf("write target fixture: %v", errTarget)
	}
	errTemporary := os.WriteFile(temporaryPath, []byte("new"), 0o600)
	if errTemporary != nil {
		t.Fatalf("write replacement fixture: %v", errTemporary)
	}

	errReplace := replaceHytaleFile(temporaryPath, targetPath)
	if errReplace != nil {
		t.Fatalf("replaceHytaleFile() error = %v", errReplace)
	}
	contents, errRead := os.ReadFile(targetPath)
	if errRead != nil {
		t.Fatalf("read replaced file: %v", errRead)
	}
	if string(contents) != "new" {
		t.Fatalf("replaced file = %q, want %q", contents, "new")
	}
	_, errTemporaryStat := os.Stat(temporaryPath)
	if !errors.Is(errTemporaryStat, os.ErrNotExist) {
		t.Fatalf("replacement temporary file still exists: %v", errTemporaryStat)
	}
}

func TestHytaleLatestVersionValidation(t *testing.T) {
	tests := []struct {
		name      string
		metadata  string
		want      string
		wantError bool
	}{
		{
			name:     "latest version",
			metadata: `<metadata><versioning><latest>0.5.6</latest><release>0.5.5</release></versioning></metadata>`,
			want:     "0.5.6",
		},
		{
			name:     "release fallback",
			metadata: `<metadata><versioning><release>0.5.5</release></versioning></metadata>`,
			want:     "0.5.5",
		},
		{
			name:      "unsafe path",
			metadata:  `<metadata><versioning><latest>../escape</latest></versioning></metadata>`,
			wantError: true,
		},
		{
			name:      "missing version",
			metadata:  `<metadata><versioning></versioning></metadata>`,
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, errWrite := io.WriteString(writer, test.metadata)
				if errWrite != nil {
					t.Errorf("write metadata response: %v", errWrite)
				}
			}))
			t.Cleanup(server.Close)

			game := &Hytale{mavenBaseURL: server.URL, httpClient: server.Client()}
			version, errVersion := game.latestVersion()
			if test.wantError {
				if errVersion == nil {
					t.Fatalf("latestVersion() = %q, want error", version)
				}
				return
			}
			if errVersion != nil {
				t.Fatalf("latestVersion() error = %v", errVersion)
			}
			if version != test.want {
				t.Errorf("latestVersion() = %q, want %q", version, test.want)
			}
		})
	}
}

func TestApplyHytaleStagedUpdate(t *testing.T) {
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "Server", "config.json"), []byte(`{"ServerName":"preserve"}`))
	writeTestFile(t, filepath.Join(directory, "Server", "universe", "world.dat"), []byte("world-data"))
	writeTestFile(t, filepath.Join(directory, "Server", "Licenses", "old.txt"), []byte("old"))
	writeTestFile(t, filepath.Join(directory, "updater", "staging", "Server", "HytaleServer.jar"), []byte("new-jar"))
	writeTestFile(t, filepath.Join(directory, "updater", "staging", "Server", "Licenses", "new.txt"), []byte("new"))
	writeTestFile(t, filepath.Join(directory, "updater", "staging", "Assets.zip"), []byte("new-assets"))

	errApply := applyHytaleStagedUpdate(directory)
	if errApply != nil {
		t.Fatalf("applyHytaleStagedUpdate() error = %v", errApply)
	}

	assertTestFile(t, filepath.Join(directory, "Server", "HytaleServer.jar"), "new-jar")
	assertTestFile(t, filepath.Join(directory, "Assets.zip"), "new-assets")
	assertTestFile(t, filepath.Join(directory, "Server", "config.json"), `{"ServerName":"preserve"}`)
	assertTestFile(t, filepath.Join(directory, "Server", "universe", "world.dat"), "world-data")
	assertTestFile(t, filepath.Join(directory, "Server", "Licenses", "new.txt"), "new")
	_, errOldLicense := os.Stat(filepath.Join(directory, "Server", "Licenses", "old.txt"))
	if !os.IsNotExist(errOldLicense) {
		t.Errorf("old license still exists, error = %v", errOldLicense)
	}
	_, errStaging := os.Stat(filepath.Join(directory, "updater", "staging"))
	if !os.IsNotExist(errStaging) {
		t.Errorf("staging directory still exists, error = %v", errStaging)
	}
}

func TestValidateHytaleUpdateEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		environment map[string]string
		wantError   bool
	}{
		{name: "both tokens", environment: map[string]string{hytaleSessionTokenEnv: "session", hytaleIdentityTokenEnv: "identity"}},
		{name: "missing session", environment: map[string]string{hytaleIdentityTokenEnv: "identity"}, wantError: true},
		{name: "missing identity", environment: map[string]string{hytaleSessionTokenEnv: "session"}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			errValidate := validateHytaleUpdateEnvironment(test.environment)
			if test.wantError && errValidate == nil {
				t.Fatal("validateHytaleUpdateEnvironment() error = nil")
			}
			if !test.wantError && errValidate != nil {
				t.Fatalf("validateHytaleUpdateEnvironment() error = %v", errValidate)
			}
		})
	}
}

func TestHytaleBootstrapEnvironmentIsolatesHostAndLaunchVariables(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"LANG=en_US.UTF-8",
		"JWT_SECRET_KEY_BASE64=controller-secret",
		hytaleSessionTokenEnv + "=old-session",
		hytaleIdentityTokenEnv + "=old-identity",
	}
	got := hytaleBootstrapEnvironment(base, map[string]string{
		hytaleSessionTokenEnv:    "new-session",
		hytaleIdentityTokenEnv:   "new-identity",
		"UNRELATED_GAME_SETTING": "must-not-leak",
	})

	values := make(map[string]string)
	for _, entry := range got {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		values[name] = value
	}
	if values[hytaleSessionTokenEnv] != "new-session" {
		t.Fatalf("session token environment = %v, want one new value", values[hytaleSessionTokenEnv])
	}
	if values[hytaleIdentityTokenEnv] != "new-identity" {
		t.Fatalf("identity token environment = %v, want one new value", values[hytaleIdentityTokenEnv])
	}
	if values["PATH"] != "/usr/bin" || values["LANG"] != "en_US.UTF-8" {
		t.Fatalf("safe host environment = %v, want PATH and LANG preserved", values)
	}
	for _, key := range []string{"JWT_SECRET_KEY_BASE64", "UNRELATED_GAME_SETTING"} {
		_, exists := values[key]
		if exists {
			t.Fatalf("bootstrap environment unexpectedly contains %s", key)
		}
	}
}

func TestHytaleLauncherCompiles(t *testing.T) {
	javac, errLookPath := exec.LookPath("javac")
	if errLookPath != nil {
		t.Skip("javac is not installed")
	}
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, hytaleLauncherFileName)
	errWrite := os.WriteFile(sourcePath, []byte(hytaleLauncherSource), 0o600)
	if errWrite != nil {
		t.Fatalf("write launcher source: %v", errWrite)
	}
	command := exec.CommandContext(context.Background(), javac, "-d", directory, sourcePath) // #nosec G204 -- javac path is resolved locally and all other paths are test-owned.
	output, errCompile := command.CombinedOutput()
	if errCompile != nil {
		t.Fatalf("javac failed: %v\n%s", errCompile, output)
	}
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	errMkdir := os.MkdirAll(filepath.Dir(path), 0o750)
	if errMkdir != nil {
		t.Fatalf("create test directory: %v", errMkdir)
	}
	errWrite := os.WriteFile(path, data, 0o600)
	if errWrite != nil {
		t.Fatalf("write test file: %v", errWrite)
	}
}

func assertTestFile(t *testing.T, path string, want string) {
	t.Helper()
	data, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("read %s: %v", path, errRead)
	}
	if string(data) != want {
		t.Errorf("%s = %q, want %q", path, data, want)
	}
}
