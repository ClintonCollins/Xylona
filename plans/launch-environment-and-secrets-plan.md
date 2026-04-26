# Launch Environment And Server Secrets Plan

## Summary

- Add a small launch environment foundation before Hytale/readiness auth work.
- Support per-server normal env vars, encrypted per-server secret env vars, and optional game default normal env vars.
- Use one secret-safe node launch env path; keep normal/secret distinctions in controller storage and API, not node protocol.
- Do not add required-env schema, secret command-line args, Steam GSLT migration, Hytale OAuth, or generic vault behavior in this plan.

## Progress

- [x] Phase 0: Start contract prerequisite
- [x] Phase 1: Secret-safe launch env plumbing
- [x] Phase 2: Per-server env + secret env
- [x] Phase 3: Game default normal env
- [x] Update readiness plan
- [x] Final validation

## Phase 0: Start Contract Prerequisite

- [x] Make `actions.StartGameServer` return a typed result/error so fail-closed env/capability errors do not look successful.
- [x] Map configuration/capability blocks to Connect `FailedPrecondition`.
- [x] Map operational start failures to appropriate operational errors.
- [x] Keep `StartGameServerResponse.error` wire-compatible, but treat Connect errors as the real contract.
- [x] Update install/update/auto-restart/scheduled callers so configuration/capability blocks are terminal/no-retry.
- [x] Ensure exactly one user-facing console message for blocked starts.

## Phase 1: Secret-Safe Launch Env Plumbing

- [x] Add runtime node capabilities separate from self-update capabilities:
  - protocol version
  - feature flag `launch_env`
  - missing/unimplemented capability RPC means remote node has no `launch_env` support
- [x] Add `GetRuntimeCapabilities` or equivalent to NodeService and `NodeClient`.
- [x] Add one `LaunchEnv map[string]string` to:
  - `node.ProcessConfig`
  - node proto `StartProcessRequest`
  - nodeclient mappings
  - supervisor `PreparedCommand`
- [x] Do not add separate `Env` / `SecretEnv` fields to the node protocol.
- [x] Define `launch_env` as secret-safe process-env injection:
  - append env only at process launch
  - do not log env
  - do not expose env in snapshots or payloads
  - clear `exec.Cmd.Env` after `Start()` succeeds
- [x] Deep-copy `LaunchEnv` at every boundary.
- [x] Keep launch env off long-lived `supervisor.Command` state, snapshots, frontend payloads, and logs.
- [x] Preserve the existing sanitized child environment from `buildChildEnvironment`, then append validated launch env immediately before process launch.
- [x] Apply launch env only to runtime game-server starts in v1, not install/update/internal commands.
- [x] Fail closed before launch when a remote node lacks `launch_env` and the start has non-empty launch env.
- [x] Allow starts with empty env to continue on older nodes.
- [x] Document trust boundary: Xylona avoids its own env leaks, but node host admins, process inspectors, crash dumps, and the launched game/plugin can still read env values.

## Phase 2: Per-Server Env + Secret Env

- [x] Add `game_server.env_vars text not null default '[]'`.
- [x] Run `mage GenerateModels` after the SQL migration because `game_server` is generated-model-backed.
- [x] Use generated models for `game_server.env_vars` in existing setters/converters.

- [x] Add `game_server_secret` table:
  - primary key `(game_server_id, kind, name)`
  - FK to `game_server` with cascade delete
  - `value_encrypted` text not null
  - nullable `updated_by_user_id` FK to user with `ON DELETE SET NULL`
  - `created_at`
  - `updated_at`
  - no `public_data` in v1
  - no SQLite `kind` check; allowed kinds enforced by typed Go helpers
- [x] Initial secret kind is `env`, where `name` is the env var name.
- [x] Add small DB helper methods for encrypted secret env:
  - set/replace
  - clear
  - list configured state
  - decrypt for launch only
- [x] Keep secret helper transactions responsible for encryption/decryption and timestamp updates.

- [x] Add public proto message `EnvironmentVariable { string name; string value; }` for environment RPC payloads.
- [x] Do not add server env values to broad `GameServer` or game-server list payloads.
- [x] Add server environment RPCs:
  - `GetGameServerEnvironment(server_id)`
  - `UpdateGameServerEnvironment(server_id, repeated EnvironmentVariable env_vars)`
  - `SetGameServerSecretEnv(server_id, name, value)`
  - `ClearGameServerSecretEnv(server_id, name)`
- [x] `GetGameServerEnvironment` returns:
  - game default env
  - server env
  - effective normal env
  - secret env states `{name, configured, updated_at}`
  - validation/conflict status for values that would block start
- [x] `GetGameServerEnvironment` requires server settings permission because normal values and secret key names can be sensitive.
- [x] Server env mutation requires server settings permission plus either superuser or `game.allow_start_arg_editing`.
- [x] Secret values are never returned by any RPC.

## Phase 3: Game Default Normal Env

- [x] Add `game.default_env_vars text not null default '[]'`.
- [x] Run `mage GenerateModels` after the SQL migration because `game` is generated-model-backed.
- [x] Update generated-model-based game converters/setters intentionally.
- [x] Use dedicated game environment RPCs for read/write rather than broad `Game` payloads.
- [x] Do not expose default env through broad `Game` or nested `GameServer.game` unless explicitly redacted.
- [x] Add optional game default env editing for superusers/game editors.
- [x] Game default env contains visible normal values only; no game-level secret defaults in v1.

- [x] Game definition import/export:
  - add optional top-level `default_env_vars` section
  - emit only when non-empty
  - treat missing and empty default env as equivalent
  - canonical hashing treats missing/empty as equivalent so existing official definitions do not churn
  - document that env defaults require a new binary to preserve on re-export

## Validation Rules

- [x] Add a small package such as `internal/controller/launchenv` for parsing, validation, merge, and redaction helpers.
- [x] Store normal env JSON as an array of `{name,value}` to keep UI order stable.
- [x] Validate names for normal and secret env:
  - portable regex `^[A-Za-z_][A-Za-z0-9_]*$`
  - max name length `128`
  - no NUL/control characters
  - case-insensitive uniqueness within each stored list
- [x] Validate values:
  - max value length `4096` bytes
  - no NUL bytes
  - empty string allowed as an explicit value
- [x] Limit each normal/secret list to `64` entries.
- [x] Enforce a conservative merged custom env size cap, for example `16 KiB`.
- [x] Keep the final Windows environment block below the platform limit.
- [x] Deny dangerous env names/prefixes case-insensitively:
  - `PATH`
  - `COMSPEC`
  - `SYSTEMROOT`
  - `WINDIR`
  - `PATHEXT`
  - `LD_*`
  - `DYLD_*`
  - `JAVA_TOOL_OPTIONS`
  - `_JAVA_OPTIONS`
  - `NODE_OPTIONS`
  - `PYTHONPATH`
- [x] Keep the deny list exact and test-covered; do not rely on vague “similar variables” logic.

## Merge Rules

- [x] Game default normal env can be overridden by per-server normal env.
- [x] Secret env names cannot duplicate the effective normal env.
- [ ] Future readiness launch-only env names cannot duplicate user normal/secret env names.
- [x] No secret-overrides-normal behavior in v1; duplicates are validation/configuration errors.

## Controller Start Flow

- [x] `StartGameServer` reloads game server, game relation, normal env JSON, and secret env metadata inside the action path.
- [x] Resolve the target node client and runtime capabilities before decrypting secret env values.
- [x] If effective normal env or secret env exists and the node lacks `launch_env`, return a configuration/capability block before decrypting secrets.
- [x] Run config pre-start and mod updates as today.
- [x] Resolve command/args as today.
- [x] Decrypt user-managed secret env only after capability checks pass and immediately before launch.
- [x] Merge game default env, per-server normal env, and user-managed secret env into one final `LaunchEnv` map.
- [ ] Later readiness/auth work may append launch-only env into the final map immediately before launch after its own capability checks.
- [x] Pass only the final `LaunchEnv` to `client.StartProcess`.

## Frontend

- [x] Add one advanced Environment section to game-server settings.
- [x] Include a compact table for visible per-server normal env vars.
- [x] Include a compact secret env manager:
  - list secret key names and configured state only
  - set/replace one secret value
  - clear one secret value
  - never display saved secret values
- [x] Add optional game default env editing to game edit UI in a compact table.
- [x] Do not build inheritance graphs, required flags, templates, scopes, or policy controls.

## Readiness Plan Update After Implementation

- [x] Update `plans/readiness-checks-plan.md` after this plan lands:
  - remove duplicate node env plumbing from readiness Phase 2
  - readiness uses `LaunchEnv` capability and final launch env merge
  - Steam GSLT migration remains readiness Phase 2, but uses `game_server_secret` typed helpers
  - Hytale refresh token uses `game_server_secret` typed helpers under a readiness/auth kind
  - Hytale session/identity tokens are launch-only env values appended to `LaunchEnv`, not stored user env vars

## Out Of Scope

- Steam GSLT plaintext migration.
- Hytale OAuth implementation.
- Required env schemas.
- Game-level secret defaults.
- Secret command-line args.
- Install/update command env injection.
- Generic vault behavior.

## Test Plan

- [x] Backend:
  - [x] start contract propagation for env/capability blocks
  - [x] runtime capability RPC
  - [x] old remote node `Unimplemented` fail-closed behavior
  - [x] normal env validation, ordering, persistence, and merge behavior
  - [x] game default env import/export and canonical hash behavior
  - [x] secret env set/clear/list status
  - [x] secret env encryption at rest
  - [x] `created_at` preservation and `updated_at` bump behavior
  - [x] secret values never returned by RPCs or logs
  - [x] permission checks for view/update/secret mutation
  - [x] launch env reaches child process
  - [x] launch env is not retained on snapshots or command state
  - [x] `exec.Cmd.Env` is cleared after successful process start
  - [x] launch env applies only to `Status_ONLINE` runtime starts
  - [x] capability checked before secret decrypt

- [x] Frontend:
  - [x] advanced server env section
  - [x] normal env table validation
  - [x] secret env set/clear/status behavior
  - [x] game default env editor
  - [x] saved secret values never echoed

- [x] Validation:
  - [x] `mage GenerateProto`
  - [x] `mage GenerateModels`
  - [x] focused `go test -race -count=1` packages
  - [x] final `go test -race -count=1 ./...`
  - [x] `golangci-lint run ./...`
  - [x] from `frontend/`: `bun run lint`
  - [x] from `frontend/`: `bun run test`
  - [x] from `frontend/`: `bun run build`
  - [x] `git diff --check`

## Assumptions

- Env vars are launch-time process env only; they do not modify persistent node/service environment.
- User-managed secret env vars are per-server in v1.
- The existing stale E2E suite remains skipped unless refreshed.
