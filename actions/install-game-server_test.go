package actions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

func TestInstallGameServerRemovesArtifactsWhenCommandStartFails(t *testing.T) {
	t.Setenv(`USERPROFILE`, t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	conn := newTestInstance(t).db
	supervisorInst, errSupervisor := supervisor.New(ctx)
	if errSupervisor != nil {
		t.Fatalf(`supervisor.New() error = %v`, errSupervisor)
	}

	inst := NewInstance(ctx, conn, supervisorInst, nil, nil, versiontracker.NewVersionStateMap(), versiontracker.ResolverConfig{})

	now := time.Now().UTC()
	owner, errCreateUser := inst.db.CreateUser(&models.UserSetter{
		ID:           omit.From(`install-owner`),
		UserName:     omit.From(`install-owner`),
		Email:        omit.From(`install-owner@example.com`),
		FirstName:    omit.From(`Install`),
		LastName:     omit.From(`Owner`),
		PasswordHash: omit.From(`hash`),
		SuperUser:    omit.From(false),
		LastLoginAt:  omit.From(now),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	})
	if errCreateUser != nil {
		t.Fatalf(`CreateUser() error = %v`, errCreateUser)
	}

	_, errInsertNode := inst.db.InsertNode(&models.NodeSetter{
		ID:      omit.From(`install-node`),
		Name:    omit.From(`Install Node`),
		IsLocal: omit.From(true),
		Host:    omit.From(`localhost`),
		Port:    omit.From(int64(8080)),
		BaseURL: omit.From(`http://localhost:8080`),
		Enabled: omit.From(true),
	})
	if errInsertNode != nil {
		t.Fatalf(`InsertNode() error = %v`, errInsertNode)
	}

	game, errInsertGame := inst.db.InsertGame(inst.db.DB, &models.GameSetter{
		ID:                        omit.From(`install-game`),
		Name:                      omit.From(`Install Test Game`),
		DefaultPort:               omit.From(int64(25565)),
		DefaultQueryPort:          omit.From(int64(25565)),
		DefaultMaxPlayers:         omit.From(int64(20)),
		LinuxInstallCommand:       omit.From(`definitely-not-a-real-command`),
		WindowsInstallCommand:     omit.From(`definitely-not-a-real-command`),
		LinuxInstallCommandType:   omit.From(CommandTypeDirect),
		WindowsInstallCommandType: omit.From(CommandTypeDirect),
	})
	if errInsertGame != nil {
		t.Fatalf(`InsertGame() error = %v`, errInsertGame)
	}

	input := &models.GameServer{
		ID:              `install-server-input`,
		UserID:          owner.ID,
		Name:            `Needs Cleanup`,
		GameID:          game.ID,
		SetPlayers:      20,
		MaxPlayers:      20,
		Map:             `world`,
		IP:              `127.0.0.1`,
		Port:            25565,
		QueryPort:       25565,
		NodeID:          `install-node`,
		MaxMemoryMB:     1024,
		MaxBackups:      2,
		BackupDirectory: filepath.Join(t.TempDir(), `backups`),
	}

	installed, errInstall := inst.InstallGameServer(game, input, owner)
	if errInstall == nil {
		t.Fatalf(`InstallGameServer() error = nil, want start failure`)
	}
	if installed != nil {
		t.Fatalf(`InstallGameServer() returned server on failure: %+v`, installed)
	}

	installPath, errDefaultInstallPath := DefaultInstallPath()
	if errDefaultInstallPath != nil {
		t.Fatalf(`DefaultInstallPath() error = %v`, errDefaultInstallPath)
	}
	serverDir := joinManagedPath(installPath, owner.UserName, `needs-cleanup`)
	_, errStat := os.Stat(serverDir)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf(`expected install directory cleanup for %q, stat error = %v`, serverDir, errStat)
	}

	gameServers, errGetServers := inst.db.GetGameServersByUser(owner.ID)
	if errGetServers != nil {
		t.Fatalf(`GetGameServersByUser() error = %v`, errGetServers)
	}
	if len(gameServers) != 0 {
		t.Fatalf(`GetGameServersByUser() returned %d servers, want 0`, len(gameServers))
	}
}
