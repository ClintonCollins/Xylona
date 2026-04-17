package rpc

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
)

func TestNewXylonaServiceReturnsErrorWhenPermissionsCannotLoad(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xylona-service.sqlite")
	conn, errNewConnection := db.NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("db.NewConnection() error = %v", errNewConnection)
	}
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close test db: %v", errClose)
		}
	})

	service, errNewService := NewXylonaService(
		context.Background(),
		conn,
		nil,
		nil,
		nil,
		nil,
		false,
		nil,
		nil,
		versiontracker.NewVersionStateMap(),
	)
	if errNewService == nil {
		t.Fatal("NewXylonaService() error = nil, want error")
	}
	if service != nil {
		t.Fatalf("NewXylonaService() service = %+v, want nil", service)
	}
}
