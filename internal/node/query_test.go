package node

import (
	"context"
	"testing"
)

func TestQueryGameServerReturnsDefaultMaxPlayersOnProbeError(t *testing.T) {
	t.Parallel()

	n := &Node{}
	cases := []struct {
		name       string
		kind       GameServerQueryKind
		maxPlayers int64
		assert     func(t *testing.T, result GameServerQueryResult)
	}{
		{
			name:       "minecraft",
			kind:       GameServerQueryKindMinecraft,
			maxPlayers: 48,
			assert: func(t *testing.T, result GameServerQueryResult) {
				t.Helper()
				if result.Minecraft == nil {
					t.Fatal("Minecraft result is nil")
				}
				if result.Minecraft.MaxPlayers != 48 {
					t.Fatalf("Minecraft max players = %d, want 48", result.Minecraft.MaxPlayers)
				}
			},
		},
		{
			name:       "source",
			kind:       GameServerQueryKindSource,
			maxPlayers: 32,
			assert: func(t *testing.T, result GameServerQueryResult) {
				t.Helper()
				if result.Source == nil {
					t.Fatal("Source result is nil")
				}
				if result.Source.MaxPlayers != 32 {
					t.Fatalf("Source max players = %d, want 32", result.Source.MaxPlayers)
				}
			},
		},
		{
			name:       "palworld",
			kind:       GameServerQueryKindPalworld,
			maxPlayers: 64,
			assert: func(t *testing.T, result GameServerQueryResult) {
				t.Helper()
				if result.Palworld == nil {
					t.Fatal("Palworld result is nil")
				}
				if result.Palworld.MaxPlayers != 64 {
					t.Fatalf("Palworld max players = %d, want 64", result.Palworld.MaxPlayers)
				}
				if result.Palworld.Responded {
					t.Fatal("Palworld responded = true, want false for probe error")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, errQuery := n.QueryGameServer(context.Background(), GameServerQueryRequest{
				Kind:       tc.kind,
				IP:         "127.0.0.1",
				QueryPort:  1,
				MaxPlayers: tc.maxPlayers,
			})
			if errQuery != nil {
				t.Fatalf("QueryGameServer: %v", errQuery)
			}
			if result.Kind != tc.kind {
				t.Fatalf("kind = %v, want %v", result.Kind, tc.kind)
			}
			tc.assert(t, result)
		})
	}
}

func TestQueryGameServerHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n := &Node{}
	_, errQuery := n.QueryGameServer(ctx, GameServerQueryRequest{Kind: GameServerQueryKindMinecraft})
	if errQuery == nil {
		t.Fatal("QueryGameServer error = nil, want canceled context error")
	}
}
