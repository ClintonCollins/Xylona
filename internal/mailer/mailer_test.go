package mailer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/webhooks"
)

// testEvent returns a fully populated AlertEvent for use in tests.
func testEvent() webhooks.AlertEvent {
	return webhooks.AlertEvent{
		EventType:    "ALERT_EVENT_TYPE_CRASH",
		ServerName:   "Minecraft Survival",
		ServerID:     "srv-123",
		ServerNodeID: "node-456",
		NodeID:       "",
		Message:      "Server process exited with code 1",
		Severity:     webhooks.SeverityCritical,
		Timestamp:    time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC),
		Fields: map[string]string{
			"Exit Code": "1",
			"Uptime":    "3h 42m",
		},
	}
}

// testNodeEvent returns an AlertEvent for node-level alerts (no server).
func testNodeEvent() webhooks.AlertEvent {
	return webhooks.AlertEvent{
		EventType: "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
		NodeID:    "node-789",
		Message:   "CPU usage at 95%",
		Severity:  webhooks.SeverityWarning,
		Timestamp: time.Date(2026, 3, 23, 14, 30, 0, 0, time.UTC),
		Fields: map[string]string{
			"CPU Usage": "95%",
		},
	}
}

func testSMTPConfig() *SMTPConfig {
	return &SMTPConfig{
		Host:       "smtp.example.com",
		Port:       587,
		User:       "user@example.com",
		Password:   "secret",
		From:       "alerts@example.com",
		TLSEnabled: true,
	}
}

// --- Subject formatting tests ---

func TestFormatSubject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event webhooks.AlertEvent
		want  string
	}{
		{
			name:  "crash event with server name",
			event: testEvent(),
			want:  "Server Crashed \u2014 Minecraft Survival",
		},
		{
			name: "status change with server name",
			event: webhooks.AlertEvent{
				EventType:  "ALERT_EVENT_TYPE_STATUS_CHANGE",
				ServerName: "Valheim Dedicated",
				Timestamp:  time.Now(),
			},
			want: "Server Status Changed \u2014 Valheim Dedicated",
		},
		{
			name:  "node event without server name",
			event: testNodeEvent(),
			want:  "Node CPU Threshold Exceeded",
		},
		{
			name: "unknown event type with server name",
			event: webhooks.AlertEvent{
				EventType:  "ALERT_EVENT_TYPE_CUSTOM_UNKNOWN",
				ServerName: "Test Server",
				Timestamp:  time.Now(),
			},
			want: "ALERT_EVENT_TYPE_CUSTOM_UNKNOWN \u2014 Test Server",
		},
		{
			name: "empty server name omits separator",
			event: webhooks.AlertEvent{
				EventType: "ALERT_EVENT_TYPE_CRASH",
				Timestamp: time.Now(),
			},
			want: "Server Crashed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := FormatSubject(tc.event)
			if got != tc.want {
				t.Errorf("FormatSubject() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- Body formatting tests ---

func TestFormatBody_CrashEvent(t *testing.T) {
	t.Parallel()

	event := testEvent()
	body := FormatBody(event)

	// Should contain the event title.
	if !strings.Contains(body, "Server Crashed") {
		t.Errorf("body missing event title, got:\n%s", body)
	}

	// Should contain the message.
	if !strings.Contains(body, event.Message) {
		t.Errorf("body missing message, got:\n%s", body)
	}

	// Should contain severity.
	if !strings.Contains(body, "critical") {
		t.Errorf("body missing severity, got:\n%s", body)
	}

	// Should contain the server name.
	if !strings.Contains(body, "Minecraft Survival") {
		t.Errorf("body missing server name, got:\n%s", body)
	}

	// Should contain timestamp.
	if !strings.Contains(body, "2026-03-23") {
		t.Errorf("body missing timestamp, got:\n%s", body)
	}

	// Should contain fields sorted alphabetically.
	if !strings.Contains(body, "Exit Code: 1") {
		t.Errorf("body missing 'Exit Code: 1', got:\n%s", body)
	}
	if !strings.Contains(body, "Uptime: 3h 42m") {
		t.Errorf("body missing 'Uptime: 3h 42m', got:\n%s", body)
	}

	// Exit Code should appear before Uptime (alphabetical sort).
	idxExit := strings.Index(body, "Exit Code")
	idxUptime := strings.Index(body, "Uptime")
	if idxExit >= idxUptime {
		t.Errorf("fields not sorted alphabetically: Exit Code at %d, Uptime at %d", idxExit, idxUptime)
	}
}

func TestFormatBody_NodeEvent(t *testing.T) {
	t.Parallel()

	event := testNodeEvent()
	body := FormatBody(event)

	// Should NOT contain "Server:" line.
	if strings.Contains(body, "Server:") {
		t.Errorf("node event body should not have Server line, got:\n%s", body)
	}

	// Should contain the node ID.
	if !strings.Contains(body, "node-789") {
		t.Errorf("body missing node ID, got:\n%s", body)
	}

	// Should contain severity.
	if !strings.Contains(body, "warning") {
		t.Errorf("body missing severity, got:\n%s", body)
	}
}

func TestFormatBody_NoFields(t *testing.T) {
	t.Parallel()

	event := webhooks.AlertEvent{
		EventType:  "ALERT_EVENT_TYPE_STATUS_CHANGE",
		ServerName: "Test Server",
		Message:    "Server started",
		Severity:   webhooks.SeverityInfo,
		Timestamp:  time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC),
	}
	body := FormatBody(event)

	// Should not have an empty "Details:" section.
	if strings.Contains(body, "Details:") {
		t.Errorf("body should not have Details section with no fields, got:\n%s", body)
	}

	// Should still contain the core info.
	if !strings.Contains(body, "Server Status Changed") {
		t.Errorf("body missing event title, got:\n%s", body)
	}
	if !strings.Contains(body, "Server started") {
		t.Errorf("body missing message, got:\n%s", body)
	}
}

func TestFormatBody_FieldsAreSorted(t *testing.T) {
	t.Parallel()

	event := webhooks.AlertEvent{
		EventType: "ALERT_EVENT_TYPE_CRASH",
		Message:   "test",
		Severity:  webhooks.SeverityInfo,
		Timestamp: time.Now(),
		Fields: map[string]string{
			"Zebra":  "last",
			"Alpha":  "first",
			"Middle": "mid",
		},
	}
	body := FormatBody(event)

	idxAlpha := strings.Index(body, "Alpha")
	idxMiddle := strings.Index(body, "Middle")
	idxZebra := strings.Index(body, "Zebra")

	if idxAlpha < 0 || idxMiddle < 0 || idxZebra < 0 {
		t.Fatalf("missing field in body:\n%s", body)
	}
	if idxAlpha >= idxMiddle || idxMiddle >= idxZebra {
		t.Errorf("fields not in alphabetical order: Alpha=%d, Middle=%d, Zebra=%d", idxAlpha, idxMiddle, idxZebra)
	}
}

// --- Config resolution tests ---

func TestResolveConfig_PerChannelOverridesSystem(t *testing.T) {
	t.Parallel()

	systemCfg := &SMTPConfig{
		Host: "system.example.com",
		Port: 25,
		From: "system@example.com",
	}
	channelCfg := &SMTPConfig{
		Host: "channel.example.com",
		Port: 587,
		From: "channel@example.com",
	}

	got := resolveConfig(systemCfg, channelCfg)
	if got.Host != "channel.example.com" {
		t.Errorf("Host = %q, want %q", got.Host, "channel.example.com")
	}
	if got.Port != 587 {
		t.Errorf("Port = %d, want %d", got.Port, 587)
	}
}

func TestResolveConfig_FallbackToSystem(t *testing.T) {
	t.Parallel()

	systemCfg := &SMTPConfig{
		Host: "system.example.com",
		Port: 25,
		From: "system@example.com",
	}

	got := resolveConfig(systemCfg, nil)
	if got.Host != "system.example.com" {
		t.Errorf("Host = %q, want %q", got.Host, "system.example.com")
	}
}

func TestResolveConfig_EmptyHostFallsBackToSystem(t *testing.T) {
	t.Parallel()

	systemCfg := &SMTPConfig{
		Host: "system.example.com",
		Port: 25,
		From: "system@example.com",
	}
	channelCfg := &SMTPConfig{
		Host: "", // empty host means not configured
		Port: 0,
	}

	got := resolveConfig(systemCfg, channelCfg)
	if got.Host != "system.example.com" {
		t.Errorf("Host = %q, want %q (should fall back to system)", got.Host, "system.example.com")
	}
}

func TestResolveConfig_BothNilReturnsNil(t *testing.T) {
	t.Parallel()

	got := resolveConfig(nil, nil)
	if got != nil {
		t.Errorf("expected nil when both configs are nil, got %+v", got)
	}
}

func TestResolveConfig_NilSystemEmptyChannelReturnsNil(t *testing.T) {
	t.Parallel()

	channelCfg := &SMTPConfig{Host: ""}
	got := resolveConfig(nil, channelCfg)
	if got != nil {
		t.Errorf("expected nil when system is nil and channel host is empty, got %+v", got)
	}
}

// --- SystemConfigResolver tests ---

type fakeSystemConfigResolver struct {
	config *SMTPConfig
	err    error
}

func (f *fakeSystemConfigResolver) ResolveSystemSMTPConfig() (*SMTPConfig, error) {
	return f.config, f.err
}

func TestMailer_Send_SystemResolverUsedWhenNoPerChannelConfig(t *testing.T) {
	t.Parallel()

	var capturedConfig *SMTPConfig

	resolver := &fakeSystemConfigResolver{
		config: &SMTPConfig{
			Host: "resolved-system.example.com",
			Port: 587,
			From: "system@example.com",
		},
	}

	m := New(resolver)
	m.retry = retryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond}
	m.sendFunc = func(_ context.Context, config *SMTPConfig, _ string, _ string, _ string) error {
		capturedConfig = config
		return nil
	}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), nil)
	if errSend != nil {
		t.Fatalf("Send returned error: %v", errSend)
	}

	if capturedConfig == nil {
		t.Fatal("capturedConfig is nil, expected resolved system config")
	}
	if capturedConfig.Host != "resolved-system.example.com" {
		t.Errorf("config host = %q, want %q", capturedConfig.Host, "resolved-system.example.com")
	}
}

func TestMailer_Send_ResolverErrorReturnsErrNoSMTPConfig(t *testing.T) {
	t.Parallel()

	resolver := &fakeSystemConfigResolver{
		err: errors.New("db unavailable"),
	}

	m := New(resolver)
	m.retry = retryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), nil)
	if errSend == nil {
		t.Fatal("expected error when resolver fails, got nil")
	}
	if !errors.Is(errSend, ErrNoSMTPConfig) {
		t.Errorf("expected ErrNoSMTPConfig, got: %v", errSend)
	}
}

func TestMailer_Send_ResolverReturnsNilConfig(t *testing.T) {
	t.Parallel()

	resolver := &fakeSystemConfigResolver{
		config: nil,
		err:    nil,
	}

	m := New(resolver)
	m.retry = retryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), nil)
	if errSend == nil {
		t.Fatal("expected error when resolver returns nil config, got nil")
	}
	if !errors.Is(errSend, ErrNoSMTPConfig) {
		t.Errorf("expected ErrNoSMTPConfig, got: %v", errSend)
	}
}

func TestMailer_Send_NilResolverNoPerChannelConfig(t *testing.T) {
	t.Parallel()

	m := New(nil)
	m.retry = retryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), nil)
	if errSend == nil {
		t.Fatal("expected error when resolver is nil and no per-channel config, got nil")
	}
	if !errors.Is(errSend, ErrNoSMTPConfig) {
		t.Errorf("expected ErrNoSMTPConfig, got: %v", errSend)
	}
}

func TestMailer_Send_PerChannelOverridesResolver(t *testing.T) {
	t.Parallel()

	var capturedConfig *SMTPConfig

	resolver := &fakeSystemConfigResolver{
		config: &SMTPConfig{
			Host: "system.example.com",
			Port: 25,
			From: "system@example.com",
		},
	}

	m := New(resolver)
	m.retry = retryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond}
	m.sendFunc = func(_ context.Context, config *SMTPConfig, _ string, _ string, _ string) error {
		capturedConfig = config
		return nil
	}

	channelCfg := &SMTPConfig{
		Host: "channel.example.com",
		Port: 465,
		From: "channel@example.com",
	}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), channelCfg)
	if errSend != nil {
		t.Fatalf("Send returned error: %v", errSend)
	}

	if capturedConfig.Host != "channel.example.com" {
		t.Errorf("config host = %q, want %q", capturedConfig.Host, "channel.example.com")
	}
}

// --- Send tests using sendFunc injection ---

func TestMailer_Send_Success(t *testing.T) {
	t.Parallel()

	var capturedTo, capturedSubject, capturedBody string
	var capturedConfig *SMTPConfig

	m := New(&fakeSystemConfigResolver{config: testSMTPConfig()})
	m.retry = retryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
	}
	m.sendFunc = func(_ context.Context, config *SMTPConfig, to string, subject string, body string) error {
		capturedTo = to
		capturedSubject = subject
		capturedBody = body
		capturedConfig = config
		return nil
	}

	event := testEvent()
	errSend := m.Send(context.Background(), "admin@example.com", event, nil)
	if errSend != nil {
		t.Fatalf("Send returned error: %v", errSend)
	}

	if capturedTo != "admin@example.com" {
		t.Errorf("to = %q, want %q", capturedTo, "admin@example.com")
	}
	wantSubject := "Server Crashed \u2014 Minecraft Survival"
	if capturedSubject != wantSubject {
		t.Errorf("subject = %q, want %q", capturedSubject, wantSubject)
	}
	if !strings.Contains(capturedBody, event.Message) {
		t.Errorf("body missing message, got:\n%s", capturedBody)
	}
	if capturedConfig.Host != "smtp.example.com" {
		t.Errorf("config host = %q, want %q", capturedConfig.Host, "smtp.example.com")
	}
}

func TestMailer_Send_PerChannelConfigOverride(t *testing.T) {
	t.Parallel()

	var capturedConfig *SMTPConfig

	m := New(&fakeSystemConfigResolver{config: testSMTPConfig()})
	m.retry = retryConfig{
		MaxAttempts: 1,
		BaseDelay:   time.Millisecond,
	}
	m.sendFunc = func(_ context.Context, config *SMTPConfig, _ string, _ string, _ string) error {
		capturedConfig = config
		return nil
	}

	channelCfg := &SMTPConfig{
		Host: "channel-smtp.example.com",
		Port: 465,
		From: "channel@example.com",
	}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), channelCfg)
	if errSend != nil {
		t.Fatalf("Send returned error: %v", errSend)
	}

	if capturedConfig.Host != "channel-smtp.example.com" {
		t.Errorf("config host = %q, want %q", capturedConfig.Host, "channel-smtp.example.com")
	}
}

func TestMailer_Send_NoConfigAvailable(t *testing.T) {
	t.Parallel()

	m := New(nil) // no system config resolver
	m.retry = retryConfig{
		MaxAttempts: 1,
		BaseDelay:   time.Millisecond,
	}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), nil)
	if errSend == nil {
		t.Fatal("expected error when no SMTP config is available, got nil")
	}
	if !errors.Is(errSend, ErrNoSMTPConfig) {
		t.Errorf("expected ErrNoSMTPConfig, got: %v", errSend)
	}
}

// --- Retry tests ---

func TestMailer_Send_RetriesOnError(t *testing.T) {
	t.Parallel()

	attempts := 0

	m := New(&fakeSystemConfigResolver{config: testSMTPConfig()})
	m.retry = retryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
	}
	m.sendFunc = func(_ context.Context, _ *SMTPConfig, _ string, _ string, _ string) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary SMTP failure")
		}
		return nil
	}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), nil)
	if errSend != nil {
		t.Fatalf("Send returned error: %v", errSend)
	}

	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestMailer_Send_FailsAfterMaxRetries(t *testing.T) {
	t.Parallel()

	attempts := 0

	m := New(&fakeSystemConfigResolver{config: testSMTPConfig()})
	m.retry = retryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
	}
	m.sendFunc = func(_ context.Context, _ *SMTPConfig, _ string, _ string, _ string) error {
		attempts++
		return errors.New("persistent SMTP failure")
	}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), nil)
	if errSend == nil {
		t.Fatal("expected error after max retries, got nil")
	}

	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}

	if !strings.Contains(errSend.Error(), "persistent SMTP failure") {
		t.Errorf("error = %q, want to contain %q", errSend.Error(), "persistent SMTP failure")
	}
}

func TestMailer_Send_NoRetryOnSingleAttempt(t *testing.T) {
	t.Parallel()

	attempts := 0

	m := New(&fakeSystemConfigResolver{config: testSMTPConfig()})
	m.retry = retryConfig{
		MaxAttempts: 1,
		BaseDelay:   time.Millisecond,
	}
	m.sendFunc = func(_ context.Context, _ *SMTPConfig, _ string, _ string, _ string) error {
		attempts++
		return errors.New("SMTP failure")
	}

	errSend := m.Send(context.Background(), "admin@example.com", testEvent(), nil)
	if errSend == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestMailer_Send_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	m := New(&fakeSystemConfigResolver{config: testSMTPConfig()})
	m.retry = retryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Second, // long delay to allow cancellation
	}
	m.sendFunc = func(_ context.Context, _ *SMTPConfig, _ string, _ string, _ string) error {
		attempts++
		// Cancel after first attempt, before retry delay
		cancel()
		return errors.New("first attempt fails")
	}

	errSend := m.Send(ctx, "admin@example.com", testEvent(), nil)
	if errSend == nil {
		t.Fatal("expected error from context cancellation, got nil")
	}
	if !strings.Contains(errSend.Error(), "context cancelled") {
		t.Errorf("error = %q, want to contain %q", errSend.Error(), "context cancelled")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (should not retry after cancellation)", attempts)
	}
}

// --- SMTP message building tests ---

func TestBuildMessage(t *testing.T) {
	t.Parallel()

	config := testSMTPConfig()
	msg := buildMessage(config.From, "admin@example.com", "Test Subject", "Test body content")

	// Should contain required headers.
	if !strings.Contains(msg, "From: alerts@example.com") {
		t.Errorf("message missing From header, got:\n%s", msg)
	}
	if !strings.Contains(msg, "To: admin@example.com") {
		t.Errorf("message missing To header, got:\n%s", msg)
	}
	if !strings.Contains(msg, "Subject: Test Subject") {
		t.Errorf("message missing Subject header, got:\n%s", msg)
	}
	if !strings.Contains(msg, "MIME-Version: 1.0") {
		t.Errorf("message missing MIME-Version header, got:\n%s", msg)
	}
	if !strings.Contains(msg, "Content-Type: text/plain; charset=UTF-8") {
		t.Errorf("message missing Content-Type header, got:\n%s", msg)
	}
	if !strings.Contains(msg, "Date: ") {
		t.Errorf("message missing Date header, got:\n%s", msg)
	}

	// Headers and body should be separated by a blank line.
	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("message should have header/body separator, got:\n%s", msg)
	}
	if parts[1] != "Test body content" {
		t.Errorf("body = %q, want %q", parts[1], "Test body content")
	}
}

func TestBuildMessage_SpecialCharactersInSubject(t *testing.T) {
	t.Parallel()

	msg := buildMessage("from@test.com", "to@test.com", "Server Crashed \u2014 My Server", "body")
	if !strings.Contains(msg, "Subject: Server Crashed \u2014 My Server") {
		t.Errorf("subject not preserved, got:\n%s", msg)
	}
}

func TestBuildMessage_StripsHeaderInjectionSequences(t *testing.T) {
	t.Parallel()

	msg := buildMessage(
		"alerts@example.com\r\nBcc: victim@example.com",
		"admin@example.com\r\nCc: spy@example.com",
		"Alert\r\nX-Injected: yes",
		"body",
	)

	for _, injectedHeader := range []string{"\r\nBcc:", "\r\nCc:", "\r\nX-Injected:"} {
		if strings.Contains(msg, injectedHeader) {
			t.Fatalf("message leaked injected header %q:\n%s", injectedHeader, msg)
		}
	}
}

// --- SendTestEmail tests ---

func TestSendTestEmail_CallsSendFuncWithTestContent(t *testing.T) {
	t.Parallel()

	var capturedTo, capturedSubject, capturedBody string
	var capturedConfig *SMTPConfig

	fakeSend := func(_ context.Context, config *SMTPConfig, to string, subject string, body string) error {
		capturedConfig = config
		capturedTo = to
		capturedSubject = subject
		capturedBody = body
		return nil
	}

	cfg := testSMTPConfig()
	errSend := sendTestEmailWithSender(context.Background(), cfg, "recipient@example.com", fakeSend)
	if errSend != nil {
		t.Fatalf("sendTestEmail() error = %v", errSend)
	}

	if capturedTo != "recipient@example.com" {
		t.Errorf("to = %q, want %q", capturedTo, "recipient@example.com")
	}
	if capturedSubject != "Xylona SMTP Test" {
		t.Errorf("subject = %q, want %q", capturedSubject, "Xylona SMTP Test")
	}
	wantBody := "This is a test email from Xylona to verify your SMTP configuration."
	if capturedBody != wantBody {
		t.Errorf("body = %q, want %q", capturedBody, wantBody)
	}
	if capturedConfig.Host != cfg.Host {
		t.Errorf("config.Host = %q, want %q", capturedConfig.Host, cfg.Host)
	}
}

func TestSendTestEmail_PropagatesSendError(t *testing.T) {
	t.Parallel()

	fakeSend := func(_ context.Context, _ *SMTPConfig, _ string, _ string, _ string) error {
		return errors.New("SMTP connection refused")
	}

	cfg := testSMTPConfig()
	errSend := sendTestEmailWithSender(context.Background(), cfg, "recipient@example.com", fakeSend)
	if errSend == nil {
		t.Fatal("sendTestEmail() expected error, got nil")
	}
	if !strings.Contains(errSend.Error(), "SMTP connection refused") {
		t.Errorf("error = %q, want to contain %q", errSend.Error(), "SMTP connection refused")
	}
}

func TestSendTestEmail_NilConfigReturnsError(t *testing.T) {
	t.Parallel()

	fakeSend := func(_ context.Context, _ *SMTPConfig, _ string, _ string, _ string) error {
		return nil
	}

	errSend := sendTestEmailWithSender(context.Background(), nil, "recipient@example.com", fakeSend)
	if errSend == nil {
		t.Fatal("sendTestEmail(nil config) expected error, got nil")
	}
	if !errors.Is(errSend, ErrNoSMTPConfig) {
		t.Errorf("error = %v, want %v", errSend, ErrNoSMTPConfig)
	}
}

// --- SMTPConfig address formatting test ---

func TestSMTPConfig_Addr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{"standard TLS", "smtp.example.com", 587, "smtp.example.com:587"},
		{"SSL port", "smtp.example.com", 465, "smtp.example.com:465"},
		{"plain port", "smtp.example.com", 25, "smtp.example.com:25"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &SMTPConfig{Host: tc.host, Port: tc.port}
			got := cfg.Addr()
			if got != tc.want {
				t.Errorf("Addr() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- Severity string in body test ---

func TestFormatBody_AllSeverities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		severity webhooks.Severity
		want     string
	}{
		{"info", webhooks.SeverityInfo, "info"},
		{"warning", webhooks.SeverityWarning, "warning"},
		{"critical", webhooks.SeverityCritical, "critical"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			event := webhooks.AlertEvent{
				EventType: "ALERT_EVENT_TYPE_CRASH",
				Message:   "test",
				Severity:  tc.severity,
				Timestamp: time.Now(),
			}
			body := FormatBody(event)
			if !strings.Contains(body, tc.want) {
				t.Errorf("body missing severity %q, got:\n%s", tc.want, body)
			}
		})
	}
}
