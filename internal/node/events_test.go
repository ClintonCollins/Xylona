package node

import (
	"testing"
	"time"
)

func TestEventEmitterPublishDeliversToSubscribers(t *testing.T) {
	emitter := NewEventEmitter()
	defer emitter.Close()

	sub := emitter.Subscribe()
	defer emitter.Unsubscribe(sub)

	want := Event{
		Type:      EventTypeProcessStatus,
		ProcessID: "srv-1",
		Timestamp: time.Now(),
	}

	go emitter.Publish(want)

	select {
	case got := <-sub:
		if got.Type != want.Type {
			t.Fatalf("Type = %q, want %q", got.Type, want.Type)
		}
		if got.ProcessID != want.ProcessID {
			t.Fatalf("ProcessID = %q, want %q", got.ProcessID, want.ProcessID)
		}
	case <-time.After(time.Second):
		t.Fatalf("subscriber did not receive event")
	}
}

func TestEventEmitterUnsubscribeStopsDelivery(t *testing.T) {
	t.Helper()

	emitter := NewEventEmitter()
	defer emitter.Close()

	sub := emitter.Subscribe()
	emitter.Unsubscribe(sub)

	// Publish should not block or panic with no live subscribers.
	emitter.Publish(Event{Type: EventTypeConsoleOutput, ProcessID: "srv-x"})
}
