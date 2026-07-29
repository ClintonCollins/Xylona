## How to build with Xylona

**This design system ships styling, not components.** Xylona's real UI is Vue 3 + Quasar, which cannot be rendered here, so `_ds_bundle.js` is intentionally empty and there is nothing on `window.Xylona`. Build screens out of ordinary HTML/JSX elements and style them **entirely** with the CSS custom properties below. Do not import components from this library — there are none. Do not invent a class vocabulary; Xylona's idiom is `var(--xy-*)` tokens plus the short utility list at the end.

**Dark-only.** There is no light theme. The page floor is `var(--xy-base)` (#0d0e0f) — never `#000`, never white. Every neutral is cool-tinted; a warm or fully desaturated gray reads as a different product.

### The token vocabulary

Depth comes from **stacked surfaces first, borders second, shadows last**:

- **Surfaces** — `--xy-base` (page floor) → `--xy-surface-0`, `--xy-surface-1` (primary app surfaces) → `--xy-surface-2` (raised panels, toolbars) → `--xy-surface-3`, `--xy-surface-4` (hover, selected).
- **Text** — `--xy-text-primary`, `--xy-text-secondary`, `--xy-text-muted`; on colored fills use `--xy-text-on-color`, `--xy-text-on-dark`, `--xy-text-on-bright`.
- **Borders** — `--xy-border` (10% white), `--xy-border-hover` (18%), `--xy-border-active` (40% blue). Focus rings use `--xy-focus-ring`.
- **Semantic** — `--xy-primary` (blue), `--xy-accent` (cyan), `--xy-success`, `--xy-danger`, `--xy-warning`, `--xy-info`, `--xy-purple`. Each has `-hover` and most have `-darker`, `-bg`, `-border`, `-muted` variants (e.g. `--xy-success-bg`, `--xy-danger-border`).
- **Spacing** — `--xy-space-2xs` (2px), `-xs` (4), `-sm` (8), `-base` (12), `-md` (16), `-lg` (24), `-xl` (32), `-2xl` (48), `-3xl` (64).
- **Radius** — `--xy-radius-sm` (4px, compact controls), `-md` (6px, buttons/inputs), `-lg` (8px, cards/dialogs), `-xl` (12px, hero panels), `-pill` (999px). A literal `1rem` or `999px` is a defect.
- **Type** — sizes `--xy-font-size-2xs` (0.6875rem) through `-xs`, `-sm`, `-base`, `-lg`, `-xl`, `-2xl` (2rem); line heights `--xy-line-height-tight|base|relaxed`.
- **Shadows** — `--xy-shadow-sm|md|lg|xl|2xl`, plus `--xy-shadow-sticky-lg`. Shadows communicate layering, never gloss; if one isn't separating planes, remove it. No glassmorphism or backdrop blur.
- **Motion** — `--xy-transition-fast` / `--xy-transition-base` with `--xy-ease-standard`. Animate transform and opacity, not layout.
- **Data** — chart categories `--xy-chart-1`…`-5` and `--xy-category-1`…`-8`; time series `--xy-series-1|2|3`, `--xy-series-neutral`, `--xy-series-limit`, `--xy-chart-grid`.
- **Layering** — `--xy-z-sticky`, `-drawer`, `-overlay`, `-fullscreen`, `-notification`. Toolbars are `--xy-toolbar-height` (50px).

### Typography

Four self-hosted families, all shipped: `--xy-font-brand` (Zen Dots), `--xy-font-display` / `--xy-font-heading` (Goldman), `--xy-font-body` (Exo 2), `--xy-font-mono` (JetBrains Mono).

- Display faces (Zen Dots, Goldman) are for identity and page titles **only**, and lose legibility below ~0.95rem. Caption and micro text is always Exo 2 or JetBrains Mono.
- `--xy-font-control` deliberately resolves to `--xy-font-body` — buttons and toolbar actions render in Exo 2 at weight 600, not Goldman.
- Anything an operator compares character by character — ports, IDs, hashes, versions, paths, metric values — is `--xy-font-mono`.

### Rules that change what you build

- **Signal reserve.** Blue and cyan carry interactive or stateful meaning. Spending accent color on decoration leaves the next genuinely stateful element with nothing to say.
- **Single primary.** One primary button per screen. Primary is `--xy-primary` background with `--xy-text-on-color`, `--xy-radius-md`, 4px 12px padding. Secondary is `--xy-surface-2` with `--xy-text-primary`.
- **Status is never color alone.** Pair every status with text, icon, or shape — a green dot by itself is not a running state.
- **Series is not status.** Ordinary time series use `--xy-series-*`. Red and amber are reserved for real state: threshold breaches and configured limits.
- **Cards don't nest.** A card inside a card means the outer container should have been an unframed layout band. Cards: `--xy-surface-1`, `--xy-radius-lg`, `--xy-border`, 16px padding. For dashboards reach first for unframed bands, tables, and status strips — not card grids.
- **No hero bands.** This is an operational product UI; dashboards lead with state, not landing-page composition.

### Shipped utility classes

Backgrounds `.bg-xy-base`, `.bg-xy-surface-0` … `.bg-xy-surface-4`; text `.text-xy-primary|secondary|muted`; fonts `.font-brand|display|body|mono`; status tints (background + matching border) `.bg-xy-success-tint`, `.bg-xy-danger-tint`, `.bg-xy-warning-tint`, `.bg-xy-info-tint`; layout `.xy-page-content` (responsive content well), `.xy-page-header`, `.xy-section-overline` (uppercase label), `.xy-nav-divider`.

`.xy-page-title` and `.xy-page-actions` are **descendant-scoped** — they only take effect inside `.xy-page-header`. Ignore the legacy aliases (`.bg-toolbar`, `.text-main-brighter`, and siblings); they are being migrated out.

### Where the truth lives

Read `styles.css` and its imports for the authoritative token values, and `guidelines/DESIGN.md` for the full rationale, named rules, and anti-references.

### Idiomatic example

```jsx
<div className="xy-page-content">
  <div className="xy-page-header">
    <h1 className="xy-page-title">Survival Server</h1>
    <div className="xy-page-actions">
      <button style={{
        background: 'var(--xy-primary)', color: 'var(--xy-text-on-color)',
        font: '600 var(--xy-font-size-base)/1.25 var(--xy-font-control)',
        padding: '4px 12px', borderRadius: 'var(--xy-radius-md)', border: 'none',
        transition: 'background var(--xy-transition-fast)',
      }}>Restart</button>
    </div>
  </div>

  <div className="xy-section-overline">Status</div>
  <div style={{
    background: 'var(--xy-surface-1)', border: '1px solid var(--xy-border)',
    borderRadius: 'var(--xy-radius-lg)', padding: 'var(--xy-space-md)',
    display: 'flex', gap: 'var(--xy-space-md)', alignItems: 'baseline',
  }}>
    <span className="bg-xy-success-tint" style={{
      color: 'var(--xy-success)', borderRadius: 'var(--xy-radius-pill)',
      padding: '2px 10px', fontSize: 'var(--xy-font-size-2xs)', fontWeight: 600,
    }}>● RUNNING</span>
    <span style={{ fontFamily: 'var(--xy-font-mono)', fontSize: 'var(--xy-font-size-sm)',
                   color: 'var(--xy-text-secondary)' }}>10.0.0.4:27015</span>
  </div>
</div>
```
