package node

import (
	"sync"

	"github.com/rs/zerolog/log"
)

// eventBufferSize is the per-subscriber buffer size for the in-process event
// emitter. Mirrors internal/eventbus' reliable buffer.
const eventBufferSize = 1024

// EventEmitter is the replayable in-process event publisher used by node event
// streams. Process lifecycle events are retained by process ID.
type EventEmitter struct {
	mu                  sync.Mutex
	subscribers         map[chan Event]struct{}
	latestProcessStatus map[string]Event
	closed              bool
}

// NewEventEmitter creates a ready-to-use emitter. Callers should Close it when
// done to release subscriber channels.
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		subscribers:         make(map[chan Event]struct{}),
		latestProcessStatus: make(map[string]Event),
	}
}

// Subscribe returns a buffered channel that receives subsequent published
// events. The returned channel is closed by Unsubscribe or Close.
func (e *EventEmitter) Subscribe() chan Event {
	return e.SubscribeWithReplay(false)
}

// SubscribeWithReplay atomically registers a subscriber and, when requested,
// queues the retained latest process-status event for every process before any
// subsequently published live event.
func (e *EventEmitter) SubscribeWithReplay(replayProcessStatus bool) chan Event {
	e.mu.Lock()
	defer e.mu.Unlock()

	bufferSize := eventBufferSize
	if replayProcessStatus {
		bufferSize += len(e.latestProcessStatus)
	}
	ch := make(chan Event, bufferSize)
	if e.closed {
		close(ch)
		return ch
	}
	if replayProcessStatus {
		for _, retained := range e.latestProcessStatus {
			retained.Replayed = true
			ch <- retained
		}
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

// Publish delivers an event to all current subscribers. Process lifecycle
// events are retained for future replay. A slow subscriber is disconnected
// instead of silently losing a lifecycle transition; it can reconnect and
// receive the retained state. Non-lifecycle events remain best-effort.
func (e *EventEmitter) Publish(event Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return
	}
	if event.Type == EventTypeProcessStatus && event.ProcessID != "" {
		event.Replayed = false
		e.latestProcessStatus[event.ProcessID] = event
	}

	for ch := range e.subscribers {
		select {
		case ch <- event:
		default:
			if event.Type == EventTypeProcessStatus {
				delete(e.subscribers, ch)
				close(ch)
				log.Warn().Str("event_type", string(event.Type)).Str("process_id", event.ProcessID).
					Msg("node event subscriber buffer full, lifecycle stream closed for replay")
				continue
			}
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
