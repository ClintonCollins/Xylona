# Readiness Checks Plan

## Summary

- Implement readiness in three strict phases.
- Phase 1 is only readiness storage + Minecraft EULA, using the existing typed start contract.
- Phase 2 fixes encrypted Steam GSLT storage/readiness using the existing `game_server_secret` helpers and removes the old plaintext `game_server.steam_game_server_login_token` column.
- Phase 3 adds Hytale account linking using the official device flow.
- Launch environment plumbing is already available through `LaunchEnv`; do not duplicate node protocol/supervisor env work here.
- Secret command-line argument substitution is explicitly out of scope for this plan.

## Progress

- [x] Phase 1: Readiness storage + Minecraft EULA
- [x] Phase 2: Encrypted Steam GSLT
- [x] Phase 3: Hytale account link
- [x] Frontend readiness UI
- [x] Final validation

## Phase 1: Readiness Storage + Minecraft EULA

- [x] Use the existing `actions.StartGameServer` typed result/error contract; do not add another taxonomy.
- [x] Map readiness blocks to the existing configuration/`FailedPrecondition` path in the RPC layer.
- [ ] Update install post-hook, auto-restart, update restart, and scheduled callers so readiness blocks are terminal/no-retry.
- [ ] Keep one owner for user-facing start failure messages to avoid duplicate logs or console lines.

- [x] Add `game_server_readiness`:
  - primary key `(game_server_id, kind)`
  - FK cascade to `game_server`
  - `public_data` JSON text
  - nullable `updated_by_user_id`
  - `created_at` and `updated_at`
  - no SQLite `kind` check; enforce allowed kinds in typed Go helpers to avoid later table rebuild migrations
- [x] Use narrow raw SQL helpers; do not generate bob models for this table.

- [x] Add `internal/controller/readiness`.
- [x] Add `CheckStartReadiness(ctx, gameServer)` before config pre-start and mod updates.
- [x] Add `PrepareLaunchSecrets(ctx, gameServer)` as a Phase 1 no-op that runs after config/mod work and immediately before `StartProcess`.

- [x] Keep Minecraft EULA acceptance out of `CreateGameServerRequest`; create can finish and start is blocked by readiness until accepted.
- [x] Reserve the removed `accept_minecraft_eula` create-request field.
- [x] Persist the Minecraft EULA readiness row only through the game-server view readiness action.
- [x] For existing Minecraft servers, lazily read `eula.txt` through the target `NodeClient` when the readiness row is missing.
- [x] If `eula.txt` contains `eula=true`, persist readiness and allow start.
- [x] If the node/file read fails, report the node/file error instead of silently treating it as missing acceptance.
- [x] If the file exists but is not accepted, block with `Minecraft EULA required`.
- [x] When accepted, write `eula.txt` with `eula=true` through the target `NodeClient` for embedded and remote nodes.

- [x] Add authenticated readiness RPCs:
  - `GetGameServerReadiness(server_id)` requires existing server view permission.
  - `AcceptMinecraftEula(server_id)` requires existing server settings permission.
- [x] Keep `GetGameServerReadiness` responses small: `kind`, `required`, `complete`, `blocking`, `message`, and minimal public metadata.

## Phase 2: Encrypted Steam GSLT

- [x] Reuse existing `game_server_secret` typed helpers; add a narrow secret kind for Steam GSLT.
- [ ] Reuse existing `LaunchEnv` capability/merge only if future Steam work needs launch-only env.
- [ ] Do not add duplicate node capability, proto, supervisor, or process env plumbing in this plan.

- [x] Extend readiness kind helpers to include `steam_gslt`.
- [x] Remove old Steam plaintext storage:
  - [x] remove `steam_game_server_login_token` from `GameServer` proto
  - [x] reserve field `20` and the old field name
  - [x] add a migration that drops `game_server.steam_game_server_login_token`
  - [x] regenerate protobuf and SQL models
  - [x] remove all Go/TS references to the old field
  - [x] do not add fallback, migration, or write-only compatibility behavior
  - [x] existing plaintext tokens are intentionally discarded; admins re-enter tokens through the new secret UI

- [x] Steam GSLT v1 scope:
  - [x] store
  - [x] clear
  - [x] migrate
  - [x] report configured/missing
- [x] Do not add generic `{{STEAM_GSLT}}` secret args in this plan.
- [x] If a specific game later needs command-line GSLT delivery, create a separate game-specific plan with explicit secret-arg redaction/capability work.

- [x] Add Steam readiness RPCs:
  - [x] `SetSteamGSLT(server_id, token)` requires server settings permission.
  - [x] `ClearSteamGSLT(server_id)` requires server settings permission.
  - [x] `GetGameServerReadiness` reports configured/missing state and never returns the token.

## Phase 3: Hytale Account Link

- [x] Implement Hytale in a focused controller package, not generic OAuth infrastructure.
- [x] Use official Hytale values:
  - `client_id=hytale-server`
  - scope `openid offline auth:server`
  - device auth `https://oauth.accounts.hytale.com/oauth2/device/auth`
  - token `https://oauth.accounts.hytale.com/oauth2/token`
  - profiles `https://account-data.hytale.com/my-account/get-profiles`
  - session `https://sessions.hytale.com/game-session/new`

- [x] Extend readiness kind helpers to include `hytale_account`.
- [x] Keep device auth flows in memory:
  - flow IDs bound to server ID, initiating user ID, expiry, and single-use state
  - temporary access token/profile list only in memory until profile selection
  - controller restart loses active device flows; user simply restarts linking
  - server-side polling enforces provider interval/slow-down/expiry regardless of frontend behavior

- [x] Store encrypted refresh token data in `game_server_secret` under a Hytale readiness/auth kind.
- [x] Store only public selected profile metadata in readiness state.
- [x] Do not persist access tokens or game session tokens.
- [x] Validate JSON shape and size before DB writes.

- [x] Before Hytale launch:
  - `CheckStartReadiness` verifies linked profile exists and assigned node supports `launch_env`; it does not refresh tokens.
  - `PrepareLaunchSecrets` takes a per-server lock.
  - Refresh OAuth.
  - Atomically persist the rotated refresh token.
  - Create a game session.
  - Append only `HYTALE_SERVER_SESSION_TOKEN` and `HYTALE_SERVER_IDENTITY_TOKEN` as launch-only values into the final `LaunchEnv`.
  - Do not store Hytale session/identity tokens as user-managed env vars.
- [x] If provider refresh succeeds but DB persistence fails, fail the start, mark/link status as needing attention, and surface relink risk because the old refresh token may already be invalid.
- [x] If refresh fails due to expired/revoked token, mark readiness incomplete and require relink.

- [x] Add Hytale RPCs:
  - [x] `StartHytaleDeviceAuth(server_id)`
  - [x] `PollHytaleDeviceAuth(flow_id)`
  - [x] `SelectHytaleProfile(server_id, flow_id, profile_uuid)`
  - [x] `ClearHytaleAccount(server_id)`
- [x] Require server settings permission for all Hytale mutation/link RPCs.
- [x] Ensure interactive OAuth is needed only for link/relink, not every start.

## Frontend

- [x] Add one compact Readiness panel on the game-server detail page near start controls.
- [x] Keep Minecraft EULA acceptance on the game-server detail readiness panel, not the create flow.
- [ ] Show only missing/blocking items and direct actions:
  - [x] accept Minecraft EULA
  - [x] set Steam token
  - [x] link Hytale account
- [x] Do not duplicate full readiness logic in list/bulk actions; rely on backend failed-precondition errors there.
- [x] Never display stored secrets back to the UI; token inputs are set/clear only.

## Test Plan

- [ ] Backend tests:
  - typed start result propagation and one user-facing error path
  - create-time Minecraft EULA acceptance
  - existing-server EULA grandfathering
  - remote-node `eula.txt` read/write behavior
  - install auto-start behavior
  - readiness table validation
  - nullable `updated_by_user_id` paths
  - [x] Steam plaintext column removal
  - [x] old Steam proto field reservation
  - [x] Steam secret storage through `game_server_secret`
  - [x] LaunchEnv capability fail-closed behavior for readiness-auth launch-only env
  - [x] Hytale flow binding
  - [x] in-memory profile selection
  - [ ] polling throttles
  - [x] refresh-token rotation path
  - [ ] refresh-success/DB-failure path
  - [ ] relink on expired/revoked refresh token

- [ ] Frontend tests:
  - server-view EULA readiness control
  - [x] readiness panel state/actions for each check
  - start button handles backend failed-precondition errors
  - [x] token fields are set/clear only and never echo saved secrets

- [ ] Validation:
  - [x] focused `go test -race -count=1` packages after each phase
  - [x] final `go test -race -count=1 ./...`
  - [x] `golangci-lint run ./...`
  - [x] from `frontend/`: `bun run lint`
  - [x] from `frontend/`: `bun run test`
  - [x] from `frontend/`: `bun run build`
  - [x] `git diff --check`

## Assumptions

- Steam support in this plan means encrypted Steam Game Server Login Token storage/readiness, not Steam account/password/Steam Guard login and not launch arg injection.
- Hytale credentials are scoped per game server.
- Hytale downloader/install automation is out of scope.
- Existing E2E is skipped unless refreshed.
