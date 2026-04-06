package rpc

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// TestStreamServerUpdates_SnapshotOnConnect verifies that opening a
// StreamServerUpdates stream immediately receives a snapshot event containing
// the current state of all game servers on the node.
func TestStreamServerUpdates_SnapshotOnConnect(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	// Inject a command with known metrics for the seeded game server.
	startedAt := time.Now().Unix() - 30
	supervisor.NewCommandWithMetrics(
		supervisorInst,
		"server-local-1",
		10.0,  // cpuPercent
		1024,  // memoryRSS
		2048,  // memoryVMS
		1.0,   // memoryPercent
		4,     // cpuCores
		8,     // numThreads
		4096,  // diskUsageBytes
		100.0, // ioReadRate
		50.0,  // ioWriteRate
		2,     // connectionCount
		startedAt,
	)

	service := FederationService{
		ctx:            context.Background(),
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	// Middleware that injects the federation peer identity into every request,
	// simulating what the mTLS middleware does in production.
	injectPeerIdentity := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), federationPeerIdentityKey, FederationPeerIdentity{
				NodeID: "node-metrics-peer",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewFederationHandler(&service)
	mux.Handle(path, injectPeerIdentity(handler))

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	// Seed a remote node so the peer identity is recognized.
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-metrics-peer")

	client := xylonaconnect.NewFederationClient(http.DefaultClient, ts.URL)

	stream, errStream := client.StreamServerUpdates(
		context.Background(),
		connect.NewRequest(&xylona.FederationStreamServerUpdatesRequest{}),
	)
	if errStream != nil {
		t.Fatalf("StreamServerUpdates() connection error = %v", errStream)
	}
	t.Cleanup(func() {
		errClose := stream.Close()
		if errClose != nil {
			t.Logf("stream close error (may be expected): %v", errClose)
		}
	})

	// The first message must be a snapshot of all servers.
	if !stream.Receive() {
		t.Fatalf("expected to receive first message, got error: %v", stream.Err())
	}

	event := stream.Msg()
	if event == nil {
		t.Fatal("received nil event message")
	}

	snapshot := event.GetSnapshot()
	if snapshot == nil {
		t.Fatalf("expected first event to be a Snapshot, got %T", event.GetEvent())
	}

	if len(snapshot.GetServers()) == 0 {
		t.Fatal("snapshot contains no servers, expected at least one")
	}

	if snapshot.GetServers()[0].GetServerId() != "server-local-1" {
		t.Errorf("first server ID = %q, want %q", snapshot.GetServers()[0].GetServerId(), "server-local-1")
	}
}

// TestStreamServerUpdates_StatusChangeEvent verifies that after the initial
// snapshot, the stream delivers a StatusChange event when a game server's
// status changes on the supervisor.
func TestStreamServerUpdates_StatusChangeEvent(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	startedAt := time.Now().Unix() - 30
	supervisor.NewCommandWithMetrics(
		supervisorInst,
		"server-local-1",
		10.0,  // cpuPercent
		1024,  // memoryRSS
		2048,  // memoryVMS
		1.0,   // memoryPercent
		4,     // cpuCores
		8,     // numThreads
		4096,  // diskUsageBytes
		100.0, // ioReadRate
		50.0,  // ioWriteRate
		2,     // connectionCount
		startedAt,
	)

	service := FederationService{
		ctx:            context.Background(),
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	injectPeerIdentity := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), federationPeerIdentityKey, FederationPeerIdentity{
				NodeID: "node-metrics-peer",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewFederationHandler(&service)
	mux.Handle(path, injectPeerIdentity(handler))

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-metrics-peer")

	client := xylonaconnect.NewFederationClient(http.DefaultClient, ts.URL)

	stream, errStream := client.StreamServerUpdates(
		context.Background(),
		connect.NewRequest(&xylona.FederationStreamServerUpdatesRequest{}),
	)
	if errStream != nil {
		t.Fatalf("StreamServerUpdates() connection error = %v", errStream)
	}
	t.Cleanup(func() {
		errClose := stream.Close()
		if errClose != nil {
			t.Logf("stream close error (may be expected): %v", errClose)
		}
	})

	// Receive and discard the initial snapshot.
	if !stream.Receive() {
		t.Fatalf("expected to receive snapshot message, got error: %v", stream.Err())
	}
	snapshot := stream.Msg().GetSnapshot()
	if snapshot == nil {
		t.Fatal("first message was not a snapshot")
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				supervisor.TriggerStatusNotification(supervisorInst, "server-local-1", xylona.Status_OFFLINE, xylona.Status_ONLINE)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		recvDone := make(chan bool, 1)
		go func() {
			recvDone <- stream.Receive()
		}()

		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for status change event")
		case ok := <-recvDone:
			if !ok {
				t.Fatalf("stream ended while waiting for status change event: %v", stream.Err())
			}
		}

		event := stream.Msg()
		if event == nil {
			t.Fatal("received nil event message")
		}

		statusChange := event.GetStatusChange()
		if statusChange == nil {
			continue
		}

		if statusChange.GetServerId() != "server-local-1" {
			t.Errorf("StatusChange server ID = %q, want %q", statusChange.GetServerId(), "server-local-1")
		}
		if statusChange.GetStatus() != xylona.Status_ONLINE {
			t.Errorf("StatusChange status = %v, want %v", statusChange.GetStatus(), xylona.Status_ONLINE)
		}
		break
	}
}

// TestStreamServerUpdates_MetricsUpdate verifies that the handler sends a
// MetricsUpdate event when a running server's metrics change between ticker
// ticks. The test overrides the metrics ticker interval to keep runtime short.
func TestStreamServerUpdates_MetricsUpdate(t *testing.T) {
	// Speed up the metrics ticker for testing.
	origInterval := streamMetricsInterval
	streamMetricsInterval = 250 * time.Millisecond
	t.Cleanup(func() { streamMetricsInterval = origInterval })

	fixture := newRBACRPCFixture(t)

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	// Inject a command with initial metrics.
	startedAt := time.Now().Unix() - 30
	supervisor.NewCommandWithMetrics(
		supervisorInst,
		"server-local-1",
		10.0,  // cpuPercent
		1024,  // memoryRSS
		2048,  // memoryVMS
		1.0,   // memoryPercent
		4,     // cpuCores
		8,     // numThreads
		4096,  // diskUsageBytes
		100.0, // ioReadRate
		50.0,  // ioWriteRate
		2,     // connectionCount
		startedAt,
	)

	service := FederationService{
		ctx:            context.Background(),
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	injectPeerIdentity := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), federationPeerIdentityKey, FederationPeerIdentity{
				NodeID: "node-metrics-peer",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewFederationHandler(&service)
	mux.Handle(path, injectPeerIdentity(handler))

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-metrics-peer")

	client := xylonaconnect.NewFederationClient(http.DefaultClient, ts.URL)

	stream, errStream := client.StreamServerUpdates(
		context.Background(),
		connect.NewRequest(&xylona.FederationStreamServerUpdatesRequest{}),
	)
	if errStream != nil {
		t.Fatalf("StreamServerUpdates() connection error = %v", errStream)
	}
	t.Cleanup(func() {
		errClose := stream.Close()
		if errClose != nil {
			t.Logf("stream close error (may be expected): %v", errClose)
		}
	})

	// Receive and discard the initial snapshot.
	if !stream.Receive() {
		t.Fatalf("expected to receive snapshot message, got error: %v", stream.Err())
	}
	snapshot := stream.Msg().GetSnapshot()
	if snapshot == nil {
		t.Fatal("first message was not a snapshot")
	}

	// Update metrics to clearly different values and wait for the next metrics event.
	supervisor.UpdateCommandMetrics(
		supervisorInst,
		"server-local-1",
		50.0,  // cpuPercent — changed from 10.0
		8192,  // memoryRSS — changed from 1024
		16384, // memoryVMS — changed from 2048
		5.0,   // memoryPercent — changed from 1.0
		4,     // cpuCores — unchanged
		12,    // numThreads — changed from 8
		65536, // diskUsageBytes — changed from 4096
		500.0, // ioReadRate — changed from 100.0
		250.0, // ioWriteRate — changed from 50.0
		10,    // connectionCount — changed from 2
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		recvDone := make(chan bool, 1)
		go func() {
			recvDone <- stream.Receive()
		}()

		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for MetricsUpdate event")
		case ok := <-recvDone:
			if !ok {
				t.Fatalf("stream ended while waiting for MetricsUpdate: %v", stream.Err())
			}
		}

		event := stream.Msg()
		if event == nil {
			t.Fatal("received nil event message")
		}

		metricsUpdate := event.GetMetricsUpdate()
		if metricsUpdate == nil {
			continue
		}

		if metricsUpdate.GetServerId() != "server-local-1" {
			t.Errorf("MetricsUpdate server ID = %q, want %q", metricsUpdate.GetServerId(), "server-local-1")
		}

		metrics := metricsUpdate.GetMetrics()
		if metrics == nil {
			t.Fatal("MetricsUpdate event has nil metrics")
		}

		if math.Abs(metrics.GetCpuPercent()-50.0) > 0.1 {
			t.Errorf("MetricsUpdate CPU percent = %v, want approximately 50.0", metrics.GetCpuPercent())
		}
		break
	}
}

// TestStreamServerUpdates_Heartbeat verifies that the handler sends periodic
// heartbeat events on the stream. The test overrides the heartbeat interval to
// keep the test runtime short.
func TestStreamServerUpdates_Heartbeat(t *testing.T) {
	// Speed up the heartbeat ticker for testing.
	origInterval := streamHeartbeatInterval
	streamHeartbeatInterval = 250 * time.Millisecond
	t.Cleanup(func() { streamHeartbeatInterval = origInterval })

	fixture := newRBACRPCFixture(t)

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	service := FederationService{
		ctx:            context.Background(),
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	injectPeerIdentity := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), federationPeerIdentityKey, FederationPeerIdentity{
				NodeID: "node-heartbeat-peer",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewFederationHandler(&service)
	mux.Handle(path, injectPeerIdentity(handler))

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-heartbeat-peer")

	client := xylonaconnect.NewFederationClient(http.DefaultClient, ts.URL)

	stream, errStream := client.StreamServerUpdates(
		context.Background(),
		connect.NewRequest(&xylona.FederationStreamServerUpdatesRequest{}),
	)
	if errStream != nil {
		t.Fatalf("StreamServerUpdates() connection error = %v", errStream)
	}
	t.Cleanup(func() {
		errClose := stream.Close()
		if errClose != nil {
			t.Logf("stream close error (may be expected): %v", errClose)
		}
	})

	// Receive and discard the initial snapshot.
	if !stream.Receive() {
		t.Fatalf("expected to receive snapshot message, got error: %v", stream.Err())
	}
	snapshot := stream.Msg().GetSnapshot()
	if snapshot == nil {
		t.Fatal("first message was not a snapshot")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		recvDone := make(chan bool, 1)
		go func() {
			recvDone <- stream.Receive()
		}()

		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for Heartbeat event")
		case ok := <-recvDone:
			if !ok {
				t.Fatalf("stream ended while waiting for Heartbeat: %v", stream.Err())
			}
		}

		event := stream.Msg()
		if event == nil {
			t.Fatal("received nil event message")
		}

		heartbeat := event.GetHeartbeat()
		if heartbeat != nil {
			break
		}
	}
}

// TestStreamServerUpdates_SnapshotOnServerCreate verifies that when a game
// server is created (signaled via the eventbus), the handler re-sends a full
// snapshot event on the stream. This allows federated peers to learn about
// newly created servers without reconnecting.
func TestStreamServerUpdates_SnapshotOnServerCreate(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	supervisorInst, errSupervisor := supervisor.New(context.Background())
	if errSupervisor != nil {
		t.Fatalf("failed to create supervisor instance: %v", errSupervisor)
	}

	service := FederationService{
		ctx:            context.Background(),
		db:             fixture.conn,
		supervisorInst: supervisorInst,
	}

	injectPeerIdentity := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), federationPeerIdentityKey, FederationPeerIdentity{
				NodeID: "node-create-peer",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewFederationHandler(&service)
	mux.Handle(path, injectPeerIdentity(handler))

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-create-peer")

	client := xylonaconnect.NewFederationClient(http.DefaultClient, ts.URL)

	stream, errStream := client.StreamServerUpdates(
		context.Background(),
		connect.NewRequest(&xylona.FederationStreamServerUpdatesRequest{}),
	)
	if errStream != nil {
		t.Fatalf("StreamServerUpdates() connection error = %v", errStream)
	}
	t.Cleanup(func() {
		errClose := stream.Close()
		if errClose != nil {
			t.Logf("stream close error (may be expected): %v", errClose)
		}
	})

	// Receive and discard the initial snapshot.
	if !stream.Receive() {
		t.Fatalf("expected to receive snapshot message, got error: %v", stream.Err())
	}
	initialSnapshot := stream.Msg().GetSnapshot()
	if initialSnapshot == nil {
		t.Fatal("first message was not a snapshot")
	}
	initialCount := len(initialSnapshot.GetServers())

	// Publish create notifications while waiting for a re-sent snapshot.
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				eventbus.Get().Publish(eventbus.TopicGameServerCreated, "new-server-id")
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		recvDone := make(chan bool, 1)
		go func() {
			recvDone <- stream.Receive()
		}()

		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for Snapshot event after game server creation")
		case ok := <-recvDone:
			if !ok {
				t.Fatalf("stream ended while waiting for Snapshot: %v", stream.Err())
			}
		}

		event := stream.Msg()
		if event == nil {
			t.Fatal("received nil event message")
		}

		snapshot := event.GetSnapshot()
		if snapshot == nil {
			continue
		}

		// The handler should have re-sent a full snapshot. At minimum it should
		// contain the same servers as the initial snapshot (the DB hasn't changed
		// so the count should match). The key assertion is that we received a
		// snapshot at all — proving the handler reacts to the eventbus topic.
		if len(snapshot.GetServers()) < initialCount {
			t.Errorf("re-sent snapshot has %d servers, want at least %d", len(snapshot.GetServers()), initialCount)
		}
		break
	}
}
