package actions

import (
	"sync"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
)

// exitHookFunc runs when a game-server process transitions to OFFLINE. It is
// given the full event so hook implementations can inspect exit code and
// intentional-stop flags.
type exitHookFunc func(event eventbus.StatusChangedEvent)

// exitHookRegistry maps serverID → one-shot exit hook. Install and update
// flows register a hook after kicking off a background process; the
// status-change subscriber fires and removes it when the process exits.
//
// This replaces the old supervisor.PreparedCommand.CallbackFunction field,
// which only worked for the embedded supervisor. With this registry, both
// embedded and remote nodes drive post-exit work via the same bus subscriber.
type exitHookRegistry struct {
	mu    sync.Mutex
	hooks map[string]exitHookFunc
}

func newExitHookRegistry() *exitHookRegistry {
	return &exitHookRegistry{hooks: make(map[string]exitHookFunc)}
}

// set installs a one-shot hook for serverID. If a hook is already registered
// for that server it is silently replaced — the most recent caller wins.
func (r *exitHookRegistry) set(serverID string, fn exitHookFunc) {
	if serverID == "" || fn == nil {
		return
	}
	r.mu.Lock()
	r.hooks[serverID] = fn
	r.mu.Unlock()
}

// clear removes any registered hook for serverID. Used when an install
// aborts before the process ever started.
func (r *exitHookRegistry) clear(serverID string) {
	r.mu.Lock()
	delete(r.hooks, serverID)
	r.mu.Unlock()
}

// take atomically removes and returns the hook registered for serverID, or
// (nil, false) if there is none. Used by the status-change subscriber so a
// hook fires at most once.
func (r *exitHookRegistry) take(serverID string) (exitHookFunc, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn, ok := r.hooks[serverID]
	if ok {
		delete(r.hooks, serverID)
	}
	return fn, ok
}
