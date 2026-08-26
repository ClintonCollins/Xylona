package rpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/internal/firstsetup"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGetSetupStatus(t *testing.T) {
	t.Parallel()

	t.Run("needed when no superuser exists", func(t *testing.T) {
		t.Parallel()
		xs := newSetupService(t)

		resp, errStatus := xs.GetSetupStatus(context.Background(), connect.NewRequest(&xylona.GetSetupStatusRequest{}))
		if errStatus != nil {
			t.Fatalf("GetSetupStatus() error = %v", errStatus)
		}
		if !resp.Msg.GetNeeded() {
			t.Fatal("GetSetupStatus() needed = false, want true")
		}
	})

	t.Run("not needed when a superuser already exists", func(t *testing.T) {
		t.Parallel()
		fixture := newRBACRPCFixture(t)
		resp, errStatus := fixture.service.GetSetupStatus(
			context.Background(),
			connect.NewRequest(&xylona.GetSetupStatusRequest{}),
		)
		if errStatus != nil {
			t.Fatalf("GetSetupStatus() error = %v", errStatus)
		}
		if resp.Msg.GetNeeded() {
			t.Fatal("GetSetupStatus() needed = true, want false")
		}
	})
}

func TestCompleteSetup(t *testing.T) {
	t.Parallel()

	t.Run("valid token creates the first superuser and is consumed", func(t *testing.T) {
		t.Parallel()
		xs := newSetupService(t)
		plaintext := setSetupToken(t, xs)

		resp, errComplete := xs.CompleteSetup(context.Background(), connect.NewRequest(&xylona.CompleteSetupRequest{
			UserName: "admin",
			Email:    "admin@localhost",
			Password: "secret-password",
			Token:    plaintext,
		}))
		if errComplete != nil {
			t.Fatalf("CompleteSetup() error = %v", errComplete)
		}
		if resp.Msg.GetUser().GetUserName() != "admin" {
			t.Fatalf("CompleteSetup() username = %q, want admin", resp.Msg.GetUser().GetUserName())
		}
		if !resp.Msg.GetUser().GetSuperUser() {
			t.Fatal("CompleteSetup() superuser = false, want true")
		}
		if len(resp.Header().Values("Set-Cookie")) < 2 {
			t.Fatal("CompleteSetup() did not set session cookies")
		}

		_, errSecond := xs.CompleteSetup(context.Background(), connect.NewRequest(&xylona.CompleteSetupRequest{
			UserName: "other",
			Email:    "other@localhost",
			Password: "secret-password",
			Token:    plaintext,
		}))
		if connect.CodeOf(errSecond) != connect.CodePermissionDenied {
			t.Fatalf("second CompleteSetup() code = %v, want %v", connect.CodeOf(errSecond), connect.CodePermissionDenied)
		}
		if !errors.Is(errSecond, firstsetup.ErrSetupTokenInvalid) {
			t.Fatalf("second CompleteSetup() error = %v, want %v", errSecond, firstsetup.ErrSetupTokenInvalid)
		}
	})

	t.Run("token is required", func(t *testing.T) {
		t.Parallel()
		xs := newSetupService(t)
		plaintext := setSetupToken(t, xs)

		_, errDenied := xs.CompleteSetup(context.Background(), connect.NewRequest(&xylona.CompleteSetupRequest{
			UserName: "admin",
			Email:    "admin@localhost",
			Password: "secret-password",
		}))
		if connect.CodeOf(errDenied) != connect.CodePermissionDenied {
			t.Fatalf("CompleteSetup() without token code = %v, want %v", connect.CodeOf(errDenied), connect.CodePermissionDenied)
		}

		resp, errComplete := xs.CompleteSetup(context.Background(), connect.NewRequest(&xylona.CompleteSetupRequest{
			UserName: "admin",
			Email:    "admin@localhost",
			Password: "secret-password",
			Token:    plaintext,
		}))
		if errComplete != nil {
			t.Fatalf("CompleteSetup() with token error = %v", errComplete)
		}
		if resp.Msg.GetUser().GetUserName() != "admin" {
			t.Fatalf("CompleteSetup() username = %q, want admin", resp.Msg.GetUser().GetUserName())
		}
	})
}

func newSetupService(t *testing.T) *XylonaService {
	t.Helper()

	conn := newRPCFixtureConnection(t, "setup-rpc.sqlite")
	secureCookieInst := securecookie.New(
		[]byte("0123456789abcdef0123456789abcdef"),
		[]byte("0123456789abcdef"),
	)
	return &XylonaService{
		ctx:          context.Background(),
		db:           conn,
		secureCookie: secureCookieInst,
		userService:  usermgmt.NewService(conn),
	}
}

func setSetupToken(t *testing.T, xs *XylonaService) string {
	t.Helper()
	plaintext, token, errIssue := firstsetup.IssueToken()
	if errIssue != nil {
		t.Fatalf("IssueToken() error = %v", errIssue)
	}
	xs.SetSetupToken(token)
	return plaintext
}

func TestCompleteSetupRequiresUsername(t *testing.T) {
	t.Parallel()
	xs := newSetupService(t)
	plaintext := setSetupToken(t, xs)

	_, errComplete := xs.CompleteSetup(context.Background(), connect.NewRequest(&xylona.CompleteSetupRequest{
		Password: "secret-password",
		Token:    plaintext,
	}))
	if connect.CodeOf(errComplete) != connect.CodeInvalidArgument {
		t.Fatalf("CompleteSetup() code = %v, want %v", connect.CodeOf(errComplete), connect.CodeInvalidArgument)
	}
	if !strings.Contains(errComplete.Error(), "user_name") {
		t.Fatalf("CompleteSetup() error = %q, want username required", errComplete.Error())
	}

	_, errRetry := xs.CompleteSetup(context.Background(), connect.NewRequest(&xylona.CompleteSetupRequest{
		UserName: "admin",
		Password: "secret-password",
		Token:    plaintext,
	}))
	if errRetry != nil {
		t.Fatalf("CompleteSetup() retry error = %v, want reusable token", errRetry)
	}
}
