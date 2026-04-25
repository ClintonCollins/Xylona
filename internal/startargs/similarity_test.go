package startargs

import "testing"

func TestFindSimilarArg(t *testing.T) {
	tests := []struct {
		name          string
		newTokens     []string
		existing      []ArgBlock
		wantMatchedID string
	}{
		{
			name:      "xmx matches by prefix",
			newTokens: []string{"-Xmx8G"},
			existing: []ArgBlock{
				{ID: "heap", Tokens: []string{"-Xmx2G"}},
			},
			wantMatchedID: "heap",
		},
		{
			name:      "port flag value pair matches by prefix",
			newTokens: []string{"--port", "25570"},
			existing: []ArgBlock{
				{ID: "port", Tokens: []string{"--port", "25565"}},
			},
			wantMatchedID: "port",
		},
		{
			name:      "different prefixes do not match",
			newTokens: []string{"-XX:+UseG1GC"},
			existing: []ArgBlock{
				{ID: "heap", Tokens: []string{"-Xmx2G"}},
			},
		},
		{
			name:      "non flag exact match only",
			newTokens: []string{"nogui"},
			existing: []ArgBlock{
				{ID: "nographic", Tokens: []string{"nographic"}},
				{ID: "nogui", Tokens: []string{"nogui"}},
			},
			wantMatchedID: "nogui",
		},
		{
			name:      "empty existing args returns no match",
			newTokens: []string{"-Xmx8G"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindSimilarArg(tt.newTokens, tt.existing)
			if tt.wantMatchedID == "" {
				if got != nil {
					t.Errorf("FindSimilarArg() = %#v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("FindSimilarArg() = nil, want ID %q", tt.wantMatchedID)
			}
			if got.ID != tt.wantMatchedID {
				t.Errorf("FindSimilarArg().ID = %q, want %q", got.ID, tt.wantMatchedID)
			}
		})
	}
}
