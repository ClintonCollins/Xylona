package actions

import (
	"sync"
	"time"
)

const intentionalStopWindow = 30 * time.Second

// intentionalStopRegistry tracks short-lived user-requested stop intents so
// bridged remote OFFLINE events can be distinguished from crashes even when
// the remote event stream cannot carry IntentionalStop.
type intentionalStopRegistry struct {
	mu        sync.Mutex
	deadlines map[string]time.Time
}

func (r *intentionalStopRegistry) mark(serverID string) {
	if serverID == "" {
		return
	}

	r.mu.Lock()
	if r.deadlines == nil {
		r.deadlines = make(map[string]time.Time)
	}
	r.deadlines[serverID] = time.Now().Add(intentionalStopWindow)
	r.mu.Unlock()
}

func (r *intentionalStopRegistry) clear(serverID string) {
	if serverID == "" {
		return
	}

	r.mu.Lock()
	delete(r.deadlines, serverID)
	r.mu.Unlock()
}

func (r *intentionalStopRegistry) take(serverID string) bool {
	if serverID == "" {
		return false
	}

	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	deadline, ok := r.deadlines[serverID]
	if !ok {
		return false
	}
	delete(r.deadlines, serverID)
	return !deadline.Before(now)
}
