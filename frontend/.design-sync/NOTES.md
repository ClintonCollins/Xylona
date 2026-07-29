# design-sync notes — Xylona

## Scope: this is a tokens-only sync, and it cannot be otherwise

Claude Design consumes **React** design systems (`non-storybook/SKILL.md`: "a non-React
DS has nothing for the claude.ai/design agent to build with"). Xylona's frontend is
Vue 3 + Quasar — 108 `.vue` SFCs, zero `.jsx`/`.tsx`, no `react` dependency — and it is
an *application*, not a published component library with a `dist/` entry.

So no Xylona component can ever ship to Claude Design. What ships instead is the
styling layer: tokens, the four brand fonts, the shipped utility classes, DESIGN.md,
and the conventions header. The design agent builds with generic React elements but
styles them with Xylona's real visual language.

The converter supports this natively — `lib/source-kit.mjs` emits
`[ZERO_MATCH] no component exports — treating as tokens-only DS` and sets
`tokensOnly: true`. That is the expected, healthy path here, **not** a failure to fix.

## Build command

Run from `frontend/`:

```
node .ds-sync/resync.mjs --config .design-sync/config.json \
  --node-modules ./.ds-sync/node_modules --entry ./.ds-entry.mjs --out ./ds-bundle
```

Three non-obvious flags, all load-bearing:

- **`--entry ./.ds-entry.mjs`** — a committed empty ES module. It exists so `PKG_DIR`
  resolves to `frontend/` (package-build.mjs walks up from the entry to the nearest
  named `package.json`). Without it, `PKG_DIR` becomes `<node-modules>/xylona`, which
  doesn't exist, and every package-relative config path breaks. It also becomes the
  empty-bodied `_ds_bundle.js`.
- **`--node-modules ./.ds-sync/node_modules`** — `vendorReact()` is called
  unconditionally (package-build.mjs:214) even for a tokens-only DS, so `react` and
  `react-dom` must resolve. They are installed **only** into the gitignored
  `.ds-sync/`, deliberately: Xylona's own `node_modules` is never polluted with React.
  Fresh clone → recreate with:
  `npm i --prefix .ds-sync esbuild ts-morph @types/react react react-dom`
- **`cssEntry: src/css/design-tokens.css`** — *not* `app.css`. See below.

## Why cssEntry is design-tokens.css, not app.css

`lib/css.mjs` `copyTokens()` returns immediately unless `tokensPkg` is set, and it
resolves tokens from `join(nodeModules, tokensPkg)` — i.e. it assumes tokens live in
an npm package. Xylona keeps them in app source, so `tokensGlob` alone is a no-op and
`tokens/` stays empty.

The first attempt used `cssEntry: src/css/app.css`. That copies app.css to
`_ds_bundle.css`, but app.css's `@import 'design-tokens.css'` is relative and the
token file was never copied alongside it — so the closure silently resolved to no
tokens at all. Pointing `cssEntry` straight at `design-tokens.css` makes the token
file itself `_ds_bundle.css`, which `styles.css` already `@import`s. Verified: 156
tokens defined.

Little is lost — `design-tokens.css` already carries the layout utilities
(`.xy-page-content`, `.xy-page-header`, `.xy-section-overline`). What app.css adds on
top is mostly Quasar-specific (`.q-notification__*`, `.q-item`), which is noise to a
React design agent.

## Known warns (expected — do not chase on re-sync)

- **`[FONT_MISSING] "Oxygen Mono"`** — a *fallback-only* family in
  `--xy-font-mono: 'JetBrains Mono', 'Oxygen Mono', monospace`. The primary family
  (JetBrains Mono) ships in three weights and was verified loading in-browser, so no
  design renders in a fallback font. Shipping Oxygen Mono would be dead weight.
- **`[DTS_REACT] @types/react not found`** — irrelevant here. It only affects prop
  extraction for React components, and there are zero components.

## Verification performed

`package-validate.mjs` exits 0, but its render check is 0/0 for a tokens-only DS — it
proves nothing about the styling layer. So the token/font layer was verified directly
in headless chromium — `node .design-sync/verify-tokens.mjs ./ds-bundle out.png`,
run from `frontend/` (it resolves `playwright` from the repo's own devDependency,
and reuses the chromium already cached by the e2e suite): all four families
report `document.fonts.check()` true and render visually distinct, `--xy-*` tokens
resolve (`--xy-base` → `rgb(13,14,15)` on body), utility classes apply, and zero
network requests failed. Re-run it after any change to the token file or font set.

## Re-sync risks

- **The conventions header can rot.** `.design-sync/conventions.md` enumerates ~54
  token names and 15 utility classes by hand. If tokens are renamed or removed in
  `design-tokens.css`, the header will confidently name things that no longer resolve
  and the design agent will emit silently unstyled output. Re-validate the names
  against `ds-bundle/_ds_bundle.css` on every re-sync; never rewrite the file wholesale.
- **The font list is pinned by hand.** `extraFonts` in config.json lists the 11
  `@fontsource` latin CSS files that `src/css/app.css` imports. If app.css gains,
  drops, or reweights a family, config.json must be updated to match — nothing
  cross-checks them.
- **React version drift.** `_vendor/` ships whatever React `.ds-sync` resolved
  (19.2.8 at time of writing). Nothing renders with it in a tokens-only bundle, so
  drift is harmless, but it is why `_vendor/` appears in the upload at all.
- **If Xylona ever grows a React component library**, this config stops being right:
  `[ZERO_MATCH]` would no longer fire, and the scope decision above should be revisited
  from the top rather than patched.
