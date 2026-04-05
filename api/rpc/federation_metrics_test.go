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
		wantCPUPercent              = 42.5
		wantMemoryRSS       uint64  = 134217728 // 128 MiB working set
		wantMemoryVMS       uint64  = 268435456 // 256 MiB private committed
		wantMemoryPercent   float32 = 3.14
		wantCPUCores        int32   = 8
		wantNumThreads      int32   = 16
		wantDiskUsageBytes  uint64  = 1073741824 // 1 GiB
		wantIOReadRate              = 1024.0
		wantIOWriteRate             = 512.0
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
	if response.Msg == nil || response.Msg.GetServer() == nil {
		t.Fatalf("GetServerDetail() returned empty server")
	}

	s := response.Msg.GetServer()

	// CpuPercent is stored as int64(cpuPct) in the handler (truncated).
	wantCPUPercentInt := int64(wantCPUPercent)
	if s.GetCpuPercent() != wantCPUPercentInt {
		t.Errorf("CpuPercent = %d, want %d", s.GetCpuPercent(), wantCPUPercentInt)
	}

	// MemoryBytes maps to memoryVMS (private committed memory).
	if s.GetMemoryBytes() != int64(wantMemoryVMS) {
		t.Errorf("MemoryBytes = %d, want %d", s.GetMemoryBytes(), int64(wantMemoryVMS))
	}

	// MemoryWorkingSetBytes maps to memoryRSS.
	if s.GetMemoryWorkingSetBytes() != int64(wantMemoryRSS) {
		t.Errorf("MemoryWorkingSetBytes = %d, want %d", s.GetMemoryWorkingSetBytes(), int64(wantMemoryRSS))
	}

	if s.GetMemoryPercent() != float64(wantMemoryPercent) {
		t.Errorf("MemoryPercent = %v, want %v", s.GetMemoryPercent(), float64(wantMemoryPercent))
	}

	if s.GetCpuCores() != wantCPUCores {
		t.Errorf("CpuCores = %d, want %d", s.GetCpuCores(), wantCPUCores)
	}

	if s.GetNumberOfThreads() != wantNumThreads {
		t.Errorf("NumberOfThreads = %d, want %d", s.GetNumberOfThreads(), wantNumThreads)
	}

	if s.GetDiskUsageBytes() != int64(wantDiskUsageBytes) {
		t.Errorf("DiskUsageBytes = %d, want %d", s.GetDiskUsageBytes(), int64(wantDiskUsageBytes))
	}

	if s.GetIoReadRate() != wantIOReadRate {
		t.Errorf("IoReadRate = %v, want %v", s.GetIoReadRate(), wantIOReadRate)
	}

	if s.GetIoWriteRate() != wantIOWriteRate {
		t.Errorf("IoWriteRate = %v, want %v", s.GetIoWriteRate(), wantIOWriteRate)
	}

	if s.GetConnectionCount() != wantConnectionCount {
		t.Errorf("ConnectionCount = %d, want %d", s.GetConnectionCount(), wantConnectionCount)
	}

	// UptimeSeconds must be at least 59 (we started 60 s ago; allow 1 s slack).
	if s.GetUptimeSeconds() < 59 {
		t.Errorf("UptimeSeconds = %d, want >= 59", s.GetUptimeSeconds())
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
	if response.Msg == nil || response.Msg.GetServer() == nil {
		t.Fatalf("GetServerDetail() returned empty server")
	}

	s := response.Msg.GetServer()

	if s.GetCpuPercent() != 0 {
		t.Errorf("CpuPercent = %d, want 0", s.GetCpuPercent())
	}
	if s.GetMemoryBytes() != 0 {
		t.Errorf("MemoryBytes = %d, want 0", s.GetMemoryBytes())
	}
	if s.GetMemoryWorkingSetBytes() != 0 {
		t.Errorf("MemoryWorkingSetBytes = %d, want 0", s.GetMemoryWorkingSetBytes())
	}
	if s.GetMemoryPercent() != 0 {
		t.Errorf("MemoryPercent = %v, want 0", s.GetMemoryPercent())
	}
	if s.GetDiskUsageBytes() != 0 {
		t.Errorf("DiskUsageBytes = %d, want 0", s.GetDiskUsageBytes())
	}
	if s.GetIoReadRate() != 0 {
		t.Errorf("IoReadRate = %v, want 0", s.GetIoReadRate())
	}
	if s.GetIoWriteRate() != 0 {
		t.Errorf("IoWriteRate = %v, want 0", s.GetIoWriteRate())
	}
	if s.GetConnectionCount() != 0 {
		t.Errorf("ConnectionCount = %d, want 0", s.GetConnectionCount())
	}
	if s.GetUptimeSeconds() != 0 {
		t.Errorf("UptimeSeconds = %d, want 0", s.GetUptimeSeconds())
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
	if response.Msg == nil || response.Msg.GetServer() == nil {
		t.Fatalf("GetServerDetail() returned empty server")
	}

	s := response.Msg.GetServer()

	if s.GetCpuPercent() != 0 || s.GetMemoryBytes() != 0 || s.GetUptimeSeconds() != 0 {
		t.Errorf("expected all metrics to be zero with nil supervisor, got cpu=%d mem=%d uptime=%d",
			s.GetCpuPercent(), s.GetMemoryBytes(), s.GetUptimeSeconds())
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

	s := response.Msg.GetServer()

	if s.GetMemoryWorkingSetBytes() != int64(rssValue) {
		t.Errorf("MemoryWorkingSetBytes = %d, want %d (RSS/working set)", s.GetMemoryWorkingSetBytes(), int64(rssValue))
	}
	if s.GetMemoryBytes() != int64(vmsValue) {
		t.Errorf("MemoryBytes = %d, want %d (VMS/private committed)", s.GetMemoryBytes(), int64(vmsValue))
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
		wantCPUPercent              = 55.0
		wantMemoryRSS       uint64  = 67108864  // 64 MiB
		wantMemoryVMS       uint64  = 134217728 // 128 MiB
		wantMemoryPercent   float32 = 1.5
		wantCPUCores        int32   = 4
		wantNumThreads      int32   = 8
		wantDiskUsageBytes  uint64  = 536870912 // 512 MiB
		wantIOReadRate              = 2048.0
		wantIOWriteRate             = 1024.0
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
	if response.Msg == nil || len(response.Msg.GetServers()) == 0 {
		t.Fatalf("ListServerSummaries() returned no servers")
	}

	var found *xylona.FederationServerSummary
	for _, s := range response.Msg.GetServers() {
		if s.GetServerId() == "server-local-1" {
			found = s
			break
		}
	}
	if found == nil {
		t.Fatalf("ListServerSummaries() did not return server-local-1")
	}

	wantCPUPercentInt := int64(wantCPUPercent)
	if found.GetCpuPercent() != wantCPUPercentInt {
		t.Errorf("CpuPercent = %d, want %d", found.GetCpuPercent(), wantCPUPercentInt)
	}
	if found.GetMemoryBytes() != int64(wantMemoryVMS) {
		t.Errorf("MemoryBytes = %d, want %d", found.GetMemoryBytes(), int64(wantMemoryVMS))
	}
	if found.GetMemoryWorkingSetBytes() != int64(wantMemoryRSS) {
		t.Errorf("MemoryWorkingSetBytes = %d, want %d", found.GetMemoryWorkingSetBytes(), int64(wantMemoryRSS))
	}
	if found.GetMemoryPercent() != float64(wantMemoryPercent) {
		t.Errorf("MemoryPercent = %v, want %v", found.GetMemoryPercent(), float64(wantMemoryPercent))
	}
	if found.GetCpuCores() != wantCPUCores {
		t.Errorf("CpuCores = %d, want %d", found.GetCpuCores(), wantCPUCores)
	}
	if found.GetNumberOfThreads() != wantNumThreads {
		t.Errorf("NumberOfThreads = %d, want %d", found.GetNumberOfThreads(), wantNumThreads)
	}
	if found.GetDiskUsageBytes() != int64(wantDiskUsageBytes) {
		t.Errorf("DiskUsageBytes = %d, want %d", found.GetDiskUsageBytes(), int64(wantDiskUsageBytes))
	}
	if found.GetIoReadRate() != wantIOReadRate {
		t.Errorf("IoReadRate = %v, want %v", found.GetIoReadRate(), wantIOReadRate)
	}
	if found.GetIoWriteRate() != wantIOWriteRate {
		t.Errorf("IoWriteRate = %v, want %v", found.GetIoWriteRate(), wantIOWriteRate)
	}
	if found.GetConnectionCount() != wantConnectionCount {
		t.Errorf("ConnectionCount = %d, want %d", found.GetConnectionCount(), wantConnectionCount)
	}
	if found.GetUptimeSeconds() < 59 {
		t.Errorf("UptimeSeconds = %d, want >= 59", found.GetUptimeSeconds())
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
	if response.Msg == nil || len(response.Msg.GetServers()) == 0 {
		t.Fatalf("ListServerSummaries() returned no servers")
	}

	var found *xylona.FederationServerSummary
	for _, s := range response.Msg.GetServers() {
		if s.GetServerId() == "server-local-1" {
			found = s
			break
		}
	}
	if found == nil {
		t.Fatalf("ListServerSummaries() did not return server-local-1")
	}

	if found.GetCpuPercent() != 0 {
		t.Errorf("CpuPercent = %d, want 0", found.GetCpuPercent())
	}
	if found.GetMemoryBytes() != 0 {
		t.Errorf("MemoryBytes = %d, want 0", found.GetMemoryBytes())
	}
	if found.GetMemoryWorkingSetBytes() != 0 {
		t.Errorf("MemoryWorkingSetBytes = %d, want 0", found.GetMemoryWorkingSetBytes())
	}
	if found.GetDiskUsageBytes() != 0 {
		t.Errorf("DiskUsageBytes = %d, want 0", found.GetDiskUsageBytes())
	}
	if found.GetUptimeSeconds() != 0 {
		t.Errorf("UptimeSeconds = %d, want 0", found.GetUptimeSeconds())
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
