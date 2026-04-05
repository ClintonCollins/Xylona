package actions

import (
	// Register Hangar as an available mod provider.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/hangar"
	// Register Modrinth as an available mod provider.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/modrinth"
	// Register Mojang as an available mod provider.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/mojang"
	// Register PaperMC as an available mod provider.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/papermc"
	// Register Steam Workshop as an available mod provider.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/steamworkshop"
	// Register Thunderstore as an available mod provider.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
)
