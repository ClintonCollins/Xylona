package actions

import (
	"errors"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestValidateWritableServerPathRejectsProtectedPath(t *testing.T) {
	gameServer := &models.GameServer{
		ID:               "server-protected-paths",
		ServerExecutable: null.From("server.jar"),
	}

	got, errValidate := validateWritableServerPath(gameServer, "server.jar")
	if !errors.Is(errValidate, ErrProtectedPath) {
		t.Fatalf("validateWritableServerPath() error = %v, want %v", errValidate, ErrProtectedPath)
	}
	if got != "" {
		t.Fatalf("validateWritableServerPath() = %q, want empty path on error", got)
	}
}
