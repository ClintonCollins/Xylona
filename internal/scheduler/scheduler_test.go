package scheduler

import (
	"slices"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/sql/models"
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

func TestUTCTasksOffHostClock(t *testing.T) {
	t.Parallel()

	london, errLoad := time.LoadLocation("Europe/London")
	if errLoad != nil {
		t.Fatalf("LoadLocation(Europe/London) error = %v", errLoad)
	}
	winter := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	tasks := []*models.ScheduledTask{
		{Name: "utc restart", Timezone: "UTC"},
		{Name: "unset zone", Timezone: ""},
		{Name: "chicago backup", Timezone: "America/Chicago"},
	}

	tests := []struct {
		name  string
		local *time.Location
		want  []string
	}{
		{name: "UTC host keeps its run times", local: time.UTC, want: nil},
		{
			name:  "offset host lists UTC tasks",
			local: time.FixedZone("CDT", -5*60*60),
			want:  []string{"utc restart", "unset zone"},
		},
		{
			name:  "host on UTC only in winter still lists them",
			local: london,
			want:  []string{"utc restart", "unset zone"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := utcTasksOffHostClock(tasks, tt.local, winter)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("utcTasksOffHostClock() = %v, want %v", got, tt.want)
			}
		})
	}
}
