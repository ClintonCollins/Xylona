package noderegistry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/nodeclient"
)

// closableFake wraps FakeNodeClient with an io.Closer so Remove/Close paths
// can be exercised.
type closableFake struct {
	*nodeclient.FakeNodeClient
	closed   atomic.Int32
	closeErr error
}

func (c *closableFake) Close() error {
	c.closed.Add(1)
	return c.closeErr
}

func newFake(id string) *nodeclient.FakeNodeClient {
	return &nodeclient.FakeNodeClient{NodeID: id}
}

func newClosable(id string) *closableFake {
	return &closableFake{FakeNodeClient: newFake(id)}
}

func TestNewRegistersEmbeddedClientUnderSelf(t *testing.T) {
	self := newFake("self")
	registry := New("self", self)

	client, errGet := registry.Get("self")
	if errGet != nil {
		t.Fatalf("Get(self) err = %v", errGet)
	}
	if client != self {
		t.Fatalf("Get(self) returned unexpected client")
	}
	if registry.SelfID() != "self" {
		t.Fatalf("SelfID = %q, want %q", registry.SelfID(), "self")
	}
}

func TestNewWithNilEmbeddedDoesNotRegisterSelf(t *testing.T) {
	registry := New("self", nil)
	if client := registry.GetSelf(); client != nil {
		t.Fatalf("GetSelf() = %v, want nil", client)
	}
	if len(registry.List()) != 0 {
		t.Fatalf("List() len = %d, want 0", len(registry.List()))
	}
}

func TestGetSelfReturnsRegisteredEmbedded(t *testing.T) {
	self := newFake("self")
	registry := New("self", self)

	if got := registry.GetSelf(); got != self {
		t.Fatalf("GetSelf() returned unexpected client")
	}
}

func TestRegisterAddsClient(t *testing.T) {
	registry := New("self", nil)
	remote := newFake("remote")
	registry.Register(remote)

	got, errGet := registry.Get("remote")
	if errGet != nil {
		t.Fatalf("Get(remote) err = %v", errGet)
	}
	if got != remote {
		t.Fatalf("Get(remote) returned unexpected client")
	}
}

func TestRegisterNilIsNoop(t *testing.T) {
	registry := New("self", nil)
	registry.Register(nil)
	if len(registry.List()) != 0 {
		t.Fatalf("List() len = %d, want 0", len(registry.List()))
	}
}

func TestRegisterReplacesExisting(t *testing.T) {
	registry := New("self", nil)
	first := newFake("dup")
	second := newFake("dup")
	registry.Register(first)
	registry.Register(second)

	got, errGet := registry.Get("dup")
	if errGet != nil {
		t.Fatalf("Get(dup) err = %v", errGet)
	}
	if got != second {
		t.Fatalf("Get(dup) returned first, want second")
	}
}

func TestGetMissingReturnsErrNodeNotRegistered(t *testing.T) {
	registry := New("self", nil)
	_, errGet := registry.Get("unknown")
	if !errors.Is(errGet, ErrNodeNotRegistered) {
		t.Fatalf("Get err = %v, want ErrNodeNotRegistered", errGet)
	}
}

func TestListReturnsAllRegisteredClients(t *testing.T) {
	self := newFake("self")
	registry := New("self", self)
	registry.Register(newFake("remote-1"))
	registry.Register(newFake("remote-2"))

	list := registry.List()
	if len(list) != 3 {
		t.Fatalf("List() len = %d, want 3", len(list))
	}

	seen := make(map[string]bool, len(list))
	for _, client := range list {
		seen[client.ID()] = true
	}
	for _, expected := range []string{"self", "remote-1", "remote-2"} {
		if !seen[expected] {
			t.Fatalf("expected %q in list; got %v", expected, seen)
		}
	}
}

func TestRemoveDeletesAndClosesClosableClients(t *testing.T) {
	registry := New("self", nil)
	remote := newClosable("remote")
	registry.Register(remote)

	registry.Remove("remote")

	_, errGet := registry.Get("remote")
	if !errors.Is(errGet, ErrNodeNotRegistered) {
		t.Fatalf("Get after Remove err = %v, want ErrNodeNotRegistered", errGet)
	}
	if remote.closed.Load() != 1 {
		t.Fatalf("closed count = %d, want 1", remote.closed.Load())
	}
}

func TestRemoveNonCloserClient(t *testing.T) {
	registry := New("self", nil)
	registry.Register(newFake("remote"))
	registry.Remove("remote")

	_, errGet := registry.Get("remote")
	if !errors.Is(errGet, ErrNodeNotRegistered) {
		t.Fatalf("Get after Remove err = %v, want ErrNodeNotRegistered", errGet)
	}
}

func TestCloseClosesAllClosableClients(t *testing.T) {
	registry := New("self", nil)
	first := newClosable("a")
	second := newClosable("b")
	registry.Register(first)
	registry.Register(second)
	registry.Register(newFake("plain"))

	errClose := registry.Close(context.Background())
	if errClose != nil {
		t.Fatalf("Close() err = %v", errClose)
	}
	if first.closed.Load() != 1 {
		t.Fatalf("first.closed = %d, want 1", first.closed.Load())
	}
	if second.closed.Load() != 1 {
		t.Fatalf("second.closed = %d, want 1", second.closed.Load())
	}
	if len(registry.List()) != 0 {
		t.Fatalf("List() len = %d, want 0 after Close", len(registry.List()))
	}
}

func TestCloseJoinsPerClientErrors(t *testing.T) {
	registry := New("self", nil)
	failing := newClosable("bad")
	failing.closeErr = errors.New("boom")
	registry.Register(failing)

	errClose := registry.Close(context.Background())
	if errClose == nil {
		t.Fatalf("Close() err = nil, want joined error")
	}
	if !errors.Is(errClose, failing.closeErr) {
		t.Fatalf("Close() err = %v, want wrapping %v", errClose, failing.closeErr)
	}
}
