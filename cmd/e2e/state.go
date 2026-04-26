package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// testUser matches helpers.ts TestUser interface.
type testUser struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	SuperUser bool   `json:"superUser"`
}

// testState matches helpers.ts TestState interface.
type testState struct {
	Mode              string            `json:"mode,omitempty"`
	BackendURL        string            `json:"backendUrl,omitempty"`
	DataDir           string            `json:"dataDir,omitempty"`
	ControllerDir     string            `json:"controllerDir,omitempty"`
	ControllerHomeDir string            `json:"controllerHomeDir,omitempty"`
	ControllerPID     int               `json:"controllerPid,omitempty"`
	NodeDir           string            `json:"nodeDir,omitempty"`
	NodeHomeDir       string            `json:"nodeHomeDir,omitempty"`
	RemoteNodePID     int               `json:"remoteNodePid,omitempty"`
	GameServerID      string            `json:"gameServerId,omitempty"`
	GameServerDir     string            `json:"gameServerDir,omitempty"`
	GameID            string            `json:"gameId,omitempty"`
	GameName          string            `json:"gameName,omitempty"`
	TargetNodeID      string            `json:"targetNodeId,omitempty"`
	RemoteNodeID      string            `json:"remoteNodeId,omitempty"`
	DummyServerPath   string            `json:"dummyServerPath,omitempty"`
	RuntimeEnv        map[string]string `json:"runtimeEnv,omitempty"`
}

func saveTestUsers(dir string, users []testUser) error {
	authDir := filepath.Join(dir, ".auth")
	errMkdir := os.MkdirAll(authDir, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("create auth dir: %w", errMkdir)
	}
	serializedUsers := make([]map[string]any, 0, len(users))
	for _, user := range users {
		serializedUsers = append(serializedUsers, map[string]any{
			"id":        user.ID,
			"username":  user.Username,
			"password":  user.Password,
			"superUser": user.SuperUser,
		})
	}
	data, errMarshal := json.MarshalIndent(serializedUsers, "", "  ")
	if errMarshal != nil {
		return fmt.Errorf("marshal test users: %w", errMarshal)
	}
	errWrite := os.WriteFile(filepath.Join(authDir, "test-users.json"), data, 0o600)
	if errWrite != nil {
		return fmt.Errorf("write test users: %w", errWrite)
	}
	return nil
}

func saveTestState(dir string, state *testState) error {
	authDir := filepath.Join(dir, ".auth")
	errMkdir := os.MkdirAll(authDir, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("create auth dir: %w", errMkdir)
	}
	data, errMarshal := json.MarshalIndent(state, "", "  ")
	if errMarshal != nil {
		return fmt.Errorf("marshal test state: %w", errMarshal)
	}
	errWrite := os.WriteFile(filepath.Join(authDir, "test-state.json"), data, 0o600)
	if errWrite != nil {
		return fmt.Errorf("write test state: %w", errWrite)
	}
	return nil
}

func loadTestState(dir string) (*testState, error) {
	data, errRead := os.ReadFile(filepath.Join(dir, ".auth", "test-state.json"))
	if errRead != nil {
		return nil, fmt.Errorf("read test state: %w", errRead)
	}

	state := &testState{}
	errUnmarshal := json.Unmarshal(data, state)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("unmarshal test state: %w", errUnmarshal)
	}
	return state, nil
}
