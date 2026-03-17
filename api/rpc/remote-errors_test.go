package rpc

import (
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
)

func TestWrapRemoteRPCError(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		unavailableMsg    string
		wantNil           bool
		wantCode          connect.Code
		wantMessageSubstr string
	}{
		{
			name:           "nil error returns nil",
			err:            nil,
			unavailableMsg: "fallback unavailable",
			wantNil:        true,
		},
		{
			name:              "connect error preserves original code",
			err:               connect.NewError(connect.CodePermissionDenied, errors.New("permission denied")),
			unavailableMsg:    "fallback unavailable",
			wantCode:          connect.CodePermissionDenied,
			wantMessageSubstr: "permission denied",
		},
		{
			name:              "non-connect error maps to unavailable",
			err:               errors.New("dial tcp timeout"),
			unavailableMsg:    "failed to reach remote node",
			wantCode:          connect.CodeUnavailable,
			wantMessageSubstr: "failed to reach remote node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := wrapRemoteRPCError(tt.err, tt.unavailableMsg)
			if tt.wantNil {
				if gotErr != nil {
					t.Fatalf("wrapRemoteRPCError() = %v, want nil", gotErr)
				}
				return
			}

			if gotErr == nil {
				t.Fatalf("wrapRemoteRPCError() = nil, want error")
			}

			gotCode := connect.CodeOf(gotErr)
			if gotCode != tt.wantCode {
				t.Fatalf("wrapRemoteRPCError() code = %v, want %v", gotCode, tt.wantCode)
			}

			if !strings.Contains(gotErr.Error(), tt.wantMessageSubstr) {
				t.Fatalf("wrapRemoteRPCError() error = %q, want substring %q", gotErr.Error(), tt.wantMessageSubstr)
			}
		})
	}
}
