package supervisor

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
)

func TestConnectTelnetAuthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		clientPassword string
		serverResult   string
		wantErr        string
	}{
		{
			name:           "terminated password waits for successful login",
			clientPassword: "correct-password",
			serverResult:   telnetLogonSuccessful,
		},
		{
			name:           "rejected password fails authentication",
			clientPassword: "incorrect-password",
			serverResult:   telnetLogonFailed + ".",
			wantErr:        "telnet authentication rejected",
		},
		{
			name:           "connection closed before result fails authentication",
			clientPassword: "correct-password",
			wantErr:        "read telnet authentication result",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			listenConfig := &net.ListenConfig{}
			listener, errListen := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
			if errListen != nil {
				t.Fatalf("listen: %v", errListen)
			}
			t.Cleanup(func() {
				errClose := listener.Close()
				if errClose != nil && !errors.Is(errClose, net.ErrClosed) {
					t.Errorf("close listener: %v", errClose)
				}
			})
			tcpAddress, ok := listener.Addr().(*net.TCPAddr)
			if !ok {
				t.Fatalf("listener address type = %T", listener.Addr())
			}

			serverResult := make(chan error, 1)
			go func() {
				serverResult <- serveTelnetAuthenticationFixture(listener, tc.clientPassword+"\n", tc.serverResult)
			}()

			connection, errConnect := connectTelnet(&TelnetCredentials{
				Port:     tcpAddress.Port,
				Password: tc.clientPassword,
			})
			if connection != nil {
				errClose := connection.Close()
				if errClose != nil {
					t.Errorf("close client connection: %v", errClose)
				}
			}
			if errServer := <-serverResult; errServer != nil {
				t.Fatalf("telnet fixture: %v", errServer)
			}
			if tc.wantErr == "" {
				if errConnect != nil {
					t.Fatalf("connectTelnet() error = %v", errConnect)
				}
				return
			}
			if errConnect == nil || !strings.Contains(errConnect.Error(), tc.wantErr) {
				t.Fatalf("connectTelnet() error = %v, want containing %q", errConnect, tc.wantErr)
			}
		})
	}
}

func serveTelnetAuthenticationFixture(listener net.Listener, wantPasswordLine string, result string) (returnErr error) {
	connection, errAccept := listener.Accept()
	if errAccept != nil {
		return fmt.Errorf("accept: %w", errAccept)
	}
	defer func() {
		errClose := connection.Close()
		if errClose != nil && !errors.Is(errClose, net.ErrClosed) {
			returnErr = errors.Join(returnErr, fmt.Errorf("close connection: %w", errClose))
		}
	}()

	_, errPrompt := io.WriteString(connection, telnetLoginPrompt)
	if errPrompt != nil {
		return fmt.Errorf("write password prompt: %w", errPrompt)
	}
	passwordLine, errPassword := bufio.NewReader(connection).ReadString('\n')
	if errPassword != nil {
		return fmt.Errorf("read password line: %w", errPassword)
	}
	if passwordLine != wantPasswordLine {
		return fmt.Errorf("password line = %q, want %q", passwordLine, wantPasswordLine)
	}
	if result == "" {
		return nil
	}
	_, errResult := io.WriteString(connection, result)
	if errResult != nil {
		return fmt.Errorf("write authentication result: %w", errResult)
	}
	return nil
}
