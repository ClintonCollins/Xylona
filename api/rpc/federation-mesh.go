package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (fs FederationService) ExchangePeerList(
	ctx context.Context,
	request *connect.Request[xylona.ExchangePeerListRequest],
) (*connect.Response[xylona.ExchangePeerListResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	localPeers, errBuild := fs.actionsInst.BuildLocalPeerList()
	if errBuild != nil {
		log.Error().Err(errBuild).Msg("Failed to build local peer list for exchange")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to build peer list"))
	}

	// Trigger auto-pairing for unknown peers in the background.
	go fs.actionsInst.ProcessReceivedPeerList(request.Msg.GetPeers(), request.Msg.GetSenderNodeId())

	return connect.NewResponse(&xylona.ExchangePeerListResponse{
		Peers: localPeers,
	}), nil
}

func (fs FederationService) NotifyPeerChange(
	ctx context.Context,
	request *connect.Request[xylona.NotifyPeerChangeRequest],
) (*connect.Response[xylona.NotifyPeerChangeResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	go fs.actionsInst.HandlePeerChange(request.Msg)

	return connect.NewResponse(&xylona.NotifyPeerChangeResponse{}), nil
}

func (fs FederationService) NotifyDeparture(
	ctx context.Context,
	request *connect.Request[xylona.NotifyDepartureRequest],
) (*connect.Response[xylona.NotifyDepartureResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	go fs.actionsInst.HandleNodeDeparture(request.Msg.GetNodeId(), request.Msg.GetReason())

	return connect.NewResponse(&xylona.NotifyDepartureResponse{}), nil
}
