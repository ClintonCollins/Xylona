package rpc

import (
	"errors"

	"connectrpc.com/connect"
)

func wrapRemoteRPCError(err error, unavailableMessage string) error {
	if err == nil {
		return nil
	}

	if code := connect.CodeOf(err); code != connect.CodeUnknown {
		return connect.NewError(code, errors.New(err.Error()))
	}

	return connect.NewError(connect.CodeUnavailable, errors.New(unavailableMessage))
}
