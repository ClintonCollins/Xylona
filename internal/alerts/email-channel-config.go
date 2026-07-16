package alerts

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/mailer"
)

const (
	// SMTPSourceController uses the controller-level email configuration.
	SMTPSourceController = "controller"
	// SMTPSourceCustom uses SMTP settings stored on the notification channel itself.
	SMTPSourceCustom = "custom"
	// legacySMTPSourceNode is accepted so existing notification channels keep
	// working after the hub-spoke terminology change.
	legacySMTPSourceNode = "node"
)

// EmailChannelConfig stores email channel delivery settings.
type EmailChannelConfig struct {
	To                     string `json:"to"`
	SMTPSource             string `json:"smtp_source"`
	SMTPHost               string `json:"smtp_host,omitempty"`
	SMTPPort               int    `json:"smtp_port,omitempty"`
	SMTPUser               string `json:"smtp_user,omitempty"`
	SMTPPassword           string `json:"smtp_password"`
	SMTPPasswordConfigured bool   `json:"smtp_password_configured,omitempty"`
	SMTPFrom               string `json:"smtp_from,omitempty"`
	SMTPTLSEnabled         bool   `json:"smtp_tls_enabled,omitempty"`
}

// ParseEmailChannelConfig parses JSON email channel configuration.
func ParseEmailChannelConfig(raw string) (EmailChannelConfig, error) {
	var config EmailChannelConfig
	errUnmarshal := json.Unmarshal([]byte(raw), &config)
	if errUnmarshal != nil {
		return EmailChannelConfig{}, errors.New("config must be valid JSON")
	}

	config.To = strings.TrimSpace(config.To)
	config.SMTPSource = strings.TrimSpace(strings.ToLower(config.SMTPSource))
	if config.SMTPSource == legacySMTPSourceNode {
		config.SMTPSource = SMTPSourceController
	}
	config.SMTPHost = strings.TrimSpace(config.SMTPHost)
	config.SMTPUser = strings.TrimSpace(config.SMTPUser)
	config.SMTPFrom = strings.TrimSpace(config.SMTPFrom)

	return config, nil
}

// Validate validates the email channel configuration.
func (c EmailChannelConfig) Validate(requirePassword bool) error {
	if c.To == "" {
		return errors.New("email notification channels require a recipient address")
	}

	_, errParseTo := mail.ParseAddress(c.To)
	if errParseTo != nil {
		return errors.New("email notification channels require a valid recipient address")
	}

	switch c.SMTPSource {
	case SMTPSourceController:
		return nil
	case SMTPSourceCustom:
		if c.SMTPHost == "" {
			return errors.New("custom smtp requires smtp_host")
		}
		if c.SMTPPort < 1 || c.SMTPPort > 65535 {
			return errors.New("custom smtp requires a valid smtp_port")
		}
		if c.SMTPUser == "" {
			return errors.New("custom smtp requires smtp_user")
		}
		if requirePassword && c.SMTPPassword == "" {
			return errors.New("custom smtp requires smtp_password")
		}
		if c.SMTPFrom == "" {
			return errors.New("custom smtp requires smtp_from")
		}
		_, errParseFrom := mail.ParseAddress(c.SMTPFrom)
		if errParseFrom != nil {
			return errors.New("custom smtp requires a valid smtp_from address")
		}
		return nil
	default:
		return errors.New(`email notification channels require smtp_source to be "controller" or "custom"`)
	}
}

// Sanitized returns a copy of the config with the SMTP password removed.
func (c EmailChannelConfig) Sanitized() EmailChannelConfig {
	c.SMTPPasswordConfigured = c.SMTPPassword != ""
	c.SMTPPassword = ""
	return c
}

// Marshal serializes the email channel configuration to JSON.
func (c EmailChannelConfig) Marshal() (string, error) {
	jsonBytes, errMarshal := json.Marshal(c)
	if errMarshal != nil {
		return "", fmt.Errorf("marshal email channel config: %w", errMarshal)
	}
	return string(jsonBytes), nil
}

// EffectiveSMTPConfig returns the per-channel SMTP configuration when custom SMTP is enabled.
func (c EmailChannelConfig) EffectiveSMTPConfig() *mailer.SMTPConfig {
	if c.SMTPSource != SMTPSourceCustom {
		return nil
	}

	return &mailer.SMTPConfig{
		Host:       c.SMTPHost,
		Port:       c.SMTPPort,
		User:       c.SMTPUser,
		Password:   c.SMTPPassword,
		From:       c.SMTPFrom,
		TLSEnabled: c.SMTPTLSEnabled,
	}
}
