# Xylona — Agent Guidelines

## Project Snapshot

Xylona is a game server control panel built to be easy to self-host and ship as a single binary. The Go backend embeds the Quasar/Vue frontend via `embed.FS`.

Core stack:
- Backend: Go 1.26, `chi`, ConnectRPC, SQLite (`modernc.org/sqlite`), bob ORM, `sql-migrate`, `zerolog`
- Frontend: Vue 3, Quasar 2, Vite, TypeScript, Pinia, ConnectRPC, Monaco Editor
- Tooling: Mage, pnpm, Playwright, Vitest

## Repo Map

- Backend entry and HTTP/API surface: `main`, `embed.go`, `api/rpc`, `api/gatekeeper`, `api/websocket`, `api/xylona-internal`
- Core backend logic: `actions`, `db`, `helpers`, `gsutils`, `supervisor`, `steamcache`
- Shared/domain packages: `cfgparse`, `cfgschema`, `pkg/eventbus`, `pkg/minecraft`, `pkg/query`, `pkg/xycrypt`, `pkg/modmanager`, `pkg/modproviders`, `pkg/sysinfo`, `pkg/version`
- Generated and build assets: `proto`, `sql/migrations`, `sql/models`, `magefiles`, `cmd`
- Frontend app: `frontend/src/pages`, `components`, `stores`, `router`, `layouts`, `boot`, `utils`, `proto`, `css`, `assets`

## Default Commands

Backend:
- `go test -race -count=1 ./...`
- `go test -short ./...`
- `go test -tags=integration ./...`
- `go build -o xylona`
- `golangci-lint run ./...`
- `mage Lint`
- `mage LintFix`

Frontend:
- `pnpm --dir frontend run dev`
- `pnpm --dir frontend run build`
- `pnpm --dir frontend run lint`
- `pnpm --dir frontend run format`
- `pnpm --dir frontend run test`
- `pnpm --dir frontend run test:coverage`
- `pnpm --dir frontend run e2e`
- `pnpm --dir frontend run e2e:federation`

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

Regenerate with:
- `mage GenerateProto`
- `mage GenerateModels`

## Working Rules

- Prefer `cmd.exe`, `bash`, or `sh` over PowerShell.
- Use LF line endings. If a touched file is CRLF, normalize it to LF.
- Skip these directories when searching unless the task needs them: `frontend/node_modules`, `frontend/.quasar`, `frontend/dist`, `cmd/minecraft_version_hasher/versions`, `dist`
- For local browser verification, you may read `XYLONA_ADMIN_USERNAME` and `XYLONA_ADMIN_PASSWORD` from `.env`; never print, log, or commit them.

## Go Conventions

- Use three import groups: stdlib, third-party, internal `github.com/ClintonCollins/Xylona/...`
- Name errors descriptively, for example `errGenerate`, `errShutdown`, `errUpsertIP`
- Handle every returned error, including deferred closes; log with `zerolog` when appropriate
- Keep struct fields unexported unless external access or serialization requires export
- Use structured `zerolog` logging and `log.Fatal()` for unrecoverable startup failures
- Follow standard Go naming; define sentinel errors in package-level `var` blocks
- Constructors should be `New()` or `NewXxx()` and return pointers
- Thread `context.Context` through cancellable work and use `sync.RWMutex` for shared mutable state when needed
- Router uses `chi`; unknown SPA routes should fall back to `index.html`
- Database access uses SQLite plus bob; DB methods live on `*db.Connection`
- New multi-word Go filenames should be kebab-case

## Frontend Conventions

- TypeScript uses ESM, pnpm, Vue 3, Quasar 2, Pinia, ConnectRPC, and Monaco
- Use PascalCase `.vue` filenames
- Generated protobuf types come from shared `.proto` files via `buf` and `protoc-gen-es`
- Before finishing frontend changes, run `pnpm --dir frontend run lint`, `format`, `test`, and `build` as appropriate

## Accepted `v-html`

These usages are intentional and should not be flagged as XSS issues unless the trust model changes:
- `frontend/src/pages/game_servers/GameServerView.vue`: console output HTML from the user's own game server, formatted by `parseConsole()`
- `frontend/src/components/shared/ClipBoardCopy.vue`: styled tooltip HTML from application-controlled props

Do not add DOMPurify or replace these with plain text unless the trust boundary changes.

## Testing

- Add or update tests for new or changed logic with real regression risk; skip trivial config, wiring, and pure presentation work unless requested
- Backend tests should be table-driven where useful, use the standard `testing` package, live beside the code, be deterministic, and prefer `errors.Is` or `errors.As`
- Use `t.TempDir()`, `t.Setenv()`, and `t.Cleanup()` for isolation; use in-memory SQLite for `db` tests
- Heavier filesystem, DB, or process tests should be skippable in `testing.Short()`
- Bug fixes should add a regression test when practical; concurrency-sensitive changes should be exercised under `-race`
- Frontend tests should focus on utilities, composables, Pinia stores, RPC wrappers, significant stateful components, and critical user flows
- Purely visual frontend changes usually need lint, format, build, and manual verification rather than automated tests

## E2E

Two self-contained Playwright suites exist, both driven by `cmd/e2e`.

Single-node:
- `cmd/e2e single-setup` builds binaries, seeds a fresh DB, starts backend on `:9091`, federation on `:9446`, and runs behind Vite on `:9002`
- Coverage areas: login, smoke, admin, permissions, page permissions, console errors, file browser, server lifecycle, config schema ordering, mods
- Entry files: `frontend/e2e/global-setup.ts`, `frontend/e2e/global-teardown.ts`, `frontend/e2e/helpers.ts`, `frontend/e2e/auth.setup.ts`

Federation:
- `cmd/e2e federation-setup` builds binaries, seeds two DBs, starts nodes on `:9081/:9444` and `:9082/:9445`, pairs them, and creates test data
- Coverage areas: pairing, console, console errors, file browser, permissions, server lifecycle
- Entry files: `frontend/e2e/federation-setup.ts`, `frontend/e2e/federation-teardown.ts`, `frontend/e2e/federation-helpers.ts`, `frontend/e2e/federation-auth.setup.ts`, `frontend/playwright-federation.config.ts`

Useful commands:
- `mage E2E`
- `mage E2EHeaded`
- `mage E2EUI`
- `mage E2EReport`
- `mage E2EFederation`
- `mage E2EFederationHeaded`
- `mage E2EFederationReport`
- Set `E2E_KEEP_DATA=1` to keep federation data for debugging

Artifacts:
- `frontend/e2e/.e2e-data/`
- `frontend/e2e/.e2e-*.lock`
- `frontend/e2e/.federation/`
- `frontend/e2e/playwright-report/`
- `frontend/e2e/playwright-report-federation/`
- `frontend/e2e/test-results/`

Orchestrator reference:
- Subcommands: `single-setup`, `single-teardown`, `federation-setup`, `federation-teardown`, `seed`
- Common flags: `--http-port`, `--fed-port`, `--e2e-dir`, `--project-root`
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
