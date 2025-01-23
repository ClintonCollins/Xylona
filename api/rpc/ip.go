package rpc

import (
	"context"
	"database/sql"
	"errors"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (xs XylonaService) ListIPs(ctx context.Context, request *connect.Request[xylona.ListIPsRequest]) (*connect.Response[xylona.ListIPsResponse], error) {
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

func (xs XylonaService) AddIP(ctx context.Context, request *connect.Request[xylona.AddIPRequest]) (*connect.Response[xylona.AddIPResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) RemoveIP(ctx context.Context, request *connect.Request[xylona.RemoveIPRequest]) (*connect.Response[xylona.RemoveIPResponse], error) {
	//TODO implement me
	panic("implement me")
}
