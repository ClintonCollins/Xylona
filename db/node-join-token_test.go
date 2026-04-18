package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConsumeNodeJoinTokenRejectsZeroRowConsume(t *testing.T) {
	t.Parallel()

	conn := newRBACMigratedConnection(t, "node-join-token-consume-zero.sqlite")

	token, _, errGenerate := conn.GenerateNodeJoinToken("Remote Node", time.Hour)
	if errGenerate != nil {
		t.Fatalf("GenerateNodeJoinToken() error = %v", errGenerate)
	}

	_, errTrigger := conn.SQLDb.ExecContext(context.Background(), `
create trigger ignore_join_token_consume
before update of consumed_at on node_join_token
for each row
begin
	select raise(ignore);
end;
`)
	if errTrigger != nil {
		t.Fatalf("create trigger error = %v", errTrigger)
	}

	consumed, errConsume := conn.ConsumeNodeJoinToken(token, "node-remote-1")
	if !errors.Is(errConsume, ErrJoinTokenInvalid) {
		t.Fatalf("ConsumeNodeJoinToken() error = %v, want %v", errConsume, ErrJoinTokenInvalid)
	}
	if consumed != nil {
		t.Fatalf("ConsumeNodeJoinToken() token = %+v, want nil", consumed)
	}
}
