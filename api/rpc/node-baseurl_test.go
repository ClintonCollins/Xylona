package rpc

import (
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
		expectError bool
	}{
		{
			name:        "trims whitespace and trailing slash",
			input:       " https://example.com/ ",
			want:        "https://example.com",
			expectError: false,
		},
		{
			name:        "keeps path segments",
			input:       "https://example.com/xylona/",
			want:        "https://example.com/xylona",
			expectError: false,
		},
		{
			name:        "rejects empty input",
			input:       "   ",
			expectError: true,
		},
		{
			name:        "rejects missing scheme",
			input:       "example.com:8080",
			expectError: true,
		},
		{
			name:        "rejects unsupported scheme",
			input:       "ftp://example.com",
			expectError: true,
		},
		{
			name:        "rejects missing host",
			input:       "https:///xylona",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errNormalize := normalizeBaseURL(tt.input)
			if (errNormalize != nil) != tt.expectError {
				t.Fatalf("normalizeBaseURL() error = %v, expectError %v", errNormalize, tt.expectError)
			}

			if got != tt.want {
				t.Errorf("normalizeBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddNodeRejectsLocalNodeCreation(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	request := connect.NewRequest(&xylona.AddNodeRequest{
		Node: &xylona.Node{
			Name: "should-fail",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errAddNode := fixture.service.AddNode(t.Context(), request)
	if errAddNode == nil {
		t.Fatalf("AddNode() error = nil, want error")
	}
	if connect.CodeOf(errAddNode) != connect.CodeInvalidArgument {
		t.Fatalf("AddNode() code = %v, want %v", connect.CodeOf(errAddNode), connect.CodeInvalidArgument)
	}
}

func TestAddNodeAllowsPairingFlowWithoutSession(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	request := connect.NewRequest(&xylona.AddNodeRequest{
		Node: &xylona.Node{
			Name:      "pairing-node",
			BaseUrl:   "https://remote.example.com:9000",
			SecretKey: "pairing-token",
		},
	})

	_, errAddNode := fixture.service.AddNode(t.Context(), request)
	if errAddNode == nil {
		t.Fatalf("AddNode() error = nil, want error")
	}
	if connect.CodeOf(errAddNode) != connect.CodeInternal {
		t.Fatalf("AddNode() code = %v, want %v", connect.CodeOf(errAddNode), connect.CodeInternal)
	}
}
