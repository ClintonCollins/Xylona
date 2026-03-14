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

### Backend
- `go test ./...` — run backend tests.
- `go test -short ./...` — run fast unit-focused backend tests.
- `go test ./... -race -count=1` — run backend tests with the race detector and disable test result caching.
- `go build -o xylona` — build backend binary locally.

### Frontend
- `pnpm --dir frontend run dev` — run frontend dev server.
- `pnpm --dir frontend run build` — build frontend SPA.
- `pnpm --dir frontend run lint` — run ESLint on frontend code.
- `pnpm --dir frontend run lint:fix` — run ESLint with auto-fix.
- `pnpm --dir frontend run format` — run Prettier to format frontend code.
- `pnpm --dir frontend run format:check` — check Prettier formatting without changes.
- `pnpm --dir frontend run test` — run Vitest unit tests.
- `pnpm --dir frontend run test:watch` — run Vitest in watch mode.
- `pnpm --dir frontend run test:coverage` — run Vitest with coverage reporting.

### Build & Codegen
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
- **Linting**: ESLint configured for Vue 3 + TypeScript; run `pnpm --dir frontend run lint` before committing.
- **Formatting**: Prettier configured for consistent code style; run `pnpm --dir frontend run format` before committing.
- **Testing**: Vitest for unit and component tests; Vue Test Utils for component testing.

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
- **Frontend**: Vitest configured with Vue Test Utils for unit and component testing. ESLint and Prettier are configured for code quality and formatting.

### Testing Conventions (Backend)

- **Table-driven tests**: Prefer table-driven tests for logic with multiple input/output scenarios, using `[]struct` + `t.Run()`.
- **Standard library only**: Use the `testing` package (no external assertion frameworks unless explicitly requested).
- **Assertion behavior**: Use `t.Fatalf()` for setup/precondition failures and `t.Errorf()` for additional case-level mismatches.
- **Test file placement**: Keep tests adjacent to the code they exercise. Use same-package tests by default; add external-package tests (`package foo_test`) when validating public API behavior.
- **Test naming**: Functions follow `TestXxx` convention; use descriptive case names focused on behavior (e.g., `"returns err on invalid version"`).

### Writing New Tests

- Place test files next to the source file they test, using the `_test.go` suffix.
- Use table-driven tests for functions with multiple scenarios.
- Use the standard `testing` package; avoid introducing external test frameworks unless explicitly requested.
- Cover both positive and negative cases, including invalid/edge-case inputs.
- Keep tests deterministic: avoid real network calls, wall-clock dependencies, random seeds without control, and fixed `time.Sleep` waits.
- Use `t.TempDir()`, `t.Setenv()`, and `t.Cleanup()` for isolation and cleanup.
- Prefer `errors.Is` / `errors.As` over brittle string matching for error assertions.
- For bug fixes, add or update a regression test that fails before the fix and passes after it.

### Testing Requirements for New Code

**When generating new or changed Go logic, always include corresponding tests unless explicitly told not to.**

Required test coverage for new code:

- **New/changed functions and methods**: Add tests covering:
  - Happy path (valid inputs, expected outputs)
  - Edge cases (empty inputs, boundary values, nil pointers)
  - Error cases (invalid inputs, expected failures)
- **Behavior changes**: Update existing tests to reflect new behavior, not just new code paths.
- **New packages**: Create at least one `*_test.go` file covering exported behavior and key internal logic.
- **Business logic**: Prioritize testing functions in `actions/`, `api/rpc/`, `helpers/`, and `pkg/` packages.
- **Database methods**: Test CRUD operations with in-memory SQLite when adding new `db/` methods, with each test isolated from shared DB state.
- **Concurrency-sensitive changes**: Add tests that exercise concurrent access and run with `-race`.
- **Integration tests**: If a test depends on filesystem/DB/process interactions, mark and structure it so it can be skipped in short runs (`testing.Short()`).

Test file structure:

```go
package mypackage

import "testing"

func TestMyFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {name: "valid input", input: ..., want: ..., wantErr: false},
        {name: "invalid input", input: ..., want: ..., wantErr: true},
        {name: "edge case", input: ..., want: ..., wantErr: false},
    }

    for _, tt := range tests {
        tt := tt
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("MyFunction() error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("MyFunction() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

**Exceptions**: Skip tests for:
- Trivial getters/setters
- Generated code (protobuf, ORM models)
- Main package entry points
- Simple wrapper functions with no logic

### Frontend Testing

Frontend uses Vitest + Vue Test Utils for unit and component testing.

#### Testing Requirements for New Frontend Code

**When generating new or changed frontend logic, always include corresponding tests for complex functionality unless explicitly told not to.**

Required test coverage for new frontend code:

- **Complex utilities and composables**: Add tests covering:
  - Happy path (valid inputs, expected outputs)
  - Edge cases (empty inputs, boundary values, null/undefined handling)
  - Error cases (invalid inputs, API failures, expected exceptions)
- **Stateful components**: Test components with significant business logic, data transformations, or conditional rendering
- **Pinia stores**: Test store actions, getters, and state mutations covering:
  - Successful API interactions
  - Error handling and fallback states
  - State consistency across actions
- **Critical user flows**: Test key interactions (form validation, authentication flows, data submission)
- **API client logic**: Test RPC client wrappers and data transformation utilities

**Exceptions**: Skip tests for:
- Simple presentational components with minimal logic
- Components that are purely layout/styling wrappers
- Generated protobuf types
- Simple getter/setter composables
- Trivial utility functions with no branching logic

#### Frontend Test File Structure

Place test files adjacent to the source file using `.test.ts` or `.spec.ts` suffix:

```typescript
// utils/myUtil.test.ts
import { describe, it, expect } from 'vitest'
import { myFunction } from './myUtil'

describe('myFunction', () => {
  it('should handle valid input', () => {
    const result = myFunction('valid')
    expect(result).toBe('expected')
  })

  it('should handle edge cases', () => {
    const result = myFunction('')
    expect(result).toBe('')
  })

  it('should throw on invalid input', () => {
    expect(() => myFunction(null)).toThrow()
  })
})
```

For Vue components:

```typescript
// components/MyComponent.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import MyComponent from './MyComponent.vue'

describe('MyComponent', () => {
  it('renders properly', () => {
    const wrapper = mount(MyComponent, { props: { msg: 'Hello' } })
    expect(wrapper.text()).toContain('Hello')
  })

  it('handles user interaction', async () => {
    const wrapper = mount(MyComponent)
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted()).toHaveProperty('submit')
  })
})
```

#### Code Quality Workflow

Before completing work on frontend code:

1. **Run linter**: `pnpm --dir frontend run lint` — fix any ESLint errors
2. **Format code**: `pnpm --dir frontend run format` — apply Prettier formatting
3. **Run tests**: `pnpm --dir frontend run test` — ensure all tests pass
4. **Check build**: `pnpm --dir frontend run build` — verify production build succeeds

For complex changes, run tests with coverage to ensure adequate coverage:
```bash
pnpm --dir frontend run test:coverage
```

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
