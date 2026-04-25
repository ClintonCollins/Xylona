package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "newer patch", a: "v1.2.4", b: "1.2.3", want: 1},
		{name: "older minor", a: "1.1.9", b: "1.2.0", want: -1},
		{name: "equal with v prefix", a: "v2.0.0", b: "2.0.0", want: 0},
		{name: "release after prerelease", a: "1.0.0", b: "1.0.0-rc.1", want: 1},
		{name: "dev sorts low", a: "dev", b: "0.0.1", want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := CompareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
