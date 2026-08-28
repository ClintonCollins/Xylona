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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	xylonadb "github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/updater"
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
			Size:        1,
		},
	})
	if errDownload == nil {
		t.Fatal("downloadSystemUpdateArtifact() error = nil, want timeout error")
	}
	if !strings.Contains(errDownload.Error(), "context deadline exceeded") {
		t.Fatalf("downloadSystemUpdateArtifact() error = %v, want context deadline exceeded", errDownload)
	}
}

func TestValidateSystemUpdateStageResult(t *testing.T) {
	input := systemUpdateRunInput{
		artifact: updater.Artifact{
			Size:   42,
			SHA256: strings.Repeat("a", 64),
		},
	}
	tests := []struct {
		name    string
		result  node.StageSelfUpdateResult
		wantErr string
	}{
		{
			name: "valid result",
			result: node.StageSelfUpdateResult{
				StageID:      "stage-1",
				BytesWritten: 42,
				SHA256:       strings.Repeat("a", 64),
			},
		},
		{
			name: "missing stage ID",
			result: node.StageSelfUpdateResult{
				BytesWritten: 42,
				SHA256:       strings.Repeat("a", 64),
			},
			wantErr: "stage ID",
		},
		{
			name: "size mismatch",
			result: node.StageSelfUpdateResult{
				StageID:      "stage-1",
				BytesWritten: 41,
				SHA256:       strings.Repeat("a", 64),
			},
			wantErr: "size",
		},
		{
			name: "checksum mismatch",
			result: node.StageSelfUpdateResult{
				StageID:      "stage-1",
				BytesWritten: 42,
				SHA256:       strings.Repeat("b", 64),
			},
			wantErr: "checksum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errValidate := validateSystemUpdateStageResult(input, tt.result)
			if tt.wantErr == "" && errValidate != nil {
				t.Fatalf("validateSystemUpdateStageResult() error = %v, want nil", errValidate)
			}
			if tt.wantErr != "" && (errValidate == nil || !strings.Contains(errValidate.Error(), tt.wantErr)) {
				t.Fatalf("validateSystemUpdateStageResult() error = %v, want error containing %q", errValidate, tt.wantErr)
			}
		})
	}
}

func TestReconcileSystemUpdateTempArtifacts(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	stalePath := filepath.Join(tempDir, "xylona-update-stale")
	recentPath := filepath.Join(tempDir, "xylona-update-recent")
	unrelatedPath := filepath.Join(tempDir, "unrelated-stale")
	for _, pathValue := range []string{stalePath, recentPath, unrelatedPath} {
		errWrite := os.WriteFile(pathValue, []byte("artifact"), 0o600)
		if errWrite != nil {
			t.Fatalf("write temp artifact %q: %v", pathValue, errWrite)
		}
	}
	errStaleTime := os.Chtimes(stalePath, now.Add(-systemUpdateTempArtifactMaxAge-time.Minute), now.Add(-systemUpdateTempArtifactMaxAge-time.Minute))
	if errStaleTime != nil {
		t.Fatalf("set stale update artifact time: %v", errStaleTime)
	}
	errRecentTime := os.Chtimes(recentPath, now.Add(-time.Minute), now.Add(-time.Minute))
	if errRecentTime != nil {
		t.Fatalf("set recent update artifact time: %v", errRecentTime)
	}
	errUnrelatedTime := os.Chtimes(unrelatedPath, now.Add(-systemUpdateTempArtifactMaxAge-time.Minute), now.Add(-systemUpdateTempArtifactMaxAge-time.Minute))
	if errUnrelatedTime != nil {
		t.Fatalf("set unrelated artifact time: %v", errUnrelatedTime)
	}

	errReconcile := reconcileSystemUpdateTempArtifacts(tempDir, now)
	if errReconcile != nil {
		t.Fatalf("reconcileSystemUpdateTempArtifacts() error = %v", errReconcile)
	}
	_, errStaleStat := os.Stat(stalePath)
	if !errors.Is(errStaleStat, os.ErrNotExist) {
		t.Fatalf("stale update artifact remains, stat error = %v", errStaleStat)
	}
	for _, pathValue := range []string{recentPath, unrelatedPath} {
		_, errStat := os.Stat(pathValue)
		if errStat != nil {
			t.Fatalf("retained temp artifact %q stat error = %v", pathValue, errStat)
		}
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

func TestSystemUpdateDrainStopsOnGameServerStopFailure(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-remote",
		SnapshotResult: &node.NodeSnapshot{OS: "linux"},
		StopProcessErr: errors.New("remote stop failed"),
	}
	registry := testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local", SnapshotResult: &node.NodeSnapshot{OS: "linux"}},
		remoteClient,
	)
	configureLifecycleActionsForParityTests(t, fixture, registry)
	job := createSystemUpdateJobForTest(
		t,
		fixture.conn,
		updater.ComponentNode,
		"node-remote",
		systemUpdateStatusPending,
		"1.2.0",
	)

	errDrain := fixture.service.drainNodeForSystemUpdate(context.Background(), systemUpdateRunInput{
		jobID:     job.ID,
		component: updater.ComponentNode,
		nodeID:    "node-remote",
	})
	if errDrain == nil {
		t.Fatal("drainNodeForSystemUpdate() error = nil, want stop failure")
	}
	if !strings.Contains(errDrain.Error(), "stop game server \"server-remote-1\"") {
		t.Fatalf("drainNodeForSystemUpdate() error = %v, want server-specific stop failure", errDrain)
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
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")
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
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	configureLifecycleActionsForParityTests(t, fixture, registry)

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
	if len(remoteClient.StopProcessCalls) != 0 {
		t.Fatalf("StopProcess calls = %d, want 0 when staging fails", len(remoteClient.StopProcessCalls))
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
	controllerService := &XylonaService{systemUpdateShutdown: func() {}}
	controllerAvailability := controllerService.controllerUpdateAvailability(context.Background(), controllerRelease)
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
	t.Setenv("XYLONA_UPDATE_ALLOW_UNSIGNED", "true")

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

func TestFillArtifactSHAVerificationPolicy(t *testing.T) {
	artifactName := fmt.Sprintf("xylona_%s_%s", runtime.GOOS, runtime.GOARCH)
	inlineSHA := "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	checksumServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, errWrite := fmt.Fprintf(w, "%s  %s\n", strings.Repeat("b", 64), artifactName)
		if errWrite != nil {
			t.Logf("write checksums response: %v", errWrite)
		}
	}))
	t.Cleanup(checksumServer.Close)

	tests := []struct {
		name          string
		allowUnsigned string
		assets        []updater.Asset
		wantSHA       string
		wantErr       error
		wantErrText   string
	}{
		{
			name:    "default rejects inline digest without checksums",
			assets:  []updater.Asset{{Name: artifactName, SHA256: inlineSHA}},
			wantErr: updater.ErrChecksumNotFound,
		},
		{
			name: "default rejects checksums without signature",
			assets: []updater.Asset{
				{Name: artifactName, SHA256: inlineSHA},
				{Name: "checksums.txt", BrowserDownloadURL: checksumServer.URL},
			},
			wantErrText: "checksum Sigstore bundle is required but missing from release",
		},
		{
			name:          "break glass trusts inline digest",
			allowUnsigned: "true",
			assets:        []updater.Asset{{Name: artifactName, SHA256: inlineSHA}},
			wantSHA:       strings.Repeat("a", 64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XYLONA_UPDATE_ALLOW_UNSIGNED", tt.allowUnsigned)
			release := &updater.Release{TagName: "v1.2.0", Version: "1.2.0", Assets: tt.assets}
			artifact, ok := updater.FindArtifact(release, updater.ComponentController, runtime.GOOS, runtime.GOARCH)
			if !ok {
				t.Fatal("FindArtifact() ok = false, want true")
			}

			errSHA := (&XylonaService{}).fillArtifactSHA(t.Context(), release, &artifact)
			if tt.wantErr != nil && !errors.Is(errSHA, tt.wantErr) {
				t.Fatalf("fillArtifactSHA() error = %v, want %v", errSHA, tt.wantErr)
			}
			if tt.wantErrText != "" && (errSHA == nil || !strings.Contains(errSHA.Error(), tt.wantErrText)) {
				t.Fatalf("fillArtifactSHA() error = %v, want text %q", errSHA, tt.wantErrText)
			}
			if tt.wantErr == nil && tt.wantErrText == "" && errSHA != nil {
				t.Fatalf("fillArtifactSHA() error = %v, want nil", errSHA)
			}
			if artifact.SHA256 != tt.wantSHA && tt.wantSHA != "" {
				t.Fatalf("fillArtifactSHA() SHA256 = %q, want %q", artifact.SHA256, tt.wantSHA)
			}
		})
	}
}

func TestVerifyChecksumsBundleRejectsMalformedBundle(t *testing.T) {
	bundleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, errWrite := w.Write([]byte("not a bundle"))
		if errWrite != nil {
			t.Logf("write bundle response: %v", errWrite)
		}
	}))
	t.Cleanup(bundleServer.Close)

	release := &updater.Release{
		Assets: []updater.Asset{
			{Name: "checksums.txt.sigstore.json", BrowserDownloadURL: bundleServer.URL},
		},
	}
	errVerify := verifyChecksumsBundle(t.Context(), release, []byte("checksums"))
	if errVerify == nil {
		t.Fatal("verifyChecksumsBundle() error = nil, want malformed bundle error")
	}
	if !strings.Contains(errVerify.Error(), "parse Sigstore bundle") {
		t.Fatalf("verifyChecksumsBundle() error = %v, want bundle parsing error", errVerify)
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
