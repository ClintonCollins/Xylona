package rpc

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestRemoteServerListCacheGetOrFetchCachesFreshResults(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	cache := newRemoteServerListCache(30 * time.Second)
	cache.now = func() time.Time { return now }

	fetchCalls := 0
	fetch := func() ([]*xylona.RemoteServerSummary, error) {
		fetchCalls++
		return []*xylona.RemoteServerSummary{
			{
				SourceNodeId:   "node-1",
				NodeId:         "node-1",
				RemoteServerId: "server-1",
				DisplayName:    "Alpha",
				LastSyncedAt:   timestamppb.New(now),
			},
		}, nil
	}

	first, usedStaleFirst, errFirst := cache.getOrFetch("node-1", fetch)
	if errFirst != nil {
		t.Fatalf("getOrFetch() first call error = %v", errFirst)
	}
	if usedStaleFirst {
		t.Fatalf("getOrFetch() first call unexpectedly used stale data")
	}
	if fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d, want 1", fetchCalls)
	}
	if len(first) != 1 {
		t.Fatalf("len(first) = %d, want 1", len(first))
	}

	// Mutate caller-owned data and verify cache returns an isolated copy.
	first[0].DisplayName = "Mutated"

	second, usedStaleSecond, errSecond := cache.getOrFetch("node-1", fetch)
	if errSecond != nil {
		t.Fatalf("getOrFetch() second call error = %v", errSecond)
	}
	if usedStaleSecond {
		t.Fatalf("getOrFetch() second call unexpectedly used stale data")
	}
	if fetchCalls != 1 {
		t.Fatalf("fetchCalls after second call = %d, want 1", fetchCalls)
	}
	if got := second[0].DisplayName; got != "Alpha" {
		t.Fatalf("second[0].DisplayName = %q, want %q", got, "Alpha")
	}
}

func TestRemoteServerListCacheGetOrFetchUsesStaleDataOnRefreshFailure(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	cache := newRemoteServerListCache(30 * time.Second)
	cache.now = func() time.Time { return now }

	initialFetch := func() ([]*xylona.RemoteServerSummary, error) {
		return []*xylona.RemoteServerSummary{
			{
				SourceNodeId:   "node-1",
				NodeId:         "node-1",
				RemoteServerId: "server-1",
				DisplayName:    "Alpha",
				IsStale:        false,
				LastSyncedAt:   timestamppb.New(now),
			},
		}, nil
	}

	_, _, errInitial := cache.getOrFetch("node-1", initialFetch)
	if errInitial != nil {
		t.Fatalf("getOrFetch() initial cache fill error = %v", errInitial)
	}

	now = now.Add(31 * time.Second)
	refreshErr := errors.New("refresh failed")
	servers, usedStale, errRefresh := cache.getOrFetch("node-1", func() ([]*xylona.RemoteServerSummary, error) {
		return nil, refreshErr
	})
	if errRefresh != nil {
		t.Fatalf("getOrFetch() stale fallback error = %v", errRefresh)
	}
	if !usedStale {
		t.Fatalf("getOrFetch() usedStale = false, want true")
	}
	if len(servers) != 1 {
		t.Fatalf("len(servers) = %d, want 1", len(servers))
	}
	if !servers[0].IsStale {
		t.Fatalf("servers[0].IsStale = false, want true")
	}
}

func TestRemoteServerListCacheGetOrFetchReturnsErrorWithoutCachedData(t *testing.T) {
	cache := newRemoteServerListCache(30 * time.Second)
	refreshErr := errors.New("refresh failed")

	servers, usedStale, errFetch := cache.getOrFetch("node-1", func() ([]*xylona.RemoteServerSummary, error) {
		return nil, refreshErr
	})
	if !errors.Is(errFetch, refreshErr) {
		t.Fatalf("getOrFetch() error = %v, want %v", errFetch, refreshErr)
	}
	if usedStale {
		t.Fatalf("getOrFetch() usedStale = true, want false")
	}
	if servers != nil {
		t.Fatalf("getOrFetch() servers = %#v, want nil", servers)
	}
}

func TestRemoteServerListCacheInvalidate(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	cache := newRemoteServerListCache(30 * time.Second)
	cache.now = func() time.Time { return now }

	_, _, errFill := cache.getOrFetch("node-1", func() ([]*xylona.RemoteServerSummary, error) {
		return []*xylona.RemoteServerSummary{
			{SourceNodeId: "node-1", NodeId: "node-1", RemoteServerId: "server-1"},
		}, nil
	})
	if errFill != nil {
		t.Fatalf("getOrFetch() fill error = %v", errFill)
	}

	cache.invalidate("node-1")

	refreshErr := errors.New("refresh failed")
	servers, usedStale, errAfterInvalidate := cache.getOrFetch("node-1", func() ([]*xylona.RemoteServerSummary, error) {
		return nil, refreshErr
	})
	if !errors.Is(errAfterInvalidate, refreshErr) {
		t.Fatalf("getOrFetch() error after invalidate = %v, want %v", errAfterInvalidate, refreshErr)
	}
	if usedStale {
		t.Fatalf("getOrFetch() usedStale after invalidate = true, want false")
	}
	if servers != nil {
		t.Fatalf("getOrFetch() servers after invalidate = %#v, want nil", servers)
	}
}
