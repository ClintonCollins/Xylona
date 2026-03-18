# Xylona — Agent Guidelines

## Project Overview

Xylona is a game server control panel designed to be easily deployable and have an extremely low barrier to entry for users. It ships as a single binary with the frontend embedded via Go's `embed.FS`.

## Code Organization & Package Structure

### Backend (Go 1.26)

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
| `cmd` | Auxiliary CLI tools (e.g., `minecraft_version_hasher`, `e2e` orchestrator, `dummy_game_server`) |
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
- `pnpm --dir frontend run e2e` — run Playwright E2E tests (requires backend on `:8080`).
- `pnpm --dir frontend run e2e:headed` — run E2E tests in headed browser mode.
- `pnpm --dir frontend run e2e:ui` — open Playwright interactive UI.
- `pnpm --dir frontend run e2e:debug` — run E2E tests in debug mode.
- `pnpm --dir frontend run e2e:report` — open the last Playwright HTML report.

### E2E Testing
- `pnpm --dir frontend run e2e:federation` — run federation two-node E2E tests (self-contained).
- `pnpm --dir frontend run e2e:federation:headed` — run federation E2E tests in headed browser mode.
- `pnpm --dir frontend run e2e:federation:report` — open the last federation Playwright HTML report.

### Build & Codegen
- `mage Build` — frontend production build + Goreleaser snapshot build.
- `mage GenerateProto` — regenerate protobuf outputs.
- `mage GenerateModels` — regenerate bob ORM models.
- `mage SQLMigrateNew` — create new SQL migration.
- `mage SQLMigrateUp` — apply SQL migrations.
- `mage SQLMigrateDown` — roll back SQL migrations.

### E2E Testing (Mage)
- `mage E2E` — run single-node E2E tests (requires backend on `:8080`).
- `mage E2EHeaded` — run single-node E2E tests in headed browser mode.
- `mage E2EUI` — open Playwright interactive UI for single-node tests.
- `mage E2EReport` — open the last single-node Playwright HTML report.
- `mage E2EFederation` — run two-node federation E2E tests (fully self-contained).
- `mage E2EFederationHeaded` — run federation E2E tests in headed browser mode.
- `mage E2EFederationReport` — open the last federation Playwright HTML report.
- `mage E2ESeed <db_path> [username] [password]` — seed a fresh SQLite database with an admin user.

## Generated Code Rules

Never hand-edit generated outputs. Regenerate them via tooling.

Generated paths:

- `proto/go/xylona/**`
- `frontend/src/proto/**`
- `sql/models/*.bob.go`

Regeneration commands:

- `mage GenerateProto`
- `mage GenerateModels`

## Shell & Terminal Usage

- **Never use PowerShell unless absolutely necessary.** PowerShell introduces significant performance slowdowns. Always prefer `cmd.exe`, `bash`, or `sh` for running commands.
- If a task can be accomplished with `cmd.exe` or a Unix-style shell (e.g., Git Bash, WSL), use that instead of PowerShell.
- Only fall back to PowerShell when a specific Windows API or cmdlet has no reasonable alternative.

## Coding Conventions

- **Always** create and commit files with LF line endings (`\n`) for every file in this repository.
- Never introduce CRLF (`\r\n`) line endings in new files. If a touched file has CRLF, normalize it to LF as part of your change.

### Go Backend

- **Import grouping**: Three groups separated by blank lines — (1) standard library, (2) third-party, (3) internal (`github.com/ClintonCollins/Xylona/...`).
- **Error variable naming**: Use descriptive `err`-prefixed names like `errGenerate`, `errShutdown`, `errUpsertIP` rather than reusing a bare `err`.
- **Error handling**: Always handle returned errors. Do not ignore errors (including errors returned by deferred calls like `Close()`). At minimum, log them with context using `zerolog`.
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

### Accepted v-html Usage

The following `v-html` usages are intentional and should NOT be flagged as XSS concerns in audits or reviews:

- **`GameServerView.vue` console output** (`v-html="gameServerOutput"`): Game server console output is rendered as HTML to support ANSI color codes and formatting parsed by `parseConsole()`. The data originates from the user's own game server process and is only visible to authenticated users who already have server access. This is an accepted trust boundary.
- **`ClipBoardCopy.vue` tooltip** (`v-html="clipboardInnerHTML"`): Used for styled tooltip content. The data comes from component props set by the application, not external user input.

Do not add DOMPurify or replace these with text rendering unless the trust model changes (e.g., multi-tenant shared servers with untrusted operators).

## Search & Indexing Guardrails

- **Prefer `rg` (ripgrep) when available.** Use `rg` for text search and `rg --files` for file discovery instead of `grep`, `find`, `Get-ChildItem`, or IDE-specific search tools. `rg` automatically respects `.gitignore` rules and skips binary files, making it faster and safer for large repos.
  - Text search: `rg "pattern"` or `rg -i "pattern"` (case-insensitive).
  - File discovery: `rg --files` or `rg --files -g "*.go"` (glob filter).
  - Scoped search: `rg "pattern" path/to/dir`.
- If `rg` is not available, fall back to `grep -r` or equivalent tools.

When exploring or searching the repo, skip large/generated/vendor-like directories unless the task explicitly needs them:

- `frontend/node_modules`
- `frontend/.quasar`
- `frontend/dist`
- `cmd/minecraft_version_hasher/versions`
- `dist`

## Browser Automation

Use `agent-browser` for web automation. Run `agent-browser --help` for all commands.

Core workflow:

1. `agent-browser open <url>` - Navigate to page
2. `agent-browser snapshot -i` - Get interactive elements with refs (@e1, @e2)
3. `agent-browser click @e1` / `fill @e2 "text"` - Interact using refs
4. Re-snapshot after page changes

## Unit & Integration Testing

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

## E2E Testing

Xylona has two Playwright E2E test suites: **single-node** (standard) and **federation** (two-node).

### Single-Node Tests

**Prerequisites**: A running backend on `:8080` with a seeded database.

The `global-setup.ts` script delegates to the Go orchestrator (`cmd/e2e single-setup`) which creates test users, a game definition, a game server, test files, and RBAC role assignments. Tests run against `http://localhost:8080`.

| Test file | Coverage |
|---|---|
| `login.spec.ts` | Authentication flow (login/logout) |
| `smoke.spec.ts` | Basic page load and navigation |
| `admin.spec.ts` | Admin panel functionality |
| `permissions.spec.ts` | Role-based access control |
| `page-permissions.spec.ts` | Page-level permission checks |
| `console-errors.spec.ts` | Verifies no console errors on pages |
| `file-browser.spec.ts` | Game server file browser |
| `game-server-lifecycle.spec.ts` | Create, start, stop, delete game servers |

### Federation Tests

Fully self-contained — builds binaries, starts two Xylona nodes, pairs them, runs tests, and tears down.

The `federation-setup.ts` script delegates to the Go orchestrator (`cmd/e2e federation-setup`) which handles:
1. Building the frontend SPA and Go binaries (`xylona`, dummy game server)
2. Seeding separate databases for each node (directly, no separate binary needed)
3. Starting both nodes with distinct ports
4. Pairing the nodes via typed ConnectRPC clients
5. Creating test game servers, files, and users

| Port | Usage |
|---|---|
| `9081` | Node A HTTP backend |
| `9082` | Node B HTTP backend |
| `9444` | Node A federation mTLS |
| `9445` | Node B federation mTLS |

| Test file | Coverage |
|---|---|
| `federation-pairing.spec.ts` | Node pairing and federation setup |
| `federation-console.spec.ts` | Remote game server console via federation |
| `federation-console-errors.spec.ts` | No console errors on federation pages |
| `federation-file-browser.spec.ts` | Remote file browser via federation |
| `federation-permissions.spec.ts` | Cross-node permission checks |
| `federation-server-lifecycle.spec.ts` | Remote server start/stop/restart |

### Debugging

- **Headed mode**: Use `mage E2EHeaded` or `mage E2EFederationHeaded` to watch the browser.
- **Keep data**: Set `E2E_KEEP_DATA=1` when running federation tests to preserve node data directories after the run for inspection.
- **HTML reports**: Use `mage E2EReport` or `mage E2EFederationReport` to view detailed test reports.
- **Playwright UI**: Use `mage E2EUI` for interactive test debugging (single-node only).

### `cmd/e2e` Orchestrator

Go CLI tool that handles all E2E test setup and teardown. Uses typed ConnectRPC clients for API calls and direct database seeding (no separate binary needed).

```bash
go run ./cmd/e2e <subcommand> [flags]
```

Subcommands:
- `single-setup` — creates test users, game, server, files, and RBAC grants (requires running backend)
- `single-teardown` — deletes all test data created by single-setup
- `federation-setup` — builds binaries, seeds DBs, starts two nodes, pairs them, creates test data
- `federation-teardown` — API cleanup, kills node processes, removes data directories
- `seed` — bootstraps a fresh SQLite database with migrations and an admin user

Common flags:
- `--backend-url` (default: `http://localhost:8080`) — backend URL for single-node commands
- `--e2e-dir` (default: `frontend/e2e`) — E2E test directory for state files
- `--project-root` (default: `.`) — project root for building binaries

Seed flags:
- `--db` (required) — path to the SQLite database file
- `--username` (default: `admin`) — admin username
- `--password` (default: `admin`) — admin password
- `--migrations` (default: `sql/migrations`) — path to SQL migration files

### Infrastructure Files

| File | Purpose |
|---|---|
| `cmd/e2e/` | Go orchestrator — setup, teardown, seeding, process management |
| `frontend/e2e/global-setup.ts` | Thin wrapper that delegates to `cmd/e2e single-setup` |
| `frontend/e2e/global-teardown.ts` | Thin wrapper that delegates to `cmd/e2e single-teardown` |
| `frontend/e2e/helpers.ts` | Shared test utilities and API helpers (used by spec files) |
| `frontend/e2e/auth.setup.ts` | Authentication setup for single-node tests |
| `frontend/e2e/federation-setup.ts` | Thin wrapper that delegates to `cmd/e2e federation-setup` |
| `frontend/e2e/federation-teardown.ts` | Thin wrapper that delegates to `cmd/e2e federation-teardown` |
| `frontend/e2e/federation-helpers.ts` | Federation-specific API helpers (used by spec files) |
| `frontend/e2e/federation-auth.setup.ts` | Authentication setup for federation tests |
| `frontend/playwright-federation.config.ts` | Playwright config for federation tests |

### Gitignored Artifacts

- `frontend/e2e/.federation/` — federation node data directories
- `frontend/e2e/playwright-report/` — single-node HTML report
- `frontend/e2e/playwright-report-federation/` — federation HTML report
- `frontend/e2e/test-results/` — test result artifacts

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

## Design Context

### Users

Xylona serves a broad range of users — from individual gamers self-hosting a Minecraft server for friends, to gaming community admins managing servers for dozens of players, to small hosting providers running multiple nodes. The common thread: they want to manage game servers without friction. The UI must be immediately understandable for first-timers while giving power users efficient workflows. Users are often checking server status, browsing files, reading console output, or adjusting configuration — tasks that benefit from information density without clutter.

### Brand Personality

**Powerful, sleek, futuristic.** Xylona should feel like a high-tech command center — confident, in-control, and visually distinct. The existing brand fonts (Zen Dots for brand, Goldman for display, Exo 2 for body) already establish a sci-fi / cyberpunk aesthetic. Lean into this identity rather than softening it.

### Aesthetic Direction

- **Visual tone**: Dark, immersive, technical. Layered dark surfaces with subtle depth. Cyan and blue accents that pop against near-black backgrounds. The UI should feel like piloting a spacecraft, not filling out a spreadsheet.
- **References**: Draw layout and UX patterns from modern SaaS dashboards (Vercel, Linear) for their polish and spatial clarity. Reference Discord and Steam for gaming-audience familiarity. Study Pterodactyl/Pelican for domain-specific patterns (console, file browser, server controls) — but differentiate through unique layout and styling choices.
- **Anti-references**: Avoid generic admin panel templates. Avoid cluttered cPanel-style density. Avoid flat, lifeless layouts with no personality.
- **Theme**: Dark mode only. The existing surface hierarchy (--xy-base through --xy-surface-4) provides excellent depth layering — use it consistently.
- **Typography**: Four-font system already in place. Zen Dots (brand moments), Goldman (headings, buttons, nav), Exo 2 (body text), JetBrains Mono (code, console, technical data). Respect the hierarchy — don't overuse brand/display fonts.

### Design Principles

1. **Command & control**: Every screen should give users a clear sense of what's happening and what they can do about it. Status, actions, and feedback should be immediately visible — not hidden behind menus.
2. **Layered depth**: Use the surface hierarchy to create visual depth. Cards float above pages, dialogs float above cards. Consistent elevation communicates structure without borders everywhere.
3. **Purposeful contrast**: Reserve high-contrast accents (cyan, blue) for interactive elements and status indicators. Let the dark surfaces breathe. White space (dark space) is a feature, not wasted space.
4. **Gaming-native but professional**: The aesthetic should feel at home next to Discord and Steam, but the UX should be as polished as Vercel or Linear. Fun and capable, never toy-like or chaotic.
5. **Information when needed**: Show essential info at a glance (server status, resource usage, player counts). Defer complexity to detail views. Progressive disclosure over information overload.

### Existing Design System

The codebase has a well-structured 3-layer token system:

- **Layer 1** (SCSS): `quasar.variables.scss` — Quasar component theming (primary, secondary, accent, spacing, typography scale)
- **Layer 2** (CSS): `design-tokens.css` — Custom properties for surfaces, semantic colors, borders, shadows, fonts, transitions
- **Layer 3** (CSS): `overrides.css` — Quasar component-level style overrides using Layer 1 and 2 tokens

When adding new styles, extend Layers 2-3. Never hardcode colors — always reference tokens. New utility classes go in `design-tokens.css`. Component overrides go in `overrides.css`.

### Color Palette

| Role | Value | Usage |
|------|-------|-------|
| Primary | `#3B82F6` | Primary actions, links, focus states |
| Secondary | `#6366F1` | Secondary actions, accents |
| Accent | `#1CB7CF` | Brand highlights, active nav, toolbar title |
| Success | `#22C55E` | Running servers, successful operations |
| Danger | `#EF4444` | Errors, destructive actions, stopped servers |
| Warning | `#F59E0B` | Alerts, caution states |
| Info | `#06B6D4` | Informational messages |
| Base | `#0D0E0F` | Page background (cool-tinted) |
| Surface 0-4 | `#141516` - `#383B3D` | Layered UI surfaces (cool-tinted) |
| Text Primary | `#E0E4E6` | Main content text |
| Text Secondary | `#979B9E` | Supporting text |
| Text Muted | `#858A8C` | Disabled/tertiary text |

### Accessibility

Best-effort approach: follow good contrast and keyboard navigation practices, but don't let strict WCAG compliance block design decisions that serve the gaming audience. Prioritize readability and discoverability over formal audit scores.
