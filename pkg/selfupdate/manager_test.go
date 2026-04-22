package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/node"
)

func TestManagerStageSelfUpdate(t *testing.T) {
	t.Parallel()

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	exePath := t.TempDir() + "/xylona-node"

	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: exePath,
		ExitFunc:       func(int) {},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}

	result, errStage := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		TargetVersion:  "1.2.3",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSize:   int64(len(content)),
		ExpectedSHA256: sum,
		Reader:         bytes.NewReader(content),
	})
	if errStage != nil {
		t.Fatalf("Stage() error = %v", errStage)
	}
	if result.BytesWritten != int64(len(content)) {
		t.Fatalf("Stage().BytesWritten = %d, want %d", result.BytesWritten, len(content))
	}
	if result.SHA256 != sum {
		t.Fatalf("Stage().SHA256 = %q, want %q", result.SHA256, sum)
	}
}

func TestManagerStageRejectsBadInput(t *testing.T) {
	t.Parallel()

	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: t.TempDir() + "/xylona-node",
		ExitFunc:       func(int) {},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}

	_, errComponent := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "controller",
		ExpectedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Reader:         bytes.NewReader([]byte("content")),
	})
	if errComponent == nil {
		t.Fatal("Stage(wrong component) error = nil, want error")
	}

	_, errHash := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Reader:         bytes.NewReader([]byte("content")),
	})
	if errHash == nil {
		t.Fatal("Stage(bad checksum) error = nil, want error")
	}
}

func TestManagerApplyStartsHelperAfterRequestContextCanceled(t *testing.T) {
	t.Parallel()

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	exitCalls := make(chan int, 1)

	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: t.TempDir() + "/xylona-node",
		ExitFunc: func(code int) {
			exitCalls <- code
		},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}

	startCalls := make(chan [2]string, 1)
	manager.startHelper = func(helperPath string, pendingPath string) error {
		startCalls <- [2]string{helperPath, pendingPath}
		return nil
	}

	result, errStage := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		TargetVersion:  "1.2.3",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSize:   int64(len(content)),
		ExpectedSHA256: sum,
		Reader:         bytes.NewReader(content),
	})
	if errStage != nil {
		t.Fatalf("Stage() error = %v", errStage)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	applyResult, errApply := manager.Apply(ctx, node.ApplySelfUpdateRequest{
		StageID:        result.StageID,
		TargetVersion:  "1.2.3",
		ExpectedSHA256: result.SHA256,
	})
	if errApply != nil {
		t.Fatalf("Apply() error = %v", errApply)
	}
	if !applyResult.Accepted {
		t.Fatal("Apply().Accepted = false, want true")
	}

	select {
	case call := <-startCalls:
		if call[0] == "" || call[1] == "" {
			t.Fatalf("start helper call = %q, want helper and pending paths", call)
		}
	default:
		t.Fatal("Apply() did not start helper")
	}
}

func TestManagerApplyPreparesInstallCandidateNextToExecutable(t *testing.T) {
	t.Parallel()

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	executableDir := t.TempDir()
	executablePath := filepath.Join(executableDir, "xylona-node")

	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: executablePath,
		ExitFunc:       func(int) {},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}

	startCalls := make(chan [2]string, 1)
	manager.startHelper = func(helperPath string, pendingPath string) error {
		startCalls <- [2]string{helperPath, pendingPath}
		return nil
	}

	result, errStage := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		TargetVersion:  "1.2.3",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSize:   int64(len(content)),
		ExpectedSHA256: sum,
		Reader:         bytes.NewReader(content),
	})
	if errStage != nil {
		t.Fatalf("Stage() error = %v", errStage)
	}

	_, errApply := manager.Apply(t.Context(), node.ApplySelfUpdateRequest{
		StageID:        result.StageID,
		TargetVersion:  "1.2.3",
		ExpectedSHA256: result.SHA256,
	})
	if errApply != nil {
		t.Fatalf("Apply() error = %v", errApply)
	}

	var call [2]string
	select {
	case call = <-startCalls:
	default:
		t.Fatal("Apply() did not start helper")
	}

	data, errRead := os.ReadFile(call[1])
	if errRead != nil {
		t.Fatalf("read pending update: %v", errRead)
	}
	var pending pendingUpdate
	errDecode := json.Unmarshal(data, &pending)
	if errDecode != nil {
		t.Fatalf("decode pending update: %v", errDecode)
	}
	if filepath.Dir(pending.StagedPath) != executableDir {
		t.Fatalf("pending staged dir = %q, want %q", filepath.Dir(pending.StagedPath), executableDir)
	}
	candidateContent, errReadCandidate := os.ReadFile(pending.StagedPath)
	if errReadCandidate != nil {
		t.Fatalf("read install candidate: %v", errReadCandidate)
	}
	if !bytes.Equal(candidateContent, content) {
		t.Fatalf("install candidate content = %q, want %q", candidateContent, content)
	}
}
