package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/modmanager"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func modMutationError(err error, operation string) error {
	if errors.Is(err, modmanager.ErrValheimServerRunning) {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("stop the Valheim server before changing mods"))
	}
	return internalErrf("failed to " + operation + " mod")
}

func (xs *XylonaService) ensureValheimModsStopped(ctx context.Context, server *models.GameServer) error {
	if server.GameID != "valheim" {
		return nil
	}
	client, errClient := xs.resolveNodeClient(server)
	if errClient != nil {
		return errClient
	}
	process, found, errProcess := client.GetProcessSnapshot(ctx, server.ID)
	if errProcess != nil {
		return connect.NewError(connect.CodeUnavailable, errors.New("cannot check whether the server is stopped; reconnect the node and retry"))
	}
	if found && (process == nil || process.Status != xylona.Status_OFFLINE.String()) {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("stop the Valheim server before installing, updating, removing, or toggling mods"))
	}
	return nil
}
