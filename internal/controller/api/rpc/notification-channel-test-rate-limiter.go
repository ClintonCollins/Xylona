package rpc

import (
	"sync"
	"time"
)

type notificationChannelTestRateLimiter struct {
	mu       sync.Mutex
	windows  map[string][]time.Time
	maxCount int
	window   time.Duration
}

func newNotificationChannelTestRateLimiter(maxCount int, window time.Duration) *notificationChannelTestRateLimiter {
	return &notificationChannelTestRateLimiter{
		windows:  make(map[string][]time.Time),
		maxCount: maxCount,
		window:   window,
	}
}

func (rl *notificationChannelTestRateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	for windowKey, timestamps := range rl.windows {
		pruned := timestamps[:0]
		for _, timestamp := range timestamps {
			if timestamp.After(cutoff) {
				pruned = append(pruned, timestamp)
			}
		}
		if len(pruned) == 0 {
			delete(rl.windows, windowKey)
			continue
		}
		rl.windows[windowKey] = pruned
	}

	timestamps := rl.windows[key]
	if len(timestamps) >= rl.maxCount {
		return false
	}

	rl.windows[key] = append(timestamps, now)
	return true
}
