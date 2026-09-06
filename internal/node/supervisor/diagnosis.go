package supervisor

import (
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// Keep process output separate from console history, which may retain lines
// from a previous execution while controller pre-start checks run.
func (c *Command) captureProcessOutput(processDone <-chan struct{}, output string) {
	c.Lock()
	defer c.Unlock()
	if c.processCtx == nil || c.processCtx.Done() != processDone || c.status != xylona.Status_ONLINE || c.InternalCommand || c.suppressStatusEvents {
		return
	}
	_, errWrite := c.failureOutput.WriteString(output)
	if errWrite != nil {
		log.Error().Err(errWrite).Str("process_id", c.ID).Msg("Unable to buffer failure evidence")
		return
	}
	if c.failureOutput.Len() > maxOutputBufferBytes {
		c.failureOutput.Next(c.failureOutput.Len() - maxOutputBufferBytes)
		c.failureOutputTruncated = true
	}
}

// Redact each byte stream before joining stdout/stderr and before the console
// record splitter inserts line breaks into oversized records.
type failureOutputWriter struct {
	command     *Command
	processDone <-chan struct{}
	secrets     []string
	pending     string
}

func (c *Command) newFailureOutputWriter(processDone <-chan struct{}) *failureOutputWriter {
	c.RLock()
	if c.status != xylona.Status_ONLINE || c.InternalCommand || c.suppressStatusEvents {
		c.RUnlock()
		return &failureOutputWriter{}
	}
	secrets := slices.Clone(c.redactValues)
	c.RUnlock()
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	return &failureOutputWriter{command: c, processDone: processDone, secrets: secrets}
}

func (w *failureOutputWriter) Write(data []byte) (int, error) {
	if w.command == nil {
		return len(data), nil
	}
	w.pending += string(data)
	w.flush(false)
	return len(data), nil
}

func (w *failureOutputWriter) flush(final bool) {
	if w.command == nil {
		return
	}
	redacted := make([]byte, 0, len(w.pending))
	consumed := 0
consume:
	for consumed < len(w.pending) {
		matched := false
		for _, secret := range w.secrets {
			if secret == "" {
				continue
			}
			remaining := w.pending[consumed:]
			if !final && len(remaining) < len(secret) && strings.HasPrefix(secret, remaining) {
				break consume
			}
			if strings.HasPrefix(remaining, secret) {
				redacted = append(redacted, "[redacted]"...)
				consumed += len(secret)
				matched = true
				break
			}
		}
		if !matched {
			redacted = append(redacted, w.pending[consumed])
			consumed++
		}
	}
	w.command.captureProcessOutput(w.processDone, string(redacted))
	w.pending = w.pending[consumed:]
	if final {
		clear(w.secrets)
		w.secrets = nil
	}
}

func (c *Command) captureFailure(generation uint64, stage string, errFailure error, exitCode *int) {
	c.Lock()
	defer c.Unlock()
	if c.processGeneration != generation || c.status != xylona.Status_ONLINE || c.InternalCommand || c.suppressStatusEvents {
		return
	}
	if stage == diagnosis.StageRuntime && c.intentionalStop.Load() {
		return
	}
	output := c.failureOutput.String()
	if c.failureOutputTruncated {
		// The raw tail may begin inside a credential. Discard enough prefix
		// to remove any partial value before redacting the complete tail.
		maxSecretBytes := 0
		for _, value := range c.redactValues {
			maxSecretBytes = max(maxSecretBytes, len(value))
		}
		output = output[min(len(output), maxSecretBytes):]
	}
	report := diagnosis.Capture(errFailure, output, c.redactValues...)
	report.ExecutionID = c.executionID
	report.AttemptStartedAt = c.attemptStartedAt
	report.OccurredAt = time.Now().UTC()
	report.Stage = stage
	report.ExitCode = exitCode
	report.EvidenceAvailable = true
	report.Truncated = report.Truncated || c.failureOutputTruncated
	c.failure = &report
}
