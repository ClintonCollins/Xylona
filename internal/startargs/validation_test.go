package startargs

import (
	"strings"
	"testing"
)

func TestValidateDefinition(t *testing.T) {
	validLinux := `[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"],"label":"Heap","managed_source":"game_server.max_memory_mb"}]`
	validWindows := `[{"id":"heap","order":1,"ownership":"editable","tokens":["-Xmx2G"],"label":"Heap","managed_source":"game_server.max_memory_mb"}]`
	tests := []struct {
		name        string
		config      DefinitionConfig
		wantErrText string
	}{
		{name: "empty definition is valid"},
		{
			name: "complete definition is valid",
			config: DefinitionConfig{
				LinuxTemplateJSON:   validLinux,
				LinuxBaseCommand:    "java",
				WindowsTemplateJSON: validWindows,
				WindowsBaseCommand:  "java.exe",
				BlocklistJSON:       `[{"pattern":"^-agentlib:","reason":"debug agent"}]`,
			},
		},
		{
			name: "template block id is required",
			config: DefinitionConfig{
				LinuxTemplateJSON: `[{"order":1,"ownership":"editable","tokens":["-Xmx2G"]}]`,
				LinuxBaseCommand:  "java",
			},
			wantErrText: "template block id is required",
		},
		{
			name: "duplicate template ids are rejected",
			config: DefinitionConfig{
				LinuxTemplateJSON: `[{"id":"heap","ownership":"editable","tokens":["a"]},{"id":"heap","ownership":"editable","tokens":["b"]}]`,
				LinuxBaseCommand:  "java",
			},
			wantErrText: `duplicate template block id "heap"`,
		},
		{
			name: "template tokens are required",
			config: DefinitionConfig{
				LinuxTemplateJSON: `[{"id":"heap","ownership":"editable","tokens":[]}]`,
				LinuxBaseCommand:  "java",
			},
			wantErrText: "must contain at least one token",
		},
		{
			name: "ownership must be supported",
			config: DefinitionConfig{
				LinuxTemplateJSON: `[{"id":"heap","ownership":"other","tokens":["a"]}]`,
				LinuxBaseCommand:  "java",
			},
			wantErrText: "invalid ownership",
		},
		{
			name: "managed source must be supported",
			config: DefinitionConfig{
				LinuxTemplateJSON: `[{"id":"heap","ownership":"editable","tokens":["a"],"managed_source":"unknown"}]`,
				LinuxBaseCommand:  "java",
			},
			wantErrText: "invalid managed source",
		},
		{
			name:        "template requires base command",
			config:      DefinitionConfig{LinuxTemplateJSON: validLinux},
			wantErrText: "base command is required",
		},
		{
			name: "shared id metadata must agree",
			config: DefinitionConfig{
				LinuxTemplateJSON:   validLinux,
				LinuxBaseCommand:    "java",
				WindowsTemplateJSON: `[{"id":"heap","ownership":"locked","tokens":["-Xmx2G"],"label":"Heap","managed_source":"game_server.max_memory_mb"}]`,
				WindowsBaseCommand:  "java.exe",
			},
			wantErrText: "same ownership on both platforms",
		},
		{
			name:        "blocklist pattern must compile",
			config:      DefinitionConfig{BlocklistJSON: `[{"pattern":"[","reason":"broken"}]`},
			wantErrText: "compile start arg blocklist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errValidate := ValidateDefinition(tt.config)
			assertErrorContains(t, errValidate, tt.wantErrText)
		})
	}
}

func TestValidateServerUpdate(t *testing.T) {
	template := `[{"id":"system","order":1,"ownership":"system","tokens":["-jar","server.jar"]},{"id":"locked","order":2,"ownership":"locked","tokens":["-safe"]},{"id":"heap","order":3,"ownership":"editable","tokens":["-Xmx2G"]}]`
	tests := []struct {
		name        string
		config      ServerConfig
		wantErrText string
	}{
		{
			name:   "empty template and patches skip blocklist validation",
			config: ServerConfig{BlocklistJSON: `[{"pattern":"["}]`},
		},
		{
			name:        "patches require a template",
			config:      ServerConfig{PatchesJSON: `[{"id":"heap","op":"remove"}]`},
			wantErrText: "does not have a start args template",
		},
		{
			name: "missing edit and remove targets are accepted",
			config: ServerConfig{
				TemplateJSON: template,
				PatchesJSON:  `[{"id":"old-edit","op":"edit","tokens":["old"]},{"id":"old-remove","op":"remove"}]`,
			},
		},
		{
			name: "add can reference an earlier add",
			config: ServerConfig{
				TemplateJSON: template,
				PatchesJSON:  `[{"id":"first","op":"add","tokens":["a"],"afterId":"heap"},{"id":"second","op":"add","tokens":["b"],"afterId":"first"}]`,
			},
		},
		{
			name: "add cannot reference a later add",
			config: ServerConfig{
				TemplateJSON: template,
				PatchesJSON:  `[{"id":"first","op":"add","tokens":["a"],"afterId":"second"},{"id":"second","op":"add","tokens":["b"],"afterId":"heap"}]`,
			},
			wantErrText: `references unknown afterId "second"`,
		},
		{
			name: "add id cannot collide with template",
			config: ServerConfig{
				TemplateJSON: template,
				PatchesJSON:  `[{"id":"heap","op":"add","tokens":["a"]}]`,
			},
			wantErrText: "collides with an existing template block",
		},
		{
			name: "duplicate add ids are rejected",
			config: ServerConfig{
				TemplateJSON: template,
				PatchesJSON:  `[{"id":"extra","op":"add","tokens":["a"]},{"id":"extra","op":"add","tokens":["b"]}]`,
			},
			wantErrText: "duplicate patch id",
		},
		{
			name: "duplicate locked patch ids are rejected",
			config: ServerConfig{
				TemplateJSON:        template,
				ExistingPatchesJSON: `[{"id":"locked","op":"edit","tokens":["-custom"]}]`,
				PatchesJSON:         `[{"id":"locked","op":"remove"},{"id":"locked","op":"edit","tokens":["-custom"]}]`,
			},
			wantErrText: `duplicate patch id "locked"`,
		},
		{
			name: "add anchor cannot be empty",
			config: ServerConfig{
				TemplateJSON: template,
				PatchesJSON:  `[{"id":"extra","op":"add","tokens":["a"],"afterId":" "}]`,
			},
			wantErrText: "has an empty afterId",
		},
		{
			name: "patch cannot target non-editable block",
			config: ServerConfig{
				TemplateJSON: template,
				PatchesJSON:  `[{"id":"system","op":"remove"}]`,
			},
			wantErrText: "targets a non-editable template block",
		},
		{
			name: "superuser can edit locked block",
			config: ServerConfig{
				TemplateJSON:     template,
				PatchesJSON:      `[{"id":"locked","op":"edit","tokens":["-custom"]}]`,
				AllowLockedEdits: true,
			},
		},
		{
			name: "superuser can remove locked block",
			config: ServerConfig{
				TemplateJSON:     template,
				PatchesJSON:      `[{"id":"locked","op":"remove"}]`,
				AllowLockedEdits: true,
			},
		},
		{
			name: "superuser locked edit is checked against blocklist",
			config: ServerConfig{
				TemplateJSON:     template,
				PatchesJSON:      `[{"id":"locked","op":"edit","tokens":["--blocked"]}]`,
				BlocklistJSON:    `[{"pattern":"^--blocked$","reason":"blocked for everyone"}]`,
				AllowLockedEdits: true,
			},
			wantErrText: `blocked start argument "--blocked": blocked for everyone`,
		},
		{
			name: "system block remains immutable for superuser",
			config: ServerConfig{
				TemplateJSON:     template,
				PatchesJSON:      `[{"id":"system","op":"edit","tokens":["other.jar"]}]`,
				AllowLockedEdits: true,
			},
			wantErrText: "targets a non-editable template block",
		},
		{
			name: "non-superuser can retain locked patch while editing ordinary block",
			config: ServerConfig{
				TemplateJSON:        template,
				ExistingPatchesJSON: `[{"id":"locked","op":"edit","tokens":["-custom"]}]`,
				PatchesJSON:         `[{"id":"locked","op":"edit","tokens":["-custom"]},{"id":"heap","op":"edit","tokens":["-Xmx4G"]}]`,
			},
		},
		{
			name: "non-superuser cannot change locked patch",
			config: ServerConfig{
				TemplateJSON:        template,
				ExistingPatchesJSON: `[{"id":"locked","op":"edit","tokens":["-custom"]}]`,
				PatchesJSON:         `[{"id":"locked","op":"edit","tokens":["-tampered"]}]`,
			},
			wantErrText: "locked start arguments may only be changed by a superuser",
		},
		{
			name: "non-superuser cannot delete locked patch",
			config: ServerConfig{
				TemplateJSON:        template,
				ExistingPatchesJSON: `[{"id":"locked","op":"remove"}]`,
				PatchesJSON:         `[]`,
			},
			wantErrText: "locked start arguments may only be changed by a superuser",
		},
		{
			name: "add requires tokens",
			config: ServerConfig{
				TemplateJSON: template,
				PatchesJSON:  `[{"id":"extra","op":"add"}]`,
			},
			wantErrText: "must contain at least one token",
		},
		{
			name: "patch operation must be supported",
			config: ServerConfig{
				TemplateJSON: template,
				PatchesJSON:  `[{"id":"heap","op":"replace","tokens":["a"]}]`,
			},
			wantErrText: "invalid operation",
		},
		{
			name: "resolved update is checked against blocklist",
			config: ServerConfig{
				TemplateJSON:  template,
				PatchesJSON:   `[{"id":"heap","op":"edit","tokens":["{{HEAP}}"]}]`,
				BlocklistJSON: `[{"pattern":"^-Xmx32G$","reason":"memory limit"}]`,
				Variables:     map[string]string{"HEAP": "-Xmx32G"},
			},
			wantErrText: `blocked start argument "-Xmx32G": memory limit`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errValidate := ValidateServerUpdate(tt.config)
			assertErrorContains(t, errValidate, tt.wantErrText)
		})
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if want == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want containing %q", err, want)
	}
}
