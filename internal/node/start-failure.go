package node

import "github.com/ClintonCollins/Xylona/internal/diagnosis"

// StartFailureError preserves classified launch evidence across transport
// boundaries while retaining the original error for local error checks.
type StartFailureError struct {
	Report diagnosis.Report
	Err    error
}

func (e *StartFailureError) Error() string {
	return e.Report.Error
}

func (e *StartFailureError) Unwrap() error {
	return e.Err
}
