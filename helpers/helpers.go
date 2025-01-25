package helpers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/rdegges/go-ipify"
	"github.com/rs/zerolog/log"
)

func CreateAppDirectory(path string) error {
	return os.MkdirAll(path, 0751)
}

func DeleteAppDirectory(path string) error {
	return os.RemoveAll(path)
}

func CheckAppDirectoryExists(path string) error {
	_, err := os.Stat(path)
	return err
}

func CreateOperatingDirectory(path string) error {
	return os.MkdirAll(path, 0751)
}

func CheckOperatingDirectoryExists(path string) error {
	_, err := os.Stat(path)
	return err
}

func CheckDirectoryExists(path string) error {
	_, err := os.Stat(path)
	return err
}

func CreateDirectory(path string) error {
	return os.MkdirAll(path, 0751)
}

func JsonPrettyEncoder(w io.Writer) *json.Encoder {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder
}

func DeletePathIfExists(filePath string) error {
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil
	}
	return os.Remove(filePath)
}

func GetIPs() ([]net.IP, error) {
	networkInterfaces, err := net.InterfaceAddrs()
	if err != nil {
		log.Error().Err(err).Msg("Unable to get network interfaces")
		return nil, err
	}
	ipsMap := make(map[string]net.IP)
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
	externalIPString, exErr := ipify.GetIp()
	if exErr != nil {
		var ips []net.IP
		for _, ip := range ipsMap {
			ips = append(ips, ip)
		}
		log.Warn().Err(exErr).Msg("Unable to get external IP")
		return ips, nil
	}
	externalIP := net.ParseIP(externalIPString)
	if externalIP == nil {
		log.Warn().Str("IP", externalIPString).Msg("Unable to parse external IP")
	}
	ipsMap[externalIP.String()] = externalIP
	var ips []net.IP
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
	return http.DefaultTransport.RoundTrip(req)
}

func GetXylonaHTTPClient() *http.Client {
	return httpClient
}

func GenerateUniqueID() (ulid.ULID, error) {
	return ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
}

func GenerateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
