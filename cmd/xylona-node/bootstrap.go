package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/nodetls"
)

// bootstrapRequest mirrors api/rpc.NodeBootstrapRequest. We copy the shape
// here instead of importing to keep the xylona-node binary free of any
// dependency on the controller-side code.
type bootstrapRequest struct {
	JoinToken       string `json:"join_token"`
	NodeName        string `json:"node_name"`
	ListenURL       string `json:"listen_url"`
	CertPEM         string `json:"cert_pem"`
	CertFingerprint string `json:"cert_fingerprint"`
}

// bootstrapResponse mirrors api/rpc.NodeBootstrapResponse.
type bootstrapResponse struct {
	NodeID       string `json:"node_id"`
	SharedSecret string `json:"shared_secret"`
}

var (
	lookupAdvertiseLocalIP = detectAdvertiseLocalIP
	readHostname           = os.Hostname
)

// performBootstrap runs the one-shot pairing exchange with the controller
// and writes a complete identity file to dataDir. It is safe to call
// concurrently with a shutdown signal — the ctx is threaded through to the
// HTTP request.
func performBootstrap(ctx context.Context, cfg *cliConfig, dataDir string) (*nodeIdentity, error) {
	controllerURL := strings.TrimRight(strings.TrimSpace(cfg.controllerURL), "/")
	if controllerURL == "" {
		return nil, errors.New("--controller-url is required to bootstrap a new node")
	}
	if _, errParse := url.Parse(controllerURL); errParse != nil {
		return nil, fmt.Errorf("invalid --controller-url: %w", errParse)
	}

	subjectCN := resolveNodeName(cfg)
	certPEM, keyPEM, fingerprint, errCert := nodetls.GenerateSelfSigned(ctx, subjectCN)
	if errCert != nil {
		return nil, fmt.Errorf("generate node TLS identity: %w", errCert)
	}

	listenURL := resolveReportedListenURL(cfg)

	reqBody, errMarshal := json.Marshal(&bootstrapRequest{
		JoinToken:       strings.TrimSpace(cfg.joinToken),
		NodeName:        resolveNodeName(cfg),
		ListenURL:       listenURL,
		CertPEM:         string(certPEM),
		CertFingerprint: fingerprint,
	})
	if errMarshal != nil {
		return nil, fmt.Errorf("marshal bootstrap request: %w", errMarshal)
	}

	endpoint := controllerURL + "/api/node/bootstrap"
	httpReq, errReq := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if errReq != nil {
		return nil, fmt.Errorf("build bootstrap request: %w", errReq)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	if cfg.skipInsecureTLS {
		log.Warn().
			Str("controller_url", controllerURL).
			Msg("bootstrap: --skip-insecure-tls set; controller certificate will NOT be verified for the pairing request")
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // G402: intentional; gated behind --skip-insecure-tls for one-shot bootstrap.
		}
	}
	httpResp, errDo := client.Do(httpReq)
	if errDo != nil {
		return nil, fmt.Errorf("send bootstrap request: %w", errDo)
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	respBody, errRead := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if errRead != nil {
		return nil, fmt.Errorf("read bootstrap response: %w", errRead)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bootstrap failed: %s: %s", httpResp.Status, strings.TrimSpace(string(respBody)))
	}

	resp := &bootstrapResponse{}
	errDecode := json.Unmarshal(respBody, resp)
	if errDecode != nil {
		return nil, fmt.Errorf("decode bootstrap response: %w", errDecode)
	}
	if strings.TrimSpace(resp.NodeID) == "" || strings.TrimSpace(resp.SharedSecret) == "" {
		return nil, errors.New("bootstrap response missing node_id or shared_secret")
	}

	identity := &nodeIdentity{
		NodeID:        resp.NodeID,
		CertPEM:       string(certPEM),
		KeyPEM:        string(keyPEM),
		Fingerprint:   fingerprint,
		ControllerURL: controllerURL,
		SharedSecret:  resp.SharedSecret,
		SchemaVersion: currentIdentitySchemaVersion,
	}
	errSave := saveIdentity(dataDir, identity)
	if errSave != nil {
		return nil, fmt.Errorf("persist node identity: %w", errSave)
	}

	log.Info().
		Str("node_id", identity.NodeID).
		Str("controller_url", identity.ControllerURL).
		Str("listen_url", listenURL).
		Msg("bootstrap complete — identity persisted")

	return identity, nil
}

// resolveReportedListenURL builds the URL the controller will use to dial
// this node. It prefers --advertise-url when set; otherwise it infers the
// advertised host from --listen. Wildcard listeners first try the local IP
// routed toward the controller, then fall back to the OS hostname.
func resolveReportedListenURL(cfg *cliConfig) string {
	if strings.TrimSpace(cfg.advertiseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(cfg.advertiseURL), "/")
	}

	host, port := splitListenHostPort(strings.TrimSpace(cfg.listen))
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = strings.TrimSpace(lookupAdvertiseLocalIP(cfg.controllerURL))
		if host == "" {
			fallbackHost, errHostname := readHostname()
			if errHostname == nil && strings.TrimSpace(fallbackHost) != "" {
				host = strings.TrimSpace(fallbackHost)
			} else {
				host = "localhost"
			}
		}
	}
	if port == "" {
		return "https://" + host
	}
	return "https://" + net.JoinHostPort(host, port)
}

func splitListenHostPort(listen string) (string, string) {
	if listen == "" {
		return "", ""
	}
	if port, ok := strings.CutPrefix(listen, ":"); ok {
		return "", port
	}
	if !strings.Contains(listen, ":") {
		return "", listen
	}

	host, port, errSplit := net.SplitHostPort(listen)
	if errSplit == nil {
		return host, port
	}
	return listen, ""
}

func detectAdvertiseLocalIP(controllerURL string) string {
	parsedURL, errParse := url.Parse(strings.TrimSpace(controllerURL))
	if errParse != nil {
		return ""
	}

	controllerHost := parsedURL.Hostname()
	if controllerHost == "" {
		return ""
	}

	controllerPort := parsedURL.Port()
	if controllerPort == "" {
		if strings.EqualFold(parsedURL.Scheme, "http") {
			controllerPort = "80"
		} else {
			controllerPort = "443"
		}
	}

	dialer := &net.Dialer{}
	conn, errDial := dialer.DialContext(context.Background(), "udp", net.JoinHostPort(controllerHost, controllerPort))
	if errDial != nil {
		return ""
	}
	defer func() {
		errClose := conn.Close()
		if errClose != nil {
			log.Debug().Err(errClose).Msg("bootstrap: close advertise IP probe connection")
		}
	}()

	udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || udpAddr.IP == nil {
		return ""
	}

	localIP := udpAddr.IP
	if localIP.IsLoopback() || localIP.IsLinkLocalUnicast() || localIP.IsLinkLocalMulticast() || localIP.IsUnspecified() {
		return ""
	}
	return localIP.String()
}

// resolveNodeName picks a display name for the node row the controller
// creates. Falls back to the OS hostname so the UI has something sensible
// when no explicit name is supplied.
func resolveNodeName(cfg *cliConfig) string {
	trimmed := strings.TrimSpace(cfg.nodeName)
	if trimmed != "" {
		return trimmed
	}
	hostName, errHost := readHostname()
	if errHost == nil && strings.TrimSpace(hostName) != "" {
		return hostName
	}
	return "Remote Node"
}
