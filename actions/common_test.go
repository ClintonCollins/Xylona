package actions

import (
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
		{name: "windows", os: Windows, want: `C:\Users\tester/Xylona`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			OperatingSystem = tt.os
			got := DefaultInstallPath()
			if got != tt.want {
				t.Errorf("DefaultInstallPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGameCommandSelectionByOperatingSystem(t *testing.T) {
	originalOS := OperatingSystem
	t.Cleanup(func() {
		OperatingSystem = originalOS
	})

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
		os           OSType
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
			os:           Linux,
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
			os:           Darwin,
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
			os:           Windows,
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
			OperatingSystem = tt.os

			if got := gameBaseCommand(game); got != tt.wantBase {
				t.Errorf("gameBaseCommand() = %q, want %q", got, tt.wantBase)
			}
			if got := gameStartArgsTemplate(game); got != tt.wantTemplate {
				t.Errorf("gameStartArgsTemplate() = %q, want %q", got, tt.wantTemplate)
			}
			if got := gameStopCommand(game); got != tt.wantStop {
				t.Errorf("gameStopCommand() = %q, want %q", got, tt.wantStop)
			}
			if got := gameInstallCommand(game); got != tt.wantInstall {
				t.Errorf("gameInstallCommand() = %q, want %q", got, tt.wantInstall)
			}
			if got := gameInstallCommandType(game); got != tt.wantType {
				t.Errorf("gameInstallCommandType() = %q, want %q", got, tt.wantType)
			}
			if got := gameUpdateCommand(game); got != tt.wantUpdate {
				t.Errorf("gameUpdateCommand() = %q, want %q", got, tt.wantUpdate)
			}
			if got := gameUpdateCommandType(game); got != tt.wantUpdType {
				t.Errorf("gameUpdateCommandType() = %q, want %q", got, tt.wantUpdType)
			}
		})
	}
}
