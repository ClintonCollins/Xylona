// Package helpers contains small reusable utilities shared across packages.
package helpers

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

var (
	listInterfaceAddrs = net.InterfaceAddrs
)

func getInterfaceIPs() ([]net.IP, error) {
	networkInterfaces, errInterfaces := listInterfaceAddrs()
	if errInterfaces != nil {
		log.Error().Err(errInterfaces).Msg("Unable to get network interfaces")
		return nil, fmt.Errorf("list network interfaces: %w", errInterfaces)
	}
	ipsMap := make(map[string]net.IP, len(networkInterfaces))
	for _, addresses := range networkInterfaces {
		ipNet, ok := addresses.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsLoopback() {
			ipsMap[ip.String()] = ip
		}
	}
	ips := make([]net.IP, 0, len(ipsMap))
	for _, ip := range ipsMap {
		ips = append(ips, ip)
	}
	return ips, nil
}

// GetBindableIPs returns the set of interface-local IP addresses this host can bind to.
func GetBindableIPs() ([]net.IP, error) {
	return getInterfaceIPs()
}

type xylonaTransport struct{}

var (
	httpClient = &http.Client{
		Timeout:   time.Second * 15,
		Transport: xylonaTransport{},
	}
)

func (x xylonaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("User-Agent", "Xylona/0.1 (https://github.com/ClintonCollins/Xylona)")
	response, errRoundTrip := http.DefaultTransport.RoundTrip(req)
	if errRoundTrip != nil {
		return nil, fmt.Errorf("perform HTTP request: %w", errRoundTrip)
	}
	return response, nil
}

// GetXylonaHTTPClient returns the shared outbound HTTP client used by helper code.
func GetXylonaHTTPClient() *http.Client {
	return httpClient
}

// GenerateUniqueID returns a UUID string.
func GenerateUniqueID() string {
	return uuid.NewString()
}
