package startargs

import (
	"reflect"
	"testing"
)

func TestResolveArgs(t *testing.T) {
	anchorID := "anchor"
	removedID := "removed"
	addedID := "added"
	addedSecondID := "added-second"

	tests := []struct {
		name               string
		template           []ArgBlock
		patches            []Patch
		vars               map[string]string
		wantArgs           []string
		wantProvenance     []string
		wantOriginalTokens map[string][]string
	}{
		{
			name:     "empty template returns empty args",
			template: nil,
			patches:  nil,
			vars:     nil,
			wantArgs:       []string{},
			wantProvenance: []string{},
		},
		{
			name: "single block returns its tokens",
			template: []ArgBlock{
				{ID: "one", Order: 1, Ownership: OwnershipEditable, Tokens: []string{"-Xmx2G"}},
			},
			wantArgs:       []string{"-Xmx2G"},
			wantProvenance: []string{"default"},
		},
		{
			name: "multiple blocks flatten in order",
			template: []ArgBlock{
				{ID: "one", Order: 1, Ownership: OwnershipLocked, Tokens: []string{"-jar", "{{SERVER_EXECUTABLE}}"}},
				{ID: "two", Order: 2, Ownership: OwnershipEditable, Tokens: []string{"nogui"}},
			},
			vars:           map[string]string{"SERVER_EXECUTABLE": "paper.jar"},
			wantArgs:       []string{"-jar", "paper.jar", "nogui"},
			wantProvenance: []string{"locked", "default"},
		},
		{
			name: "edit patch replaces tokens and tracks original tokens",
			template: []ArgBlock{
				{ID: "heap", Order: 1, Ownership: OwnershipEditable, Tokens: []string{"-Xmx2G"}},
			},
			patches: []Patch{
				{ID: "heap", Op: PatchOpEdit, Tokens: []string{"-Xmx8G"}},
			},
			wantArgs:           []string{"-Xmx8G"},
			wantProvenance:     []string{"edited"},
			wantOriginalTokens: map[string][]string{"heap": {"-Xmx2G"}},
		},
		{
			name: "remove patch excludes matching block",
			template: []ArgBlock{
				{ID: "jar", Order: 1, Ownership: OwnershipSystem, Tokens: []string{"-jar", "paper.jar"}},
				{ID: "nogui", Order: 2, Ownership: OwnershipEditable, Tokens: []string{"nogui"}},
			},
			patches: []Patch{
				{ID: "nogui", Op: PatchOpRemove},
			},
			wantArgs:       []string{"-jar", "paper.jar"},
			wantProvenance: []string{"system"},
		},
		{
			name: "add patch inserts after specified anchor",
			template: []ArgBlock{
				{ID: anchorID, Order: 1, Ownership: OwnershipEditable, Tokens: []string{"-Xmx2G"}},
				{ID: "tail", Order: 2, Ownership: OwnershipEditable, Tokens: []string{"nogui"}},
			},
			patches: []Patch{
				{ID: addedID, Op: PatchOpAdd, Tokens: []string{"-XX:+UseG1GC"}, AfterID: &anchorID},
			},
			wantArgs:       []string{"-Xmx2G", "-XX:+UseG1GC", "nogui"},
			wantProvenance: []string{"default", "added", "default"},
		},
		{
			name: "add patch with nil anchor inserts at beginning",
			template: []ArgBlock{
				{ID: "jar", Order: 1, Ownership: OwnershipSystem, Tokens: []string{"-jar", "paper.jar"}},
			},
			patches: []Patch{
				{ID: addedID, Op: PatchOpAdd, Tokens: []string{"-Xmx2G"}},
			},
			wantArgs:       []string{"-Xmx2G", "-jar", "paper.jar"},
			wantProvenance: []string{"added", "system"},
		},
		{
			name: "multiple adds at same anchor keep patch array order",
			template: []ArgBlock{
				{ID: anchorID, Order: 1, Ownership: OwnershipEditable, Tokens: []string{"-Xmx2G"}},
			},
			patches: []Patch{
				{ID: addedID, Op: PatchOpAdd, Tokens: []string{"-XX:+UseG1GC"}, AfterID: &anchorID},
				{ID: addedSecondID, Op: PatchOpAdd, Tokens: []string{"-XX:+UseZGC"}, AfterID: &anchorID},
			},
			wantArgs:       []string{"-Xmx2G", "-XX:+UseG1GC", "-XX:+UseZGC"},
			wantProvenance: []string{"default", "added", "added"},
		},
		{
			name: "orphaned edit and remove patches are skipped",
			template: []ArgBlock{
				{ID: "one", Order: 1, Ownership: OwnershipEditable, Tokens: []string{"-Xmx2G"}},
			},
			patches: []Patch{
				{ID: "missing", Op: PatchOpEdit, Tokens: []string{"-Xmx8G"}},
				{ID: "missing-remove", Op: PatchOpRemove},
			},
			wantArgs:       []string{"-Xmx2G"},
			wantProvenance: []string{"default"},
		},
		{
			name: "orphaned add patch anchored to removed arg is skipped",
			template: []ArgBlock{
				{ID: removedID, Order: 1, Ownership: OwnershipEditable, Tokens: []string{"-Xmx2G"}},
				{ID: "tail", Order: 2, Ownership: OwnershipEditable, Tokens: []string{"nogui"}},
			},
			patches: []Patch{
				{ID: removedID, Op: PatchOpRemove},
				{ID: addedID, Op: PatchOpAdd, Tokens: []string{"-XX:+UseG1GC"}, AfterID: &removedID},
			},
			wantArgs:       []string{"nogui"},
			wantProvenance: []string{"default"},
		},
		{
			name: "placeholder resolution happens per token",
			template: []ArgBlock{
				{ID: "log", Order: 1, Ownership: OwnershipEditable, Tokens: []string{"--log=server-{{PORT}}.txt"}},
			},
			vars:           map[string]string{"PORT": "25565"},
			wantArgs:       []string{"--log=server-25565.txt"},
			wantProvenance: []string{"default"},
		},
		{
			name: "placeholder value with spaces stays a single token",
			template: []ArgBlock{
				{ID: "name", Order: 1, Ownership: OwnershipEditable, Tokens: []string{"--name={{SERVER_NAME}}"}},
			},
			vars:           map[string]string{"SERVER_NAME": "My Great Server"},
			wantArgs:       []string{"--name=My Great Server"},
			wantProvenance: []string{"default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs, gotBlocks, errResolve := ResolveArgs(tt.template, tt.patches, tt.vars)
			if errResolve != nil {
				t.Fatalf("ResolveArgs() error = %v", errResolve)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("ResolveArgs() args = %#v, want %#v", gotArgs, tt.wantArgs)
			}

			gotProvenance := make([]string, 0, len(gotBlocks))
			for _, block := range gotBlocks {
				gotProvenance = append(gotProvenance, block.Provenance)
			}

			if !reflect.DeepEqual(gotProvenance, tt.wantProvenance) {
				t.Errorf("ResolveArgs() provenance = %#v, want %#v", gotProvenance, tt.wantProvenance)
			}

			for blockID, wantOriginal := range tt.wantOriginalTokens {
				found := false
				for _, block := range gotBlocks {
					if block.ID != blockID {
						continue
					}
					found = true
					if !reflect.DeepEqual(block.OriginalTokens, wantOriginal) {
						t.Errorf("ResolveArgs() original tokens for %q = %#v, want %#v", blockID, block.OriginalTokens, wantOriginal)
					}
				}
				if !found {
					t.Errorf("ResolveArgs() missing block %q", blockID)
				}
			}
		})
	}
}
