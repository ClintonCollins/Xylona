package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestResolveBackupCapability(t *testing.T) {
	tests := []struct {
		name            string
		nodeOS          string
		linuxAllowed    bool
		windowsAllowed  bool
		wantSupported   bool
		wantUnavailable bool
	}{
		{
			name:          "linux supported",
			nodeOS:        "linux",
			linuxAllowed:  true,
			wantSupported: true,
		},
		{
			name:          "darwin uses linux capability",
			nodeOS:        "darwin",
			linuxAllowed:  true,
			wantSupported: true,
		},
		{
			name:           "windows supported",
			nodeOS:         "windows",
			windowsAllowed: true,
			wantSupported:  true,
		},
		{
			name:          "platform explicitly unsupported",
			nodeOS:        "linux",
			wantSupported: false,
		},
		{
			name:            "unknown platform is unavailable",
			nodeOS:          "plan9",
			linuxAllowed:    true,
			windowsAllowed:  true,
			wantUnavailable: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &nodeclient.FakeNodeClient{
				NodeID: "node-remote",
				SnapshotResult: &node.NodeSnapshot{
					OS: test.nodeOS,
				},
			}
			registry := noderegistry.New("node-local", nil)
			registry.Register(client)
			gameServer := &models.GameServer{
				GameID: "game-test",
				NodeID: "node-remote",
			}
			game := &models.Game{
				ID:                  "game-test",
				LinuxAllowBackups:   test.linuxAllowed,
				WindowsAllowBackups: test.windowsAllowed,
			}

			capability, errCapability := ResolveBackupCapability(
				context.Background(),
				nil,
				registry,
				gameServer,
				game,
			)
			if test.wantUnavailable {
				if !errors.Is(errCapability, ErrBackupCapabilityUnavailable) {
					t.Fatalf("ResolveBackupCapability() error = %v, want %v", errCapability, ErrBackupCapabilityUnavailable)
				}
				if capability.DisabledReason == "" {
					t.Fatal("ResolveBackupCapability().DisabledReason = empty, want unavailable reason")
				}
				return
			}
			if errCapability != nil {
				t.Fatalf("ResolveBackupCapability() error = %v", errCapability)
			}
			if capability.Supported != test.wantSupported {
				t.Fatalf("ResolveBackupCapability().Supported = %v, want %v", capability.Supported, test.wantSupported)
			}
			if !test.wantSupported && capability.DisabledReason == "" {
				t.Fatal("ResolveBackupCapability().DisabledReason = empty, want unsupported reason")
			}
		})
	}
}

func TestResolveBackupCapabilityFailsClosedWhenRemoteNodeIsUnavailable(t *testing.T) {
	registry := noderegistry.New("node-local", nil)
	gameServer := &models.GameServer{
		GameID: "game-test",
		NodeID: "node-remote",
	}
	game := &models.Game{
		ID:                  "game-test",
		LinuxAllowBackups:   true,
		WindowsAllowBackups: true,
	}

	capability, errCapability := ResolveBackupCapability(
		context.Background(),
		nil,
		registry,
		gameServer,
		game,
	)
	if !errors.Is(errCapability, ErrBackupCapabilityUnavailable) {
		t.Fatalf("ResolveBackupCapability() error = %v, want %v", errCapability, ErrBackupCapabilityUnavailable)
	}
	if capability.Supported {
		t.Fatal("ResolveBackupCapability().Supported = true, want false")
	}
}
