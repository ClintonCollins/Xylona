# Xylona — Agent Guidelines

## Project Snapshot

Xylona is a game server control panel built to be easy to self-host and ship as a single binary. The Go backend embeds the Quasar/Vue frontend via `embed.FS`.

Core stack:
- Backend: Go 1.26, `chi`, ConnectRPC, SQLite (`modernc.org/sqlite`), bob ORM, `sql-migrate`, `zerolog`
- Frontend: Vue 3, Quasar 2, Vite, TypeScript, Pinia, ConnectRPC, Monaco Editor
- Tooling: Mage, Bun, Playwright, Vitest

## Repo Map

- Backend entry and embedded assets: `cmd/xylona`, `internal/webui`, `sql/migrations`
- Controller logic and API surface: `internal/controller/actions`, `internal/controller/api/{events,gatekeeper,rpc,websocket}`, `internal/gameintegrations`
- Node runtime boundary: `cmd/xylona-node`, `internal/node`, `internal/nodeclient`, `internal/noderegistry`, `internal/nodetls`, `internal/node/supervisor`
- App services: `internal/db`, `internal/modmanager`, `internal/scheduler`, `internal/alerts`, `internal/usermgmt`, `internal/mailer`, `internal/webhooks`, `internal/selfupdate`, `internal/updater`, `internal/steamcache`
- Shared/domain packages: `pkg/cfgparse`, `pkg/cfgschema`, `pkg/gsutils`, `pkg/helpers`, `pkg/minecraft`, `pkg/query`, `pkg/xycrypt`, `pkg/modproviders`, `pkg/updateproviders`, `pkg/passwordhash`, `pkg/version`
- Generated and build assets: `proto`, `sql/migrations`, `sql/models`, `magefiles`, `cmd`
- Frontend app: `frontend/src/pages`, `components`, `stores`, `router`, `layouts`, `boot`, `utils`, `proto`, `css`, `assets`

## Project Commands

Backend:
- `go test -race -count=1 ./...`
- `go test -short ./...`
- `go test -tags=integration ./...`
- `go build -o xylona ./cmd/xylona`
- `golangci-lint run ./...`
- `mage Lint`
- `mage LintFix`

Frontend:
- from `frontend/`: `bun run dev`
- from `frontend/`: `bun run build`
- from `frontend/`: `bun run lint`
- from `frontend/`: `bun run format`
- from `frontend/`: `bun run format:check`
- from `frontend/`: `bun run test`
- from `frontend/`: `bun run test:coverage`
- from `frontend/`: `bun run e2e`

Build and codegen:
- `mage Build`
- `mage GenerateProto`
- `mage GenerateModels`
- `mage SQLMigrateNew`
- `mage SQLMigrateUp`
- `mage SQLMigrateDown`

## Generated Code

Never hand-edit generated outputs:
- `proto/go/xylona/**`
- `frontend/src/proto/**`
- `sql/models/*.bob.go`
- `sql/models/dberrors/*.bob.go`
- `sql/models/dbinfo/*.bob.go`
- `sql/models/enums/*.bob.go`

Regenerate with:
- `mage GenerateProto`
- `mage GenerateModels`

## Local Rules

- Use LF line endings. If a touched file is CRLF, normalize it to LF.
- Skip these directories when searching unless the task needs them: `frontend/node_modules`, `frontend/.quasar`, `internal/webui/dist`, `cmd/minecraft_version_hasher/versions`, `dist`
- `/docs/` is intentionally ignored as local scratch space. Do not put durable project documentation there unless the ignore policy changes first.
- For local browser verification, you may read `XYLONA_ADMIN_USERNAME` and `XYLONA_ADMIN_PASSWORD` from `.env`; never print, log, or commit them.

## Go Conventions

- Use three import groups: stdlib, third-party, internal `github.com/ClintonCollins/Xylona/...`
- Name errors descriptively, for example `errGenerate`, `errShutdown`, `errUpsertIP`
- Handle every returned error, including deferred closes; log with `zerolog` when appropriate
- Do not silently discard errors with `_ = ...`. If an error truly cannot be returned to the caller, handle it locally with an explicit comment and at minimum log it.
- Deferred cleanup errors must not be ignored. Return them when possible; otherwise log them with useful context.
- Keep struct fields unexported unless external access or serialization requires export
- Use structured `zerolog` logging and `log.Fatal()` for unrecoverable startup failures
- Follow standard Go naming; define sentinel errors in package-level `var` blocks
- Constructors should be `New()` or `NewXxx()` and return pointers
- Thread `context.Context` through cancellable work and use `sync.RWMutex` for shared mutable state when needed
- Router uses `chi`; unknown SPA routes should fall back to `index.html`
- Database access uses SQLite plus bob; DB methods live on `*db.Connection`
- New multi-word Go filenames should be kebab-case

## Frontend Conventions

- TypeScript uses ESM, Bun, Vue 3, Quasar 2, Pinia, ConnectRPC, and Monaco
- Use PascalCase `.vue` filenames
- Generated protobuf types come from shared `.proto` files via `buf` and `protoc-gen-es`
- Before finishing frontend changes, run `bun run lint`, `bun run format`, `bun run test`, and `bun run build` from `frontend/` as appropriate

## Accepted `v-html`

These usages are intentional and should not be flagged as XSS issues unless the trust model changes:
- `frontend/src/pages/game_servers/GameServerView.vue`: console output HTML from the user's own game server, formatted by `parseConsole()`
- `frontend/src/components/shared/ClipBoardCopy.vue`: styled tooltip HTML from application-controlled props

Do not add DOMPurify or replace these with plain text unless the trust boundary changes.

## Testing

- Backend tests should be table-driven where useful, use the standard `testing` package, live beside the code, be deterministic, and prefer `errors.Is` or `errors.As`
- Use `t.TempDir()`, `t.Setenv()`, and `t.Cleanup()` for isolation; use in-memory SQLite for `internal/db` tests
- Heavier filesystem, DB, or process tests should be skippable in `testing.Short()`
- Frontend tests should focus on utilities, composables, Pinia stores, RPC wrappers, significant stateful components, and critical user flows
- Purely visual frontend changes usually need lint, format, build, and manual verification rather than automated tests

## E2E And Integration

The Playwright suite is workflow-focused and driven by `cmd/e2e`.

Local controller:
- `cmd/e2e setup --mode local-controller` builds binaries, seeds a fresh DB, starts the controller on `:9091`, and runs behind Vite on `:9002`
- Coverage areas: login, smoke navigation, game server lifecycle, console, files, backups, RBAC, game definitions/start args, notifications
- Entry files: `frontend/e2e/global-setup.ts`, `frontend/e2e/global-teardown.ts`, `frontend/e2e/api.ts`, `frontend/e2e/auth.ts`, `frontend/e2e/fixtures.ts`, `frontend/e2e/pages.ts`, `frontend/e2e/auth.setup.ts`

Remote node:
- `cmd/e2e setup --mode remote-node` builds controller and node binaries, seeds a fresh DB, starts the controller plus one remote `xylona-node`, pairs the node, and creates a server assigned to that node
- `mage E2ERemoteNode` runs the remote-node browser smoke suite
- `mage IntegrationLocal` runs the Go process integration test with controller restart/redial coverage

Useful commands:
- `mage E2ESmoke`
- `mage E2E`
- `mage E2ERemoteNode`
- `mage E2EHeaded`
- `mage E2EUI`
- `mage E2EReport`
- `mage IntegrationLocal`
- `mage IntegrationLive`

Artifacts:
- `frontend/e2e/.e2e-data/`
- `frontend/e2e/.e2e-*.lock`
- `frontend/e2e/playwright-report/`
- `frontend/e2e/test-results/`

Orchestrator reference:
- Subcommands: `setup`, `teardown`, `status`, `seed`
- Common flags: `--mode`, `--http-port`, `--node-port`, `--e2e-dir`, `--project-root`
- Seed flags: `--db`, `--username`, `--password`, `--migrations`

## Design Context

Audience:
- Self-hosters, gaming community admins, and small hosting providers
- The UI should be immediately understandable to first-timers while staying efficient for power users

Visual direction:
- Powerful, sleek, futuristic, and dark-only
- Favor layered dark surfaces, cyan and blue accents, and a high-tech command-center feel
- Reference Vercel and Linear for polish, Discord and Steam for audience familiarity, and Pterodactyl or Pelican for domain patterns
- Avoid generic admin templates, cPanel-style clutter, and flat lifeless layouts
- Typography hierarchy: Zen Dots for brand, Goldman for headings and controls, Exo 2 for body, JetBrains Mono for technical text

Design principles:
- Command and control: status, actions, and feedback should be obvious
- Layered depth: use the surface hierarchy consistently
- Purposeful contrast: reserve bright accents for interactive or stateful elements
- Gaming-native but professional: distinctive, capable, never chaotic
- Progressive disclosure: show essentials first, defer complexity to detail views

Design system:
- Extend `frontend/src/css/design-tokens.css` for tokens and utilities
- Extend `frontend/src/css/overrides.css` for component overrides
- Do not hardcode colors
- Theme colors: primary `#3B82F6`, secondary `#6366F1`, accent `#1CB7CF`, success `#22C55E`, danger `#EF4444`, warning `#F59E0B`, info `#06B6D4`, base `#0D0E0F`, surfaces `#141516` through `#383B3D`, text `#E0E4E6`, `#979B9E`, `#858A8C`

Accessibility:
- Follow good contrast and keyboard-navigation practices, but optimize for readability and discoverability over rigid formal WCAG scoring
