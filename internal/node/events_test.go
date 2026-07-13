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

func TestEventEmitterReplayPrecedesLiveEvents(t *testing.T) {
	emitter := NewEventEmitter()
	defer emitter.Close()

	emitter.Publish(Event{
		Type:               EventTypeProcessStatus,
		ProcessID:          "srv-1",
		Status:             "ONLINE",
		ExecutionID:        "execution-1",
		TransitionSequence: 1,
	})

	sub := emitter.SubscribeWithReplay(true)
	defer emitter.Unsubscribe(sub)
	emitter.Publish(Event{
		Type:               EventTypeProcessStatus,
		ProcessID:          "srv-1",
		Status:             "OFFLINE",
		ExecutionID:        "execution-1",
		TransitionSequence: 2,
	})

	replayed := <-sub
	if !replayed.Replayed || replayed.TransitionSequence != 1 {
		t.Fatalf("replayed event = %+v, want replayed sequence 1", replayed)
	}
	live := <-sub
	if live.Replayed || live.TransitionSequence != 2 {
		t.Fatalf("live event = %+v, want live sequence 2", live)
	}
}

func TestEventEmitterClosesSlowLifecycleSubscriber(t *testing.T) {
	emitter := NewEventEmitter()
	defer emitter.Close()

	sub := emitter.Subscribe()
	for range eventBufferSize {
		emitter.Publish(Event{Type: EventTypeMetrics})
	}
	emitter.Publish(Event{Type: EventTypeProcessStatus, ProcessID: "srv-1"})

	for range eventBufferSize {
		_, open := <-sub
		if !open {
			t.Fatal("subscriber closed before buffered events were drained")
		}
	}
	_, open := <-sub
	if open {
		t.Fatal("slow lifecycle subscriber remained open")
	}
}
