# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

**Primary: self-hosting operators.** Individual gamers running servers for friends, gaming community admins managing servers for many players, and small hosting providers operating multiple nodes. They are usually checking server state, reading console output, browsing files, managing backups, adjusting configuration, or controlling server lifecycle.

The interface should be immediately understandable for a first-time self-hoster while staying fast and information-rich for power users who repeat the same operational tasks every day.

**Secondary, explicitly lower priority: shared-map visitors.** Unauthenticated people who open a shared live-map link (`/shared/palworld-map`, `/shared/7-days-to-die-map`, `/shared/minecraft-map`). They are viewers, not operators: they arrive from a link an admin handed out, they have no account, and they cannot act on the server. This audience is real and should not be designed as though it were an admin, but it must not pull design attention away from the operator workflows that matter.

## Product Purpose

Xylona is a game server control panel built to be easy to self-host and ship as a single binary. The Go backend embeds a Quasar and Vue frontend, so the product should feel cohesive, dependable, and local-first rather than like a loose admin template.

Success means an admin can understand what is happening, take the next action with confidence, and recover from routine issues without hunting through hidden menus or noisy screens.

## Positioning

The differentiating claim is the **conjunction**, not any single element. A neighboring control panel can truthfully copy any one of these; the confirmed position is that Xylona holds all four at once.

1. **Single-binary self-hosting.** The Go binary embeds the frontend, SQLite is the only datastore, and migrations are embedded and applied on startup. No Compose stack, no external database, no separate web server.
2. **Controller plus node fleet.** One control plane commands remote `xylona-node` instances using certificate-fingerprint pinning, encrypted shared secrets, and one-time join tokens, without becoming hosting-provider software.
3. **Operational safety by default.** Checksum-verified mod installs where the provider advertises a checksum, staged and capacity-checked updates with a retained rollback executable, and AES-GCM encryption at rest for control-plane secrets under a documented recovery model.
4. **Approachable for a first-time admin.** A first-time self-hoster can succeed without cPanel-style clutter, while a repeat operator stays fast.

## Operating Context

Xylona runs on hardware the operator controls: a home machine, a VPS, or a small fleet. Both the controller and remote node install themselves as Windows services or systemd units through their `service install` CLI, or run in the foreground under an external supervisor. By default, the controller listens on all interfaces at `:8080`; set `HOST=localhost` to restrict it to loopback.

By default, state lives in `./data.sqlite`; `DB_FILE_PATH` can relocate it. Embedded migrations are applied automatically at startup. The database and `ENCRYPTION_KEY_BASE64` are a **matched recovery set**: neither alone can restore encrypted control-plane secrets. Built-in backups cover game-server data, not control-plane secrets.

Operator workflows, as routed in the app: lifecycle control, console, players, live map, file browser, metrics, configuration, settings, start command, mods, alerts, schedules, backups, and per-server access. Around those sit the games catalog and editor, node management, notifications, and admin surfaces for users, settings, and updates.

Supporting operational facts:

- `GET /api/health` is liveness only; `GET /api/ready` gates on a database ping; `/metrics` is Prometheus and is **disabled by default**.
- Mod provider integrity is uneven and stated honestly: PaperMC, Hangar, and Modrinth are checksum-verified; Thunderstore and Steam Workshop are best-effort.
- Palworld map imagery is optional, not shipped in releases, and downloaded on demand into `palworld-map-tiles`. Public map links are served from the controller's own listener, so visitors never hit the upstream tile host.
- Controller and node updates restart themselves; Windows services use a helper process to replace the locked executable and restart through SCM.

## Capabilities and Constraints

**Stack.** Go 1.27.0 backend; Bun 1.3.12 toolchain; Quasar 2 / Vue 3 SPA with Pinia and vue-router 5; Connect RPC over protobuf for the API; SQLite for storage. The production frontend bundle is embedded at `internal/webui/dist`, so the backend must be rebuilt after a frontend build.

**Notable frontend dependencies** that constrain design: Monaco (file and config editing), Leaflet (live maps), Chart.js via vue-chartjs (metrics), and `@fontsource` packages for the four identity typefaces.

**Browser target.** Modern evergreen browsers with native `BigInt`, including Safari 15.6 and later.

**Access model.** RBAC across users, roles, and permissions, with per-server access control and node-scoped operations.

**Terminology future work must preserve:** controller, node, game server, game (the definition or template a server is created from), mod provider, backup, schedule, alert.

**Platform tooling constraint.** On Windows, frontend installs and normal build tooling use Bun, but Vitest and Playwright still require Node-compatible execution until Bun's worker runtime compatibility catches up.

**Verification constraint.** For local browser verification, credentials may be read from `.env` when needed, but must never be printed, logged, or committed.

**Undecided.** Licensing and distribution terms are not settled; the repository carries no `LICENSE` file. Do not state or imply a license.

## Brand Commitments

The name **Xylona** and the author attribution (Clinton Collins) are fixed.

The four-typeface identity is already shipped and is binding: **Zen Dots**, **Goldman**, **Exo 2**, and **JetBrains Mono**, all self-hosted via `@fontsource` packages rather than a CDN. Role assignment for these faces belongs to `frontend/DESIGN.md`.

The product is **dark-only**. This is a product commitment, not a theming preference, and there is no light mode to design for.

Existing identity assets: `frontend/public/favicon.ico` and the icon set in `frontend/public/icons/`.

## Evidence on Hand

The public GitHub repository (`github.com/ClintonCollins/Xylona`) and its README are the only real evidence that exists. The README is unusually detailed on security posture, recovery, and provider integrity, and is a legitimate source to cite.

**Absences that future work must not fabricate:**

- No users, install counts, adoption figures, or community size.
- No testimonials, case studies, reviews, or named deployments.
- No benchmarks, performance numbers, or comparisons against other control panels.
- No press, funding, awards, or endorsements.
- No pricing, tiers, commercial offering, or hosted service.
- No uptime, SLA, support, or stability guarantees. The README's own Status section describes the project as evolving quickly and tells operators to verify and back up before promoting to production; do not write copy that contradicts that.

## Product Principles

1. **Operator understanding precedes automation.** Show state, and show what an action will do, before offering to do it for them. Lifecycle state, health, and errors are never hidden behind vague menus.
2. **Recoverable by design, and honest about the edges.** Risky operations carry a documented recovery path, and the product states plainly where a guarantee stops — as it already does for best-effort mod providers and for what backups do not cover.
3. **One binary, no ceremony.** Capability must not require additional infrastructure. If a feature would demand an external service to be useful, that cost is part of the feature's design.
4. **Scales without changing shape.** Moving from one machine to many nodes uses the same vocabulary, the same mental model, and the same screens. Fleet operation is not a separate product.
5. **Legible to a first-timer, fast for a regular.** Progressive disclosure serves both audiences from one interface rather than forking into "simple" and "advanced" modes.

## Accessibility & Inclusion

Prioritize readability, discoverability, keyboard navigation, and strong contrast. Optimize for real-world usability over rigid score-chasing, but do not ignore common needs such as reduced motion, color-blind-safe status cues, and clear focus states.

Status must never rely on color alone. Reduced-motion behavior is already implemented globally in `frontend/src/css/design-tokens.css` and must be preserved.

## Accepted Audit Boundaries

The following `v-html` usages are intentional and should not be flagged as XSS issues unless their trust boundary or escaping changes:

- `frontend/src/pages/game_servers/GameServerView.vue`: formatted output from the operator's authenticated game server; this source is an explicit trust boundary.
- `frontend/src/components/ClipBoardCopy.vue`: styled tooltip HTML from application-controlled props.
- `frontend/src/components/games/GameFormOverviewTab.vue`: command previews produced by `highlightCommand()`, which escapes command text before adding syntax-highlighting spans.
- `frontend/src/components/game_servers/ModDetailDialog.vue`: third-party description HTML sanitized with DOMPurify and an explicit allowlist before rendering.

Treat new or materially changed `v-html` as trust-boundary work. Escape or sanitize untrusted content before rendering it.
