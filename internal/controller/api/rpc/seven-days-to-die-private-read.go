package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const sevenDaysToDiePrivateWebAPINodeProtocol int64 = 10

type sevenDaysToDiePrivateReadOutcome uint8

const (
	sevenDaysToDiePrivateReadReady sevenDaysToDiePrivateReadOutcome = iota
	sevenDaysToDiePrivateReadNodeUnavailable
	sevenDaysToDiePrivateReadServerOffline
	sevenDaysToDiePrivateReadRuntimeUnavailable
	sevenDaysToDiePrivateReadUnsupported
)

type sevenDaysToDiePrivateReadAccess struct {
	client           nodeclient.NodeClient
	workingDirectory string
	tokenName        string
	tokenSecret      string
}

func (xs *XylonaService) prepareSevenDaysToDiePrivateRead(
	ctx context.Context,
	gameServer *models.GameServer,
) (sevenDaysToDiePrivateReadAccess, sevenDaysToDiePrivateReadOutcome, error) {
	var access sevenDaysToDiePrivateReadAccess

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return access, sevenDaysToDiePrivateReadNodeUnavailable, nil //nolint:nilerr // Node reachability is a typed operational outcome.
	}
	process, found, errProcess := client.GetProcessSnapshot(ctx, gameServer.ID)
	if errProcess != nil {
		if errors.Is(errProcess, context.Canceled) || errors.Is(errProcess, context.DeadlineExceeded) {
			return access, sevenDaysToDiePrivateReadReady, connect.NewError(contextConnectCode(errProcess), errProcess)
		}
		return access, sevenDaysToDiePrivateReadNodeUnavailable, nil
	}
	if !found || process == nil || process.Status != xylona.Status_ONLINE.String() {
		return access, sevenDaysToDiePrivateReadServerOffline, nil
	}

	capabilities, errCapabilities := client.GetRuntimeCapabilities(ctx)
	if errCapabilities != nil {
		if errors.Is(errCapabilities, context.Canceled) || errors.Is(errCapabilities, context.DeadlineExceeded) {
			return access, sevenDaysToDiePrivateReadReady, connect.NewError(contextConnectCode(errCapabilities), errCapabilities)
		}
		return access, sevenDaysToDiePrivateReadRuntimeUnavailable, nil
	}
	if capabilities.ProtocolVersion < sevenDaysToDiePrivateWebAPINodeProtocol {
		return access, sevenDaysToDiePrivateReadUnsupported, nil
	}
	if xs.actionsInst == nil {
		return access, sevenDaysToDiePrivateReadReady, internalErrf("7 Days to Die WebAPI credentials are unavailable")
	}

	tokenName, tokenSecret, errCredentials := xs.actionsInst.SevenDaysToDieMapCredentials(gameServer)
	if errCredentials != nil {
		return access, sevenDaysToDiePrivateReadReady, internalErrf("failed to resolve 7 Days to Die WebAPI credentials")
	}
	access = sevenDaysToDiePrivateReadAccess{
		client:           client,
		workingDirectory: gameServer.Directory,
		tokenName:        tokenName,
		tokenSecret:      tokenSecret,
	}
	return access, sevenDaysToDiePrivateReadReady, nil
}
