package eventbus

import (
	"sync"
	"testing"
	"time"
)

func TestSubscribe_ReceivesPublishedMessage(t *testing.T) {
	eb := Get()
	topic := "test.subscribe_receives"
	ch := eb.Subscribe(topic)
	defer eb.Unsubscribe(topic, ch)

	// The test goroutine blocks on the channel receive. We launch a goroutine
	// to call Publish after a brief yield so the test goroutine is already
	// waiting in the select before Publish fires.
	go func() {
		// Yield to allow the main goroutine to enter the select below.
		time.Sleep(5 * time.Millisecond)
		eb.Publish(topic, "hello")
	}()

	select {
	case msg := <-ch:
		val, ok := msg.(string)
		if !ok {
			t.Fatalf("expected string, got %T", msg)
		}
		if val != "hello" {
			t.Fatalf("expected 'hello', got %q", val)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestMultipleSubscribers_SameTopic(t *testing.T) {
	eb := Get()
	topic := "test.multi_sub"
	ch1 := eb.Subscribe(topic)
	ch2 := eb.Subscribe(topic)
	defer eb.Unsubscribe(topic, ch1)
	defer eb.Unsubscribe(topic, ch2)

	// Both goroutines block on their channel. Publish fires after a brief yield
	// to ensure both goroutines are already waiting in their selects.
	var wg sync.WaitGroup
	results := make([]any, 2)
	received := make([]bool, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		select {
		case results[0] = <-ch1:
			received[0] = true
		case <-time.After(time.Second):
		}
	}()
	go func() {
		defer wg.Done()
		select {
		case results[1] = <-ch2:
			received[1] = true
		case <-time.After(time.Second):
		}
	}()

	// Yield to allow both receiver goroutines to enter their selects.
	time.Sleep(5 * time.Millisecond)
	eb.Publish(topic, "broadcast")

	wg.Wait()
	for i, ok := range received {
		if !ok {
			t.Fatalf("subscriber %d timed out waiting for message", i+1)
		}
		if results[i] != "broadcast" {
			t.Fatalf("subscriber %d: expected 'broadcast', got %v", i+1, results[i])
		}
	}
}

func TestUnsubscribe_StopsDelivery(t *testing.T) {
	eb := Get()
	topic := "test.unsub"
	ch := eb.Subscribe(topic)

	eb.Unsubscribe(topic, ch)

	// Channel should be closed after unsubscribe
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}
}

func TestPublish_NonBlocking_DropsOnFullChannel(t *testing.T) {
	eb := Get()
	topic := "test.drop"
	ch := eb.Subscribe(topic)
	defer eb.Unsubscribe(topic, ch)

	// Unbuffered channel — publish without a receiver should drop silently
	eb.Publish(topic, "should_drop")

	select {
	case <-ch:
		t.Fatal("expected message to be dropped on unbuffered channel with no active receiver")
	case <-time.After(50 * time.Millisecond):
		// Expected — message was dropped
	}
}

func TestSubscribeAfterPublish_MissesMessage(t *testing.T) {
	eb := Get()
	topic := "test.late_sub"
	eb.Publish(topic, "early")

	ch := eb.Subscribe(topic)
	defer eb.Unsubscribe(topic, ch)

	select {
	case <-ch:
		t.Fatal("should not receive message published before subscribe")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestSubscribeReliable_BufferedChannel(t *testing.T) {
	eb := Get()
	topic := "test.reliable"
	ch := eb.SubscribeReliable(topic)
	defer eb.Unsubscribe(topic, ch)

	// Should be able to publish without a receiver (buffered)
	eb.Publish(topic, "buffered_msg")

	select {
	case msg := <-ch:
		if msg != "buffered_msg" {
			t.Fatalf("expected 'buffered_msg', got %v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out — message should be in buffer")
	}
}

func TestSubscribeReliable_HighThroughput(t *testing.T) {
	eb := Get()
	topic := "test.reliable_throughput"
	ch := eb.SubscribeReliable(topic)
	defer eb.Unsubscribe(topic, ch)

	count := 100
	for i := range count {
		eb.Publish(topic, i)
	}

	received := 0
	for received < count {
		select {
		case <-ch:
			received++
		case <-time.After(time.Second):
			t.Fatalf("only received %d of %d messages", received, count)
		}
	}
}

func TestTypedEvent_ServerCrashed(t *testing.T) {
	eb := Get()
	ch := eb.SubscribeReliable(TopicGameServerCrashed)
	defer eb.Unsubscribe(TopicGameServerCrashed, ch)

	event := ServerCrashedEvent{
		ServerID:     "srv-1",
		ServerNodeID: "node-a",
		ExitCode:     1,
		Timestamp:    time.Now(),
	}
	eb.Publish(TopicGameServerCrashed, event)

	select {
	case msg := <-ch:
		got, ok := msg.(ServerCrashedEvent)
		if !ok {
			t.Fatalf("expected ServerCrashedEvent, got %T", msg)
		}
		if got.ServerID != "srv-1" {
			t.Fatalf("expected server ID 'srv-1', got %q", got.ServerID)
		}
		if got.ServerNodeID != "node-a" {
			t.Fatalf("expected server node ID 'node-a', got %q", got.ServerNodeID)
		}
		if got.ExitCode != 1 {
			t.Fatalf("expected exit code 1, got %d", got.ExitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestTypedEvent_ThresholdCrossed(t *testing.T) {
	eb := Get()
	ch := eb.SubscribeReliable(TopicGameServerCPUThreshold)
	defer eb.Unsubscribe(TopicGameServerCPUThreshold, ch)

	event := ThresholdEvent{
		ServerID:     "srv-2",
		ServerNodeID: "node-b",
		CurrentValue: 95.5,
		Threshold:    90.0,
		Direction:    ThresholdEntered,
	}
	eb.Publish(TopicGameServerCPUThreshold, event)

	select {
	case msg := <-ch:
		got, ok := msg.(ThresholdEvent)
		if !ok {
			t.Fatalf("expected ThresholdEvent, got %T", msg)
		}
		if got.Direction != ThresholdEntered {
			t.Fatalf("expected direction Entered, got %v", got.Direction)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestTypedEvent_StatusChanged(t *testing.T) {
	eb := Get()
	ch := eb.SubscribeReliable(TopicGameServerStatusChanged)
	defer eb.Unsubscribe(TopicGameServerStatusChanged, ch)

	event := StatusChangedEvent{
		ServerID:     "srv-3",
		ServerNodeID: "node-a",
		OldStatus:    "online",
		NewStatus:    "offline",
	}
	eb.Publish(TopicGameServerStatusChanged, event)

	select {
	case msg := <-ch:
		got, ok := msg.(StatusChangedEvent)
		if !ok {
			t.Fatalf("expected StatusChangedEvent, got %T", msg)
		}
		if got.NewStatus != "offline" {
			t.Fatalf("expected new status 'offline', got %q", got.NewStatus)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestTypedEvent_NodeThreshold(t *testing.T) {
	eb := Get()
	ch := eb.SubscribeReliable(TopicNodeCPUThreshold)
	defer eb.Unsubscribe(TopicNodeCPUThreshold, ch)

	event := NodeThresholdEvent{
		NodeID:       "node-c",
		CurrentValue: 88.0,
		Threshold:    85.0,
		Direction:    ThresholdEntered,
	}
	eb.Publish(TopicNodeCPUThreshold, event)

	select {
	case msg := <-ch:
		got, ok := msg.(NodeThresholdEvent)
		if !ok {
			t.Fatalf("expected NodeThresholdEvent, got %T", msg)
		}
		if got.NodeID != "node-c" {
			t.Fatalf("expected node ID 'node-c', got %q", got.NodeID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
