package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/webhooks"
)

// ErrNoSMTPConfig is returned when no SMTP configuration is available.
var ErrNoSMTPConfig = errors.New("mailer: no SMTP configuration available")

// SMTPConfig holds the SMTP server connection details.
type SMTPConfig struct {
	Host       string
	Port       int
	User       string
	Password   string
	From       string
	TLSEnabled bool
}

// Addr returns the host:port address string for the SMTP server.
func (c *SMTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// retryConfig controls retry behavior for email delivery.
type retryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

// defaultRetryConfig is the production retry configuration.
var defaultRetryConfig = retryConfig{
	MaxAttempts: 3,
	BaseDelay:   time.Second,
}

// dialTimeout is the connection timeout for SMTP dial operations.
const dialTimeout = 10 * time.Second

// sendFunc is the function signature for sending an email via SMTP.
// This allows injection of test doubles.
type sendFunc func(ctx context.Context, config *SMTPConfig, to string, subject string, body string) error

// Mailer delivers email notifications using SMTP.
type Mailer struct {
	systemConfig *SMTPConfig
	retry        retryConfig
	sendFunc     sendFunc
}

// New creates a Mailer with the given system-level SMTP configuration.
// The system config is used as a fallback when no per-channel config is provided.
func New(systemConfig *SMTPConfig) *Mailer {
	return &Mailer{
		systemConfig: systemConfig,
		retry:        defaultRetryConfig,
		sendFunc:     sendSMTP,
	}
}

// Send delivers an email notification for the given alert event.
// If perChannelConfig is non-nil and has a non-empty Host, it takes precedence
// over the system SMTP config.
func (m *Mailer) Send(ctx context.Context, to string, event webhooks.AlertEvent, perChannelConfig *SMTPConfig) error {
	config := resolveConfig(m.systemConfig, perChannelConfig)
	if config == nil {
		return ErrNoSMTPConfig
	}

	subject := FormatSubject(event)
	body := FormatBody(event)

	var lastErr error
	for attempt := range m.retry.MaxAttempts {
		if attempt > 0 {
			delay := m.retry.BaseDelay * (1 << (attempt - 1))
			select {
			case <-ctx.Done():
				return fmt.Errorf("mailer: context cancelled during retry: %w", ctx.Err())
			case <-time.After(delay):
			}
		}

		errSend := m.sendFunc(ctx, config, to, subject, body)
		if errSend == nil {
			return nil
		}

		lastErr = errSend

		if attempt+1 < m.retry.MaxAttempts {
			log.Warn().
				Err(errSend).
				Str("to", to).
				Str("host", config.Host).
				Int("attempt", attempt+1).
				Int("max_attempts", m.retry.MaxAttempts).
				Msg("Email delivery attempt failed, retrying")
		} else {
			log.Warn().
				Err(errSend).
				Str("to", to).
				Str("host", config.Host).
				Int("attempt", attempt+1).
				Int("max_attempts", m.retry.MaxAttempts).
				Msg("Email delivery failed after all attempts")
		}
	}

	return lastErr
}

// FormatSubject returns a formatted email subject line for the given alert event.
// Format: "Event Title" or "Event Title — Server Name" if a server name is present.
func FormatSubject(event webhooks.AlertEvent) string {
	title := webhooks.EventTypeTitle(event.EventType)
	if event.ServerName == "" {
		return title
	}
	return title + " \u2014 " + event.ServerName
}

// FormatBody returns a formatted plain-text email body for the given alert event.
func FormatBody(event webhooks.AlertEvent) string {
	var b strings.Builder

	title := webhooks.EventTypeTitle(event.EventType)
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", len(title)))
	b.WriteString("\n\n")

	b.WriteString(event.Message)
	b.WriteString("\n\n")

	b.WriteString("Severity: ")
	b.WriteString(event.Severity.String())
	b.WriteString("\n")

	if event.ServerName != "" {
		b.WriteString("Server: ")
		b.WriteString(event.ServerName)
		b.WriteString("\n")
	}

	if event.NodeID != "" {
		b.WriteString("Node: ")
		b.WriteString(event.NodeID)
		b.WriteString("\n")
	}

	b.WriteString("Time: ")
	b.WriteString(event.Timestamp.UTC().Format("2006-01-02 15:04:05 UTC"))
	b.WriteString("\n")

	if len(event.Fields) > 0 {
		b.WriteString("\nDetails:\n")

		keys := make([]string, 0, len(event.Fields))
		for k := range event.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			b.WriteString("  ")
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(event.Fields[k])
			b.WriteString("\n")
		}
	}

	return b.String()
}

// buildMessage constructs a raw RFC 2822 email message string.
func buildMessage(from string, to string, subject string, body string) string {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(to)
	b.WriteString("\r\n")
	b.WriteString("Date: ")
	b.WriteString(time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(subject)
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

// resolveConfig returns the effective SMTP config to use. Per-channel config
// takes precedence if it has a non-empty Host. Returns nil if no valid config
// is available.
func resolveConfig(systemConfig *SMTPConfig, perChannelConfig *SMTPConfig) *SMTPConfig {
	if perChannelConfig != nil && perChannelConfig.Host != "" {
		return perChannelConfig
	}
	return systemConfig
}

// sendSMTP is the production implementation that sends an email via SMTP.
func sendSMTP(ctx context.Context, config *SMTPConfig, to string, subject string, body string) error {
	msg := buildMessage(config.From, to, subject, body)
	addr := config.Addr()

	if config.TLSEnabled {
		return sendWithTLS(ctx, config, to, msg, addr)
	}

	return sendPlain(ctx, config, to, msg, addr)
}

// sendWithTLS sends an email using an explicit TLS connection (SMTPS on port 465)
// or STARTTLS on other ports.
func sendWithTLS(ctx context.Context, config *SMTPConfig, to string, msg string, addr string) error {
	tlsConfig := &tls.Config{
		ServerName: config.Host,
		MinVersion: tls.VersionTLS12,
	}

	// Port 465 uses implicit TLS (SMTPS).
	if config.Port == 465 {
		return sendImplicitTLS(ctx, config, to, msg, addr, tlsConfig)
	}

	// Other ports (typically 587) use STARTTLS.
	return sendSTARTTLS(ctx, config, to, msg, addr, tlsConfig)
}

// sendImplicitTLS connects with TLS from the start (port 465 / SMTPS).
func sendImplicitTLS(ctx context.Context, config *SMTPConfig, to string, msg string, addr string, tlsConfig *tls.Config) error {
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: dialTimeout},
		Config:    tlsConfig,
	}
	conn, errDial := dialer.DialContext(ctx, "tcp", addr)
	if errDial != nil {
		return fmt.Errorf("mailer: TLS dial to %s failed: %w", addr, errDial)
	}

	client, errClient := smtp.NewClient(conn, config.Host)
	if errClient != nil {
		errClose := conn.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close TLS connection after client creation failure")
		}
		return fmt.Errorf("mailer: SMTP client creation failed: %w", errClient)
	}

	return completeDelivery(client, config, to, msg)
}

// sendSTARTTLS connects in plain text then upgrades to TLS via STARTTLS.
func sendSTARTTLS(ctx context.Context, config *SMTPConfig, to string, msg string, addr string, tlsConfig *tls.Config) error {
	netDialer := &net.Dialer{Timeout: dialTimeout}
	conn, errDial := netDialer.DialContext(ctx, "tcp", addr)
	if errDial != nil {
		return fmt.Errorf("mailer: dial to %s failed: %w", addr, errDial)
	}

	client, errClient := smtp.NewClient(conn, config.Host)
	if errClient != nil {
		errClose := conn.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close connection after SMTP client creation failure")
		}
		return fmt.Errorf("mailer: SMTP client creation failed: %w", errClient)
	}

	errStartTLS := client.StartTLS(tlsConfig)
	if errStartTLS != nil {
		errClose := client.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close SMTP client after STARTTLS failure")
		}
		return fmt.Errorf("mailer: STARTTLS failed: %w", errStartTLS)
	}

	return completeDelivery(client, config, to, msg)
}

// sendPlain sends an email without any TLS.
func sendPlain(ctx context.Context, config *SMTPConfig, to string, msg string, addr string) error {
	netDialer := &net.Dialer{Timeout: dialTimeout}
	conn, errDial := netDialer.DialContext(ctx, "tcp", addr)
	if errDial != nil {
		return fmt.Errorf("mailer: dial to %s failed: %w", addr, errDial)
	}

	client, errClient := smtp.NewClient(conn, config.Host)
	if errClient != nil {
		errClose := conn.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close connection after SMTP client creation failure")
		}
		return fmt.Errorf("mailer: SMTP client creation failed: %w", errClient)
	}

	return completeDelivery(client, config, to, msg)
}

// completeDelivery performs authentication (if configured) and sends the email
// using the given SMTP client.
func completeDelivery(client *smtp.Client, config *SMTPConfig, to string, msg string) error {
	defer func() {
		errQuit := client.Quit()
		if errQuit != nil {
			log.Warn().Err(errQuit).Msg("Failed to quit SMTP client")
		}
	}()

	if config.User != "" && config.Password != "" {
		auth := smtp.PlainAuth("", config.User, config.Password, config.Host)
		errAuth := client.Auth(auth)
		if errAuth != nil {
			return fmt.Errorf("mailer: authentication failed: %w", errAuth)
		}
	}

	errFrom := client.Mail(config.From)
	if errFrom != nil {
		return fmt.Errorf("mailer: MAIL FROM failed: %w", errFrom)
	}

	errRcpt := client.Rcpt(to)
	if errRcpt != nil {
		return fmt.Errorf("mailer: RCPT TO failed: %w", errRcpt)
	}

	writer, errData := client.Data()
	if errData != nil {
		return fmt.Errorf("mailer: DATA command failed: %w", errData)
	}

	_, errWrite := writer.Write([]byte(msg))
	if errWrite != nil {
		return fmt.Errorf("mailer: failed to write message body: %w", errWrite)
	}

	errCloseWriter := writer.Close()
	if errCloseWriter != nil {
		return fmt.Errorf("mailer: failed to close message writer: %w", errCloseWriter)
	}

	return nil
}
