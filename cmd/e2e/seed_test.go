package main

import (
	"context"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/securecookie"

	rpcpkg "github.com/ClintonCollins/Xylona/internal/controller/api/rpc"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestRunSeedCreatesAdminThatCanLogin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seed-login.sqlite")
	migrationsDir, errMigrationsDir := filepath.Abs(filepath.Join("..", "..", "sql", "migrations"))
	if errMigrationsDir != nil {
		t.Fatalf("filepath.Abs() error = %v", errMigrationsDir)
	}

	errSeed := runSeed(dbPath, "seed-admin", "seed-password-123", migrationsDir)
	if errSeed != nil {
		t.Fatalf("runSeed() error = %v", errSeed)
	}

	conn, errNewConnection := db.NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("NewConnection() error = %v", errNewConnection)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("close test db: %v", errClose)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	service, errService := rpcpkg.NewXylonaService(
		ctx,
		conn,
		nil,
		nil,
		securecookie.New(
			[]byte("0123456789abcdef0123456789abcdef"),
			[]byte("0123456789abcdef"),
		),
		false,
		nil,
		nil,
		nil,
	)
	if errService != nil {
		t.Fatalf("NewXylonaService() error = %v", errService)
	}

	response, errLogin := service.Login(ctx, connect.NewRequest(&xylona.LoginRequest{
		UserName: "seed-admin",
		Password: "seed-password-123",
	}))
	if errLogin != nil {
		t.Fatalf("Login() error = %v", errLogin)
	}
	if response.Msg == nil || response.Msg.GetUser() == nil {
		t.Fatal("Login() returned empty response")
	}
	if response.Msg.GetUser().GetUserName() != "seed-admin" {
		t.Fatalf("Login().User.UserName = %q, want %q", response.Msg.GetUser().GetUserName(), "seed-admin")
	}
}
