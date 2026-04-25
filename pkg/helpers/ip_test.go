package helpers

import (
	"net"
	"testing"
)

func TestGetBindableIPsReturnsOnlyBindableInterfaceAddresses(t *testing.T) {
	originalListInterfaceAddrs := listInterfaceAddrs
	t.Cleanup(func() {
		listInterfaceAddrs = originalListInterfaceAddrs
	})

	listInterfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("10.0.0.10"), Mask: net.CIDRMask(24, 32)},
			&net.IPNet{IP: net.ParseIP("fd00::10"), Mask: net.CIDRMask(64, 128)},
			&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
			&net.IPNet{IP: net.ParseIP("169.254.10.1"), Mask: net.CIDRMask(16, 32)},
		}, nil
	}
	ips, errGetIPs := GetBindableIPs()
	if errGetIPs != nil {
		t.Fatalf("GetBindableIPs() error = %v", errGetIPs)
	}

	got := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		got[ip.String()] = struct{}{}
	}

	if len(got) != 2 {
		t.Fatalf("GetBindableIPs() returned %d IPs, want 2", len(got))
	}
	if _, ok := got["10.0.0.10"]; !ok {
		t.Errorf("GetBindableIPs() missing IPv4 interface address")
	}
	if _, ok := got["fd00::10"]; !ok {
		t.Errorf("GetBindableIPs() missing IPv6 interface address")
	}
	if _, ok := got["127.0.0.1"]; ok {
		t.Errorf("GetBindableIPs() included loopback address")
	}
	if _, ok := got["169.254.10.1"]; ok {
		t.Errorf("GetBindableIPs() included link-local address")
	}
}
