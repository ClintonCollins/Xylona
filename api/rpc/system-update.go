package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	xylonadb "github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
	"github.com/ClintonCollins/Xylona/pkg/selfupdate"
	"github.com/ClintonCollins/Xylona/pkg/updater"
	"github.com/ClintonCollins/Xylona/pkg/version"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	systemUpdateStatusPending     = "pending"
	systemUpdateStatusRunning     = "running"
	systemUpdateStatusDraining    = "draining"
	systemUpdateStatusDownloading = "downloading"
	systemUpdateStatusStaging     = "staging"
	systemUpdateStatusApplying    = "applying"
	systemUpdateStatusRestarting  = "restarting"
	systemUpdateStatusSucceeded   = "succeeded"
	systemUpdateStatusFailed      = "failed"

	systemUpdatePhaseCheck     = "check"
	systemUpdatePhasePreflight = "preflight"
	systemUpdatePhaseDrain     = "drain"
	systemUpdatePhaseDownload  = "download"
	systemUpdatePhaseVerify    = "verify"
	systemUpdatePhaseStage     = "stage"
	systemUpdatePhaseApply     = "apply"
	systemUpdatePhaseRestart   = "restart"
	systemUpdatePhaseComplete  = "complete"
	systemUpdatePhaseFailure   = "failure"

	defaultSystemUpdateDownloadTimeout    = 20 * time.Minute
	defaultSystemUpdateRemoteStageTimeout = 10 * time.Minute
	defaultSystemUpdateRemoteApplyTimeout = 30 * time.Second
	defaultSystemUpdateNodePollTimeout    = 2 * time.Minute
	defaultSystemUpdateNodePollInterval   = 3 * time.Second
	defaultSystemUpdateAvailabilityProbes = 8
)

var (
	systemUpdateDownloadTimeout    = defaultSystemUpdateDownloadTimeout
	systemUpdateRemoteStageTimeout = defaultSystemUpdateRemoteStageTimeout
	systemUpdateRemoteApplyTimeout = defaultSystemUpdateRemoteApplyTimeout
	systemUpdateNodePollTimeout    = defaultSystemUpdateNodePollTimeout
	systemUpdateNodePollInterval   = defaultSystemUpdateNodePollInterval
)

type systemUpdateRunInput struct {
	jobID        string
	component    string
	nodeID       string
	target       string
	artifact     updater.Artifact
	artifactOS   string
	artifactArch string
}

// CheckSystemUpdates checks GitHub Releases for controller and node updates.
func (xs *XylonaService) CheckSystemUpdates(ctx context.Context, request *connect.Request[xylona.CheckSystemUpdatesRequest]) (*connect.Response[xylona.CheckSystemUpdatesResponse], error) {
	_, errUser := xs.requireSuperUserForUserManagement(request.Header())
	if errUser != nil {
		return nil, errUser
	}

	release, errRelease := xs.fetchLatestSystemRelease(ctx)
	if errRelease != nil {
		if isLatestSystemReleaseNotFound(errRelease) {
			response, errUnavailable := xs.systemUpdateReleaseUnavailableAvailability(ctx, request.Msg, "no published system update release found for the configured repository")
			if errUnavailable != nil {
				return nil, errUnavailable
			}
			return connect.NewResponse(response), nil
		}
		return nil, connect.NewError(connect.CodeUnavailable, errRelease)
	}

	response := &xylona.CheckSystemUpdatesResponse{}
	nodeID := strings.TrimSpace(request.Msg.GetNodeId())
	if nodeID == "" {
		controllerAvailability := xs.controllerUpdateAvailability(ctx, release)
		response.Updates = append(response.Updates, controllerAvailability)
	}

	if request.Msg.GetIncludeNodes() || nodeID != "" {
		nodeUpdates, errNodes := xs.nodeUpdateAvailability(ctx, release, nodeID)
		if errNodes != nil {
			return nil, errNodes
		}
		response.Updates = append(response.Updates, nodeUpdates...)
	}

	return connect.NewResponse(response), nil
}

// StartSystemUpdate creates an async controller or node update job.
func (xs *XylonaService) StartSystemUpdate(ctx context.Context, request *connect.Request[xylona.StartSystemUpdateRequest]) (*connect.Response[xylona.StartSystemUpdateResponse], error) {
	user, errUser := xs.requireSuperUserForUserManagement(request.Header())
	if errUser != nil {
		return nil, errUser
	}
	if !request.Msg.GetConfirmedDrain() {
		return nil, invalidArg("confirmation is required because game servers will be stopped and left offline")
	}

	release, errRelease := xs.fetchLatestSystemRelease(ctx)
	if errRelease != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errRelease)
	}

	component, errComponent := systemUpdateComponentFromProto(request.Msg.GetComponent())
	if errComponent != nil {
		return nil, errComponent
	}

	input, currentVersion, errPrepare := xs.prepareSystemUpdateStart(ctx, release, component, strings.TrimSpace(request.Msg.GetNodeId()), strings.TrimSpace(request.Msg.GetTargetVersion()))
	if errPrepare != nil {
		return nil, errPrepare
	}

	errActive := xs.ensureNoActiveSystemUpdateJob(component, input.nodeID)
	if errActive != nil {
		return nil, errActive
	}

	job, errCreate := xs.db.CreateSystemUpdateJob(xylonadb.CreateSystemUpdateJobParams{
		Component:         component,
		NodeID:            input.nodeID,
		CurrentVersion:    currentVersion,
		TargetVersion:     input.target,
		Status:            systemUpdateStatusPending,
		Phase:             systemUpdatePhaseCheck,
		ArtifactName:      input.artifact.Name,
		ArtifactSHA256:    input.artifact.SHA256,
		RequestedByUserID: user.ID,
	})
	if errCreate != nil {
		if errors.Is(errCreate, xylonadb.ErrSystemUpdateJobActive) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errCreate)
		}
		return nil, connect.NewError(connect.CodeInternal, errCreate)
	}
	input.jobID = job.ID

	// #nosec G118 -- accepted system update jobs intentionally outlive the request context.
	go xs.runSystemUpdateJob(input)

	return connect.NewResponse(&xylona.StartSystemUpdateResponse{
		Job: xs.systemUpdateJobToProto(ctx, job),
	}), nil
}

// ListSystemUpdateJobs returns recent update jobs.
func (xs *XylonaService) ListSystemUpdateJobs(ctx context.Context, request *connect.Request[xylona.ListSystemUpdateJobsRequest]) (*connect.Response[xylona.ListSystemUpdateJobsResponse], error) {
	_, errUser := xs.requireSuperUserForUserManagement(request.Header())
	if errUser != nil {
		return nil, errUser
	}
	jobs, errJobs := xs.db.ListSystemUpdateJobs(int(request.Msg.GetLimit()), int(request.Msg.GetOffset()))
	if errJobs != nil {
		return nil, connect.NewError(connect.CodeInternal, errJobs)
	}
	resp := &xylona.ListSystemUpdateJobsResponse{Jobs: make([]*xylona.SystemUpdateJob, 0, len(jobs))}
	for _, job := range jobs {
		resp.Jobs = append(resp.Jobs, xs.systemUpdateJobToProto(ctx, job))
	}
	return connect.NewResponse(resp), nil
}

// GetSystemUpdateJob returns a single update job and its event history.
func (xs *XylonaService) GetSystemUpdateJob(ctx context.Context, request *connect.Request[xylona.GetSystemUpdateJobRequest]) (*connect.Response[xylona.GetSystemUpdateJobResponse], error) {
	_, errUser := xs.requireSuperUserForUserManagement(request.Header())
	if errUser != nil {
		return nil, errUser
	}
	job, errJob := xs.db.GetSystemUpdateJob(strings.TrimSpace(request.Msg.GetJobId()))
	if errJob != nil {
		return nil, dbLookup(errJob)
	}
	events, errEvents := xs.db.GetSystemUpdateJobEvents(job.ID)
	if errEvents != nil {
		return nil, connect.NewError(connect.CodeInternal, errEvents)
	}
	resp := &xylona.GetSystemUpdateJobResponse{
		Job:    xs.systemUpdateJobToProto(ctx, job),
		Events: make([]*xylona.SystemUpdateJobEvent, 0, len(events)),
	}
	for _, event := range events {
		resp.Events = append(resp.Events, systemUpdateJobEventToProto(event))
	}
	return connect.NewResponse(resp), nil
}

// ResumeSystemUpdateJobs reconciles update jobs left non-terminal by a controller restart.
func (xs *XylonaService) ResumeSystemUpdateJobs(ctx context.Context) {
	jobs, errJobs := xs.db.ListActiveSystemUpdateJobs()
	if errJobs != nil {
		log.Warn().Err(errJobs).Msg("system update: failed to scan restart jobs")
		return
	}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		nodeID := systemUpdateJobNodeID(job)
		if job.Status != systemUpdateStatusRestarting {
			xs.failResumedSystemUpdateJob(ctx, job, nodeID, "system update did not complete before controller restart", "controller restarted before update handoff completed")
			continue
		}
		switch job.Component {
		case updater.ComponentController:
			xs.resumeControllerSystemUpdateJob(ctx, job, nodeID)
		case updater.ComponentNode:
			jobCopy := job
			go xs.resumeNodeSystemUpdateJob(ctx, jobCopy, nodeID)
		default:
			xs.failResumedSystemUpdateJob(ctx, job, nodeID, "system update has unknown component after controller restart", "unknown update component")
		}
	}
}

func (xs *XylonaService) resumeControllerSystemUpdateJob(ctx context.Context, job *xylonadb.SystemUpdateJob, nodeID string) {
	if updater.CompareVersions(version.SoftwareVersion, job.TargetVersion) >= 0 {
		errRecord := xs.recordSystemUpdateProgress(ctx, job.ID, updater.ComponentController, nodeID, systemUpdateStatusSucceeded, systemUpdatePhaseComplete, 100, "controller restarted on target version", "", true)
		if errRecord != nil {
			log.Warn().Err(errRecord).Str("job_id", job.ID).Msg("system update: failed to complete resumed controller job")
		}
		return
	}
	xs.failResumedSystemUpdateJob(ctx, job, nodeID, "controller restarted before reaching target version", fmt.Sprintf("controller version %s is older than target %s", version.SoftwareVersion, job.TargetVersion))
}

func (xs *XylonaService) resumeNodeSystemUpdateJob(ctx context.Context, job *xylonadb.SystemUpdateJob, nodeID string) {
	if nodeID == "" {
		xs.failResumedSystemUpdateJob(ctx, job, nodeID, "node update could not resume after controller restart", "node id is missing")
		return
	}
	errPoll := xs.pollNodeTargetVersion(ctx, nodeID, job.TargetVersion)
	if errPoll != nil {
		xs.failResumedSystemUpdateJob(ctx, job, nodeID, "node update did not report target version after controller restart", errPoll.Error())
		return
	}
	errRecord := xs.recordSystemUpdateProgress(ctx, job.ID, updater.ComponentNode, nodeID, systemUpdateStatusSucceeded, systemUpdatePhaseComplete, 100, "node updated successfully; game servers remain offline", "", true)
	if errRecord != nil {
		log.Warn().Err(errRecord).Str("job_id", job.ID).Msg("system update: failed to complete resumed node job")
	}
}

func (xs *XylonaService) failResumedSystemUpdateJob(ctx context.Context, job *xylonadb.SystemUpdateJob, nodeID string, message string, errorMessage string) {
	errRecord := xs.recordSystemUpdateProgress(ctx, job.ID, job.Component, nodeID, systemUpdateStatusFailed, systemUpdatePhaseFailure, 100, message, errorMessage, true)
	if errRecord != nil {
		log.Warn().Err(errRecord).Str("job_id", job.ID).Msg("system update: failed to mark resumed job failed")
	}
}

func systemUpdateJobNodeID(job *xylonadb.SystemUpdateJob) string {
	if job == nil {
		return ""
	}
	if !job.NodeID.Valid {
		return ""
	}
	return strings.TrimSpace(job.NodeID.String)
}

func (xs *XylonaService) prepareSystemUpdateStart(ctx context.Context, release *updater.Release, component string, nodeID string, requestedTarget string) (systemUpdateRunInput, string, error) {
	target := release.Version
	if requestedTarget != "" && requestedTarget != release.Version && requestedTarget != release.TagName {
		return systemUpdateRunInput{}, "", invalidArg("target_version does not match the latest release")
	}

	switch component {
	case updater.ComponentController:
		errAvailable := ensureSystemUpdateTargetIsNewer(component, version.SoftwareVersion, target)
		if errAvailable != nil {
			return systemUpdateRunInput{}, "", errAvailable
		}
		manager, errManager := xs.controllerSelfUpdateManager()
		if errManager != nil {
			return systemUpdateRunInput{}, "", connect.NewError(connect.CodeFailedPrecondition, errManager)
		}
		caps := manager.Capabilities()
		artifact, ok := updater.FindArtifact(release, updater.ComponentController, runtime.GOOS, runtime.GOARCH)
		if !ok {
			return systemUpdateRunInput{}, "", connect.NewError(connect.CodeFailedPrecondition, errors.New("no controller artifact is available for this platform"))
		}
		errSHA := xs.fillArtifactSHA(ctx, release, &artifact)
		if errSHA != nil {
			return systemUpdateRunInput{}, "", connect.NewError(connect.CodeFailedPrecondition, errSHA)
		}
		if !caps.Supported {
			return systemUpdateRunInput{}, "", connect.NewError(connect.CodeFailedPrecondition, errors.New(caps.Reason))
		}
		input := systemUpdateRunInput{
			component:    component,
			nodeID:       xs.selfNodeID(),
			target:       target,
			artifact:     artifact,
			artifactOS:   runtime.GOOS,
			artifactArch: runtime.GOARCH,
		}
		return input, version.SoftwareVersion, nil
	case updater.ComponentNode:
		if nodeID == "" {
			return systemUpdateRunInput{}, "", invalidArg("node_id is required for node updates")
		}
		client, errClient := xs.resolveNodeClientByID(nodeID)
		if errClient != nil {
			return systemUpdateRunInput{}, "", errClient
		}
		capsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		caps, errCaps := client.GetUpdateCapabilities(capsCtx)
		if errCaps != nil {
			return systemUpdateRunInput{}, "", connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("node does not support self-update: %w", errCaps))
		}
		errAvailable := ensureSystemUpdateTargetIsNewer(component, caps.CurrentVersion, target)
		if errAvailable != nil {
			return systemUpdateRunInput{}, "", errAvailable
		}
		if !caps.Supported {
			return systemUpdateRunInput{}, "", connect.NewError(connect.CodeFailedPrecondition, errors.New(caps.Reason))
		}
		artifact, ok := updater.FindArtifact(release, updater.ComponentNode, caps.OS, caps.Architecture)
		if !ok {
			return systemUpdateRunInput{}, "", connect.NewError(connect.CodeFailedPrecondition, errors.New("no xylona-node artifact is available for this node platform"))
		}
		errSHA := xs.fillArtifactSHA(ctx, release, &artifact)
		if errSHA != nil {
			return systemUpdateRunInput{}, "", connect.NewError(connect.CodeFailedPrecondition, errSHA)
		}
		input := systemUpdateRunInput{
			component:    component,
			nodeID:       nodeID,
			target:       target,
			artifact:     artifact,
			artifactOS:   caps.OS,
			artifactArch: caps.Architecture,
		}
		return input, caps.CurrentVersion, nil
	default:
		return systemUpdateRunInput{}, "", invalidArg("unsupported update component")
	}
}

func (xs *XylonaService) ensureNoActiveSystemUpdateJob(component string, nodeID string) error {
	activeJob, errActive := xs.db.GetActiveSystemUpdateJob(component, nodeID)
	if errActive == nil {
		message := fmt.Sprintf("system update job %s is already active for this target", activeJob.ID)
		return connect.NewError(connect.CodeFailedPrecondition, errors.New(message))
	}
	if errors.Is(errActive, sql.ErrNoRows) {
		return nil
	}
	return connect.NewError(connect.CodeInternal, errActive)
}

func ensureSystemUpdateTargetIsNewer(component string, currentVersion string, targetVersion string) error {
	if updater.CompareVersions(targetVersion, currentVersion) > 0 {
		return nil
	}
	message := fmt.Sprintf("%s is already on version %s; latest release is %s", component, currentVersion, targetVersion)
	return connect.NewError(connect.CodeFailedPrecondition, errors.New(message))
}

func (xs *XylonaService) runSystemUpdateJob(input systemUpdateRunInput) {
	ctx := context.Background()
	if xs.ctx != nil {
		ctx = xs.ctx
	}

	errRunning := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusRunning, systemUpdatePhasePreflight, 5, "preflight checks passed", "", false)
	if errRunning != nil {
		log.Warn().Err(errRunning).Str("job_id", input.jobID).Msg("system update: failed to mark running")
	}

	errRun := xs.runSystemUpdateJobInner(ctx, input)
	if errRun != nil {
		errFail := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusFailed, systemUpdatePhaseFailure, 100, "update failed", errRun.Error(), true)
		if errFail != nil {
			log.Error().Err(errFail).Str("job_id", input.jobID).Msg("system update: failed to mark job failed")
		}
	}
}

func (xs *XylonaService) runSystemUpdateJobInner(ctx context.Context, input systemUpdateRunInput) error {
	errDrain := xs.drainNodeForSystemUpdate(ctx, input)
	if errDrain != nil {
		return fmt.Errorf("drain target node for system update: %w", errDrain)
	}

	artifactPath, errDownload := xs.downloadSystemUpdateArtifact(ctx, input)
	if errDownload != nil {
		return fmt.Errorf("download system update artifact: %w", errDownload)
	}
	defer func() {
		errRemove := os.Remove(artifactPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Debug().Err(errRemove).Str("path", artifactPath).Msg("system update: failed to remove temp artifact")
		}
	}()

	switch input.component {
	case updater.ComponentController:
		return xs.applyControllerSystemUpdate(ctx, input, artifactPath)
	case updater.ComponentNode:
		return xs.applyRemoteNodeSystemUpdate(ctx, input, artifactPath)
	default:
		return errors.New("unknown update component")
	}
}

func (xs *XylonaService) drainNodeForSystemUpdate(ctx context.Context, input systemUpdateRunInput) error {
	errProgress := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusDraining, systemUpdatePhaseDrain, 10, "stopping game servers on target node", "", false)
	if errProgress != nil {
		return errProgress
	}

	servers, errServers := xs.db.GetGameServersByNodeID(input.nodeID)
	if errServers != nil {
		return fmt.Errorf("list target node game servers: %w", errServers)
	}
	for _, gameServer := range servers {
		xs.actionsInst.StopGameServer(gameServer)
	}
	if len(servers) == 0 {
		return nil
	}

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		allOffline := true
		for _, gameServer := range servers {
			statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			drained, errStatus := xs.gameServerConfirmedOfflineForSystemUpdate(statusCtx, gameServer)
			cancel()
			if errStatus != nil {
				log.Debug().Err(errStatus).Str("game_server_id", gameServer.ID).Msg("system update: drain status unavailable")
			}
			if !drained {
				allOffline = false
				break
			}
		}
		if allOffline {
			errDrained := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusDraining, systemUpdatePhaseDrain, 25, "target node drained; game servers will remain offline", "", false)
			return errDrained
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for game servers to stop: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	return errors.New("timed out waiting for game servers to stop")
}

func (xs *XylonaService) gameServerConfirmedOfflineForSystemUpdate(ctx context.Context, gameServer *models.GameServer) (bool, error) {
	snap, errSnap := xs.resolveProcessSnapshot(ctx, gameServer)
	if errSnap != nil {
		return false, errSnap
	}
	if snap == nil {
		return true, nil
	}
	statusValue, ok := xylona.Status_value[snap.Status]
	if !ok {
		return false, nil
	}
	return xylona.Status(statusValue) == xylona.Status_OFFLINE, nil
}

func (xs *XylonaService) downloadSystemUpdateArtifact(ctx context.Context, input systemUpdateRunInput) (string, error) {
	errProgress := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusDownloading, systemUpdatePhaseDownload, 30, "downloading release artifact", "", false)
	if errProgress != nil {
		return "", errProgress
	}

	tempFile, errTemp := os.CreateTemp("", "xylona-update-*")
	if errTemp != nil {
		return "", fmt.Errorf("create temp artifact: %w", errTemp)
	}
	tempPath := tempFile.Name()
	errClose := tempFile.Close()
	if errClose != nil {
		return "", fmt.Errorf("close temp artifact: %w", errClose)
	}

	var lastPercent int32 = 30
	progress := func(downloaded int64, total int64) {
		if total <= 0 {
			return
		}
		progressValue := int64(30) + (downloaded * 25 / total)
		progressValue = min(progressValue, 55)
		// #nosec G115 -- progressValue is clamped to the int32-safe range 30..55.
		percent := int32(progressValue)
		if percent <= lastPercent+4 {
			return
		}
		lastPercent = percent
		errRecord := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusDownloading, systemUpdatePhaseDownload, percent, "downloading release artifact", "", false)
		if errRecord != nil {
			log.Debug().Err(errRecord).Str("job_id", input.jobID).Msg("system update: failed to record download progress")
		}
	}

	downloadTimeout := systemUpdateDownloadTimeout
	if downloadTimeout <= 0 {
		downloadTimeout = defaultSystemUpdateDownloadTimeout
	}
	downloadCtx, cancelDownload := context.WithTimeout(ctx, downloadTimeout)
	defer cancelDownload()

	_, _, errDownload := updater.DownloadToFile(downloadCtx, http.DefaultClient, input.artifact.DownloadURL, tempPath, input.artifact.SHA256, input.artifact.Size, progress)
	if errDownload != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("download system update artifact: %w", errDownload)
	}
	errVerify := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusDownloading, systemUpdatePhaseVerify, 58, "artifact checksum verified", "", false)
	if errVerify != nil {
		_ = os.Remove(tempPath)
		return "", errVerify
	}
	return tempPath, nil
}

func (xs *XylonaService) applyControllerSystemUpdate(ctx context.Context, input systemUpdateRunInput, artifactPath string) error {
	manager, errManager := xs.controllerSelfUpdateManager()
	if errManager != nil {
		return errManager
	}
	file, errOpen := os.Open(artifactPath)
	if errOpen != nil {
		return fmt.Errorf("open controller artifact: %w", errOpen)
	}
	defer func() {
		_ = file.Close()
	}()

	errStageProgress := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusStaging, systemUpdatePhaseStage, 65, "staging controller update", "", false)
	if errStageProgress != nil {
		return errStageProgress
	}
	stageResult, errStage := manager.Stage(ctx, node.StageSelfUpdateRequest{
		Component:      updater.ComponentController,
		TargetVersion:  input.target,
		OS:             input.artifactOS,
		Architecture:   input.artifactArch,
		ExpectedSize:   input.artifact.Size,
		ExpectedSHA256: input.artifact.SHA256,
		Reader:         file,
	})
	if errStage != nil {
		return fmt.Errorf("stage controller update: %w", errStage)
	}

	errApplyProgress := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusApplying, systemUpdatePhaseApply, 80, "starting controller update helper", "", false)
	if errApplyProgress != nil {
		return errApplyProgress
	}
	errRestarting := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusRestarting, systemUpdatePhaseRestart, 90, "controller is restarting", "", false)
	if errRestarting != nil {
		return errRestarting
	}
	_, errApply := manager.Apply(ctx, node.ApplySelfUpdateRequest{
		StageID:        stageResult.StageID,
		TargetVersion:  input.target,
		ExpectedSHA256: stageResult.SHA256,
	})
	if errApply != nil {
		return fmt.Errorf("apply controller update: %w", errApply)
	}
	return nil
}

func (xs *XylonaService) applyRemoteNodeSystemUpdate(ctx context.Context, input systemUpdateRunInput, artifactPath string) error {
	client, errClient := xs.resolveNodeClientByID(input.nodeID)
	if errClient != nil {
		return errClient
	}
	file, errOpen := os.Open(artifactPath)
	if errOpen != nil {
		return fmt.Errorf("open node artifact: %w", errOpen)
	}
	defer func() {
		_ = file.Close()
	}()

	errStageProgress := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusStaging, systemUpdatePhaseStage, 65, "streaming artifact to node", "", false)
	if errStageProgress != nil {
		return errStageProgress
	}
	stageTimeout := systemUpdateRemoteStageTimeout
	if stageTimeout <= 0 {
		stageTimeout = defaultSystemUpdateRemoteStageTimeout
	}
	stageCtx, cancelStage := context.WithTimeout(ctx, stageTimeout)
	stageResult, errStage := client.StageSelfUpdate(stageCtx, node.StageSelfUpdateRequest{
		Component:      updater.ComponentNode,
		TargetVersion:  input.target,
		OS:             input.artifactOS,
		Architecture:   input.artifactArch,
		ExpectedSize:   input.artifact.Size,
		ExpectedSHA256: input.artifact.SHA256,
		Reader:         file,
	})
	cancelStage()
	if errStage != nil {
		return fmt.Errorf("stage remote node update: %w", errStage)
	}

	errApplyProgress := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusApplying, systemUpdatePhaseApply, 80, "asking node to apply staged update", "", false)
	if errApplyProgress != nil {
		return errApplyProgress
	}
	errRestarting := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusRestarting, systemUpdatePhaseRestart, 90, "remote node update handoff requested; waiting for node to report target version", "", false)
	if errRestarting != nil {
		return errRestarting
	}
	applyTimeout := systemUpdateRemoteApplyTimeout
	if applyTimeout <= 0 {
		applyTimeout = defaultSystemUpdateRemoteApplyTimeout
	}
	applyCtx, cancelApply := context.WithTimeout(ctx, applyTimeout)
	applyResult, errApply := client.ApplySelfUpdate(applyCtx, node.ApplySelfUpdateRequest{
		StageID:        stageResult.StageID,
		TargetVersion:  input.target,
		ExpectedSHA256: stageResult.SHA256,
	})
	cancelApply()
	if errApply != nil {
		if isRemoteApplyHandoffAmbiguous(errApply) {
			errPoll := xs.pollNodeTargetVersion(ctx, input.nodeID, input.target)
			if errPoll == nil {
				return xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusSucceeded, systemUpdatePhaseComplete, 100, "node updated successfully; game servers remain offline", "", true)
			}
			return fmt.Errorf("apply remote node update did not confirm and target version did not appear: %w", errors.Join(errApply, errPoll))
		}
		return fmt.Errorf("apply remote node update: %w", errApply)
	}
	if !applyResult.Accepted {
		return errors.New("node rejected staged update")
	}

	errPoll := xs.pollNodeTargetVersion(ctx, input.nodeID, input.target)
	if errPoll != nil {
		return errPoll
	}
	errSuccess := xs.recordSystemUpdateProgress(ctx, input.jobID, input.component, input.nodeID, systemUpdateStatusSucceeded, systemUpdatePhaseComplete, 100, "node updated successfully; game servers remain offline", "", true)
	return errSuccess
}

func isRemoteApplyHandoffAmbiguous(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	switch connect.CodeOf(err) {
	case connect.CodeCanceled, connect.CodeDeadlineExceeded, connect.CodeUnavailable, connect.CodeUnknown:
		return true
	default:
		return false
	}
}

func (xs *XylonaService) pollNodeTargetVersion(ctx context.Context, nodeID string, targetVersion string) error {
	pollTimeout := systemUpdateNodePollTimeout
	if pollTimeout <= 0 {
		pollTimeout = defaultSystemUpdateNodePollTimeout
	}
	pollInterval := systemUpdateNodePollInterval
	if pollInterval <= 0 {
		pollInterval = defaultSystemUpdateNodePollInterval
	}
	deadline := time.Now().Add(pollTimeout)
	for {
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for node to return on target version")
		}
		client, errClient := xs.resolveNodeClientByID(nodeID)
		if errClient == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			errPing := client.Ping(pingCtx)
			cancel()
			if errPing == nil {
				snapCtx, snapCancel := context.WithTimeout(ctx, 5*time.Second)
				snapshot, errSnapshot := client.GetNodeSnapshot(snapCtx)
				snapCancel()
				if errSnapshot == nil && snapshot != nil && updater.CompareVersions(snapshot.XylonaVersion, targetVersion) >= 0 {
					return nil
				}
			}
		}
		wait := pollInterval
		remaining := time.Until(deadline)
		if remaining < wait {
			wait = remaining
		}
		if wait <= 0 {
			return errors.New("timed out waiting for node to return on target version")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("poll node target version: %w", ctx.Err())
		case <-time.After(wait):
		}
	}
}

func (xs *XylonaService) recordSystemUpdateProgress(ctx context.Context, jobID string, component string, nodeID string, status string, phase string, progress int32, message string, errorMessage string, completed bool) error {
	job, errUpdate := xs.db.UpdateSystemUpdateJobState(jobID, xylonadb.UpdateSystemUpdateJobParams{
		Status:          status,
		Phase:           phase,
		ProgressPercent: progress,
		Error:           errorMessage,
		Completed:       completed,
	})
	if errUpdate != nil {
		return fmt.Errorf("update system update job state: %w", errUpdate)
	}
	_, errEvent := xs.db.AddSystemUpdateJobEvent(jobID, status, phase, progress, message, errorMessage)
	if errEvent != nil {
		return fmt.Errorf("add system update job event: %w", errEvent)
	}
	if xs.systemUpdateBroadcast != nil {
		xs.systemUpdateBroadcast.BroadcastSystemUpdateProgress(&xylona.SystemUpdateProgress{
			JobId:           jobID,
			Component:       systemUpdateComponentToProto(component),
			NodeId:          nodeID,
			Status:          systemUpdateStatusToProto(status),
			Phase:           systemUpdatePhaseToProto(phase),
			ProgressPercent: progress,
			Message:         message,
			TargetVersion:   job.TargetVersion,
			Error:           errorMessage,
		})
	}
	_ = ctx
	return nil
}

func (xs *XylonaService) controllerSelfUpdateManager() (*selfupdate.Manager, error) {
	stageDir := strings.TrimSpace(os.Getenv("XYLONA_UPDATE_STAGE_DIR"))
	manager, errManager := selfupdate.NewDefaultManager(updater.ComponentController, stageDir)
	if errManager != nil {
		return nil, fmt.Errorf("create controller self-update manager: %w", errManager)
	}
	return manager, nil
}

func (xs *XylonaService) fetchLatestSystemRelease(ctx context.Context) (*updater.Release, error) {
	owner := strings.TrimSpace(os.Getenv("XYLONA_UPDATE_GITHUB_OWNER"))
	if owner == "" {
		owner = "ClintonCollins"
	}
	repo := strings.TrimSpace(os.Getenv("XYLONA_UPDATE_GITHUB_REPO"))
	if repo == "" {
		repo = "Xylona"
	}
	client := updater.NewGitHubClient(owner, repo)
	baseURL := strings.TrimSpace(os.Getenv("XYLONA_UPDATE_RELEASE_API_URL"))
	if baseURL != "" {
		client.BaseURL = baseURL
	}
	release, errRelease := client.LatestRelease(ctx)
	if errRelease != nil {
		return nil, fmt.Errorf("fetch latest system release: %w", errRelease)
	}
	return release, nil
}

func isLatestSystemReleaseNotFound(err error) bool {
	var statusErr *updater.HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusNotFound
}

func (xs *XylonaService) systemUpdateReleaseUnavailableAvailability(ctx context.Context, request *xylona.CheckSystemUpdatesRequest, reason string) (*xylona.CheckSystemUpdatesResponse, error) {
	response := &xylona.CheckSystemUpdatesResponse{}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		response.Updates = append(response.Updates, xs.controllerUpdateReleaseUnavailableAvailability(reason))
	}

	if request.GetIncludeNodes() || nodeID != "" {
		nodeUpdates, errNodes := xs.nodeUpdateReleaseUnavailableAvailability(ctx, nodeID, reason)
		if errNodes != nil {
			return nil, errNodes
		}
		response.Updates = append(response.Updates, nodeUpdates...)
	}

	return response, nil
}

func (xs *XylonaService) controllerUpdateReleaseUnavailableAvailability(reason string) *xylona.SystemUpdateAvailability {
	availability := &xylona.SystemUpdateAvailability{
		Component:      xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_CONTROLLER,
		CurrentVersion: version.SoftwareVersion,
		Os:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		Reason:         reason,
	}
	manager, errManager := xs.controllerSelfUpdateManager()
	if errManager != nil {
		return availability
	}
	caps := manager.Capabilities()
	availability.ServiceManagerSupported = caps.ServiceManagerSupported
	availability.InstallPathWritable = caps.InstallPathWritable
	return availability
}

func (xs *XylonaService) nodeUpdateReleaseUnavailableAvailability(ctx context.Context, onlyNodeID string, reason string) ([]*xylona.SystemUpdateAvailability, error) {
	nodes, errNodes := xs.db.GetAllNodes()
	if errNodes != nil {
		return nil, connect.NewError(connect.CodeInternal, errNodes)
	}
	selfNodeID := xs.selfNodeID()
	candidates := make([]*models.Node, 0, len(nodes))
	for _, nodeRow := range nodes {
		if onlyNodeID != "" && nodeRow.ID != onlyNodeID {
			continue
		}
		if nodeRow.ID == selfNodeID {
			continue
		}
		candidates = append(candidates, nodeRow)
	}
	if len(candidates) == 0 {
		return []*xylona.SystemUpdateAvailability{}, nil
	}

	out := make([]*xylona.SystemUpdateAvailability, len(candidates))
	jobs := make(chan int, len(candidates))
	workerCount := min(defaultSystemUpdateAvailabilityProbes, len(candidates))
	var wg sync.WaitGroup
	for range workerCount {
		wg.Go(func() {
			for idx := range jobs {
				out[idx] = xs.singleNodeUpdateReleaseUnavailableAvailability(ctx, candidates[idx], reason)
			}
		})
	}
	for idx := range candidates {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
	return out, nil
}

func (xs *XylonaService) singleNodeUpdateReleaseUnavailableAvailability(ctx context.Context, nodeRow *models.Node, reason string) *xylona.SystemUpdateAvailability {
	availability := &xylona.SystemUpdateAvailability{
		Component: xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_NODE,
		NodeId:    nodeRow.ID,
		NodeName:  nodeRow.Name,
		Reason:    reason,
	}
	client, errClient := xs.resolveNodeClientByID(nodeRow.ID)
	if errClient != nil {
		availability.Reason = errClient.Error()
		return availability
	}
	capsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	caps, errCaps := client.GetUpdateCapabilities(capsCtx)
	cancel()
	if errCaps != nil {
		availability.Reason = "node does not support update capability RPC"
		return availability
	}
	availability.CurrentVersion = caps.CurrentVersion
	availability.Os = caps.OS
	availability.Architecture = caps.Architecture
	availability.ServiceManagerSupported = caps.ServiceManagerSupported
	availability.InstallPathWritable = caps.InstallPathWritable
	return availability
}

func (xs *XylonaService) fillArtifactSHA(ctx context.Context, release *updater.Release, artifact *updater.Artifact) error {
	if artifact == nil {
		return errors.New("artifact is required")
	}
	requireSignature := envBool("XYLONA_UPDATE_REQUIRE_SIGNATURE")
	if strings.TrimSpace(artifact.SHA256) != "" && !requireSignature {
		artifact.SHA256 = normalizeSHA256(artifact.SHA256)
		return nil
	}
	checksumAsset, ok := updater.FindChecksumAsset(release)
	if !ok {
		return updater.ErrChecksumNotFound
	}
	checksumBytes, errDownload := downloadBytes(ctx, checksumAsset.BrowserDownloadURL, 2*1024*1024)
	if errDownload != nil {
		return fmt.Errorf("download checksums: %w", errDownload)
	}
	errSig := maybeVerifyChecksumsSignature(ctx, release, checksumBytes)
	if errSig != nil {
		return fmt.Errorf("verify checksum signature: %w", errSig)
	}
	checksums := updater.ParseChecksums(string(checksumBytes))
	sum := checksums[artifact.Name]
	if sum == "" {
		return fmt.Errorf("%w for %s", updater.ErrChecksumNotFound, artifact.Name)
	}
	artifact.SHA256 = normalizeSHA256(sum)
	return nil
}

func normalizeSHA256(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}

func maybeVerifyChecksumsSignature(ctx context.Context, release *updater.Release, checksumBytes []byte) error {
	requireSignature := envBool("XYLONA_UPDATE_REQUIRE_SIGNATURE")
	if !requireSignature {
		return nil
	}
	signatureAsset, ok := updater.FindChecksumSignatureAsset(release)
	if !ok {
		return errors.New("checksum signature is required but missing from release")
	}
	sigBytes, errDownload := downloadBytes(ctx, signatureAsset.BrowserDownloadURL, 1024*1024)
	if errDownload != nil {
		return fmt.Errorf("download checksum signature: %w", errDownload)
	}
	dir, errDir := os.MkdirTemp("", "xylona-checksums-*")
	if errDir != nil {
		return fmt.Errorf("create checksum verification dir: %w", errDir)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()
	checksumPath := filepath.Join(dir, "checksums.txt")
	signaturePath := filepath.Join(dir, signatureAsset.Name)
	errWriteChecksum := os.WriteFile(checksumPath, checksumBytes, 0o600)
	if errWriteChecksum != nil {
		return fmt.Errorf("write checksums for verification: %w", errWriteChecksum)
	}
	errWriteSignature := os.WriteFile(signaturePath, sigBytes, 0o600)
	if errWriteSignature != nil {
		return fmt.Errorf("write checksum signature for verification: %w", errWriteSignature)
	}
	errVerify := updater.VerifyDetachedSignatureWithGPG(ctx, checksumPath, signaturePath, os.Getenv("XYLONA_UPDATE_GPG_KEYRING"), os.Getenv("XYLONA_UPDATE_GPG_FINGERPRINT"))
	if errVerify != nil {
		return fmt.Errorf("verify checksum signature: %w", errVerify)
	}
	return nil
}

func downloadBytes(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if errReq != nil {
		return nil, fmt.Errorf("create download request: %w", errReq)
	}
	req.Header.Set("User-Agent", "Xylona-Updater")
	resp, errDo := http.DefaultClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("download release asset: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download release asset: status %d", resp.StatusCode)
	}
	data, errRead := io.ReadAll(io.LimitReader(resp.Body, limit))
	if errRead != nil {
		return nil, fmt.Errorf("read release asset: %w", errRead)
	}
	return data, nil
}

func (xs *XylonaService) controllerUpdateAvailability(ctx context.Context, release *updater.Release) *xylona.SystemUpdateAvailability {
	availability := &xylona.SystemUpdateAvailability{
		Component:      xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_CONTROLLER,
		CurrentVersion: version.SoftwareVersion,
		LatestVersion:  release.Version,
		Os:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
	}
	availability.UpdateAvailable = updater.CompareVersions(release.Version, version.SoftwareVersion) > 0
	manager, errManager := xs.controllerSelfUpdateManager()
	if errManager != nil {
		availability.Reason = errManager.Error()
		return availability
	}
	caps := manager.Capabilities()
	availability.ServiceManagerSupported = caps.ServiceManagerSupported
	availability.InstallPathWritable = caps.InstallPathWritable
	artifact, ok := updater.FindArtifact(release, updater.ComponentController, runtime.GOOS, runtime.GOARCH)
	if ok {
		availability.ArtifactName = artifact.Name
		errSHA := xs.fillArtifactSHA(ctx, release, &artifact)
		if errSHA != nil {
			availability.Reason = errSHA.Error()
			return availability
		}
		availability.ArtifactSha256 = artifact.SHA256
	}
	if !ok {
		availability.Reason = "no controller artifact is available for this platform"
		return availability
	}
	if !caps.Supported {
		availability.Reason = caps.Reason
		return availability
	}
	availability.Updateable = availability.GetUpdateAvailable()
	return availability
}

func (xs *XylonaService) nodeUpdateAvailability(ctx context.Context, release *updater.Release, onlyNodeID string) ([]*xylona.SystemUpdateAvailability, error) {
	nodes, errNodes := xs.db.GetAllNodes()
	if errNodes != nil {
		return nil, connect.NewError(connect.CodeInternal, errNodes)
	}
	selfNodeID := xs.selfNodeID()
	candidates := make([]*models.Node, 0, len(nodes))
	for _, nodeRow := range nodes {
		if onlyNodeID != "" && nodeRow.ID != onlyNodeID {
			continue
		}
		if nodeRow.ID == selfNodeID {
			continue
		}
		candidates = append(candidates, nodeRow)
	}
	if len(candidates) == 0 {
		return []*xylona.SystemUpdateAvailability{}, nil
	}

	out := make([]*xylona.SystemUpdateAvailability, len(candidates))
	jobs := make(chan int, len(candidates))
	workerCount := min(defaultSystemUpdateAvailabilityProbes, len(candidates))
	var wg sync.WaitGroup
	for range workerCount {
		wg.Go(func() {
			for idx := range jobs {
				out[idx] = xs.singleNodeUpdateAvailability(ctx, release, candidates[idx])
			}
		})
	}
	for idx := range candidates {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
	return out, nil
}

func (xs *XylonaService) singleNodeUpdateAvailability(ctx context.Context, release *updater.Release, nodeRow *models.Node) *xylona.SystemUpdateAvailability {
	availability := &xylona.SystemUpdateAvailability{
		Component:     xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_NODE,
		NodeId:        nodeRow.ID,
		NodeName:      nodeRow.Name,
		LatestVersion: release.Version,
	}
	client, errClient := xs.resolveNodeClientByID(nodeRow.ID)
	if errClient != nil {
		availability.Reason = errClient.Error()
		return availability
	}
	capsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	caps, errCaps := client.GetUpdateCapabilities(capsCtx)
	cancel()
	if errCaps != nil {
		availability.Reason = "node does not support update capability RPC"
		return availability
	}
	availability.CurrentVersion = caps.CurrentVersion
	availability.Os = caps.OS
	availability.Architecture = caps.Architecture
	availability.ServiceManagerSupported = caps.ServiceManagerSupported
	availability.InstallPathWritable = caps.InstallPathWritable
	availability.UpdateAvailable = updater.CompareVersions(release.Version, caps.CurrentVersion) > 0
	artifact, ok := updater.FindArtifact(release, updater.ComponentNode, caps.OS, caps.Architecture)
	if ok {
		availability.ArtifactName = artifact.Name
		errSHA := xs.fillArtifactSHA(ctx, release, &artifact)
		if errSHA != nil {
			availability.Reason = errSHA.Error()
			return availability
		}
		availability.ArtifactSha256 = artifact.SHA256
	}
	if !ok {
		availability.Reason = "no xylona-node artifact is available for this node platform"
		return availability
	}
	if !caps.Supported {
		availability.Reason = caps.Reason
		return availability
	}
	availability.Updateable = availability.GetUpdateAvailable()
	return availability
}

func (xs *XylonaService) resolveNodeClientByID(nodeID string) (nodeclientCompat, error) {
	if xs.nodeRegistry == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node registry unavailable"))
	}
	client, errGet := xs.nodeRegistry.Get(nodeID)
	if errGet != nil {
		if errors.Is(errGet, noderegistry.ErrNodeNotRegistered) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("node %q is not currently reachable", nodeID))
		}
		return nil, connect.NewError(connect.CodeFailedPrecondition, errGet)
	}
	return client, nil
}

type nodeclientCompat interface {
	Ping(ctx context.Context) error
	GetNodeSnapshot(ctx context.Context) (*node.NodeSnapshot, error)
	GetUpdateCapabilities(ctx context.Context) (node.UpdateCapabilities, error)
	StageSelfUpdate(ctx context.Context, req node.StageSelfUpdateRequest) (node.StageSelfUpdateResult, error)
	ApplySelfUpdate(ctx context.Context, req node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error)
}

func (xs *XylonaService) systemUpdateJobToProto(ctx context.Context, job *xylonadb.SystemUpdateJob) *xylona.SystemUpdateJob {
	if job == nil {
		return &xylona.SystemUpdateJob{}
	}
	out := &xylona.SystemUpdateJob{
		Id:                job.ID,
		Component:         systemUpdateComponentToProto(job.Component),
		CurrentVersion:    job.CurrentVersion,
		TargetVersion:     job.TargetVersion,
		Status:            systemUpdateStatusToProto(job.Status),
		Phase:             systemUpdatePhaseToProto(job.Phase),
		ProgressPercent:   job.ProgressPercent,
		Error:             job.Error.String,
		ArtifactName:      job.ArtifactName.String,
		ArtifactSha256:    job.ArtifactSHA256.String,
		RequestedByUserId: job.RequestedByUserID.String,
		CreatedAt:         timestamppb.New(job.CreatedAt),
		UpdatedAt:         timestamppb.New(job.UpdatedAt),
	}
	if job.NodeID.Valid {
		out.NodeId = job.NodeID.String
		nodeRow, errNode := xs.db.GetNodeByID(job.NodeID.String)
		if errNode == nil {
			out.NodeName = nodeRow.Name
		}
		servers, errServers := xs.db.GetGameServersByNodeID(job.NodeID.String)
		if errServers == nil {
			out.AffectedGameServers = make([]*xylona.GameServer, 0, len(servers))
			for _, server := range servers {
				out.AffectedGameServers = append(out.AffectedGameServers, helpers.GameServerModelToProto(server, xs.versionState))
			}
		}
	}
	if job.RequestedByUserID.Valid {
		user, errUser := xs.db.GetUserByID(job.RequestedByUserID.String)
		if errUser == nil {
			out.RequestedByUserName = user.UserName
		}
	}
	if job.StartedAt.Valid {
		out.StartedAt = timestamppb.New(job.StartedAt.Time)
	}
	if job.CompletedAt.Valid {
		out.CompletedAt = timestamppb.New(job.CompletedAt.Time)
	}
	_ = ctx
	return out
}

func systemUpdateJobEventToProto(event *xylonadb.SystemUpdateJobEvent) *xylona.SystemUpdateJobEvent {
	if event == nil {
		return &xylona.SystemUpdateJobEvent{}
	}
	return &xylona.SystemUpdateJobEvent{
		Id:              event.ID,
		JobId:           event.JobID,
		Status:          systemUpdateStatusToProto(event.Status),
		Phase:           systemUpdatePhaseToProto(event.Phase),
		ProgressPercent: event.ProgressPercent,
		Message:         event.Message.String,
		Error:           event.Error.String,
		CreatedAt:       timestamppb.New(event.CreatedAt),
	}
}

func systemUpdateComponentFromProto(component xylona.SystemUpdateComponent) (string, error) {
	switch component {
	case xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_CONTROLLER:
		return updater.ComponentController, nil
	case xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_NODE:
		return updater.ComponentNode, nil
	default:
		return "", invalidArg("component is required")
	}
}

func systemUpdateComponentToProto(component string) xylona.SystemUpdateComponent {
	switch component {
	case updater.ComponentController:
		return xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_CONTROLLER
	case updater.ComponentNode:
		return xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_NODE
	default:
		return xylona.SystemUpdateComponent_SYSTEM_UPDATE_COMPONENT_UNSPECIFIED
	}
}

func systemUpdateStatusToProto(status string) xylona.SystemUpdateJobStatus {
	switch status {
	case systemUpdateStatusPending:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_PENDING
	case systemUpdateStatusRunning:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_RUNNING
	case systemUpdateStatusDraining:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_DRAINING
	case systemUpdateStatusDownloading:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_DOWNLOADING
	case systemUpdateStatusStaging:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_STAGING
	case systemUpdateStatusApplying:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_APPLYING
	case systemUpdateStatusRestarting:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_RESTARTING
	case systemUpdateStatusSucceeded:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_SUCCEEDED
	case systemUpdateStatusFailed:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_FAILED
	default:
		return xylona.SystemUpdateJobStatus_SYSTEM_UPDATE_JOB_STATUS_UNSPECIFIED
	}
}

func systemUpdatePhaseToProto(phase string) xylona.SystemUpdatePhase {
	switch phase {
	case systemUpdatePhaseCheck:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_CHECK
	case systemUpdatePhasePreflight:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_PREFLIGHT
	case systemUpdatePhaseDrain:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_DRAIN
	case systemUpdatePhaseDownload:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_DOWNLOAD
	case systemUpdatePhaseVerify:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_VERIFY
	case systemUpdatePhaseStage:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_STAGE
	case systemUpdatePhaseApply:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_APPLY
	case systemUpdatePhaseRestart:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_RESTART
	case systemUpdatePhaseComplete:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_COMPLETE
	case systemUpdatePhaseFailure:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_FAILURE
	default:
		return xylona.SystemUpdatePhase_SYSTEM_UPDATE_PHASE_UNSPECIFIED
	}
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
