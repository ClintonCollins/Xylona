// Package rcon implements Source-compatible, Minecraft-compatible, and Rust
// WebSocket RCON transports.
package rcon

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	packetTypeResponseValue  int32 = 0
	packetTypeExecuteCommand int32 = 2
	packetTypeAuthResponse   int32 = 2
	packetTypeAuth           int32 = 3

	defaultTimeout      = 10 * time.Second
	responseIdleTimeout = 250 * time.Millisecond
	maxPacketBytes      = 4 << 20
	maxResponseBytes    = 4 << 20
)

// Protocol identifies the RCON transport and response-framing behavior.
type Protocol int

const (
	// ProtocolSource reads response packets until the server goes idle.
	ProtocolSource Protocol = iota + 1
	// ProtocolMinecraft reads response packets until the server goes idle.
	ProtocolMinecraft
	// ProtocolRustWeb uses Rust's WebSocket JSON RCON transport.
	ProtocolRustWeb
)

// Client executes one authenticated RCON command per TCP connection.
type Client struct {
	Address  string
	Password string
	Protocol Protocol
	Timeout  time.Duration
}

type packet struct {
	id   int32
	kind int32
	body string
}

// Execute authenticates, sends command, and returns the complete response.
func (c Client) Execute(ctx context.Context, command string) (response string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	command = strings.TrimSpace(command)
	if command == "" || strings.ContainsRune(command, '\x00') {
		return "", errors.New("rcon: command is empty or contains a NUL byte")
	}
	if c.Address == "" || c.Password == "" {
		return "", errors.New("rcon: address and password are required")
	}
	if c.Protocol != ProtocolSource && c.Protocol != ProtocolMinecraft && c.Protocol != ProtocolRustWeb {
		return "", errors.New("rcon: unsupported protocol")
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if c.Protocol == ProtocolRustWeb {
		return c.executeRustWeb(ctx, command, timeout)
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, errDial := dialer.DialContext(ctx, "tcp", c.Address)
	if errDial != nil {
		return "", fmt.Errorf("rcon: connect: %w", errDial)
	}
	defer func() {
		errClose := conn.Close()
		if errClose != nil {
			err = errors.Join(err, fmt.Errorf("rcon: close connection: %w", errClose))
		}
	}()

	deadline := time.Now().Add(timeout)
	ctxDeadline, hasContextDeadline := ctx.Deadline()
	if hasContextDeadline && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	errDeadline := conn.SetDeadline(deadline)
	if errDeadline != nil {
		return "", fmt.Errorf("rcon: set deadline: %w", errDeadline)
	}

	errAuth := authenticate(conn, c.Password)
	if errAuth != nil {
		return "", errAuth
	}
	return executeIdleDelimited(ctx, conn, command, deadline)
}

type rustWebRequest struct {
	Identifier int64  `json:"Identifier"`
	Message    string `json:"Message"`
	Name       string `json:"Name"`
}

type rustWebResponse struct {
	Identifier int64  `json:"Identifier"`
	Message    string `json:"Message"`
}

func (c Client) executeRustWeb(ctx context.Context, command string, timeout time.Duration) (response string, err error) {
	endpoint := url.URL{
		Scheme: "ws",
		Host:   c.Address,
		Path:   "/" + c.Password,
	}
	dialer := websocket.Dialer{HandshakeTimeout: timeout}
	connection, httpResponse, errDial := dialer.DialContext(ctx, endpoint.String(), http.Header{})
	if errDial != nil {
		errResponseClose := closeHTTPResponse(httpResponse)
		return "", errors.Join(fmt.Errorf("rcon: connect Rust WebRCON: %w", errDial), errResponseClose)
	}
	defer func() {
		errClose := connection.Close()
		if errClose != nil {
			err = errors.Join(err, fmt.Errorf("rcon: close Rust WebRCON: %w", errClose))
		}
	}()
	connection.SetReadLimit(maxResponseBytes)

	deadline := time.Now().Add(timeout)
	ctxDeadline, hasContextDeadline := ctx.Deadline()
	if hasContextDeadline && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	errWriteDeadline := connection.SetWriteDeadline(deadline)
	if errWriteDeadline != nil {
		return "", fmt.Errorf("rcon: set Rust WebRCON write deadline: %w", errWriteDeadline)
	}
	errWrite := connection.WriteJSON(rustWebRequest{
		Identifier: 1,
		Message:    command,
		Name:       "Xylona",
	})
	if errWrite != nil {
		return "", fmt.Errorf("rcon: send Rust WebRCON command: %w", errWrite)
	}
	errReadDeadline := connection.SetReadDeadline(deadline)
	if errReadDeadline != nil {
		return "", fmt.Errorf("rcon: set Rust WebRCON read deadline: %w", errReadDeadline)
	}
	for {
		var result rustWebResponse
		errRead := connection.ReadJSON(&result)
		if errRead != nil {
			return "", fmt.Errorf("rcon: read Rust WebRCON response: %w", errRead)
		}
		if result.Identifier == 1 {
			return strings.TrimSpace(result.Message), nil
		}
	}
}

func closeHTTPResponse(response *http.Response) error {
	if response == nil || response.Body == nil {
		return nil
	}
	errClose := response.Body.Close()
	if errClose != nil {
		return fmt.Errorf("rcon: close HTTP response: %w", errClose)
	}
	return nil
}

func authenticate(conn net.Conn, password string) error {
	errWrite := writePacket(conn, packet{id: 1, kind: packetTypeAuth, body: password})
	if errWrite != nil {
		return fmt.Errorf("rcon: send authentication: %w", errWrite)
	}
	for range 2 {
		response, errRead := readPacket(conn)
		if errRead != nil {
			return fmt.Errorf("rcon: read authentication response: %w", errRead)
		}
		if response.kind != packetTypeAuthResponse {
			continue
		}
		if response.id == -1 {
			return errors.New("rcon: authentication failed")
		}
		if response.id != 1 {
			return errors.New("rcon: authentication response ID did not match")
		}
		return nil
	}
	return errors.New("rcon: authentication response was not received")
}

func executeIdleDelimited(
	ctx context.Context,
	conn net.Conn,
	command string,
	overallDeadline time.Time,
) (string, error) {
	errCommand := writePacket(conn, packet{id: 2, kind: packetTypeExecuteCommand, body: command})
	if errCommand != nil {
		return "", fmt.Errorf("rcon: send command: %w", errCommand)
	}

	var response strings.Builder
	for {
		idleDeadline := time.Now().Add(responseIdleTimeout)
		if overallDeadline.Before(idleDeadline) {
			idleDeadline = overallDeadline
		}
		errDeadline := conn.SetReadDeadline(idleDeadline)
		if errDeadline != nil {
			return "", fmt.Errorf("rcon: set response deadline: %w", errDeadline)
		}
		responsePacket, errRead := readPacket(conn)
		if errRead != nil {
			var netError net.Error
			if errors.As(errRead, &netError) && netError.Timeout() {
				errContext := ctx.Err()
				if errContext != nil {
					return "", fmt.Errorf("rcon: command context ended: %w", errContext)
				}
				return strings.TrimSpace(response.String()), nil
			}
			if response.Len() > 0 && errors.Is(errRead, io.EOF) {
				return strings.TrimSpace(response.String()), nil
			}
			return "", fmt.Errorf("rcon: read command response: %w", errRead)
		}
		if responsePacket.id == 2 {
			if response.Len()+len(responsePacket.body) > maxResponseBytes {
				return "", errors.New("rcon: command response exceeds size limit")
			}
			response.WriteString(responsePacket.body)
		}
	}
}

func writePacket(w io.Writer, value packet) error {
	if strings.ContainsRune(value.body, '\x00') {
		return errors.New("packet body contains a NUL byte")
	}
	size := 10 + len(value.body)
	if size > maxPacketBytes {
		return errors.New("packet exceeds size limit")
	}
	buffer := make([]byte, size+4)
	binary.LittleEndian.PutUint32(buffer[0:4], uint32(size))
	// RCON defines IDs and packet kinds as signed 32-bit values on the wire;
	// converting them to their bit-equivalent uint32 representation is intentional.
	binary.LittleEndian.PutUint32(buffer[4:8], uint32(value.id))    //nolint:gosec // Preserve signed RCON ID bits, including -1.
	binary.LittleEndian.PutUint32(buffer[8:12], uint32(value.kind)) //nolint:gosec // Preserve the protocol's signed packet-kind bits.
	copy(buffer[12:], value.body)
	_, errWrite := w.Write(buffer)
	if errWrite != nil {
		return fmt.Errorf("write RCON packet: %w", errWrite)
	}
	return nil
}

func readPacket(r io.Reader) (packet, error) {
	var size int32
	errSize := binary.Read(r, binary.LittleEndian, &size)
	if errSize != nil {
		return packet{}, fmt.Errorf("read RCON packet size: %w", errSize)
	}
	if size < 10 || size > maxPacketBytes {
		return packet{}, fmt.Errorf("invalid packet size %d", size)
	}
	payload := make([]byte, size)
	_, errRead := io.ReadFull(r, payload)
	if errRead != nil {
		return packet{}, fmt.Errorf("read RCON packet payload: %w", errRead)
	}
	if payload[len(payload)-1] != 0 || payload[len(payload)-2] != 0 {
		return packet{}, errors.New("packet is missing terminators")
	}
	return packet{
		// The protocol specifies signed int32 fields; these conversions restore
		// the exact wire bits, including the -1 authentication-failure ID.
		id:   int32(binary.LittleEndian.Uint32(payload[0:4])), //nolint:gosec // Restore the signed RCON ID.
		kind: int32(binary.LittleEndian.Uint32(payload[4:8])), //nolint:gosec // Restore the signed RCON packet kind.
		body: string(payload[8 : len(payload)-2]),
	}, nil
}
