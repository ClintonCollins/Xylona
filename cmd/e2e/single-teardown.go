package main

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func runSingleTeardown(ctx context.Context, backendURL, adminUsername, adminPassword, e2eDir string) error {
	log.Info().Msgf("[E2E Teardown] Connecting to backend at %s", backendURL)

	client, errLogin := newAuthenticatedClient(ctx, backendURL, adminUsername, adminPassword)
	if errLogin != nil {
		log.Warn().Err(errLogin).Msg("[E2E Teardown] Warning: could not log in as admin, skipping cleanup")
		return nil
	}

	state, errState := loadTestState(e2eDir)
	if errState != nil {
		log.Warn().Err(errState).Msg("[E2E Teardown] Warning: could not load test state")
		state = &testState{}
	}

	// Clean up test files from the game server.
	if state.GameServerID != "" {
		_, errDelete := client.rpc.GameServerFilesDelete(ctx, connect.NewRequest(&xylona.GameServerFilesDeleteRequest{
			GameServerId: state.GameServerID,
			FullFilePaths: []string{
				"e2e-test-config.cfg",
				"e2e-test-readme.txt",
				"e2e-test-subdir",
			},
		}))
		if errDelete != nil {
			log.Warn().Err(errDelete).Msg("[E2E Teardown] Warning: could not delete test files")
		} else {
			log.Info().Msg("[E2E Teardown] Deleted test files")
		}

		// Stop and remove the game server if we created it.
		if state.GameID != "" {
			_, errStop := client.rpc.StopGameServer(ctx, connect.NewRequest(&xylona.StopGameServerRequest{
				ServerId: state.GameServerID,
			}))
			if errStop != nil {
				log.Warn().Err(errStop).Msg("[E2E Teardown] Warning: could not stop game server")
			} else {
				log.Info().Msg("[E2E Teardown] Stopped test game server")
			}

			time.Sleep(1 * time.Second)

			_, errRemove := client.rpc.RemoveGameServer(ctx, connect.NewRequest(&xylona.RemoveGameServerRequest{
				ServerId: state.GameServerID,
			}))
			if errRemove != nil {
				log.Warn().Err(errRemove).Msg("[E2E Teardown] Warning: could not remove game server")
			} else {
				log.Info().Msg("[E2E Teardown] Removed test game server")
			}

			_, errRemoveGame := client.rpc.RemoveGame(ctx, connect.NewRequest(&xylona.RemoveGameRequest{
				GameId: state.GameID,
			}))
			if errRemoveGame != nil {
				log.Warn().Err(errRemoveGame).Msg("[E2E Teardown] Warning: could not remove game")
			} else {
				log.Info().Msg("[E2E Teardown] Removed test game definition")
			}
		}
	}

	// Delete test users.
	users, errUsers := loadTestUsers(e2eDir)
	if errUsers != nil {
		log.Warn().Err(errUsers).Msg("[E2E Teardown] Warning: could not load test users")
	}
	if len(users) == 0 {
		log.Info().Msg("[E2E Teardown] No test users to clean up")
	} else {
		for _, user := range users {
			_, errDel := client.rpc.DeleteUser(ctx, connect.NewRequest(&xylona.DeleteUserRequest{
				Id: user.ID,
			}))
			if errDel != nil {
				log.Warn().Err(errDel).Msgf("[E2E Teardown] Warning: could not delete user %s", user.Username)
			} else {
				log.Info().Msgf("[E2E Teardown] Deleted user: %s (id: %s)", user.Username, user.ID)
			}
		}
	}

	log.Info().Msg("[E2E Teardown] Teardown complete")
	return nil
}
