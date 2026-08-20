package startargs

import (
	"slices"
	"strings"
	"testing"
)

func TestResolveServer(t *testing.T) {
	tests := []struct {
		name        string
		config      ServerConfig
		wantArgs    []string
		wantErrText string
	}{
		{
			name:     "empty template returns empty args",
			config:   ServerConfig{},
			wantArgs: []string{},
		},
		{
			name: "flattens ordered blocks and resolves placeholders",
			config: ServerConfig{
				TemplateJSON: `[{"id":"tail","order":2,"ownership":"editable","tokens":["nogui"]},{"id":"jar","order":1,"ownership":"locked","tokens":["-jar","{{SERVER_EXECUTABLE}}"]}]`,
				Variables:    map[string]string{"SERVER_EXECUTABLE": "paper.jar"},
			},
			wantArgs: []string{"-jar", "paper.jar", "nogui"},
		},
		{
			name: "applies edit remove and chained add patches",
			config: ServerConfig{
				TemplateJSON: `[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"]},{"id":"remove","order":2,"ownership":"editable","tokens":["old"]},{"id":"tail","order":3,"ownership":"editable","tokens":["nogui"]}]`,
				PatchesJSON:  `[{"id":"prefix","op":"add","tokens":["prefix"]},{"id":"heap","op":"edit","tokens":["-Xmx8G"]},{"id":"remove","op":"remove"},{"id":"gc","op":"add","tokens":["-XX:+UseG1GC"],"afterId":"heap"},{"id":"log","op":"add","tokens":["--log={{PORT}}"],"afterId":"gc"}]`,
				Variables:    map[string]string{"PORT": "25565"},
			},
			wantArgs: []string{"prefix", "-Xmx8G", "-XX:+UseG1GC", "--log=25565", "nogui"},
		},
		{
			name: "preserves add order at the same anchor",
			config: ServerConfig{
				TemplateJSON: `[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"]}]`,
				PatchesJSON:  `[{"id":"g1","op":"add","tokens":["-XX:+UseG1GC"],"afterId":"heap"},{"id":"z","op":"add","tokens":["-XX:+UseZGC"],"afterId":"heap"}]`,
			},
			wantArgs: []string{"-Xmx2G", "-XX:+UseG1GC", "-XX:+UseZGC"},
		},
		{
			name: "skips orphaned edit and remove patches",
			config: ServerConfig{
				TemplateJSON: `[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"]}]`,
				PatchesJSON:  `[{"id":"missing","op":"edit","tokens":["-Xmx8G"]},{"id":"missing-remove","op":"remove"}]`,
			},
			wantArgs: []string{"-Xmx2G"},
		},
		{
			name: "skips add anchored to a removed block",
			config: ServerConfig{
				TemplateJSON: `[{"id":"removed","order":1,"ownership":"editable","tokens":["old"]},{"id":"tail","order":2,"ownership":"editable","tokens":["nogui"]}]`,
				PatchesJSON:  `[{"id":"removed","op":"remove"},{"id":"orphan","op":"add","tokens":["unused"],"afterId":"removed"}]`,
			},
			wantArgs: []string{"nogui"},
		},
		{
			name: "keeps placeholder values with spaces in one token",
			config: ServerConfig{
				TemplateJSON: `[{"id":"name","order":1,"ownership":"editable","tokens":["--name={{SERVER_NAME}}"]}]`,
				Variables:    map[string]string{"SERVER_NAME": "My Great Server"},
			},
			wantArgs: []string{"--name=My Great Server"},
		},
		{
			name: "rejects malformed persisted patches",
			config: ServerConfig{
				TemplateJSON: `[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"]}]`,
				PatchesJSON:  `[{`,
			},
			wantErrText: "parse start arg patches",
		},
		{
			name: "checks final resolved token against blocklist",
			config: ServerConfig{
				TemplateJSON:  `[{"id":"debug","order":1,"ownership":"editable","tokens":["{{DEBUG_ARG}}"]}]`,
				BlocklistJSON: `[{"pattern":"^-agentlib:","reason":"debug agent"}]`,
				Variables:     map[string]string{"DEBUG_ARG": "-agentlib:jdwp"},
			},
			wantErrText: `blocked start argument "-agentlib:jdwp": debug agent`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs, errResolve := ResolveServer(tt.config)
			if tt.wantErrText != "" {
				if errResolve == nil || !strings.Contains(errResolve.Error(), tt.wantErrText) {
					t.Fatalf("ResolveServer() error = %v, want containing %q", errResolve, tt.wantErrText)
				}
				return
			}
			if errResolve != nil {
				t.Fatalf("ResolveServer() error = %v", errResolve)
			}
			if !slices.Equal(gotArgs, tt.wantArgs) {
				t.Errorf("ResolveServer() = %#v, want %#v", gotArgs, tt.wantArgs)
			}
		})
	}
}
