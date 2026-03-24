package rpc

import (
	"context"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs *XylonaService) resolveLocalVersionData(ctx context.Context, gameServer *models.GameServer, opts actions.VersionResolveOptions) (string, *xylona.VersionInfo) {
	if xs.actionsInst != nil {
		version, state := xs.actionsInst.ResolveVersionData(ctx, gameServer, opts)
		return version, versionStateToProto(state)
	}

	version := versiontracker.ResolveCurrentVersion(gameServer)
	if xs.versionState == nil {
		return version, nil
	}
	return version, versionStateToProto(xs.versionState.Get(gameServer.ID))
}

func (fs *FederationService) resolveLocalVersionData(ctx context.Context, gameServer *models.GameServer, opts actions.VersionResolveOptions) (string, *xylona.VersionInfo) {
	if fs.actionsInst != nil {
		version, state := fs.actionsInst.ResolveVersionData(ctx, gameServer, opts)
		return version, versionStateToProto(state)
	}

	version := versiontracker.ResolveCurrentVersion(gameServer)
	if fs.versionState == nil {
		return version, nil
	}
	return version, versionStateToProto(fs.versionState.Get(gameServer.ID))
}
