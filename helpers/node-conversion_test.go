package helpers

import (
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestNodeProtoToModel(t *testing.T) {
	tests := []struct {
		name          string
		input         *xylona.Node
		wantName      string
		wantHost      string
		wantBaseURL   string
		wantIsLocal   bool
		wantInsecure  bool
		wantSecretKey string
		wantSecretSet bool
	}{
		{
			name: "trims editable fields and keeps remote secret",
			input: &xylona.Node{
				Id:        "remote-1",
				Name:      "  Remote Node  ",
				Host:      "  remote-host  ",
				Port:      8080,
				SecretKey: "  remote-secret  ",
				Local:     false,
				BaseUrl:   "  https://panel.example.com  ",
			},
			wantName:      "Remote Node",
			wantHost:      "remote-host",
			wantBaseURL:   "https://panel.example.com",
			wantIsLocal:   false,
			wantInsecure:  false,
			wantSecretKey: "remote-secret",
			wantSecretSet: true,
		},
		{
			name: "empty secret key maps to null",
			input: &xylona.Node{
				Id:               "local-1",
				Name:             "localhost",
				Host:             "localhost",
				Port:             8080,
				SecretKey:        "   ",
				Local:            true,
				BaseUrl:          "http://localhost:8080",
				AllowInsecureTls: true,
			},
			wantName:      "localhost",
			wantHost:      "localhost",
			wantBaseURL:   "http://localhost:8080",
			wantIsLocal:   true,
			wantInsecure:  true,
			wantSecretSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NodeProtoToModel(tt.input)

			if got.Name != tt.wantName {
				t.Errorf("NodeProtoToModel().Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Host != tt.wantHost {
				t.Errorf("NodeProtoToModel().Host = %q, want %q", got.Host, tt.wantHost)
			}
			if got.BaseURL != tt.wantBaseURL {
				t.Errorf("NodeProtoToModel().BaseURL = %q, want %q", got.BaseURL, tt.wantBaseURL)
			}
			if got.IsLocal != tt.wantIsLocal {
				t.Errorf("NodeProtoToModel().IsLocal = %v, want %v", got.IsLocal, tt.wantIsLocal)
			}
			if got.AllowInsecureTLS != tt.wantInsecure {
				t.Errorf("NodeProtoToModel().AllowInsecureTLS = %v, want %v", got.AllowInsecureTLS, tt.wantInsecure)
			}

			if tt.wantSecretSet {
				gotSecret, okSecret := got.SecretKey.Get()
				if !okSecret {
					t.Fatalf("NodeProtoToModel().SecretKey should be set")
				}
				if gotSecret != tt.wantSecretKey {
					t.Errorf("NodeProtoToModel().SecretKey = %q, want %q", gotSecret, tt.wantSecretKey)
				}
			} else if !got.SecretKey.IsNull() {
				t.Errorf("NodeProtoToModel().SecretKey should be null")
			}
		})
	}
}

func TestNodeModelToSetter(t *testing.T) {
	tests := []struct {
		name             string
		input            *models.Node
		wantBaseURL      string
		wantSecretKey    string
		wantSecretIsNull bool
		wantInsecure     bool
	}{
		{
			name: "includes base URL and secret key when set",
			input: &models.Node{
				ID:               "remote-1",
				Name:             "Remote",
				Host:             "remote-host",
				Port:             8080,
				BaseURL:          "https://panel.example.com",
				SecretKey:        null.From("remote-secret"),
				AllowInsecureTLS: true,
			},
			wantBaseURL:      "https://panel.example.com",
			wantSecretKey:    "remote-secret",
			wantSecretIsNull: false,
			wantInsecure:     true,
		},
		{
			name: "keeps null secret key for local node",
			input: &models.Node{
				ID:               "local-1",
				Name:             "localhost",
				Host:             "localhost",
				Port:             8080,
				BaseURL:          "http://localhost:8080",
				SecretKey:        null.Val[string]{},
				AllowInsecureTLS: false,
			},
			wantBaseURL:      "http://localhost:8080",
			wantSecretIsNull: true,
			wantInsecure:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setter := NodeModelToSetter(tt.input)

			gotBaseURL, okBaseURL := setter.BaseURL.Get()
			if !okBaseURL {
				t.Fatalf("NodeModelToSetter().BaseURL should be set")
			}
			if gotBaseURL != tt.wantBaseURL {
				t.Errorf("NodeModelToSetter().BaseURL = %q, want %q", gotBaseURL, tt.wantBaseURL)
			}
			gotAllowInsecureTLS, okAllowInsecureTLS := setter.AllowInsecureTLS.Get()
			if !okAllowInsecureTLS {
				t.Fatalf("NodeModelToSetter().AllowInsecureTLS should be set")
			}
			if gotAllowInsecureTLS != tt.wantInsecure {
				t.Errorf("NodeModelToSetter().AllowInsecureTLS = %v, want %v", gotAllowInsecureTLS, tt.wantInsecure)
			}

			secretNull, okSecretNull := setter.SecretKey.GetNull()
			if !okSecretNull {
				t.Fatalf("NodeModelToSetter().SecretKey should be set")
			}

			if tt.wantSecretIsNull {
				if !secretNull.IsNull() {
					t.Errorf("NodeModelToSetter().SecretKey should be null")
				}
				return
			}

			gotSecret, okSecret := secretNull.Get()
			if !okSecret {
				t.Fatalf("NodeModelToSetter().SecretKey value should be set")
			}
			if gotSecret != tt.wantSecretKey {
				t.Errorf("NodeModelToSetter().SecretKey = %q, want %q", gotSecret, tt.wantSecretKey)
			}
		})
	}
}
