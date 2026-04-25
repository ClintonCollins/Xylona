package defaultpaths

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolveInstallPath(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		home        string
		user        string
		userProfile string
		want        string
		wantErr     error
	}{
		{
			name: "linux uses home",
			goos: "linux",
			home: "/home/alice",
			user: "ignored",
			want: "/home/alice/xylona",
		},
		{
			name: "darwin falls back to user",
			goos: "darwin",
			user: "alice",
			want: "/home/alice/xylona",
		},
		{
			name:        "windows uses user profile",
			goos:        "windows",
			userProfile: filepath.Join("C:", "Users", "Alice"),
			want:        filepath.Join("C:", "Users", "Alice", "Xylona"),
		},
		{
			name:    "unix missing home and user",
			goos:    "linux",
			wantErr: ErrMissingUnixHomeUser,
		},
		{
			name:    "windows missing user profile",
			goos:    "windows",
			wantErr: ErrMissingWindowsUserProfile,
		},
		{
			name:    "unsupported os",
			goos:    "plan9",
			wantErr: ErrUnsupportedOS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errResolve := ResolveInstallPath(tt.goos, tt.home, tt.user, tt.userProfile)
			if tt.wantErr != nil {
				if !errors.Is(errResolve, tt.wantErr) {
					t.Fatalf("ResolveInstallPath() error = %v, want %v", errResolve, tt.wantErr)
				}
				return
			}
			if errResolve != nil {
				t.Fatalf("ResolveInstallPath() error = %v", errResolve)
			}
			if got != tt.want {
				t.Fatalf("ResolveInstallPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
