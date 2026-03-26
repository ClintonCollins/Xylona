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
