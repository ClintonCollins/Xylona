package actions

import (
	"runtime/debug"
	"sync/atomic"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var backgroundTaskLogger atomic.Pointer[zerolog.Logger]

// runBackgroundTask executes a background job step and recovers from panics so
// the enclosing long-running loop can continue on the next tick.
func runBackgroundTask(jobName string, phase string, fields map[string]string, fn func()) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		backgroundLogger := log.Logger
		configuredLogger := backgroundTaskLogger.Load()
		if configuredLogger != nil {
			backgroundLogger = *configuredLogger
		}

		event := backgroundLogger.Error().
			Str("background_job", jobName).
			Str("phase", phase).
			Interface("panic_value", recovered).
			Str("panic_stack", string(debug.Stack()))

		for key, value := range fields {
			event = event.Str(key, value)
		}

		event.Msg("Recovered from panic in background job")
	}()

	fn()
}
