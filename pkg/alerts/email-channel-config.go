package alerts

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"

	"github.com/ClintonCollins/Xylona/pkg/mailer"
)

const (
	SMTPSourceNode   = "node"
	SMTPSourceCustom = "custom"
)

type EmailChannelConfig struct {
	To                    string `json:"to"`
	SMTPSource            string `json:"smtp_source"`
	SMTPHost              string `json:"smtp_host,omitempty"`
	SMTPPort              int    `json:"smtp_port,omitempty"`
	SMTPUser              string `json:"smtp_user,omitempty"`
	SMTPPassword          string `json:"smtp_password"`
	SMTPPasswordConfigured bool   `json:"smtp_password_configured,omitempty"`
	SMTPFrom              string `json:"smtp_from,omitempty"`
	SMTPTLSEnabled        bool   `json:"smtp_tls_enabled,omitempty"`
}

func ParseEmailChannelConfig(raw string) (EmailChannelConfig, error) {
	var config EmailChannelConfig
	errUnmarshal := json.Unmarshal([]byte(raw), &config)
	if errUnmarshal != nil {
		return EmailChannelConfig{}, fmt.Errorf("config must be valid JSON")
	}

	config.To = strings.TrimSpace(config.To)
	config.SMTPSource = strings.TrimSpace(strings.ToLower(config.SMTPSource))
	config.SMTPHost = strings.TrimSpace(config.SMTPHost)
	config.SMTPUser = strings.TrimSpace(config.SMTPUser)
	config.SMTPFrom = strings.TrimSpace(config.SMTPFrom)

	return config, nil
}

func (c EmailChannelConfig) Validate(requirePassword bool) error {
	if c.To == "" {
		return fmt.Errorf("email notification channels require a recipient address")
	}

	_, errParseTo := mail.ParseAddress(c.To)
	if errParseTo != nil {
		return fmt.Errorf("email notification channels require a valid recipient address")
	}

	switch c.SMTPSource {
	case SMTPSourceNode:
		return nil
	case SMTPSourceCustom:
		if c.SMTPHost == "" {
			return fmt.Errorf("custom smtp requires smtp_host")
		}
		if c.SMTPPort < 1 || c.SMTPPort > 65535 {
			return fmt.Errorf("custom smtp requires a valid smtp_port")
		}
		if c.SMTPUser == "" {
			return fmt.Errorf("custom smtp requires smtp_user")
		}
		if requirePassword && c.SMTPPassword == "" {
			return fmt.Errorf("custom smtp requires smtp_password")
		}
		if c.SMTPFrom == "" {
			return fmt.Errorf("custom smtp requires smtp_from")
		}
		_, errParseFrom := mail.ParseAddress(c.SMTPFrom)
		if errParseFrom != nil {
			return fmt.Errorf("custom smtp requires a valid smtp_from address")
		}
		return nil
	default:
		return fmt.Errorf("email notification channels require smtp_source to be \"node\" or \"custom\"")
	}
}

func (c EmailChannelConfig) Sanitized() EmailChannelConfig {
	c.SMTPPasswordConfigured = c.SMTPPassword != ""
	c.SMTPPassword = ""
	return c
}

func (c EmailChannelConfig) Marshal() (string, error) {
	jsonBytes, errMarshal := json.Marshal(c)
	if errMarshal != nil {
		return "", errMarshal
	}
	return string(jsonBytes), nil
}

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
