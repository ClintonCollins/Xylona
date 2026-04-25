package helpers

import (
	"reflect"
	"time"
)

// GameServerVersionStateProvider supplies version state data for game server
// protobuf conversion.
type GameServerVersionStateProvider interface {
	GameServerVersionState(gameServerID string) GameServerVersionState
}

// GameServerVersionStatus is the converter-local status for game server
// version tracking.
type GameServerVersionStatus int

const (
	// GameServerVersionStatusNoTracker indicates no version tracker is available.
	GameServerVersionStatusNoTracker GameServerVersionStatus = iota
	// GameServerVersionStatusUnchecked indicates a tracker exists but has not checked yet.
	GameServerVersionStatusUnchecked
	// GameServerVersionStatusChecking indicates a version check is in progress.
	GameServerVersionStatusChecking
	// GameServerVersionStatusChecked indicates a version check completed successfully.
	GameServerVersionStatusChecked
	// GameServerVersionStatusError indicates the last version check failed.
	GameServerVersionStatusError
)

// GameServerVersionState is the subset of version tracker state needed by the
// game server protobuf converter.
type GameServerVersionState struct {
	Status                GameServerVersionStatus
	InstalledVersion      string
	LatestVersion         string
	UpdateAvailable       bool
	LastCheckTime         time.Time
	TrackerType           string
	InstalledVersionLabel string
	LatestVersionLabel    string
	InstalledBranch       string
	LatestBranch          string
}

func hasGameServerVersionStateProvider(versionStates GameServerVersionStateProvider) bool {
	if versionStates == nil {
		return false
	}

	value := reflect.ValueOf(versionStates)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}
