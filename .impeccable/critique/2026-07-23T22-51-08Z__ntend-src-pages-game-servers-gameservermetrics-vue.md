---
target: game server metrics page
total_score: 25
p0_count: 0
p1_count: 2
timestamp: 2026-07-23T22-51-08Z
slug: ntend-src-pages-game-servers-gameservermetrics-vue
---
# Critique — Game Server Metrics (GameServerMetrics.vue)

Method: dual-agent (A: design review · B: deterministic detector)

## Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 3 | Rich 10-state pill + freshness, but "Latest Xs ago" text never re-renders as time passes |
| 2 | Match System / Real World | 3 | Strong units/copy; "RSS", "rollups" assume Linux-admin literacy |
| 3 | User Control and Freedom | 2 | No zoom/brush, no custom range, range resets to 1h every visit, fixed 60s refresh |
| 4 | Consistency and Standards | 3 | Token discipline exemplary; "peak" series is amber on CPU but red on latency/frame time |
| 5 | Error Prevention | 3 | Read-only page; stale-response guard; solid |
| 6 | Recognition Rather Than Recall | 2 | Event↔chart correlation requires memorizing timestamps across 3 screens; no threshold context |
| 7 | Flexibility and Efficiency | 1 | No URL state, keyboard shortcuts, export, or zoom — thin for daily power users |
| 8 | Aesthetic and Minimalist Design | 3 | Clean and dense; 9 permanent explainer sentences cost every scan |
| 9 | Error Recovery | 3 | Error banner with Retry; empty state with actionable CTA |
| 10 | Help and Documentation | 2 | Chart descriptions double as micro-help; no deeper explanation of resolution/coverage |
| **Total** | | **25/40** | **Acceptable — held back by interaction depth, not visual craft** |

## Anti-Patterns Verdict

LLM assessment: not AI slop — the data honesty (null-vs-zero discipline, capability-gated empty labels, coverage ratios) reads as genuinely engineered. But it plateaus at "competent Chart.js in cards": nine visually identical 2-col chart cards regardless of metric importance, stock Chart.js legend/tooltip, no crosshair, no annotations. The `.metrics-current` strip brushes the hero-metric template but escapes via its bordered table-strip execution.

Deterministic scan: 0 findings across all four files (exit 0). No hardcoded palette, gradient, or slop-rule hits — token discipline is airtight.

Browser overlays: skipped — no dev server was running and the page requires an authenticated backend session.

## Overall Impression

The engineering under this page is better than the experience on top of it. Telemetry integrity, the 10-state view model, and accessibility scaffolding are Grafana-class. But the page is a passive chart viewer, not the command center the brand promises: nothing correlates, nothing triages, nothing escalates when the server is in trouble. CPU at 99% renders identically to CPU at 3%. Biggest opportunity: turn nine isolated cards into one correlated, health-aware instrument.

## What's Working

1. **Telemetry integrity as a design feature** — null propagation surfaces as coverage %, "Unknown" instead of fake zeros, capability-aware empty states. Strongest trust signal on the page.
2. **The 10-state view model** — loading/error/no-data/warming-up/offline/node-unavailable/collector-error/stale/reconnecting/live, each with calm plain-language detail.
3. **Accessibility scaffolding in charts** — sr-only summaries, keyboard-focusable details tables, real reduced-motion handling.

## Priority Issues

- **[P1] No cross-chart correlation** — tooltips are per-chart; lifecycle events render only as a bottom-of-page list. Fix: synced vertical crosshair across all charts + events as slim vertical markers on every chart (data already fetched together). Suggested: /impeccable craft (correlation layer).
- **[P1] No health semantics** — values render neutrally regardless of severity; the only state shown is collection state, not server health. Fix: threshold semantics on current-strip cells (glyph + text, not color alone) and faint warning/danger band regions on charts; tokens already exist. Suggested: /impeccable craft or bolder.
- **[P2] Permanently dead charts for unsupported capabilities** — games without FPS/query telemetry show full 210px apology panels forever. Fix: collapse to one-line compact rows when `querySupported === false`. Suggested: /impeccable distill.
- **[P2] Chart palette fights status semantics** — `--xy-chart-3` IS danger red, `--xy-chart-4` IS warning amber; directory size plots in alarm red. Fix: neutral series tokens; reserve red/amber for real breaches. Suggested: /impeccable colorize.
- **[P2] View state not shareable/durable** — selected range lives only in a ref; resets to 1h on every visit; no link handoff. Fix: sync to `route.query.range`. Suggested: /impeccable harden.

## Persona Red Flags

**Alex (power user):** re-selects 24h every morning (no URL/persisted state); no drag-zoom; 9 clicks to see all min/avg/max summaries; fixed 60s refresh; no export; no keyboard shortcuts; two dead panels between him and the timeline.

**Sam (accessibility):** timeline event severity is color-only (9px dot, violates house rule); no aria-live for status pill flips or error banner; frozen "Xs ago" text; trend shape hover-only; verify contrast of unselected toggle labels and muted xs text.

## Minor Observations

- Empty-state CTA forces 24h even when user is already on 7d/30d/90d — make conditional.
- Live dot is static; a token-gated pulse (already reduced-motion-safe) would cheaply sell "live".
- Full dataset rebuild every ~3s live tick; legend-hidden state may reset — verify.
- Timeline detail fallback prints raw kind string ("lifecycle").
- Degenerate x-axis with exactly one sample.
- "Configured memory" cell is config, not telemetry — merge into RSS cell as denominator.
- Unstyled default `<details>` disclosure triangle.

## Questions to Consider

1. What if the nine charts became one instrument — a shared-x-axis "flight recorder" column with synced crosshair and event markers piercing every lane?
2. What if the page triaged instead of listed — troubled metrics promoted, healthy ones collapsed to sparkline rows?
3. What if a metrics view were a shareable object — range + zoom + pinned timestamp in the URL, pasteable into the community Discord?
