package rpc

import (
	"context"
	"database/sql"
	"errors"

	connect_go "github.com/bufbuild/connect-go"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (xs XylonaService) ListIPs(ctx context.Context, request *connect_go.Request[xylona.ListIPsRequest]) (*connect_go.Response[xylona.ListIPsResponse], error) {
	ips, err := xs.db.GetAllIPs()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("no IPs found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, err)
	}
	ipProtos := make([]*xylona.IP, len(ips))
	for i, ip := range ips {
		ipProto := helpers.IPModelToProto(ip)
		ipProtos[i] = ipProto
	}
	response := &connect_go.Response[xylona.ListIPsResponse]{Msg: &xylona.ListIPsResponse{Ips: ipProtos}}
	return response, nil
}

func (xs XylonaService) AddIP(ctx context.Context, request *connect_go.Request[xylona.AddIPRequest]) (*connect_go.Response[xylona.AddIPResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) RemoveIP(ctx context.Context, request *connect_go.Request[xylona.RemoveIPRequest]) (*connect_go.Response[xylona.RemoveIPResponse], error) {
	//TODO implement me
	panic("implement me")
}
