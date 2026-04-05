package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func runFederationTeardown(ctx context.Context, e2eDir string, keepData bool, nodeAPort, nodeBPort int) {
	log.Info().Msg("[Federation Teardown] Starting federation cleanup...")

	federationDir := filepath.Join(e2eDir, ".federation")
	nodeAURL := fmt.Sprintf("http://localhost:%d", nodeAPort)
	nodeBURL := fmt.Sprintf("http://localhost:%d", nodeBPort)

	state, errState := loadFederationState(e2eDir)
	if errState != nil {
		log.Warn().Err(errState).Msg("[Federation Teardown] Warning: could not load federation state")
		state = &federationTestState{
			NodeAURL: nodeAURL,
			NodeBURL: nodeBURL,
		}
	}

	// Try to clean up via API on Node B first.
	clientB, errLoginB := newAuthenticatedClient(ctx, nodeBURL, "admin", "admin")
	if errLoginB != nil {
		log.Warn().Err(errLoginB).Msg("[Federation Teardown] Could not connect to Node B for cleanup")
	} else {
		if state.GameServerID != "" {
			_, errStop := clientB.rpc.StopGameServer(ctx, connect.NewRequest(&xylona.StopGameServerRequest{
				ServerId: state.GameServerID,
			}))
			if errStop != nil {
				log.Warn().Err(errStop).Msg("[Federation Teardown] Could not stop game server")
			} else {
				log.Info().Msg("[Federation Teardown] Stopped game server on Node B")
				time.Sleep(1 * time.Second)
			}

			_, errRemove := clientB.rpc.RemoveGameServer(ctx, connect.NewRequest(&xylona.RemoveGameServerRequest{
				ServerId: state.GameServerID,
			}))
			if errRemove != nil {
				log.Warn().Err(errRemove).Msg("[Federation Teardown] Could not remove game server")
			} else {
				log.Info().Msg("[Federation Teardown] Removed game server from Node B")
			}
		}

		if state.GameID != "" {
			_, errRemoveGame := clientB.rpc.RemoveGame(ctx, connect.NewRequest(&xylona.RemoveGameRequest{
				GameId: state.GameID,
			}))
			if errRemoveGame != nil {
				log.Warn().Err(errRemoveGame).Msg("[Federation Teardown] Could not remove game")
			} else {
				log.Info().Msg("[Federation Teardown] Removed game from Node B")
			}
		}
	}

	// Delete test users from Node A and unpair.
	clientA, errLoginA := newAuthenticatedClient(ctx, nodeAURL, "admin", "admin")
	if errLoginA != nil {
		log.Warn().Err(errLoginA).Msg("[Federation Teardown] Could not connect to Node A for cleanup")
	} else {
		for _, user := range state.TestUsers {
			_, errDel := clientA.rpc.DeleteUser(ctx, connect.NewRequest(&xylona.DeleteUserRequest{
				Id: user.ID,
			}))
			if errDel != nil {
				log.Warn().Err(errDel).Msgf("[Federation Teardown] Could not delete user %s", user.Username)
			} else {
				log.Info().Msgf("[Federation Teardown] Deleted user: %s", user.Username)
			}
		}

		// Unpair nodes.
		if state.PairedNodeIDOnA != "" {
			_, errUnpair := clientA.rpc.RemoveNode(ctx, connect.NewRequest(&xylona.RemoveNodeRequest{
				NodeId: state.PairedNodeIDOnA,
			}))
			if errUnpair != nil {
				log.Warn().Err(errUnpair).Msg("[Federation Teardown] Could not unpair nodes")
			} else {
				log.Info().Msg("[Federation Teardown] Removed paired node from Node A")
			}
		}
	}

	// Kill both Xylona processes.
	killByPIDFile(filepath.Join(federationDir, "node-a.pid"), "Node A")
	killByPIDFile(filepath.Join(federationDir, "node-b.pid"), "Node B")

	// Clean up data unless keep-data is set.
	if !keepData {
		log.Info().Msg("[Federation Teardown] Cleaning up federation data...")
		stateFile := filepath.Join(federationDir, "state.json")
		_ = os.Remove(stateFile)

		for _, dir := range []string{"node-a", "node-b", "game-server-data"} {
			fullPath := filepath.Join(federationDir, dir)
			_ = os.RemoveAll(fullPath)
		}
	} else {
		log.Info().Msg("[Federation Teardown] keep-data set, preserving data for debugging")
	}

	// Clean the frontend dist built during setup.
	cleanFrontendDist(e2eDir)

	// Release the suite lock.
	releaseLock(e2eDir, "federation")

	log.Info().Msg("[Federation Teardown] Teardown complete")
}
