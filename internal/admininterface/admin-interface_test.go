package admininterface

import "testing"

func TestLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		gameID        string
		wantTransport string
		wantPort      int64
		wantUsername  string
		wantSupported bool
	}{
		{name: "7 Days to Die Telnet", gameID: "7_days_to_die", wantTransport: TransportTelnet, wantPort: 27016, wantSupported: true},
		{name: "Source RCON uses game port", gameID: "counter_strike_2", wantTransport: TransportRCON, wantPort: 27015, wantSupported: true},
		{name: "Palworld REST", gameID: "palworld", wantTransport: TransportREST, wantPort: 27016, wantUsername: "admin", wantSupported: true},
		{name: "Satisfactory REST uses main port", gameID: "satisfactory", wantTransport: TransportREST, wantPort: 27015, wantSupported: true},
		{name: "stdin only game", gameID: "minecraft"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			profile, supported := Lookup(tc.gameID, 27015, 27016)
			if supported != tc.wantSupported {
				t.Fatalf("Lookup() supported = %t, want %t", supported, tc.wantSupported)
			}
			if !supported {
				return
			}
			if profile.Transport != tc.wantTransport || profile.Port != tc.wantPort ||
				profile.Username != tc.wantUsername || profile.SecretKind == "" || profile.SecretName == "" {
				t.Fatalf("Lookup() profile = %+v", profile)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		gameID   string
		password string
		wantErr  bool
	}{
		{name: "safe shared password", gameID: "factorio", password: "Valid-Admin_123!"},
		{name: "too short", gameID: "factorio", password: "short", wantErr: true},
		{name: "space", gameID: "factorio", password: "invalid password", wantErr: true},
		{name: "quote", gameID: "factorio", password: `invalid"password`, wantErr: true},
		{name: "Palworld comma", gameID: "palworld", password: "invalid,password", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			errValidate := ValidatePassword(tc.gameID, tc.password)
			if tc.wantErr != (errValidate != nil) {
				t.Fatalf("ValidatePassword() error = %v, want error %t", errValidate, tc.wantErr)
			}
		})
	}
}
