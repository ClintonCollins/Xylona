package steamcache

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchDetails_ParsesSteamCmdResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := steamCmdResponse{
			Data: map[string]steamCmdAppData{
				"896660": {
					Common: struct {
						Name   string `json:"name"`
						OSList string `json:"oslist"`
						Parent string `json:"parent"`
						Type   string `json:"type"`
					}{
						Name:   "Valheim Dedicated Server",
						OSList: "windows,linux",
						Parent: "892970",
						Type:   "Tool",
					},
					Config: struct {
						InstallDir string                        `json:"installdir"`
						Launch     map[string]steamCmdLaunchEntry `json:"launch"`
					}{
						InstallDir: "Valheim dedicated server",
						Launch: map[string]steamCmdLaunchEntry{
							"0": {
								Executable:  "valheim_server.exe",
								Arguments:   "-nographics -batchmode",
								Description: "Run Server",
								Config: struct {
									OSList string `json:"oslist"`
								}{OSList: "windows"},
							},
						},
					},
				},
			},
			Status: "success",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := &Client{detailsURLFmt: server.URL + "/%s"}
	details, err := c.FetchDetails(context.Background(), "896660")
	if err != nil {
		t.Fatalf("FetchDetails() error: %v", err)
	}

	if details.Name != "Valheim Dedicated Server" {
		t.Errorf("Name = %q, want %q", details.Name, "Valheim Dedicated Server")
	}
	if details.Type != "Tool" {
		t.Errorf("Type = %q, want %q", details.Type, "Tool")
	}
	if !details.WindowsSupport {
		t.Error("expected WindowsSupport = true")
	}
	if !details.LinuxSupport {
		t.Error("expected LinuxSupport = true")
	}
	if details.InstallDirectory != "Valheim dedicated server" {
		t.Errorf("InstallDirectory = %q, want %q", details.InstallDirectory, "Valheim dedicated server")
	}
	if details.ParentAppID != "892970" {
		t.Errorf("ParentAppID = %q, want %q", details.ParentAppID, "892970")
	}
	if len(details.LaunchConfigs) != 1 {
		t.Fatalf("LaunchConfigs length = %d, want 1", len(details.LaunchConfigs))
	}
	if details.LaunchConfigs[0].Executable != "valheim_server.exe" {
		t.Errorf("LaunchConfigs[0].Executable = %q, want %q", details.LaunchConfigs[0].Executable, "valheim_server.exe")
	}
	if details.LaunchConfigs[0].OS != "windows" {
		t.Errorf("LaunchConfigs[0].OS = %q, want %q", details.LaunchConfigs[0].OS, "windows")
	}
}

func TestFetchDetails_LinuxOnlyApp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := steamCmdResponse{
			Data: map[string]steamCmdAppData{
				"123": {
					Common: struct {
						Name   string `json:"name"`
						OSList string `json:"oslist"`
						Parent string `json:"parent"`
						Type   string `json:"type"`
					}{
						Name:   "Linux Only Server",
						OSList: "linux",
						Type:   "Tool",
					},
				},
			},
			Status: "success",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := &Client{detailsURLFmt: server.URL + "/%s"}
	details, err := c.FetchDetails(context.Background(), "123")
	if err != nil {
		t.Fatalf("FetchDetails() error: %v", err)
	}
	if details.WindowsSupport {
		t.Error("expected WindowsSupport = false")
	}
	if !details.LinuxSupport {
		t.Error("expected LinuxSupport = true")
	}
}

func TestFetchDetails_HandlesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	c := &Client{detailsURLFmt: server.URL + "/%s"}
	_, err := c.FetchDetails(context.Background(), "999999")
	if err == nil {
		t.Fatal("FetchDetails() expected error for 404 response")
	}
}

func TestFetchDetails_HandlesMissingAppData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := steamCmdResponse{
			Data:   map[string]steamCmdAppData{},
			Status: "success",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := &Client{detailsURLFmt: server.URL + "/%s"}
	_, err := c.FetchDetails(context.Background(), "896660")
	if err == nil {
		t.Fatal("FetchDetails() expected error when app data is missing from response")
	}
}

func TestFetchDetails_HandlesServerDown(t *testing.T) {
	c := &Client{detailsURLFmt: "http://localhost:1/%s"}
	_, err := c.FetchDetails(context.Background(), "896660")
	if err == nil {
		t.Fatal("FetchDetails() expected error for unreachable server")
	}
}

func TestFetchDetails_MultipleLaunchConfigs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := steamCmdResponse{
			Data: map[string]steamCmdAppData{
				"100": {
					Common: struct {
						Name   string `json:"name"`
						OSList string `json:"oslist"`
						Parent string `json:"parent"`
						Type   string `json:"type"`
					}{
						Name:   "Multi Launch Server",
						OSList: "windows,linux",
						Type:   "Tool",
					},
					Config: struct {
						InstallDir string                        `json:"installdir"`
						Launch     map[string]steamCmdLaunchEntry `json:"launch"`
					}{
						InstallDir: "multi_server",
						Launch: map[string]steamCmdLaunchEntry{
							"0": {
								Executable: "server_win.exe",
								Config: struct {
									OSList string `json:"oslist"`
								}{OSList: "windows"},
							},
							"1": {
								Executable: "server_linux",
								Config: struct {
									OSList string `json:"oslist"`
								}{OSList: "linux"},
							},
						},
					},
				},
			},
			Status: "success",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := &Client{detailsURLFmt: server.URL + "/%s"}
	details, err := c.FetchDetails(context.Background(), "100")
	if err != nil {
		t.Fatalf("FetchDetails() error: %v", err)
	}
	if len(details.LaunchConfigs) != 2 {
		t.Errorf("LaunchConfigs length = %d, want 2", len(details.LaunchConfigs))
	}
}

func TestNew_SetsDefaults(t *testing.T) {
	c := New()
	if c.detailsURLFmt == "" {
		t.Error("detailsURLFmt not set")
	}
}
