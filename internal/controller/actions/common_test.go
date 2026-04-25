package actions

import (
	"path/filepath"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestDetectOperatingSystem(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		want   OSType
		wantOK bool
	}{
		{name: "windows", goos: "windows", want: Windows, wantOK: true},
		{name: "linux", goos: "linux", want: Linux, wantOK: true},
		{name: "darwin", goos: "darwin", want: Darwin, wantOK: true},
		{name: "unsupported", goos: "plan9", want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := detectOperatingSystem(tt.goos)
			if ok != tt.wantOK {
				t.Fatalf("detectOperatingSystem(%q) ok = %v, want %v", tt.goos, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("detectOperatingSystem(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		})
	}
}

func TestInitOperatingSystemReturnsErrorForUnsupportedOS(t *testing.T) {
	originalOS := OperatingSystem
	t.Cleanup(func() {
		OperatingSystem = originalOS
	})

	errInitOperatingSystem := initOperatingSystem("plan9")
	if errInitOperatingSystem == nil {
		t.Fatal("initOperatingSystem() error = nil, want error")
	}
	if OperatingSystem != originalOS {
		t.Fatalf("initOperatingSystem() changed OperatingSystem to %q, want %q", OperatingSystem, originalOS)
	}
}

func TestDefaultInstallPath(t *testing.T) {
	originalOS := OperatingSystem
	t.Cleanup(func() {
		OperatingSystem = originalOS
	})

	t.Setenv("HOME", "/home/tester")
	t.Setenv("USER", "tester")
	t.Setenv("USERPROFILE", `C:\Users\tester`)

	tests := []struct {
		name string
		os   OSType
		want string
	}{
		{name: "linux", os: Linux, want: "/home/tester/xylona"},
		{name: "darwin", os: Darwin, want: "/home/tester/xylona"},
		{name: "windows", os: Windows, want: filepath.Join(`C:\Users\tester`, "Xylona")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			OperatingSystem = tt.os
			got, errDefaultInstallPath := DefaultInstallPath()
			if errDefaultInstallPath != nil {
				t.Fatalf("DefaultInstallPath() error = %v", errDefaultInstallPath)
			}
			if got != tt.want {
				t.Errorf("DefaultInstallPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultInstallPathReturnsErrorWhenUserHomeIsMissing(t *testing.T) {
	originalOS := OperatingSystem
	t.Cleanup(func() {
		OperatingSystem = originalOS
	})

	OperatingSystem = Linux
	t.Setenv("HOME", "")
	t.Setenv("USER", "")

	got, errDefaultInstallPath := DefaultInstallPath()
	if errDefaultInstallPath == nil {
		t.Fatal("DefaultInstallPath() error = nil, want error")
	}
	if got != "" {
		t.Fatalf("DefaultInstallPath() path = %q, want empty", got)
	}
}

func TestJoinForNodeOS(t *testing.T) {
	tests := []struct {
		name   string
		nodeOS OSType
		parts  []string
		want   string
	}{
		{
			name:   "linux root joined with user and slug",
			nodeOS: Linux,
			parts:  []string{"/home/clinton/xylona", "clinton", "media-minecraft"},
			want:   "/home/clinton/xylona/clinton/media-minecraft",
		},
		{
			name:   "linux trims trailing slashes and skips empty segments",
			nodeOS: Linux,
			parts:  []string{"/home/clinton/xylona/", "", "clinton/", "/media-minecraft"},
			want:   "/home/clinton/xylona/clinton/media-minecraft",
		},
		{
			name:   "darwin uses unix separator",
			nodeOS: Darwin,
			parts:  []string{"/Users/clinton/xylona", "clinton", "media-minecraft"},
			want:   "/Users/clinton/xylona/clinton/media-minecraft",
		},
		{
			name:   "windows uses backslash separator",
			nodeOS: Windows,
			parts:  []string{`C:\Users\Clinton\Xylona`, "Clinton", "media-minecraft"},
			want:   `C:\Users\Clinton\Xylona\Clinton\media-minecraft`,
		},
		{
			name:   "windows normalizes mixed separators on input",
			nodeOS: Windows,
			parts:  []string{`C:\Users\Clinton\Xylona/`, "/Clinton/", "media-minecraft/"},
			want:   `C:\Users\Clinton\Xylona\Clinton\media-minecraft`,
		},
		{
			name:   "unknown OS falls back to unix separator",
			nodeOS: OSType(""),
			parts:  []string{"/opt/xylona", "clinton", "server"},
			want:   "/opt/xylona/clinton/server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinForNodeOS(tt.nodeOS, tt.parts...)
			if got != tt.want {
				t.Errorf("joinForNodeOS(%q, %q) = %q, want %q", tt.nodeOS, tt.parts, got, tt.want)
			}
		})
	}
}

func TestGameCommandSelectionByOperatingSystem(t *testing.T) {
	game := &models.Game{
		LinuxBaseCommand:          "linux-base",
		LinuxStartArgsTemplate:    null.From("linux-template"),
		LinuxStopCommand:          "linux-stop",
		LinuxInstallCommand:       "linux-install",
		LinuxInstallCommandType:   "bash",
		LinuxUpdateCommand:        "linux-update",
		LinuxUpdateCommandType:    "bash",
		WindowsBaseCommand:        "windows-base",
		WindowsStartArgsTemplate:  null.From("windows-template"),
		WindowsStopCommand:        "windows-stop",
		WindowsInstallCommand:     "windows-install",
		WindowsInstallCommandType: "cmd",
		WindowsUpdateCommand:      "windows-update",
		WindowsUpdateCommandType:  "cmd",
	}

	tests := []struct {
		name         string
		nodeOS       OSType
		wantBase     string
		wantTemplate string
		wantStop     string
		wantInstall  string
		wantType     string
		wantUpdate   string
		wantUpdType  string
	}{
		{
			name:         "linux uses unix commands",
			nodeOS:       Linux,
			wantBase:     "linux-base",
			wantTemplate: "linux-template",
			wantStop:     "linux-stop",
			wantInstall:  "linux-install",
			wantType:     "bash",
			wantUpdate:   "linux-update",
			wantUpdType:  "bash",
		},
		{
			name:         "darwin uses unix commands",
			nodeOS:       Darwin,
			wantBase:     "linux-base",
			wantTemplate: "linux-template",
			wantStop:     "linux-stop",
			wantInstall:  "linux-install",
			wantType:     "bash",
			wantUpdate:   "linux-update",
			wantUpdType:  "bash",
		},
		{
			name:         "windows uses windows commands",
			nodeOS:       Windows,
			wantBase:     "windows-base",
			wantTemplate: "windows-template",
			wantStop:     "windows-stop",
			wantInstall:  "windows-install",
			wantType:     "cmd",
			wantUpdate:   "windows-update",
			wantUpdType:  "cmd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gameBaseCommand(game, tt.nodeOS); got != tt.wantBase {
				t.Errorf("gameBaseCommand() = %q, want %q", got, tt.wantBase)
			}
			if got := gameStartArgsTemplate(game, tt.nodeOS); got != tt.wantTemplate {
				t.Errorf("gameStartArgsTemplate() = %q, want %q", got, tt.wantTemplate)
			}
			if got := gameStopCommand(game, tt.nodeOS); got != tt.wantStop {
				t.Errorf("gameStopCommand() = %q, want %q", got, tt.wantStop)
			}
			if got := gameInstallCommand(game, tt.nodeOS); got != tt.wantInstall {
				t.Errorf("gameInstallCommand() = %q, want %q", got, tt.wantInstall)
			}
			if got := gameInstallCommandType(game, tt.nodeOS); got != tt.wantType {
				t.Errorf("gameInstallCommandType() = %q, want %q", got, tt.wantType)
			}
			if got := gameUpdateCommand(game, tt.nodeOS); got != tt.wantUpdate {
				t.Errorf("gameUpdateCommand() = %q, want %q", got, tt.wantUpdate)
			}
			if got := gameUpdateCommandType(game, tt.nodeOS); got != tt.wantUpdType {
				t.Errorf("gameUpdateCommandType() = %q, want %q", got, tt.wantUpdType)
			}
		})
	}
}
