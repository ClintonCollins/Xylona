package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/mailer"
	"github.com/ClintonCollins/Xylona/pkg/webhooks"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// --- mock channel store ---

type mockChannelStore struct {
	mu       sync.Mutex
	channels map[string]*models.NotificationChannel
	err      error
}

func (m *mockChannelStore) GetNotificationChannelByID(id string) (*models.NotificationChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}
	ch, ok := m.channels[id]
	if !ok {
		return nil, errors.New("channel not found")
	}
	return ch, nil
}

// --- mock history store ---

type historyRecord struct {
	ID             string
	RuleID         string
	UserID         string
	ServerID       string
	ServerNodeID   string
	NodeID         string
	EventType      string
	EventData      string
	ChannelType    string
	DeliveryStatus string
	DeliveryError  string
}

type mockHistoryStore struct {
	mu      sync.Mutex
	records map[string]*historyRecord
	nextID  int
	// insertErr simulates an error on InsertAlertHistory.
	insertErr error
	// updateErr simulates an error on UpdateAlertHistoryDeliveryStatus.
	updateErr error
}

func newMockHistoryStore() *mockHistoryStore {
	return &mockHistoryStore{
		records: make(map[string]*historyRecord),
	}
}

func (m *mockHistoryStore) InsertAlertHistory(ruleID, userID, serverID, serverNodeID, nodeID, eventType, eventData, channelType, deliveryStatus string) (*models.AlertHistory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.insertErr != nil {
		return nil, m.insertErr
	}

	m.nextID++
	id := fmt.Sprintf("hist-%d", m.nextID)
	m.records[id] = &historyRecord{
		ID:             id,
		RuleID:         ruleID,
		UserID:         userID,
		ServerID:       serverID,
		ServerNodeID:   serverNodeID,
		NodeID:         nodeID,
		EventType:      eventType,
		EventData:      eventData,
		ChannelType:    channelType,
		DeliveryStatus: deliveryStatus,
	}
	return &models.AlertHistory{ID: id}, nil
}

func (m *mockHistoryStore) UpdateAlertHistoryDeliveryStatus(id, status, deliveryError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.updateErr != nil {
		return m.updateErr
	}

	rec, ok := m.records[id]
	if !ok {
		return errors.New("history record not found")
	}
	rec.DeliveryStatus = status
	rec.DeliveryError = deliveryError
	return nil
}

// allRecords returns copies of all records under the lock.
func (m *mockHistoryStore) allRecords() []historyRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]historyRecord, 0, len(m.records))
	for _, rec := range m.records {
		out = append(out, *rec)
	}
	return out
}

// --- mock webhook sender ---

type webhookCall struct {
	channelType string
	config      webhooks.ChannelConfig
	event       webhooks.AlertEvent
}

type mockWebhookSender struct {
	mu    sync.Mutex
	calls []webhookCall
	err   error
}

func (m *mockWebhookSender) Send(_ context.Context, channelType string, config webhooks.ChannelConfig, event webhooks.AlertEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, webhookCall{channelType: channelType, config: config, event: event})
	return m.err
}

func (m *mockWebhookSender) getCalls() []webhookCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]webhookCall, len(m.calls))
	copy(out, m.calls)
	return out
}

// --- mock email sender ---

type emailCall struct {
	to               string
	event            webhooks.AlertEvent
	perChannelConfig *mailer.SMTPConfig
}

type mockEmailSender struct {
	mu    sync.Mutex
	calls []emailCall
	err   error
}

func (m *mockEmailSender) Send(_ context.Context, to string, event webhooks.AlertEvent, perChannelConfig *mailer.SMTPConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, emailCall{to: to, event: event, perChannelConfig: perChannelConfig})
	return m.err
}

func (m *mockEmailSender) getCalls() []emailCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]emailCall, len(m.calls))
	copy(out, m.calls)
	return out
}

// --- helper to wait for mock state ---

// waitFor polls fn until it returns true or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("waitFor timed out")
}

// --- Tests ---

func TestDeliveryWorker_PicksUpJob(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-1": {
				ID:          "ch-1",
				UserID:      "u1",
				Name:        "discord-alerts",
				ChannelType: webhooks.ChannelTypeDiscord,
				Config:      `{"url":"https://discord.com/api/webhooks/test"}`,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobChan <- DeliveryJob{
		RuleID:       "r1",
		ChannelID:    "ch-1",
		UserID:       "u1",
		EventType:    "ALERT_EVENT_TYPE_CRASH",
		EventData:    `{"exit_code":137}`,
		ServerID:     "srv-1",
		ServerNodeID: "node-a",
	}

	// Wait for the webhook sender to receive the call.
	waitFor(t, 5*time.Second, func() bool {
		return len(whSender.getCalls()) == 1
	})

	calls := whSender.getCalls()
	if calls[0].channelType != webhooks.ChannelTypeDiscord {
		t.Errorf("webhook call channelType = %q, want %q", calls[0].channelType, webhooks.ChannelTypeDiscord)
	}
	if calls[0].config.URL != "https://discord.com/api/webhooks/test" {
		t.Errorf("webhook call URL = %q, want %q", calls[0].config.URL, "https://discord.com/api/webhooks/test")
	}
	if calls[0].event.EventType != "ALERT_EVENT_TYPE_CRASH" {
		t.Errorf("webhook call EventType = %q, want %q", calls[0].event.EventType, "ALERT_EVENT_TYPE_CRASH")
	}
	if calls[0].event.ServerID != "srv-1" {
		t.Errorf("webhook call ServerID = %q, want %q", calls[0].event.ServerID, "srv-1")
	}
}

func TestDeliveryWorker_ResolvesChannelFromStore(t *testing.T) {
	slackConfig := `{"url":"https://hooks.slack.com/services/test"}`
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-slack": {
				ID:          "ch-slack",
				UserID:      "u1",
				Name:        "slack-alerts",
				ChannelType: webhooks.ChannelTypeSlack,
				Config:      slackConfig,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobChan <- DeliveryJob{
		RuleID:    "r1",
		ChannelID: "ch-slack",
		UserID:    "u1",
		EventType: "ALERT_EVENT_TYPE_STATUS_CHANGE",
		EventData: `{"new_status":"offline","old_status":"online"}`,
		ServerID:  "srv-1",
	}

	waitFor(t, 5*time.Second, func() bool {
		return len(whSender.getCalls()) == 1
	})

	calls := whSender.getCalls()
	if calls[0].channelType != webhooks.ChannelTypeSlack {
		t.Errorf("channelType = %q, want %q", calls[0].channelType, webhooks.ChannelTypeSlack)
	}
	if calls[0].config.URL != "https://hooks.slack.com/services/test" {
		t.Errorf("config URL = %q, want %q", calls[0].config.URL, "https://hooks.slack.com/services/test")
	}
}

func TestDeliveryWorker_DispatchesToCorrectSender(t *testing.T) {
	tests := []struct {
		name          string
		channelType   string
		config        string
		wantWebhook   bool
		wantEmail     bool
		wantEmailTo   string
		wantSMTPHost  string
		wantChannelTy string
	}{
		{
			name:          "Discord webhook",
			channelType:   webhooks.ChannelTypeDiscord,
			config:        `{"url":"https://discord.com/api/webhooks/1"}`,
			wantWebhook:   true,
			wantChannelTy: webhooks.ChannelTypeDiscord,
		},
		{
			name:          "Slack webhook",
			channelType:   webhooks.ChannelTypeSlack,
			config:        `{"url":"https://hooks.slack.com/services/1"}`,
			wantWebhook:   true,
			wantChannelTy: webhooks.ChannelTypeSlack,
		},
		{
			name:          "Generic webhook",
			channelType:   webhooks.ChannelTypeGeneric,
			config:        `{"url":"https://example.com/webhook"}`,
			wantWebhook:   true,
			wantChannelTy: webhooks.ChannelTypeGeneric,
		},
		{
			name:         "Email",
			channelType:  ChannelTypeEmail,
			config:       `{"to":"admin@example.com","smtp_source":"custom","smtp_host":"mail.example.com","smtp_port":587,"smtp_user":"user","smtp_password":"pass","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
			wantEmail:    true,
			wantEmailTo:  "admin@example.com",
			wantSMTPHost: "mail.example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			channelStore := &mockChannelStore{
				channels: map[string]*models.NotificationChannel{
					"ch-test": {
						ID:          "ch-test",
						UserID:      "u1",
						Name:        "test-channel",
						ChannelType: tc.channelType,
						Config:      tc.config,
						Enabled:     1,
					},
				},
			}
			historyStore := newMockHistoryStore()
			whSender := &mockWebhookSender{}
			emailSender := &mockEmailSender{}

			jobChan := make(chan DeliveryJob, 8)
			pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

			ctx := t.Context()

			pool.Start(ctx)

			jobChan <- DeliveryJob{
				RuleID:       "r1",
				ChannelID:    "ch-test",
				UserID:       "u1",
				EventType:    "ALERT_EVENT_TYPE_CRASH",
				EventData:    `{"exit_code":1}`,
				ServerID:     "srv-1",
				ServerNodeID: "node-a",
			}

			if tc.wantWebhook {
				waitFor(t, 5*time.Second, func() bool {
					return len(whSender.getCalls()) == 1
				})
				calls := whSender.getCalls()
				if calls[0].channelType != tc.wantChannelTy {
					t.Errorf("webhook channelType = %q, want %q", calls[0].channelType, tc.wantChannelTy)
				}
			}

			if tc.wantEmail {
				waitFor(t, 5*time.Second, func() bool {
					return len(emailSender.getCalls()) == 1
				})
				calls := emailSender.getCalls()
				if calls[0].to != tc.wantEmailTo {
					t.Errorf("email to = %q, want %q", calls[0].to, tc.wantEmailTo)
				}
				if calls[0].perChannelConfig == nil {
					t.Fatal("email perChannelConfig = nil, want non-nil")
				}
				if calls[0].perChannelConfig.Host != tc.wantSMTPHost {
					t.Errorf("email SMTP host = %q, want %q", calls[0].perChannelConfig.Host, tc.wantSMTPHost)
				}
			}
		})
	}
}

func TestDeliveryWorker_UpdatesHistoryOnSuccess(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-1": {
				ID:          "ch-1",
				UserID:      "u1",
				Name:        "discord-alerts",
				ChannelType: webhooks.ChannelTypeDiscord,
				Config:      `{"url":"https://discord.com/api/webhooks/test"}`,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{} // no error — success
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobChan <- DeliveryJob{
		RuleID:       "r1",
		ChannelID:    "ch-1",
		UserID:       "u1",
		EventType:    "ALERT_EVENT_TYPE_CRASH",
		EventData:    "{}",
		ServerID:     "srv-1",
		ServerNodeID: "node-a",
	}

	// Wait for the history record to be updated.
	waitFor(t, 5*time.Second, func() bool {
		records := historyStore.allRecords()
		for _, rec := range records {
			if rec.DeliveryStatus == deliveryStatusSent {
				return true
			}
		}
		return false
	})

	records := historyStore.allRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(records))
	}
	rec := records[0]
	if rec.DeliveryStatus != deliveryStatusSent {
		t.Errorf("delivery status = %q, want %q", rec.DeliveryStatus, deliveryStatusSent)
	}
	if rec.DeliveryError != "" {
		t.Errorf("delivery error = %q, want empty", rec.DeliveryError)
	}
	if rec.ChannelType != webhooks.ChannelTypeDiscord {
		t.Errorf("channel type = %q, want %q", rec.ChannelType, webhooks.ChannelTypeDiscord)
	}
}

func TestDeliveryWorker_UpdatesHistoryOnFailure(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-1": {
				ID:          "ch-1",
				UserID:      "u1",
				Name:        "discord-alerts",
				ChannelType: webhooks.ChannelTypeDiscord,
				Config:      `{"url":"https://discord.com/api/webhooks/test"}`,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{err: errors.New("webhook delivery failed")}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobChan <- DeliveryJob{
		RuleID:       "r1",
		ChannelID:    "ch-1",
		UserID:       "u1",
		EventType:    "ALERT_EVENT_TYPE_CRASH",
		EventData:    "{}",
		ServerID:     "srv-1",
		ServerNodeID: "node-a",
	}

	waitFor(t, 5*time.Second, func() bool {
		records := historyStore.allRecords()
		for _, rec := range records {
			if rec.DeliveryStatus == deliveryStatusFailed {
				return true
			}
		}
		return false
	})

	records := historyStore.allRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(records))
	}
	rec := records[0]
	if rec.DeliveryStatus != deliveryStatusFailed {
		t.Errorf("delivery status = %q, want %q", rec.DeliveryStatus, deliveryStatusFailed)
	}
	if rec.DeliveryError == "" {
		t.Error("delivery error is empty, want non-empty error message")
	}
}

func TestDeliveryWorker_HandlesChannelLookupError(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{},
		// "ch-missing" is not in the map so it will return "channel not found".
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobChan <- DeliveryJob{
		RuleID:    "r1",
		ChannelID: "ch-missing",
		UserID:    "u1",
		EventType: "ALERT_EVENT_TYPE_CRASH",
		EventData: "{}",
		ServerID:  "srv-1",
	}

	// The history should record a FAILED status because the channel lookup failed.
	waitFor(t, 5*time.Second, func() bool {
		records := historyStore.allRecords()
		for _, rec := range records {
			if rec.DeliveryStatus == deliveryStatusFailed {
				return true
			}
		}
		return false
	})

	records := historyStore.allRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(records))
	}
	rec := records[0]
	if rec.DeliveryStatus != deliveryStatusFailed {
		t.Errorf("delivery status = %q, want %q", rec.DeliveryStatus, deliveryStatusFailed)
	}

	// Webhook sender should NOT have been called.
	if len(whSender.getCalls()) != 0 {
		t.Errorf("webhook sender called %d times, want 0", len(whSender.getCalls()))
	}
}

func TestDeliveryWorker_HandlesUnsupportedChannelType(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-1": {
				ID:          "ch-1",
				UserID:      "u1",
				Name:        "unknown-type",
				ChannelType: "NOTIFICATION_CHANNEL_TYPE_UNKNOWN",
				Config:      `{"url":"https://example.com"}`,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobChan <- DeliveryJob{
		RuleID:    "r1",
		ChannelID: "ch-1",
		UserID:    "u1",
		EventType: "ALERT_EVENT_TYPE_CRASH",
		EventData: "{}",
		ServerID:  "srv-1",
	}

	waitFor(t, 5*time.Second, func() bool {
		records := historyStore.allRecords()
		for _, rec := range records {
			if rec.DeliveryStatus == deliveryStatusFailed {
				return true
			}
		}
		return false
	})

	records := historyStore.allRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(records))
	}
	if records[0].DeliveryStatus != deliveryStatusFailed {
		t.Errorf("delivery status = %q, want %q", records[0].DeliveryStatus, deliveryStatusFailed)
	}
}

func TestDeliveryWorker_GracefulShutdown(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)

	// Cancel context and close the channel — workers should shut down without panic.
	cancel()
	close(jobChan)

	// Wait for all workers to exit deterministically.
	pool.Wait()
}

func TestDeliveryWorker_DrainsBufferedJobsOnCancellation(t *testing.T) {
	for iteration := range 20 {
		channelStore := &mockChannelStore{
			channels: map[string]*models.NotificationChannel{
				"ch-1": {
					ID:          "ch-1",
					UserID:      "u1",
					Name:        "discord-alerts",
					ChannelType: webhooks.ChannelTypeDiscord,
					Config:      `{"url":"https://discord.com/api/webhooks/test"}`,
					Enabled:     1,
				},
			},
		}
		historyStore := newMockHistoryStore()
		whSender := &mockWebhookSender{}
		emailSender := &mockEmailSender{}

		jobChan := make(chan DeliveryJob, 6)
		for jobIndex := range 6 {
			jobChan <- DeliveryJob{
				RuleID:       fmt.Sprintf("r%d", jobIndex),
				ChannelID:    "ch-1",
				UserID:       "u1",
				EventType:    "ALERT_EVENT_TYPE_CRASH",
				EventData:    `{"exit_code":1}`,
				ServerID:     "srv-1",
				ServerNodeID: "node-a",
			}
		}

		pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		pool.Start(ctx)
		close(jobChan)
		pool.Wait()

		if len(whSender.getCalls()) != 6 {
			t.Fatalf("iteration %d: webhook calls = %d, want 6", iteration, len(whSender.getCalls()))
		}

		records := historyStore.allRecords()
		if len(records) != 6 {
			t.Fatalf("iteration %d: history records = %d, want 6", iteration, len(records))
		}
		for _, rec := range records {
			if rec.DeliveryStatus != deliveryStatusSent {
				t.Fatalf("iteration %d: delivery status = %q, want %q", iteration, rec.DeliveryStatus, deliveryStatusSent)
			}
		}
	}
}

func TestDeliveryWorker_MultipleJobsProcessed(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-1": {
				ID:          "ch-1",
				UserID:      "u1",
				Name:        "discord",
				ChannelType: webhooks.ChannelTypeDiscord,
				Config:      `{"url":"https://discord.com/api/webhooks/test"}`,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 16)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobCount := 5
	for i := range jobCount {
		jobChan <- DeliveryJob{
			RuleID:       "r1",
			ChannelID:    "ch-1",
			UserID:       "u1",
			EventType:    "ALERT_EVENT_TYPE_CRASH",
			EventData:    "{}",
			ServerID:     "srv-1",
			ServerNodeID: "node-" + string(rune('a'+i)),
		}
	}

	waitFor(t, 5*time.Second, func() bool {
		return len(whSender.getCalls()) == jobCount
	})

	if len(whSender.getCalls()) != jobCount {
		t.Errorf("webhook calls = %d, want %d", len(whSender.getCalls()), jobCount)
	}

	waitFor(t, 5*time.Second, func() bool {
		records := historyStore.allRecords()
		sent := 0
		for _, rec := range records {
			if rec.DeliveryStatus == deliveryStatusSent {
				sent++
			}
		}
		return sent == jobCount
	})
}

func TestDeliveryWorker_BuildsAlertEventFields(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-1": {
				ID:          "ch-1",
				UserID:      "u1",
				Name:        "generic-wh",
				ChannelType: webhooks.ChannelTypeGeneric,
				Config:      `{"url":"https://example.com/hook"}`,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	eventDataJSON, errMarshal := json.Marshal(eventData{CurrentValue: 95.5, Threshold: 90.0})
	if errMarshal != nil {
		t.Fatalf("failed to marshal event data: %v", errMarshal)
	}

	jobChan <- DeliveryJob{
		RuleID:       "r1",
		ChannelID:    "ch-1",
		UserID:       "u1",
		EventType:    "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		EventData:    string(eventDataJSON),
		ServerID:     "srv-1",
		ServerNodeID: "node-a",
	}

	waitFor(t, 5*time.Second, func() bool {
		return len(whSender.getCalls()) == 1
	})

	ev := whSender.getCalls()[0].event
	if ev.EventType != "ALERT_EVENT_TYPE_CPU_THRESHOLD" {
		t.Errorf("event.EventType = %q, want %q", ev.EventType, "ALERT_EVENT_TYPE_CPU_THRESHOLD")
	}
	if ev.ServerID != "srv-1" {
		t.Errorf("event.ServerID = %q, want %q", ev.ServerID, "srv-1")
	}
	if ev.ServerNodeID != "node-a" {
		t.Errorf("event.ServerNodeID = %q, want %q", ev.ServerNodeID, "node-a")
	}
	if ev.Severity != webhooks.SeverityWarning {
		t.Errorf("event.Severity = %v, want %v", ev.Severity, webhooks.SeverityWarning)
	}
	if ev.Message == "" {
		t.Error("event.Message is empty, want non-empty")
	}
	if ev.Timestamp.IsZero() {
		t.Error("event.Timestamp is zero, want non-zero")
	}
}

func TestDeliveryWorker_CrashEventSeverity(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-1": {
				ID:          "ch-1",
				UserID:      "u1",
				Name:        "generic",
				ChannelType: webhooks.ChannelTypeGeneric,
				Config:      `{"url":"https://example.com/hook"}`,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobChan <- DeliveryJob{
		RuleID:       "r1",
		ChannelID:    "ch-1",
		UserID:       "u1",
		EventType:    "ALERT_EVENT_TYPE_CRASH",
		EventData:    `{"exit_code":137}`,
		ServerID:     "srv-1",
		ServerNodeID: "node-a",
	}

	waitFor(t, 5*time.Second, func() bool {
		return len(whSender.getCalls()) == 1
	})

	ev := whSender.getCalls()[0].event
	if ev.Severity != webhooks.SeverityCritical {
		t.Errorf("crash event severity = %v, want %v", ev.Severity, webhooks.SeverityCritical)
	}
}

func TestDeliveryWorker_StatusChangeEventSeverity(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-1": {
				ID:          "ch-1",
				UserID:      "u1",
				Name:        "generic",
				ChannelType: webhooks.ChannelTypeGeneric,
				Config:      `{"url":"https://example.com/hook"}`,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobChan <- DeliveryJob{
		RuleID:    "r1",
		ChannelID: "ch-1",
		UserID:    "u1",
		EventType: "ALERT_EVENT_TYPE_STATUS_CHANGE",
		EventData: `{"new_status":"offline"}`,
		ServerID:  "srv-1",
	}

	waitFor(t, 5*time.Second, func() bool {
		return len(whSender.getCalls()) == 1
	})

	ev := whSender.getCalls()[0].event
	if ev.Severity != webhooks.SeverityInfo {
		t.Errorf("status change event severity = %v, want %v", ev.Severity, webhooks.SeverityInfo)
	}
}

func TestSeverityForEventType(t *testing.T) {
	tests := []struct {
		eventType string
		want      webhooks.Severity
	}{
		{"ALERT_EVENT_TYPE_CRASH", webhooks.SeverityCritical},
		{"ALERT_EVENT_TYPE_STATUS_CHANGE", webhooks.SeverityInfo},
		{"ALERT_EVENT_TYPE_CPU_THRESHOLD", webhooks.SeverityWarning},
		{"ALERT_EVENT_TYPE_MEMORY_THRESHOLD", webhooks.SeverityWarning},
		{"ALERT_EVENT_TYPE_DISK_THRESHOLD", webhooks.SeverityWarning},
		{"ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD", webhooks.SeverityWarning},
		{"ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", webhooks.SeverityWarning},
		{"ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD", webhooks.SeverityWarning},
		{"ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD", webhooks.SeverityWarning},
		{"ALERT_EVENT_TYPE_UNKNOWN", webhooks.SeverityInfo},
	}

	for _, tc := range tests {
		t.Run(tc.eventType, func(t *testing.T) {
			got := severityForEventType(tc.eventType)
			if got != tc.want {
				t.Errorf("severityForEventType(%q) = %v, want %v", tc.eventType, got, tc.want)
			}
		})
	}
}

func TestParseWebhookConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantURL string
		wantErr bool
	}{
		{
			name:    "valid config",
			input:   `{"url":"https://example.com/hook"}`,
			wantURL: "https://example.com/hook",
		},
		{
			name:    "empty JSON object",
			input:   `{}`,
			wantURL: "",
		},
		{
			name:    "invalid JSON",
			input:   `{not valid`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cfg webhooks.ChannelConfig
			errParse := json.Unmarshal([]byte(tc.input), &cfg)
			if tc.wantErr {
				if errParse == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if errParse != nil {
				t.Fatalf("unexpected error: %v", errParse)
			}
			if cfg.URL != tc.wantURL {
				t.Errorf("URL = %q, want %q", cfg.URL, tc.wantURL)
			}
		})
	}
}

func TestParseEmailConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTo   string
		wantHost string
		wantErr  bool
	}{
		{
			name:     "valid email config",
			input:    `{"to":"admin@example.com","smtp_host":"mail.example.com","smtp_port":587,"smtp_user":"user","smtp_password":"pass","smtp_from":"noreply@example.com","smtp_tls_enabled":true}`,
			wantTo:   "admin@example.com",
			wantHost: "mail.example.com",
		},
		{
			name:    "invalid JSON",
			input:   `{broken`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, errParse := ParseEmailChannelConfig(tc.input)
			if tc.wantErr {
				if errParse == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if errParse != nil {
				t.Fatalf("unexpected error: %v", errParse)
			}
			if cfg.To != tc.wantTo {
				t.Errorf("To = %q, want %q", cfg.To, tc.wantTo)
			}
			if cfg.SMTPHost != tc.wantHost {
				t.Errorf("SMTPHost = %q, want %q", cfg.SMTPHost, tc.wantHost)
			}
		})
	}
}

func TestDeliveryWorker_HistoryInsertError(t *testing.T) {
	channelStore := &mockChannelStore{
		channels: map[string]*models.NotificationChannel{
			"ch-1": {
				ID:          "ch-1",
				UserID:      "u1",
				Name:        "discord",
				ChannelType: webhooks.ChannelTypeDiscord,
				Config:      `{"url":"https://discord.com/api/webhooks/test"}`,
				Enabled:     1,
			},
		},
	}
	historyStore := newMockHistoryStore()
	historyStore.insertErr = errors.New("db insert failed")
	whSender := &mockWebhookSender{}
	emailSender := &mockEmailSender{}

	jobChan := make(chan DeliveryJob, 8)
	pool := NewDeliveryPool(channelStore, historyStore, whSender, emailSender, jobChan)

	ctx := t.Context()

	pool.Start(ctx)

	jobChan <- DeliveryJob{
		RuleID:    "r1",
		ChannelID: "ch-1",
		UserID:    "u1",
		EventType: "ALERT_EVENT_TYPE_CRASH",
		EventData: "{}",
		ServerID:  "srv-1",
	}

	// Worker should handle the error gracefully without panic. Give it time
	// to process and ensure no webhook call was made (since history insert
	// fails, the worker should log and skip delivery).
	time.Sleep(200 * time.Millisecond)

	// The webhook sender should NOT have been called since we can't track status.
	if len(whSender.getCalls()) != 0 {
		t.Errorf("webhook calls = %d, want 0 (should not deliver when history insert fails)", len(whSender.getCalls()))
	}
}

func TestBuildFieldsFromEventData(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
	}{
		{
			name:    "threshold event",
			input:   `{"current_value":95.5,"threshold":90.0}`,
			wantLen: 2,
		},
		{
			name:    "status change event",
			input:   `{"new_status":"offline","old_status":"online"}`,
			wantLen: 2,
		},
		{
			name:    "crash event",
			input:   `{"exit_code":137}`,
			wantLen: 1,
		},
		{
			name:    "empty data",
			input:   `{}`,
			wantLen: 0,
		},
		{
			name:    "invalid JSON returns nil",
			input:   `{bad`,
			wantLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := buildFieldsFromEventData(tc.input)
			if len(fields) != tc.wantLen {
				t.Errorf("buildFieldsFromEventData() returned %d fields, want %d; fields: %v", len(fields), tc.wantLen, fields)
			}
		})
	}
}

func TestBuildFieldsFromEventData_PreservesExplicitZeroValues(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFields map[string]string
	}{
		{
			name:  "crash exit code zero",
			input: `{"exit_code":0}`,
			wantFields: map[string]string{
				"exit_code": "0",
			},
		},
		{
			name:  "threshold zero values",
			input: `{"current_value":0,"threshold":0,"direction":"entered"}`,
			wantFields: map[string]string{
				"current_value": "0",
				"threshold":     "0",
				"direction":     "entered",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := buildFieldsFromEventData(tc.input)
			if !reflect.DeepEqual(fields, tc.wantFields) {
				t.Fatalf("buildFieldsFromEventData() = %#v, want %#v", fields, tc.wantFields)
			}
		})
	}
}
