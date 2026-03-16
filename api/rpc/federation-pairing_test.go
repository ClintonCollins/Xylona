package rpc

import "testing"

func TestFormatPairingToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short token",
			input: "abcd1234",
			want:  "abcd1234",
		},
		{
			name:  "16 char token",
			input: "abcd1234efgh5678",
			want:  "abcd1234-efgh5678",
		},
		{
			name:  "64 char token",
			input: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			want:  "abcdef01-23456789-abcdef01-23456789-abcdef01-23456789-abcdef01-23456789",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPairingToken(tt.input)
			if got != tt.want {
				t.Errorf("formatPairingToken(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizePairingToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "formatted token",
			input: "abcd1234-efgh5678",
			want:  "abcd1234efgh5678",
		},
		{
			name:  "raw token",
			input: "abcd1234efgh5678",
			want:  "abcd1234efgh5678",
		},
		{
			name:  "with spaces and dashes",
			input: "  abcd-1234-efgh-5678  ",
			want:  "abcd1234efgh5678",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePairingToken(tt.input)
			if got != tt.want {
				t.Errorf("normalizePairingToken(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFingerprintsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{
			name: "identical",
			a:    "abc123",
			b:    "abc123",
			want: true,
		},
		{
			name: "case insensitive",
			a:    "ABC123",
			b:    "abc123",
			want: true,
		},
		{
			name: "different",
			a:    "abc123",
			b:    "def456",
			want: false,
		},
		{
			name: "with whitespace",
			a:    "  abc123  ",
			b:    "abc123",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fingerprintsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("fingerprintsEqual(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
