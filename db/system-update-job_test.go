package db

import (
	"errors"
	"testing"
)

func TestSystemUpdateJobLifecycle(t *testing.T) {
	conn := newRBACMigratedConnection(t, "system-update-job.sqlite")
	seedRBACFixture(t, conn)

	job, errCreate := conn.CreateSystemUpdateJob(CreateSystemUpdateJobParams{
		Component:         "node",
		NodeID:            "node-local",
		CurrentVersion:    "1.0.0",
		TargetVersion:     "1.1.0",
		Status:            "pending",
		Phase:             "check",
		ArtifactName:      "xylona-node_linux_amd64",
		ArtifactSHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RequestedByUserID: "user-admin",
	})
	if errCreate != nil {
		t.Fatalf("CreateSystemUpdateJob() error = %v", errCreate)
	}
	if job.ID == "" {
		t.Fatal("CreateSystemUpdateJob().ID is empty")
	}
	if !job.NodeID.Valid || job.NodeID.String != "node-local" {
		t.Fatalf("CreateSystemUpdateJob().NodeID = (%q, %v), want node-local", job.NodeID.String, job.NodeID.Valid)
	}

	updated, errUpdate := conn.UpdateSystemUpdateJobState(job.ID, UpdateSystemUpdateJobParams{
		Status:          "downloading",
		Phase:           "download",
		ProgressPercent: 40,
	})
	if errUpdate != nil {
		t.Fatalf("UpdateSystemUpdateJobState() error = %v", errUpdate)
	}
	if updated.Status != "downloading" || updated.ProgressPercent != 40 {
		t.Fatalf("updated job = status %q progress %d", updated.Status, updated.ProgressPercent)
	}
	if !updated.StartedAt.Valid {
		t.Fatal("updated job StartedAt.Valid = false, want true")
	}

	_, errEvent := conn.AddSystemUpdateJobEvent(job.ID, "downloading", "download", 40, "downloading", "")
	if errEvent != nil {
		t.Fatalf("AddSystemUpdateJobEvent() error = %v", errEvent)
	}
	events, errEvents := conn.GetSystemUpdateJobEvents(job.ID)
	if errEvents != nil {
		t.Fatalf("GetSystemUpdateJobEvents() error = %v", errEvents)
	}
	if len(events) != 1 {
		t.Fatalf("GetSystemUpdateJobEvents() len = %d, want 1", len(events))
	}

	completed, errComplete := conn.UpdateSystemUpdateJobState(job.ID, UpdateSystemUpdateJobParams{
		Status:          "failed",
		Phase:           "failure",
		ProgressPercent: 100,
		Error:           "boom",
		Completed:       true,
	})
	if errComplete != nil {
		t.Fatalf("UpdateSystemUpdateJobState(completed) error = %v", errComplete)
	}
	if !completed.CompletedAt.Valid {
		t.Fatal("completed job CompletedAt.Valid = false, want true")
	}
	if !completed.Error.Valid || completed.Error.String != "boom" {
		t.Fatalf("completed job Error = (%q, %v), want boom", completed.Error.String, completed.Error.Valid)
	}
}

func TestSystemUpdateJobRejectsDuplicateActiveTarget(t *testing.T) {
	conn := newRBACMigratedConnection(t, "system-update-job-active.sqlite")
	seedRBACFixture(t, conn)

	first, errCreate := conn.CreateSystemUpdateJob(CreateSystemUpdateJobParams{
		Component:         "node",
		NodeID:            "node-local",
		CurrentVersion:    "1.0.0",
		TargetVersion:     "1.1.0",
		Status:            "pending",
		Phase:             "check",
		ArtifactName:      "xylona-node_linux_amd64",
		ArtifactSHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RequestedByUserID: "user-admin",
	})
	if errCreate != nil {
		t.Fatalf("CreateSystemUpdateJob(first) error = %v", errCreate)
	}

	_, errDuplicate := conn.CreateSystemUpdateJob(CreateSystemUpdateJobParams{
		Component:         "node",
		NodeID:            "node-local",
		CurrentVersion:    "1.0.0",
		TargetVersion:     "1.1.0",
		Status:            "pending",
		Phase:             "check",
		ArtifactName:      "xylona-node_linux_amd64",
		ArtifactSHA256:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		RequestedByUserID: "user-admin",
	})
	if !errors.Is(errDuplicate, ErrSystemUpdateJobActive) {
		t.Fatalf("CreateSystemUpdateJob(duplicate) error = %v, want ErrSystemUpdateJobActive", errDuplicate)
	}

	_, errComplete := conn.UpdateSystemUpdateJobState(first.ID, UpdateSystemUpdateJobParams{
		Status:          "failed",
		Phase:           "failure",
		ProgressPercent: 100,
		Completed:       true,
	})
	if errComplete != nil {
		t.Fatalf("UpdateSystemUpdateJobState(first complete) error = %v", errComplete)
	}

	next, errNext := conn.CreateSystemUpdateJob(CreateSystemUpdateJobParams{
		Component:         "node",
		NodeID:            "node-local",
		CurrentVersion:    "1.0.0",
		TargetVersion:     "1.1.0",
		Status:            "pending",
		Phase:             "check",
		ArtifactName:      "xylona-node_linux_amd64",
		ArtifactSHA256:    "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		RequestedByUserID: "user-admin",
	})
	if errNext != nil {
		t.Fatalf("CreateSystemUpdateJob(after complete) error = %v", errNext)
	}
	if next.ID == first.ID {
		t.Fatal("CreateSystemUpdateJob(after complete) reused first job ID")
	}
}
