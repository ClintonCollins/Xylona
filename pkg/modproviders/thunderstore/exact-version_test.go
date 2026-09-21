package thunderstore

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestExactVersion(t *testing.T) {
	for _, test := range []struct {
		name         string
		source       string
		version      string
		responseName string
		responseVer  string
		dependencies []string
		status       int
		wantError    bool
	}{
		{name: "pinned old dependencies", dependencies: []string{"Other-Library-1.0.0"}},
		{name: "no dependencies", dependencies: []string{}},
		{name: "different package", responseName: "Other", dependencies: []string{}, wantError: true},
		{name: "latest version rejected", responseVer: "2.0.0", dependencies: []string{}, wantError: true},
		{name: "missing dependency metadata", wantError: true},
		{name: "missing version", status: http.StatusNotFound, wantError: true},
		{name: "invalid package", source: "../Package", wantError: true},
		{name: "invalid version", version: "../1.0.0", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.source == "" {
				test.source = "Team-Package"
			}
			if test.version == "" {
				test.version = "1.0.0"
			}
			if test.responseName == "" {
				test.responseName = "Package"
			}
			if test.responseVer == "" {
				test.responseVer = "1.0.0"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/experimental/package/Team/Package/1.0.0/" {
					t.Errorf("unexpected endpoint %q", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if test.status != 0 {
					w.WriteHeader(test.status)
					return
				}
				writeTestJSON(t, w, map[string]any{
					"namespace": "Team", "name": test.responseName, "version_number": test.responseVer,
					"dependencies": test.dependencies,
				})
			}))
			defer srv.Close()
			result, errExact := newTestProvider(srv).ExactVersion(t.Context(), test.source, test.version)
			if (errExact != nil) != test.wantError {
				t.Fatalf("ExactVersion error = %v, wantError %v", errExact, test.wantError)
			}
			if test.wantError {
				return
			}
			if result.ID != "Team-Package" || result.Version != "1.0.0" || !slices.Equal(result.Dependencies, test.dependencies) {
				t.Fatalf("unexpected exact metadata: %+v", result)
			}
		})
	}
}
