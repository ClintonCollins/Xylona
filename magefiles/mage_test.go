//go:build mage

package main

import "testing"

func TestValidateDeployParam(t *testing.T) {
	tests := []struct {
		name    string
		param   string
		value   string
		wantErr bool
	}{
		{name: "valid hostname", param: "host", value: "example.com", wantErr: false},
		{name: "valid IP address", param: "host", value: "192.168.1.1", wantErr: false},
		{name: "valid username", param: "user", value: "deploy-user", wantErr: false},
		{name: "valid service name", param: "service", value: "xylona", wantErr: false},
		{name: "valid path", param: "path", value: "/usr/local/bin/xylona", wantErr: false},
		{name: "valid path with dots", param: "path", value: "/opt/xylona/bin/xylona.v2", wantErr: false},
		{name: "valid underscore", param: "service", value: "my_service", wantErr: false},
		{name: "semicolon injection", param: "service", value: "xylona; rm -rf /", wantErr: true},
		{name: "backtick injection", param: "host", value: "`whoami`", wantErr: true},
		{name: "dollar sign injection", param: "host", value: "$(whoami)", wantErr: true},
		{name: "pipe injection", param: "service", value: "xylona | cat /etc/passwd", wantErr: true},
		{name: "ampersand injection", param: "host", value: "host && echo pwned", wantErr: true},
		{name: "empty string", param: "host", value: "", wantErr: true},
		{name: "single quote injection", param: "user", value: "user'--", wantErr: true},
		{name: "double quote injection", param: "path", value: "/tmp/\"evil\"", wantErr: true},
		{name: "newline injection", param: "host", value: "host\nevil", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeployParam(tt.param, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateDeployParam(%q, %q) error = %v, wantErr %v",
					tt.param, tt.value, err, tt.wantErr)
			}
		})
	}
}
