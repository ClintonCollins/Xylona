package rpc

import (
	// Register Hangar for variant and mod provider discovery.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/hangar"
	// Register Modrinth for variant and mod provider discovery.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/modrinth"
	// Register Mojang for variant and mod provider discovery.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/mojang"
	// Register PaperMC for variant and mod provider discovery.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/papermc"
	// Register Steam Workshop for variant and mod provider discovery.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/steamworkshop"
	// Register Thunderstore for variant and mod provider discovery.
	_ "github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
)
