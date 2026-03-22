package modproviders_test

import (
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"

	// Blank imports trigger init() registration for all providers.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/hangar"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/modrinth"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/papermc"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/steamworkshop"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
)

func TestAllProvidersRegistered(t *testing.T) {
	expected := []string{"modrinth", "hangar", "thunderstore", "papermc", "steam_workshop"}

	providers := modproviders.AllProviders()
	for _, id := range expected {
		_, ok := providers[id]
		if !ok {
			t.Errorf("provider %q not registered — missing blank import?", id)
		}
	}
}

func TestProviderCount(t *testing.T) {
	providers := modproviders.AllProviders()
	// Account for the mock providers registered by provider_test.go in the same package.
	// We just verify that at least the 5 real providers are present.
	if len(providers) < 5 {
		t.Errorf("expected at least 5 providers, got %d", len(providers))
	}
}
