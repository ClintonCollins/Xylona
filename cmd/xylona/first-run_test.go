package main

import (
	"net"
	"slices"
	"strings"
	"testing"
)

func TestChooseFirstRunAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		setupNeeded        bool
		isTerminal         bool
		flagsPresent       bool
		setupUsername      string
		setupPasswordStdin bool
		choice             firstRunChoice
		want               firstRunChoice
		wantErr            string
	}{
		{
			name:               "non-interactive flags take precedence",
			flagsPresent:       true,
			setupUsername:      "admin",
			setupPasswordStdin: true,
			want:               firstRunChoiceCLI,
		},
		{
			name:               "non-interactive flags without username fail",
			flagsPresent:       true,
			setupPasswordStdin: true,
			wantErr:            "username is required",
		},
		{
			name:          "non-interactive flags without password on a non-tty fail",
			flagsPresent:  true,
			setupUsername: "admin",
			wantErr:       "--setup-password-stdin",
		},
		{
			name:       "already installed starts the service",
			isTerminal: true,
		},
		{
			name:        "no tty starts awaiting-setup without prompting",
			setupNeeded: true,
		},
		{
			name:        "tty chooser cli",
			setupNeeded: true,
			isTerminal:  true,
			choice:      firstRunChoiceCLI,
			want:        firstRunChoiceCLI,
		},
		{
			name:        "tty chooser browser",
			setupNeeded: true,
			isTerminal:  true,
			choice:      firstRunChoiceBrowser,
			want:        firstRunChoiceBrowser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, errAction := chooseFirstRunAction(
				tt.setupNeeded,
				tt.isTerminal,
				tt.flagsPresent,
				tt.setupUsername,
				tt.setupPasswordStdin,
				tt.choice,
			)
			if tt.wantErr != "" {
				if errAction == nil {
					t.Fatal("chooseFirstRunAction() error = nil, want error")
				}
				if !strings.Contains(errAction.Error(), tt.wantErr) {
					t.Fatalf("chooseFirstRunAction() error = %q, want substring %q", errAction.Error(), tt.wantErr)
				}
				return
			}
			if errAction != nil {
				t.Fatalf("chooseFirstRunAction() error = %v", errAction)
			}
			if got != tt.want {
				t.Fatalf("chooseFirstRunAction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPromptFirstRunChoice(t *testing.T) {
	t.Parallel()

	choice, errPrompt := promptFirstRunChoice(strings.NewReader("2\n"), &strings.Builder{})
	if errPrompt != nil {
		t.Fatalf("promptFirstRunChoice() error = %v", errPrompt)
	}
	if choice != firstRunChoiceBrowser {
		t.Fatalf("promptFirstRunChoice() = %v, want browser", choice)
	}

	_, errInvalid := promptFirstRunChoice(strings.NewReader("nope\n"), &strings.Builder{})
	if errInvalid == nil {
		t.Fatal("promptFirstRunChoice() error = nil, want invalid choice")
	}
	if !strings.Contains(errInvalid.Error(), "choice must be 1 or 2") {
		t.Fatalf("promptFirstRunChoice() error = %q", errInvalid.Error())
	}
}

func TestSetupAccessURLs(t *testing.T) {
	originalBindableIPs := firstRunBindableIPs
	t.Cleanup(func() {
		firstRunBindableIPs = originalBindableIPs
	})
	firstRunBindableIPs = func() ([]net.IP, error) {
		return []net.IP{
			net.ParseIP("fd00::10"),
			net.ParseIP("10.0.0.2"),
		}, nil
	}

	tests := []struct {
		name string
		host string
		want []string
	}{
		{
			name: "empty wildcard lists loopback before sorted interface addresses",
			want: []string{
				"http://127.0.0.1:8080/setup?token=token-value",
				"http://10.0.0.2:8080/setup?token=token-value",
				"http://[fd00::10]:8080/setup?token=token-value",
			},
		},
		{
			name: "IPv4 wildcard lists only IPv4 addresses",
			host: "0.0.0.0",
			want: []string{
				"http://127.0.0.1:8080/setup?token=token-value",
				"http://10.0.0.2:8080/setup?token=token-value",
			},
		},
		{
			name: "IPv6 wildcard lists only IPv6 addresses",
			host: "::",
			want: []string{
				"http://[::1]:8080/setup?token=token-value",
				"http://[fd00::10]:8080/setup?token=token-value",
			},
		},
		{
			name: "explicit host remains authoritative",
			host: "panel.example.com",
			want: []string{"http://panel.example.com:8080/setup?token=token-value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setupAccessURLs(Configuration{Host: tt.host, HTTPPort: 8080}, "token-value")
			if !slices.Equal(got, tt.want) {
				t.Fatalf("setupAccessURLs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShouldLaunchLocalBrowser(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "1.2.3.4 22")
	t.Setenv("DISPLAY", ":0")
	t.Setenv("WAYLAND_DISPLAY", "")
	if shouldLaunchLocalBrowser() {
		t.Fatal("shouldLaunchLocalBrowser() = true, want false over SSH")
	}
}
