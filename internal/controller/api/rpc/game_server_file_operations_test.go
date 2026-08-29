package rpc

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/node"
)

func TestFileMutationErrorDescribesProtectedLaunchFiles(t *testing.T) {
	errResult := fileMutationError(node.ErrProtectedPath)
	if connect.CodeOf(errResult) != connect.CodePermissionDenied {
		t.Fatalf("fileMutationError() code = %v, want %v", connect.CodeOf(errResult), connect.CodePermissionDenied)
	}

	want := "permission_denied: the operation targets the configured server executable or launch command; only superusers may modify protected server files"
	if errResult.Error() != want {
		t.Fatalf("fileMutationError() error = %q, want %q", errResult.Error(), want)
	}
}

func TestFileMutationErrorDescribesOversizedDownload(t *testing.T) {
	const detail = "node: download exceeds maximum allowed size: content length 100000000001 bytes exceeds limit 100000000000 bytes"
	tests := []struct {
		name  string
		input error
	}{
		{
			name:  "embedded node error",
			input: fmt.Errorf("nodeclient: download file from URL: %w: content length 100000000001 bytes exceeds limit 100000000000 bytes", node.ErrDownloadTooLarge),
		},
		{
			name: "remote node error",
			input: fmt.Errorf(
				"nodeclient: download file from URL: %w",
				connect.NewError(connect.CodeResourceExhausted, errors.New(detail)),
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errResult := fileMutationError(tc.input)
			if connect.CodeOf(errResult) != connect.CodeResourceExhausted {
				t.Fatalf("fileMutationError() code = %v, want %v", connect.CodeOf(errResult), connect.CodeResourceExhausted)
			}
			want := "resource_exhausted: " + detail
			if errResult.Error() != want {
				t.Fatalf("fileMutationError() error = %q, want %q", errResult.Error(), want)
			}
		})
	}
}

func TestFileMutationErrorSanitizesUnexpectedErrors(t *testing.T) {
	errInput := errors.New(`open /srv/xylona/servers/abc/protected.jar: permission denied`)

	errResult := fileMutationError(errInput)
	if connect.CodeOf(errResult) != connect.CodeInternal {
		t.Fatalf("fileMutationError() code = %v, want %v", connect.CodeOf(errResult), connect.CodeInternal)
	}
	if errResult.Error() == errInput.Error() {
		t.Fatalf("fileMutationError() leaked raw error %q", errResult.Error())
	}
	if errResult.Error() != "internal: file operation failed" {
		t.Fatalf("fileMutationError() error = %q, want %q", errResult.Error(), "internal: file operation failed")
	}
}
