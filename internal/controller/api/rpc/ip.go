package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/controller/protomap"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func normalizeBindableIPs(rawIPs []node.BindableIP, nodeID string) []*xylona.IP {
	ips := make([]*xylona.IP, 0, len(rawIPs))
	for _, rawIP := range rawIPs {
		address := strings.TrimSpace(rawIP.Address)
		if address == "" {
			continue
		}
		ips = append(ips, &xylona.IP{
			Address:  address,
			Usable:   rawIP.Usable,
			External: rawIP.External,
			NodeId:   nodeID,
		})
	}
	return ips
}

func (xs *XylonaService) listRuntimeIPs(ctx context.Context, nodeID string) []*xylona.IP {
	if xs == nil {
		return nil
	}

	if xs.nodeRegistry != nil {
		client, errGetClient := xs.nodeRegistry.Get(nodeID)
		if errGetClient == nil {
			baseCtx := ctx
			if baseCtx == nil {
				baseCtx = context.Background()
			}
			runtimeCtx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
			defer cancel()

			rawIPs, errIPs := client.ListBindableIPs(runtimeCtx)
			if errIPs == nil {
				return normalizeBindableIPs(rawIPs, nodeID)
			}
		}
	}

	if nodeID != xs.selfNodeID() {
		return nil
	}

	localIPs, errLocalIPs := helpers.GetBindableIPs()
	if errLocalIPs != nil {
		return nil
	}

	rawIPs := make([]node.BindableIP, 0, len(localIPs))
	for _, localIP := range localIPs {
		if localIP == nil {
			continue
		}
		rawIPs = append(rawIPs, node.BindableIP{
			Address:  localIP.String(),
			Usable:   true,
			External: !localIP.IsPrivate(),
		})
	}
	return normalizeBindableIPs(rawIPs, nodeID)
}

func mergeNodeIPs(storedIPs []*models.IP, runtimeIPs []*xylona.IP, nodeID string) []*xylona.IP {
	merged := make([]*xylona.IP, 0, len(storedIPs)+len(runtimeIPs))
	seenAddresses := make(map[string]struct{}, len(storedIPs)+len(runtimeIPs))

	for _, storedIP := range storedIPs {
		if storedIP == nil {
			continue
		}
		address := strings.TrimSpace(storedIP.Address)
		if address == "" {
			continue
		}
		ipProto := protomap.IPModelToProto(storedIP)
		ipProto.NodeId = nodeID
		merged = append(merged, ipProto)
		seenAddresses[address] = struct{}{}
	}

	for _, runtimeIP := range runtimeIPs {
		if runtimeIP == nil {
			continue
		}
		address := strings.TrimSpace(runtimeIP.GetAddress())
		if address == "" {
			continue
		}
		if _, exists := seenAddresses[address]; exists {
			continue
		}
		runtimeIP.NodeId = nodeID
		merged = append(merged, runtimeIP)
		seenAddresses[address] = struct{}{}
	}

	return merged
}

func (xs *XylonaService) resolveListIPsNodeID(requestNodeID string) (string, error) {
	nodeID := strings.TrimSpace(requestNodeID)
	if nodeID == "" {
		nodeID = xs.selfNodeID()
	}
	if nodeID == "" {
		return "", errors.New("local node ID is unavailable")
	}

	_, errNode := xs.db.GetNodeByID(nodeID)
	if errNode != nil {
		return "", fmt.Errorf("resolve list IPs node ID: %w", errNode)
	}
	return nodeID, nil
}

// ListIPs returns configured and runtime-discovered IP addresses for the selected node.
func (xs *XylonaService) ListIPs(ctx context.Context, request *connect.Request[xylona.ListIPsRequest]) (*connect.Response[xylona.ListIPsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	nodeID, errNodeID := xs.resolveListIPsNodeID(request.Msg.GetNodeId())
	if errNodeID != nil {
		if errors.Is(errNodeID, sql.ErrNoRows) {
			return nil, invalidArg("invalid node")
		}
		return nil, connect.NewError(connect.CodeInternal, errNodeID)
	}

	storedIPs, errIPs := xs.db.GetIPsByNodeID(nodeID)
	if errIPs != nil {
		return nil, connect.NewError(connect.CodeInternal, errIPs)
	}

	runtimeIPs := xs.listRuntimeIPs(ctx, nodeID)
	ipProtos := mergeNodeIPs(storedIPs, runtimeIPs, nodeID)

	response := &connect.Response[xylona.ListIPsResponse]{Msg: &xylona.ListIPsResponse{Ips: ipProtos}}
	return response, nil
}

// AddIP creates a new configured IP address.
func (xs *XylonaService) AddIP(_ context.Context, request *connect.Request[xylona.AddIPRequest]) (*connect.Response[xylona.AddIPResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}
	ipProto := request.Msg.GetIp()
	if ipProto == nil {
		return nil, invalidArg("ip is required")
	}
	if net.ParseIP(ipProto.GetAddress()) == nil {
		return nil, invalidArg("invalid IP address format")
	}
	nodeID := strings.TrimSpace(ipProto.GetNodeId())
	if nodeID == "" {
		return nil, invalidArg("node ID is required")
	}
	_, errNode := xs.db.GetNodeByID(nodeID)
	if errNode != nil {
		if errors.Is(errNode, sql.ErrNoRows) {
			return nil, invalidArg("invalid node")
		}
		return nil, connect.NewError(connect.CodeInternal, errNode)
	}

	ipSetter := &models.IPSetter{
		Address:  omit.From(ipProto.GetAddress()),
		Usable:   omit.From(ipProto.GetUsable()),
		External: omit.From(ipProto.GetExternal()),
		NodeID:   omit.From(nodeID),
	}

	_, errInsertIP := xs.db.InsertIP(ipSetter)
	if errInsertIP != nil {
		return nil, connect.NewError(connect.CodeInternal, errInsertIP)
	}

	return &connect.Response[xylona.AddIPResponse]{Msg: &xylona.AddIPResponse{}}, nil
}

// RemoveIP deletes a configured IP address.
func (xs *XylonaService) RemoveIP(_ context.Context, request *connect.Request[xylona.RemoveIPRequest]) (*connect.Response[xylona.RemoveIPResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}
	ipProto := request.Msg.GetIp()
	if ipProto == nil {
		return nil, invalidArg("ip is required")
	}
	address := ipProto.GetAddress()
	if address == "" {
		return nil, invalidArg("ip address is required")
	}
	nodeID := strings.TrimSpace(ipProto.GetNodeId())
	if nodeID != "" {
		_, errNode := xs.db.GetNodeByID(nodeID)
		if errNode != nil {
			if errors.Is(errNode, sql.ErrNoRows) {
				return nil, invalidArg("invalid node")
			}
			return nil, connect.NewError(connect.CodeInternal, errNode)
		}

		_, errGetIP := xs.db.GetIPByNodeIDAndAddress(nodeID, address)
		if errGetIP != nil {
			if errors.Is(errGetIP, sql.ErrNoRows) {
				return nil, notFoundErr()
			}
			return nil, connect.NewError(connect.CodeInternal, errGetIP)
		}

		errDeleteIP := xs.db.DeleteIPByNodeID(nodeID, address)
		if errDeleteIP != nil {
			return nil, connect.NewError(connect.CodeInternal, errDeleteIP)
		}
	} else {
		_, errGetIP := xs.db.GetIPByAddress(address)
		if errGetIP != nil {
			if errors.Is(errGetIP, sql.ErrNoRows) {
				return nil, notFoundErr()
			}
			return nil, connect.NewError(connect.CodeInternal, errGetIP)
		}

		errDeleteIP := xs.db.DeleteIP(address)
		if errDeleteIP != nil {
			return nil, connect.NewError(connect.CodeInternal, errDeleteIP)
		}
	}

	return &connect.Response[xylona.RemoveIPResponse]{Msg: &xylona.RemoveIPResponse{}}, nil
}
