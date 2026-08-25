package startargs

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func testDefinition(templateJSON string, blocklistJSON string) DefinitionConfig {
	return DefinitionConfig{
		LinuxTemplateJSON: templateJSON,
		LinuxBaseCommand:  "runner",
		BlocklistJSON:     blocklistJSON,
	}
}

func TestDefinitionConfigForGOOS(t *testing.T) {
	definition := DefinitionConfig{
		LinuxTemplateJSON:   "linux-template",
		LinuxBaseCommand:    "linux-command",
		WindowsTemplateJSON: "windows-template",
		WindowsBaseCommand:  "windows-command",
		BlocklistJSON:       "blocklist",
	}
	tests := []struct {
		name                string
		goos                string
		baseCommandOverride string
		want                TargetConfig
	}{
		{
			name: "linux configuration",
			goos: "linux",
			want: TargetConfig{
				templateJSON:  "linux-template",
				baseCommand:   "linux-command",
				blocklistJSON: "blocklist",
			},
		},
		{
			name: "normalized windows configuration",
			goos: " WINDOWS ",
			want: TargetConfig{
				templateJSON:  "windows-template",
				baseCommand:   "windows-command",
				blocklistJSON: "blocklist",
			},
		},
		{
			name: "non-windows uses linux configuration",
			goos: "darwin",
			want: TargetConfig{
				templateJSON:  "linux-template",
				baseCommand:   "linux-command",
				blocklistJSON: "blocklist",
			},
		},
		{
			name:                "trimmed override wins",
			goos:                "windows",
			baseCommandOverride: " custom-command ",
			want: TargetConfig{
				templateJSON:  "windows-template",
				baseCommand:   "custom-command",
				blocklistJSON: "blocklist",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := definition.ForGOOS(tt.goos, tt.baseCommandOverride)
			if got != tt.want {
				t.Errorf("ForGOOS() = %#v, want %#v", got, tt.want)
			}
			if got.ConfiguredBaseCommand() != tt.want.baseCommand {
				t.Errorf("ConfiguredBaseCommand() = %q, want %q", got.ConfiguredBaseCommand(), tt.want.baseCommand)
			}
		})
	}
}

func TestResolveServer(t *testing.T) {
	tests := []struct {
		name        string
		config      ServerConfig
		want        ResolvedCommand
		wantErr     error
		wantErrText string
	}{
		{
			name:        "missing template",
			config:      ServerConfig{},
			wantErr:     ErrStartCommandTemplateMissing,
			wantErrText: ErrStartCommandTemplateMissing.Error(),
		},
		{
			name: "missing base command",
			config: ServerConfig{
				Definition: DefinitionConfig{LinuxTemplateJSON: `[{"id":"arg","tokens":["value"]}]`},
			},
			wantErr:     ErrBaseCommandMissing,
			wantErrText: ErrBaseCommandMissing.Error(),
		},
		{
			name: "selects windows command and template",
			config: ServerConfig{
				Definition: DefinitionConfig{
					LinuxTemplateJSON:   `[{"id":"platform","tokens":["linux"]}]`,
					LinuxBaseCommand:    "linux-runner",
					WindowsTemplateJSON: `[{"id":"platform","tokens":["windows"]}]`,
					WindowsBaseCommand:  "windows-runner",
				},
				GOOS: "windows",
			},
			want: ResolvedCommand{BaseCommand: "windows-runner", Args: []string{"windows"}},
		},
		{
			name: "resolves placeholders in base command and arguments",
			config: ServerConfig{
				Definition: DefinitionConfig{
					LinuxTemplateJSON: `[{"id":"jar","tokens":["-jar","{{SERVER_EXECUTABLE}}"]}]`,
					LinuxBaseCommand:  "{{JAVA_HOME}}/bin/java",
				},
				Variables: map[string]string{
					"JAVA_HOME":         "/opt/java",
					"SERVER_EXECUTABLE": "paper.jar",
				},
			},
			want: ResolvedCommand{BaseCommand: "/opt/java/bin/java", Args: []string{"-jar", "paper.jar"}},
		},
		{
			name: "override takes precedence and resolves placeholders",
			config: ServerConfig{
				Definition: DefinitionConfig{
					LinuxTemplateJSON: `[{"id":"arg","tokens":["value"]}]`,
					LinuxBaseCommand:  "definition-runner",
				},
				BaseCommandOverride: " {{CUSTOM_RUNNER}} ",
				Variables:           map[string]string{"CUSTOM_RUNNER": "custom-runner"},
			},
			want: ResolvedCommand{BaseCommand: "custom-runner", Args: []string{"value"}},
		},
		{
			name: "flattens ordered blocks",
			config: ServerConfig{
				Definition: testDefinition(`[{"id":"tail","order":2,"ownership":"editable","tokens":["nogui"]},{"id":"jar","order":1,"ownership":"locked","tokens":["-jar","server.jar"]}]`, ""),
			},
			want: ResolvedCommand{BaseCommand: "runner", Args: []string{"-jar", "server.jar", "nogui"}},
		},
		{
			name: "keeps placeholder values with spaces in one argument",
			config: ServerConfig{
				Definition: testDefinition(`[{"id":"name","tokens":["--name={{SERVER_NAME}}"]}]`, ""),
				Variables:  map[string]string{"SERVER_NAME": "My Great Server"},
			},
			want: ResolvedCommand{BaseCommand: "runner", Args: []string{"--name=My Great Server"}},
		},
		{
			name: "applies edit remove and chained add patches",
			config: ServerConfig{
				Definition:  testDefinition(`[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"]},{"id":"remove","order":2,"ownership":"editable","tokens":["old"]},{"id":"tail","order":3,"ownership":"editable","tokens":["nogui"]}]`, ""),
				PatchesJSON: `[{"id":"prefix","op":"add","tokens":["prefix"]},{"id":"heap","op":"edit","tokens":["-Xmx8G"]},{"id":"remove","op":"remove"},{"id":"gc","op":"add","tokens":["-XX:+UseG1GC"],"afterId":"heap"},{"id":"log","op":"add","tokens":["--log={{PORT}}"],"afterId":"gc"}]`,
				Variables:   map[string]string{"PORT": "25565"},
			},
			want: ResolvedCommand{BaseCommand: "runner", Args: []string{"prefix", "-Xmx8G", "-XX:+UseG1GC", "--log=25565", "nogui"}},
		},
		{
			name: "preserves add order at the same anchor",
			config: ServerConfig{
				Definition:  testDefinition(`[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"]}]`, ""),
				PatchesJSON: `[{"id":"g1","op":"add","tokens":["-XX:+UseG1GC"],"afterId":"heap"},{"id":"z","op":"add","tokens":["-XX:+UseZGC"],"afterId":"heap"}]`,
			},
			want: ResolvedCommand{BaseCommand: "runner", Args: []string{"-Xmx2G", "-XX:+UseG1GC", "-XX:+UseZGC"}},
		},
		{
			name: "skips orphaned persisted patches",
			config: ServerConfig{
				Definition:  testDefinition(`[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"]}]`, ""),
				PatchesJSON: `[{"id":"missing","op":"edit","tokens":["-Xmx8G"]},{"id":"missing-remove","op":"remove"}]`,
			},
			want: ResolvedCommand{BaseCommand: "runner", Args: []string{"-Xmx2G"}},
		},
		{
			name: "skips add anchored to a removed block",
			config: ServerConfig{
				Definition:  testDefinition(`[{"id":"removed","order":1,"ownership":"editable","tokens":["old"]},{"id":"tail","order":2,"ownership":"editable","tokens":["nogui"]}]`, ""),
				PatchesJSON: `[{"id":"removed","op":"remove"},{"id":"orphan","op":"add","tokens":["unused"],"afterId":"removed"}]`,
			},
			want: ResolvedCommand{BaseCommand: "runner", Args: []string{"nogui"}},
		},
		{
			name: "rejects malformed persisted patches",
			config: ServerConfig{
				Definition:  testDefinition(`[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"]}]`, ""),
				PatchesJSON: `[{`,
			},
			wantErrText: "parse start arg patches",
		},
		{
			name: "checks resolved arguments against blocklist",
			config: ServerConfig{
				Definition: testDefinition(
					`[{"id":"debug","order":1,"ownership":"editable","tokens":["{{DEBUG_ARG}}"]}]`,
					`[{"pattern":"^-agentlib:","reason":"debug agent"}]`,
				),
				Variables: map[string]string{"DEBUG_ARG": "-agentlib:jdwp"},
			},
			wantErrText: `blocked start argument "-agentlib:jdwp": debug agent`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errResolve := ResolveServer(tt.config)
			if tt.wantErr != nil && !errors.Is(errResolve, tt.wantErr) {
				t.Fatalf("ResolveServer() error = %v, want %v", errResolve, tt.wantErr)
			}
			if tt.wantErrText != "" {
				if errResolve == nil || !strings.Contains(errResolve.Error(), tt.wantErrText) {
					t.Fatalf("ResolveServer() error = %v, want containing %q", errResolve, tt.wantErrText)
				}
				return
			}
			if errResolve != nil {
				t.Fatalf("ResolveServer() error = %v", errResolve)
			}
			if got.BaseCommand != tt.want.BaseCommand || !slices.Equal(got.Args, tt.want.Args) {
				t.Errorf("ResolveServer() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
