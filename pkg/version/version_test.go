package version

import (
	"runtime/debug"
	"testing"
)

func TestResolveSoftwareVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		versionStamp string
		buildInfo    *debug.BuildInfo
		want         string
	}{
		{
			name:         "uses explicit stamp when provided",
			versionStamp: "v1.2.3-4-gabc1234",
			want:         "v1.2.3-4-gabc1234",
		},
		{
			name:         "uses module version from build info",
			versionStamp: "",
			buildInfo: &debug.BuildInfo{
				Main: debug.Module{
					Version: "v2.0.1",
				},
			},
			want: "v2.0.1",
		},
		{
			name:         "uses vcs revision when module version is devel",
			versionStamp: "",
			buildInfo: &debug.BuildInfo{
				Main: debug.Module{
					Version: "(devel)",
				},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "595854b1234567890"},
					{Key: "vcs.modified", Value: "false"},
				},
			},
			want: "dev-g595854b12345",
		},
		{
			name:         "marks dirty vcs revisions",
			versionStamp: "",
			buildInfo: &debug.BuildInfo{
				Main: debug.Module{
					Version: "(devel)",
				},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "595854b1234567890"},
					{Key: "vcs.modified", Value: "true"},
				},
			},
			want: "dev-g595854b12345-dirty",
		},
		{
			name:         "falls back to dev when no build info exists",
			versionStamp: "",
			buildInfo:    nil,
			want:         "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveSoftwareVersion(tt.versionStamp, tt.buildInfo)
			if got != tt.want {
				t.Errorf("resolveSoftwareVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetVCSSettings(t *testing.T) {
	t.Parallel()

	settings := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "  abcdef1234567890  "},
		{Key: "vcs.modified", Value: "TRUE"},
	}

	gotRevision, gotModified := getVCSSettings(settings)
	if gotRevision != "abcdef1234567890" {
		t.Errorf("getVCSSettings() revision = %q, want %q", gotRevision, "abcdef1234567890")
	}
	if !gotModified {
		t.Errorf("getVCSSettings() modified = %t, want %t", gotModified, true)
	}
}
