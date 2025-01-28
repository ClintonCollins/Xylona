package eventbus

import "sync"

var (
	bus *EventBus
)

const (
	TopicGameServerCreated = "game_server_created"
	TopicGameServerRemoved = "game_server_removed"
)
type EventBus struct {
	subscribers map[string][]chan interface{}
	*sync.RWMutex
}

func Get() *EventBus {
	if bus == nil {
		bus = &EventBus{
			subscribers: make(map[string][]chan interface{}, 10),
			RWMutex: &sync.RWMutex{},
		}
	}
	return bus
}

func (e *EventBus) Subscribe(event string) chan interface{} {
	e.Lock()
	defer e.Unlock()
	subscriber := make(chan interface{})
	e.subscribers[event] = append(e.subscribers[event], subscriber)
	return subscriber
}

func (e *EventBus) Publish(event string, data interface{}) {
	e.RLock()
	defer e.RUnlock()
	for _, subscriber := range e.subscribers[event] {
		select {
		case subscriber <- data:
		default:
			// Discard the message if the channel is full or not listening.
		}
	}
}

func (e *EventBus) Unsubscribe(event string, subscriber chan interface{}) {
	e.Lock()
	defer e.Unlock()
	for i, sub := range e.subscribers[event] {
		if sub == subscriber {
			e.subscribers[event] = append(e.subscribers[event][:i], e.subscribers[event][i+1:]...)
			close(subscriber)
			return
		}
	}
}
