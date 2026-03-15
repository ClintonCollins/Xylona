package rpc

import (
	"database/sql"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestDispatchLocalOrRemoteLocalHandler(t *testing.T) {
	localCalled := false
	remoteCalled := false

	resp, errDispatch := dispatchLocalOrRemote(
		&models.GameServer{ID: "server-1"},
		nil,
		func(gameServer *models.GameServer) (*connect.Response[xylona.StartGameServerResponse], error) {
			localCalled = true
			if gameServer.ID != "server-1" {
				t.Errorf("local gameServer.ID = %q, want %q", gameServer.ID, "server-1")
			}
			return connect.NewResponse(&xylona.StartGameServerResponse{}), nil
		},
		func() (*connect.Response[xylona.StartGameServerResponse], error) {
			remoteCalled = true
			return nil, errors.New("unexpected remote call")
		},
	)

	if errDispatch != nil {
		t.Fatalf("dispatchLocalOrRemote() error = %v, want nil", errDispatch)
	}
	if resp == nil || resp.Msg == nil {
		t.Fatalf("dispatchLocalOrRemote() returned nil response")
	}
	if !localCalled {
		t.Errorf("local handler was not called")
	}
	if remoteCalled {
		t.Errorf("remote handler should not have been called")
	}
}

func TestDispatchLocalOrRemoteRemoteHandlerOnNotFound(t *testing.T) {
	localCalled := false
	remoteCalled := false

	resp, errDispatch := dispatchLocalOrRemote(
		nil,
		sql.ErrNoRows,
		func(gameServer *models.GameServer) (*connect.Response[xylona.StartGameServerResponse], error) {
			localCalled = true
			return nil, errors.New("unexpected local call")
		},
		func() (*connect.Response[xylona.StartGameServerResponse], error) {
			remoteCalled = true
			return connect.NewResponse(&xylona.StartGameServerResponse{}), nil
		},
	)

	if errDispatch != nil {
		t.Fatalf("dispatchLocalOrRemote() error = %v, want nil", errDispatch)
	}
	if resp == nil || resp.Msg == nil {
		t.Fatalf("dispatchLocalOrRemote() returned nil response")
	}
	if localCalled {
		t.Errorf("local handler should not have been called")
	}
	if !remoteCalled {
		t.Errorf("remote handler was not called")
	}
}

func TestDispatchLocalOrRemoteInternalErrorOnLookupFailure(t *testing.T) {
	resp, errDispatch := dispatchLocalOrRemote(
		nil,
		errors.New("boom"),
		func(gameServer *models.GameServer) (*connect.Response[xylona.StartGameServerResponse], error) {
			return connect.NewResponse(&xylona.StartGameServerResponse{}), nil
		},
		func() (*connect.Response[xylona.StartGameServerResponse], error) {
			return connect.NewResponse(&xylona.StartGameServerResponse{}), nil
		},
	)

	if resp != nil {
		t.Fatalf("dispatchLocalOrRemote() response = %v, want nil", resp)
	}
	if errDispatch == nil {
		t.Fatalf("dispatchLocalOrRemote() error = nil, want error")
	}
	if connect.CodeOf(errDispatch) != connect.CodeInternal {
		t.Errorf("dispatchLocalOrRemote() code = %v, want %v", connect.CodeOf(errDispatch), connect.CodeInternal)
	}
}

func TestDispatchLocalOrRemoteInternalErrorOnNilLocalServer(t *testing.T) {
	resp, errDispatch := dispatchLocalOrRemote(
		nil,
		nil,
		func(gameServer *models.GameServer) (*connect.Response[xylona.StartGameServerResponse], error) {
			return connect.NewResponse(&xylona.StartGameServerResponse{}), nil
		},
		func() (*connect.Response[xylona.StartGameServerResponse], error) {
			return connect.NewResponse(&xylona.StartGameServerResponse{}), nil
		},
	)

	if resp != nil {
		t.Fatalf("dispatchLocalOrRemote() response = %v, want nil", resp)
	}
	if errDispatch == nil {
		t.Fatalf("dispatchLocalOrRemote() error = nil, want error")
	}
	if connect.CodeOf(errDispatch) != connect.CodeInternal {
		t.Errorf("dispatchLocalOrRemote() code = %v, want %v", connect.CodeOf(errDispatch), connect.CodeInternal)
	}
}
