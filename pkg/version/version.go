package version

import (
	"runtime/debug"
	"strings"
)

var softwareVersionStamp string

var SoftwareVersion = resolveSoftwareVersion(softwareVersionStamp, readBuildInfo())
var SystemVersion = SoftwareVersion

const (
	// FederationProtocolVersion is the version of the federation protocol.
	FederationProtocolVersion = 1

	// FederationCapabilities is a comma-separated list of capabilities supported by this node.
	FederationCapabilities = "server_list,server_detail,remote_actions,console_streaming,status_streaming,file_operations,update,edit,remove"
)

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
