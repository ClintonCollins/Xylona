package adminipc

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/firstsetup"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestUserHandlerCompleteSetup(t *testing.T) {
	t.Parallel()

	conn := dbtest.NewMigratedConnection(t, "adminipc-complete-setup.sqlite")
	handler := NewUserHandler(usermgmt.NewService(conn))
	request := connect.NewRequest(&xylona.CompleteSetupRequest{
		UserName: "admin",
		Email:    "admin@localhost",
		Password: "secret-password",
	})

	response, errComplete := handler.CompleteSetup(context.Background(), request)
	if errComplete != nil {
		t.Fatalf("CompleteSetup() error = %v", errComplete)
	}
	if !response.Msg.GetUser().GetSuperUser() {
		t.Fatal("CompleteSetup() superuser = false, want true")
	}

	_, errSecond := handler.CompleteSetup(context.Background(), request)
	if connect.CodeOf(errSecond) != connect.CodeFailedPrecondition {
		t.Fatalf("second CompleteSetup() code = %v, want %v", connect.CodeOf(errSecond), connect.CodeFailedPrecondition)
	}
	if !errors.Is(errSecond, firstsetup.ErrAlreadyInstalled) {
		t.Fatalf("second CompleteSetup() error = %v, want %v", errSecond, firstsetup.ErrAlreadyInstalled)
	}
}
