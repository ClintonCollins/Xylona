package node

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestStartProcessRejectsInvalidLaunchEnvironmentAtNodeBoundary(t *testing.T) {
	supervisorInst, errNew := supervisor.New(context.Background())
	if errNew != nil {
		t.Fatalf("supervisor.New() error = %v", errNew)
	}
	nodeInst := New(context.Background(), supervisorInst, nil)

	secretValue := "must-not-appear"
	_, errStart := nodeInst.StartProcess(ProcessConfig{
		ID:          "invalid-env",
		BaseCommand: "unused",
		LaunchEnv: map[string]string{
			"JDK_JAVA_OPTIONS": secretValue,
		},
	}, xylona.Status_ONLINE)
	if errStart == nil {
		t.Fatal("StartProcess() error = nil, want launch environment validation error")
	}
	validationError := &launchenv.ValidationError{}
	if !errors.As(errStart, &validationError) {
		t.Fatalf("StartProcess() error = %T %v, want *launchenv.ValidationError", errStart, errStart)
	}
	if strings.Contains(errStart.Error(), secretValue) {
		t.Fatalf("StartProcess() error leaked launch environment value: %v", errStart)
	}
}
