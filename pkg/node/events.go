package node

import (
	"sync"

	"github.com/rs/zerolog/log"
)

// eventBufferSize is the per-subscriber buffer size for the in-process event
// emitter. Mirrors pkg/eventbus' reliable buffer.
const eventBufferSize = 1024

// EventEmitter is a minimal in-process event publisher that the controller
// will subscribe to in Step 9. Until then it stays empty; the existing
// pkg/eventbus continues to carry events.
type EventEmitter struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	closed      bool
}

// NewEventEmitter creates a ready-to-use emitter. Callers should Close it when
// done to release subscriber channels.
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		subscribers: make(map[chan Event]struct{}),
	}
}

// Subscribe returns a buffered channel that receives subsequent published
// events. The returned channel is closed by Unsubscribe or Close.
func (e *EventEmitter) Subscribe() chan Event {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan Event, eventBufferSize)
	if e.closed {
		close(ch)
		return ch
	}
	e.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscriber and closes its channel. Calling Unsubscribe
// with a channel that is not registered is a no-op.
func (e *EventEmitter) Unsubscribe(ch chan Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, ok := e.subscribers[ch]
	if !ok {
		return
	}
	delete(e.subscribers, ch)
	close(ch)
}

// Publish delivers an event to all current subscribers. Slow subscribers whose
// buffers are full will drop the event; a warning is logged for visibility.
func (e *EventEmitter) Publish(event Event) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for ch := range e.subscribers {
		select {
		case ch <- event:
		default:
			log.Warn().Str("event_type", string(event.Type)).Str("process_id", event.ProcessID).
				Msg("node event subscriber buffer full, event dropped")
		}
	}
}

// Close shuts down the emitter and closes all subscriber channels. Subsequent
// Subscribe calls return a closed channel.
func (e *EventEmitter) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return
	}
	e.closed = true
	for ch := range e.subscribers {
		close(ch)
	}
	e.subscribers = nil
}
