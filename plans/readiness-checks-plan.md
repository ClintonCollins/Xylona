# Readiness Checks Plan

## Summary

- Implement readiness in three strict phases.
- Phase 1 is only readiness storage + Minecraft EULA, using the existing typed start contract.
- Phase 2 fixes encrypted Steam GSLT storage/readiness using the existing `game_server_secret` helpers.
- Phase 3 adds Hytale account linking using the official device flow.
- Launch environment plumbing is already available through `LaunchEnv`; do not duplicate node protocol/supervisor env work here.
- Secret command-line argument substitution is explicitly out of scope for this plan.

## Progress

- [ ] Phase 1: Readiness storage + Minecraft EULA
- [ ] Phase 2: Encrypted Steam GSLT
- [ ] Phase 3: Hytale account link
- [ ] Frontend readiness UI
- [ ] Final validation

## Phase 1: Readiness Storage + Minecraft EULA

- [ ] Use the existing `actions.StartGameServer` typed result/error contract; do not add another taxonomy.
- [ ] Map readiness blocks to the existing configuration/`FailedPrecondition` path in the RPC layer.
- [ ] Update install post-hook, auto-restart, update restart, and scheduled callers so readiness blocks are terminal/no-retry.
- [ ] Keep one owner for user-facing start failure messages to avoid duplicate logs or console lines.

- [ ] Add `game_server_readiness`:
  - primary key `(game_server_id, kind)`
  - FK cascade to `game_server`
  - `public_data` JSON text
  - `secret_data_encrypted` text for later phases
  - nullable `updated_by_user_id`
  - `created_at` and `updated_at`
  - no SQLite `kind` check; enforce allowed kinds in typed Go helpers to avoid later table rebuild migrations
- [ ] Use either narrow raw SQL helpers or regenerated bob models intentionally; do not mix both approaches accidentally.

- [ ] Add `internal/controller/readiness`.
- [ ] Add `CheckStartReadiness(ctx, gameServer)` before config pre-start and mod updates.
- [ ] Add `PrepareLaunchSecrets(ctx, gameServer)` as a Phase 1 no-op that runs after config/mod work and immediately before `StartProcess`.

- [ ] Add `accept_minecraft_eula` to `CreateGameServerRequest`.
- [ ] Reject Minecraft create with `FailedPrecondition` when `accept_minecraft_eula` is false.
- [ ] Persist the Minecraft EULA readiness row before install starts when accepted.
- [ ] For existing Minecraft servers, lazily read `eula.txt` through the target `NodeClient` when the readiness row is missing.
- [ ] If `eula.txt` contains `eula=true`, persist readiness and allow start.
- [ ] If the node/file read fails, report the node/file error instead of silently treating it as missing acceptance.
- [ ] If the file exists but is not accepted, block with `Minecraft EULA required`.
- [ ] When accepted, write `eula.txt` with `eula=true` through the target `NodeClient` for embedded and remote nodes.

- [ ] Add authenticated readiness RPCs:
  - `GetGameServerReadiness(server_id)` requires existing server view permission.
  - `AcceptMinecraftEula(server_id)` requires existing server settings permission.
- [ ] Keep `GetGameServerReadiness` responses small: `kind`, `required`, `complete`, `blocking`, `message`, and minimal public metadata.

## Phase 2: Encrypted Steam GSLT

- [ ] Reuse existing `game_server_secret` typed helpers; add a narrow secret kind for Steam GSLT.
- [ ] Reuse existing `LaunchEnv` capability/merge only if future Steam work needs launch-only env.
- [ ] Do not add duplicate node capability, proto, supervisor, or process env plumbing in this plan.

- [ ] Extend readiness kind helpers to include `steam_gslt`.
- [ ] Migrate existing `game_server.steam_game_server_login_token` safely:
  - SQL migration adds/extends schema only.
  - Run idempotent post-key startup data migration after `ENCRYPTION_KEY_BASE64` is loaded.
  - For each row, transactionally insert encrypted `game_server_secret` data, decrypt-verify it, then clear plaintext.
  - If write/verification fails, leave plaintext untouched and fail startup loudly.
  - Treat the old proto field as write-only compatibility.
  - Accept create/edit input, store encrypted readiness, and never return the secret.
  - Preserve existing encrypted secrets when edit requests round-trip blank/redacted token.
  - Document one-way compatibility: older binaries after migration will no longer see cleared plaintext tokens.

- [ ] Steam GSLT v1 scope:
  - store
  - clear
  - migrate
  - report configured/missing
- [ ] Do not add generic `{{STEAM_GSLT}}` secret args in this plan.
- [ ] If a specific game later needs command-line GSLT delivery, create a separate game-specific plan with explicit secret-arg redaction/capability work.

- [ ] Add Steam readiness RPCs:
  - `SetSteamGSLT(server_id, token)` requires server settings permission.
  - `ClearSteamGSLT(server_id)` requires server settings permission.
  - `GetGameServerReadiness` never returns the token.

## Phase 3: Hytale Account Link

- [ ] Implement Hytale in a focused controller package, not generic OAuth infrastructure.
- [ ] Use official Hytale values:
  - `client_id=hytale-server`
  - scope `openid offline auth:server`
  - device auth `https://oauth.accounts.hytale.com/oauth2/device/auth`
  - token `https://oauth.accounts.hytale.com/oauth2/token`
  - profiles `https://account-data.hytale.com/my-account/get-profiles`
  - session `https://sessions.hytale.com/game-session/new`

- [ ] Extend readiness kind helpers to include `hytale_account`.
- [ ] Keep device auth flows in memory:
  - flow IDs bound to server ID, initiating user ID, expiry, and single-use state
  - temporary access token/profile list only in memory until profile selection
  - controller restart loses active device flows; user simply restarts linking
  - server-side polling enforces provider interval/slow-down/expiry regardless of frontend behavior

- [ ] Store encrypted refresh token data in `game_server_secret` under a Hytale readiness/auth kind.
- [ ] Store only public selected profile metadata in readiness state.
- [ ] Do not persist access tokens or game session tokens.
- [ ] Validate JSON shape and size before DB writes.

- [ ] Before Hytale launch:
  - `CheckStartReadiness` verifies linked profile exists and assigned node supports `launch_env`; it does not refresh tokens.
  - `PrepareLaunchSecrets` takes a per-server lock.
  - Refresh OAuth.
  - Atomically persist the rotated refresh token.
  - Create a game session.
  - Append only `HYTALE_SERVER_SESSION_TOKEN` and `HYTALE_SERVER_IDENTITY_TOKEN` as launch-only values into the final `LaunchEnv`.
  - Do not store Hytale session/identity tokens as user-managed env vars.
- [ ] If provider refresh succeeds but DB persistence fails, fail the start, mark/link status as needing attention, and surface relink risk because the old refresh token may already be invalid.
- [ ] If refresh fails due to expired/revoked token, mark readiness incomplete and require relink.

- [ ] Add Hytale RPCs:
  - `StartHytaleDeviceAuth(server_id)`
  - `PollHytaleDeviceAuth(flow_id)`
  - `SelectHytaleProfile(server_id, flow_id, profile_uuid)`
  - `ClearHytaleAccount(server_id)`
- [ ] Require server settings permission for all Hytale mutation/link RPCs.
- [ ] Ensure OAuth is needed only for link/relink, not every start.

## Frontend

- [ ] Add one compact Readiness panel on the game-server detail page near start controls.
- [ ] Add a Minecraft EULA checkbox/control to the create flow before install.
- [ ] Show only missing/blocking items and direct actions:
  - accept Minecraft EULA
  - set Steam token
  - link Hytale account
- [ ] Do not duplicate full readiness logic in list/bulk actions; rely on backend failed-precondition errors there.
- [ ] Never display stored secrets back to the UI; token inputs are set/clear only.

## Test Plan

- [ ] Backend tests:
  - typed start result propagation and one user-facing error path
  - create-time Minecraft EULA acceptance
  - existing-server EULA grandfathering
  - remote-node `eula.txt` read/write behavior
  - install auto-start behavior
  - readiness table validation
  - nullable `updated_by_user_id` paths
  - post-key Steam plaintext migration
  - blank edit preservation
  - write-only proto compatibility
  - startup failure on partial migration errors
  - Steam secret storage through `game_server_secret`
  - LaunchEnv capability fail-closed behavior for readiness-auth launch-only env
  - Hytale flow binding
  - in-memory profile selection
  - polling throttles
  - refresh-token rotation locking
  - refresh-success/DB-failure path
  - relink on expired/revoked refresh token

- [ ] Frontend tests:
  - create-flow EULA control
  - readiness panel state/actions for each check
  - start button handles backend failed-precondition errors
  - token fields are set/clear only and never echo saved secrets

- [ ] Validation:
  - focused `go test -race -count=1` packages after each phase
  - final `go test -race -count=1 ./...`
  - `golangci-lint run ./...`
  - from `frontend/`: `bun run lint`
  - from `frontend/`: `bun run test`
  - from `frontend/`: `bun run build`
  - `git diff --check`

## Assumptions

- Steam support in this plan means encrypted Steam Game Server Login Token storage/readiness, not Steam account/password/Steam Guard login and not launch arg injection.
- Hytale credentials are scoped per game server.
- Hytale downloader/install automation is out of scope.
- Existing E2E is skipped unless refreshed.
