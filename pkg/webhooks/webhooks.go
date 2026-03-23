package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Severity represents the severity level of an alert event.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityCritical
)

// String returns the human-readable name of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// AlertEvent represents a normalized alert event ready for delivery.
type AlertEvent struct {
	EventType    string   // e.g., "ALERT_EVENT_TYPE_CRASH"
	ServerName   string   // human-readable name, empty for node events
	ServerID     string   // empty for node events
	ServerNodeID string   // empty for node events
	NodeID       string   // empty for server events
	Message      string   // human-readable description
	Severity     Severity // Info, Warning, Critical
	Timestamp    time.Time
	Fields       map[string]string // additional key-value metadata
}

// ChannelConfig holds the parsed webhook URL and any channel-specific settings.
type ChannelConfig struct {
	URL string `json:"url"`
}

// Channel type constants matching the proto enum string values.
const (
	ChannelTypeDiscord = "NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD"
	ChannelTypeSlack   = "NOTIFICATION_CHANNEL_TYPE_WEBHOOK_SLACK"
	ChannelTypeGeneric = "NOTIFICATION_CHANNEL_TYPE_WEBHOOK_GENERIC"
)

// ErrRateLimited is returned when the per-channel rate limit is exceeded.
var ErrRateLimited = errors.New("webhooks: rate limit exceeded")

// ErrUnsupportedChannel is returned when the channel type is not recognized.
var ErrUnsupportedChannel = errors.New("webhooks: unsupported channel type")

// DeliveryError wraps delivery failures with status code and body details.
type DeliveryError struct {
	StatusCode int
	Body       string
	Err        error
}

func (e *DeliveryError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("webhooks: delivery failed (status %d): %s: %v", e.StatusCode, e.Body, e.Err)
	}
	return fmt.Sprintf("webhooks: delivery failed (status %d): %s", e.StatusCode, e.Body)
}

func (e *DeliveryError) Unwrap() error {
	return e.Err
}

// retryConfig controls retry behavior. Exported fields allow test overrides.
type retryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

// defaultRetryConfig is the production retry configuration.
var defaultRetryConfig = retryConfig{
	MaxAttempts: 3,
	BaseDelay:   time.Second,
}

// Sender delivers webhook payloads with retry and rate limiting.
type Sender struct {
	client      *http.Client
	rateLimiter *rateLimiter
	retry       retryConfig
}

// NewSender creates a Sender with default production settings.
func NewSender() *Sender {
	return &Sender{
		client:      &http.Client{Timeout: 10 * time.Second},
		rateLimiter: newRateLimiter(10, time.Minute),
		retry:       defaultRetryConfig,
	}
}

// Send delivers an alert event to the given webhook. channelType is the proto
// enum string (e.g., "NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD").
func (s *Sender) Send(ctx context.Context, channelType string, config ChannelConfig, event AlertEvent) error {
	if !s.rateLimiter.Allow(config.URL) {
		return ErrRateLimited
	}

	payload, errFormat := formatPayload(channelType, event)
	if errFormat != nil {
		return errFormat
	}

	body, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return fmt.Errorf("webhooks: failed to marshal payload: %w", errMarshal)
	}

	var lastErr error
	for attempt := range s.retry.MaxAttempts {
		if attempt > 0 {
			delay := s.retry.BaseDelay * (1 << (attempt - 1))
			select {
			case <-ctx.Done():
				return fmt.Errorf("webhooks: context cancelled during retry: %w", ctx.Err())
			case <-time.After(delay):
			}
		}

		errSend := s.doPost(ctx, config.URL, body)
		if errSend == nil {
			return nil
		}

		lastErr = errSend

		// Only retry on 5xx errors or context-independent transport errors.
		var deliveryErr *DeliveryError
		if errors.As(errSend, &deliveryErr) && deliveryErr.StatusCode < 500 {
			// 4xx errors are not retryable.
			return deliveryErr
		}

		if attempt+1 < s.retry.MaxAttempts {
			log.Warn().
				Err(errSend).
				Str("channel_type", channelType).
				Str("url", config.URL).
				Int("attempt", attempt+1).
				Int("max_attempts", s.retry.MaxAttempts).
				Msg("Webhook delivery attempt failed, retrying")
		} else {
			log.Warn().
				Err(errSend).
				Str("channel_type", channelType).
				Str("url", config.URL).
				Int("attempt", attempt+1).
				Int("max_attempts", s.retry.MaxAttempts).
				Msg("Webhook delivery failed after all attempts")
		}
	}

	return lastErr
}

// doPost performs a single HTTP POST request and returns the result.
func (s *Sender) doPost(ctx context.Context, url string, body []byte) error {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if errReq != nil {
		return fmt.Errorf("webhooks: failed to create request: %w", errReq)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, errDo := s.client.Do(req)
	if errDo != nil {
		return &DeliveryError{
			StatusCode: 0,
			Body:       "",
			Err:        errDo,
		}
	}
	defer func() {
		errClose := resp.Body.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close webhook response body")
		}
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	respBody, errRead := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if errRead != nil {
		respBody = []byte("(failed to read response body)")
	}

	return &DeliveryError{
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
	}
}

// formatPayload selects the formatter based on channel type.
func formatPayload(channelType string, event AlertEvent) (any, error) {
	switch channelType {
	case ChannelTypeDiscord:
		return FormatDiscord(event), nil
	case ChannelTypeSlack:
		return FormatSlack(event), nil
	case ChannelTypeGeneric:
		return FormatGeneric(event), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedChannel, channelType)
	}
}

// eventTypeTitle maps proto event type strings to human-readable titles.
var eventTypeTitle = map[string]string{
	"ALERT_EVENT_TYPE_CRASH":                  "Server Crashed",
	"ALERT_EVENT_TYPE_STATUS_CHANGE":          "Server Status Changed",
	"ALERT_EVENT_TYPE_CPU_THRESHOLD":          "CPU Threshold Exceeded",
	"ALERT_EVENT_TYPE_MEMORY_THRESHOLD":       "Memory Threshold Exceeded",
	"ALERT_EVENT_TYPE_DISK_THRESHOLD":         "Disk Threshold Exceeded",
	"ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD": "Player Count Threshold",
	"ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD":     "Node CPU Threshold Exceeded",
	"ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD":  "Node Memory Threshold Exceeded",
	"ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD":    "Node Disk Threshold Exceeded",
}

// EventTypeTitle returns a human-readable title for the given event type string.
// Falls back to the raw event type if no mapping exists.
func EventTypeTitle(eventType string) string {
	title, ok := eventTypeTitle[eventType]
	if ok {
		return title
	}
	return eventType
}

// rateLimiter implements a per-channel sliding window rate limiter.
type rateLimiter struct {
	mu       sync.Mutex
	windows  map[string][]time.Time
	maxCount int
	window   time.Duration
}

func newRateLimiter(maxCount int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		windows:  make(map[string][]time.Time),
		maxCount: maxCount,
		window:   window,
	}
}

// Allow checks if a request to the given channel is allowed. Returns true and
// records the request if within limits.
func (rl *rateLimiter) Allow(channelKey string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Prune expired entries.
	timestamps := rl.windows[channelKey]
	pruned := timestamps[:0]
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}

	if len(pruned) >= rl.maxCount {
		rl.windows[channelKey] = pruned
		return false
	}

	rl.windows[channelKey] = append(pruned, now)
	return true
}
