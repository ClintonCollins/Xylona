package actions

import (
	"context"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
)

func TestEnsureTelnetInputSupported(t *testing.T) {
	tests := []struct {
		name       string
		capability bool
		wantError  bool
	}{
		{name: "supported", capability: true},
		{name: "upgrade required", capability: false, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &runtimeCapabilitiesClientFake{
				result: node.RuntimeCapabilities{TelnetInput: test.capability},
			}
			inst := &Instance{ctx: context.Background()}

			errSupported := inst.ensureTelnetInputSupported(client)
			if test.wantError && errSupported == nil {
				t.Fatal("ensureTelnetInputSupported() error = nil")
			}
			if !test.wantError && errSupported != nil {
				t.Fatalf("ensureTelnetInputSupported() error = %v", errSupported)
			}
			if client.calls != 1 {
				t.Fatalf("GetRuntimeCapabilities call count = %d, want 1", client.calls)
			}
		})
	}
}
