package main

import "testing"

func TestResolveReportedListenURL(t *testing.T) {
	originalLookup := lookupAdvertiseLocalIP
	originalHostname := readHostname
	t.Cleanup(func() {
		lookupAdvertiseLocalIP = originalLookup
		readHostname = originalHostname
	})

	tests := []struct {
		name          string
		cfg           *cliConfig
		lookupIP      string
		hostname      string
		wantListenURL string
	}{
		{
			name: "explicit advertise URL wins",
			cfg: &cliConfig{
				advertiseURL:  " https://node.example.com:9500/ ",
				listen:        ":9500",
				controllerURL: "https://controller.example.com",
			},
			wantListenURL: "https://node.example.com:9500",
		},
		{
			name: "wildcard listen advertises routed IPv4",
			cfg: &cliConfig{
				listen:        ":9500",
				controllerURL: "https://controller.example.com:7777",
			},
			lookupIP:      "192.168.4.25",
			hostname:      "Clintons-Windows-11-Desktop",
			wantListenURL: "https://192.168.4.25:9500",
		},
		{
			name: "wildcard IPv6 listen advertises routed IPv4",
			cfg: &cliConfig{
				listen:        "[::]:9500",
				controllerURL: "https://controller.example.com:7777",
			},
			lookupIP:      "192.168.4.25",
			hostname:      "Clintons-Windows-11-Desktop",
			wantListenURL: "https://192.168.4.25:9500",
		},
		{
			name: "wildcard listen falls back to hostname when route lookup fails",
			cfg: &cliConfig{
				listen:        ":9500",
				controllerURL: "https://controller.example.com:7777",
			},
			hostname:      "Clintons-Windows-11-Desktop",
			wantListenURL: "https://Clintons-Windows-11-Desktop:9500",
		},
		{
			name: "explicit listen host is preserved",
			cfg: &cliConfig{
				listen:        "media:9500",
				controllerURL: "https://controller.example.com:7777",
			},
			lookupIP:      "192.168.4.25",
			hostname:      "Clintons-Windows-11-Desktop",
			wantListenURL: "https://media:9500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookupAdvertiseLocalIP = func(_ string) string {
				return tt.lookupIP
			}
			readHostname = func() (string, error) {
				return tt.hostname, nil
			}

			gotListenURL := resolveReportedListenURL(tt.cfg)
			if gotListenURL != tt.wantListenURL {
				t.Fatalf("resolveReportedListenURL() = %q, want %q", gotListenURL, tt.wantListenURL)
			}
		})
	}
}
