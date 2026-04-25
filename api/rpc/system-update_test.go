package rpc

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	xylonadb "github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/pkg/updater"
	"github.com/ClintonCollins/Xylona/pkg/version"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestPrepareSystemUpdateStartRejectsCurrentVersions(t *testing.T) {
	oldVersion := version.SoftwareVersion
	version.SoftwareVersion = "1.2.3"
	t.Cleanup(func() {
		version.SoftwareVersion = oldVersion
	})

	release := &updater.Release{
		TagName: "v1.2.3",
		Version: "1.2.3",
	}

	t.Run("controller", func(t *testing.T) {
		service := &XylonaService{}
		_, _, errPrepare := service.prepareSystemUpdateStart(context.Background(), release, updater.ComponentController, "", "")
		requireSystemUpdateFailedPrecondition(t, errPrepare)
	})

	t.Run("node", func(t *testing.T) {
		remoteClient := &nodeclient.FakeNodeClient{
			NodeID: "node-remote",
			UpdateCapabilitiesResult: node.UpdateCapabilities{
				Supported:      true,
				CurrentVersion: "1.2.3",
				OS:             runtime.GOOS,
				Architecture:   runtime.GOARCH,
			},
		}
		registry := noderegistry.New("node-local", nil)
		registry.Register(remoteClient)
		service := &XylonaService{nodeRegistry: registry}

		_, _, errPrepare := service.prepareSystemUpdateStart(context.Background(), release, updater.ComponentNode, "node-remote", "")
		requireSystemUpdateFailedPrecondition(t, errPrepare)
	})
}

func TestResumeSystemUpdateJobsReconcilesControllerJobs(t *testing.T) {
	oldVersion := version.SoftwareVersion
	t.Cleanup(func() {
		version.SoftwareVersion = oldVersion
	})

	tests := []struct {
		name           string
		runningVersion string
		jobStatus      string
		targetVersion  string
		wantStatus     string
		wantError      string
	}{
		{
			name:           "restarted on target completes job",
			runningVersion: "1.2.0",
			jobStatus:      systemUpdateStatusRestarting,
			targetVersion:  "1.2.0",
			wantStatus:     systemUpdateStatusSucceeded,
		},
		{
			name:           "restarted below target fails job",
			runningVersion: "1.1.0",
			jobStatus:      systemUpdateStatusRestarting,
			targetVersion:  "1.2.0",
			wantStatus:     systemUpdateStatusFailed,
			wantError:      "older than target",
		},
		{
			name:           "interrupted before handoff fails job",
			runningVersion: "1.1.0",
			jobStatus:      systemUpdateStatusDownloading,
			targetVersion:  "1.2.0",
			wantStatus:     systemUpdateStatusFailed,
			wantError:      "before update handoff completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version.SoftwareVersion = tt.runningVersion
			fixture := newRBACRPCFixture(t)
			job := createSystemUpdateJobForTest(t, fixture.conn, updater.ComponentController, "node-local", tt.jobStatus, tt.targetVersion)

			fixture.service.ResumeSystemUpdateJobs(context.Background())

			got := getSystemUpdateJobForTest(t, fixture.conn, job.ID)
			if got.Status != tt.wantStatus {
				t.Fatalf("resumed job status = %q, want %q", got.Status, tt.wantStatus)
			}
			if !got.CompletedAt.Valid {
				t.Fatal("resumed job CompletedAt.Valid = false, want true")
			}
			if tt.wantError != "" && !strings.Contains(got.Error.String, tt.wantError) {
				t.Fatalf("resumed job error = %q, want it to contain %q", got.Error.String, tt.wantError)
			}
			_, errActive := fixture.conn.GetActiveSystemUpdateJob(updater.ComponentController, "node-local")
			if !errors.Is(errActive, sql.ErrNoRows) {
				t.Fatalf("GetActiveSystemUpdateJob() error = %v, want sql.ErrNoRows", errActive)
			}
		})
	}
}

func TestResumeSystemUpdateJobsCompletesRestartingNodeJob(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	_, errNode := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)`,
		"node-remote", "Remote Node", "https://node.example", true,
	)
	if errNode != nil {
		t.Fatalf("insert remote node: %v", errNode)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		SnapshotResult: &node.NodeSnapshot{
			XylonaVersion: "1.2.0",
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	job := createSystemUpdateJobForTest(t, fixture.conn, updater.ComponentNode, "node-remote", systemUpdateStatusRestarting, "1.2.0")

	fixture.service.ResumeSystemUpdateJobs(context.Background())

	got := waitForSystemUpdateJobStatus(t, fixture.conn, job.ID, systemUpdateStatusSucceeded)
	if !got.CompletedAt.Valid {
		t.Fatal("resumed node job CompletedAt.Valid = false, want true")
	}
	_, errActive := fixture.conn.GetActiveSystemUpdateJob(updater.ComponentNode, "node-remote")
	if !errors.Is(errActive, sql.ErrNoRows) {
		t.Fatalf("GetActiveSystemUpdateJob() error = %v, want sql.ErrNoRows", errActive)
	}
}

func TestDownloadSystemUpdateArtifactTimesOut(t *testing.T) {
	oldTimeout := systemUpdateDownloadTimeout
	systemUpdateDownloadTimeout = 100 * time.Millisecond
	t.Cleanup(func() {
		systemUpdateDownloadTimeout = oldTimeout
	})

	blockedDownload := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(blockedDownload.Close)

	fixture := newRBACRPCFixture(t)
	job := createSystemUpdateJobForTest(t, fixture.conn, updater.ComponentNode, "node-local", systemUpdateStatusPending, "1.2.0")

	_, errDownload := fixture.service.downloadSystemUpdateArtifact(context.Background(), systemUpdateRunInput{
		jobID:     job.ID,
		component: updater.ComponentNode,
		nodeID:    "node-local",
		artifact: updater.Artifact{
			Name:        "xylona-node-test",
			DownloadURL: blockedDownload.URL,
		},
	})
	if errDownload == nil {
		t.Fatal("downloadSystemUpdateArtifact() error = nil, want timeout error")
	}
	if !strings.Contains(errDownload.Error(), "context deadline exceeded") {
		t.Fatalf("downloadSystemUpdateArtifact() error = %v, want context deadline exceeded", errDownload)
	}
}

func TestSystemUpdateDrainRequiresConfirmedOffline(t *testing.T) {
	tests := []struct {
		name    string
		client  *nodeclient.FakeNodeClient
		want    bool
		wantErr bool
	}{
		{
			name:   "missing process is drained",
			client: &nodeclient.FakeNodeClient{NodeID: "node-remote"},
			want:   true,
		},
		{
			name: "offline process is drained",
			client: &nodeclient.FakeNodeClient{
				NodeID:                  "node-remote",
				GetProcessSnapshotFound: true,
				GetProcessSnapshotResult: &node.ProcessSnapshot{
					Status: xylona.Status_OFFLINE.String(),
				},
			},
			want: true,
		},
		{
			name: "unknown process status is not drained",
			client: &nodeclient.FakeNodeClient{
				NodeID:                  "node-remote",
				GetProcessSnapshotFound: true,
				GetProcessSnapshotResult: &node.ProcessSnapshot{
					Status: xylona.Status_UNKNOWN.String(),
				},
			},
		},
		{
			name: "snapshot error is not drained",
			client: &nodeclient.FakeNodeClient{
				NodeID:                "node-remote",
				GetProcessSnapshotErr: errors.New("snapshot unavailable"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &XylonaService{
				nodeRegistry: testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, tt.client),
			}
			got, errDrain := service.gameServerConfirmedOfflineForSystemUpdate(context.Background(), &models.GameServer{
				ID:     "server-remote-1",
				NodeID: "node-remote",
			})
			if tt.wantErr && errDrain == nil {
				t.Fatal("gameServerConfirmedOfflineForSystemUpdate() error = nil, want error")
			}
			if !tt.wantErr && errDrain != nil {
				t.Fatalf("gameServerConfirmedOfflineForSystemUpdate() error = %v, want nil", errDrain)
			}
			if got != tt.want {
				t.Fatalf("gameServerConfirmedOfflineForSystemUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunSystemUpdateJobFailsTerminalWhenRemoteStageTimesOut(t *testing.T) {
	oldStageTimeout := systemUpdateRemoteStageTimeout
	systemUpdateRemoteStageTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		systemUpdateRemoteStageTimeout = oldStageTimeout
	})

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, errWrite := w.Write(content)
		if errWrite != nil {
			t.Logf("write artifact response: %v", errWrite)
		}
	}))
	t.Cleanup(downloadServer.Close)

	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	stageStarted := make(chan struct{})
	remoteClient.StageSelfUpdateFunc = func(ctx context.Context, _ node.StageSelfUpdateRequest) (node.StageSelfUpdateResult, error) {
		close(stageStarted)
		<-ctx.Done()
		return node.StageSelfUpdateResult{}, ctx.Err()
	}
	remoteClient.ApplySelfUpdateFunc = func(context.Context, node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error) {
		return node.ApplySelfUpdateResult{}, errors.New("apply should not be called after stage timeout")
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	job := createSystemUpdateJobForTest(t, fixture.conn, updater.ComponentNode, "node-remote", systemUpdateStatusPending, "1.2.0")
	fixture.service.runSystemUpdateJob(systemUpdateRunInput{
		jobID:     job.ID,
		component: updater.ComponentNode,
		nodeID:    "node-remote",
		target:    "1.2.0",
		artifact: updater.Artifact{
			Name:        "xylona-node-test",
			DownloadURL: downloadServer.URL,
			Size:        int64(len(content)),
			SHA256:      sum,
		},
		artifactOS:   runtime.GOOS,
		artifactArch: runtime.GOARCH,
	})

	select {
	case <-stageStarted:
	default:
		t.Fatal("StageSelfUpdate was not called")
	}
	got := getSystemUpdateJobForTest(t, fixture.conn, job.ID)
	if got.Status != systemUpdateStatusFailed {
		t.Fatalf("job status = %q, want %q", got.Status, systemUpdateStatusFailed)
	}
	if !got.CompletedAt.Valid {
		t.Fatal("job CompletedAt.Valid = false, want true")
	}
	if !strings.Contains(got.Error.String, "context deadline exceeded") {
		t.Fatalf("job error = %q, want context deadline exceeded", got.Error.String)
	}
	_, errActive := fixture.conn.GetActiveSystemUpdateJob(updater.ComponentNode, "node-remote")
	if !errors.Is(errActive, sql.ErrNoRows) {
		t.Fatalf("GetActiveSystemUpdateJob() error = %v, want sql.ErrNoRows", errActive)
	}
}

func TestRunSystemUpdateJobCompletesWhenRemoteApplyTimeoutThenNodeReportsTarget(t *testing.T) {
	oldApplyTimeout := systemUpdateRemoteApplyTimeout
	oldPollTimeout := systemUpdateNodePollTimeout
	oldPollInterval := systemUpdateNodePollInterval
	systemUpdateRemoteApplyTimeout = 50 * time.Millisecond
	systemUpdateNodePollTimeout = 200 * time.Millisecond
	systemUpdateNodePollInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		systemUpdateRemoteApplyTimeout = oldApplyTimeout
		systemUpdateNodePollTimeout = oldPollTimeout
		systemUpdateNodePollInterval = oldPollInterval
	})

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, errWrite := w.Write(content)
		if errWrite != nil {
			t.Logf("write artifact response: %v", errWrite)
		}
	}))
	t.Cleanup(downloadServer.Close)

	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		StageSelfUpdateResult: node.StageSelfUpdateResult{
			StageID:      "stage-1",
			BytesWritten: int64(len(content)),
			SHA256:       sum,
		},
		SnapshotResult: &node.NodeSnapshot{
			XylonaVersion: "1.2.0",
		},
	}
	applyStarted := make(chan struct{})
	remoteClient.ApplySelfUpdateFunc = func(ctx context.Context, _ node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error) {
		close(applyStarted)
		<-ctx.Done()
		return node.ApplySelfUpdateResult{}, ctx.Err()
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	job := createSystemUpdateJobForTest(t, fixture.conn, updater.ComponentNode, "node-remote", systemUpdateStatusPending, "1.2.0")
	fixture.service.runSystemUpdateJob(systemUpdateRunInput{
		jobID:     job.ID,
		component: updater.ComponentNode,
		nodeID:    "node-remote",
		target:    "1.2.0",
		artifact: updater.Artifact{
			Name:        "xylona-node-test",
			DownloadURL: downloadServer.URL,
			Size:        int64(len(content)),
			SHA256:      sum,
		},
		artifactOS:   runtime.GOOS,
		artifactArch: runtime.GOARCH,
	})

	select {
	case <-applyStarted:
	default:
		t.Fatal("ApplySelfUpdate was not called")
	}
	got := getSystemUpdateJobForTest(t, fixture.conn, job.ID)
	if got.Status != systemUpdateStatusSucceeded {
		t.Fatalf("job status = %q, want %q", got.Status, systemUpdateStatusSucceeded)
	}
	if !got.CompletedAt.Valid {
		t.Fatal("job CompletedAt.Valid = false, want true")
	}
	if got.Error.Valid {
		t.Fatalf("job error = %q, want empty", got.Error.String)
	}
	_, errActive := fixture.conn.GetActiveSystemUpdateJob(updater.ComponentNode, "node-remote")
	if !errors.Is(errActive, sql.ErrNoRows) {
		t.Fatalf("GetActiveSystemUpdateJob() error = %v, want sql.ErrNoRows", errActive)
	}
}

func TestCheckSystemUpdatesTreatsMissingLatestReleaseAsUnavailableTarget(t *testing.T) {
	releaseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "{\"message\":\"Not Found\"}", http.StatusNotFound)
	}))
	t.Cleanup(releaseServer.Close)
	t.Setenv("XYLONA_UPDATE_RELEASE_API_URL", releaseServer.URL)

	fixture := newRBACRPCFixture(t)
	req := connect.NewRequest(&xylona.CheckSystemUpdatesRequest{IncludeNodes: true})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errCheck := fixture.service.CheckSystemUpdates(t.Context(), req)
	if errCheck != nil {
		t.Fatalf("CheckSystemUpdates() error = %v, want nil", errCheck)
	}
	updates := resp.Msg.GetUpdates()
	if len(updates) != 1 {
		t.Fatalf("CheckSystemUpdates() updates = %d, want 1", len(updates))
	}
	update := updates[0]
	if update.GetComponent() != xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_CONTROLLER {
		t.Fatalf("update component = %v, want controller", update.GetComponent())
	}
	if update.GetUpdateAvailable() {
		t.Fatal("updateAvailable = true, want false")
	}
	if update.GetUpdateable() {
		t.Fatal("updateable = true, want false")
	}
	if !strings.Contains(update.GetReason(), "no published system update release") {
		t.Fatalf("reason = %q, want missing release reason", update.GetReason())
	}
}

func TestSystemUpdateAvailabilityBlocksChecksumResolutionFailures(t *testing.T) {
	oldVersion := version.SoftwareVersion
	version.SoftwareVersion = "1.0.0"
	t.Cleanup(func() {
		version.SoftwareVersion = oldVersion
	})

	controllerRelease := &updater.Release{
		TagName: "v1.2.0",
		Version: "1.2.0",
		Assets: []updater.Asset{
			{Name: fmt.Sprintf("xylona_%s_%s", runtime.GOOS, runtime.GOARCH), BrowserDownloadURL: "https://example.invalid/xylona"},
		},
	}
	controllerAvailability := (&XylonaService{}).controllerUpdateAvailability(context.Background(), controllerRelease)
	if !controllerAvailability.GetUpdateAvailable() {
		t.Fatal("controller updateAvailable = false, want true")
	}
	if controllerAvailability.GetUpdateable() {
		t.Fatal("controller updateable = true, want false")
	}
	if !strings.Contains(controllerAvailability.GetReason(), updater.ErrChecksumNotFound.Error()) {
		t.Fatalf("controller reason = %q, want checksum error", controllerAvailability.GetReason())
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		UpdateCapabilitiesResult: node.UpdateCapabilities{
			Supported:      true,
			CurrentVersion: "1.0.0",
			OS:             runtime.GOOS,
			Architecture:   runtime.GOARCH,
		},
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(remoteClient)
	service := &XylonaService{nodeRegistry: registry}
	nodeRelease := &updater.Release{
		TagName: "v1.2.0",
		Version: "1.2.0",
		Assets: []updater.Asset{
			{Name: fmt.Sprintf("xylona-node_%s_%s", runtime.GOOS, runtime.GOARCH), BrowserDownloadURL: "https://example.invalid/xylona-node"},
		},
	}
	nodeAvailability := service.singleNodeUpdateAvailability(context.Background(), nodeRelease, &models.Node{ID: "node-remote", Name: "Remote"})
	if !nodeAvailability.GetUpdateAvailable() {
		t.Fatal("node updateAvailable = false, want true")
	}
	if nodeAvailability.GetUpdateable() {
		t.Fatal("node updateable = true, want false")
	}
	if !strings.Contains(nodeAvailability.GetReason(), updater.ErrChecksumNotFound.Error()) {
		t.Fatalf("node reason = %q, want checksum error", nodeAvailability.GetReason())
	}
}

func TestNodeUpdateAvailabilityProbesNodesConcurrently(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	nodeIDs := []string{"node-remote-a", "node-remote-b", "node-remote-c"}
	started := make(chan string, len(nodeIDs))
	releaseProbes := make(chan struct{})
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	for _, nodeID := range nodeIDs {
		insertRemoteNodeForParityTests(t, fixture, nodeID)
		currentNodeID := nodeID
		registry.Register(&nodeclient.FakeNodeClient{
			NodeID: currentNodeID,
			UpdateCapabilitiesFunc: func(ctx context.Context) (node.UpdateCapabilities, error) {
				started <- currentNodeID
				select {
				case <-releaseProbes:
				case <-ctx.Done():
					return node.UpdateCapabilities{}, ctx.Err()
				}
				return node.UpdateCapabilities{
					Supported:      true,
					CurrentVersion: "1.0.0",
					OS:             runtime.GOOS,
					Architecture:   runtime.GOARCH,
				}, nil
			},
		})
	}
	fixture.service.nodeRegistry = registry

	nodes, errNodes := fixture.conn.GetAllNodes()
	if errNodes != nil {
		t.Fatalf("GetAllNodes() error = %v", errNodes)
	}
	expectedOrder := make([]string, 0, len(nodes))
	for _, nodeRow := range nodes {
		if nodeRow.ID == fixture.service.selfNodeID() {
			continue
		}
		expectedOrder = append(expectedOrder, nodeRow.ID)
	}

	ctx := t.Context()
	resultCh := make(chan struct {
		updates []*xylona.SystemUpdateAvailability
		err     error
	}, 1)
	release := &updater.Release{
		TagName: "v1.2.0",
		Version: "1.2.0",
		Assets: []updater.Asset{
			{
				Name:               fmt.Sprintf("xylona-node_%s_%s", runtime.GOOS, runtime.GOARCH),
				BrowserDownloadURL: "https://example.invalid/xylona-node",
				Size:               1,
				SHA256:             "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
	}
	go func() {
		updates, errAvailability := fixture.service.nodeUpdateAvailability(ctx, release, "")
		resultCh <- struct {
			updates []*xylona.SystemUpdateAvailability
			err     error
		}{updates: updates, err: errAvailability}
	}()

	seen := make(map[string]bool, len(nodeIDs))
	for range nodeIDs {
		select {
		case nodeID := <-started:
			seen[nodeID] = true
		case result := <-resultCh:
			t.Fatalf("nodeUpdateAvailability returned before all probes started: updates=%d err=%v", len(result.updates), result.err)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for concurrent capability probes; started=%v", seen)
		}
	}
	for _, nodeID := range nodeIDs {
		if !seen[nodeID] {
			t.Fatalf("probe for %q did not start; started=%v", nodeID, seen)
		}
	}
	close(releaseProbes)

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("nodeUpdateAvailability() error = %v", result.err)
		}
		if len(result.updates) != len(expectedOrder) {
			t.Fatalf("nodeUpdateAvailability() len = %d, want %d", len(result.updates), len(expectedOrder))
		}
		for idx, update := range result.updates {
			if update.GetNodeId() != expectedOrder[idx] {
				t.Fatalf("update[%d].NodeId = %q, want %q", idx, update.GetNodeId(), expectedOrder[idx])
			}
			if !update.GetUpdateable() {
				t.Fatalf("update[%d].Updateable = false, want true", idx)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for node update availability result")
	}
}

func TestFillArtifactSHARequiresChecksumsWhenSignaturesAreRequired(t *testing.T) {
	t.Setenv("XYLONA_UPDATE_REQUIRE_SIGNATURE", "1")

	release := &updater.Release{
		TagName: "v1.2.0",
		Version: "1.2.0",
		Assets: []updater.Asset{
			{
				Name:               fmt.Sprintf("xylona_%s_%s", runtime.GOOS, runtime.GOARCH),
				BrowserDownloadURL: "https://example.invalid/xylona",
				SHA256:             "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
	}
	artifact, ok := updater.FindArtifact(release, updater.ComponentController, runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Fatal("FindArtifact() ok = false, want true")
	}

	errSHA := (&XylonaService{}).fillArtifactSHA(context.Background(), release, &artifact)
	if !errors.Is(errSHA, updater.ErrChecksumNotFound) {
		t.Fatalf("fillArtifactSHA() error = %v, want ErrChecksumNotFound", errSHA)
	}
}

func TestMaybeVerifyChecksumsSignatureRequiresTrustedVerifier(t *testing.T) {
	t.Setenv("XYLONA_UPDATE_REQUIRE_SIGNATURE", "1")

	signatureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, errWrite := w.Write([]byte("signature"))
		if errWrite != nil {
			t.Logf("write signature response: %v", errWrite)
		}
	}))
	t.Cleanup(signatureServer.Close)

	release := &updater.Release{
		Assets: []updater.Asset{
			{Name: "checksums.txt.sig", BrowserDownloadURL: signatureServer.URL},
		},
	}
	errVerify := maybeVerifyChecksumsSignature(context.Background(), release, []byte("checksums"))
	if errVerify == nil {
		t.Fatal("maybeVerifyChecksumsSignature() error = nil, want trusted verifier error")
	}
	if !strings.Contains(errVerify.Error(), "trusted GPG keyring or fingerprint is required") {
		t.Fatalf("maybeVerifyChecksumsSignature() error = %v, want trusted verifier error", errVerify)
	}
}

func requireSystemUpdateFailedPrecondition(t *testing.T, err error) {
	t.Helper()

	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("error code = %s, want %s; error = %v", connect.CodeOf(err), connect.CodeFailedPrecondition, err)
	}
}

func createSystemUpdateJobForTest(t *testing.T, conn *xylonadb.Connection, component string, nodeID string, status string, targetVersion string) *xylonadb.SystemUpdateJob {
	t.Helper()

	job, errCreate := conn.CreateSystemUpdateJob(xylonadb.CreateSystemUpdateJobParams{
		Component:         component,
		NodeID:            nodeID,
		CurrentVersion:    "1.0.0",
		TargetVersion:     targetVersion,
		Status:            status,
		Phase:             systemUpdatePhaseCheck,
		ArtifactName:      component + "-artifact",
		ArtifactSHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RequestedByUserID: "user-admin",
	})
	if errCreate != nil {
		t.Fatalf("CreateSystemUpdateJob() error = %v", errCreate)
	}
	return job
}

func getSystemUpdateJobForTest(t *testing.T, conn *xylonadb.Connection, jobID string) *xylonadb.SystemUpdateJob {
	t.Helper()

	job, errGet := conn.GetSystemUpdateJob(jobID)
	if errGet != nil {
		t.Fatalf("GetSystemUpdateJob() error = %v", errGet)
	}
	return job
}

func waitForSystemUpdateJobStatus(t *testing.T, conn *xylonadb.Connection, jobID string, wantStatus string) *xylonadb.SystemUpdateJob {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	var job *xylonadb.SystemUpdateJob
	for time.Now().Before(deadline) {
		job = getSystemUpdateJobForTest(t, conn, jobID)
		if job.Status == wantStatus {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job == nil {
		t.Fatalf("job %s did not load before deadline", jobID)
	}
	t.Fatalf("job %s status = %q, want %q", jobID, job.Status, wantStatus)
	return nil
}
