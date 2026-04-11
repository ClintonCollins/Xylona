# Xylona

Xylona is a self-hosted game server control panel built to stay approachable for first-time admins while still feeling fast for repeat operators. The backend is written in Go and embeds the Quasar/Vue frontend into a single binary for production builds.

## Status

Xylona is still evolving quickly. Expect active iteration, and treat upgrades the way you would any fast-moving self-hosted control-plane project: verify, back up, and test before promoting to a production environment.

## Features

- Start, stop, and restart game servers
- View console output and runtime status
- Manage files and backups
- Configure game and server settings
- Administer users, permissions, nodes, and federation

## Development

### Prerequisites

- [Go 1.26.2](https://go.dev/doc/install)
- [Node.js 22 LTS](https://nodejs.org/)
- [pnpm 10.32.1](https://pnpm.io/installation)
- [golangci-lint](https://golangci-lint.run/welcome/install/)
- [Mage](https://magefile.org/) for build, codegen, and E2E helpers

Optional tooling:

- `sql-migrate` if you need to create or run migration commands manually
- `buf` if you need to regenerate protobuf clients
- `goreleaser` if you want to run `mage Build`

### Clone And Install

```bash
git clone https://github.com/ClintonCollins/Xylona.git
```

```bash
pnpm --dir frontend install
```

### Runtime Configuration

Set your signing and encryption secrets in the environment or `.env`:

```bash
COOKIE_HASH_KEY_BASE64=<base64-encoded securecookie hash key; 32 or 64 bytes recommended>
COOKIE_BLOCK_KEY_BASE64=<base64-encoded 32-byte securecookie block key>
JWT_SECRET_KEY_BASE64=<base64-encoded 32+ byte signing key>
ENCRYPTION_KEY_BASE64=<base64-encoded 32+ byte encryption key>
```

`ENCRYPTION_KEY_BASE64` is required. Xylona does not fall back to the JWT secret for database encryption, and startup fails if the key is missing or too short.

User passwords use bcrypt in the auth and user RPC paths. `pkg/xycrypt` provides AES-GCM encryption for stored secrets such as node API keys, and its Argon2id helpers are used for non-password secret tokens such as local and federation secret keys.

Optional runtime controls:

```bash
METRICS_ENABLED=false
HTTP_READ_TIMEOUT=15m
HTTP_WRITE_TIMEOUT=15m
HTTP_IDLE_TIMEOUT=30m
FEDERATION_READ_TIMEOUT=15m
FEDERATION_WRITE_TIMEOUT=15m
FEDERATION_IDLE_TIMEOUT=30m
```

Metrics are disabled by default. Enable them explicitly when you intend to expose a Prometheus scrape target.

### Operational Endpoints

Xylona exposes separate probes for process liveness and application readiness:

- `GET /api/health` is a narrow liveness probe. It only proves the HTTP process is up and can answer requests.
- `GET /api/ready` is the readiness probe. It returns `200` only after the database ping succeeds and returns `503` when the database is unavailable.
- `GET /metrics` is the Prometheus scrape endpoint. It is only mounted when `METRICS_ENABLED=true`.

`/metrics` is served from the main HTTP listener, so enable it only when your scrape target and network exposure are intentional.

### Backend Workflow

Xylona uses SQLite locally and applies embedded SQL migrations automatically on startup. You do not need `sql/dbconfig.yml`, and you do not need a manual migration step just to boot the app.

The Go binary embeds `frontend/dist`, so build the frontend bundle once before compiling or running the backend:

```bash
pnpm --dir frontend run build
```

```bash
go run .
```

For a compiled binary:

```bash
go build -o xylona
```

The default app bind is `localhost:8080`.

### Frontend Workflow

Run the backend in one terminal, then start the Quasar dev server in another:

```bash
pnpm --dir frontend run dev
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
pnpm --dir frontend run lint
```

```bash
pnpm --dir frontend run test
```

```bash
pnpm --dir frontend run build
```

Mage helpers:

```bash
mage Build
```

```bash
mage GenerateProto
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

Single-node suite:

```bash
pnpm --dir frontend run e2e
```

```bash
mage E2E
```

Federation suite:

```bash
pnpm --dir frontend run e2e:federation
```

```bash
mage E2EFederation
```

Useful variants:

```bash
mage E2EHeaded
```

```bash
mage E2EFederationHeaded
```

```bash
mage E2EReport
```

```bash
mage E2EFederationReport
```

Set `E2E_KEEP_DATA=1` to preserve federation test data for debugging.

## Before Opening A PR

- Run `golangci-lint run ./...`
- Run `go test -race -count=1 ./...`
- Run `pnpm --dir frontend run lint`
- Run `pnpm --dir frontend run test`
- Run `pnpm --dir frontend run build`
- Rebuild generated code with `mage GenerateProto` or `mage GenerateModels` only when you changed the corresponding sources
