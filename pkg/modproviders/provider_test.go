package modproviders

import (
	"context"
	"testing"
)

// mockProvider is a minimal ModProvider implementation for use in tests.
type mockProvider struct {
	id             string
	requiresAPIKey bool
}

func (m *mockProvider) ID() string {
	return m.id
}

func (m *mockProvider) Search(_ context.Context, _ string, _ SearchParams) (SearchResult, error) {
	return SearchResult{
		Results:   []ModSearchResult{{Source: m.id, SourceID: "result-1", Name: "Test Mod"}},
		TotalHits: 1,
	}, nil
}

func (m *mockProvider) GetModDetails(_ context.Context, sourceID string, _ SearchParams) (*ModDetails, error) {
	return &ModDetails{
		Source:   m.id,
		SourceID: sourceID,
		Name:     "Test Mod",
	}, nil
}

func (m *mockProvider) GetVersions(_ context.Context, _ string, _ string, _ SearchParams) ([]ModVersion, error) {
	return []ModVersion{
		{VersionID: "v1.0.0", VersionString: "1.0.0"},
	}, nil
}

func (m *mockProvider) Download(_ context.Context, _ string, _ string, _ string) ([]DownloadedFile, error) {
	return []DownloadedFile{
		{Path: "mods/test.jar", Hash: "abc123", Size: 1024, IsPrimary: true},
	}, nil
}

func (m *mockProvider) CheckForUpdate(_ context.Context, _ string, _ string) (*ModVersion, error) {
	return &ModVersion{VersionID: "v1.1.0", VersionString: "1.1.0"}, nil
}

func (m *mockProvider) RequiresAPIKey() bool {
	return m.requiresAPIKey
}

func TestRegisterAndGetProvider(t *testing.T) {
	p := &mockProvider{id: "test-register-get"}
	RegisterProvider(p)

	got, ok := GetProvider("test-register-get")
	if !ok {
		t.Fatal("GetProvider() ok = false, want true")
	}
	if got.ID() != "test-register-get" {
		t.Errorf("GetProvider() ID = %q, want %q", got.ID(), "test-register-get")
	}
}

func TestGetProvider_UnknownID(t *testing.T) {
	_, ok := GetProvider("does-not-exist-xyz")
	if ok {
		t.Error("GetProvider() ok = true for unknown ID, want false")
	}
}

func TestRegisterProvider_PanicOnDuplicate(t *testing.T) {
	p := &mockProvider{id: "test-duplicate-panic"}
	RegisterProvider(p)

	defer func() {
		r := recover()
		if r == nil {
			t.Error("RegisterProvider() did not panic on duplicate ID, want panic")
		}
	}()

	RegisterProvider(&mockProvider{id: "test-duplicate-panic"})
}

func TestAllProviders(t *testing.T) {
	p1 := &mockProvider{id: "test-all-providers-alpha"}
	p2 := &mockProvider{id: "test-all-providers-beta"}
	RegisterProvider(p1)
	RegisterProvider(p2)

	all := AllProviders()

	if _, ok := all["test-all-providers-alpha"]; !ok {
		t.Error("AllProviders() missing test-all-providers-alpha")
	}
	if _, ok := all["test-all-providers-beta"]; !ok {
		t.Error("AllProviders() missing test-all-providers-beta")
	}
}

func TestAllProviders_ReturnsCopy(t *testing.T) {
	p := &mockProvider{id: "test-all-copy"}
	RegisterProvider(p)

	all := AllProviders()
	// Mutate the returned map.
	delete(all, "test-all-copy")

	// The registry itself should be unaffected.
	_, ok := GetProvider("test-all-copy")
	if !ok {
		t.Error("AllProviders() mutation affected the registry, want independent copy")
	}
}

func TestRequiresAPIKey(t *testing.T) {
	tests := []struct {
		name           string
		requiresAPIKey bool
	}{
		{name: "provider requires API key", requiresAPIKey: true},
		{name: "provider does not require API key", requiresAPIKey: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := &mockProvider{id: "test-apikey-" + tt.name, requiresAPIKey: tt.requiresAPIKey}
			if got := p.RequiresAPIKey(); got != tt.requiresAPIKey {
				t.Errorf("RequiresAPIKey() = %v, want %v", got, tt.requiresAPIKey)
			}
		})
	}
}
