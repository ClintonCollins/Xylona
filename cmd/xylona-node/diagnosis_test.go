package main

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/diagnosis"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestNodeFailureEvidenceTransport(t *testing.T) {
	if testing.Short() {
		t.Skip("process and TLS integration test")
	}
	for _, stage := range []string{diagnosis.StageLaunch, diagnosis.StageRuntime} {
		t.Run(stage, func(t *testing.T) {
			url, fingerprint := newTestServer(t, "node-test-secret")
			client, errClient := nodeclient.NewGRPCClient("node", url, fingerprint, "node-test-secret")
			if errClient != nil {
				t.Fatal(errClient)
			}
			t.Cleanup(func() {
				errClose := client.Close()
				if errClose != nil {
					t.Error(errClose)
				}
			})
			attempt := time.Now().Add(-time.Minute).UTC()
			cfg := node.ProcessConfig{
				ID: "test-failure", ExecutionID: "test-execution", AttemptStartedAt: attempt,
				BaseCommand: filepath.Join(t.TempDir(), "private-token"), RedactValues: []string{"private-token"},
			}
			if stage == diagnosis.StageRuntime {
				cfg.BaseCommand = "sh"
				cfg.Args = []string{"-c", "echo final-stderr private-token node-test-secret >&2; exit 23"}
				if runtime.GOOS == "windows" {
					cfg.BaseCommand = "cmd"
					cfg.Args = []string{"/c", "echo final-stderr private-token node-test-secret 1>&2 & exit /b 23"}
				}
			}
			errStart := client.StartProcess(t.Context(), cfg, xylona.Status_ONLINE)
			if stage == diagnosis.StageLaunch {
				failure, isFailure := errors.AsType[*node.StartFailureError](errStart)
				if !isFailure || failure.Report.Category != diagnosis.CategoryMissingExecutable || failure.Report.ExitCode != nil ||
					!failure.Report.AttemptStartedAt.Equal(attempt) || strings.Contains(errStart.Error(), "private-token") {
					t.Fatalf("structured start failure = %v", errStart)
				}
			} else if errStart != nil {
				t.Fatal(errStart)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			stream, errStream := client.StreamEvents(ctx)
			if errStream != nil {
				t.Fatal(errStream)
			}
			for event := range stream {
				if event.ProcessID != cfg.ID || event.Status != xylona.Status_OFFLINE.String() {
					continue
				}
				report := event.Failure
				if report == nil || report.Stage != stage || report.ExecutionID != cfg.ExecutionID || !report.AttemptStartedAt.Equal(attempt) {
					t.Fatalf("transport report = %+v", report)
				}
				if strings.Contains(report.Error+report.Evidence, "private-token") || strings.Contains(report.Error+report.Evidence, "node-test-secret") {
					t.Fatal("credentials leaked in transport evidence")
				}
				if stage == diagnosis.StageRuntime && (!strings.Contains(report.Evidence, "final-stderr") || report.ExitCode == nil || *report.ExitCode != 23) {
					t.Fatalf("runtime evidence = %+v", report)
				}
				return
			}
			t.Fatal("terminal report was not replayed or delivered")
		})
	}
}
