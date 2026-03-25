package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/mailer"
	"github.com/ClintonCollins/Xylona/pkg/webhooks"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// defaultWorkerCount is the number of goroutines in the delivery pool.
const defaultWorkerCount = 3

const shutdownDrainJobTimeout = 5 * time.Second

// Channel type constant for email notifications.
const ChannelTypeEmail = "NOTIFICATION_CHANNEL_TYPE_EMAIL"

// Delivery status constants matching proto enum string values.
const (
	deliveryStatusPending = "DELIVERY_STATUS_PENDING"
	deliveryStatusSent    = "DELIVERY_STATUS_SENT"
	deliveryStatusFailed  = "DELIVERY_STATUS_FAILED"
)

// ChannelStore abstracts the DB method for resolving notification channels.
type ChannelStore interface {
	GetNotificationChannelByID(id string) (*models.NotificationChannel, error)
}

// HistoryStore abstracts the DB methods for recording alert delivery history.
type HistoryStore interface {
	InsertAlertHistory(ruleID, userID, serverID, serverNodeID, nodeID, eventType, eventData, channelType, deliveryStatus string) (*models.AlertHistory, error)
	UpdateAlertHistoryDeliveryStatus(id, status, deliveryError string) error
}

// WebhookSender abstracts the webhook delivery API.
type WebhookSender interface {
	Send(ctx context.Context, channelType string, config webhooks.ChannelConfig, event webhooks.AlertEvent) error
}

// EmailSender abstracts the email delivery API.
type EmailSender interface {
	Send(ctx context.Context, to string, event webhooks.AlertEvent, perChannelConfig *mailer.SMTPConfig) error
}

// emailChannelConfig represents the JSON config stored for email notification
// channels. It includes the recipient address and per-channel SMTP settings.
type emailChannelConfig struct {
	To             string `json:"to"`
	SMTPHost       string `json:"smtp_host"`
	SMTPPort       int    `json:"smtp_port"`
	SMTPUser       string `json:"smtp_user"`
	SMTPPassword   string `json:"smtp_password"`
	SMTPFrom       string `json:"smtp_from"`
	SMTPTLSEnabled bool   `json:"smtp_tls_enabled"`
}

type deliveryEventData struct {
	CurrentValue *float64 `json:"current_value"`
	NewStatus    *string  `json:"new_status,omitempty"`
	OldStatus    *string  `json:"old_status,omitempty"`
	ExitCode     *int     `json:"exit_code,omitempty"`
	Threshold    *float64 `json:"threshold"`
	Direction    *string  `json:"direction,omitempty"`
}

// DeliveryPool manages a pool of goroutines that consume DeliveryJob values
// and dispatch notifications through the appropriate channel.
type DeliveryPool struct {
	channelStore ChannelStore
	historyStore HistoryStore
	webhookSend  WebhookSender
	emailSend    EmailSender
	jobChan      <-chan DeliveryJob
	workerCount  int
	wg           sync.WaitGroup
}

// NewDeliveryPool creates a DeliveryPool with the default worker count.
func NewDeliveryPool(
	channelStore ChannelStore,
	historyStore HistoryStore,
	webhookSend WebhookSender,
	emailSend EmailSender,
	jobChan <-chan DeliveryJob,
) *DeliveryPool {
	return &DeliveryPool{
		channelStore: channelStore,
		historyStore: historyStore,
		webhookSend:  webhookSend,
		emailSend:    emailSend,
		jobChan:      jobChan,
		workerCount:  defaultWorkerCount,
	}
}

// Start spawns the worker goroutines. Each goroutine runs until the context is
// cancelled or the job channel is closed. Call Wait to block until all workers
// have exited.
func (p *DeliveryPool) Start(ctx context.Context) {
	for i := range p.workerCount {
		p.wg.Go(func() {
			p.worker(ctx, i)
		})
	}
	log.Info().Int("workers", p.workerCount).Msg("Alert delivery pool started")
}

// Wait blocks until all worker goroutines have exited.
func (p *DeliveryPool) Wait() {
	p.wg.Wait()
}

// worker is the main loop for a single delivery goroutine.
func (p *DeliveryPool) worker(ctx context.Context, id int) {
	logger := log.With().Int("worker_id", id).Logger()
	logger.Debug().Msg("Delivery worker started")

	for {
		if ctx.Err() != nil {
			p.drainPendingJobs(logger)
			logger.Debug().Msg("Delivery worker stopping (context cancelled)")
			return
		}

		select {
		case <-ctx.Done():
			p.drainPendingJobs(logger)
			logger.Debug().Msg("Delivery worker stopping (context cancelled)")
			return
		case job, ok := <-p.jobChan:
			if !ok {
				logger.Debug().Msg("Delivery worker stopping (channel closed)")
				return
			}
			if ctx.Err() != nil {
				p.processDrainedJob(job)
				p.drainPendingJobs(logger)
				logger.Debug().Msg("Delivery worker stopping (context cancelled)")
				return
			}
			p.processJob(ctx, job)
		}
	}
}

func (p *DeliveryPool) drainPendingJobs(logger zerolog.Logger) {
	for {
		select {
		case job, ok := <-p.jobChan:
			if !ok {
				return
			}
			p.processDrainedJob(job)
		default:
			logger.Debug().Msg("Delivery worker drained buffered jobs after cancellation")
			return
		}
	}
}

func (p *DeliveryPool) processDrainedJob(job DeliveryJob) {
	drainCtx, cancel := context.WithTimeout(context.Background(), shutdownDrainJobTimeout)
	defer cancel()
	p.processJob(drainCtx, job)
}

// processJob handles a single delivery job: resolves the channel, creates the
// history record, dispatches the notification, and updates the status.
func (p *DeliveryPool) processJob(ctx context.Context, job DeliveryJob) {
	logger := log.With().
		Str("rule_id", job.RuleID).
		Str("channel_id", job.ChannelID).
		Str("event_type", job.EventType).
		Logger()

	// Step 1: Resolve the notification channel from the DB so we know the
	// channel_type for the history record.
	channel, errChannel := p.channelStore.GetNotificationChannelByID(job.ChannelID)
	if errChannel != nil {
		logger.Error().Err(errChannel).Msg("Failed to resolve notification channel")
		// Record a failed history entry with an empty channel type since we
		// couldn't resolve it.
		p.insertFailedHistory(job, "", fmt.Sprintf("channel lookup failed: %v", errChannel))
		return
	}

	// Step 2: Create a pending alert history record with the resolved
	// channel type.
	history, errInsert := p.historyStore.InsertAlertHistory(
		job.RuleID,
		job.UserID,
		job.ServerID,
		job.ServerNodeID,
		job.NodeID,
		job.EventType,
		job.EventData,
		channel.ChannelType,
		deliveryStatusPending,
	)
	if errInsert != nil {
		logger.Error().Err(errInsert).Msg("Failed to insert alert history, skipping delivery")
		return
	}

	// Step 3: Dispatch to the correct sender.
	alertEvent := buildAlertEvent(job)
	errDeliver := p.dispatch(ctx, channel, alertEvent)

	// Step 4: Update history with result.
	if errDeliver != nil {
		logger.Warn().Err(errDeliver).Str("channel_type", channel.ChannelType).Msg("Delivery failed")
		p.failHistory(history.ID, errDeliver.Error())
		return
	}

	errUpdate := p.historyStore.UpdateAlertHistoryDeliveryStatus(history.ID, deliveryStatusSent, "")
	if errUpdate != nil {
		logger.Error().Err(errUpdate).Msg("Failed to update alert history to sent")
	}
}

// dispatch routes the alert event to the appropriate sender based on the
// channel type.
func (p *DeliveryPool) dispatch(ctx context.Context, channel *models.NotificationChannel, event webhooks.AlertEvent) error {
	switch channel.ChannelType {
	case webhooks.ChannelTypeDiscord, webhooks.ChannelTypeSlack, webhooks.ChannelTypeGeneric:
		return p.dispatchWebhook(ctx, channel, event)
	case ChannelTypeEmail:
		return p.dispatchEmail(ctx, channel, event)
	default:
		return fmt.Errorf("unsupported channel type: %s", channel.ChannelType)
	}
}

// dispatchWebhook parses the webhook config and sends via the webhook sender.
func (p *DeliveryPool) dispatchWebhook(ctx context.Context, channel *models.NotificationChannel, event webhooks.AlertEvent) error {
	var config webhooks.ChannelConfig
	errParse := json.Unmarshal([]byte(channel.Config), &config)
	if errParse != nil {
		return fmt.Errorf("failed to parse webhook config: %w", errParse)
	}

	return p.webhookSend.Send(ctx, channel.ChannelType, config, event)
}

// dispatchEmail parses the email config and sends via the email sender.
func (p *DeliveryPool) dispatchEmail(ctx context.Context, channel *models.NotificationChannel, event webhooks.AlertEvent) error {
	var config emailChannelConfig
	errParse := json.Unmarshal([]byte(channel.Config), &config)
	if errParse != nil {
		return fmt.Errorf("failed to parse email config: %w", errParse)
	}

	var smtpConfig *mailer.SMTPConfig
	if config.SMTPHost != "" {
		smtpConfig = &mailer.SMTPConfig{
			Host:       config.SMTPHost,
			Port:       config.SMTPPort,
			User:       config.SMTPUser,
			Password:   config.SMTPPassword,
			From:       config.SMTPFrom,
			TLSEnabled: config.SMTPTLSEnabled,
		}
	}

	return p.emailSend.Send(ctx, config.To, event, smtpConfig)
}

// insertFailedHistory creates a new alert history record with a failed status
// when the delivery cannot proceed (e.g., channel lookup failure). This is used
// when we don't yet have a history record to update.
func (p *DeliveryPool) insertFailedHistory(job DeliveryJob, channelType string, deliveryError string) {
	history, errInsert := p.historyStore.InsertAlertHistory(
		job.RuleID,
		job.UserID,
		job.ServerID,
		job.ServerNodeID,
		job.NodeID,
		job.EventType,
		job.EventData,
		channelType,
		deliveryStatusFailed,
	)
	if errInsert != nil {
		log.Error().Err(errInsert).Str("rule_id", job.RuleID).Msg("Failed to insert failed alert history")
		return
	}
	// Also store the error message via the update method.
	p.failHistory(history.ID, deliveryError)
}

// failHistory updates the alert history record with a failed status and the
// given error message.
func (p *DeliveryPool) failHistory(historyID string, deliveryError string) {
	errUpdate := p.historyStore.UpdateAlertHistoryDeliveryStatus(historyID, deliveryStatusFailed, deliveryError)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("alert_history_id", historyID).Msg("Failed to update alert history to failed")
	}
}

// buildAlertEvent constructs a webhooks.AlertEvent from a DeliveryJob.
func buildAlertEvent(job DeliveryJob) webhooks.AlertEvent {
	return webhooks.AlertEvent{
		EventType:    job.EventType,
		ServerID:     job.ServerID,
		ServerNodeID: job.ServerNodeID,
		NodeID:       job.NodeID,
		Message:      webhooks.EventTypeTitle(job.EventType),
		Severity:     severityForEventType(job.EventType),
		Timestamp:    time.Now(),
		Fields:       buildFieldsFromEventData(job.EventData),
	}
}

// severityForEventType maps event type strings to their appropriate severity
// level.
func severityForEventType(eventType string) webhooks.Severity {
	switch eventType {
	case "ALERT_EVENT_TYPE_CRASH":
		return webhooks.SeverityCritical
	case "ALERT_EVENT_TYPE_STATUS_CHANGE":
		return webhooks.SeverityInfo
	case "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		"ALERT_EVENT_TYPE_MEMORY_THRESHOLD",
		"ALERT_EVENT_TYPE_DISK_THRESHOLD",
		"ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD",
		"ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
		"ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD",
		"ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD":
		return webhooks.SeverityWarning
	default:
		return webhooks.SeverityInfo
	}
}

// buildFieldsFromEventData parses the JSON event data and returns a
// human-readable key-value map of fields suitable for inclusion in the
// AlertEvent.
func buildFieldsFromEventData(eventDataJSON string) map[string]string {
	var data deliveryEventData
	errUnmarshal := json.Unmarshal([]byte(eventDataJSON), &data)
	if errUnmarshal != nil {
		return nil
	}

	fields := make(map[string]string)
	if data.CurrentValue != nil {
		fields["current_value"] = strconv.FormatFloat(*data.CurrentValue, 'f', -1, 64)
	}
	if data.Threshold != nil {
		fields["threshold"] = strconv.FormatFloat(*data.Threshold, 'f', -1, 64)
	}
	if data.NewStatus != nil && *data.NewStatus != "" {
		fields["new_status"] = *data.NewStatus
	}
	if data.OldStatus != nil && *data.OldStatus != "" {
		fields["old_status"] = *data.OldStatus
	}
	if data.ExitCode != nil {
		fields["exit_code"] = strconv.Itoa(*data.ExitCode)
	}
	if data.Direction != nil && *data.Direction != "" {
		fields["direction"] = *data.Direction
	}

	if len(fields) == 0 {
		return nil
	}

	return fields
}
