package rpc

import (
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/sql/models"
)

type validationParityFixture struct {
	Port              []validationParityPortCase   `json:"port"`
	PlayerCount       []validationParityCountCase  `json:"playerCount"`
	PlayerCountAtMost []validationParityAtMostCase `json:"playerCountAtMost"`
	MaxMemory         []validationParityMemoryCase `json:"maxMemory"`
}

type validationParityPortCase struct {
	Name     string `json:"name"`
	Value    int64  `json:"value"`
	Expected string `json:"expected"`
}

type validationParityCountCase struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Minimum  int64  `json:"minimum"`
	Value    int64  `json:"value"`
	Expected string `json:"expected"`
}

type validationParityAtMostCase struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	MaximumLabel string `json:"maximumLabel"`
	Value        int64  `json:"value"`
	Maximum      *int64 `json:"maximum"`
	Expected     string `json:"expected"`
}

type validationParityMemoryCase struct {
	Name     string `json:"name"`
	Value    int64  `json:"value"`
	Expected string `json:"expected"`
}

func TestValidateGameServerSubmissionParity(t *testing.T) {
	fixture := makeValidationParityFixture()
	game := &models.Game{ID: `minecraft`}

	for _, tt := range fixture.Port {
		t.Run(fmt.Sprintf("port/%s", tt.Name), func(t *testing.T) {
			errValidate := validateGameServerPort(tt.Value, `Port`)
			assertValidationParityResult(t, errValidate, tt.Expected)
		})
	}

	for _, tt := range fixture.PlayerCount {
		t.Run(fmt.Sprintf("player-count/%s", tt.Name), func(t *testing.T) {
			errValidate := validateGameServerPlayerCount(tt.Value, tt.Label, tt.Minimum)
			assertValidationParityResult(t, errValidate, tt.Expected)
		})
	}

	for _, tt := range fixture.PlayerCountAtMost {
		t.Run(fmt.Sprintf("player-count-at-most/%s", tt.Name), func(t *testing.T) {
			errValidate := validateGameServerPlayerCountAtMost(tt.Value, tt.Label, tt.Maximum, tt.MaximumLabel)
			assertValidationParityResult(t, errValidate, tt.Expected)
		})
	}

	for _, tt := range fixture.MaxMemory {
		t.Run(fmt.Sprintf("max-memory/%s", tt.Name), func(t *testing.T) {
			errValidate := validateGameServerMaxMemory(tt.Value)
			assertValidationParityResult(t, errValidate, tt.Expected)
		})
	}

	t.Run("submission validator matches shared cases", func(t *testing.T) {
		submission := &models.GameServer{
			Port:        25565,
			QueryPort:   25566,
			SetPlayers:  0,
			MaxPlayers:  1,
			MaxMemoryMB: 128,
		}

		if errValidate := validateGameServerSubmission(game, submission, true); errValidate != nil {
			t.Fatalf("validateGameServerSubmission() error = %v, want nil", errValidate)
		}
	})
}

func makeValidationParityFixture() validationParityFixture {
	return validationParityFixture{
		Port: []validationParityPortCase{
			{
				Name:     `rejects ports below the valid range`,
				Value:    0,
				Expected: `Port must be between 1 and 65535`,
			},
			{
				Name:     `accepts a typical minecraft server port`,
				Value:    25565,
				Expected: `ok`,
			},
		},
		PlayerCount: []validationParityCountCase{
			{
				Name:     `rejects values below the configured minimum`,
				Label:    `Max Players`,
				Minimum:  1,
				Value:    0,
				Expected: `Max Players must be 1 or greater`,
			},
			{
				Name:     `accepts an in-range player count`,
				Label:    `Max Players`,
				Minimum:  1,
				Value:    32,
				Expected: `ok`,
			},
		},
		PlayerCountAtMost: []validationParityAtMostCase{
			{
				Name:         `defers while the related maximum is still unset`,
				Label:        `Set Players`,
				MaximumLabel: `Max Players`,
				Value:        4,
				Maximum:      nil,
				Expected:     `ok`,
			},
			{
				Name:         `rejects values above the related maximum`,
				Label:        `Set Players`,
				MaximumLabel: `Max Players`,
				Value:        12,
				Maximum:      int64Ptr(10),
				Expected:     `Set Players cannot exceed Max Players`,
			},
			{
				Name:         `accepts values within the related maximum`,
				Label:        `Set Players`,
				MaximumLabel: `Max Players`,
				Value:        10,
				Maximum:      int64Ptr(10),
				Expected:     `ok`,
			},
		},
		MaxMemory: []validationParityMemoryCase{
			{
				Name:     `rejects limits below the minimum`,
				Value:    64,
				Expected: `Max Memory MB must be at least 128`,
			},
			{
				Name:     `accepts a valid memory limit`,
				Value:    2048,
				Expected: `ok`,
			},
		},
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

func assertValidationParityResult(t *testing.T, err error, expected string) {
	t.Helper()

	if expected == `ok` {
		if err != nil {
			t.Fatalf("validation result = %v, want nil", err)
		}
		return
	}

	if err == nil {
		t.Fatalf("validation result = nil, want %q", expected)
	}

	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("validation code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}

	got := strings.TrimPrefix(err.Error(), connect.CodeInvalidArgument.String()+`: `)
	got = strings.TrimPrefix(got, connect.CodeInvalidArgument.String()+`:`)
	got = strings.TrimSpace(got)

	if got != expected {
		t.Fatalf("validation result = %q, want %q", got, expected)
	}
}
