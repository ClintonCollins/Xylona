// Package sevendaystodiemod installs Xylona's native 7 Days to Die helper mod.
package sevendaystodiemod

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	modDirectory     = "Mods/Xylona_LandClaims"
	webServerDLLPath = "Mods/TFP_WebServer/WebServer.dll"
)

// ErrAssetsUnavailable reports a build missing one of the embedded helper files.
var ErrAssetsUnavailable = errors.New("land claim helper assets are unavailable")

//go:embed assets/ModInfo.xml
var modInfo []byte

//go:embed assets/v2.6/XylonaLandClaims.dll
var v26DLL []byte

//go:embed assets/v3/XylonaLandClaims.dll
var v3DLL []byte

// Install installs or repairs the native land-claim WebAPI helper.
func Install(
	ctx context.Context,
	client nodeclient.NodeClient,
	gameServer *models.GameServer,
	policy node.ProtectionPolicy,
) error {
	if client == nil {
		return errors.New("install land claim helper: node client is missing")
	}
	if gameServer == nil {
		return errors.New("install land claim helper: game server is missing")
	}
	errContext := ctx.Err()
	if errContext != nil {
		return fmt.Errorf("install land claim helper: %w", errContext)
	}

	dll := v26DLL
	_, errStat := client.StatFile(ctx, gameServer.Directory, webServerDLLPath)
	if errors.Is(errStat, os.ErrNotExist) {
		dll = v3DLL
	} else if errStat != nil {
		return fmt.Errorf("detect 7 Days to Die WebServer version: %w", errStat)
	}
	if len(modInfo) == 0 || len(dll) == 0 {
		return ErrAssetsUnavailable
	}

	errCreate := client.CreateFileOrDirectory(ctx, gameServer.Directory, modDirectory, "", true, policy)
	if errCreate != nil {
		return fmt.Errorf("create land claim helper directory: %w", errCreate)
	}
	errWriteDLL := client.WriteFile(ctx, gameServer.Directory, modDirectory+"/XylonaLandClaims.dll", dll, policy)
	if errWriteDLL != nil {
		return fmt.Errorf("write land claim helper DLL: %w", errWriteDLL)
	}
	errWriteModInfo := client.WriteFile(ctx, gameServer.Directory, modDirectory+"/ModInfo.xml", modInfo, policy)
	if errWriteModInfo != nil {
		return fmt.Errorf("write land claim helper metadata: %w", errWriteModInfo)
	}

	return nil
}
