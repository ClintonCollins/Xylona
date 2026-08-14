# Xylona

Xylona is a self-hosted game server control panel built to stay approachable for first-time admins while still feeling fast for repeat operators. The backend is written in Go and embeds the Quasar/Vue frontend into a single binary for production builds.

## Status

Xylona is still evolving quickly. Expect active iteration, and treat upgrades the way you would any fast-moving self-hosted control-plane project: verify, back up, and test before promoting to a production environment.

## Features

- Start, stop, and restart game servers
- View console output and runtime status
- Manage files and backups
- Configure game and server settings
- Administer users, permissions, and nodes

## Development

### Prerequisites

- [Go 1.26.6](https://go.dev/doc/install)
- [Bun 1.3.12](https://bun.sh/docs/installation)
- [golangci-lint](https://golangci-lint.run/welcome/install/)
- [Mage](https://magefile.org/) for build, codegen, and E2E helpers

Windows note: frontend installs and normal build tooling use Bun, but Vitest and
Playwright still need Node-compatible execution on Windows until Bun's worker
runtime compatibility catches up.

Optional tooling:

- `sql-migrate` if you need to create or run migration commands manually
- `buf` if you need to regenerate protobuf clients
- `govulncheck` if you want to run the Go vulnerability scan locally
- `goreleaser` if you want to run `mage Build`

### Clone And Install

```bash
git clone https://github.com/ClintonCollins/Xylona.git
```

```bash
cd frontend
bun install
```

### Runtime Configuration

Set your signing and encryption secrets in the environment or `.env`:

```bash
COOKIE_HASH_KEY_BASE64=<base64-encoded securecookie hash key; 32 or 64 bytes recommended>
COOKIE_BLOCK_KEY_BASE64=<base64-encoded 32-byte securecookie block key>
JWT_SECRET_KEY_BASE64=<base64-encoded 32+ byte legacy database-decryption fallback; not used for session auth>
ENCRYPTION_KEY_BASE64=<base64-encoded encryption key; 32 bytes recommended, first 32 bytes used>
```

`ENCRYPTION_KEY_BASE64` is required. Startup validates it before opening or migrating `data.sqlite`, and fails if the key is missing or does not decode to at least 32 bytes. Xylona does not fall back to the JWT secret for new database encryption. Sessions use `COOKIE_*` securecookie keys, not JWT. For compatibility with older deployments, Xylona uses the first 32 decoded bytes of `ENCRYPTION_KEY_BASE64` as the AES-256 key and can use the first 32 decoded bytes of `JWT_SECRET_KEY_BASE64` only as a legacy decryption fallback for ciphertext that was encrypted with that older key.

User passwords use Argon2id in the auth and user RPC paths. `pkg/xycrypt` provides AES-GCM encryption for stored secrets such as node shared secrets. Node join tokens are stored as SHA-256 hashes of 256-bit random values.

Optional runtime controls (defaults are `6h` / `6h` / `24h` to leave room for large file transfers):

```bash
METRICS_ENABLED=false
HTTP_READ_TIMEOUT=6h
HTTP_WRITE_TIMEOUT=6h
HTTP_IDLE_TIMEOUT=24h
XYLONA_UPDATE_RESTART_MODE=self
```

Metrics are disabled by default. Enable them explicitly when you intend to expose a Prometheus scrape target.

### Operating System Services

The controller and remote node can register themselves as automatically starting services on Windows and systemd-based Linux distributions. Service installation registers the current binary in place: move or delete that binary only after uninstalling or reinstalling the service with its new path.

`service install` enables startup after future reboots. Add `--start` to start immediately. The remaining lifecycle commands are the same for both binaries:

```text
service status
service stop
service start
service uninstall
```

For `xylona-node`, place `--data-dir`, pairing, listen, and metrics options after `service install`, as shown below. Foreground options placed before the `service` subcommand are rejected so they cannot be silently ignored.

Uninstalling removes only the operating-system service registration. It does not delete controller configuration, databases, node identities, game servers, backups, or the executable.

#### Windows

From an elevated PowerShell window, install the controller as `Xylona`:

```powershell
.\xylona.exe service install --start
```

Install an already-paired node as `Xylona Node`, using the same node data directory that contains `node-identity.json`:

```powershell
.\xylona-node.exe service install --data-dir C:\Xylona\node-data --start
```

For a new node, the install command can perform the one-time pairing before registering the service:

```powershell
.\xylona-node.exe service install `
  --controller-url https://xylona.example.com `
  --join-token <one-time-token> `
  --data-dir C:\Xylona\node-data `
  --start
```

The join token and other pairing-only options are never stored in the service command line. If pairing succeeds but service registration fails, retain the created identity and retry using only `--data-dir`.

Both Windows services run as `LocalSystem`. Keep the executable, `.env`, database, node identity, and managed server directories writable only by trusted administrators. LocalSystem may not have the same network-share access as an interactive user. Controller and node logs are written to the Windows Application event log under the `Xylona` and `XylonaNode` sources.

The installed service uses the executable directory as its working directory, so controller `.env`, relative `DB_FILE_PATH`, and `data.sqlite` paths continue to resolve beside `xylona.exe`. The node always records its data directory as an absolute service argument.

#### Linux With systemd

Run installation through `sudo`. When `--user` is omitted, Xylona uses the original sudo-invoking user rather than root:

```bash
sudo ./xylona service install --start
sudo ./xylona-node service install --data-dir /srv/xylona-node-data --start
```

To pair a new node during installation:

```bash
sudo ./xylona-node service install \
  --controller-url https://xylona.example.com \
  --join-token <one-time-token> \
  --data-dir /srv/xylona-node-data \
  --start
```

Use `--user <existing-user>` to select a different account. Direct root installation without sudo defaults to root and prints a warning. The installer does not create accounts or recursively change existing data ownership; the selected account must already be able to read and write the controller/node data and all game-server paths it manages. For a new node data path, only missing directories created by the installer are reassigned; inaccessible existing ancestors are rejected before pairing.

The CLI creates and enables `xylona.service` or `xylona-node.service` under `/etc/systemd/system`. Logs are available through:

```bash
journalctl -u xylona.service
journalctl -u xylona-node.service
```

Only systemd is supported by the Linux service CLI. The installer does not alter UFW, firewalld, Windows Firewall, or other host firewall configuration; allow the configured controller and node ports separately.

Controller and node binary updates restart themselves by default. On Unix, Xylona replaces and executes the updated binary in the existing process after graceful shutdown, preserving systemd supervision. The service user must be able to write the executable directory for in-application updates. Windows uses a helper because a running executable cannot be replaced; built-in services have the helper replace the binary and explicitly restart the same service through SCM, with delayed recovery actions retained as a crash fallback. Other external supervisors can use `XYLONA_UPDATE_RESTART_MODE=service-manager`, but must independently let the helper survive the service exit and restart the process after replacement.

Native Windows MSI/MSIX packages are not part of the current release. The intended future native path is separate signed WiX MSI packages for the controller and node, with Windows Installer owning service registration and binary upgrades while mutable data remains outside the installation directory.

System updates are downloaded, checksum-verified, capacity-checked, and staged before Xylona stops game servers on the target node. Update storage keeps the newest rollback executable and at most two unapplied staged updates; confirmed, superseded, expired, and orphaned handoff artifacts are reconciled automatically at startup and before the next update.

### Secret Storage And Recovery

`data.sqlite` and `ENCRYPTION_KEY_BASE64` are a matched recovery set. Keep them together in disaster-recovery planning: the database alone is not enough to recover encrypted control-plane secrets, and the encryption key alone is not enough to recreate state.

Built-in game-server backups cover game-server data, not Xylona control-plane secrets. If you need to back up users, roles, API keys, node identity, or notification credentials, back up `data.sqlite` and your environment secrets separately.

| Secret class | Storage at rest | Included in built-in game-server backups | Notes |
| --- | --- | --- | --- |
| User passwords | Argon2id hash | No | Non-reversible password hashes stored in the database. |
| Node join tokens | SHA-256 hash | No | One-time bootstrap tokens; hashes only. |
| Remote node certificate fingerprints | Plaintext in `data.sqlite` | No | Public certificate pinning material, not secret. |
| Remote node shared secrets | AES-GCM encrypted in `data.sqlite` | No | Encrypted with `ENCRYPTION_KEY_BASE64`. If an enabled node's ciphertext cannot be decrypted, startup warns and skips that node instead of aborting. |
| System config secrets | AES-GCM encrypted in `data.sqlite` | No | Includes values such as SMTP credentials. |
| Notification channel configs | AES-GCM encrypted in `data.sqlite` | No | Includes webhook URLs and similar credentials. |
| Node API keys | AES-GCM encrypted in `data.sqlite` | No | Provider/API tokens stored encrypted at rest. |
| Cookie secrets | Environment / `.env` | No | Session cookie HMAC/block keys. Not stored in the database. |
| JWT secret | Environment / `.env` | No | Required at startup, but unused for authentication. Kept only as a legacy AES decryption fallback for older ciphertext. |

If you suspect `data.sqlite` or a backup copy of it was exposed, treat remote node shared secrets as compromised. Xylona does not rotate those secrets automatically in that situation. Rotate them by removing affected remote nodes while Xylona is stopped, then re-pairing those nodes after restart.

### Mod Provider Integrity

Xylona verifies provider-advertised download integrity where the current provider integration exposes a checksum:

| Provider | Integrity mode | Notes |
| --- | --- | --- |
| PaperMC | Checksum-verified | Requires advertised SHA-256; install/update fails if missing or mismatched. |
| Hangar | Checksum-verified | Requires advertised SHA-256; install/update fails if missing or mismatched. |
| Modrinth | Checksum-verified | Requires advertised SHA-256 on the selected primary file; install/update fails if missing or mismatched. |
| Thunderstore | Best-effort | Current integration records the downloaded SHA-256 after the fact, but the package metadata path used by Xylona does not supply a checksum to verify before install. |
| Steam Workshop | Best-effort | `steamcmd` output is not integrity-verified by Xylona in this flow. |

### Operational Endpoints

Xylona exposes separate probes for process liveness and application readiness:

- `GET /api/health` is a narrow liveness probe. It only proves the HTTP process is up and can answer requests.
- `GET /api/ready` is the readiness probe. It returns `200` only after the database ping succeeds and returns `503` when the database is unavailable.
- `GET /metrics` is the Prometheus scrape endpoint. It is only mounted when `METRICS_ENABLED=true`.

`/metrics` is served from the main HTTP listener, so enable it only when your scrape target and network exposure are intentional.

### Palworld Map Tiles

Palworld map imagery is optional and is not embedded in Xylona releases. A server administrator can open **Map imagery** on a Palworld live map and select **Install / repair**. Xylona then downloads the Palpagos and World Tree tile pyramids into `palworld-map-tiles` beside the resolved `data.sqlite` path and serves them from `/palworld-map-tiles/` on the controller's existing HTTP listener.

Existing valid tiles are reused, so running the action again repairs missing or invalid files without downloading the complete dataset again. Public map links use the same controller-hosted tiles and do not make visitors request imagery from the upstream tile host.

### Backend Workflow

Xylona uses SQLite locally and applies embedded SQL migrations automatically on startup. You do not need `sql/dbconfig.yml`, and you do not need a manual migration step just to boot the app.

The Go layout follows the usual `cmd`, `internal`, and `pkg` split: binaries live in `cmd/`, app-owned backend code lives in `internal/`, and reusable domain helpers live in `pkg/`.

The Go binary embeds `internal/webui/dist`, so build the frontend bundle once before compiling or running the backend:

```bash
cd frontend
bun run build
```

```bash
go run ./cmd/xylona
```

For a compiled binary:

```bash
go build -o xylona ./cmd/xylona
```

The default app bind is `localhost:8080`.

### Frontend Workflow

Run the backend in one terminal, then start the Quasar dev server in another:

```bash
cd frontend
bun run dev
```

The frontend dev server proxies API traffic to the backend using the project proxy configuration. The frontend build targets modern evergreen browsers with native `BigInt` support, including Safari 15.6+.

### Common Commands

Backend:

```bash
go test -race -count=1 ./...
```

```bash
golangci-lint run ./...
```

Frontend:

```bash
cd frontend
bun run lint
```

```bash
cd frontend
bun run format:check
```

```bash
cd frontend
bun run test
```

```bash
cd frontend
bun run build
```

Mage helpers:

```bash
mage Build
```

```bash
mage GenerateProto
```

Protocol Buffer generation runs locally and never uploads source schemas to
Buf. Install the pinned Go plugins once, and run `bun install` in `frontend/`
before generating:

```bash
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.18.1
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.3
```

```bash
mage GenerateModels
```

```bash
mage SQLMigrateNew <name>
```

```bash
mage SQLMigrateUp
```

```bash
mage SQLMigrateDown
```

## E2E Testing

The browser suite is intentionally small and workflow-focused. Backend edge
cases and controller/node boundary behavior belong in Go tests.

Fast smoke suite:

```bash
cd frontend
bun run e2e:smoke
```

```bash
mage E2ESmoke
```

Full local-controller browser suite:

```bash
cd frontend
bun run e2e
```

Useful variants:

```bash
mage E2E
```

```bash
mage E2ERemoteNode
```

```bash
mage E2EHeaded
```

```bash
mage E2EReport
```

Local process integration tests use real controller and `xylona-node` binaries
without external network calls:

```bash
mage IntegrationLocal
```

Live provider/API integration tests are separate and opt-in:

```bash
mage IntegrationLive
```

The `cmd/e2e` orchestrator exposes `setup`, `teardown`, `status`, and `seed`.
Use `--mode local-controller` for the normal browser environment and
`--mode remote-node` for controller plus remote node coverage. Common flags are
`--http-port`, `--node-port`, `--e2e-dir`, and `--project-root`; `seed` keeps
`--db`, `--username`, `--password`, and `--migrations`.

## Before Opening A PR

- Run `golangci-lint run ./...`
- Run `govulncheck ./...`
- Run `go test -race -count=1 ./...`
- Run `bun run lint` from `frontend/`
- Run `bun run format:check` from `frontend/`
- Run `bun run test` from `frontend/`
- Run `bun audit --audit-level=high` from `frontend/`
- Run `bun run build` from `frontend/`
- Rebuild generated code with `mage GenerateProto` or `mage GenerateModels` only when you changed the corresponding sources
