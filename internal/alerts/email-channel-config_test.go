package alerts

import "testing"

func TestParseEmailChannelConfigSMTPSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		wantSource string
	}{
		{
			name:       "controller source",
			raw:        `{"to":"admin@example.com","smtp_source":"controller"}`,
			wantSource: SMTPSourceController,
		},
		{
			name:       "legacy node source",
			raw:        `{"to":"admin@example.com","smtp_source":"node"}`,
			wantSource: SMTPSourceController,
		},
		{
			name:       "custom source",
			raw:        `{"to":"admin@example.com","smtp_source":"custom","smtp_host":"smtp.example.com","smtp_port":587,"smtp_user":"mailer","smtp_from":"sender@example.com"}`,
			wantSource: SMTPSourceCustom,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			config, errParse := ParseEmailChannelConfig(testCase.raw)
			if errParse != nil {
				t.Fatalf("ParseEmailChannelConfig() error = %v", errParse)
			}
			if config.SMTPSource != testCase.wantSource {
				t.Errorf("SMTPSource = %q, want %q", config.SMTPSource, testCase.wantSource)
			}
			errValidate := config.Validate(false)
			if errValidate != nil {
				t.Fatalf("Validate() error = %v", errValidate)
			}
		})
	}
}
