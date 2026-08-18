package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// testEvent returns a fully populated AlertEvent for use in tests.
func testEvent() AlertEvent {
	return AlertEvent{
		EventType:    "ALERT_EVENT_TYPE_CRASH",
		ServerName:   "Minecraft Survival",
		ServerID:     "srv-123",
		ServerNodeID: "node-456",
		NodeID:       "",
		Message:      "Server process exited with code 1",
		Severity:     SeverityCritical,
		Timestamp:    time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC),
		Fields: map[string]string{
			"Exit Code": "1",
			"Uptime":    "3h 42m",
		},
	}
}

// newTestSender creates a Sender with short timeouts suitable for testing.
// SSRF checking is disabled because httptest servers bind to localhost.
func newTestSender() *Sender {
	return &Sender{
		client:      &http.Client{Timeout: 2 * time.Second},
		rateLimiter: newRateLimiter(10),
		retry: retryConfig{
			MaxAttempts: 3,
			BaseDelay:   time.Millisecond, // no real waiting in tests
		},
		ssrfCheckFn: nil, // skip SSRF for localhost test servers
	}
}

// --- Formatting tests ---

func TestFormatDiscord_CrashEvent(t *testing.T) {
	t.Parallel()

	event := testEvent()
	payload := FormatDiscord(event)

	if len(payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(payload.Embeds))
	}

	embed := payload.Embeds[0]

	if embed.Title != "Server Crashed" {
		t.Errorf("title = %q, want %q", embed.Title, "Server Crashed")
	}

	if embed.Description != event.Message {
		t.Errorf("description = %q, want %q", embed.Description, event.Message)
	}

	if embed.Color != ColorCritical {
		t.Errorf("color = 0x%X, want 0x%X", embed.Color, ColorCritical)
	}

	if embed.Timestamp != "2026-03-23T12:00:00Z" {
		t.Errorf("timestamp = %q, want %q", embed.Timestamp, "2026-03-23T12:00:00Z")
	}

	if len(embed.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(embed.Fields))
	}

	// Fields are sorted by key: "Exit Code" before "Uptime".
	if embed.Fields[0].Name != "Exit Code" || embed.Fields[0].Value != "1" {
		t.Errorf("field[0] = %+v, want Exit Code=1", embed.Fields[0])
	}
	if embed.Fields[1].Name != "Uptime" || embed.Fields[1].Value != "3h 42m" {
		t.Errorf("field[1] = %+v, want Uptime=3h 42m", embed.Fields[1])
	}

	for i, f := range embed.Fields {
		if !f.Inline {
			t.Errorf("field[%d].Inline = false, want true", i)
		}
	}
}

func TestFormatDiscord_SeverityColors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		severity Severity
		want     int
	}{
		{"Info", SeverityInfo, ColorInfo},
		{"Warning", SeverityWarning, ColorWarning},
		{"Critical", SeverityCritical, ColorCritical},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			event := AlertEvent{
				EventType: "ALERT_EVENT_TYPE_STATUS_CHANGE",
				Message:   "test",
				Severity:  tc.severity,
				Timestamp: time.Now(),
			}
			payload := FormatDiscord(event)
			got := payload.Embeds[0].Color
			if got != tc.want {
				t.Errorf("color = 0x%X, want 0x%X", got, tc.want)
			}
		})
	}
}

func TestFormatSlack_CrashEvent(t *testing.T) {
	t.Parallel()

	event := testEvent()
	payload := FormatSlack(event)

	// Expect 4 blocks: header, section (message), section (fields), context.
	if len(payload.Blocks) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(payload.Blocks))
	}

	// Header block.
	header := payload.Blocks[0]
	if header.Type != "header" {
		t.Errorf("block[0].Type = %q, want %q", header.Type, "header")
	}
	if header.Text == nil || header.Text.Text != "Server Crashed" {
		t.Errorf("header text = %v, want %q", header.Text, "Server Crashed")
	}

	// Section block with message.
	section := payload.Blocks[1]
	if section.Type != "section" {
		t.Errorf("block[1].Type = %q, want %q", section.Type, "section")
	}
	if section.Text == nil || section.Text.Text != event.Message {
		t.Errorf("section text = %v, want %q", section.Text, event.Message)
	}
	if section.Text != nil && section.Text.Type != "mrkdwn" {
		t.Errorf("section text type = %q, want %q", section.Text.Type, "mrkdwn")
	}

	// Section block with fields.
	fieldsBlock := payload.Blocks[2]
	if fieldsBlock.Type != "section" {
		t.Errorf("block[2].Type = %q, want %q", fieldsBlock.Type, "section")
	}
	if len(fieldsBlock.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fieldsBlock.Fields))
	}

	// Context block with timestamp.
	ctxBlock := payload.Blocks[3]
	if ctxBlock.Type != "context" {
		t.Errorf("block[3].Type = %q, want %q", ctxBlock.Type, "context")
	}
	if len(ctxBlock.Elements) != 1 {
		t.Fatalf("expected 1 context element, got %d", len(ctxBlock.Elements))
	}
	if ctxBlock.Elements[0].Type != "mrkdwn" {
		t.Errorf("context element type = %q, want %q", ctxBlock.Elements[0].Type, "mrkdwn")
	}
}

func TestFormatGeneric_CrashEvent(t *testing.T) {
	t.Parallel()

	event := testEvent()
	payload := FormatGeneric(event)

	if payload.EventType != event.EventType {
		t.Errorf("EventType = %q, want %q", payload.EventType, event.EventType)
	}
	if payload.Severity != "critical" {
		t.Errorf("Severity = %q, want %q", payload.Severity, "critical")
	}
	if payload.Message != event.Message {
		t.Errorf("Message = %q, want %q", payload.Message, event.Message)
	}
	if payload.ServerName != event.ServerName {
		t.Errorf("ServerName = %q, want %q", payload.ServerName, event.ServerName)
	}
	if payload.ServerID != event.ServerID {
		t.Errorf("ServerID = %q, want %q", payload.ServerID, event.ServerID)
	}
	if payload.ServerNodeID != event.ServerNodeID {
		t.Errorf("ServerNodeID = %q, want %q", payload.ServerNodeID, event.ServerNodeID)
	}
	if payload.Timestamp != "2026-03-23T12:00:00Z" {
		t.Errorf("Timestamp = %q, want %q", payload.Timestamp, "2026-03-23T12:00:00Z")
	}
	if len(payload.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(payload.Fields))
	}
	if payload.Fields["Exit Code"] != "1" {
		t.Errorf("Fields[Exit Code] = %q, want %q", payload.Fields["Exit Code"], "1")
	}
}

// --- Delivery tests ---

func TestSend_Discord_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}

		var payload DiscordPayload
		errDecode := json.NewDecoder(r.Body).Decode(&payload)
		if errDecode != nil {
			t.Fatalf("failed to decode request body: %v", errDecode)
		}
		if len(payload.Embeds) != 1 {
			t.Errorf("expected 1 embed, got %d", len(payload.Embeds))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := newTestSender()
	config := ChannelConfig{URL: server.URL}
	errSend := sender.Send(context.Background(), ChannelTypeDiscord, config, testEvent())
	if errSend != nil {
		t.Fatalf("Send returned error: %v", errSend)
	}
}

func TestSend_Slack_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload SlackPayload
		errDecode := json.NewDecoder(r.Body).Decode(&payload)
		if errDecode != nil {
			t.Fatalf("failed to decode request body: %v", errDecode)
		}
		if len(payload.Blocks) < 3 {
			t.Errorf("expected at least 3 blocks, got %d", len(payload.Blocks))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := newTestSender()
	config := ChannelConfig{URL: server.URL}
	errSend := sender.Send(context.Background(), ChannelTypeSlack, config, testEvent())
	if errSend != nil {
		t.Fatalf("Send returned error: %v", errSend)
	}
}

func TestSend_Generic_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload GenericPayload
		errDecode := json.NewDecoder(r.Body).Decode(&payload)
		if errDecode != nil {
			t.Fatalf("failed to decode request body: %v", errDecode)
		}
		if payload.EventType != "ALERT_EVENT_TYPE_CRASH" {
			t.Errorf("EventType = %q, want ALERT_EVENT_TYPE_CRASH", payload.EventType)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := newTestSender()
	config := ChannelConfig{URL: server.URL}
	errSend := sender.Send(context.Background(), ChannelTypeGeneric, config, testEvent())
	if errSend != nil {
		t.Fatalf("Send returned error: %v", errSend)
	}
}

func TestSend_RetryOn5xx(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := newTestSender()
	config := ChannelConfig{URL: server.URL}
	errSend := sender.Send(context.Background(), ChannelTypeGeneric, config, testEvent())
	if errSend != nil {
		t.Fatalf("Send returned error: %v", errSend)
	}

	got := int(attempts.Load())
	if got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestSend_FailAfterMaxRetries(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	sender := newTestSender()
	config := ChannelConfig{URL: server.URL}
	errSend := sender.Send(context.Background(), ChannelTypeGeneric, config, testEvent())
	if errSend == nil {
		t.Fatal("expected error after max retries, got nil")
	}

	var deliveryErr *DeliveryError
	if !errors.As(errSend, &deliveryErr) {
		t.Fatalf("expected DeliveryError, got %T: %v", errSend, errSend)
	}
	if deliveryErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", deliveryErr.StatusCode, http.StatusInternalServerError)
	}

	got := int(attempts.Load())
	if got != 3 {
		t.Errorf("attempts = %d, want 3 (max retries)", got)
	}
}

func TestSend_RedactsWebhookURLInLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var logBuffer bytes.Buffer
	originalLogger := log.Logger
	log.Logger = zerolog.New(&logBuffer)
	t.Cleanup(func() {
		log.Logger = originalLogger
	})

	sender := newTestSender()
	config := ChannelConfig{URL: server.URL + "/api/webhooks/123/super-secret-token"}
	errSend := sender.Send(context.Background(), ChannelTypeGeneric, config, testEvent())
	if errSend == nil {
		t.Fatal("expected send error, got nil")
	}

	logOutput := logBuffer.String()
	if strings.Contains(logOutput, "super-secret-token") {
		t.Fatalf("log output leaked webhook token: %s", logOutput)
	}
	if strings.Contains(logOutput, "/api/webhooks/123/super-secret-token") {
		t.Fatalf("log output leaked webhook path: %s", logOutput)
	}
	if !strings.Contains(logOutput, server.URL) {
		t.Fatalf("log output = %q, want redacted base URL %q", logOutput, server.URL)
	}
}

func TestSend_RejectsUnsupportedWebhookURLScheme(t *testing.T) {
	t.Parallel()

	sender := newTestSender()
	errSend := sender.Send(
		context.Background(),
		ChannelTypeGeneric,
		ChannelConfig{URL: "file:///tmp/webhook"},
		testEvent(),
	)
	if errSend == nil {
		t.Fatal("expected invalid webhook URL error, got nil")
	}
}

func TestSend_RateLimited(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := &Sender{
		client:      &http.Client{Timeout: 2 * time.Second},
		rateLimiter: newRateLimiter(10),
		retry: retryConfig{
			MaxAttempts: 3,
			BaseDelay:   time.Millisecond,
		},
		ssrfCheckFn: nil,
	}

	config := ChannelConfig{URL: server.URL}
	event := testEvent()

	// Exhaust the rate limit.
	for range 10 {
		errSend := sender.Send(context.Background(), ChannelTypeGeneric, config, event)
		if errSend != nil {
			t.Fatalf("Send returned error before rate limit: %v", errSend)
		}
	}

	// The 11th request should be rate limited.
	errSend := sender.Send(context.Background(), ChannelTypeGeneric, config, event)
	if !errors.Is(errSend, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", errSend)
	}
}

func TestRateLimiter_RemovesExpiredKeysAcrossChannels(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(10)
	rl.windows["stale"] = []time.Time{time.Now().Add(-2 * time.Minute)}

	allowed := rl.Allow("fresh")
	if !allowed {
		t.Fatal("Allow(fresh) = false, want true")
	}
	if _, exists := rl.windows["stale"]; exists {
		t.Fatalf("stale rate limiter key was not removed: %#v", rl.windows)
	}
}

func TestSend_Timeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Delay longer than the client timeout.
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := &Sender{
		client:      &http.Client{Timeout: 50 * time.Millisecond},
		rateLimiter: newRateLimiter(100),
		retry: retryConfig{
			MaxAttempts: 1, // no retries for timeout test
			BaseDelay:   time.Millisecond,
		},
		ssrfCheckFn: nil,
	}

	config := ChannelConfig{URL: server.URL}
	errSend := sender.Send(context.Background(), ChannelTypeGeneric, config, testEvent())
	if errSend == nil {
		t.Fatal("expected timeout error, got nil")
	}

	var deliveryErr *DeliveryError
	if !errors.As(errSend, &deliveryErr) {
		t.Fatalf("expected DeliveryError wrapping timeout, got %T: %v", errSend, errSend)
	}
	if deliveryErr.Err == nil {
		t.Fatal("expected wrapped error for timeout, got nil")
	}
}

func TestSend_DeliveryErrorDetails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid payload"}`))
	}))
	defer server.Close()

	sender := newTestSender()
	config := ChannelConfig{URL: server.URL}
	errSend := sender.Send(context.Background(), ChannelTypeGeneric, config, testEvent())
	if errSend == nil {
		t.Fatal("expected DeliveryError, got nil")
	}

	var deliveryErr *DeliveryError
	if !errors.As(errSend, &deliveryErr) {
		t.Fatalf("expected DeliveryError, got %T: %v", errSend, errSend)
	}
	if deliveryErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d", deliveryErr.StatusCode, http.StatusBadRequest)
	}
	if deliveryErr.Body != `{"error": "invalid payload"}` {
		t.Errorf("Body = %q, want %q", deliveryErr.Body, `{"error": "invalid payload"}`)
	}
}

func TestSend_NoRetryOn4xx(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer server.Close()

	sender := newTestSender()
	config := ChannelConfig{URL: server.URL}
	errSend := sender.Send(context.Background(), ChannelTypeGeneric, config, testEvent())
	if errSend == nil {
		t.Fatal("expected error for 4xx, got nil")
	}

	var deliveryErr *DeliveryError
	if !errors.As(errSend, &deliveryErr) {
		t.Fatalf("expected DeliveryError, got %T: %v", errSend, errSend)
	}
	if deliveryErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d", deliveryErr.StatusCode, http.StatusBadRequest)
	}

	got := int(attempts.Load())
	if got != 1 {
		t.Errorf("attempts = %d, want 1 (4xx should not retry)", got)
	}
}

// --- SSRF protection tests ---

func TestIsPrivateOrReservedIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"loopback IPv4", "127.0.0.1", true},
		{"loopback IPv4 other", "127.0.0.2", true},
		{"loopback IPv6", "::1", true},
		{"private 10.x", "10.0.0.1", true},
		{"private 172.16.x", "172.16.0.1", true},
		{"private 172.31.x", "172.31.255.255", true},
		{"private 192.168.x", "192.168.1.1", true},
		{"link-local IPv4", "169.254.1.1", true},
		{"cloud metadata", "169.254.169.254", true},
		{"link-local IPv6", "fe80::1", true},
		{"unique-local IPv6", "fd00::1", true},
		{"unspecified IPv4", "0.0.0.0", true},
		{"unspecified IPv6", "::", true},
		{"public IPv4", "8.8.8.8", false},
		{"public IPv4 other", "1.1.1.1", false},
		{"non-private 172", "172.15.0.1", false},
		{"non-private 172 upper", "172.32.0.1", false},
		{"CGNAT shared address", "100.64.0.1", true},
		{"CGNAT upper", "100.127.255.255", true},
		{"non-CGNAT 100.63", "100.63.255.255", false},
		{"non-CGNAT 100.128", "100.128.0.1", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", tc.ip)
			}
			got := isPrivateOrReservedIP(ip)
			if got != tc.want {
				t.Errorf("isPrivateOrReservedIP(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestValidateWebhookTarget_BlocksLoopback(t *testing.T) {
	t.Parallel()

	errValidate := ValidateWebhookTarget("http://127.0.0.1:8080/hook")
	if errValidate == nil {
		t.Fatal("ValidateWebhookTarget(loopback) expected error, got nil")
	}
	if !errors.Is(errValidate, ErrSSRFBlocked) {
		t.Errorf("error = %v, want ErrSSRFBlocked", errValidate)
	}
}

func TestValidateWebhookTarget_BlocksPrivate(t *testing.T) {
	t.Parallel()

	errValidate := ValidateWebhookTarget("http://192.168.1.1:9000/webhook")
	if errValidate == nil {
		t.Fatal("ValidateWebhookTarget(private) expected error, got nil")
	}
	if !errors.Is(errValidate, ErrSSRFBlocked) {
		t.Errorf("error = %v, want ErrSSRFBlocked", errValidate)
	}
}

func TestValidateWebhookTarget_BlocksMetadata(t *testing.T) {
	t.Parallel()

	errValidate := ValidateWebhookTarget("http://169.254.169.254/latest/meta-data/")
	if errValidate == nil {
		t.Fatal("ValidateWebhookTarget(metadata) expected error, got nil")
	}
}

func TestValidateWebhookTarget_AllowsPublic(t *testing.T) {
	t.Parallel()

	// Use IP literal to avoid DNS lookup in test.
	errValidate := ValidateWebhookTarget("https://8.8.8.8/webhook")
	if errValidate != nil {
		t.Fatalf("ValidateWebhookTarget(public IP) error = %v", errValidate)
	}
}

func TestRejectRedirects(t *testing.T) {
	t.Parallel()

	errRedirect := rejectRedirects(nil, nil)
	if !errors.Is(errRedirect, errRedirectsDisabled) {
		t.Fatalf("rejectRedirects() error = %v, want %v", errRedirect, errRedirectsDisabled)
	}
}

func TestDialValidatedBlocksPrivateResolvedIP(t *testing.T) {
	t.Parallel()

	originalLookup := lookupIPAddr
	lookupIPAddr = func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
	}
	t.Cleanup(func() {
		lookupIPAddr = originalLookup
	})

	dialer := &net.Dialer{Timeout: 50 * time.Millisecond}
	_, errDial := dialValidated(context.Background(), "tcp", "metadata.example:80", dialer)
	if !errors.Is(errDial, ErrSSRFBlocked) {
		t.Fatalf("dialValidated() error = %v, want ErrSSRFBlocked", errDial)
	}
}

func TestValidateWebhookTarget_InvalidURL(t *testing.T) {
	t.Parallel()

	errValidate := ValidateWebhookTarget("://bad")
	if errValidate == nil {
		t.Fatal("ValidateWebhookTarget(invalid URL) expected error, got nil")
	}
}

// --- Human-readable title tests ---

func TestEventTypeTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		eventType string
		want      string
	}{
		{"ALERT_EVENT_TYPE_CRASH", "Server Crashed"},
		{"ALERT_EVENT_TYPE_STATUS_CHANGE", "Server Status Changed"},
		{"ALERT_EVENT_TYPE_CPU_THRESHOLD", "CPU Threshold Exceeded"},
		{"ALERT_EVENT_TYPE_MEMORY_THRESHOLD", "Memory Threshold Exceeded"},
		{"ALERT_EVENT_TYPE_DISK_THRESHOLD", "Disk Threshold Exceeded"},
		{"ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD", "Player Count Threshold"},
		{"ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", "Node CPU Threshold Exceeded"},
		{"ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD", "Node Memory Threshold Exceeded"},
		{"ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD", "Node Disk Threshold Exceeded"},
		{"ALERT_EVENT_TYPE_UNKNOWN_CUSTOM", "ALERT_EVENT_TYPE_UNKNOWN_CUSTOM"}, // fallback
	}

	for _, tc := range tests {
		t.Run(tc.eventType, func(t *testing.T) {
			t.Parallel()

			got := EventTypeTitle(tc.eventType)
			if got != tc.want {
				t.Errorf("EventTypeTitle(%q) = %q, want %q", tc.eventType, got, tc.want)
			}
		})
	}
}
