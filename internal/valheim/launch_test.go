package valheim

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func nativeArgs() []string {
	return []string{"-name", "Test server", "-world", "世界", "-password", "example-secret", "-port", "2456", "-savedir", "data", "-instanceid", "server", "-logFile", "-", "-nographics", "-batchmode"}
}

func TestValheimNativeLaunch(t *testing.T) {
	for _, platform := range []string{"linux", "windows"} {
		t.Run(platform, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "Server 世界 with spaces")
			errDirectory := os.MkdirAll(filepath.Join(root, "linux64"), 0o750)
			if errDirectory != nil {
				t.Fatal(errDirectory)
			}
			executable := "valheim_server.exe"
			if platform == "linux" {
				executable = "valheim_server.x86_64"
			}
			for _, file := range []string{executable, "UnityPlayer.so", "linux64/steamclient.so"} {
				errWrite := os.WriteFile(filepath.Join(root, file), []byte("fixture"), 0o600)
				if errWrite != nil {
					t.Fatal(errWrite)
				}
			}
			actualRoot, actualExe, environment, errPrepare := PrepareNative(platform, root, executable, "server", nativeArgs(), nil)
			if errPrepare != nil {
				t.Fatal(errPrepare)
			}
			if actualRoot != root || actualExe != filepath.Join(root, executable) || environment["SteamAppId"] != "892970" {
				t.Fatal("incorrect native invocation")
			}
			if platform == "linux" && environment["LD_LIBRARY_PATH"] != filepath.Join(root, "linux64") {
				t.Fatal("library path is not node-owned")
			}
			for _, args := range [][]string{slices.Delete(nativeArgs(), 4, 6), slices.Replace(nativeArgs(), 5, 6, "")} {
				_, _, _, errUnprotected := PrepareNative(platform, root, executable, "server", args, nil)
				if errUnprotected != nil {
					t.Fatalf("password-free native launch rejected: %v", errUnprotected)
				}
			}
			_, _, _, errEnvironment := PrepareNative(platform, root, executable, "server", nativeArgs(), map[string]string{"SteamAppId": "1"})
			if errEnvironment == nil {
				t.Fatal("accepted wrong Steam app identity")
			}
		})
	}
}

func TestValheimManagedArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"duplicate save root", append(nativeArgs(), "-savedir", "elsewhere")},
		{"equals bypass", append(nativeArgs(), "-PORT=1234")},
		{"duplicate secret", append(nativeArgs(), "-password", "other-secret")},
		{"world traversal", slices.Replace(nativeArgs(), 3, 4, "../world")},
		{"port overflow", slices.Replace(nativeArgs(), 7, 8, "65535")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ValidateArgs(tc.args, "server") == nil {
				t.Fatal("accepted unsafe arguments")
			}
		})
	}
}

func TestValheimOptionalPassword(t *testing.T) {
	for _, test := range []struct {
		name      string
		args      []string
		wantError bool
	}{
		{name: "omitted", args: slices.Delete(nativeArgs(), 4, 6)},
		{name: "empty", args: slices.Replace(nativeArgs(), 5, 6, "")},
		{name: "configured", args: nativeArgs()},
		{name: "too short", args: slices.Replace(nativeArgs(), 5, 6, "tiny"), wantError: true},
		{name: "contained in server name", args: slices.Replace(nativeArgs(), 5, 6, "Test server"), wantError: true},
		{name: "missing value", args: append(slices.Delete(nativeArgs(), 4, 6), "-password"), wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			errArgs := ValidateArgs(test.args, "server")
			if (errArgs != nil) != test.wantError {
				t.Fatalf("ValidateArgs() = %v, want error %v", errArgs, test.wantError)
			}
		})
	}
}
