package node

import (
	"testing"
	"time"
)

func TestNodeSnapshotRunningGameServerCount(t *testing.T) {
	snapshot := &NodeSnapshot{Processes: []ProcessSnapshot{
		{ID: "srv-online", Status: "ONLINE"},
		{ID: "srv-starting", Status: "PRE_START"},
		{ID: "srv-installing", Status: "INSTALLING"},
		{ID: "srv-updating", Status: "UPDATING"},
		{ID: "srv-offline", Status: "OFFLINE"},
		{ID: "srv-other-node", Status: "ONLINE"},
		{ID: minecraftMapProcessID("srv-online"), Status: "ONLINE"},
	}}
	gameServerIDs := map[string]struct{}{
		"srv-online": {}, "srv-starting": {}, "srv-installing": {}, "srv-updating": {}, "srv-offline": {},
	}

	tests := []struct {
		name string
		ids  map[string]struct{}
		want int
	}{
		{name: "counts only this node's running game servers", ids: gameServerIDs, want: 4},
		{name: "no game servers", ids: nil, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := snapshot.RunningGameServerCount(test.ids); got != test.want {
				t.Fatalf("RunningGameServerCount() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestProcessConfigNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   ProcessConfig
		want ProcessConfig
	}{
		{
			name: "trims base command and copies args",
			in: ProcessConfig{
				ID:               "srv-1",
				BaseCommand:      "  ./run.sh  ",
				Args:             []string{"--port", "27015"},
				WorkingDirectory: "/srv",
			},
			want: ProcessConfig{
				ID:               "srv-1",
				BaseCommand:      "./run.sh",
				Args:             []string{"--port", "27015"},
				WorkingDirectory: "/srv",
				StopTimeout:      defaultStopTimeout,
			},
		},
		{
			name: "preserves explicit stop timeout",
			in: ProcessConfig{
				ID:          "srv-2",
				BaseCommand: "go",
				StopTimeout: 30 * time.Second,
			},
			want: ProcessConfig{
				ID:          "srv-2",
				BaseCommand: "go",
				StopTimeout: 30 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.normalize()
			if got.ID != tt.want.ID {
				t.Fatalf("ID = %q, want %q", got.ID, tt.want.ID)
			}
			if got.BaseCommand != tt.want.BaseCommand {
				t.Fatalf("BaseCommand = %q, want %q", got.BaseCommand, tt.want.BaseCommand)
			}
			if got.WorkingDirectory != tt.want.WorkingDirectory {
				t.Fatalf("WorkingDirectory = %q, want %q", got.WorkingDirectory, tt.want.WorkingDirectory)
			}
			if got.StopTimeout != tt.want.StopTimeout {
				t.Fatalf("StopTimeout = %v, want %v", got.StopTimeout, tt.want.StopTimeout)
			}
			if len(got.Args) != len(tt.want.Args) {
				t.Fatalf("Args = %v, want %v", got.Args, tt.want.Args)
			}
			for i := range got.Args {
				if got.Args[i] != tt.want.Args[i] {
					t.Fatalf("Args[%d] = %q, want %q", i, got.Args[i], tt.want.Args[i])
				}
			}
		})
	}
}
