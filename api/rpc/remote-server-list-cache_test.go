package rpc

import (
	"errors"
	"fmt"
	"sync"
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

func TestRemoteServerListCacheGetOrFetchTTLZeroNeverExpires(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	cache := newRemoteServerListCache(0)
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

	_, _, errFirst := cache.getOrFetch("node-1", fetch)
	if errFirst != nil {
		t.Fatalf("getOrFetch() first call error = %v", errFirst)
	}

	now = now.Add(24 * time.Hour)
	servers, usedStale, errSecond := cache.getOrFetch("node-1", fetch)
	if errSecond != nil {
		t.Fatalf("getOrFetch() second call error = %v", errSecond)
	}
	if usedStale {
		t.Fatalf("getOrFetch() usedStale = true, want false")
	}
	if fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d, want 1 (TTL=0 should not expire)", fetchCalls)
	}
	if len(servers) != 1 || servers[0] == nil {
		t.Fatalf("servers = %#v, want one non-nil server", servers)
	}
}

func TestMarkRemoteServerSummariesStalePreservesExistingSyncTime(t *testing.T) {
	t.Parallel()

	fetchedAt := time.Unix(1_700_000_100, 0).UTC()
	existingSyncedAt := timestamppb.New(fetchedAt.Add(-30 * time.Second))

	source := []*xylona.RemoteServerSummary{
		{
			RemoteServerId: "server-with-sync",
			LastSyncedAt:   existingSyncedAt,
			IsStale:        false,
			Status:         xylona.Status_ONLINE,
		},
		{
			RemoteServerId: "server-without-sync",
			LastSyncedAt:   nil,
			IsStale:        false,
			Status:         xylona.Status_UNKNOWN,
		},
	}

	stale := markRemoteServerSummariesStale(source, fetchedAt)
	if len(stale) != 2 {
		t.Fatalf("len(stale) = %d, want 2", len(stale))
	}
	if !stale[0].IsStale || !stale[1].IsStale {
		t.Fatalf("all stale summaries must be marked stale: %#v", stale)
	}
	if stale[0].Status != xylona.Status_OFFLINE || stale[1].Status != xylona.Status_OFFLINE {
		t.Fatalf("stale summaries must be forced offline: %#v", stale)
	}

	if stale[0].LastSyncedAt == nil {
		t.Fatalf("stale[0].LastSyncedAt = nil, want existing timestamp")
	}
	if !stale[0].LastSyncedAt.AsTime().Equal(existingSyncedAt.AsTime()) {
		t.Fatalf("stale[0].LastSyncedAt = %v, want %v", stale[0].LastSyncedAt.AsTime(), existingSyncedAt.AsTime())
	}

	if stale[1].LastSyncedAt == nil {
		t.Fatalf("stale[1].LastSyncedAt = nil, want fetchedAt timestamp")
	}
	if !stale[1].LastSyncedAt.AsTime().Equal(fetchedAt) {
		t.Fatalf("stale[1].LastSyncedAt = %v, want %v", stale[1].LastSyncedAt.AsTime(), fetchedAt)
	}

	// Ensure source data is unchanged.
	if source[0].IsStale || source[1].IsStale {
		t.Fatalf("source summaries were mutated: %#v", source)
	}
	if source[0].Status != xylona.Status_ONLINE || source[1].Status != xylona.Status_UNKNOWN {
		t.Fatalf("source statuses were mutated: %#v", source)
	}
}

func TestRemoteServerListCacheGetOrFetchConcurrentReadsReturnIsolatedCopies(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	cache := newRemoteServerListCache(30 * time.Second)
	cache.now = func() time.Time { return now }

	_, _, errSeed := cache.getOrFetch("node-1", func() ([]*xylona.RemoteServerSummary, error) {
		return []*xylona.RemoteServerSummary{
			{
				SourceNodeId:   "node-1",
				NodeId:         "node-1",
				RemoteServerId: "server-1",
				DisplayName:    "Alpha",
				LastSyncedAt:   timestamppb.New(now),
			},
		}, nil
	})
	if errSeed != nil {
		t.Fatalf("failed to seed cache: %v", errSeed)
	}

	const workers = 40
	var wg sync.WaitGroup
	wg.Add(workers)

	errCh := make(chan error, workers)
	for workerID := range workers {
		workerID := workerID
		go func() {
			defer wg.Done()

			servers, usedStale, errGet := cache.getOrFetch("node-1", func() ([]*xylona.RemoteServerSummary, error) {
				return nil, errors.New("fetch callback should not run for fresh cache reads")
			})
			if errGet != nil {
				errCh <- fmt.Errorf("worker %d getOrFetch error: %w", workerID, errGet)
				return
			}
			if usedStale {
				errCh <- fmt.Errorf("worker %d unexpectedly received stale data", workerID)
				return
			}
			if len(servers) != 1 || servers[0] == nil {
				errCh <- fmt.Errorf("worker %d received invalid server list: %#v", workerID, servers)
				return
			}

			servers[0].DisplayName = fmt.Sprintf("mutated-%d", workerID)
			if servers[0].LastSyncedAt != nil {
				servers[0].LastSyncedAt.Seconds = int64(workerID)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for errConcurrent := range errCh {
		if errConcurrent != nil {
			t.Fatalf("concurrent cache read failed: %v", errConcurrent)
		}
	}

	servers, usedStale, errGet := cache.getOrFetch("node-1", func() ([]*xylona.RemoteServerSummary, error) {
		return nil, errors.New("fetch callback should not run for fresh cache reads")
	})
	if errGet != nil {
		t.Fatalf("post-concurrency getOrFetch() error = %v", errGet)
	}
	if usedStale {
		t.Fatalf("post-concurrency getOrFetch() usedStale = true, want false")
	}
	if len(servers) != 1 || servers[0] == nil {
		t.Fatalf("post-concurrency server list = %#v, want one non-nil entry", servers)
	}
	if servers[0].DisplayName != "Alpha" {
		t.Fatalf("cached display name mutated across callers: got %q, want %q", servers[0].DisplayName, "Alpha")
	}
	if servers[0].LastSyncedAt.AsTime().Unix() != now.Unix() {
		t.Fatalf("cached last synced time mutated across callers: got %d, want %d", servers[0].LastSyncedAt.Seconds, now.Unix())
	}
}
