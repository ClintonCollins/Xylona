package rpc

import (
	"errors"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestRemoveGameServerDeleteBackups(t *testing.T) {
	const (
		serverID   = "server-remote-1"
		serverDir  = "/srv/remote-server"
		backupRoot = "/srv/backups"
		archiveDir = backupRoot + "/" + serverID
	)
	archives := []string{"newer.zip", "older.zip"}

	cases := []struct {
		name              string
		userID            string
		deleteBackups     bool
		backupNodeID      string
		deleteFilesResult []string
		deleteFilesErr    error
		wantCode          connect.Code
		wantDeleteDirs    []string
		wantStopped       bool
		wantRemoved       bool
		wantBackupRows    int
	}{
		{
			name:           "keeps backup archives when not asked",
			userID:         "user-admin",
			wantDeleteDirs: []string{serverDir},
			wantStopped:    true,
			wantRemoved:    true,
		},
		{
			name:              "deletes each backup archive before the server files",
			userID:            "user-owner",
			deleteBackups:     true,
			deleteFilesResult: archives,
			wantDeleteDirs:    []string{archiveDir, archiveDir, serverDir},
			wantStopped:       true,
			wantRemoved:       true,
		},
		{
			name:           "delete permission alone can remove without deleting backups",
			userID:         "user-other",
			wantDeleteDirs: []string{serverDir},
			wantStopped:    true,
			wantRemoved:    true,
		},
		{
			name:           "deleting backups without backup permission is denied",
			userID:         "user-other",
			deleteBackups:  true,
			wantCode:       connect.CodePermissionDenied,
			wantBackupRows: 2,
		},
		{
			name:           "a failed backup delete stops the removal",
			userID:         "user-admin",
			deleteBackups:  true,
			deleteFilesErr: errors.New("archive is locked"),
			wantCode:       connect.CodeInternal,
			wantDeleteDirs: []string{archiveDir},
			wantStopped:    true,
			wantBackupRows: 2,
		},
		{
			name:           "a backup on another node refuses before stopping the server",
			userID:         "user-admin",
			deleteBackups:  true,
			backupNodeID:   "node-local",
			wantCode:       connect.CodeFailedPrecondition,
			wantBackupRows: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			insertRemoteNodeForParityTests(t, fixture, "node-remote")
			insertRemoteServerForParityTests(t, fixture, serverID)

			// user-other may delete the server but holds no backup permission.
			errRole := fixture.conn.CreateRoleWithPermissions("remover", "Remover", "", []string{"game_server.delete"})
			if errRole != nil {
				t.Fatalf("CreateRoleWithPermissions() error = %v", errRole)
			}
			errAssign := fixture.conn.CreateUserRoleAssignment("assignment-remover", "user-other", "remover", serverID, "user-admin")
			if errAssign != nil {
				t.Fatalf("CreateUserRoleAssignment() error = %v", errAssign)
			}

			backupNodeID := tc.backupNodeID
			if backupNodeID == "" {
				backupNodeID = "node-remote"
			}
			createdAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
			for index, archive := range archives {
				_, errCreate := fixture.conn.CreateGameServerBackup(db.CreateGameServerBackupParams{
					GameServerID:  serverID,
					NodeID:        backupNodeID,
					CreatedBy:     "user-owner",
					TriggerSource: "manual",
					ArchivePath:   archiveDir + "/" + archive,
					ArchiveRoot:   backupRoot,
					ArchiveFormat: "zip",
					Status:        "completed",
					SizeBytes:     1024,
					CreatedAt:     createdAt.Add(-time.Duration(index) * time.Hour),
				})
				if errCreate != nil {
					t.Fatalf("CreateGameServerBackup(%s) error = %v", archive, errCreate)
				}
			}

			remoteClient := &nodeclient.FakeNodeClient{
				NodeID:            "node-remote",
				SnapshotResult:    &node.NodeSnapshot{OS: "linux"},
				DeleteFilesResult: tc.deleteFilesResult,
				DeleteFilesErr:    tc.deleteFilesErr,
			}
			registry := testParityRegistry(
				&nodeclient.FakeNodeClient{NodeID: "node-local", SnapshotResult: &node.NodeSnapshot{OS: "linux"}},
				remoteClient,
			)
			configureLifecycleActionsForParityTests(t, fixture, registry)

			request := connect.NewRequest(&xylona.RemoveGameServerRequest{
				ServerId:      serverID,
				DeleteBackups: tc.deleteBackups,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, tc.userID)

			_, errRemove := fixture.service.RemoveGameServer(t.Context(), request)
			if tc.wantCode == 0 && errRemove != nil {
				t.Fatalf("RemoveGameServer() error = %v", errRemove)
			}
			if tc.wantCode != 0 && connect.CodeOf(errRemove) != tc.wantCode {
				t.Fatalf("RemoveGameServer() code = %v, want %v (error %v)", connect.CodeOf(errRemove), tc.wantCode, errRemove)
			}

			var deleteDirs []string
			for index, call := range remoteClient.DeleteFilesCalls {
				deleteDirs = append(deleteDirs, call.Directory)
				if call.Directory == archiveDir && !slices.Equal(call.Files, []string{archives[index]}) {
					t.Fatalf("DeleteFiles call %d files = %v, want only %q", index, call.Files, archives[index])
				}
			}
			if !slices.Equal(deleteDirs, tc.wantDeleteDirs) {
				t.Fatalf("DeleteFiles directories = %v, want %v", deleteDirs, tc.wantDeleteDirs)
			}
			if stopped := len(remoteClient.StopProcessCalls) > 0; stopped != tc.wantStopped {
				t.Fatalf("server stopped = %v, want %v", stopped, tc.wantStopped)
			}
			_, errGet := fixture.conn.GetGameServerByID(serverID)
			if removed := errGet != nil; removed != tc.wantRemoved {
				t.Fatalf("server removed = %v (lookup error %v), want %v", removed, errGet, tc.wantRemoved)
			}
			backups, errList := fixture.conn.ListGameServerBackupsByGameServerID(serverID)
			if errList != nil {
				t.Fatalf("ListGameServerBackupsByGameServerID() error = %v", errList)
			}
			if len(backups) != tc.wantBackupRows {
				t.Fatalf("backup rows = %d, want %d", len(backups), tc.wantBackupRows)
			}
		})
	}
}
