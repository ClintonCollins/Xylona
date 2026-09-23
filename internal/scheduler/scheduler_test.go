package scheduler

import (
	"testing"
	"time"
)

func TestNextRun(t *testing.T) {
	t.Parallel()

	chicago, errLoad := time.LoadLocation("America/Chicago")
	if errLoad != nil {
		t.Fatalf("LoadLocation(America/Chicago) error = %v", errLoad)
	}
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		cron     string
		timezone string
		want     time.Time
		wantErr  bool
	}{
		{
			name:     "UTC runs on the UTC clock, not the host clock",
			cron:     "0 3 * * *",
			timezone: "UTC",
			want:     time.Date(2026, 9, 23, 3, 0, 0, 0, time.UTC),
		},
		{
			name:     "empty zone means UTC",
			cron:     "0 3 * * *",
			timezone: "",
			want:     time.Date(2026, 9, 23, 3, 0, 0, 0, time.UTC),
		},
		{
			name:     "named zone runs on that zone's clock",
			cron:     "0 3 * * *",
			timezone: "America/Chicago",
			want:     time.Date(2026, 9, 23, 3, 0, 0, 0, chicago),
		},
		{
			name:     "weekday list",
			cron:     "30 4 * * 1,3",
			timezone: "UTC",
			want:     time.Date(2026, 9, 23, 4, 30, 0, 0, time.UTC),
		},
		{name: "minute out of range", cron: "99 3 * * *", timezone: "UTC", wantErr: true},
		{name: "weekday out of range", cron: "0 3 * * 7", timezone: "UTC", wantErr: true},
		{name: "date that never exists", cron: "0 0 30 2 *", timezone: "UTC", wantErr: true},
		{name: "unknown zone", cron: "0 3 * * *", timezone: "Mars/Olympus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, errNext := NextRun(tt.cron, tt.timezone, now)
			if tt.wantErr {
				if errNext == nil {
					t.Fatalf("NextRun(%q, %q) = %v, want error", tt.cron, tt.timezone, got)
				}
				return
			}
			if errNext != nil {
				t.Fatalf("NextRun(%q, %q) error = %v", tt.cron, tt.timezone, errNext)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("NextRun(%q, %q) = %v, want %v", tt.cron, tt.timezone, got, tt.want)
			}
		})
	}
}
