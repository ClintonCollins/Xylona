package rpc

import (
	"database/sql"
	"errors"

	"connectrpc.com/connect"
)

// dbLookup maps database lookup errors to connect error codes.
// sql.ErrNoRows returns CodeNotFound; all other errors return CodeInternal.
func dbLookup(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return notFoundErr()
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

// dbLookupMsg maps database lookup errors to connect error codes with a custom
// internal error message. sql.ErrNoRows returns CodeNotFound; all other errors
// return CodeInternal with msg.
func dbLookupMsg(err error, msg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return notFoundErr()
	}
	return connect.NewError(connect.CodeInternal, errors.New(msg))
}

// internalErr returns a CodeInternal error with a generic message.
func internalErr() error {
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

// internalErrf returns a CodeInternal error with a custom message.
func internalErrf(msg string) error {
	return connect.NewError(connect.CodeInternal, errors.New(msg))
}

// notFoundErr returns a CodeNotFound error.
func notFoundErr() error {
	return connect.NewError(connect.CodeNotFound, errors.New("not found"))
}

// permissionDenied returns a CodePermissionDenied error with a custom message.
func permissionDenied(msg string) error {
	return connect.NewError(connect.CodePermissionDenied, errors.New(msg))
}

// unauthenticated returns a CodeUnauthenticated error.
func unauthenticated() error {
	return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

// invalidArg returns a CodeInvalidArgument error with a custom message.
func invalidArg(msg string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}
