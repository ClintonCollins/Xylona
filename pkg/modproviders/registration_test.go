package modproviders_test

import (
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"

	// Blank imports trigger init() registration for all providers.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/hangar"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/modrinth"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/mojang"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/papermc"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/steamworkshop"
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
)

func TestAllProvidersRegistered(t *testing.T) {
	expected := []string{"modrinth", "hangar", "mojang", "thunderstore", "papermc", "steam_workshop"}

	providers := modproviders.AllProviders()
	for _, id := range expected {
		_, ok := providers[id]
		if !ok {
			t.Errorf("provider %q not registered — missing blank import?", id)
		}
	}
}
