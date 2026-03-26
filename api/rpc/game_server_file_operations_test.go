package rpc

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
)

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
