package rpc

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"unsafe"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

type testStreamingHandlerConn struct {
	spec   connect.Spec
	header http.Header
}

func (c *testStreamingHandlerConn) Spec() connect.Spec {
	return c.spec
}

func (c *testStreamingHandlerConn) Peer() connect.Peer {
	return connect.Peer{}
}

func (c *testStreamingHandlerConn) Receive(_ any) error {
	return nil
}

func (c *testStreamingHandlerConn) RequestHeader() http.Header {
	return c.header
}

func (c *testStreamingHandlerConn) Send(_ any) error {
	return nil
}

func (c *testStreamingHandlerConn) ResponseHeader() http.Header {
	return http.Header{}
}

func (c *testStreamingHandlerConn) ResponseTrailer() http.Header {
	return http.Header{}
}

func setUnaryRequestProcedure[T any](t *testing.T, req *connect.Request[T], procedure string) {
	t.Helper()

	specValue := reflect.ValueOf(req).Elem().FieldByName("spec")
	reflect.NewAt(specValue.Type(), unsafe.Pointer(specValue.UnsafeAddr())).Elem().Set(reflect.ValueOf(connect.Spec{
		Procedure: procedure,
	}))
}

func TestSessionAuthInterceptorWrapUnary(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	interceptorAny := NewSessionAuthInterceptor(fixture.conn, fixture.secureCookie)
	interceptor, ok := interceptorAny.(*sessionAuthInterceptor)
	if !ok {
		t.Fatal("NewSessionAuthInterceptor() returned unexpected interceptor type")
	}

	tests := []struct {
		name           string
		procedure      string
		withSession    bool
		wantErr        bool
		wantCode       connect.Code
		wantNextCalled bool
	}{
		{
			name:           "public login procedure is allowed without session",
			procedure:      xylonaconnect.XylonaLoginProcedure,
			wantNextCalled: true,
		},
		{
			name:           "protected endpoint is allowed with a valid session",
			procedure:      xylonaconnect.XylonaListRolesProcedure,
			withSession:    true,
			wantNextCalled: true,
		},
		{
			name:      "protected endpoint rejects missing session",
			procedure: xylonaconnect.XylonaListRolesProcedure,
			wantErr:   true,
			wantCode:  connect.CodeUnauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := connect.NewRequest(&xylona.ListRolesRequest{})
			setUnaryRequestProcedure(t, request, tt.procedure)

			if tt.withSession {
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
				sessionCookies, errGetSession := gatekeeper.GetSessionFromHeader(request.Header())
				if errGetSession != nil {
					t.Fatalf("GetSessionFromHeader() error = %v", errGetSession)
				}
				request.Header().Set(
					"Cookie",
					gatekeeper.SessionIDCookieName+"="+sessionCookies.SessionID+";"+
						gatekeeper.SessionTokenCookieName+"="+sessionCookies.SessionToken,
				)
			}

			nextCalled := false
			next := func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
				nextCalled = true
				return connect.NewResponse(&xylona.ListRolesResponse{}), nil
			}

			response, errWrap := interceptor.WrapUnary(next)(context.Background(), request)
			if tt.wantErr {
				if errWrap == nil {
					t.Fatalf("WrapUnary() error = nil, want code %v", tt.wantCode)
				}
				if connect.CodeOf(errWrap) != tt.wantCode {
					t.Fatalf("WrapUnary() code = %v, want %v", connect.CodeOf(errWrap), tt.wantCode)
				}
				if response != nil {
					t.Fatalf("WrapUnary() response = %#v, want nil", response)
				}
			} else {
				if errWrap != nil {
					t.Fatalf("WrapUnary() error = %v", errWrap)
				}
				if response == nil {
					t.Fatal("WrapUnary() response = nil, want non-nil")
				}
			}

			if nextCalled != tt.wantNextCalled {
				t.Fatalf("WrapUnary() next called = %v, want %v", nextCalled, tt.wantNextCalled)
			}
		})
	}
}

func TestSessionAuthInterceptorWrapStreamingHandler(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	interceptorAny := NewSessionAuthInterceptor(fixture.conn, fixture.secureCookie)
	interceptor, ok := interceptorAny.(*sessionAuthInterceptor)
	if !ok {
		t.Fatal("NewSessionAuthInterceptor() returned unexpected interceptor type")
	}

	tests := []struct {
		name           string
		withSession    bool
		wantErr        bool
		wantCode       connect.Code
		wantNextCalled bool
	}{
		{
			name:           "protected streaming handler is allowed with valid session",
			withSession:    true,
			wantNextCalled: true,
		},
		{
			name:     "protected streaming handler rejects missing session",
			wantErr:  true,
			wantCode: connect.CodeUnauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := connect.NewRequest(&xylona.ListRolesRequest{})
			setUnaryRequestProcedure(t, request, xylonaconnect.XylonaListRolesProcedure)

			header := http.Header{}
			if tt.withSession {
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
				header = request.Header().Clone()
			}

			streamingConn := &testStreamingHandlerConn{
				spec: connect.Spec{
					Procedure: xylonaconnect.XylonaListRolesProcedure,
				},
				header: header,
			}

			nextCalled := false
			next := func(_ context.Context, _ connect.StreamingHandlerConn) error {
				nextCalled = true
				return nil
			}

			errWrap := interceptor.WrapStreamingHandler(next)(context.Background(), streamingConn)
			if tt.wantErr {
				if errWrap == nil {
					t.Fatalf("WrapStreamingHandler() error = nil, want code %v", tt.wantCode)
				}
				if connect.CodeOf(errWrap) != tt.wantCode {
					t.Fatalf("WrapStreamingHandler() code = %v, want %v", connect.CodeOf(errWrap), tt.wantCode)
				}
			} else if errWrap != nil {
				t.Fatalf("WrapStreamingHandler() error = %v", errWrap)
			}

			if nextCalled != tt.wantNextCalled {
				t.Fatalf("WrapStreamingHandler() next called = %v, want %v", nextCalled, tt.wantNextCalled)
			}
		})
	}
}
