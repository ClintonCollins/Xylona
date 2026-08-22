package rpc

import (
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

var publicSessionOptionalProcedures = map[string]struct{}{
	xylonaconnect.XylonaCheckUserAuthenticatedProcedure:        {},
	xylonaconnect.XylonaLoginProcedure:                         {},
	xylonaconnect.XylonaLogoutProcedure:                        {},
	xylonaconnect.XylonaGetPublicPalworldMapProcedure:          {},
	xylonaconnect.XylonaGetPublicSevenDaysToDieMapProcedure:    {},
	xylonaconnect.XylonaGetPublicMinecraftMapProcedure:         {},
	xylonaconnect.XylonaResolvePublicGameServerMapProcedure:    {},
	xylonaconnect.XylonaGetPublicGameServerStatusPageProcedure: {},
}

type sessionAuthInterceptor struct {
	db           *db.Connection
	secureCookie *securecookie.SecureCookie
}

// NewSessionAuthInterceptor enforces session authentication for protected RPC procedures.
func NewSessionAuthInterceptor(database *db.Connection, secureCookie *securecookie.SecureCookie) connect.Interceptor {
	return &sessionAuthInterceptor{
		db:           database,
		secureCookie: secureCookie,
	}
}

// NewUnaryTimeoutInterceptor applies a default timeout to unary RPC handlers.
func NewUnaryTimeoutInterceptor(timeout time.Duration) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if timeout <= 0 {
				return next(ctx, req)
			}

			deadline, hasDeadline := ctx.Deadline()
			if hasDeadline && time.Until(deadline) <= timeout {
				return next(ctx, req)
			}

			ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			return next(ctxWithTimeout, req)
		}
	})
}

func (i *sessionAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		errAuth := i.authorize(req.Spec().Procedure, req.Header())
		if errAuth != nil {
			return nil, errAuth
		}
		return next(ctx, req)
	}
}

func (i *sessionAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *sessionAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		errAuth := i.authorize(conn.Spec().Procedure, conn.RequestHeader())
		if errAuth != nil {
			return errAuth
		}
		return next(ctx, conn)
	}
}

func (i *sessionAuthInterceptor) authorize(procedure string, header http.Header) error {
	if _, ok := publicSessionOptionalProcedures[procedure]; ok {
		return nil
	}

	sessionCookies, errGetSession := gatekeeper.GetSessionFromHeader(header)
	if errGetSession != nil {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	_, errGetUser := gatekeeper.GetUserFromSession(
		sessionCookies.SessionID,
		sessionCookies.SessionToken,
		i.db,
		i.secureCookie,
	)
	if errGetUser != nil {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	return nil
}
