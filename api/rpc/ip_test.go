package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestAddIPValidIPv4(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address:  "192.168.1.100",
			Usable:   true,
			External: false,
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
}

func TestAddIPValidIPv6(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address:  "::1",
			Usable:   true,
			External: false,
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

func TestRemoveIPValid(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Add an IP first.
	addRequest := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address:  "10.0.0.1",
			Usable:   true,
			External: true,
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

func TestListIPsReturnsAllIPs(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// The fixture seeds one IP (127.0.0.1). Add another.
	addRequest := connect.NewRequest(&xylona.AddIPRequest{
		Ip: &xylona.IP{
			Address:  "10.10.10.10",
			Usable:   true,
			External: true,
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

	if len(listResponse.Msg.GetIps()) < 2 {
		t.Fatalf("ListIPs() returned %d IPs, want at least 2", len(listResponse.Msg.GetIps()))
	}

	foundSeeded := false
	foundAdded := false
	for _, ip := range listResponse.Msg.GetIps() {
		switch ip.GetAddress() {
		case "127.0.0.1":
			foundSeeded = true
		case "10.10.10.10":
			foundAdded = true
		}
	}
	if !foundSeeded {
		t.Errorf("ListIPs() missing seeded IP 127.0.0.1")
	}
	if !foundAdded {
		t.Errorf("ListIPs() missing added IP 10.10.10.10")
	}
}
