## Code Review Report

### Summary
| Metric | Value |
|--------|-------|
| **Scope** | `feature/federated-user-management` vs `master` — 269 files, ~47.5k lines added |
| **Findings** | 10 total: 1 Critical, 2 High, 5 Medium, 2 Low |
| **Automated** | go vet: clean, go build: clean, tsc: clean (pre-existing e2e issues only) |
| **Verdict** | **NEEDS WORK** |

---

### Critical

**C1: WebSocket console subscription bypasses RBAC** — `api/websocket/websocket.go:1204-1218`
The `GetGameServerConsole` websocket handler adds the server ID to subscribed outputs without checking `game_server.console` permission. Any authenticated user can subscribe to any local server's console output. This undermines the entire RBAC system the branch implements.

### High

**H1: Swapped function names** — `api/websocket/websocket.go:274-333`
`listenForGameServersAdded` subscribes to `TopicGameServerRemoved` and vice versa. Not a runtime bug (both are called), but a maintenance time bomb.

**H2: `ListServerSummaries` returns all servers without acting-user identity** — `api/rpc/federation.go:232-248`
When no acting-user headers are set, the permission filter is skipped entirely. A peer node can enumerate all servers (IPs, ports, metrics) without user-level authorization.

### Medium

- **M1**: `sendOwnedServersMetrics` broadcasts all remote server metrics to every user without RBAC filtering (`websocket.go:713-770`)
- **M2**: `granted_by` FK in RBAC migration lacks `ON DELETE CASCADE/SET NULL` — will block user deletion (`migrations/20260316004000`)
- **M3**: `computeEffectivePermissions` returns nil on error, frontend shows all controls (`permissions.go:50-52`)
- **M4**: `pollRemoteMetrics` fetches all remote servers without acting-user identity (compounds M1)
- **M5**: `wrapRemoteRPCError` forwards raw remote error messages to clients (`remote-errors.go:14`)

### Low

- **L1**: Rate limiter localhost check uses `RemoteAddr` after `RealIP` middleware — bypassable via `X-Forwarded-For` behind proxies
- **L2**: `GetGameServersAccessibleByUser` does N+1 queries for granted servers (`db/game-server.go:113`)

### Positives

- Thorough RBAC model with proper system roles, transactions, and FK integrity
- Excellent test coverage across RBAC, federation, user management, and frontend
- File upload/download endpoints now properly require authentication
- Security headers and rate limiting added
- Clean `dispatchGameServerRequest` pattern for local/remote routing
- Super-user deletion protection works correctly

---

### Required Before Merge
1. **Fix C1** — Add RBAC permission check to websocket console subscription
2. **Fix H2** — Either require acting-user identity or document the trust model and reduce returned fields

### Strongly Recommended
3. **Fix H1** — Swap the misleading function names before someone adds logic to the wrong one
4. **Fix M2** — Add `ON DELETE CASCADE` or `SET NULL` to `granted_by` FK (will block user deletion in production)
