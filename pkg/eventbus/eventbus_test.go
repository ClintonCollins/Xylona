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

func TestPublish_NoSubscribers_NoPanic(t *testing.T) {
	eb := Get()
	// Should not panic or block
	eb.Publish("test.no_subs", "orphan")
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

func TestConcurrent_SubscribePublishUnsubscribe(t *testing.T) {
	eb := Get()
	topic := "test.concurrent"

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := eb.Subscribe(topic)
			eb.Publish(topic, "msg")
			eb.Unsubscribe(topic, ch)
		}()
	}
	wg.Wait()
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
	for i := 0; i < count; i++ {
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
