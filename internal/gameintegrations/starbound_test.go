package gameintegrations

import (
	"errors"
	"testing"
)

func TestStarboundSteamUsername(t *testing.T) {
	tests := []struct {
		name        string
		environment map[string]string
		want        string
		wantErr     bool
	}{
		{name: "configured", environment: map[string]string{"STEAM_USERNAME": "owner"}, want: "owner"},
		{name: "case insensitive", environment: map[string]string{"steam_username": " owner "}, want: "owner"},
		{name: "missing", environment: map[string]string{}, wantErr: true},
		{name: "blank", environment: map[string]string{"STEAM_USERNAME": "  "}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, errUsername := StarboundSteamUsername(test.environment)
			if test.wantErr {
				if !errors.Is(errUsername, ErrStarboundSteamUsernameRequired) {
					t.Fatalf("StarboundSteamUsername() error = %v, want %v", errUsername, ErrStarboundSteamUsernameRequired)
				}
				return
			}
			if errUsername != nil {
				t.Fatalf("StarboundSteamUsername() error = %v", errUsername)
			}
			if got != test.want {
				t.Fatalf("StarboundSteamUsername() = %q, want %q", got, test.want)
			}
		})
	}
}
