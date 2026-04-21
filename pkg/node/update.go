package node

import "io"

// UpdateCapabilities describes whether a node can accept a self-update.
type UpdateCapabilities struct {
	Supported               bool
	Reason                  string
	Component               string
	CurrentVersion          string
	OS                      string
	Architecture            string
	ProtocolVersion         int64
	ServiceManagerSupported bool
	InstallPathWritable     bool
	InstallPath             string
}

// StageSelfUpdateRequest carries a verified update artifact to a node.
type StageSelfUpdateRequest struct {
	Component      string
	TargetVersion  string
	OS             string
	Architecture   string
	ExpectedSize   int64
	ExpectedSHA256 string
	Reader         io.Reader
}

// StageSelfUpdateResult is returned after a node has persisted a staged artifact.
type StageSelfUpdateResult struct {
	StageID      string
	BytesWritten int64
	SHA256       string
}

// ApplySelfUpdateRequest asks a node to hand off a staged artifact to its updater.
type ApplySelfUpdateRequest struct {
	StageID        string
	TargetVersion  string
	ExpectedSHA256 string
}

// ApplySelfUpdateResult reports whether the handoff was accepted.
type ApplySelfUpdateResult struct {
	Accepted bool
	Message  string
}
