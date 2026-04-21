package helpers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/rdegges/go-ipify"
	"github.com/rs/zerolog/log"
)

var (
	listInterfaceAddrs = net.InterfaceAddrs
	lookupExternalIP   = ipify.GetIp
)

// DeleteAppDirectory removes the application data directory.
func DeleteAppDirectory(path string) error {
	errRemove := os.RemoveAll(path)
	if errRemove != nil {
		return fmt.Errorf("delete app directory %s: %w", path, errRemove)
	}
	return nil
}

// CheckDirectoryExists reports whether the provided directory exists.
func CheckDirectoryExists(path string) error {
	_, errStat := os.Stat(path)
	if errStat != nil {
		return fmt.Errorf("stat directory %s: %w", path, errStat)
	}
	return nil
}

// CreateDirectory creates the provided directory if it does not exist.
func CreateDirectory(path string) error {
	errMkdir := os.MkdirAll(path, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("create directory %s: %w", path, errMkdir)
	}
	return nil
}

// JSONPrettyEncoder returns a JSON encoder configured for pretty-printed output.
func JSONPrettyEncoder(w io.Writer) *json.Encoder {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder
}

// DeletePathIfExists removes a path when it exists and ignores missing files.
func DeletePathIfExists(filePath string) error {
	_, errStat := os.Stat(filePath)
	if os.IsNotExist(errStat) {
		return nil
	}
	if errStat != nil {
		return fmt.Errorf("stat path %s: %w", filePath, errStat)
	}

	errRemove := os.Remove(filePath)
	if errRemove != nil {
		return fmt.Errorf("delete path %s: %w", filePath, errRemove)
	}
	return nil
}

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

// GetIPs returns the set of detected local and public IP addresses for this host.
func GetIPs() ([]net.IP, error) {
	localIPs, errLocalIPs := getInterfaceIPs()
	if errLocalIPs != nil {
		return nil, errLocalIPs
	}

	ipsMap := make(map[string]net.IP, len(localIPs)+1)
	for _, ip := range localIPs {
		if ip == nil {
			continue
		}
		ipsMap[ip.String()] = ip
	}

	externalIPString, exErr := lookupExternalIP()
	if exErr != nil {
		log.Warn().Err(exErr).Msg("Unable to get external IP")
		return localIPs, nil
	}
	externalIP := net.ParseIP(externalIPString)
	if externalIP == nil {
		log.Warn().Str("IP", externalIPString).Msg("Unable to parse external IP")
	} else {
		ipsMap[externalIP.String()] = externalIP
	}
	ips := make([]net.IP, 0, len(ipsMap))
	for _, ip := range ipsMap {
		ips = append(ips, ip)
	}
	return ips, nil
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

// GenerateUniqueID returns a ULID using cryptographically secure entropy.
func GenerateUniqueID() (ulid.ULID, error) {
	id, errGenerate := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if errGenerate != nil {
		return ulid.ULID{}, fmt.Errorf("generate unique ID: %w", errGenerate)
	}
	return id, nil
}

// GenerateRandomString returns a random hexadecimal string of the requested byte length.
func GenerateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, errRead := rand.Read(b)
	if errRead != nil {
		return "", fmt.Errorf("generate random bytes: %w", errRead)
	}
	return hex.EncodeToString(b), nil
}
