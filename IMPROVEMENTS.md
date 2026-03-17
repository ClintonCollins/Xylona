# Xylona — Project Improvement Notes

A living document for tracking code quality issues, missing features, and architectural observations.
Organized by priority. Specific file:line references included where applicable.

---

## Security

### Trust-On-First-Use Certificate Model
**File:** `helpers/federation-mtls.go`
Fingerprints are exchanged out-of-band during pairing with no revocation mechanism.
A compromised peer certificate remains trusted indefinitely.
Consider adding a certificate revocation list or short-lived rotating certificates.

### No Replay Protection on Federation Requests
Federation requests carry no nonce or request ID. Replaying captured requests to idempotent
endpoints (e.g., status updates, sync) is possible. Add a short-lived nonce or timestamp window check.

---

## Performance

### No Pagination on List Endpoints
`ListGameServers`, `ListNodes`, `ListUsers`, and others return unbounded result sets.
Add cursor- or offset-based pagination before deployments with large datasets.

---

## Code Quality

### Telnet Console Not Implemented
**File:** `supervisor/runner.go:693`
TODO comment for sending input via telnet. Currently only stdin works. This blocks support for
games that use a telnet-style admin console (e.g., 7 Days to Die, some modded servers).

---

## Testing Gaps

### Frontend — Still Missing

- Any component with significant conditional rendering

### Integration Tests Missing
- Federation pairing + request flow end-to-end
- Game server lifecycle (create -> start -> stop -> delete)
- RBAC grant -> permission check flow

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

- **Missing empty states** — many lists show nothing with no explanation when empty
- **Table rows not fully clickable** — only the link text in a cell triggers navigation, not the whole row
- **No confirmation for destructive actions** in some places (inconsistent — some have dialogs, some don't)
- **No dark mode persistence** — preference not stored across sessions

---

*Last updated: 2026-03-17*
