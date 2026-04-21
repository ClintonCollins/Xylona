// Package version exposes Xylona build and protocol version information.
package version

import (
	"runtime/debug"
	"strings"
)

var softwareVersionStamp string

// SoftwareVersion is the application version reported to users and peers.
var SoftwareVersion = resolveSoftwareVersion(softwareVersionStamp, readBuildInfo())

// SystemVersion is the runtime version string exposed in system info responses.
var SystemVersion = SoftwareVersion

func readBuildInfo() *debug.BuildInfo {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return buildInfo
}

func resolveSoftwareVersion(versionStamp string, buildInfo *debug.BuildInfo) string {
	normalizedVersionStamp := strings.TrimSpace(versionStamp)
	if normalizedVersionStamp != "" {
		return normalizedVersionStamp
	}

	if buildInfo == nil {
		return "dev"
	}

	mainVersion := strings.TrimSpace(buildInfo.Main.Version)
	if mainVersion != "" && mainVersion != "(devel)" {
		return mainVersion
	}

	vcsRevision, vcsModified := getVCSSettings(buildInfo.Settings)
	if vcsRevision == "" {
		return "dev"
	}

	shortRevision := vcsRevision
	if len(shortRevision) > 12 {
		shortRevision = shortRevision[:12]
	}

	if vcsModified {
		return "dev-g" + shortRevision + "-dirty"
	}

	return "dev-g" + shortRevision
}

func getVCSSettings(settings []debug.BuildSetting) (string, bool) {
	vcsRevision := ""
	vcsModified := false

	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			vcsRevision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			vcsModified = strings.EqualFold(strings.TrimSpace(setting.Value), "true")
		}
	}

	return vcsRevision, vcsModified
}
