package actions

import (
	"errors"
	"fmt"
)

// StartFailureKind classifies start failures for callers that need to decide
// whether retrying is useful.
type StartFailureKind string

const (
	// StartFailureConfiguration means a user-fixable setting or node capability blocked the start.
	StartFailureConfiguration StartFailureKind = "configuration"
	// StartFailureUnavailable means the target node was not reachable.
	StartFailureUnavailable StartFailureKind = "unavailable"
	// StartFailureInternal means Xylona failed after configuration checks passed.
	StartFailureInternal StartFailureKind = "internal"
)

// StartGameServerResult describes a successfully issued start request.
type StartGameServerResult struct {
	Started bool
}

// StartGameServerError wraps a failed start with a small retry/transport hint.
type StartGameServerError struct {
	Kind    StartFailureKind
	Message string
	Err     error
}

func (err *StartGameServerError) Error() string {
	if err == nil {
		return ""
	}
	if err.Err == nil {
		return err.Message
	}
	if err.Message == "" {
		return err.Err.Error()
	}
	return fmt.Sprintf("%s: %v", err.Message, err.Err)
}

func (err *StartGameServerError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func startConfigurationError(message string, err error) error {
	return &StartGameServerError{Kind: StartFailureConfiguration, Message: message, Err: err}
}

func startUnavailableError(message string, err error) error {
	return &StartGameServerError{Kind: StartFailureUnavailable, Message: message, Err: err}
}

func startInternalError(message string, err error) error {
	return &StartGameServerError{Kind: StartFailureInternal, Message: message, Err: err}
}

func startFailureKind(err error) (StartFailureKind, bool) {
	if err == nil {
		return "", false
	}
	var startErr *StartGameServerError
	if errors.As(err, &startErr) && startErr != nil {
		return startErr.Kind, true
	}
	return "", false
}

// IsStartConfigurationError reports whether a start failed for a user-fixable
// configuration or capability reason. These failures should not be retried
// automatically.
func IsStartConfigurationError(err error) bool {
	kind, ok := startFailureKind(err)
	return ok && kind == StartFailureConfiguration
}

// IsStartUnavailableError reports whether the target node could not be reached.
func IsStartUnavailableError(err error) bool {
	kind, ok := startFailureKind(err)
	return ok && kind == StartFailureUnavailable
}
