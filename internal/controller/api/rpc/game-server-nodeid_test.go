package rpc

import "testing"

func TestFallbackNodeID(t *testing.T) {
	tests := []struct {
		name          string
		requestNodeID string
		defaultNodeID string
		wantNodeID    string
	}{
		{
			name:          "uses provided node id",
			requestNodeID: "node-123",
			defaultNodeID: "default-node",
			wantNodeID:    "node-123",
		},
		{
			name:          "falls back for empty node id",
			requestNodeID: "",
			defaultNodeID: "default-node",
			wantNodeID:    "default-node",
		},
		{
			name:          "falls back for whitespace node id",
			requestNodeID: "   ",
			defaultNodeID: "default-node",
			wantNodeID:    "default-node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNodeID := fallbackNodeID(tt.requestNodeID, tt.defaultNodeID)
			if gotNodeID != tt.wantNodeID {
				t.Errorf("fallbackNodeID(%q, %q) = %q, want %q", tt.requestNodeID, tt.defaultNodeID, gotNodeID, tt.wantNodeID)
			}
		})
	}
}
