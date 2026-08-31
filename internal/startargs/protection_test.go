package startargs

import "testing"

func TestIsProtectedServerPath(t *testing.T) {
	tests := []struct {
		name             string
		relativePath     string
		baseCommand      string
		serverExecutable string
		want             bool
	}{
		{
			name:             "base command relative executable is protected",
			relativePath:     "7DaysToDieServer",
			baseCommand:      "./7DaysToDieServer",
			serverExecutable: "",
			want:             true,
		},
		{
			name:             "server executable is protected",
			relativePath:     "paper-1.21.4-100.jar",
			baseCommand:      "java",
			serverExecutable: "paper-1.21.4-100.jar",
			want:             true,
		},
		{
			name:             "unrelated path is not protected",
			relativePath:     "plugins/example.jar",
			baseCommand:      "./7DaysToDieServer",
			serverExecutable: "paper.jar",
			want:             false,
		},
		{
			name:             "empty server executable means no protection for that field",
			relativePath:     "paper.jar",
			baseCommand:      "java",
			serverExecutable: "",
			want:             false,
		},
		{
			name:             "system binary base command is not protected",
			relativePath:     "java",
			baseCommand:      "java",
			serverExecutable: "",
			want:             false,
		},
		{
			name:             "subdirectory containing executable name is not a false positive",
			relativePath:     "plugins/7DaysToDieServer",
			baseCommand:      "./7DaysToDieServer",
			serverExecutable: "",
			want:             false,
		},
		{
			name:             "mixed slashes still match",
			relativePath:     `bin\server.exe`,
			baseCommand:      "./bin/server.exe",
			serverExecutable: "",
			want:             true,
		},
		{
			name:             "install directory placeholder remains protected",
			relativePath:     "scripts/custom-start.sh",
			baseCommand:      "{{INSTALL_DIR}}/scripts/custom-start.sh",
			serverExecutable: "",
			want:             true,
		},
		{
			name:             "ancestor of executable is protected",
			relativePath:     "bin",
			baseCommand:      "java",
			serverExecutable: "bin/server.exe",
			want:             true,
		},
		{
			name:             "server root is protected when executable policy exists",
			relativePath:     "",
			baseCommand:      "java",
			serverExecutable: "paper-1.21.4-100.jar",
			want:             true,
		},
		{
			name:             "dot root is protected when executable policy exists",
			relativePath:     ".",
			baseCommand:      "java",
			serverExecutable: "paper-1.21.4-100.jar",
			want:             true,
		},
		{
			name:             "ancestor of relative base command is protected",
			relativePath:     "scripts",
			baseCommand:      "{{INSTALL_DIR}}/scripts/custom-start.sh",
			serverExecutable: "",
			want:             true,
		},
		{
			name:             "sibling directory is not protected",
			relativePath:     "plugins",
			baseCommand:      "java",
			serverExecutable: "bin/server.exe",
			want:             false,
		},
		{
			name:             "prefix-named sibling of executable directory is not protected",
			relativePath:     "bin2",
			baseCommand:      "java",
			serverExecutable: "bin/server.exe",
			want:             false,
		},
		{
			name:             "root is not protected when no policy exists",
			relativePath:     "",
			baseCommand:      "java",
			serverExecutable: "",
			want:             false,
		},
		{
			name:             "case-variant ancestor is protected",
			relativePath:     "BIN",
			baseCommand:      "java",
			serverExecutable: "bin/server.exe",
			want:             true,
		},
		{
			name:             "case-variant executable is protected",
			relativePath:     "bin/SERVER.EXE",
			baseCommand:      "java",
			serverExecutable: "bin/server.exe",
			want:             true,
		},
		{
			name:             "case-variant prefix sibling is not protected",
			relativePath:     "BIN2",
			baseCommand:      "java",
			serverExecutable: "bin/server.exe",
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsProtectedServerPath(tt.relativePath, tt.baseCommand, tt.serverExecutable)
			if got != tt.want {
				t.Errorf("IsProtectedServerPath() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestIsReservedManagedPath(t *testing.T) {
	tests := []struct {
		name         string
		relativePath string
		want         bool
	}{
		{name: "managed BlueMap directory", relativePath: ".xylona/bluemap", want: true},
		{name: "managed BlueMap web asset", relativePath: ".xylona/bluemap/web/index.html", want: true},
		{name: "managed xylona root", relativePath: ".xylona", want: true},
		{name: "case-variant managed xylona root", relativePath: ".XYLONA", want: true},
		{name: "case-variant managed BlueMap asset", relativePath: ".Xylona/BlueMap/web/index.html", want: true},
		{name: "world files are not reserved", relativePath: "world/level.dat", want: false},
		{name: "empty path is not reserved", relativePath: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsReservedManagedPath(tt.relativePath)
			if got != tt.want {
				t.Errorf("IsReservedManagedPath(%q) = %t, want %t", tt.relativePath, got, tt.want)
			}
		})
	}
}
