package eventbus

import (
	"sync"

	"github.com/rs/zerolog/log"
)

var (
	bus     *EventBus
	busOnce sync.Once
)

const (
	TopicGameServerCreated = "game_server_created"
	TopicGameServerRemoved = "game_server_removed"

	ReliableBufferSize = 1024
)

type EventBus struct {
	subscribers   map[string][]chan any
	reliableChans map[chan any]bool
	mu            sync.RWMutex
}

func Get() *EventBus {
	busOnce.Do(func() {
		bus = &EventBus{
			subscribers:   make(map[string][]chan any, 10),
			reliableChans: make(map[chan any]bool),
		}
	})
	return bus
}

func (e *EventBus) Subscribe(event string) chan any {
	e.mu.Lock()
	defer e.mu.Unlock()
	subscriber := make(chan any)
	e.subscribers[event] = append(e.subscribers[event], subscriber)
	return subscriber
}

// SubscribeReliable returns a buffered channel for topics where message loss
// is unacceptable (e.g., alert events). Uses a buffer of ReliableBufferSize.
// Publish will log a warning if the buffer is full instead of silently dropping.
func (e *EventBus) SubscribeReliable(event string) chan any {
	e.mu.Lock()
	defer e.mu.Unlock()
	subscriber := make(chan any, ReliableBufferSize)
	e.subscribers[event] = append(e.subscribers[event], subscriber)
	if e.reliableChans == nil {
		e.reliableChans = make(map[chan any]bool)
	}
	e.reliableChans[subscriber] = true
	return subscriber
}

func (e *EventBus) Publish(event string, data any) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, subscriber := range e.subscribers[event] {
		select {
		case subscriber <- data:
		default:
			if e.reliableChans[subscriber] {
				log.Warn().Str("topic", event).Int("bufferSize", ReliableBufferSize).Msg("Reliable subscriber buffer full, message dropped")
			}
		}
	}
}

func (e *EventBus) Unsubscribe(event string, subscriber chan any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, sub := range e.subscribers[event] {
		if sub == subscriber {
			e.subscribers[event] = append(e.subscribers[event][:i], e.subscribers[event][i+1:]...)
			delete(e.reliableChans, subscriber)
			close(subscriber)
			return
		}
	}
}
