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
