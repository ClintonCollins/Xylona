# Xylona — Agent Guidelines

## Project Overview

Xylona is a game server control panel designed to be easily deployable and have an extremely low barrier to entry for users. It ships as a single binary with the frontend embedded via Go's `embed.FS`.

## Code Organization & Package Structure

### Backend (Go 1.23)

The backend is the project root and uses the Go module path `github.com/ClintonCollins/Xylona`.

| Package | Purpose |
|---|---|
| `main` (root) | Application entry point, HTTP server setup, router configuration, graceful shutdown |
| `actions` | Business logic layer (background tasks, file operations, post-install actions) |
| `api/rpc` | ConnectRPC service handlers (auth, game servers, games, IPs, nodes, users) |
| `api/websocket` | WebSocket handling via the Melody library |
| `api/gatekeeper` | Authentication/authorization middleware |
| `api/xylona-internal` | Internal game-specific logic (e.g., Minecraft) |
| `db` | Database connection and per-entity query methods (SQLite via `modernc.org/sqlite`, bob ORM) |
| `helpers` | Utility functions and model↔proto conversion/assertion helpers |
| `gsutils` | Game server utilities |
| `pkg/eventbus` | Internal event bus |
| `pkg/minecraft` | Minecraft-specific utilities |
| `pkg/query` | Game server query protocol support |
| `pkg/xycrypt` | Cryptographic helpers (Argon2id hashing) |
| `proto` | Protobuf definitions (`.proto` files) and generated Go/TypeScript code |
| `sql/migrations` | SQL migration files (used with `sql-migrate`) |
| `sql/models` | Generated ORM models (bob) |
| `supervisor` | Game server process lifecycle management (start, stop, I/O, listeners) |
| `magefiles` | Mage build tasks |
| `cmd` | Auxiliary CLI tools (e.g., `minecraft_version_hasher`) |
| `embed.go` | Embeds the built frontend SPA into the Go binary |

### Frontend (`/frontend`) — Vite + TypeScript + Vue 3 + Quasar 2

The frontend is a Quasar 2 SPA managed with **pnpm** and built with **Vite** via `@quasar/app-vite`.

| Directory | Purpose |
|---|---|
| `src/pages` | Route-level Vue components organized by domain (`game_servers/`, `games/`, `nodes/`, `admin/`, `other/`) |
| `src/components` | Reusable Vue components organized by domain (`game_servers/`, `games/`, `keys/`, `nodes/`, `editor/`) |
| `src/stores` | Pinia state stores (`xylona.ts` for auth state, toolbar nav state) |
| `src/router` | Vue Router configuration |
| `src/layouts` | Quasar layout components |
| `src/boot` | Quasar boot files |
| `src/utils` | Shared utility functions (e.g., ConnectRPC client setup) |
| `src/proto` | Generated TypeScript protobuf types from `@bufbuild/protobuf` |
| `src/css` | Global stylesheets |
| `src/assets` | Static assets (images, backgrounds) |

## Canonical Commands

Use these as the default commands for this repo:

- `go test ./...` — run backend tests.
- `go build -o xylona` — build backend binary locally.
- `pnpm --dir frontend run dev` — run frontend dev server.
- `pnpm --dir frontend run build` — build frontend SPA.
- `mage Build` — frontend production build + Goreleaser snapshot build.
- `mage GenerateProto` — regenerate protobuf outputs.
- `mage GenerateModels` — regenerate bob ORM models.
- `mage SQLMigrateUp` — apply SQL migrations.
- `mage SQLMigrateDown` — roll back SQL migrations.

## Generated Code Rules

Never hand-edit generated outputs. Regenerate them via tooling.

Generated paths:

- `proto/go/xylona/**`
- `frontend/src/proto/**`
- `sql/models/*.bob.go`

Regeneration commands:

- `mage GenerateProto`
- `mage GenerateModels`

## Coding Conventions

- **Always** create and commit files with LF line endings (`\n`).

### Go Backend

- **Import grouping**: Three groups separated by blank lines — (1) standard library, (2) third-party, (3) internal (`github.com/ClintonCollins/Xylona/...`).
- **Error variable naming**: Use descriptive `err`-prefixed names like `errGenerate`, `errShutdown`, `errUpsertIP` rather than reusing a bare `err`.
- **Struct fields**: Unexported (lowercase) by default; exported only when needed for serialization or external access.
- **Logging**: Use `zerolog` (`github.com/rs/zerolog/log`) with structured fields (`.Str()`, `.Err()`, `.Bool()`, `.Msg()`). Use `log.Fatal()` for unrecoverable startup errors.
- **Naming**: Standard Go conventions — PascalCase for exported identifiers, camelCase for unexported. Package names are short, lowercase, single-word.
- **Error sentinel values**: Defined as package-level `var` blocks (e.g., `var ErrCommandDoesNotExist = errors.New(...)`).
- **Constructor pattern**: `New()` or `NewXxx()` functions return struct pointers.
- **Concurrency**: `sync.RWMutex` for protecting shared state; `context.Context` threaded through for cancellation.
- **Router**: `go-chi/chi/v5` with middleware chain. SPA fallback handler redirects unknown paths to `index.html`.
- **API layer**: ConnectRPC (Connect protocol) for the RPC API; protobuf schemas in `/proto`.
- **Database**: SQLite with `modernc.org/sqlite` (pure Go driver), `bob` as the query builder/ORM, `sql-migrate` for migrations. DB methods are receiver methods on `*db.Connection`.
- **Configuration**: Environment variables loaded via `godotenv` and parsed with `caarlos0/env`.
- **File naming**: Use kebab-case for new multi-word files (e.g., `game-server.go`, `local-secret-keys.go`). Do not rename existing files solely for style unless explicitly requested.

### TypeScript Frontend

- **Module system**: ESM (`"type": "module"` in `package.json`).
- **State management**: Pinia stores with typed state interfaces.
- **API communication**: ConnectRPC with `@connectrpc/connect-web` and `@bufbuild/protobuf` for type-safe RPC calls.
- **Component naming**: PascalCase `.vue` files (e.g., `GameServerView.vue`, `StatusBadge.vue`).
- **Protobuf types**: Generated from shared `.proto` definitions using `@bufbuild/protoc-gen-es` via `buf`.
- **Package manager**: pnpm (v9) with lockfile (`pnpm-lock.yaml`).
- **Code editor**: Monaco Editor integration for in-browser file editing.

## Search & Indexing Guardrails

When exploring or searching the repo, skip large/generated/vendor-like directories unless the task explicitly needs them:

- `frontend/node_modules`
- `frontend/.quasar`
- `frontend/dist`
- `cmd/minecraft_version_hasher/versions`
- `dist`

## Unit & Integration Testing

### Current State

- **Backend**: Minimal test coverage. One test file exists at `pkg/xycrypt/xycrypt_test.go`.
- **Frontend**: No tests configured (the `test` script in `package.json` is a no-op).

### Testing Conventions (Backend)

- **Table-driven tests**: The existing test uses the standard Go table-driven pattern with `[]struct` test cases and `t.Run()` sub-tests.
- **Standard library only**: Tests use `testing.T` with `t.Errorf()` for assertions — no external assertion libraries.
- **Test file placement**: Tests live alongside the code they test in the same package (e.g., `xycrypt_test.go` in `package xycrypt`).
- **Test naming**: Functions follow `TestXxx` convention; table entries use descriptive lowercase `name` fields (e.g., `"match 1"`, `"invalid version"`).

### Writing New Tests

- Place test files next to the source file they test, using the `_test.go` suffix.
- Use table-driven tests for functions with multiple input/output scenarios.
- Use the standard `testing` package; avoid introducing external test frameworks unless explicitly requested.
- Cover both positive and negative cases, including invalid/edge-case inputs.

## Key Dependencies

### Backend
- **HTTP Router**: `go-chi/chi/v5`
- **RPC**: `connectrpc.com/connect` (Connect protocol over HTTP)
- **Logging**: `rs/zerolog`
- **Database**: `modernc.org/sqlite` + `stephenafamo/bob` ORM + `rubenv/sql-migrate`
- **Auth**: `golang-jwt/jwt/v5`, `gorilla/securecookie`
- **WebSocket**: `olahol/melody`
- **Protobuf**: `google.golang.org/protobuf`
- **Build**: Mage (`magefiles/`)

### Frontend
- **Framework**: Vue 3 + Quasar 2
- **Build**: Vite via `@quasar/app-vite`
- **State**: Pinia
- **RPC**: `@connectrpc/connect-web` + `@bufbuild/protobuf`
- **Editor**: Monaco Editor
