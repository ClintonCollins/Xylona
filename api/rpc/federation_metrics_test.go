package rpc

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// TestGetServerDetailPopulatesMetricsFromSupervisor verifies that when a game
// server process is tracked by the supervisor with known metric values,
// GetServerDetail returns those values in the FederationServerSummary.
func TestGetServerDetailPopulatesMetricsFromSupervisor(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-metrics-peer")

	errCreateGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-metrics-grant-1",
		"server-local-1",
		"node-metrics-peer",
		"user-owner",
		"owner",
		"viewer",
		"user-admin",
	)
	if errCreateGrant != nil {
		t.Fatalf("failed to create federated grant: %v", errCreateGrant)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	// Known metric values to inject.
	var (
		wantCPUPercent      float64 = 42.5
		wantMemoryRSS       uint64  = 134217728 // 128 MiB working set
		wantMemoryVMS       uint64  = 268435456 // 256 MiB private committed
		wantMemoryPercent   float32 = 3.14
		wantCPUCores        int32   = 8
		wantNumThreads      int32   = 16
		wantDiskUsageBytes  uint64  = 1073741824 // 1 GiB
		wantIOReadRate      float64 = 1024.0
		wantIOWriteRate     float64 = 512.0
		wantConnectionCount int32   = 7
	)

	// Place the start time 60 seconds in the past so uptime is deterministic.
	startedAt := time.Now().Unix() - 60

	supervisor.NewCommandWithMetrics(
		supervisorInst,
		"server-local-1",
		wantCPUPercent,
		wantMemoryRSS,
		wantMemoryVMS,
		wantMemoryPercent,
		wantCPUCores,
		wantNumThreads,
		wantDiskUsageBytes,
		wantIOReadRate,
		wantIOWriteRate,
		wantConnectionCount,
		startedAt,
	)

	service := FederationService{
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-metrics-peer",
	})

	request := connect.NewRequest(&xylona.FederationGetServerDetailRequest{
		ServerId: "server-local-1",
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "user-owner")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-metrics-peer")

	response, errDetail := service.GetServerDetail(peerCtx, request)
	if errDetail != nil {
		t.Fatalf("GetServerDetail() error = %v", errDetail)
	}
	if response.Msg == nil || response.Msg.Server == nil {
		t.Fatalf("GetServerDetail() returned empty server")
	}

	s := response.Msg.Server

	// CpuPercent is stored as int64(cpuPct) in the handler (truncated).
	wantCPUPercentInt := int64(wantCPUPercent)
	if s.CpuPercent != wantCPUPercentInt {
		t.Errorf("CpuPercent = %d, want %d", s.CpuPercent, wantCPUPercentInt)
	}

	// MemoryBytes maps to memoryVMS (private committed memory).
	if s.MemoryBytes != int64(wantMemoryVMS) {
		t.Errorf("MemoryBytes = %d, want %d", s.MemoryBytes, int64(wantMemoryVMS))
	}

	// MemoryWorkingSetBytes maps to memoryRSS.
	if s.MemoryWorkingSetBytes != int64(wantMemoryRSS) {
		t.Errorf("MemoryWorkingSetBytes = %d, want %d", s.MemoryWorkingSetBytes, int64(wantMemoryRSS))
	}

	if s.MemoryPercent != float64(wantMemoryPercent) {
		t.Errorf("MemoryPercent = %v, want %v", s.MemoryPercent, float64(wantMemoryPercent))
	}

	if s.CpuCores != wantCPUCores {
		t.Errorf("CpuCores = %d, want %d", s.CpuCores, wantCPUCores)
	}

	if s.NumberOfThreads != wantNumThreads {
		t.Errorf("NumberOfThreads = %d, want %d", s.NumberOfThreads, wantNumThreads)
	}

	if s.DiskUsageBytes != int64(wantDiskUsageBytes) {
		t.Errorf("DiskUsageBytes = %d, want %d", s.DiskUsageBytes, int64(wantDiskUsageBytes))
	}

	if s.IoReadRate != wantIOReadRate {
		t.Errorf("IoReadRate = %v, want %v", s.IoReadRate, wantIOReadRate)
	}

	if s.IoWriteRate != wantIOWriteRate {
		t.Errorf("IoWriteRate = %v, want %v", s.IoWriteRate, wantIOWriteRate)
	}

	if s.ConnectionCount != wantConnectionCount {
		t.Errorf("ConnectionCount = %d, want %d", s.ConnectionCount, wantConnectionCount)
	}

	// UptimeSeconds must be at least 59 (we started 60 s ago; allow 1 s slack).
	if s.UptimeSeconds < 59 {
		t.Errorf("UptimeSeconds = %d, want >= 59", s.UptimeSeconds)
	}
}

// TestGetServerDetailReturnsZeroMetricsWhenNoSupervisorCommand verifies that
// GetServerDetail returns zeroed metric fields when the supervisor has no
// running command for the requested server (i.e., the server is offline).
func TestGetServerDetailReturnsZeroMetricsWhenNoSupervisorCommand(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-zero-metrics-peer")

	errCreateGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-zero-metrics-grant-1",
		"server-local-1",
		"node-zero-metrics-peer",
		"user-owner",
		"owner",
		"viewer",
		"user-admin",
	)
	if errCreateGrant != nil {
		t.Fatalf("failed to create federated grant: %v", errCreateGrant)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	// No command injected — supervisor has no entry for "server-local-1".

	service := FederationService{
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-zero-metrics-peer",
	})

	request := connect.NewRequest(&xylona.FederationGetServerDetailRequest{
		ServerId: "server-local-1",
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "user-owner")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-zero-metrics-peer")

	response, errDetail := service.GetServerDetail(peerCtx, request)
	if errDetail != nil {
		t.Fatalf("GetServerDetail() error = %v", errDetail)
	}
	if response.Msg == nil || response.Msg.Server == nil {
		t.Fatalf("GetServerDetail() returned empty server")
	}

	s := response.Msg.Server

	if s.CpuPercent != 0 {
		t.Errorf("CpuPercent = %d, want 0", s.CpuPercent)
	}
	if s.MemoryBytes != 0 {
		t.Errorf("MemoryBytes = %d, want 0", s.MemoryBytes)
	}
	if s.MemoryWorkingSetBytes != 0 {
		t.Errorf("MemoryWorkingSetBytes = %d, want 0", s.MemoryWorkingSetBytes)
	}
	if s.MemoryPercent != 0 {
		t.Errorf("MemoryPercent = %v, want 0", s.MemoryPercent)
	}
	if s.DiskUsageBytes != 0 {
		t.Errorf("DiskUsageBytes = %d, want 0", s.DiskUsageBytes)
	}
	if s.IoReadRate != 0 {
		t.Errorf("IoReadRate = %v, want 0", s.IoReadRate)
	}
	if s.IoWriteRate != 0 {
		t.Errorf("IoWriteRate = %v, want 0", s.IoWriteRate)
	}
	if s.ConnectionCount != 0 {
		t.Errorf("ConnectionCount = %d, want 0", s.ConnectionCount)
	}
	if s.UptimeSeconds != 0 {
		t.Errorf("UptimeSeconds = %d, want 0", s.UptimeSeconds)
	}
}

// TestGetServerDetailMetricsNilSupervisor verifies that GetServerDetail
// succeeds and returns zeroed metric fields when no supervisor instance is
// attached to the FederationService at all.
func TestGetServerDetailMetricsNilSupervisor(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-nil-sup-peer")

	errCreateGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-nil-sup-grant-1",
		"server-local-1",
		"node-nil-sup-peer",
		"user-owner",
		"owner",
		"viewer",
		"user-admin",
	)
	if errCreateGrant != nil {
		t.Fatalf("failed to create federated grant: %v", errCreateGrant)
	}

	// supervisorInst intentionally left nil.
	service := FederationService{
		db:             fixture.conn,
		supervisorInst: nil,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-nil-sup-peer",
	})

	request := connect.NewRequest(&xylona.FederationGetServerDetailRequest{
		ServerId: "server-local-1",
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "user-owner")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-nil-sup-peer")

	response, errDetail := service.GetServerDetail(peerCtx, request)
	if errDetail != nil {
		t.Fatalf("GetServerDetail() with nil supervisor error = %v", errDetail)
	}
	if response.Msg == nil || response.Msg.Server == nil {
		t.Fatalf("GetServerDetail() returned empty server")
	}

	s := response.Msg.Server

	if s.CpuPercent != 0 || s.MemoryBytes != 0 || s.UptimeSeconds != 0 {
		t.Errorf("expected all metrics to be zero with nil supervisor, got cpu=%d mem=%d uptime=%d",
			s.CpuPercent, s.MemoryBytes, s.UptimeSeconds)
	}
}

// TestGetServerDetailMetricsFieldMapping verifies that the field assignment in
// GetServerDetail correctly maps Metrics() return values: specifically that
// MemoryBytes receives the VMS (private committed) value and
// MemoryWorkingSetBytes receives the RSS (working set) value — not swapped.
func TestGetServerDetailMetricsFieldMapping(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-field-map-peer")

	errCreateGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-field-map-grant-1",
		"server-local-1",
		"node-field-map-peer",
		"user-owner",
		"owner",
		"viewer",
		"user-admin",
	)
	if errCreateGrant != nil {
		t.Fatalf("failed to create federated grant: %v", errCreateGrant)
	}

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	// Use distinct, non-zero values for RSS and VMS so a swap would be caught.
	const rssValue uint64 = 111111111
	const vmsValue uint64 = 999999999

	supervisor.NewCommandWithMetrics(
		supervisorInst,
		"server-local-1",
		0,        // cpuPercent
		rssValue, // memoryRSS → should appear as MemoryWorkingSetBytes
		vmsValue, // memoryVMS → should appear as MemoryBytes
		0, 0, 0, 0, 0, 0, 0, 0,
	)

	service := FederationService{
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-field-map-peer",
	})

	request := connect.NewRequest(&xylona.FederationGetServerDetailRequest{
		ServerId: "server-local-1",
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "user-owner")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-field-map-peer")

	response, errDetail := service.GetServerDetail(peerCtx, request)
	if errDetail != nil {
		t.Fatalf("GetServerDetail() error = %v", errDetail)
	}

	s := response.Msg.Server

	if s.MemoryWorkingSetBytes != int64(rssValue) {
		t.Errorf("MemoryWorkingSetBytes = %d, want %d (RSS/working set)", s.MemoryWorkingSetBytes, int64(rssValue))
	}
	if s.MemoryBytes != int64(vmsValue) {
		t.Errorf("MemoryBytes = %d, want %d (VMS/private committed)", s.MemoryBytes, int64(vmsValue))
	}
}

// TestGetServerDetailNotFound verifies that GetServerDetail returns
// CodeNotFound when the requested server does not exist in the database.
func TestGetServerDetailNotFound(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-notfound-peer")

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	service := FederationService{
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-notfound-peer",
	})

	request := connect.NewRequest(&xylona.FederationGetServerDetailRequest{
		ServerId: "nonexistent-server-id",
	})
	// Use super-user path to bypass per-server permission check so the test
	// reaches the DB lookup rather than failing at the permission gate.
	request.Header().Set(helpers.FederationActingUserIDHeader, "user-admin")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-notfound-peer")
	request.Header().Set(helpers.FederationActingSuperHeader, "true")

	_, errDetail := service.GetServerDetail(peerCtx, request)
	if errDetail == nil {
		t.Fatal("GetServerDetail() expected error for nonexistent server, got nil")
	}
	if connect.CodeOf(errDetail) != connect.CodeNotFound {
		t.Errorf("GetServerDetail() code = %v, want %v", connect.CodeOf(errDetail), connect.CodeNotFound)
	}
}

// TestListServerSummariesPopulatesMetrics verifies that when a game server process
// is tracked by the supervisor with known metric values, ListServerSummaries
// returns those values in the FederationServerSummary for that server.
func TestListServerSummariesPopulatesMetrics(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-list-metrics-peer")

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	var (
		wantCPUPercent      float64 = 55.0
		wantMemoryRSS       uint64  = 67108864  // 64 MiB
		wantMemoryVMS       uint64  = 134217728 // 128 MiB
		wantMemoryPercent   float32 = 1.5
		wantCPUCores        int32   = 4
		wantNumThreads      int32   = 8
		wantDiskUsageBytes  uint64  = 536870912 // 512 MiB
		wantIOReadRate      float64 = 2048.0
		wantIOWriteRate     float64 = 1024.0
		wantConnectionCount int32   = 3
	)
	startedAt := time.Now().Unix() - 60

	supervisor.NewCommandWithMetrics(
		supervisorInst,
		"server-local-1",
		wantCPUPercent,
		wantMemoryRSS,
		wantMemoryVMS,
		wantMemoryPercent,
		wantCPUCores,
		wantNumThreads,
		wantDiskUsageBytes,
		wantIOReadRate,
		wantIOWriteRate,
		wantConnectionCount,
		startedAt,
	)

	service := FederationService{
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-list-metrics-peer",
	})

	// No acting identity — returns all servers without per-server permission checks.
	request := connect.NewRequest(&xylona.FederationListServerSummariesRequest{})

	response, errList := service.ListServerSummaries(peerCtx, request)
	if errList != nil {
		t.Fatalf("ListServerSummaries() error = %v", errList)
	}
	if response.Msg == nil || len(response.Msg.Servers) == 0 {
		t.Fatalf("ListServerSummaries() returned no servers")
	}

	var found *xylona.FederationServerSummary
	for _, s := range response.Msg.Servers {
		if s.ServerId == "server-local-1" {
			found = s
			break
		}
	}
	if found == nil {
		t.Fatalf("ListServerSummaries() did not return server-local-1")
	}

	wantCPUPercentInt := int64(wantCPUPercent)
	if found.CpuPercent != wantCPUPercentInt {
		t.Errorf("CpuPercent = %d, want %d", found.CpuPercent, wantCPUPercentInt)
	}
	if found.MemoryBytes != int64(wantMemoryVMS) {
		t.Errorf("MemoryBytes = %d, want %d", found.MemoryBytes, int64(wantMemoryVMS))
	}
	if found.MemoryWorkingSetBytes != int64(wantMemoryRSS) {
		t.Errorf("MemoryWorkingSetBytes = %d, want %d", found.MemoryWorkingSetBytes, int64(wantMemoryRSS))
	}
	if found.MemoryPercent != float64(wantMemoryPercent) {
		t.Errorf("MemoryPercent = %v, want %v", found.MemoryPercent, float64(wantMemoryPercent))
	}
	if found.CpuCores != wantCPUCores {
		t.Errorf("CpuCores = %d, want %d", found.CpuCores, wantCPUCores)
	}
	if found.NumberOfThreads != wantNumThreads {
		t.Errorf("NumberOfThreads = %d, want %d", found.NumberOfThreads, wantNumThreads)
	}
	if found.DiskUsageBytes != int64(wantDiskUsageBytes) {
		t.Errorf("DiskUsageBytes = %d, want %d", found.DiskUsageBytes, int64(wantDiskUsageBytes))
	}
	if found.IoReadRate != wantIOReadRate {
		t.Errorf("IoReadRate = %v, want %v", found.IoReadRate, wantIOReadRate)
	}
	if found.IoWriteRate != wantIOWriteRate {
		t.Errorf("IoWriteRate = %v, want %v", found.IoWriteRate, wantIOWriteRate)
	}
	if found.ConnectionCount != wantConnectionCount {
		t.Errorf("ConnectionCount = %d, want %d", found.ConnectionCount, wantConnectionCount)
	}
	if found.UptimeSeconds < 59 {
		t.Errorf("UptimeSeconds = %d, want >= 59", found.UptimeSeconds)
	}
}

// TestListServerSummariesZeroMetricsWhenOffline verifies that ListServerSummaries
// returns zeroed metric fields for a server when the supervisor has no running
// command for it (i.e., the server is offline).
func TestListServerSummariesZeroMetricsWhenOffline(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-list-zero-metrics-peer")

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	// No command registered — supervisor has no entry for "server-local-1".

	service := FederationService{
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID: "node-list-zero-metrics-peer",
	})

	request := connect.NewRequest(&xylona.FederationListServerSummariesRequest{})

	response, errList := service.ListServerSummaries(peerCtx, request)
	if errList != nil {
		t.Fatalf("ListServerSummaries() error = %v", errList)
	}
	if response.Msg == nil || len(response.Msg.Servers) == 0 {
		t.Fatalf("ListServerSummaries() returned no servers")
	}

	var found *xylona.FederationServerSummary
	for _, s := range response.Msg.Servers {
		if s.ServerId == "server-local-1" {
			found = s
			break
		}
	}
	if found == nil {
		t.Fatalf("ListServerSummaries() did not return server-local-1")
	}

	if found.CpuPercent != 0 {
		t.Errorf("CpuPercent = %d, want 0", found.CpuPercent)
	}
	if found.MemoryBytes != 0 {
		t.Errorf("MemoryBytes = %d, want 0", found.MemoryBytes)
	}
	if found.MemoryWorkingSetBytes != 0 {
		t.Errorf("MemoryWorkingSetBytes = %d, want 0", found.MemoryWorkingSetBytes)
	}
	if found.DiskUsageBytes != 0 {
		t.Errorf("DiskUsageBytes = %d, want 0", found.DiskUsageBytes)
	}
	if found.UptimeSeconds != 0 {
		t.Errorf("UptimeSeconds = %d, want 0", found.UptimeSeconds)
	}
}

// TestGetServerDetailUnauthenticated verifies that GetServerDetail returns
// CodePermissionDenied when no federation peer identity is present.
func TestGetServerDetailUnauthenticated(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	service := FederationService{
		db: fixture.conn,
	}

	// No federation peer identity in context.
	request := connect.NewRequest(&xylona.FederationGetServerDetailRequest{
		ServerId: "server-local-1",
	})

	_, errDetail := service.GetServerDetail(context.Background(), request)
	if errDetail == nil {
		t.Fatal("GetServerDetail() expected error for unauthenticated request, got nil")
	}
	if connect.CodeOf(errDetail) != connect.CodePermissionDenied {
		t.Errorf("GetServerDetail() code = %v, want %v", connect.CodeOf(errDetail), connect.CodePermissionDenied)
	}
}
