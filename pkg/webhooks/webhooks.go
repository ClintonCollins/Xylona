// Package webhooks formats and delivers alert notifications to webhook targets.
package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Severity represents the severity level of an alert event.
type Severity int

// Severity values classify alert urgency for outbound webhook payloads.
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

// ErrInvalidWebhookURL is returned when the webhook configuration URL is invalid.
var ErrInvalidWebhookURL = errors.New("webhooks: invalid webhook url")

// ErrSSRFBlocked is returned when a webhook URL resolves to a private or reserved IP address.
var ErrSSRFBlocked = errors.New("webhooks: target resolves to a private or reserved IP address")

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
	// ssrfCheckFn validates a URL against SSRF rules. Defaults to
	// ValidateWebhookTarget. Tests may set this to nil to skip the check
	// when using httptest servers on localhost.
	ssrfCheckFn func(rawURL string) error
}

// NewSender creates a Sender with default production settings.
func NewSender() *Sender {
	return &Sender{
		client:      &http.Client{Timeout: 10 * time.Second},
		rateLimiter: newRateLimiter(10),
		retry:       defaultRetryConfig,
		ssrfCheckFn: ValidateWebhookTarget,
	}
}

// Send delivers an alert event to the given webhook. channelType is the proto
// enum string (e.g., "NOTIFICATION_CHANNEL_TYPE_WEBHOOK_DISCORD").
func (s *Sender) Send(ctx context.Context, channelType string, config ChannelConfig, event AlertEvent) error {
	errValidate := ValidateChannelConfig(config)
	if errValidate != nil {
		return errValidate
	}
	config.URL = strings.TrimSpace(config.URL)
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
				Str("url", redactWebhookLogURL(config.URL)).
				Int("attempt", attempt+1).
				Int("max_attempts", s.retry.MaxAttempts).
				Msg("Webhook delivery attempt failed, retrying")
		} else {
			log.Warn().
				Err(errSend).
				Str("channel_type", channelType).
				Str("url", redactWebhookLogURL(config.URL)).
				Int("attempt", attempt+1).
				Int("max_attempts", s.retry.MaxAttempts).
				Msg("Webhook delivery failed after all attempts")
		}
	}

	return lastErr
}

// doPost performs a single HTTP POST request and returns the result.
// It re-validates the target against SSRF before dialing in case DNS changed
// since the initial validation.
func (s *Sender) doPost(ctx context.Context, targetURL string, body []byte) error {
	if s.ssrfCheckFn != nil {
		errSSRF := s.ssrfCheckFn(targetURL)
		if errSSRF != nil {
			return errSSRF
		}
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
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

func redactWebhookLogURL(rawURL string) string {
	parsedURL, errParse := url.Parse(rawURL)
	if errParse != nil {
		return "[invalid webhook url]"
	}
	if parsedURL.Host == "" {
		return "[invalid webhook url]"
	}
	if parsedURL.Scheme == "" {
		return parsedURL.Host
	}
	return fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
}

// privateRanges defines the CIDR ranges considered private or reserved.
var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // Link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique-local
	}
	for _, cidr := range cidrs {
		_, ipNet, errParse := net.ParseCIDR(cidr)
		if errParse != nil {
			panic("webhooks: bad CIDR: " + cidr)
		}
		privateRanges = append(privateRanges, ipNet)
	}
}

// isPrivateOrReservedIP reports whether the given IP is loopback, private,
// link-local, cloud metadata (169.254.169.254), unique-local, or unspecified.
func isPrivateOrReservedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateWebhookTarget parses the URL, resolves its hostname to IP addresses,
// and rejects any that are private, reserved, or cloud metadata endpoints.
func ValidateWebhookTarget(rawURL string) error {
	parsedURL, errParse := url.Parse(rawURL)
	if errParse != nil {
		return fmt.Errorf("%w: %w", ErrInvalidWebhookURL, errParse)
	}

	hostname := parsedURL.Hostname()
	if hostname == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidWebhookURL)
	}

	// Check if hostname is already an IP literal.
	ip := net.ParseIP(hostname)
	if ip != nil {
		if isPrivateOrReservedIP(ip) {
			return fmt.Errorf("%w: %s", ErrSSRFBlocked, hostname)
		}
		return nil
	}

	// Resolve hostname to IPs.
	addrs, errLookup := net.DefaultResolver.LookupHost(context.Background(), hostname)
	if errLookup != nil {
		return fmt.Errorf("%w: DNS resolution failed: %w", ErrInvalidWebhookURL, errLookup)
	}

	for _, addr := range addrs {
		resolved := net.ParseIP(addr)
		if resolved == nil {
			continue
		}
		if isPrivateOrReservedIP(resolved) {
			return fmt.Errorf("%w: %s resolves to %s", ErrSSRFBlocked, hostname, addr)
		}
	}

	return nil
}

// ValidateChannelConfig validates the configured webhook destination.
func ValidateChannelConfig(config ChannelConfig) error {
	rawURL := strings.TrimSpace(config.URL)
	if rawURL == "" {
		return fmt.Errorf("%w: url is required", ErrInvalidWebhookURL)
	}
	parsedURL, errParse := url.Parse(rawURL)
	if errParse != nil {
		return fmt.Errorf("%w: %w", ErrInvalidWebhookURL, errParse)
	}
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: unsupported scheme %q", ErrInvalidWebhookURL, parsedURL.Scheme)
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidWebhookURL)
	}
	return nil
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

func newRateLimiter(maxCount int) *rateLimiter {
	return &rateLimiter{
		windows:  make(map[string][]time.Time),
		maxCount: maxCount,
		window:   time.Minute,
	}
}

// Allow checks if a request to the given channel is allowed. Returns true and
// records the request if within limits.
func (rl *rateLimiter) Allow(channelKey string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	rl.pruneExpiredLocked(cutoff)

	timestamps := rl.windows[channelKey]
	if len(timestamps) >= rl.maxCount {
		return false
	}

	rl.windows[channelKey] = append(timestamps, now)
	return true
}

func (rl *rateLimiter) pruneExpiredLocked(cutoff time.Time) {
	for channelKey, timestamps := range rl.windows {
		pruned := timestamps[:0]
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				pruned = append(pruned, ts)
			}
		}
		if len(pruned) == 0 {
			delete(rl.windows, channelKey)
			continue
		}
		rl.windows[channelKey] = pruned
	}
}
