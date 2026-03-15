package rpc

import (
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const remoteServerListCacheTTL = 30 * time.Second

type remoteServerListCacheEntry struct {
	fetchedAt time.Time
	servers   []*xylona.RemoteServerSummary
}

type remoteServerListCache struct {
	ttl time.Duration
	now func() time.Time

	mu      sync.RWMutex
	entries map[string]remoteServerListCacheEntry
}

func newRemoteServerListCache(ttl time.Duration) *remoteServerListCache {
	return &remoteServerListCache{
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[string]remoteServerListCacheEntry),
	}
}

func (c *remoteServerListCache) getOrFetch(nodeID string, fetch func() ([]*xylona.RemoteServerSummary, error)) ([]*xylona.RemoteServerSummary, bool, error) {
	freshServers, fresh := c.getFresh(nodeID)
	if fresh {
		return freshServers, false, nil
	}

	fetchedServers, errFetch := fetch()
	if errFetch == nil {
		c.set(nodeID, fetchedServers, c.now())
		return cloneRemoteServerSummaries(fetchedServers), false, nil
	}

	staleServers, staleFetchedAt, hasStale := c.getAny(nodeID)
	if !hasStale {
		return nil, false, errFetch
	}

	return markRemoteServerSummariesStale(staleServers, staleFetchedAt), true, nil
}

func (c *remoteServerListCache) invalidate(nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, nodeID)
}

func (c *remoteServerListCache) getFresh(nodeID string) ([]*xylona.RemoteServerSummary, bool) {
	c.mu.RLock()
	entry, exists := c.entries[nodeID]
	c.mu.RUnlock()
	if !exists {
		return nil, false
	}

	if c.ttl > 0 && c.now().Sub(entry.fetchedAt) > c.ttl {
		return nil, false
	}

	return cloneRemoteServerSummaries(entry.servers), true
}

func (c *remoteServerListCache) getAny(nodeID string) ([]*xylona.RemoteServerSummary, time.Time, bool) {
	c.mu.RLock()
	entry, exists := c.entries[nodeID]
	c.mu.RUnlock()
	if !exists {
		return nil, time.Time{}, false
	}
	return cloneRemoteServerSummaries(entry.servers), entry.fetchedAt, true
}

func (c *remoteServerListCache) set(nodeID string, servers []*xylona.RemoteServerSummary, fetchedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[nodeID] = remoteServerListCacheEntry{
		fetchedAt: fetchedAt,
		servers:   cloneRemoteServerSummaries(servers),
	}
}

func cloneRemoteServerSummaries(summaries []*xylona.RemoteServerSummary) []*xylona.RemoteServerSummary {
	cloned := make([]*xylona.RemoteServerSummary, 0, len(summaries))
	for _, summary := range summaries {
		cloned = append(cloned, cloneRemoteServerSummary(summary))
	}
	return cloned
}

func cloneRemoteServerSummary(summary *xylona.RemoteServerSummary) *xylona.RemoteServerSummary {
	if summary == nil {
		return nil
	}
	return proto.Clone(summary).(*xylona.RemoteServerSummary)
}

func markRemoteServerSummariesStale(summaries []*xylona.RemoteServerSummary, fetchedAt time.Time) []*xylona.RemoteServerSummary {
	stale := cloneRemoteServerSummaries(summaries)
	defaultSyncedAt := timestamppb.New(fetchedAt)

	for _, summary := range stale {
		if summary == nil {
			continue
		}
		summary.IsStale = true
		if summary.LastSyncedAt == nil || summary.LastSyncedAt.AsTime().IsZero() {
			summary.LastSyncedAt = defaultSyncedAt
		}
	}

	return stale
}
