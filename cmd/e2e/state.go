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
	GameServerID      string `json:"gameServerId,omitempty"`
	GameID            string `json:"gameId,omitempty"`
	GameName          string `json:"gameName,omitempty"`
	NoTrackerServerID string `json:"noTrackerServerId,omitempty"`
}

// federationTestState matches federation-helpers.ts FederationTestState.
type federationTestState struct {
	NodeAURL        string     `json:"nodeAUrl"`
	NodeBURL        string     `json:"nodeBUrl"`
	NodeAID         string     `json:"nodeAId,omitempty"`
	NodeBID         string     `json:"nodeBId,omitempty"`
	PairedNodeIDOnA string     `json:"pairedNodeIdOnA,omitempty"`
	PairedNodeIDOnB string     `json:"pairedNodeIdOnB,omitempty"`
	GameServerID    string     `json:"gameServerId,omitempty"`
	GameID          string     `json:"gameId,omitempty"`
	TestUsers       []testUser `json:"testUsers,omitempty"`
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

func saveFederationState(dir string, state *federationTestState) error {
	fedDir := filepath.Join(dir, ".federation")
	errMkdir := os.MkdirAll(fedDir, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("create federation dir: %w", errMkdir)
	}
	data, errMarshal := json.MarshalIndent(state, "", "  ")
	if errMarshal != nil {
		return fmt.Errorf("marshal federation state: %w", errMarshal)
	}
	errWrite := os.WriteFile(filepath.Join(fedDir, "state.json"), data, 0o600)
	if errWrite != nil {
		return fmt.Errorf("write federation state: %w", errWrite)
	}
	return nil
}

func loadFederationState(dir string) (*federationTestState, error) {
	data, errRead := os.ReadFile(filepath.Join(dir, ".federation", "state.json"))
	if errRead != nil {
		if os.IsNotExist(errRead) {
			return &federationTestState{}, nil
		}
		return nil, fmt.Errorf("read federation state: %w", errRead)
	}
	var state federationTestState
	errUnmarshal := json.Unmarshal(data, &state)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("unmarshal federation state: %w", errUnmarshal)
	}
	return &state, nil
}
