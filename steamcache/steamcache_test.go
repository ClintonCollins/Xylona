package steamcache

import (
	"context"
	"errors"
	"testing"
)

// mockFetcher implements Fetcher for testing.
type mockFetcher struct {
	apps []SteamApp
	err  error
}

func (m *mockFetcher) FetchAppList(ctx context.Context) ([]SteamApp, error) {
	return m.apps, m.err
}

func TestFilterApps(t *testing.T) {
	apps := []SteamApp{
		{AppID: "1", Name: "Counter-Strike 2"},
		{AppID: "2", Name: "Counter-Strike 2 Dedicated Server"},
		{AppID: "3", Name: "Valheim Dedicated Server"},
		{AppID: "4", Name: "Rust"},
		{AppID: "5", Name: "Team Fortress 2 Server"},
		{AppID: "6", Name: "Some Game"},
		{AppID: "7", Name: "DEDICATED thing"},
		{AppID: "8", Name: "my SERVER app"},
	}

	filtered := FilterApps(apps)

	want := map[string]bool{
		"2": true, // "Dedicated Server"
		"3": true, // "Dedicated Server"
		"5": true, // "Server"
		"7": true, // "DEDICATED"
		"8": true, // "SERVER"
	}

	if len(filtered) != len(want) {
		t.Fatalf("FilterApps() returned %d apps, want %d", len(filtered), len(want))
	}

	for _, app := range filtered {
		if !want[app.AppID] {
			t.Errorf("FilterApps() included unexpected app %q (ID %s)", app.Name, app.AppID)
		}
	}
}

func TestSearch(t *testing.T) {
	apps := make([]SteamApp, 30)
	for i := range 30 {
		apps[i] = SteamApp{
			AppID: string(rune('0' + i)),
			Name:  "Test Server " + string(rune('A'+i)),
		}
	}

	fetcher := &mockFetcher{apps: apps}
	cache := New(fetcher)

	ctx := context.Background()
	errStart := cache.loadApps(ctx)
	if errStart != nil {
		t.Fatalf("loadApps() error = %v", errStart)
	}

	results := cache.Search("Test Server")
	if len(results) != 20 {
		t.Errorf("Search() returned %d results, want 20 (max limit)", len(results))
	}

	results = cache.Search("Server A")
	if len(results) != 1 {
		t.Errorf("Search(\"Server A\") returned %d results, want 1", len(results))
	}

	results = cache.Search("nonexistent")
	if len(results) != 0 {
		t.Errorf("Search(\"nonexistent\") returned %d results, want 0", len(results))
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	apps := []SteamApp{
		{AppID: "1", Name: "Valheim Dedicated Server"},
		{AppID: "2", Name: "Rust Dedicated Server"},
	}

	fetcher := &mockFetcher{apps: apps}
	cache := New(fetcher)

	ctx := context.Background()
	errLoad := cache.loadApps(ctx)
	if errLoad != nil {
		t.Fatalf("loadApps() error = %v", errLoad)
	}

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "lowercase query", query: "valheim", want: 1},
		{name: "uppercase query", query: "VALHEIM", want: 1},
		{name: "mixed case query", query: "VaLhEiM", want: 1},
		{name: "lowercase dedicated", query: "dedicated", want: 2},
		{name: "uppercase dedicated", query: "DEDICATED", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := cache.Search(tt.query)
			if len(results) != tt.want {
				t.Errorf("Search(%q) returned %d results, want %d", tt.query, len(results), tt.want)
			}
		})
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	apps := []SteamApp{
		{AppID: "1", Name: "Valheim Dedicated Server"},
	}

	fetcher := &mockFetcher{apps: apps}
	cache := New(fetcher)

	ctx := context.Background()
	errLoad := cache.loadApps(ctx)
	if errLoad != nil {
		t.Fatalf("loadApps() error = %v", errLoad)
	}

	results := cache.Search("")
	if len(results) != 0 {
		t.Errorf("Search(\"\") returned %d results, want 0", len(results))
	}
}

func TestCacheRetainsDataOnFetchFailure(t *testing.T) {
	initialApps := []SteamApp{
		{AppID: "1", Name: "Valheim Dedicated Server"},
		{AppID: "2", Name: "Rust Dedicated Server"},
	}

	fetcher := &mockFetcher{apps: initialApps}
	cache := New(fetcher)

	ctx := context.Background()
	errLoad := cache.loadApps(ctx)
	if errLoad != nil {
		t.Fatalf("initial loadApps() error = %v", errLoad)
	}

	results := cache.Search("Dedicated")
	if len(results) != 2 {
		t.Fatalf("initial Search(\"Dedicated\") returned %d results, want 2", len(results))
	}

	// Make subsequent fetches fail.
	fetcher.err = errors.New("network error")

	errReload := cache.loadApps(ctx)
	if errReload == nil {
		t.Fatal("expected loadApps() to return error on fetch failure")
	}

	// Cache should retain previous data.
	results = cache.Search("Dedicated")
	if len(results) != 2 {
		t.Errorf("after failed refresh, Search(\"Dedicated\") returned %d results, want 2", len(results))
	}
}
