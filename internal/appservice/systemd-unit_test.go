package appservice

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/selfupdate"
)

func TestBuildSystemdUnit(t *testing.T) {
	executablePath := filepath.Join(t.TempDir(), "Xylona $Node%")
	definition := Definition{
		Name:             "XylonaNode",
		UnitName:         "xylona-node.service",
		DisplayName:      "Xylona Node",
		Description:      "Xylona remote game server node",
		ExecutablePath:   executablePath,
		WorkingDirectory: filepath.Dir(executablePath),
		Arguments: []string{
			"--listen",
			":9500",
			"--data-dir",
			filepath.Join(t.TempDir(), "node data"),
		},
	}
	account := Account{
		Username:       "alice",
		UID:            "1000",
		PrimaryGroup:   "games",
		PrimaryGroupID: "1000",
	}

	unit, errUnit := buildSystemdUnit(definition, account)
	if errUnit != nil {
		t.Fatalf("buildSystemdUnit() error = %v", errUnit)
	}
	required := []string{
		systemdManagedMarker,
		"Description=Xylona remote game server node",
		"Wants=network-online.target",
		"After=network-online.target",
		"StartLimitIntervalSec=5min",
		"StartLimitBurst=5",
		"User=alice",
		"Group=games",
		"Environment=\"" + selfupdate.RestartModeEnvironment + "=" + string(selfupdate.RestartModeSelf) + "\"",
		"Restart=on-failure",
		"RestartSec=10s",
		"TimeoutStopSec=2min",
		"WantedBy=multi-user.target",
	}
	for _, expected := range required {
		if !strings.Contains(unit, expected) {
			t.Fatalf("systemd unit does not contain %q:\n%s", expected, unit)
		}
	}
	if strings.Contains(unit, "join-token") || strings.Contains(unit, "controller-url") {
		t.Fatalf("systemd unit contains bootstrap-only configuration:\n%s", unit)
	}
	if !strings.Contains(unit, "$$Node%%") {
		t.Fatalf("ExecStart did not escape dollar/percent expansion:\n%s", unit)
	}
}

func TestQuoteSystemdValue(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		execValue bool
		want      string
		wantError bool
	}{
		{name: "setting preserves dollar", value: "/srv/$x%y", want: `"/srv/$x%%y"`},
		{name: "exec escapes dollar", value: "/srv/$x%y", execValue: true, want: `"/srv/$$x%%y"`},
		{name: "quotes and backslashes", value: `C:\Xylona "Node"`, want: `"C:\\Xylona \"Node\""`},
		{name: "newline rejected", value: "bad\nvalue", wantError: true},
		{name: "NUL rejected", value: "bad\x00value", wantError: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got string
			var errQuote error
			if testCase.execValue {
				got, errQuote = quoteSystemdExecArgument(testCase.value)
			} else {
				got, errQuote = quoteSystemdValue(testCase.value)
			}
			if testCase.wantError {
				if errQuote == nil {
					t.Fatal("quote systemd value returned no error")
				}
				return
			}
			if errQuote != nil {
				t.Fatalf("quote systemd value error = %v", errQuote)
			}
			if got != testCase.want {
				t.Fatalf("quoted value = %q, want %q", got, testCase.want)
			}
		})
	}
}
