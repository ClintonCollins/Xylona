package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestAddIPValidIPv4(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address:  "192.168.1.100",
			Usable:   true,
			External: false,
			NodeId:   "node-local",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errAddIP := fixture.service.AddIP(context.Background(), request)
	if errAddIP != nil {
		t.Fatalf("AddIP() error = %v", errAddIP)
	}

	ip, errGetIP := fixture.conn.GetIPByAddress("192.168.1.100")
	if errGetIP != nil {
		t.Fatalf("GetIPByAddress() error = %v", errGetIP)
	}
	if ip.Address != "192.168.1.100" {
		t.Errorf("IP.Address = %q, want %q", ip.Address, "192.168.1.100")
	}
	if !ip.Usable {
		t.Errorf("IP.Usable = %v, want %v", ip.Usable, true)
	}
	if ip.External {
		t.Errorf("IP.External = %v, want %v", ip.External, false)
	}
	if ip.NodeID != "node-local" {
		t.Errorf("IP.NodeID = %q, want %q", ip.NodeID, "node-local")
	}
}

func TestAddIPValidIPv6(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address:  "::1",
			Usable:   true,
			External: false,
			NodeId:   "node-local",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errAddIP := fixture.service.AddIP(context.Background(), request)
	if errAddIP != nil {
		t.Fatalf("AddIP() error = %v", errAddIP)
	}

	ip, errGetIP := fixture.conn.GetIPByAddress("::1")
	if errGetIP != nil {
		t.Fatalf("GetIPByAddress() error = %v", errGetIP)
	}
	if ip.Address != "::1" {
		t.Errorf("IP.Address = %q, want %q", ip.Address, "::1")
	}
}

func TestAddIPInvalidFormat(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	tests := []struct {
		name    string
		address string
	}{
		{name: "garbage string", address: "not-an-ip"},
		{name: "empty string", address: ""},
		{name: "partial address", address: "192.168.1"},
		{name: "out of range octet", address: "999.999.999.999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := connect.NewRequest(&xylona.AddIPRequest{
				Ip: &xylona.IP{
					Address: tt.address,
					Usable:  true,
					NodeId:  "node-local",
				},
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

			_, errAddIP := fixture.service.AddIP(context.Background(), request)
			if errAddIP == nil {
				t.Fatalf("AddIP(%q) expected error, got nil", tt.address)
			}
			if connect.CodeOf(errAddIP) != connect.CodeInvalidArgument {
				t.Errorf("AddIP(%q) code = %v, want %v", tt.address, connect.CodeOf(errAddIP), connect.CodeInvalidArgument)
			}
		})
	}
}

func TestAddIPNilIP(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.AddIPRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errAddIP := fixture.service.AddIP(context.Background(), request)
	if errAddIP == nil {
		t.Fatalf("AddIP(nil ip) expected error, got nil")
	}
	if connect.CodeOf(errAddIP) != connect.CodeInvalidArgument {
		t.Errorf("AddIP(nil ip) code = %v, want %v", connect.CodeOf(errAddIP), connect.CodeInvalidArgument)
	}
}

func TestAddIPRequiresNodeID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address: "192.168.1.200",
			Usable:  true,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errAddIP := fixture.service.AddIP(context.Background(), request)
	if errAddIP == nil {
		t.Fatalf("AddIP() expected error, got nil")
	}
	if connect.CodeOf(errAddIP) != connect.CodeInvalidArgument {
		t.Errorf("AddIP() code = %v, want %v", connect.CodeOf(errAddIP), connect.CodeInvalidArgument)
	}
}

func TestAddIPRejectsUnknownNodeID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address: "192.168.1.201",
			Usable:  true,
			NodeId:  "node-missing",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errAddIP := fixture.service.AddIP(context.Background(), request)
	if errAddIP == nil {
		t.Fatalf("AddIP() expected error, got nil")
	}
	if connect.CodeOf(errAddIP) != connect.CodeInvalidArgument {
		t.Errorf("AddIP() code = %v, want %v", connect.CodeOf(errAddIP), connect.CodeInvalidArgument)
	}
}

func TestRemoveIPValid(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Add an IP first.
	addRequest := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address:  "10.0.0.1",
			Usable:   true,
			External: true,
			NodeId:   "node-local",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, addRequest, "user-admin")

	_, errAddIP := fixture.service.AddIP(context.Background(), addRequest)
	if errAddIP != nil {
		t.Fatalf("AddIP() setup error = %v", errAddIP)
	}

	// Remove it.
	removeRequest := connect.NewRequest(&xylona.RemoveIPRequest{
		Ip: &xylona.IP{
			Address: "10.0.0.1",
			NodeId:  "node-local",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, removeRequest, "user-admin")

	_, errRemoveIP := fixture.service.RemoveIP(context.Background(), removeRequest)
	if errRemoveIP != nil {
		t.Fatalf("RemoveIP() error = %v", errRemoveIP)
	}

	// Verify it's gone.
	_, errGetIP := fixture.conn.GetIPByAddress("10.0.0.1")
	if errGetIP == nil {
		t.Errorf("GetIPByAddress() expected error after deletion, got nil")
	}
}

func TestRemoveIPNonExistent(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.RemoveIPRequest{
		Ip: &xylona.IP{
			Address: "172.16.0.99",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errRemoveIP := fixture.service.RemoveIP(context.Background(), request)
	if errRemoveIP == nil {
		t.Fatalf("RemoveIP(non-existent) expected error, got nil")
	}
	if connect.CodeOf(errRemoveIP) != connect.CodeNotFound {
		t.Errorf("RemoveIP(non-existent) code = %v, want %v", connect.CodeOf(errRemoveIP), connect.CodeNotFound)
	}
}

func TestListIPsDefaultsToLocalNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	fixture.service.nodeRegistry = noderegistry.New("node-local", &nodeclient.FakeNodeClient{
		NodeID: "node-local",
		BindableIPsResult: []node.BindableIP{
			{Address: "127.0.0.1", Usable: true},
		},
	})

	addRequest := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address:  "192.168.1.150",
			Usable:   true,
			External: true,
			NodeId:   "node-local",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, addRequest, "user-admin")

	_, errAddIP := fixture.service.AddIP(context.Background(), addRequest)
	if errAddIP != nil {
		t.Fatalf("AddIP() setup error = %v", errAddIP)
	}

	listRequest := connect.NewRequest(&xylona.ListIPsRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listRequest, "user-admin")

	listResponse, errListIPs := fixture.service.ListIPs(context.Background(), listRequest)
	if errListIPs != nil {
		t.Fatalf("ListIPs() error = %v", errListIPs)
	}

	if len(listResponse.Msg.GetIps()) != 2 {
		t.Fatalf("ListIPs() returned %d IPs, want 2", len(listResponse.Msg.GetIps()))
	}

	foundSeeded := false
	foundAdded := false
	for _, ip := range listResponse.Msg.GetIps() {
		switch ip.GetAddress() {
		case "127.0.0.1":
			foundSeeded = true
			if ip.GetNodeId() != "node-local" {
				t.Errorf("seeded IP node_id = %q, want %q", ip.GetNodeId(), "node-local")
			}
		case "192.168.1.150":
			foundAdded = true
			if ip.GetNodeId() != "node-local" {
				t.Errorf("added IP node_id = %q, want %q", ip.GetNodeId(), "node-local")
			}
		case "127.0.0.2":
			t.Errorf("ListIPs() returned remote IP %q for default local-node request", ip.GetAddress())
		}
	}
	if !foundSeeded {
		t.Errorf("ListIPs() missing seeded IP 127.0.0.1")
	}
	if !foundAdded {
		t.Errorf("ListIPs() missing added IP 192.168.1.150")
	}
}

func TestListIPsReturnsRequestedRemoteNodeIPsAndDedupesDetectedIPs(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)

	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{
		NodeID: "node-local",
	})
	registry.Register(&nodeclient.FakeNodeClient{
		NodeID: "node-alt",
		BindableIPsResult: []node.BindableIP{
			{Address: "127.0.0.2", Usable: true},
			{Address: "203.0.113.42", Usable: true, External: true},
		},
	})
	fixture.service.nodeRegistry = registry

	listRequest := connect.NewRequest(&xylona.ListIPsRequest{NodeId: "node-alt"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listRequest, "user-admin")

	listResponse, errListIPs := fixture.service.ListIPs(context.Background(), listRequest)
	if errListIPs != nil {
		t.Fatalf("ListIPs() error = %v", errListIPs)
	}

	if len(listResponse.Msg.GetIps()) != 2 {
		t.Fatalf("ListIPs() returned %d IPs, want 2", len(listResponse.Msg.GetIps()))
	}

	foundManual := false
	foundDetected := false
	for _, ip := range listResponse.Msg.GetIps() {
		if ip.GetNodeId() != "node-alt" {
			t.Errorf("ListIPs() node_id = %q, want %q", ip.GetNodeId(), "node-alt")
		}
		switch ip.GetAddress() {
		case "127.0.0.2":
			foundManual = true
		case "203.0.113.42":
			foundDetected = true
		case "127.0.0.1":
			t.Errorf("ListIPs() returned local-node IP %q for remote request", ip.GetAddress())
		}
	}
	if !foundManual {
		t.Errorf("ListIPs() missing manual remote IP 127.0.0.2")
	}
	if !foundDetected {
		t.Errorf("ListIPs() missing detected remote IP 203.0.113.42")
	}
}
