package rpc

import "testing"

func TestNormalizeFederationPort(t *testing.T) {
	tests := []struct {
		name    string
		port    int64
		want    int
		wantErr bool
	}{
		{
			name:    "allows zero for fallback behavior",
			port:    0,
			want:    0,
			wantErr: false,
		},
		{
			name:    "accepts valid TCP port",
			port:    8443,
			want:    8443,
			wantErr: false,
		},
		{
			name:    "rejects negative port",
			port:    -1,
			want:    0,
			wantErr: true,
		},
		{
			name:    "rejects out of range port",
			port:    65536,
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errNormalize := normalizeFederationPort(tt.port)
			if (errNormalize != nil) != tt.wantErr {
				t.Fatalf("normalizeFederationPort() error = %v, wantErr %v", errNormalize, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("normalizeFederationPort() = %v, want %v", got, tt.want)
			}
		})
	}
}
