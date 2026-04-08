package rpc

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// ListFederationAdvisories returns federation advisory entries visible to the caller.
func (xs *XylonaService) ListFederationAdvisories(
	_ context.Context,
	request *connect.Request[xylona.ListFederationAdvisoriesRequest],
) (*connect.Response[xylona.ListFederationAdvisoriesResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	limit := request.Msg.GetLimit()
	if limit <= 0 {
		limit = 50
	}

	advisories, total, errList := xs.db.ListFederationAdvisories(
		request.Msg.GetUnreadOnly(),
		int(limit),
		int(request.Msg.GetOffset()),
	)
	if errList != nil {
		return nil, connect.NewError(connect.CodeInternal, errList)
	}

	protoAdvisories := make([]*xylona.FederationAdvisory, len(advisories))
	for i, a := range advisories {
		protoAdvisories[i] = &xylona.FederationAdvisory{
			Id:                 a.ID,
			Type:               a.Type,
			Title:              a.Title,
			Message:            a.Message,
			SourceNodeId:       a.SourceNodeID,
			SourceNodeName:     a.SourceNodeName,
			SubjectNodeId:      a.SubjectNodeID,
			SubjectNodeName:    a.SubjectNodeName,
			SubjectNodeBaseUrl: a.SubjectNodeBaseURL,
			Read:               a.Read,
			CreatedAt:          timestamppb.New(a.CreatedAt),
		}
	}

	return connect.NewResponse(&xylona.ListFederationAdvisoriesResponse{
		Advisories: protoAdvisories,
		TotalCount: helpers.ClampInt32FromInt64(total),
	}), nil
}

// MarkAdvisoriesRead marks the requested federation advisories as read.
func (xs *XylonaService) MarkAdvisoriesRead(
	_ context.Context,
	request *connect.Request[xylona.MarkAdvisoriesReadRequest],
) (*connect.Response[xylona.MarkAdvisoriesReadResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	errMark := xs.db.MarkAdvisoriesRead(request.Msg.GetAdvisoryIds())
	if errMark != nil {
		return nil, connect.NewError(connect.CodeInternal, errMark)
	}
	return connect.NewResponse(&xylona.MarkAdvisoriesReadResponse{}), nil
}

// GetUnreadAdvisoryCount returns the number of unread federation advisories.
func (xs *XylonaService) GetUnreadAdvisoryCount(
	_ context.Context,
	request *connect.Request[xylona.GetUnreadAdvisoryCountRequest],
) (*connect.Response[xylona.GetUnreadAdvisoryCountResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	count, errCount := xs.db.GetUnreadAdvisoryCount()
	if errCount != nil {
		return nil, connect.NewError(connect.CodeInternal, errCount)
	}
	return connect.NewResponse(&xylona.GetUnreadAdvisoryCountResponse{
		Count: helpers.ClampInt32FromInt64(count),
	}), nil
}

// LeaveFederation removes the local node from the current federation mesh.
func (xs *XylonaService) LeaveFederation(
	_ context.Context,
	request *connect.Request[xylona.LeaveFederationRequest],
) (*connect.Response[xylona.LeaveFederationResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	errLeave := xs.actionsInst.LeaveFederation()
	if errLeave != nil {
		return nil, connect.NewError(connect.CodeInternal, errLeave)
	}

	return connect.NewResponse(&xylona.LeaveFederationResponse{}), nil
}
