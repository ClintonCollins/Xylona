package rpc

import (
	"context"
	"database/sql"
	"errors"
	"net"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs XylonaService) ListIPs(ctx context.Context, request *connect.Request[xylona.ListIPsRequest]) (*connect.Response[xylona.ListIPsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	ips, err := xs.db.GetAllIPs()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("no IPs found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	ipProtos := make([]*xylona.IP, len(ips))
	for i, ip := range ips {
		ipProto := helpers.IPModelToProto(ip)
		ipProtos[i] = ipProto
	}
	response := &connect.Response[xylona.ListIPsResponse]{Msg: &xylona.ListIPsResponse{Ips: ipProtos}}
	return response, nil
}

func (xs XylonaService) AddIP(_ context.Context, request *connect.Request[xylona.AddIPRequest]) (*connect.Response[xylona.AddIPResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	ipProto := request.Msg.GetIp()
	if ipProto == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ip is required"))
	}
	if net.ParseIP(ipProto.GetAddress()) == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid IP address format"))
	}

	ipSetter := &models.IPSetter{
		Address:  omit.From(ipProto.GetAddress()),
		Usable:   omit.From(ipProto.GetUsable()),
		External: omit.From(ipProto.GetExternal()),
	}

	_, errInsertIP := xs.db.InsertIP(ipSetter)
	if errInsertIP != nil {
		return nil, connect.NewError(connect.CodeInternal, errInsertIP)
	}

	return &connect.Response[xylona.AddIPResponse]{Msg: &xylona.AddIPResponse{}}, nil
}

func (xs XylonaService) RemoveIP(_ context.Context, request *connect.Request[xylona.RemoveIPRequest]) (*connect.Response[xylona.RemoveIPResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	ipProto := request.Msg.GetIp()
	if ipProto == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ip is required"))
	}
	address := ipProto.GetAddress()
	if address == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ip address is required"))
	}

	_, errGetIP := xs.db.GetIPByAddress(address)
	if errGetIP != nil {
		if errors.Is(errGetIP, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("IP not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errGetIP)
	}

	errDeleteIP := xs.db.DeleteIP(address)
	if errDeleteIP != nil {
		return nil, connect.NewError(connect.CodeInternal, errDeleteIP)
	}

	return &connect.Response[xylona.RemoveIPResponse]{Msg: &xylona.RemoveIPResponse{}}, nil
}
