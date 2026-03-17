# Xylona — Project Improvement Notes

A living document for tracking code quality issues, missing features, and architectural observations.
Organized by priority. Specific file:line references included where applicable.

---

## Security (Fix First)

### Cookie Secure Flag Disabled
**File:** `api/rpc/auth.go:107, 116, 146, 156`
Session cookies are set with `Secure: false`, meaning they can be transmitted over plain HTTP.
Should be conditional on environment: `Secure: !isDevelopment`.

### Session Token Logged
**File:** `api/rpc/auth.go:75`
`log.Printf("User session: %+v", x)` logs the entire session object including the token.
Session tokens must never appear in logs — replace with structured logging that excludes sensitive fields.

### Path Traversal in File Operations
**File:** `api/rpc/game_server_file_operations.go`
File operation endpoints accept user-supplied `fullFilePath` with no evidence of `..` sanitization.
A request like `../../etc/passwd` could escape the game server directory.
Paths should be canonicalized and validated against the server's base directory before use.

### No Rate Limiting on Auth Endpoints
**File:** `api/rpc/auth.go` (login handler)
The login endpoint has no rate limiting or lockout, making it vulnerable to brute force.
Should add per-IP or per-username throttling using a middleware or token bucket.

### Federation Peer Ownership Not Validated
**File:** `api/rpc/federation.go:48-56`
`authenticateRequest()` checks that a NodeID is present in context but does not verify the peer
actually owns that NodeID. The entire trust model depends on mTLS being correctly configured —
if it's misconfigured, the federation surface is unguarded.

### Trust-On-First-Use Certificate Model
**File:** `helpers/federation-mtls.go`
Fingerprints are exchanged out-of-band during pairing with no revocation mechanism.
A compromised peer certificate remains trusted indefinitely.
Consider adding a certificate revocation list or short-lived rotating certificates.

### No Replay Protection on Federation Requests
Federation requests carry no nonce or request ID. Replaying captured requests to idempotent
endpoints (e.g., status updates, sync) is possible. Add a short-lived nonce or timestamp window check.

---

## Panicking Unimplemented Endpoints

These methods are registered in the proto and reachable by clients but will crash the server:

**File:** `api/rpc/game.go:121-133`
```
ImportGame  — panic("implement me")
ExportGame  — panic("implement me")
GetBranches — panic("implement me")
```

**File:** `api/rpc/ip.go:31-39`
```
AddIP    — stub, no implementation
RemoveIP — stub, no implementation
```

All should return `connect.NewError(connect.CodeUnimplemented, ...)` until properly implemented.

---

## Bug-Level Issues

### Silent Logout Failure
**File:** `api/rpc/auth.go:129-133`
If the session delete DB call fails, the handler logs a warning but returns HTTP 200.
The client believes logout succeeded while the session remains valid in the database.

### Background Task Errors Silently Dropped
**File:** `actions/background_tasks.go:112`
`_ = errGroup.Wait()` discards all errors from background goroutines.
Failures in background work are invisible — should log each error with context.

### File Close Errors Ignored
**Files:** `actions/file_actions.go:136, 237, 275, 285, 392, 582, 635, 841, 909, 935` and others
`_ = file.Close()` silently discards close errors. A failed close can indicate data was not flushed
to disk. At minimum, these should be logged with the filename for diagnostics.

---

## Performance

### N+1 Query in RBAC Listing
**File:** `api/rpc/rbac.go:32-36`
Loads all roles, then queries permissions separately for each role in a loop.
Replace with a JOIN or use bob's eager-loading to fetch in one query.

### Super User Count Loads All Users
**File:** `api/rpc/user.go:276-289`
Iterates all users to count super users. Should be `SELECT COUNT(*) FROM user WHERE super_user = 1`.

### No Pagination on List Endpoints
`ListGameServers`, `ListNodes`, `ListUsers`, and others return unbounded result sets.
Add cursor- or offset-based pagination before deployments with large datasets.

### WebSocket Output Buffer Unbounded
**File:** `supervisor/runner.go`
Game server stdout is buffered for replay to new WebSocket connections with no size cap.
A chatty server could grow this indefinitely. Add a ring buffer or max-line cap.

---

## Code Quality

### Hard-coded Session Duration
**File:** `api/rpc/auth.go:72`
`time.Now().Add(24 * time.Hour * 30)` — 30-day sessions are hard-coded.
Should be an environment-variable-backed config option.

### Printf-Style Logging in Federation
**File:** `helpers/federation-mtls.go`
Uses `fmt.Printf()` for a certificate regeneration warning. Should use `log.Warn()` with
structured fields for consistency with the rest of the codebase.

### TODO Left Incomplete
**File:** `actions/actions.go:102`
Comment says "TODO walk directory recursively to get size" but the code already uses
`filepath.WalkDir`. Either the TODO is stale (remove it) or the implementation is wrong (fix it).

### Telnet Console Not Implemented
**File:** `supervisor/runner.go:693`
TODO comment for sending input via telnet. Currently only stdin works. This blocks support for
games that use a telnet-style admin console (e.g., 7 Days to Die, some modded servers).

### Magefiles Deploy Uses String Interpolation for Shell Commands
**File:** `magefiles/mage.go:131-149`
The deploy task builds a shell command by string concatenation (e.g., `"sudo systemctl stop " + service`).
If `service` were ever user-influenced, this would be a command injection vector.
Use `exec.Command()` with separate arguments instead.

---

## Testing Gaps

### Backend — No Tests

| Package / File | What's Missing |
|---|---|
| `db/game-server.go` | All CRUD operations |
| `db/game.go` | Game creation, updates, queries |
| `db/ip.go` | IP operations |
| `db/node.go` | Node queries |
| `db/revoked-jwt.go` | JWT revocation and lookup |
| `db/local-settings.go` | Settings persistence |
| `db/local-secret-keys.go` | Secret key storage |
| `db/node-sync-queue.go` | Sync queue operations |
| `api/rpc/auth.go` | Login, logout, session validation |
| `api/rpc/game.go` | Any coverage (panicking endpoints) |
| `api/rpc/ip.go` | Any coverage |
| `helpers/assertions.go` | Model↔proto conversion correctness |

### Frontend — Very Thin Coverage
Around 47 Vue components exist with only 3 test files. Priority components to cover:

- `GameServerLayout.vue` — permission gating logic
- `frontend/src/stores/xylona.ts` — auth state, initial fetch
- File operation components — path handling
- Any component with significant conditional rendering

### Integration Tests Missing
- Federation pairing + request flow end-to-end
- Game server lifecycle (create → start → stop → delete)
- RBAC grant → permission check flow

---

## Architecture

### XylonaService Is Too Large
`api/rpc/game_server.go`, `auth.go`, `rbac.go`, `user.go`, `node.go` etc. all hang off a single
`XylonaService` struct. At 40+ methods it's hard to navigate and reason about dependencies.
Consider splitting into focused services: `AuthService`, `GameServerService`, `NodeService`, etc.,
each with its own file and only the dependencies it needs.

### Federation Logic Is Spread Across Too Many Packages
Federation-related code lives in `api/rpc/`, `helpers/`, `actions/`, and `db/` with no clear
ownership boundary. A dedicated `api/federation/` package (or at least clearer internal naming)
would make the trust boundary and message flow easier to audit.

### No Audit Trail
Critical operations (role assignment, server creation/deletion, federation pairing, user deletion)
leave no audit record. Adding a lightweight `audit_log` table with `actor_id`, `action`, `target_id`,
`created_at` would be valuable for production use.

### Database Schema — Orphan Risk on User Deletion
When a user is deleted, it's unclear what happens to game servers they own — foreign key behavior
should be explicit (CASCADE or RESTRICT) rather than implicit.

---

## Missing Features (Game Server Panel Context)

These are capabilities users would expect from a mature game server control panel.

### Backup & Restore
The RBAC schema already defines a `game_server.backup` permission, but no backup implementation
exists. At minimum: manual trigger to zip the server directory, configurable retention, download link.
Stretch: scheduled backups, restore from backup.

### Scheduled Tasks
No way to schedule server start, stop, restart, or backup. A cron-style scheduler (even simple
`hh:mm` daily triggers) would reduce manual work for common maintenance windows.

### Metrics & Monitoring
- No CPU/RAM tracking per game server process
- No disk space warnings
- No crash/restart counter or uptime display
- No alerting (email, webhook, Discord) on server crash or resource exhaustion

### Log Viewer
Users cannot view server logs through the UI. A real-time log tail (WebSocket-streamed, like the
console but read-only replay from a log file) would be high-value.

### Player Management
For games that support it: whitelist management, ban list, operator/admin promotion, player kick.
Minecraft's RCON or file-based player management would be a natural starting point.

### Server Templates / Presets
Allow saving a game server configuration as a template for quick re-deployment of common setups.

### One-Click Game Updates
Detect when a newer version of the game binary is available and provide an in-panel update flow.
Minecraft has a well-known version manifest API that could power this.

### Federation Health Dashboard
Currently there is no UI to view the health of paired nodes — whether they're reachable,
last sync time, certificate expiry. A simple federation status page would reduce operational friction.

### Multi-Factor Authentication
Session-based auth with a username/password is the only option. TOTP (e.g., Google Authenticator)
would be a straightforward addition for admin accounts.

---

## Frontend UX Gaps

- **Missing loading states** on most data-fetching pages (UserList, NodeList, GameServerList)
- **Missing empty states** — many lists show nothing with no explanation when empty
- **Error messages not surfaced** — errors are often logged to console only, not shown in the UI
- **Table rows not fully clickable** — only the link text in a cell triggers navigation, not the whole row
- **No confirmation for destructive actions** in some places (inconsistent — some have dialogs, some don't)
- **No dark mode persistence** — preference not stored across sessions

---

## Quick Wins (Low Effort, High Value)

1. Replace `panic("implement me")` with `connect.CodeUnimplemented` returns
2. Remove the session token `log.Printf`
3. Add `Secure: isProduction` to cookie configuration
4. Fix the silent logout failure to return an error
5. Add `Version` field to `GameServerModelToProto` (already done — remove from list when merged)
6. Replace super user `COUNT` loop with a DB-level count query
7. Add structured `log.Warn()` for the federation certificate regeneration message
8. Add basic path canonicalization to file operation endpoints

---

*Last updated: 2026-03-17*
